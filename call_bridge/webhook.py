"""LiveKit webhook receiver.

Verifies webhook authenticity using ``livekit.api.WebhookReceiver`` and
dispatches ``participant_left`` / ``room_finished`` events to clean up
guest sessions and their corresponding call.member state events.
"""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from aiohttp import web as aiohttp_web
from livekit.api import WebhookReceiver, TokenVerifier

if TYPE_CHECKING:
    from mautrix.api import HTTPAPI
    from mautrix.util.async_db import Database

from .db import (
    get_session_by_identity,
    get_sessions_by_lk_room,
    delete_session,
)
from .membership import clear_call_member

log = logging.getLogger("maubot.wally.webhook")


def create_webhook_receiver(api_key: str, api_secret: str) -> WebhookReceiver:
    """Build a ``WebhookReceiver`` that can verify LiveKit webhook JWTs."""
    verifier = TokenVerifier(api_key=api_key, api_secret=api_secret)
    return WebhookReceiver(verifier)


async def handle_webhook(
    request: aiohttp_web.Request,
    receiver: WebhookReceiver,
    db: Database,
    client_api: HTTPAPI,
) -> aiohttp_web.Response:
    """Process an incoming LiveKit webhook request.

    Args:
        request: The aiohttp request from LiveKit.
        receiver: Pre-configured ``WebhookReceiver`` for JWT verification.
        db: The maubot database handle.
        client_api: The bot's Matrix HTTPAPI for clearing state events.

    Returns:
        200 on success (including unknown events), 401 on verification failure.
    """
    body = await request.text()
    auth_header = request.headers.get("Authorization", "")

    try:
        event = receiver.receive(body, auth_header)
    except Exception as exc:
        log.warning("Webhook verification failed: %s", exc)
        return aiohttp_web.Response(status=401, text=str(exc))

    event_type = event.event
    log.debug("Received LiveKit webhook: %s", event_type)

    if event_type == "participant_left":
        await _handle_participant_left(event, db, client_api)
    elif event_type == "room_finished":
        await _handle_room_finished(event, db, client_api)
    else:
        log.debug("Ignoring unhandled webhook event type: %s", event_type)

    # Always return 200 to prevent LiveKit from retrying.
    return aiohttp_web.Response(status=200)


async def _handle_participant_left(event, db: Database, client_api: HTTPAPI) -> None:
    """Clean up a single guest session when they leave the LiveKit room."""
    identity = event.participant.identity
    log.info("participant_left: identity=%s", identity)

    session = await get_session_by_identity(db, identity)
    if session is None:
        log.debug("No guest session found for identity %s (may be a Matrix user)", identity)
        return

    # Clear the call.member state event so EC no longer shows the guest.
    try:
        await clear_call_member(client_api, session["room_id"], session["state_key"])
        log.info(
            "Cleared call.member for session %s (room=%s, state_key=%s)",
            session["id"],
            session["room_id"],
            session["state_key"],
        )
    except Exception as exc:
        log.error("Failed to clear call.member for session %s: %s", session["id"], exc)

    await delete_session(db, session["id"])
    log.info("Deleted guest session %s", session["id"])


async def _handle_room_finished(event, db: Database, client_api: HTTPAPI) -> None:
    """Clean up all guest sessions when a LiveKit room is destroyed."""
    room_name = event.room.name
    log.info("room_finished: lk_room=%s", room_name)

    sessions = await get_sessions_by_lk_room(db, room_name)
    if not sessions:
        log.debug("No guest sessions found for LiveKit room %s", room_name)
        return

    for session in sessions:
        try:
            await clear_call_member(client_api, session["room_id"], session["state_key"])
            log.info(
                "Cleared call.member for session %s (room_finished)",
                session["id"],
            )
        except Exception as exc:
            log.error("Failed to clear call.member for session %s: %s", session["id"], exc)

        await delete_session(db, session["id"])

    log.info("Cleaned up %d guest sessions for LiveKit room %s", len(sessions), room_name)
