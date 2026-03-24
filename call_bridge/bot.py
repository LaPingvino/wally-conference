"""Main Maubot plugin class for Wally Conference (Call Bridge)."""

from __future__ import annotations

import asyncio
import time
import uuid
from urllib.parse import quote, urlencode

from maubot import Plugin
from maubot.handlers import command, web
from mautrix.types import MessageEvent, RoomID
from aiohttp import web as aiohttp_web
from mautrix.util.config import BaseProxyConfig

from .config import Config
from .db import (
    upgrade_table,
    create_session,
    count_active_sessions,
    count_all_active_sessions,
    count_all_active_breakouts,
    count_active_breakouts,
    get_active_breakouts,
    get_session,
    delete_session,
    get_all_sessions_in_room,
)
from .identity import livekit_room_alias, livekit_identity
from .jwt_service import make_guest_jwt
from .membership import send_call_member, clear_call_member
from .security import (
    RateLimiter,
    validate_room_id,
    validate_display_name,
    add_cors_headers,
    cors_preflight,
)
from .webhook import create_webhook_receiver, handle_webhook
from .cleanup import cleanup_loop
from .breakout import (
    create_breakout as do_create_breakout,
    end_breakout as do_end_breakout,
    move_to_breakout as do_move_to_breakout,
)


class CallBridgeBot(Plugin):
    """Maubot plugin that bridges guest callers into MatrixRTC/LiveKit calls."""

    rate_limiter: RateLimiter
    _cleanup_task: asyncio.Task | None

    @classmethod
    def get_config_class(cls) -> type[BaseProxyConfig]:
        return Config

    @classmethod
    def get_db_upgrade_table(cls) -> None:
        return upgrade_table

    async def start(self) -> None:
        await super().start()
        self.config.load_and_update()
        self.rate_limiter = RateLimiter(
            max_per_minute=self.config["rate_limit_per_minute"]
        )
        self._webhook_receiver = create_webhook_receiver(
            api_key=self.config["livekit_api_key"],
            api_secret=self.config["livekit_api_secret"],
        )
        self._setup_routes()
        # Start background cleanup task
        cleanup_interval = self.config["cleanup_interval"]
        self._cleanup_task = asyncio.create_task(
            cleanup_loop(self.database, self.client.api, cleanup_interval)
        )
        self.log.info("Wally Conference plugin started")

    def _setup_routes(self) -> None:
        """Register HTTP routes on the maubot webapp sub-application."""
        self.webapp.router.add_route("OPTIONS", "/join", self._options_handler)
        self.webapp.router.add_post("/join", self.handle_guest_join)
        self.webapp.router.add_post("/webhook", self.handle_livekit_webhook)
        self.webapp.router.add_get("/health", self.handle_health)
        self.webapp.router.add_post("/breakout/create", self.handle_breakout_create)
        self.webapp.router.add_post("/breakout/move", self.handle_breakout_move)

    # ── Permission helpers ────────────────────────────────────

    def _is_admin_room(self, room_id: str) -> bool:
        """Check if a room is an admin room (or admin_rooms is empty = all rooms)."""
        admin_rooms = self.config["admin_rooms"]
        return not admin_rooms or room_id in admin_rooms

    async def _is_moderator(self, room_id: str, user_id: str) -> bool:
        """Check if user has moderator power level (>= 50) in the room."""
        try:
            levels = await self.client.get_state_event(room_id, "m.room.power_levels")
            user_pl = levels.get("users", {}).get(user_id, levels.get("users_default", 0))
            return user_pl >= 50
        except Exception:
            return False

    async def _require_moderator(self, evt: MessageEvent) -> bool:
        """Check moderator permission, reply with error if denied. Returns True if allowed."""
        if not await self._is_moderator(evt.room_id, evt.sender):
            await evt.reply("Permission denied: moderator power level required.")
            return False
        return True

    # ══════════════════════════════════════════════════════════
    #  Bot Matrix commands (!wc)
    # ══════════════════════════════════════════════════════════

    @command.new("wc", help="Wally Conference commands")
    async def wc(self, evt: MessageEvent) -> None:
        """Root command group. Shows help if called without subcommand."""
        await evt.reply(
            "Wally Conference commands:\n"
            "- `!wc status` — show bot status\n"
            "- `!wc link` — generate guest join link\n"
            "- `!wc invite <room_id>` — bot joins a room\n"
            "- `!wc leave <room_id>` — bot leaves a room\n"
            "- `!wc kick <session_id>` — remove a guest\n"
            "- `!wc config` — show configuration\n"
            "- `!wc breakout create <topic>` — create breakout\n"
            "- `!wc breakout list` — list active breakouts\n"
            "- `!wc breakout end <id>` — end breakout\n"
            "- `!wc breakout move <session_id> <breakout_id>` — move guest"
        )

    # ── !wc status ────────────────────────────────────────────

    @wc.subcommand("status", help="Show bot status")
    async def cmd_status(self, evt: MessageEvent) -> None:
        """Show active guests count, active breakouts, and bot health."""
        guests = await count_all_active_sessions(self.database)
        breakouts = await count_all_active_breakouts(self.database)
        lk_configured = bool(
            self.config["livekit_api_key"] and self.config["livekit_api_secret"]
        )

        room_guests = await count_active_sessions(self.database, evt.room_id)
        room_breakouts = await count_active_breakouts(self.database, evt.room_id)

        await evt.reply(
            f"**Wally Conference Status**\n\n"
            f"Global: {guests} active guests, {breakouts} active breakouts\n"
            f"This room: {room_guests} guests, {room_breakouts} breakouts\n"
            f"LiveKit configured: {'yes' if lk_configured else 'NO'}\n"
            f"Bot user: `{self.client.mxid}`"
        )

    # ── !wc link ──────────────────────────────────────────────

    @wc.subcommand("link", help="Generate a guest join link for this room")
    async def cmd_link(self, evt: MessageEvent) -> None:
        """Generate and send a guest join link for the current room."""
        # The join URL depends on where maubot is hosted.
        # We provide the API endpoint info so the user can build a guest page.
        room_id = evt.room_id
        await evt.reply(
            f"**Guest Join Info**\n\n"
            f"Room ID: `{room_id}`\n\n"
            f"Guests can join by POSTing to the `/join` endpoint with:\n"
            f"```json\n"
            f'{{"room_id": "{room_id}", "display_name": "Guest Name"}}\n'
            f"```\n\n"
            f"The response includes `ec_url` — a ready-to-open Element Call link."
        )

    # ── !wc invite <room_id> ──────────────────────────────────

    @wc.subcommand("invite", help="Bot joins a room")
    @command.argument("target_room", required=True)
    async def cmd_invite(self, evt: MessageEvent, target_room: str) -> None:
        """Bot joins the specified room. Requires moderator in current room."""
        if not await self._require_moderator(evt):
            return

        err = validate_room_id(target_room)
        if err:
            await evt.reply(f"Invalid room ID: {err}")
            return

        try:
            await self.client.join_room(target_room)
            await evt.reply(f"Joined room `{target_room}`.")
        except Exception as e:
            await evt.reply(f"Failed to join room: {e}")

    # ── !wc leave <room_id> ───────────────────────────────────

    @wc.subcommand("leave", help="Bot leaves a room")
    @command.argument("target_room", required=True)
    async def cmd_leave(self, evt: MessageEvent, target_room: str) -> None:
        """Bot leaves the specified room. Requires moderator."""
        if not await self._require_moderator(evt):
            return

        err = validate_room_id(target_room)
        if err:
            await evt.reply(f"Invalid room ID: {err}")
            return

        try:
            await self.client.leave_room(target_room)
            await evt.reply(f"Left room `{target_room}`.")
        except Exception as e:
            await evt.reply(f"Failed to leave room: {e}")

    # ── !wc kick <session_id> ─────────────────────────────────

    @wc.subcommand("kick", help="Remove a guest by session ID")
    @command.argument("session_id", required=True)
    async def cmd_kick(self, evt: MessageEvent, session_id: str) -> None:
        """Remove a guest: clear their call.member state event and delete DB session."""
        if not await self._require_moderator(evt):
            return

        session = await get_session(self.database, session_id)
        if session is None:
            await evt.reply(f"Session `{session_id}` not found.")
            return

        # Clear call.member
        try:
            await clear_call_member(
                self.client.api, session["room_id"], session["state_key"]
            )
        except Exception as e:
            self.log.error(f"Failed to clear call.member for kicked session: {e}")

        await delete_session(self.database, session_id)
        await evt.reply(
            f"Kicked guest `{session['display_name']}` (session `{session_id}`)."
        )

    # ── !wc config ────────────────────────────────────────────

    @wc.subcommand("config", help="Show current plugin configuration")
    async def cmd_config(self, evt: MessageEvent) -> None:
        """Show current config with secrets redacted. Admin rooms only."""
        if not self._is_admin_room(evt.room_id):
            await evt.reply("This command is only available in admin rooms.")
            return

        if not await self._require_moderator(evt):
            return

        # Show config with secrets redacted
        cfg = {
            "livekit_url": self.config["livekit_url"],
            "livekit_api_key": self.config["livekit_api_key"][:4] + "****"
            if self.config["livekit_api_key"]
            else "(not set)",
            "livekit_api_secret": "****" if self.config["livekit_api_secret"] else "(not set)",
            "livekit_service_url": self.config["livekit_service_url"],
            "guest_token_ttl": self.config["guest_token_ttl"],
            "max_guests_per_room": self.config["max_guests_per_room"],
            "allowed_origins": self.config["allowed_origins"],
            "rate_limit_per_minute": self.config["rate_limit_per_minute"],
            "auto_join_invites": self.config["auto_join_invites"],
            "admin_rooms": self.config["admin_rooms"] or "(all rooms)",
            "ec_base_url": self.config["ec_base_url"],
            "cleanup_interval": self.config["cleanup_interval"],
            "max_breakouts_per_room": self.config["max_breakouts_per_room"],
        }

        lines = ["**Wally Conference Configuration**\n"]
        for k, v in cfg.items():
            lines.append(f"- `{k}`: `{v}`")

        await evt.reply("\n".join(lines))

    # ══════════════════════════════════════════════════════════
    #  Breakout bot commands (!wc breakout)
    # ══════════════════════════════════════════════════════════

    @wc.subcommand("breakout", help="Breakout room management")
    async def cmd_breakout(self, evt: MessageEvent) -> None:
        """Breakout subcommand group. Shows help if no sub-subcommand."""
        await evt.reply(
            "Breakout commands:\n"
            "- `!wc breakout create <topic>` — create a breakout room\n"
            "- `!wc breakout list` — list active breakouts\n"
            "- `!wc breakout end <id>` — end a breakout room\n"
            "- `!wc breakout move <session_id> <breakout_id>` — move guest to breakout"
        )

    @cmd_breakout.subcommand("create", help="Create a breakout room")
    @command.argument("topic", required=True, pass_raw=True)
    async def cmd_breakout_create(self, evt: MessageEvent, topic: str) -> None:
        """Create a breakout room for the current Matrix room."""
        if not await self._require_moderator(evt):
            return

        topic = topic.strip()
        if not topic:
            await evt.reply("Usage: `!wc breakout create <topic>`")
            return

        # Check breakout capacity
        max_breakouts = self.config["max_breakouts_per_room"]
        active = await count_active_breakouts(self.database, evt.room_id)
        if active >= max_breakouts:
            await evt.reply(
                f"Breakout capacity reached ({max_breakouts} per room)."
            )
            return

        result = await do_create_breakout(
            db=self.database,
            matrix_room_id=evt.room_id,
            topic=topic,
            created_by=evt.sender,
        )

        await evt.reply(
            f"**Breakout room created**\n\n"
            f"ID: `{result['breakout_id']}`\n"
            f"Topic: {result['topic']}\n"
            f"LiveKit room: `{result['lk_alias'][:16]}...`\n\n"
            f"Use `!wc breakout move <session_id> {result['breakout_id']}` to move guests."
        )

    @cmd_breakout.subcommand("list", help="List active breakout rooms")
    async def cmd_breakout_list(self, evt: MessageEvent) -> None:
        """List active breakout rooms for the current Matrix room."""
        breakouts = await get_active_breakouts(self.database, evt.room_id)

        if not breakouts:
            await evt.reply("No active breakout rooms in this room.")
            return

        lines = ["**Active Breakout Rooms**\n"]
        for br in breakouts:
            lines.append(
                f"- `{br['id']}` — {br['topic'] or '(no topic)'} "
                f"(created by `{br['created_by']}`)"
            )

        await evt.reply("\n".join(lines))

    @cmd_breakout.subcommand("end", help="End a breakout room")
    @command.argument("breakout_id", required=True)
    async def cmd_breakout_end(self, evt: MessageEvent, breakout_id: str) -> None:
        """End a breakout room, clearing all guest sessions in it."""
        if not await self._require_moderator(evt):
            return

        try:
            cleaned = await do_end_breakout(
                db=self.database,
                client_api=self.client.api,
                breakout_id=breakout_id.strip(),
            )
            await evt.reply(
                f"Breakout `{breakout_id}` ended. "
                f"Cleaned up {cleaned} guest session(s)."
            )
        except ValueError as e:
            await evt.reply(f"Error: {e}")

    @cmd_breakout.subcommand("move", help="Move a guest to a breakout room")
    @command.argument("session_id", required=True)
    @command.argument("breakout_id", required=True)
    async def cmd_breakout_move(
        self, evt: MessageEvent, session_id: str, breakout_id: str
    ) -> None:
        """Move a guest session to a breakout room, issuing a new JWT."""
        if not await self._require_moderator(evt):
            return

        try:
            result = await do_move_to_breakout(
                db=self.database,
                session_id=session_id.strip(),
                breakout_id=breakout_id.strip(),
                client_api=self.client.api,
                lk_api_key=self.config["livekit_api_key"],
                lk_api_secret=self.config["livekit_api_secret"],
                livekit_url=self.config["livekit_url"],
                ec_base_url=self.config["ec_base_url"],
                ttl_seconds=self.config["guest_token_ttl"],
            )
            await evt.reply(
                f"Moved session `{session_id}` to breakout `{breakout_id}`.\n"
                f"New EC URL: {result['ec_url']}"
            )
        except ValueError as e:
            await evt.reply(f"Error: {e}")

    # ══════════════════════════════════════════════════════════
    #  CORS preflight
    # ══════════════════════════════════════════════════════════

    async def _options_handler(
        self, request: aiohttp_web.Request
    ) -> aiohttp_web.Response:
        return cors_preflight(self.config["allowed_origins"])

    # ══════════════════════════════════════════════════════════
    #  HTTP endpoints
    # ══════════════════════════════════════════════════════════

    # ── Guest join ───────────────────────────────────────────

    async def handle_guest_join(
        self, request: aiohttp_web.Request
    ) -> aiohttp_web.Response:
        """Handle a guest join request.

        Validates input, rate-limits, checks room membership and capacity,
        generates credentials, sends the call.member state event, and returns
        a JSON payload with everything the guest needs to connect.
        """
        allowed_origins = self.config["allowed_origins"]

        # Rate limit by IP
        remote_ip = request.remote or "unknown"
        if not self.rate_limiter.check(remote_ip):
            return add_cors_headers(
                aiohttp_web.json_response(
                    {"error": "Rate limited"}, status=429
                ),
                allowed_origins,
            )

        # Parse body
        try:
            body = await request.json()
        except Exception:
            return add_cors_headers(
                aiohttp_web.json_response(
                    {"error": "Invalid JSON body"}, status=400
                ),
                allowed_origins,
            )

        room_id = body.get("room_id", "")
        raw_name = body.get("display_name", "")
        breakout_id = body.get("breakout_id")

        # Validate room_id
        room_err = validate_room_id(room_id)
        if room_err:
            return add_cors_headers(
                aiohttp_web.json_response(
                    {"error": room_err}, status=400
                ),
                allowed_origins,
            )

        # Validate display_name
        display_name, name_err = validate_display_name(raw_name)
        if name_err:
            return add_cors_headers(
                aiohttp_web.json_response(
                    {"error": name_err}, status=400
                ),
                allowed_origins,
            )

        # Check bot is in the room
        try:
            members = await self.client.get_joined_members(room_id)
            if self.client.mxid not in members:
                raise KeyError("bot not in members")
        except Exception:
            return add_cors_headers(
                aiohttp_web.json_response(
                    {"error": "Bot not in room"}, status=403
                ),
                allowed_origins,
            )

        # Check guest capacity
        max_guests = self.config["max_guests_per_room"]
        active_count = await count_active_sessions(self.database, room_id)
        if active_count >= max_guests:
            return add_cors_headers(
                aiohttp_web.json_response(
                    {"error": "Guest capacity reached"}, status=429
                ),
                allowed_origins,
            )

        # Generate identifiers
        session_id = str(uuid.uuid4())
        device_id = f"GUEST_{uuid.uuid4().hex[:8]}"
        bot_mxid = self.client.mxid

        # Compute LiveKit room alias and participant identity
        lk_room = livekit_room_alias(room_id)
        lk_ident = livekit_identity(bot_mxid, device_id, session_id)

        # Compute state key: _@bot:server_DEVICE_ID
        state_key = f"_{bot_mxid}_{device_id}"

        # Token TTL
        ttl_seconds = self.config["guest_token_ttl"]
        expires_ms = ttl_seconds * 1000

        # Issue LiveKit JWT
        jwt_token = make_guest_jwt(
            lk_api_key=self.config["livekit_api_key"],
            lk_api_secret=self.config["livekit_api_secret"],
            livekit_room_alias=lk_room,
            participant_identity=lk_ident,
            participant_name=display_name,
            ttl_seconds=ttl_seconds,
        )

        # Compute expiry timestamp
        expires_at = int(time.time()) + ttl_seconds

        # Store session in DB
        await create_session(
            db=self.database,
            session_id=session_id,
            room_id=room_id,
            bot_user_id=bot_mxid,
            device_id=device_id,
            display_name=display_name,
            lk_identity=lk_ident,
            lk_room=lk_room,
            state_key=state_key,
            expires_at=expires_at,
            breakout_id=breakout_id,
        )

        # Send call.member state event
        lk_service_url = self.config["livekit_service_url"]
        try:
            await send_call_member(
                client_api=self.client.api,
                room_id=room_id,
                state_key=state_key,
                device_id=device_id,
                session_id=session_id,
                lk_service_url=lk_service_url,
                expires_ms=expires_ms,
            )
        except Exception as e:
            self.log.error(f"Failed to send call.member event: {e}")
            # Still return the JWT so the guest can connect to LiveKit
            # even if the Matrix state event failed

        # Build EC URL
        livekit_url = self.config["livekit_url"]
        ec_base = self.config["ec_base_url"]
        ec_params = urlencode(
            {
                "embed": "true",
                "widgetId": f"guest-{session_id[:8]}",
                "roomId": room_id,
                "livekitToken": jwt_token,
                "livekitRoom": lk_room,
                "livekitUrl": livekit_url,
                "displayName": display_name,
                "skipLobby": "true",
                "header": "none",
            },
            quote_via=quote,
        )
        ec_url = f"{ec_base}?{ec_params}"

        response_data = {
            "jwt": jwt_token,
            "livekit_url": livekit_url,
            "livekit_room": lk_room,
            "session_id": session_id,
            "ec_url": ec_url,
            "expires_at": expires_at,
        }

        return add_cors_headers(
            aiohttp_web.json_response(response_data, status=200),
            allowed_origins,
        )

    # ── Webhook ────────────────────────────────────────────────

    async def handle_livekit_webhook(
        self, request: aiohttp_web.Request
    ) -> aiohttp_web.Response:
        """Handle LiveKit webhook events (participant_left, room_finished, etc.)."""
        return await handle_webhook(
            request,
            self._webhook_receiver,
            self.database,
            self.client.api,
        )

    # ── Health ───────────────────────────────────────────────

    async def handle_health(
        self, request: aiohttp_web.Request
    ) -> aiohttp_web.Response:
        """Health check endpoint."""
        guests = await count_all_active_sessions(self.database)
        breakouts = await count_all_active_breakouts(self.database)
        lk_ok = bool(
            self.config["livekit_api_key"] and self.config["livekit_api_secret"]
        )
        return aiohttp_web.json_response({
            "status": "ok",
            "active_guests": guests,
            "active_breakouts": breakouts,
            "matrix_connected": True,
            "livekit_configured": lk_ok,
        })

    # ── Breakout HTTP endpoints ──────────────────────────────

    async def handle_breakout_create(
        self, request: aiohttp_web.Request
    ) -> aiohttp_web.Response:
        """Create a breakout room via HTTP."""
        allowed_origins = self.config["allowed_origins"]

        try:
            body = await request.json()
        except Exception:
            return add_cors_headers(
                aiohttp_web.json_response(
                    {"error": "Invalid JSON body"}, status=400
                ),
                allowed_origins,
            )

        room_id = body.get("room_id", "")
        topic = body.get("topic", "").strip()
        user_id = body.get("user_id", "")

        room_err = validate_room_id(room_id)
        if room_err:
            return add_cors_headers(
                aiohttp_web.json_response({"error": room_err}, status=400),
                allowed_origins,
            )

        if not topic:
            return add_cors_headers(
                aiohttp_web.json_response(
                    {"error": "topic is required"}, status=400
                ),
                allowed_origins,
            )

        # Check breakout capacity
        max_breakouts = self.config["max_breakouts_per_room"]
        active = await count_active_breakouts(self.database, room_id)
        if active >= max_breakouts:
            return add_cors_headers(
                aiohttp_web.json_response(
                    {"error": "Breakout capacity reached"}, status=429
                ),
                allowed_origins,
            )

        result = await do_create_breakout(
            db=self.database,
            matrix_room_id=room_id,
            topic=topic,
            created_by=user_id or "http-api",
        )

        return add_cors_headers(
            aiohttp_web.json_response({
                "breakout_id": result["breakout_id"],
                "livekit_room": result["lk_alias"],
                "topic": result["topic"],
            }),
            allowed_origins,
        )

    async def handle_breakout_move(
        self, request: aiohttp_web.Request
    ) -> aiohttp_web.Response:
        """Move a guest to a breakout room via HTTP."""
        allowed_origins = self.config["allowed_origins"]

        try:
            body = await request.json()
        except Exception:
            return add_cors_headers(
                aiohttp_web.json_response(
                    {"error": "Invalid JSON body"}, status=400
                ),
                allowed_origins,
            )

        session_id = body.get("session_id", "").strip()
        breakout_id = body.get("breakout_id", "").strip()

        if not session_id or not breakout_id:
            return add_cors_headers(
                aiohttp_web.json_response(
                    {"error": "session_id and breakout_id are required"}, status=400
                ),
                allowed_origins,
            )

        try:
            result = await do_move_to_breakout(
                db=self.database,
                session_id=session_id,
                breakout_id=breakout_id,
                client_api=self.client.api,
                lk_api_key=self.config["livekit_api_key"],
                lk_api_secret=self.config["livekit_api_secret"],
                livekit_url=self.config["livekit_url"],
                ec_base_url=self.config["ec_base_url"],
                ttl_seconds=self.config["guest_token_ttl"],
            )
        except ValueError as e:
            return add_cors_headers(
                aiohttp_web.json_response({"error": str(e)}, status=404),
                allowed_origins,
            )

        return add_cors_headers(
            aiohttp_web.json_response({
                "jwt": result["jwt"],
                "livekit_room": result["livekit_room"],
                "ec_url": result["ec_url"],
            }),
            allowed_origins,
        )

    # ══════════════════════════════════════════════════════════
    #  Lifecycle
    # ══════════════════════════════════════════════════════════

    async def stop(self) -> None:
        self.log.info("Wally Conference plugin stopping")
        if self._cleanup_task is not None and not self._cleanup_task.done():
            self._cleanup_task.cancel()
            try:
                await self._cleanup_task
            except asyncio.CancelledError:
                pass
            self._cleanup_task = None
        await super().stop()
