# Setup Guide

This guide walks you through deploying Call Bridge on your own Matrix server. Each phase is independently deployable — you can stop at any phase and have a working (partial) system.

## Prerequisites

- A Matrix homeserver (Synapse, Dendrite, or Continuwuity)
- A LiveKit server (self-hosted or cloud)
- lk-jwt-service configured and running
- Element Call integrated into your Matrix client
- Maubot instance (for Phase 2+)

## Phase 1: EC Patch (zero risk)

Apply the Element Call patch that adds `livekitToken` URL parameter support. This is completely inert until the bot exists — it adds no behavioral change to normal calls.

### For Wally/Cinny fork users

The patch is included in `02-element-call.patch` starting from version `wally/v4.11.1-3`. If you're building from the Codeberg repo, it's already applied.

### For other Element Call deployments

Add this to your EC build, in `src/livekit/openIDSFU.ts` at the beginning of the JWT acquisition function:

```typescript
const urlParams = new URLSearchParams(window.location.search);
const preIssuedJwt = urlParams.get("livekitToken");
const preIssuedRoom = urlParams.get("livekitRoom");

if (preIssuedJwt && preIssuedRoom) {
  const sfuUrl = urlParams.get("livekitUrl") ?? fallbackSfuUrl;
  return { url: sfuUrl, jwt: preIssuedJwt };
}
```

**Verification:** After deploying, open a call normally. Everything should work exactly as before. The patch only activates when `livekitToken` is present in the URL.

## Phase 2: Bot deployment

### 2.1 Create a Matrix bot account

Register a new account on your homeserver for the bot:

```bash
# Synapse
register_new_matrix_user -c /etc/synapse/homeserver.yaml \
  -u call-bridge -p <password> --no-admin

# Continuwuity
# Use the admin API or register via a client
```

### 2.2 Install Maubot

If you don't already have Maubot:

```bash
pip install maubot
# Or use Docker: https://docs.mau.fi/maubot/setup/docker.html
```

Configure Maubot to connect to your homeserver. See [Maubot setup docs](https://docs.mau.fi/maubot/setup/).

### 2.3 Install the plugin

```bash
# Download the latest release
wget https://github.com/LaPingvino/<botname>/releases/latest/download/call-bridge.mbp

# Upload to Maubot
mbc upload call-bridge.mbp
```

Or via the Maubot web admin interface: upload the `.mbp` file.

### 2.4 Create a Maubot client

In Maubot admin, create a client for the bot account:

- **Homeserver:** your Matrix server URL
- **Access token:** log in as the bot and get a token, or use `mbc auth`

### 2.5 Create a plugin instance

In Maubot admin, create an instance:

- **Plugin:** `eu.kiefte.call-bridge`
- **Client:** the bot client you just created
- **Primary user:** `@call-bridge:yourserver`

### 2.6 Configure the instance

Edit the instance config (via Maubot admin web UI):

```yaml
# LiveKit server credentials
livekit_url: "wss://livekit.yourserver.com"
livekit_api_key: "your-livekit-api-key"
livekit_api_secret: "your-livekit-api-secret"
livekit_service_url: "https://jwt.yourserver.com"

# Guest settings
guest_token_ttl: 7200          # 2 hours
max_guests_per_room: 20

# Security
allowed_origins: "https://cinny.yourserver.com"  # your client URL
rate_limit_per_minute: 5
```

### 2.7 Invite the bot to VC rooms

In any room where you want guest access:

```
/invite @call-bridge:yourserver
```

Or configure auto-join in the bot config:
```yaml
auto_join_invites: true
```

### 2.8 Test guest access

```bash
curl -X POST https://maubot.yourserver.com/_matrix/maubot/plugin/call-bridge/join \
  -H "Content-Type: application/json" \
  -d '{"room_id": "!yourroom:yourserver", "display_name": "Test Guest"}'
```

You should get back:
```json
{
  "jwt": "eyJ...",
  "livekit_url": "wss://livekit.yourserver.com",
  "livekit_room": "base64hash...",
  "ec_url": "https://cinny.yourserver.com/public/element-call/index.html?livekitToken=eyJ...&..."
}
```

Open the `ec_url` in a browser. You should appear in the call.

## Phase 3: LiveKit webhooks

Configure LiveKit to send webhooks to the bot for immediate cleanup on disconnect.

### 3.1 Configure LiveKit

In your LiveKit config (`livekit.yaml` or environment variables):

```yaml
webhook:
  urls:
    - "https://maubot.yourserver.com/_matrix/maubot/plugin/call-bridge/webhook"
  api_key: "your-livekit-api-key"
```

Or via environment:
```bash
LIVEKIT_WEBHOOK_URLS="https://maubot.yourserver.com/_matrix/maubot/plugin/call-bridge/webhook"
```

### 3.2 Verify

Restart LiveKit. When a guest disconnects, their `call.member` state event should be cleared within seconds (check the Matrix room state).

**Without webhooks:** The bot's background cleanup task clears expired sessions every 5 minutes, and the `call.member` event's `expires` field (default 2h) causes EC to auto-hide stale participants. Webhooks just make it faster.

## Phase 4: Bot commands

Once the bot is in a room, moderators can use Matrix commands:

```
!call status              — Show active guests and breakout rooms
!call invite !room:server — Bot joins another room
!call kick <identity>     — Remove a guest
!call breakout create     — Create a breakout room (Phase 5)
```

## Phase 5: Breakout rooms

Breakout rooms work through the same bot. A moderator in a call room:

```
!call breakout create "Topic A"
!call breakout create "Topic B"
!call breakout move @user:server breakout-abc
!call breakout end breakout-abc
```

The bot issues new LiveKit JWTs for the breakout room alias and notifies participants.

## Phase 6: Guest join UI (Wally)

A dedicated guest join page in Wally at `/call/guest/:roomId`:

- Shows room name and current participant count
- Name input field
- Optional shared-key password field
- "Join Call" button
- Loads patched EC iframe on join

This is a future Wally feature. Until then, the `ec_url` from the bot's `/join` response works directly.

## Reverse proxy configuration

If your Maubot is behind a reverse proxy (Caddy, nginx, etc.), ensure the webhook and join endpoints are accessible:

### Caddy

```caddy
maubot.yourserver.com {
    reverse_proxy localhost:29316
}
```

### nginx

```nginx
server {
    server_name maubot.yourserver.com;
    location / {
        proxy_pass http://localhost:29316;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

## Troubleshooting

### Bot doesn't appear in room
- Check the bot is invited and has joined
- Check bot has power level >= 0 (default for `call.member` state events)

### Guest JWT rejected by LiveKit
- Verify `livekit_api_key` and `livekit_api_secret` match your LiveKit server
- Check the LiveKit room alias hash matches (enable debug logging)

### Guest invisible to other participants
- Verify the EC patch is applied (check for `livekitToken` in the EC source)
- Check the `call.member` state event was sent (inspect room state)
- Verify the identity hash in the state event matches the JWT identity

### Webhook not firing
- Check LiveKit config has the correct webhook URL
- Verify the URL is reachable from LiveKit (not blocked by firewall)
- Check Maubot logs for webhook errors
