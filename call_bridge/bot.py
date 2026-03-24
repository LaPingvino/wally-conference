"""Main Maubot plugin class for Wally Conference (Call Bridge)."""

from __future__ import annotations

import uuid
from urllib.parse import quote, urlencode

from maubot import Plugin
from aiohttp import web as aiohttp_web
from mautrix.util.config import BaseProxyConfig

from .config import Config
from .db import upgrade_table, create_session, count_active_sessions
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


class CallBridgeBot(Plugin):
    """Maubot plugin that bridges guest callers into MatrixRTC/LiveKit calls."""

    rate_limiter: RateLimiter

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
        self._setup_routes()
        self.log.info("Wally Conference plugin started")

    def _setup_routes(self) -> None:
        """Register HTTP routes on the maubot webapp sub-application."""
        self.webapp.router.add_route("OPTIONS", "/join", self._options_handler)
        self.webapp.router.add_post("/join", self.handle_guest_join)
        self.webapp.router.add_post("/webhook", self.handle_livekit_webhook)
        self.webapp.router.add_get("/health", self.handle_health)
        self.webapp.router.add_post("/breakout/create", self.handle_breakout_create)
        self.webapp.router.add_post("/breakout/move", self.handle_breakout_move)

    # ── CORS preflight ───────────────────────────────────────

    async def _options_handler(
        self, request: aiohttp_web.Request
    ) -> aiohttp_web.Response:
        return cors_preflight(self.config["allowed_origins"])

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
        import time

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

    # ── Webhook (stub) ───────────────────────────────────────

    async def handle_livekit_webhook(
        self, request: aiohttp_web.Request
    ) -> aiohttp_web.Response:
        """Handle LiveKit webhook events (participant_left, etc.)."""
        return aiohttp_web.json_response(
            {"error": "not implemented"}, status=501
        )

    # ── Health ───────────────────────────────────────────────

    async def handle_health(
        self, request: aiohttp_web.Request
    ) -> aiohttp_web.Response:
        """Health check endpoint."""
        return aiohttp_web.json_response({"status": "ok"})

    # ── Breakout stubs ───────────────────────────────────────

    async def handle_breakout_create(
        self, request: aiohttp_web.Request
    ) -> aiohttp_web.Response:
        """Create a breakout room."""
        return aiohttp_web.json_response(
            {"error": "not implemented"}, status=501
        )

    async def handle_breakout_move(
        self, request: aiohttp_web.Request
    ) -> aiohttp_web.Response:
        """Move participants to a breakout room."""
        return aiohttp_web.json_response(
            {"error": "not implemented"}, status=501
        )

    async def stop(self) -> None:
        self.log.info("Wally Conference plugin stopping")
        await super().stop()
