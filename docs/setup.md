# Setup Guide

This guide walks you through deploying Wally Conference on your own Matrix server. Each phase is independently deployable — you can stop at any phase and have a working (partial) system.

## Prerequisites

- A Matrix homeserver (Synapse, Dendrite, or Continuwuity)
- A LiveKit server (self-hosted or cloud)
- lk-jwt-service configured and running
- Element Call integrated into your Matrix client

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
  -u wally-conference -p <password> --no-admin

# Continuwuity
# Use the admin API or register via a client
```

### 2.2 Install the service

#### Arch Linux (PKGBUILD)

```bash
cd wally-conference-git
makepkg -si
```

This installs the binary, systemd service, and config template.

#### From source

```bash
git clone https://github.com/LaPingvino/wally-conference
cd wally-conference
go build -o wally-conference .
sudo install -Dm755 wally-conference /usr/bin/wally-conference
```

### 2.3 Configure

```bash
sudo mkdir -p /etc/wally-conference
sudo cp config.example.yaml /etc/wally-conference/config.yaml
sudo editor /etc/wally-conference/config.yaml
```

Set at minimum:

```yaml
# Matrix credentials
homeserver: "https://matrix.yourserver.com"
user_id: "@wally-conference:yourserver.com"
password: "your-bot-password"   # or use access_token

# LiveKit server credentials
livekit_url: "wss://livekit.yourserver.com"
livekit_api_key: "your-livekit-api-key"
livekit_api_secret: "your-livekit-api-secret"
livekit_service_url: "https://jwt.yourserver.com"

# Security
allowed_origins: "https://cinny.yourserver.com"  # your client URL

# Element Call
ec_base_url: "https://cinny.yourserver.com/public/element-call/index.html"
```

### 2.4 Start the service

```bash
sudo systemctl enable --now wally-conference
```

Check logs:

```bash
journalctl -u wally-conference -f
```

### 2.5 Invite the bot to VC rooms

In any room where you want guest access:

```
/invite @wally-conference:yourserver
```

Or configure auto-join in the config:
```yaml
auto_join_invites: true
```

### 2.6 Test guest access

```bash
curl -X POST http://localhost:8080/join \
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
    - "https://yourserver.com/wally-conference/webhook"
  api_key: "your-livekit-api-key"
```

### 3.2 Verify

Restart LiveKit. When a guest disconnects, their `call.member` state event should be cleared within seconds (check the Matrix room state).

**Without webhooks:** The bot's background cleanup task clears expired sessions every 5 minutes, and the `call.member` event's `expires` field (default 2h) causes EC to auto-hide stale participants. Webhooks just make it faster.

## Phase 4: Bot commands

Once the bot is in a room, moderators can use Matrix commands:

```
!wc status              — Show active guests and breakout rooms
!wc invite !room:server — Bot joins another room
!wc kick <session-id>   — Remove a guest
!wc breakout create     — Create a breakout room (Phase 5)
```

## Phase 5: Breakout rooms

Breakout rooms work through the same bot. A moderator in a call room:

```
!wc breakout create "Topic A"
!wc breakout create "Topic B"
!wc breakout move <session-id> <breakout-id>
!wc breakout end <breakout-id>
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

### Caddy

```caddy
yourserver.com {
    handle_path /wally-conference/* {
        reverse_proxy localhost:8080
    }
}
```

### nginx

```nginx
server {
    server_name yourserver.com;
    location /wally-conference/ {
        proxy_pass http://localhost:8080/;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

## Troubleshooting

### Bot doesn't appear in room
- Check the bot is invited and has joined
- Check bot has power level >= 0 (default for `call.member` state events)

### Guest JWT rejected by LiveKit
- Verify `livekit_api_key` and `livekit_api_secret` match your LiveKit server
- Check the LiveKit room alias hash matches (check service logs)

### Guest invisible to other participants
- Verify the EC patch is applied (check for `livekitToken` in the EC source)
- Check the `call.member` state event was sent (inspect room state)
- Verify the identity hash in the state event matches the JWT identity

### Webhook not firing
- Check LiveKit config has the correct webhook URL
- Verify the URL is reachable from LiveKit (not blocked by firewall)
- Check service logs for webhook errors
