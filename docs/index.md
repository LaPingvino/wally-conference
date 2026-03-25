# Call Bridge — Guest Access & Breakout Rooms for Element Call

Call Bridge (Wally Conference) is a standalone Go service that adds guest access and breakout room support to Matrix voice/video rooms powered by [Element Call](https://github.com/element-hq/element-call) and [LiveKit](https://livekit.io/). It uses [mautrix-go](https://github.com/mautrix/go) for Matrix integration, the same library used by all mautrix bridges.

## What it does

- **Guest access**: Anyone with a shareable link can join a voice/video call without a Matrix account
- **Breakout rooms**: Split call participants into smaller groups for discussion, then bring them back
- **Federation-compatible**: Other Matrix clients (Element Web, etc.) see guest participants in their call UI
- **Graceful degradation**: Each component can be deployed independently; partial setups are still useful

## How it works

```
Guest clicks link → Wally guest page (enter name)
  → POST /join to Call Bridge bot
  → Bot issues LiveKit JWT + sends call.member state event
  → Guest loads Element Call with pre-issued JWT
  → All participants (Matrix + guest) see each other
  → On disconnect: webhook triggers cleanup
```

The bot acts as a bridge between unauthenticated guests and the Matrix/LiveKit call infrastructure. It uses a single Matrix bot account to proxy guest presence into Matrix rooms.

## Documentation

- [Architecture](architecture.md) — how the components fit together
- [Setup Guide](setup.md) — step-by-step deployment on your own server
- [Configuration](configuration.md) — all config options explained
- [API Reference](api.md) — HTTP endpoints and bot commands
- [Degradation Matrix](degradation.md) — what works when components are missing
- [EC Patch](ec-patch.md) — the Element Call patch for pre-issued JWT support
- [Security](security.md) — rate limiting, abuse prevention, trust model
- [Development](development.md) — building, testing, contributing

## Requirements

- Matrix homeserver (Synapse, Dendrite, or Continuwuity)
- [LiveKit](https://livekit.io/) server (self-hosted or cloud)
- [lk-jwt-service](https://github.com/element-hq/lk-jwt-service) (for Matrix-authenticated users)
- A Matrix client with Element Call support (e.g., [Wally/Cinny](https://codeberg.org/lapingvino/cinny))

## Quick start

See the [Setup Guide](setup.md) for full instructions. The short version:

```bash
# 1. Install from the wally-conference-git PKGBUILD (Arch Linux)
#    or build from source: go build -o wally-conference .

# 2. Copy config.example.yaml to /etc/wally-conference/config.yaml and edit

# 3. Set homeserver, bot credentials, and LiveKit API keys

# 4. Start the service: systemctl enable --now wally-conference

# 5. Invite the bot to your VC rooms

# 6. (Optional) Apply the EC patch to your Cinny/Wally build
```

## License

AGPL-3.0-or-later
