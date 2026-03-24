from mautrix.util.config import BaseProxyConfig, ConfigUpdateHelper


class Config(BaseProxyConfig):
    def do_update(self, helper: ConfigUpdateHelper) -> None:
        # LiveKit
        helper.copy("livekit_url")
        helper.copy("livekit_api_key")
        helper.copy("livekit_api_secret")
        helper.copy("livekit_service_url")

        # Guest settings
        helper.copy("guest_token_ttl")
        helper.copy("max_guests_per_room")
        helper.copy("require_active_call")

        # Security
        helper.copy("allowed_origins")
        helper.copy("rate_limit_per_minute")

        # Bot behavior
        helper.copy("auto_join_invites")
        helper.copy("admin_rooms")

        # Element Call URL
        helper.copy("ec_base_url")

        # Cleanup
        helper.copy("cleanup_interval")

        # Breakout rooms
        helper.copy("max_breakouts_per_room")
