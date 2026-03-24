"""Test vectors for LiveKit identity hash functions.

These must match lk-jwt-service's Go implementation exactly.
Verified against: SHA256 of the raw input string, base64-encoded with padding stripped.
"""

import base64
import hashlib

import sys
import os

# Add parent directory to path so we can import the module directly
# without triggering the maubot-dependent __init__.py
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from call_bridge.identity import (  # noqa: E402
    livekit_breakout_alias,
    livekit_identity,
    livekit_room_alias,
)


def _expected(raw: str) -> str:
    """Compute the expected hash independently for verification."""
    digest = hashlib.sha256(raw.encode()).digest()
    return base64.b64encode(digest).decode().rstrip("=")


class TestLivekitRoomAlias:
    def test_basic(self):
        room_id = "!testRoom:example.com"
        result = livekit_room_alias(room_id)
        expected = _expected("!testRoom:example.com|m.call#ROOM")
        assert result == expected

    def test_different_rooms_differ(self):
        a = livekit_room_alias("!room1:example.com")
        b = livekit_room_alias("!room2:example.com")
        assert a != b

    def test_no_padding(self):
        result = livekit_room_alias("!testRoom:example.com")
        assert "=" not in result

    def test_deterministic(self):
        a = livekit_room_alias("!abc:example.com")
        b = livekit_room_alias("!abc:example.com")
        assert a == b

    def test_known_vector(self):
        """Cross-check: SHA256 of '!testRoom:example.com|m.call#ROOM' as unpadded base64."""
        raw = "!testRoom:example.com|m.call#ROOM"
        digest = hashlib.sha256(raw.encode()).hexdigest()
        result = livekit_room_alias("!testRoom:example.com")
        # Decode our result back to hex and compare
        decoded = base64.b64decode(result + "==")  # re-pad for decoding
        assert decoded.hex() == digest


class TestLivekitIdentity:
    def test_basic(self):
        result = livekit_identity("@user:example.com", "DEVICE1", "session-uuid")
        expected = _expected("@user:example.com|DEVICE1|session-uuid")
        assert result == expected

    def test_different_sessions_differ(self):
        a = livekit_identity("@user:example.com", "DEV1", "session-a")
        b = livekit_identity("@user:example.com", "DEV1", "session-b")
        assert a != b

    def test_different_devices_differ(self):
        a = livekit_identity("@user:example.com", "DEV1", "session-a")
        b = livekit_identity("@user:example.com", "DEV2", "session-a")
        assert a != b

    def test_no_padding(self):
        result = livekit_identity("@user:example.com", "DEVICE1", "session-uuid")
        assert "=" not in result

    def test_bot_guest_identity(self):
        """Simulate the bot proxying a guest: bot userId + synthetic device + session UUID."""
        result = livekit_identity(
            "@call-bridge:kiefte.eu", "GUEST_a3f9c1", "550e8400-e29b-41d4-a716-446655440000"
        )
        expected = _expected(
            "@call-bridge:kiefte.eu|GUEST_a3f9c1|550e8400-e29b-41d4-a716-446655440000"
        )
        assert result == expected


class TestLivekitBreakoutAlias:
    def test_basic(self):
        result = livekit_breakout_alias("!room:example.com", "abc123")
        expected = _expected("!room:example.com|m.call#BREAKOUT#abc123")
        assert result == expected

    def test_differs_from_main_room(self):
        main = livekit_room_alias("!room:example.com")
        breakout = livekit_breakout_alias("!room:example.com", "abc123")
        assert main != breakout

    def test_different_breakout_ids_differ(self):
        a = livekit_breakout_alias("!room:example.com", "group-a")
        b = livekit_breakout_alias("!room:example.com", "group-b")
        assert a != b

    def test_no_padding(self):
        result = livekit_breakout_alias("!room:example.com", "test")
        assert "=" not in result
