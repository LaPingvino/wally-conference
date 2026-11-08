package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
)

// hashUnpaddedBase64 computes SHA-256 of data and returns unpadded standard base64.
func hashUnpaddedBase64(data string) string {
	h := sha256.Sum256([]byte(data))
	return base64.RawStdEncoding.EncodeToString(h[:])
}

// marshalStrings JSON-encodes a string list the way lk-jwt-service does (serde_json: no HTML
// escaping) — the input of every MSC4195 hash.
func marshalStrings(ss ...string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(ss) // a []string always encodes
	return strings.TrimSuffix(buf.String(), "\n")
}

// LiveKitRoomAliasForSlot is MSC4195's room alias, as lk-jwt-service (v0.4.1+) computes it:
// base64_unpadded(SHA256(JSON([roomId, slotId]))).
func LiveKitRoomAliasForSlot(matrixRoomID, slotID string) string {
	return hashUnpaddedBase64(marshalStrings(matrixRoomID, slotID))
}

// LiveKitRoomAlias is the LiveKit room of a room's main call (slot "m.call#ROOM").
//
// Audit 2026-11-08: this was SHA256(roomId + "|m.call#ROOM"), an earlier MSC4195 draft. The
// lk-jwt-service here (0.9.0) hashes JSON([roomId, slot]), so guests whose token the bot signed
// itself sat alone in a LiveKit room no member was in.
func LiveKitRoomAlias(matrixRoomID string) string {
	return LiveKitRoomAliasForSlot(matrixRoomID, "m.call#ROOM")
}

// LiveKitRoomAliasForMode returns the LiveKit room name based on the alias mode.
// "raw" = use Matrix room ID directly (older lk-jwt-service).
// "hash" = SHA256 hash per MSC4195 (newer lk-jwt-service).
func LiveKitRoomAliasForMode(matrixRoomID, mode string) string {
	if mode == "hash" {
		return LiveKitRoomAlias(matrixRoomID)
	}
	return matrixRoomID // raw mode — pass through
}

// LiveKitIdentity returns the LiveKit participant identity for a call member.
//
// Element Call expects the LK identity to match the format it reads from
// call.member state events: "userId:deviceId". If the identity doesn't match,
// EC's MatrixAudioRenderer refuses to render the participant's tracks.
func LiveKitIdentity(userID, deviceID string) string {
	return userID + ":" + deviceID
}

// LiveKitBreakoutAlias derives a LiveKit room name for a breakout room.
//
// Formula: base64_unpadded(SHA256(roomId | "m.call#BREAKOUT#" + breakoutId))
func LiveKitBreakoutAlias(matrixRoomID, breakoutID string) string {
	return hashUnpaddedBase64(matrixRoomID + "|m.call#BREAKOUT#" + breakoutID)
}
