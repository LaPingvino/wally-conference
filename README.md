# Wally Conference

Guest access and breakout rooms for Matrix voice/video calls powered by [Element Call](https://github.com/element-hq/element-call) and [LiveKit](https://livekit.io/).

A [Maubot](https://docs.mau.fi/maubot/) plugin that lets anyone join a call with a shareable link — no Matrix account required.

## Features

- **Guest access** — share a link, guests enter their name and join the call
- **Breakout rooms** — split participants into groups for discussion
- **Federation-compatible** — all Matrix clients see guest participants
- **Graceful degradation** — each component deploys independently
- **Secure** — rate limiting, capacity limits, server-side JWT signing

## How it works

The bot bridges guests into Matrix calls by:
1. Issuing LiveKit JWTs for guests (server-side, no credentials exposed)
2. Sending `call.member` state events so all clients see the guest
3. Cleaning up when guests disconnect (via LiveKit webhooks)

See the [full documentation](docs/index.md) for architecture details, setup guide, and API reference.

## Quick start

```bash
# Install in Maubot
mbc upload wally-conference.mbp

# Configure LiveKit credentials in the Maubot admin UI

# Invite the bot to your VC rooms
/invite @wally-conference:yourserver.com

# Generate a guest link
!wc link
```

## Documentation

- [Architecture](docs/architecture.md)
- [Setup Guide](docs/setup.md)
- [Configuration](docs/configuration.md)
- [API Reference](docs/api.md)
- [Degradation Matrix](docs/degradation.md)
- [EC Patch](docs/ec-patch.md)
- [Security](docs/security.md)
- [Development](docs/development.md)

## Status

**Pre-alpha** — implementation plan complete, code in progress.

## License

AGPL-3.0-or-later
