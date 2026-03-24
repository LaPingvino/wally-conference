# Degradation Matrix

Call Bridge is designed for incremental deployment. Each component adds value independently, and missing components degrade gracefully.

## Components

| Code | Component | What it provides |
|------|-----------|-----------------|
| **A** | Maubot plugin | JWT issuance, call.member proxy, guest management |
| **B** | EC patch | Element Call accepts pre-issued LiveKit JWT via URL param |
| **C** | LiveKit webhook | Immediate cleanup when guest disconnects |
| **W** | Wally guest UI | Shareable link page with name entry |

## Combination matrix

| A | B | C | W | Guest access | Breakout | Behavior |
|:-:|:-:|:-:|:-:|:------------|:---------|:---------|
| - | - | - | - | No | No | **Current state.** Calls work normally for Matrix users only. |
| - | B | - | - | No | No | EC patch is inert — `livekitToken` param ignored when absent. **Zero impact.** Safe to deploy early. |
| A | - | - | - | API-only | API-only | Bot issues JWTs and sends call.member events. Guests must manually construct EC URLs (developer/testing use). All clients see guest via state events. |
| A | B | - | - | Manual URL | Manual | Guests join via bot-issued JWT + patched EC URL. call.member sent so all clients see guest. **No automatic cleanup on disconnect** — guest "ghosts" until background task (5 min) or TTL expiry (2h). |
| A | B | C | - | Full (API) | Full (API) | **Full backend.** Guest joins via `POST /join`, gets JWT, opens EC URL. Webhook cleans up immediately on disconnect. Only missing the nice shareable-link UI. |
| A | B | C | W | **Full** | **Full** | **Complete system.** Shareable link → name entry → auto-join → instant cleanup. |

## Individual failure modes

### Bot goes offline

| Aspect | Impact |
|--------|--------|
| New guest joins | **Blocked** — no JWT issuance |
| Existing guests in call | **Unaffected** — LiveKit doesn't care about the bot |
| Guest disconnects | call.member lingers until TTL expiry (EC auto-hides after `expires` ms) |
| Normal Matrix users | **Unaffected** — they use lk-jwt-service, not the bot |
| Breakout management | **Blocked** — no new breakouts or moves |
| Recovery | Restart maubot. No data loss (DB persists). |

### LiveKit webhook endpoint unreachable

| Aspect | Impact |
|--------|--------|
| Guest disconnects | call.member not immediately cleared |
| Fallback | Bot's background cleanup task runs every 5 min; `expires` field in call.member causes EC to auto-hide after TTL |
| Other calls | **Unaffected** |
| Recovery | Fix network/proxy. LiveKit retries webhooks with backoff. |

### Bot loses Matrix connection

| Aspect | Impact |
|--------|--------|
| JWT issuance | **Still works** — no Matrix needed for LiveKit JWT generation |
| call.member events | **Blocked** — guests join LiveKit but are invisible to Matrix clients |
| Patched EC | Shows guest via LiveKit participant list fallback (if implemented) |
| Non-patched clients | Guest is a "ghost" — in the call but invisible |
| Recovery | Reconnect. Bot sends pending state events. |

### LiveKit goes down

| Aspect | Impact |
|--------|--------|
| All calls | **Broken** — not just guests, all participants lose media |
| Matrix side | **Unaffected** — state events and messages still work |
| Recovery | Restart LiveKit. EC has reconnection logic; participants rejoin automatically. |

### Database lost/corrupted

| Aspect | Impact |
|--------|--------|
| New guest joins | **Still work** — JWT issuance is stateless |
| Cleanup | **Broken** — bot can't find sessions to clean up |
| Orphaned state events | Expire via TTL (default 2h) |
| Recovery | Recreate DB. Orphans self-heal via TTL. |

## Design principles

1. **TTL as ultimate safety net**: Every call.member event has an `expires` field. Even if all cleanup mechanisms fail, participants auto-disappear after TTL.

2. **Stateless JWT issuance**: The bot can issue valid LiveKit JWTs without consulting the database or Matrix. The DB is only needed for tracking (cleanup, capacity limits).

3. **Bot is non-critical for existing calls**: The bot only manages guest lifecycle. Normal Matrix users use lk-jwt-service independently. If the bot crashes, ongoing calls continue.

4. **EC patch is a no-op by default**: The `livekitToken` URL param is only checked when present. Normal EC behavior is completely unchanged.
