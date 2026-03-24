# Development Guide

## Prerequisites

- Python 3.10+
- A Maubot development instance ([setup guide](https://docs.mau.fi/maubot/dev/getting-started.html))
- LiveKit server (local or remote) with API key/secret
- A Matrix homeserver with a bot account

## Project structure

```
wally-conference/
├── maubot.yaml              # Plugin metadata
├── base-config.yaml          # Default configuration
├── requirements.txt          # Python dependencies
├── call_bridge/
│   ├── __init__.py           # Exports CallBridgeBot
│   ├── bot.py                # Main Plugin class, routes, command handlers
│   ├── config.py             # Config schema (BaseProxyConfig subclass)
│   ├── db.py                 # Database helpers (guest_session, breakout_room)
│   ├── identity.py           # LiveKit identity hash functions
│   ├── jwt_service.py        # LiveKit JWT issuance
│   ├── membership.py         # call.member state event management
│   ├── webhook.py            # LiveKit webhook receiver
│   ├── breakout.py           # Breakout room logic
│   ├── security.py           # Rate limiting, input validation
│   └── cleanup.py            # Background session expiry task
├── tests/
│   ├── test_identity.py      # Hash function test vectors
│   ├── test_jwt.py           # JWT generation tests
│   ├── test_membership.py    # State event format tests
│   └── test_security.py      # Rate limiter, validation tests
└── docs/
    └── ...
```

## Building the plugin

```bash
# Install maubot CLI
pip install maubot

# Build the .mbp package
mbc build -o wally-conference.mbp

# Upload to your Maubot instance
mbc upload wally-conference.mbp -s https://maubot.yourserver.com
```

## Running tests

```bash
pip install -r requirements.txt
pip install pytest pytest-asyncio

pytest tests/ -v
```

## Test vectors for identity hashes

These must match lk-jwt-service's output exactly:

```python
# From lk-jwt-service test code
def test_room_alias():
    assert livekit_room_alias("!testRoom:example.com") == \
        base64_unpadded(sha256("!testRoom:example.com|m.call#ROOM"))

def test_participant_identity():
    assert livekit_identity("@user:example.com", "DEVICE1", "session-uuid") == \
        base64_unpadded(sha256("@user:example.com|DEVICE1|session-uuid"))
```

## Local development with maubot

```bash
# Clone and set up maubot dev environment
git clone https://github.com/maubot/maubot
cd maubot
pip install -e .

# Run maubot locally
python -m maubot

# In another terminal, build and upload plugin
cd /path/to/wally-conference
mbc build && mbc upload wally-conference.mbp -s http://localhost:29316
```

## Debugging

Enable debug logging in the Maubot instance config:

```yaml
logging:
  loggers:
    maubot.instance.wally-conference:
      level: DEBUG
```

The bot logs all guest joins, JWT issuances, state event sends, and webhook events at DEBUG level.

## Contributing

1. Fork the repo
2. Create a feature branch
3. Write tests for new functionality
4. Submit a pull request

Key guidelines:
- All hash computations must match lk-jwt-service exactly (test vectors required)
- HTTP endpoints must validate all input
- State events must follow MSC4143 format
- Rate limiting must be applied to all guest-facing endpoints
