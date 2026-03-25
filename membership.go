package main

import (
	"context"
	"fmt"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// callMemberEventType is the Matrix event type for call membership (MSC3401/MSC4143).
var callMemberEventType = event.Type{Type: "org.matrix.msc3401.call.member", Class: event.StateEventType}

// SendCallMember sends a call.member state event into a Matrix room.
//
// The bot sends these events as itself, using a per-guest state key
// (_@bot:server_GUEST_xxx) so that Element Call sees each guest as a
// distinct call participant.
func SendCallMember(
	ctx context.Context,
	client *mautrix.Client,
	roomID id.RoomID,
	stateKey string,
	deviceID string,
	sessionID string,
	lkServiceURL string,
	expiresMS int,
) error {
	content := map[string]interface{}{
		"application": "m.call",
		"call_id":     "",
		"scope":       "m.room",
		"device_id":   deviceID,
		"expires":     expiresMS,
		"created_ts":  time.Now().UnixMilli(),
		"focus_active": map[string]interface{}{
			"type":            "livekit",
			"focus_selection": "oldest_membership",
		},
		"foci_preferred": []map[string]interface{}{
			{
				"type":               "livekit",
				"livekit_service_url": lkServiceURL,
			},
		},
	}

	_, err := client.SendStateEvent(ctx, roomID, callMemberEventType, stateKey, content)
	if err != nil {
		return fmt.Errorf("send call.member: %w", err)
	}
	return nil
}

// ClearCallMember clears a call.member state event (empty content signals departure).
func ClearCallMember(
	ctx context.Context,
	client *mautrix.Client,
	roomID id.RoomID,
	stateKey string,
) error {
	_, err := client.SendStateEvent(ctx, roomID, callMemberEventType, stateKey, map[string]interface{}{})
	if err != nil {
		return fmt.Errorf("clear call.member: %w", err)
	}
	return nil
}
