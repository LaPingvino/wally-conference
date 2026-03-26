package main

import (
	"crypto/sha256"
	"encoding/base64"
)

// hashUnpaddedBase64 computes SHA-256 of data and returns unpadded standard base64.
func hashUnpaddedBase64(data string) string {
	h := sha256.Sum256([]byte(data))
	return base64.RawStdEncoding.EncodeToString(h[:])
}

// LiveKitRoomAlias derives the LiveKit room name from a Matrix room ID
// using the MSC4195 hash formula: base64_unpadded(SHA256(roomId | "m.call#ROOM")).
func LiveKitRoomAlias(matrixRoomID string) string {
	return hashUnpaddedBase64(matrixRoomID + "|m.call#ROOM")
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

// LiveKitIdentity derives a LiveKit participant identity from a membership triple.
//
// Formula: base64_unpadded(SHA256(userId | deviceId | sessionId))
func LiveKitIdentity(userID, deviceID, sessionID string) string {
	return hashUnpaddedBase64(userID + "|" + deviceID + "|" + sessionID)
}

// LiveKitBreakoutAlias derives a LiveKit room name for a breakout room.
//
// Formula: base64_unpadded(SHA256(roomId | "m.call#BREAKOUT#" + breakoutId))
func LiveKitBreakoutAlias(matrixRoomID, breakoutID string) string {
	return hashUnpaddedBase64(matrixRoomID + "|m.call#BREAKOUT#" + breakoutID)
}
