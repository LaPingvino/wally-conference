package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

// expected computes the expected hash independently for verification.
func expected(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return base64.RawStdEncoding.EncodeToString(h[:])
}

func TestLivekitRoomAlias_Basic(t *testing.T) {
	roomID := "!testRoom:example.com"
	result := LiveKitRoomAlias(roomID)
	want := expected("!testRoom:example.com|m.call#ROOM")
	if result != want {
		t.Errorf("LiveKitRoomAlias(%q) = %q, want %q", roomID, result, want)
	}
}

func TestLivekitRoomAlias_DifferentRoomsDiffer(t *testing.T) {
	a := LiveKitRoomAlias("!room1:example.com")
	b := LiveKitRoomAlias("!room2:example.com")
	if a == b {
		t.Error("different rooms should produce different aliases")
	}
}

func TestLivekitRoomAlias_NoPadding(t *testing.T) {
	result := LiveKitRoomAlias("!testRoom:example.com")
	if strings.Contains(result, "=") {
		t.Errorf("result should not contain padding, got %q", result)
	}
}

func TestLivekitRoomAlias_Deterministic(t *testing.T) {
	a := LiveKitRoomAlias("!abc:example.com")
	b := LiveKitRoomAlias("!abc:example.com")
	if a != b {
		t.Error("same input should produce same output")
	}
}

func TestLivekitRoomAlias_KnownVector(t *testing.T) {
	// Cross-check: SHA256 of "!testRoom:example.com|m.call#ROOM" as unpadded base64.
	raw := "!testRoom:example.com|m.call#ROOM"
	h := sha256.Sum256([]byte(raw))
	wantHex := hex.EncodeToString(h[:])

	result := LiveKitRoomAlias("!testRoom:example.com")

	// Decode our result back to hex and compare.
	// RawStdEncoding has no padding, so we can decode directly.
	decoded, err := base64.RawStdEncoding.DecodeString(result)
	if err != nil {
		t.Fatalf("failed to decode result: %v", err)
	}
	gotHex := hex.EncodeToString(decoded)
	if gotHex != wantHex {
		t.Errorf("hex mismatch: got %s, want %s", gotHex, wantHex)
	}
}

func TestLivekitIdentity_Basic(t *testing.T) {
	result := LiveKitIdentity("@user:example.com", "DEVICE1", "session-uuid")
	want := expected("@user:example.com|DEVICE1|session-uuid")
	if result != want {
		t.Errorf("LiveKitIdentity() = %q, want %q", result, want)
	}
}

func TestLivekitIdentity_DifferentSessionsDiffer(t *testing.T) {
	a := LiveKitIdentity("@user:example.com", "DEV1", "session-a")
	b := LiveKitIdentity("@user:example.com", "DEV1", "session-b")
	if a == b {
		t.Error("different sessions should differ")
	}
}

func TestLivekitIdentity_DifferentDevicesDiffer(t *testing.T) {
	a := LiveKitIdentity("@user:example.com", "DEV1", "session-a")
	b := LiveKitIdentity("@user:example.com", "DEV2", "session-a")
	if a == b {
		t.Error("different devices should differ")
	}
}

func TestLivekitIdentity_NoPadding(t *testing.T) {
	result := LiveKitIdentity("@user:example.com", "DEVICE1", "session-uuid")
	if strings.Contains(result, "=") {
		t.Errorf("result should not contain padding, got %q", result)
	}
}

func TestLivekitIdentity_BotGuestIdentity(t *testing.T) {
	// Simulate the bot proxying a guest: bot userId + synthetic device + session UUID.
	result := LiveKitIdentity(
		"@call-bridge:kiefte.eu",
		"GUEST_a3f9c1",
		"550e8400-e29b-41d4-a716-446655440000",
	)
	want := expected("@call-bridge:kiefte.eu|GUEST_a3f9c1|550e8400-e29b-41d4-a716-446655440000")
	if result != want {
		t.Errorf("bot guest identity: got %q, want %q", result, want)
	}
}

func TestLivekitBreakoutAlias_Basic(t *testing.T) {
	result := LiveKitBreakoutAlias("!room:example.com", "abc123")
	want := expected("!room:example.com|m.call#BREAKOUT#abc123")
	if result != want {
		t.Errorf("LiveKitBreakoutAlias() = %q, want %q", result, want)
	}
}

func TestLivekitBreakoutAlias_DiffersFromMainRoom(t *testing.T) {
	main := LiveKitRoomAlias("!room:example.com")
	breakout := LiveKitBreakoutAlias("!room:example.com", "abc123")
	if main == breakout {
		t.Error("breakout alias should differ from main room alias")
	}
}

func TestLivekitBreakoutAlias_DifferentIDsDiffer(t *testing.T) {
	a := LiveKitBreakoutAlias("!room:example.com", "group-a")
	b := LiveKitBreakoutAlias("!room:example.com", "group-b")
	if a == b {
		t.Error("different breakout IDs should produce different aliases")
	}
}

func TestLivekitBreakoutAlias_NoPadding(t *testing.T) {
	result := LiveKitBreakoutAlias("!room:example.com", "test")
	if strings.Contains(result, "=") {
		t.Errorf("result should not contain padding, got %q", result)
	}
}
