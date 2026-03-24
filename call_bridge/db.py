"""Database schema and helpers for guest sessions and breakout rooms.

Uses maubot's built-in database API (asyncpg / aiosqlite via mautrix).
"""

from __future__ import annotations

from mautrix.util.async_db import Database, UpgradeTable

upgrade_table = UpgradeTable()


@upgrade_table.register(description="Initial schema: guest_session and breakout_room")
async def upgrade_v1(db: Database) -> None:
    await db.execute(
        """
        CREATE TABLE IF NOT EXISTS guest_session (
            id           TEXT PRIMARY KEY,
            room_id      TEXT NOT NULL,
            bot_user_id  TEXT NOT NULL,
            device_id    TEXT NOT NULL,
            display_name TEXT NOT NULL,
            lk_identity  TEXT NOT NULL,
            lk_room      TEXT NOT NULL,
            breakout_id  TEXT,
            created_at   INTEGER NOT NULL,
            expires_at   INTEGER NOT NULL,
            state_key    TEXT NOT NULL
        )
        """
    )
    await db.execute(
        """
        CREATE TABLE IF NOT EXISTS breakout_room (
            id              TEXT PRIMARY KEY,
            matrix_room_id  TEXT NOT NULL,
            topic           TEXT,
            lk_alias        TEXT NOT NULL,
            created_by      TEXT NOT NULL,
            created_at      INTEGER NOT NULL,
            ended_at        INTEGER
        )
        """
    )


# ── Query helpers ────────────────────────────────────────


async def create_session(
    db: Database,
    session_id: str,
    room_id: str,
    bot_user_id: str,
    device_id: str,
    display_name: str,
    lk_identity: str,
    lk_room: str,
    state_key: str,
    expires_at: int,
    breakout_id: str | None = None,
) -> None:
    """Insert a new guest session row."""
    import time

    await db.execute(
        """
        INSERT INTO guest_session
            (id, room_id, bot_user_id, device_id, display_name,
             lk_identity, lk_room, breakout_id, created_at, expires_at, state_key)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
        """,
        session_id,
        room_id,
        bot_user_id,
        device_id,
        display_name,
        lk_identity,
        lk_room,
        breakout_id,
        int(time.time()),
        expires_at,
        state_key,
    )


async def get_session_by_identity(db: Database, lk_identity: str) -> dict | None:
    """Look up a guest session by its LiveKit identity hash."""
    row = await db.fetchrow(
        "SELECT * FROM guest_session WHERE lk_identity = $1", lk_identity
    )
    return dict(row) if row else None


async def get_session(db: Database, session_id: str) -> dict | None:
    """Look up a guest session by its primary key."""
    row = await db.fetchrow(
        "SELECT * FROM guest_session WHERE id = $1", session_id
    )
    return dict(row) if row else None


async def delete_session(db: Database, session_id: str) -> None:
    """Remove a guest session."""
    await db.execute("DELETE FROM guest_session WHERE id = $1", session_id)


async def count_active_sessions(db: Database, room_id: str) -> int:
    """Count active (non-expired) guest sessions in a room."""
    import time

    row = await db.fetchrow(
        "SELECT COUNT(*) AS cnt FROM guest_session WHERE room_id = $1 AND expires_at > $2",
        room_id,
        int(time.time()),
    )
    return row["cnt"] if row else 0


async def get_expired_sessions(db: Database) -> list[dict]:
    """Return all sessions whose expiry has passed."""
    import time

    rows = await db.fetch(
        "SELECT * FROM guest_session WHERE expires_at <= $1", int(time.time())
    )
    return [dict(r) for r in rows]


# ── Breakout helpers ─────────────────────────────────────


async def create_breakout(
    db: Database,
    breakout_id: str,
    matrix_room_id: str,
    topic: str | None,
    lk_alias: str,
    created_by: str,
) -> None:
    """Insert a new breakout room row."""
    import time

    await db.execute(
        """
        INSERT INTO breakout_room
            (id, matrix_room_id, topic, lk_alias, created_by, created_at, ended_at)
        VALUES ($1, $2, $3, $4, $5, $6, NULL)
        """,
        breakout_id,
        matrix_room_id,
        topic,
        lk_alias,
        created_by,
        int(time.time()),
    )


async def get_breakout(db: Database, breakout_id: str) -> dict | None:
    """Look up a breakout room by ID."""
    row = await db.fetchrow(
        "SELECT * FROM breakout_room WHERE id = $1", breakout_id
    )
    return dict(row) if row else None


async def count_active_breakouts(db: Database, matrix_room_id: str) -> int:
    """Count active (non-ended) breakout rooms for a parent room."""
    row = await db.fetchrow(
        "SELECT COUNT(*) AS cnt FROM breakout_room WHERE matrix_room_id = $1 AND ended_at IS NULL",
        matrix_room_id,
    )
    return row["cnt"] if row else 0
