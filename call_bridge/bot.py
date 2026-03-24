"""Main Maubot plugin class for Wally Conference (Call Bridge)."""

from __future__ import annotations

from maubot import Plugin
from aiohttp import web as aiohttp_web
from mautrix.util.config import BaseProxyConfig

from .config import Config
from .db import upgrade_table


class CallBridgeBot(Plugin):
    """Maubot plugin that bridges guest callers into MatrixRTC/LiveKit calls."""

    @classmethod
    def get_config_class(cls) -> type[BaseProxyConfig]:
        return Config

    @classmethod
    def get_db_upgrade_table(cls) -> None:
        return upgrade_table

    async def start(self) -> None:
        await super().start()
        self.config.load_and_update()
        self._setup_routes()
        self.log.info("Wally Conference plugin started")

    def _setup_routes(self) -> None:
        """Register HTTP routes on the maubot webapp sub-application."""
        self.webapp.router.add_post("/join", self.handle_guest_join)
        self.webapp.router.add_post("/webhook", self.handle_livekit_webhook)
        self.webapp.router.add_get("/health", self.handle_health)
        self.webapp.router.add_post("/breakout/create", self.handle_breakout_create)
        self.webapp.router.add_post("/breakout/move", self.handle_breakout_move)

    async def handle_guest_join(
        self, request: aiohttp_web.Request
    ) -> aiohttp_web.Response:
        """Handle a guest join request. Returns a LiveKit JWT."""
        return aiohttp_web.json_response(
            {"error": "not implemented"}, status=501
        )

    async def handle_livekit_webhook(
        self, request: aiohttp_web.Request
    ) -> aiohttp_web.Response:
        """Handle LiveKit webhook events (participant_left, etc.)."""
        return aiohttp_web.json_response(
            {"error": "not implemented"}, status=501
        )

    async def handle_health(
        self, request: aiohttp_web.Request
    ) -> aiohttp_web.Response:
        """Health check endpoint."""
        return aiohttp_web.json_response({"status": "ok"})

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
