"""Send and clear org.matrix.msc3401.call.member state events.

The bot sends these events as itself, using a per-guest state key
(``_@bot:server_GUEST_xxx``) so that Element Call sees each guest as a
distinct call participant.
"""

from __future__ import annotations

import time
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from mautrix.api import HTTPAPI


async def send_call_member(
    client_api: HTTPAPI,
    room_id: str,
    state_key: str,
    device_id: str,
    session_id: str,
    lk_service_url: str,
    expires_ms: int,
) -> None:
    """Send a call.member state event into *room_id*.

    Args:
        client_api: The bot's mautrix HTTPAPI (``self.client.api``).
        room_id: Matrix room ID to send the event into.
        state_key: Full state key, e.g. ``_@bot:hs_GUEST_abc123``.
        device_id: Synthetic device ID for this guest (e.g. ``GUEST_abc123``).
        session_id: UUID4 session / membership ID.
        lk_service_url: The LiveKit JWT service URL for foci_preferred.
        expires_ms: Membership expiry in milliseconds from now.
    """
    content = {
        "application": "m.call",
        "call_id": "",
        "scope": "m.room",
        "device_id": device_id,
        "expires": expires_ms,
        "created_ts": int(time.time() * 1000),
        "focus_active": {
            "type": "livekit",
            "focus_selection": "oldest_membership",
        },
        "foci_preferred": [
            {
                "type": "livekit",
                "livekit_service_url": lk_service_url,
            }
        ],
    }

    await client_api.request(
        method="PUT",
        path=f"/_matrix/client/v3/rooms/{room_id}/state/org.matrix.msc3401.call.member/{state_key}",
        content=content,
    )


async def clear_call_member(
    client_api: HTTPAPI,
    room_id: str,
    state_key: str,
) -> None:
    """Clear a call.member state event (empty content signals departure).

    Args:
        client_api: The bot's mautrix HTTPAPI.
        room_id: Matrix room ID.
        state_key: The state key to clear.
    """
    await client_api.request(
        method="PUT",
        path=f"/_matrix/client/v3/rooms/{room_id}/state/org.matrix.msc3401.call.member/{state_key}",
        content={},
    )
