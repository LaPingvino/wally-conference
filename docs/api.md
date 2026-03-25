# API Reference

## HTTP Endpoints

All endpoints are served at:
```
https://<host>:<port>/<path>
```

The listen address is configured in `config.yaml` (default `:8080`). In production, use a reverse proxy (Caddy, nginx) to add TLS.

### POST /join — Guest join

Request a LiveKit JWT to join a call as a guest.

**Request:**
```json
{
  "room_id": "!abc:yourserver.com",
  "display_name": "Alice",
  "password": "optional-shared-key",
  "breakout_id": null
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `room_id` | string | Yes | Matrix room ID of the call room |
| `display_name` | string | Yes | Guest's display name (max 50 chars) |
| `password` | string | No | Shared key for SHARED_KEY E2EE rooms |
| `breakout_id` | string | No | Breakout room ID (if joining a breakout) |

**Response (200):**
```json
{
  "jwt": "eyJhbGciOiJIUzI1NiIs...",
  "livekit_url": "wss://livekit.yourserver.com",
  "livekit_room": "base64hash...",
  "session_id": "a3f9c1d2-...",
  "ec_url": "https://cinny.yourserver.com/public/element-call/index.html?livekitToken=eyJ...&livekitRoom=base64hash&livekitUrl=wss://livekit.yourserver.com&displayName=Alice&skipLobby=true",
  "expires_at": 1711241767
}
```

| Field | Type | Description |
|-------|------|-------------|
| `jwt` | string | LiveKit JWT for direct connection |
| `livekit_url` | string | LiveKit server WebSocket URL |
| `livekit_room` | string | LiveKit room alias (SHA256 hash) |
| `session_id` | string | Unique session ID for this guest |
| `ec_url` | string | Ready-to-open URL that loads patched Element Call |
| `expires_at` | integer | Unix timestamp when the JWT expires |

**Error responses:**

| Status | Body | Cause |
|--------|------|-------|
| 400 | `{"error": "Invalid room_id"}` | Malformed Matrix room ID |
| 400 | `{"error": "display_name required"}` | Missing or empty display name |
| 403 | `{"error": "Bot not in room"}` | Bot hasn't joined the room |
| 403 | `{"error": "No active call"}` | No call.member events in room |
| 429 | `{"error": "Rate limited"}` | Too many requests from this IP |
| 429 | `{"error": "Guest capacity reached"}` | Room has max_guests_per_room guests |

### POST /webhook — LiveKit webhook

Receives LiveKit webhook events. Configured in LiveKit server config.

**Request:** LiveKit webhook payload with `Authorization` header containing signed JWT.

**Handled events:**
- `participant_left` — clears guest's call.member state event and DB session
- `room_finished` — clears all guest sessions for that room

**Response:** `200 OK` (always, to prevent LiveKit retries on handled events)

### GET /health — Health check

Returns bot status.

**Response (200):**
```json
{
  "status": "ok",
  "active_guests": 3,
  "active_breakouts": 1,
  "matrix_connected": true,
  "livekit_configured": true
}
```

### POST /breakout/create — Create breakout room

**Request:**
```json
{
  "room_id": "!abc:yourserver.com",
  "topic": "Discussion Group A",
  "user_id": "@admin:yourserver.com"
}
```

**Response (200):**
```json
{
  "breakout_id": "abc12345",
  "livekit_room": "breakouthash...",
  "topic": "Discussion Group A"
}
```

### POST /breakout/move — Move participant to breakout

**Request:**
```json
{
  "room_id": "!abc:yourserver.com",
  "breakout_id": "abc12345",
  "session_id": "guest-session-uuid"
}
```

**Response (200):**
```json
{
  "jwt": "eyJ...",
  "livekit_room": "breakouthash...",
  "ec_url": "https://..."
}
```

## Bot Commands

Send these as messages in a Matrix room where the bot is present.

| Command | Description | Required power level |
|---------|-------------|---------------------|
| `!wc status` | Show active guests, breakout rooms, and bot health | Any |
| `!wc invite !room:server` | Bot joins a room | Moderator |
| `!wc leave !room:server` | Bot leaves a room | Moderator |
| `!wc kick <session-id>` | Remove a guest from the call | Moderator |
| `!wc breakout create <topic>` | Create a breakout room | Moderator |
| `!wc breakout list` | List active breakout rooms | Any |
| `!wc breakout end <id>` | End a breakout room, move participants back | Moderator |
| `!wc breakout move <user> <id>` | Move a participant to a breakout | Moderator |
| `!wc link` | Generate a shareable guest join link for the current room | Any |
| `!wc config` | Show current plugin configuration | Admin |

The command prefix `!wc` stands for "Wally Conference".
