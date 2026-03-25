# Development Guide

## Prerequisites

- Go 1.22+
- A LiveKit server (local or remote) with API key/secret
- A Matrix homeserver with a bot account

## Project structure

```
wally-conference/
├── main.go              # Entry point, config loading, service startup
├── config.go            # Config struct + YAML loading
├── identity.go          # LiveKit room alias + participant identity hashes
├── identity_test.go     # Test vectors matching lk-jwt-service
├── jwt.go               # LiveKit JWT issuance using livekit protocol/auth
├── membership.go        # Send/clear call.member state events via mautrix-go
├── handler_join.go      # POST /join handler
├── handler_webhook.go   # POST /webhook handler (LiveKit)
├── handler_health.go    # GET /health handler
├── handler_breakout.go  # POST /breakout/* handlers
├── security.go          # Rate limiter, input validation, CORS
├── db.go                # SQLite schema + queries
├── cleanup.go           # Background goroutine for expired sessions
├── commands.go          # Matrix room commands (!wc status, etc.)
├── go.mod
├── go.sum
├── config.example.yaml  # Example configuration
├── docs/                # Documentation
└── README.md            # Project overview
```

## Building

```bash
go build -o wally-conference .
```

## Running

```bash
./wally-conference /etc/wally-conference/config.yaml
```

## Running tests

```bash
go test ./... -v
```

## Test vectors for identity hashes

These must match lk-jwt-service's output exactly:

```go
// From lk-jwt-service test code
func TestRoomAlias(t *testing.T) {
    result := LiveKitRoomAlias("!testRoom:example.com")
    expected := hashUnpaddedBase64("!testRoom:example.com|m.call#ROOM")
    assert(result == expected)
}

func TestParticipantIdentity(t *testing.T) {
    result := LiveKitIdentity("@user:example.com", "DEVICE1", "session-uuid")
    expected := hashUnpaddedBase64("@user:example.com|DEVICE1|session-uuid")
    assert(result == expected)
}
```

## Key dependencies

- `maunium.net/go/mautrix` — Matrix client, event types, room IDs
- `github.com/livekit/protocol` — LiveKit JWT issuance (AccessToken, auth, webhook)
- `modernc.org/sqlite` — pure Go SQLite driver (no CGO required)
- `gopkg.in/yaml.v3` — YAML config parsing
- `github.com/google/uuid` — UUID generation for session IDs

## Systemd service

The service runs as a systemd unit (`wally-conference.service`). Install via the `wally-conference-git` PKGBUILD.

```bash
sudo systemctl enable --now wally-conference
```

Configuration: `/etc/wally-conference/config.yaml`
Database: `/var/lib/wally-conference/wally-conference.db`
Logs: `journalctl -u wally-conference`

## Debugging

Run with verbose logging:

```bash
./wally-conference /etc/wally-conference/config.yaml 2>&1 | tee /tmp/wally.log
```

The bot logs all guest joins, JWT issuances, state event sends, and webhook events.

## Contributing

1. Fork the repo
2. Create a feature branch
3. Write tests for new functionality
4. Run `go test ./...` to verify
5. Submit a pull request

Key guidelines:
- All hash computations must match lk-jwt-service exactly (test vectors required)
- HTTP endpoints must validate all input
- State events must follow MSC4143 format
- Rate limiting must be applied to all guest-facing endpoints
