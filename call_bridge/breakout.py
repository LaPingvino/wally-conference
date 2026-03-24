"""Breakout room management.

Creates, ends, and moves guests between breakout rooms.  Each breakout is a
separate LiveKit room with an alias derived from the parent Matrix room ID
and a short breakout identifier.
"""

from __future__ import annotations

import time
import uuid
from typing import TYPE_CHECKING
from urllib.parse import quote, urlencode

if TYPE_CHECKING:
    from mautrix.api import HTTPAPI
    from mautrix.util.async_db import Database

from .db import (
    create_breakout as db_create_breakout,
    get_breakout,
    get_session,
    delete_session,
    end_breakout_db,
    get_sessions_for_breakout,
    update_session_breakout,
)
from .identity import livekit_breakout_alias, livekit_identity
from .jwt_service import make_guest_jwt
from .membership import clear_call_member


async def create_breakout(
    db: Database,
    matrix_room_id: str,
    topic: str | None,
    created_by: str,
) -> dict:
    """Create a new breakout room.

    Generates a breakout_id, computes the LiveKit alias, and stores the
    breakout in the database.

    Returns:
        A dict with ``breakout_id``, ``lk_alias``, and ``topic``.
    """
    breakout_id = uuid.uuid4().hex[:8]
    lk_alias = livekit_breakout_alias(matrix_room_id, breakout_id)

    await db_create_breakout(
        db=db,
        breakout_id=breakout_id,
        matrix_room_id=matrix_room_id,
        topic=topic,
        lk_alias=lk_alias,
        created_by=created_by,
    )

    return {
        "breakout_id": breakout_id,
        "lk_alias": lk_alias,
        "topic": topic,
    }


async def end_breakout(
    db: Database,
    client_api: HTTPAPI,
    breakout_id: str,
) -> int:
    """End a breakout room.

    Clears all guest sessions that are in this breakout (removes their
    call.member state events) and marks the breakout as ended in the DB.

    Returns:
        The number of guest sessions that were cleaned up.
    """
    breakout = await get_breakout(db, breakout_id)
    if breakout is None:
        raise ValueError(f"Breakout {breakout_id} not found")

    if breakout.get("ended_at") is not None:
        raise ValueError(f"Breakout {breakout_id} already ended")

    # Clear all guest sessions in this breakout
    sessions = await get_sessions_for_breakout(db, breakout_id)
    for session in sessions:
        try:
            await clear_call_member(
                client_api, session["room_id"], session["state_key"]
            )
        except Exception:
            pass  # best effort
        await delete_session(db, session["id"])

    await end_breakout_db(db, breakout_id)
    return len(sessions)


async def move_to_breakout(
    db: Database,
    session_id: str,
    breakout_id: str,
    client_api: HTTPAPI,
    lk_api_key: str,
    lk_api_secret: str,
    livekit_url: str,
    ec_base_url: str,
    ttl_seconds: int,
) -> dict:
    """Move a guest session to a breakout room.

    Issues a new JWT for the breakout room's LiveKit alias, updates the
    session in the DB, and clears the old call.member state event (guests
    in breakouts are LiveKit-only; no Matrix room).

    Returns:
        A dict with ``jwt``, ``livekit_room``, ``ec_url``.
    """
    session = await get_session(db, session_id)
    if session is None:
        raise ValueError(f"Session {session_id} not found")

    breakout = await get_breakout(db, breakout_id)
    if breakout is None:
        raise ValueError(f"Breakout {breakout_id} not found")
    if breakout.get("ended_at") is not None:
        raise ValueError(f"Breakout {breakout_id} already ended")

    lk_room = breakout["lk_alias"]

    # Recompute identity for the same session (identity stays the same,
    # only the room changes)
    lk_ident = livekit_identity(
        session["bot_user_id"], session["device_id"], session["id"]
    )

    jwt_token = make_guest_jwt(
        lk_api_key=lk_api_key,
        lk_api_secret=lk_api_secret,
        livekit_room_alias=lk_room,
        participant_identity=lk_ident,
        participant_name=session["display_name"],
        ttl_seconds=ttl_seconds,
    )

    # Clear the call.member from the main room (guest is leaving main)
    try:
        await clear_call_member(
            client_api, session["room_id"], session["state_key"]
        )
    except Exception:
        pass  # best effort

    # Update DB
    new_expires = int(time.time()) + ttl_seconds
    await update_session_breakout(db, session_id, breakout_id, lk_room, new_expires)

    # Build EC URL for breakout
    ec_params = urlencode(
        {
            "embed": "true",
            "widgetId": f"guest-{session_id[:8]}",
            "roomId": breakout["matrix_room_id"],
            "livekitToken": jwt_token,
            "livekitRoom": lk_room,
            "livekitUrl": livekit_url,
            "displayName": session["display_name"],
            "skipLobby": "true",
            "header": "none",
        },
        quote_via=quote,
    )
    ec_url = f"{ec_base_url}?{ec_params}"

    return {
        "jwt": jwt_token,
        "livekit_room": lk_room,
        "ec_url": ec_url,
    }
