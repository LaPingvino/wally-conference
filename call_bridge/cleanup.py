"""Background task that periodically expires stale guest sessions.

Runs as a long-lived ``asyncio.Task`` started by the bot's ``start()``
method and cancelled in ``stop()``.  Every *cleanup_interval* seconds it
queries the database for sessions whose ``expires_at`` has passed, clears
their ``call.member`` state events, and deletes the DB rows.
"""

from __future__ import annotations

import asyncio
import logging
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from mautrix.api import HTTPAPI
    from mautrix.util.async_db import Database

from .db import get_expired_sessions, delete_session
from .membership import clear_call_member

log = logging.getLogger("maubot.wally.cleanup")


async def cleanup_loop(
    db: Database,
    client_api: HTTPAPI,
    interval: int,
) -> None:
    """Run the cleanup cycle forever (until cancelled).

    Args:
        db: The maubot database handle.
        client_api: The bot's Matrix HTTPAPI for clearing state events.
        interval: Seconds between each cleanup sweep.
    """
    log.info("Cleanup task started (interval=%ds)", interval)
    try:
        while True:
            await asyncio.sleep(interval)
            await _run_cleanup(db, client_api)
    except asyncio.CancelledError:
        log.info("Cleanup task cancelled")
        raise


async def _run_cleanup(db: Database, client_api: HTTPAPI) -> None:
    """Perform a single cleanup sweep."""
    expired = await get_expired_sessions(db)
    if not expired:
        return

    log.info("Cleaning up %d expired guest session(s)", len(expired))

    for session in expired:
        try:
            await clear_call_member(client_api, session["room_id"], session["state_key"])
            log.debug(
                "Cleared call.member for expired session %s (room=%s)",
                session["id"],
                session["room_id"],
            )
        except Exception as exc:
            log.error(
                "Failed to clear call.member for expired session %s: %s",
                session["id"],
                exc,
            )

        await delete_session(db, session["id"])

    log.info("Expired session cleanup complete: removed %d session(s)", len(expired))
