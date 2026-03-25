# Architecture

## Component overview

```
┌─────────────────────────────────────────────────────────┐
│                    Matrix Homeserver                      │
│  (Synapse / Dendrite / Continuwuity)                     │
│                                                          │
│  Rooms contain:                                          │
│  - org.matrix.msc3401.call.member state events           │
│  - Regular Matrix messages                               │
│  - Bot account (@call-bridge:yourserver)                 │
└──────────────┬───────────────────────────┬───────────────┘
               │                           │
               │ Matrix C-S API            │ Matrix C-S API
               │                           │
┌──────────────▼──────────┐  ┌─────────────▼──────────────┐
│   Wally Conference Bot   │  │    Wally / Cinny Client     │
│    (standalone Go svc)   │  │    (patched Element Call)   │
│                          │  │                             │
│  - Guest join endpoint   │  │  - Normal Matrix users      │
│  - JWT issuance          │  │  - Patched EC accepts       │
│  - call.member proxy     │  │    livekitToken URL param   │
│  - Webhook receiver      │  │  - Guest join UI page       │
│  - Breakout management   │  │                             │
└──────────┬───────────────┘  └─────────────┬──────────────┘
           │                                │
           │ LiveKit JWT                    │ LiveKit JWT
           │ (issued by bot)               │ (via lk-jwt-service)
           │                                │
┌──────────▼────────────────────────────────▼──────────────┐
│                     LiveKit SFU                           │
│                                                          │
│  - Rooms auto-created on first join                      │
│  - Room name = SHA256(matrixRoomId | "m.call#ROOM")      │
│  - Participant identity = SHA256(userId|deviceId|session) │
│  - Sends webhooks on participant join/leave              │
└──────────────────────────────────────────────────────────┘
```

## Participant identity model

Element Call's participant model is **Matrix-state-first**: every visible call participant must have an `org.matrix.msc3401.call.member` state event in the Matrix room. EC maps these to LiveKit participants via identity hashes.

### For regular Matrix users

```
Matrix user → OpenID token → lk-jwt-service → LiveKit JWT
                                                 ↓
Identity = SHA256(matrixUserId | deviceId | sessionUUID)
```

### For guests (via Call Bridge bot)

```
Guest → POST /join → Bot generates:
  1. Synthetic device_id: GUEST_<random>
  2. Session UUID
  3. LiveKit identity = SHA256(botUserId | GUEST_xxx | sessionUUID)
  4. LiveKit JWT with that identity
  5. call.member state event with matching device_id + session
```

The bot sends the state event as itself (`@call-bridge:yourserver`) but with a unique `device_id` per guest. EC computes the identity hash from the state event and matches it against the LiveKit participant — so the guest appears as a real participant.

### Display name mapping

EC reads the display name from the Matrix room member event. Since the bot account sends all guest state events, the bot's display name is shown by default. To differentiate:

- The bot sets `display_name` in the LiveKit JWT (shown in the LiveKit participant metadata)
- The patched EC can read display name from LiveKit metadata when the Matrix member name is the bot's name

## call.member state event

State key format (MSC4143): `_@call-bridge:yourserver_GUEST_abc123`

Content:
```json
{
  "application": "m.call",
  "call_id": "",
  "scope": "m.room",
  "device_id": "GUEST_abc123",
  "expires": 7200000,
  "created_ts": 1711234567890,
  "focus_active": {
    "type": "livekit",
    "focus_selection": "oldest_membership"
  },
  "foci_preferred": [
    {
      "type": "livekit",
      "livekit_service_url": "https://jwt.yourserver.com"
    }
  ]
}
```

## LiveKit room alias derivation

Both lk-jwt-service and the bot use the same formula:

```
room_alias = base64_unpadded(SHA256(matrixRoomId + "|" + "m.call#ROOM"))
```

For breakout rooms:
```
breakout_alias = base64_unpadded(SHA256(matrixRoomId + "|" + "m.call#BREAKOUT#" + breakoutId))
```

## Room capability state event (`eu.kiefte.wally.conference`)

When the bot joins a room, it sets a custom state event of type `eu.kiefte.wally.conference` (state key: `""`). This advertises the bot's presence and capabilities to Matrix clients.

**Content:**
```json
{
  "version": "0.1.0",
  "endpoint": "https://yourserver.com/wally-conference",
  "bot_user_id": "@wally-conference:yourserver.com",
  "features": ["guest_join", "breakout_rooms", "webhooks"]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `version` | string | Bot protocol version (semver) |
| `endpoint` | string | Public URL of the bot's HTTP API (from `public_url` config) |
| `bot_user_id` | string | The bot's Matrix user ID |
| `features` | array | List of supported features |

**Client usage:** A Matrix client can check for this state event to detect whether guest calling is available in a room. If present, the client can show a "Guest link" button that calls the bot's `/join` endpoint (using `endpoint` from the event) and shares the resulting `ec_url`. The `features` array lets the client selectively show UI for breakout rooms and other capabilities.

The bot updates this event whenever it restarts (to reflect version changes) or when its configuration changes.

## Breakout rooms

Breakout rooms are **LiveKit-only** — no Matrix room is created. The bot:

1. Generates a breakout ID (short UUID)
2. Computes a breakout room alias using the formula above
3. Issues new JWTs pointing at the breakout room alias
4. Clears guest call.member events from the main room
5. Sends a custom state event tracking breakout membership
6. When the breakout ends, re-issues JWTs for the main room

LiveKit rooms auto-create when the first participant joins, so no LiveKit API call is needed to "create" a breakout room.

## E2EE considerations

| Mode | Guest support | Notes |
|------|--------------|-------|
| `NONE` | Full | No encryption, works immediately |
| `SHARED_KEY` | Full | Guest needs the password (included in join link) |
| `PER_PARTICIPANT` | Not supported | Keys distributed via Matrix E2EE to-device messages; guest has no Matrix identity to receive keys |

Most voice/video rooms use `NONE` or `SHARED_KEY`. The Wally patch already disables `PER_PARTICIPANT` for non-encrypted rooms.
