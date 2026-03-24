# Configuration

The plugin is configured via the Maubot admin interface. Edit the instance configuration to set these values.

## Full configuration reference

```yaml
# ── LiveKit ──────────────────────────────────────────────
# LiveKit server WebSocket URL (used in JWT and returned to guests)
livekit_url: "wss://livekit.yourserver.com"

# LiveKit API credentials (from your LiveKit server config)
# KEEP THESE SECRET — they allow signing JWTs for any room
livekit_api_key: ""
livekit_api_secret: ""

# URL of lk-jwt-service (shown in call.member foci_preferred)
# This is what regular Matrix users use; guests bypass it via bot-issued JWT
livekit_service_url: "https://jwt.yourserver.com"

# ── Guest settings ───────────────────────────────────────
# How long guest JWTs are valid (seconds)
# Also used for call.member expires field (converted to ms)
guest_token_ttl: 7200  # 2 hours

# Maximum concurrent guests per room
max_guests_per_room: 20

# Require an active call (existing call.member events) before allowing guests
require_active_call: true

# ── Security ─────────────────────────────────────────────
# CORS allowed origins for the /join endpoint
# Set to your client URL in production; "*" for development only
allowed_origins: "https://cinny.yourserver.com"

# Rate limit: max guest join requests per IP per minute
rate_limit_per_minute: 5

# ── Bot behavior ─────────────────────────────────────────
# Automatically accept room invites
auto_join_invites: true

# Rooms where admin commands (!wc config, !wc invite) are accepted
# Empty = admin commands work in any room (less secure)
admin_rooms: []

# ── Element Call URL ─────────────────────────────────────
# Base URL for the patched Element Call instance
# Used to construct the ec_url in /join responses
ec_base_url: "https://cinny.yourserver.com/public/element-call/index.html"

# ── Cleanup ──────────────────────────────────────────────
# How often to run the background cleanup task (seconds)
cleanup_interval: 300  # 5 minutes

# ── Breakout rooms ───────────────────────────────────────
# Maximum concurrent breakout rooms per parent room
max_breakouts_per_room: 10
```

## Environment variables

The plugin reads configuration from Maubot's instance config, not environment variables. However, you may want to set these in your Maubot deployment:

```bash
# If running Maubot in Docker, pass LiveKit credentials as env vars
# and reference them in the Maubot config via templating
LIVEKIT_API_KEY=your-key
LIVEKIT_API_SECRET=your-secret
```

## Finding your LiveKit credentials

### Self-hosted LiveKit

In your LiveKit config file (usually `livekit.yaml`):

```yaml
keys:
  your-api-key: your-api-secret
```

### lk-jwt-service

If you're already running lk-jwt-service, the same API key/secret are in its config:

```bash
# Check lk-jwt-service environment
grep -E 'LIVEKIT_KEY|LIVEKIT_SECRET' /path/to/lk-jwt-service.env
```

## Security notes

- **Never expose `livekit_api_key`/`livekit_api_secret` to the browser.** The bot signs JWTs server-side.
- **Set `allowed_origins`** to your specific client domain in production. `"*"` allows any website to create guest sessions.
- **Set `admin_rooms`** to restrict admin commands to specific rooms (e.g., a private admin room).
- **Rate limiting** is per-IP. Behind a reverse proxy, ensure `X-Real-IP` or `X-Forwarded-For` is set correctly.
