"""LiveKit identity hash functions.

These must produce identical output to lk-jwt-service's Go implementation.
The separator is a literal pipe character ``|``.  Output is unpadded base64.
"""

import base64
import hashlib


def _hash_unpadded_b64(data: str) -> str:
    """SHA-256 hash *data* (UTF-8) and return unpadded base64."""
    digest = hashlib.sha256(data.encode()).digest()
    return base64.b64encode(digest).decode().rstrip("=")


def livekit_room_alias(matrix_room_id: str) -> str:
    """Derive the LiveKit room name from a Matrix room ID.

    Formula: ``base64_unpadded(SHA256(roomId | "m.call#ROOM"))``

    The ``|`` is a literal pipe separator, matching lk-jwt-service and MSC4195.
    """
    return _hash_unpadded_b64(f"{matrix_room_id}|m.call#ROOM")


def livekit_identity(user_id: str, device_id: str, session_id: str) -> str:
    """Derive a LiveKit participant identity from a membership triple.

    Formula: ``base64_unpadded(SHA256(userId | deviceId | sessionId))``
    """
    return _hash_unpadded_b64(f"{user_id}|{device_id}|{session_id}")


def livekit_breakout_alias(matrix_room_id: str, breakout_id: str) -> str:
    """Derive a LiveKit room name for a breakout room.

    Formula: ``base64_unpadded(SHA256(roomId | "m.call#BREAKOUT#" + breakoutId))``
    """
    return _hash_unpadded_b64(f"{matrix_room_id}|m.call#BREAKOUT#{breakout_id}")
