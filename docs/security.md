# Security Model

## Trust boundaries

```
Untrusted          Trusted (server-side)        Trusted (infrastructure)
─────────          ─────────────────────        ─────────────────────────
Guest browser  →   Call Bridge bot          →   LiveKit SFU
                   - validates room_id          - verifies JWT signature
                   - rate limits by IP          - enforces room isolation
                   - checks capacity
                   - signs JWT server-side
                   - sends Matrix state events

                   Matrix homeserver
                   - authenticates bot account
                   - enforces room power levels
                   - distributes state events
```

## What guests CAN do

- Join a specific LiveKit room (the one in their JWT)
- Publish audio/video
- Subscribe to other participants' audio/video
- Appear in the participant list (via bot's call.member event)
- Send LiveKit data messages (chat within EC)

## What guests CANNOT do

- Join any other LiveKit room (JWT is room-specific)
- Send Matrix messages (they have no Matrix account)
- Read Matrix room history
- Access other rooms on the homeserver
- Stay beyond JWT TTL (LiveKit disconnects expired tokens)
- Impersonate a Matrix user (identity is bot-scoped)

## Attack surface analysis

### Malicious guest floods /join endpoint

**Mitigation:** Rate limiting per IP (default 5/minute). Capacity limit per room (default 20). Configurable in plugin config.

### Malicious guest shares JWT with others

**Impact:** Limited. JWT is room-specific and time-limited. Multiple connections with the same identity are handled by LiveKit (last connection wins). The bot tracks sessions and can `!wc kick` specific guests.

### Attacker brute-forces room IDs

**Mitigation:** Bot verifies it has joined the room before issuing JWT. Rooms the bot isn't in return 403. Rate limiting prevents rapid enumeration.

### Attacker intercepts guest JWT

**Impact:** Can join the call for the JWT's remaining TTL. Same as intercepting any bearer token.

**Mitigation:** Use HTTPS for all endpoints. JWTs are short-lived (default 2h).

### LiveKit API key/secret leaked

**Impact:** Critical — attacker can issue JWTs for any room on the LiveKit server.

**Mitigation:** Keys are only stored in the Maubot config (server-side). Never sent to browsers. Rotate keys if compromised.

### Bot account compromised

**Impact:** Attacker can send state events in any room the bot has joined. Can create fake guest participants.

**Mitigation:** Use a strong password. Don't give the bot admin power level. Monitor bot activity via Matrix audit logs. Use a dedicated bot account (not a human account).

### Guest stays after call ends

**Mitigation:** TTL on call.member events (default 2h). Background cleanup task (every 5 min). LiveKit webhook for immediate cleanup. Bot can `!wc kick` manually.

## E2EE trust model

| E2EE Mode | Guest trust level | Notes |
|-----------|------------------|-------|
| `NONE` | N/A | No encryption. Guest sees everything. |
| `SHARED_KEY` | Same as any participant with the password | Password is in the join link. Anyone with the link can decrypt. |
| `PER_PARTICIPANT` | **Not supported** | Requires Matrix E2EE identity. Guest has none. |

## Recommendations for production

1. **Set `allowed_origins`** to your specific client domain
2. **Set `admin_rooms`** to a private admin room
3. **Use HTTPS** for all endpoints (Maubot, LiveKit, Matrix)
4. **Set `require_active_call: true`** to prevent guests joining empty rooms
5. **Monitor** the bot's Matrix activity for unexpected state events
6. **Rotate** LiveKit API keys periodically
7. **Keep `max_guests_per_room` reasonable** to prevent resource exhaustion
