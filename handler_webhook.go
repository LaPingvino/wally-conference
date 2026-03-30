package main

import (
	"context"
	"net/http"

	"github.com/livekit/protocol/auth"
	livekit "github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/webhook"
	"maunium.net/go/mautrix/id"
)

// HandleWebhook handles POST /webhook — LiveKit webhook events.
func (svc *Service) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	provider := auth.NewSimpleKeyProvider(svc.Config.LiveKitAPIKey, svc.Config.LiveKitAPISecret)

	event, err := webhook.ReceiveWebhookEvent(r, provider)
	if err != nil {
		logf("webhook", "Webhook verification failed: %v", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	logf("webhook", "Received LiveKit webhook: %s", event.GetEvent())

	ctx := context.Background()

	switch event.GetEvent() {
	case "participant_left":
		svc.handleParticipantLeft(ctx, event)
	case "room_finished":
		svc.handleRoomFinished(ctx, event)
	default:
		logf("webhook", "Ignoring unhandled webhook event type: %s", event.GetEvent())
	}

	// Always return 200 to prevent LiveKit from retrying.
	w.WriteHeader(http.StatusOK)
}

func (svc *Service) handleParticipantLeft(ctx context.Context, evt *livekit.WebhookEvent) {
	if evt.GetParticipant() == nil {
		return
	}
	identity := evt.GetParticipant().GetIdentity()
	logf("webhook", "participant_left: identity=%s", identity)

	session, err := GetSessionByIdentity(svc.DB, identity)
	if err != nil {
		logf("webhook", "Error looking up session by identity: %v", err)
		return
	}
	if session == nil {
		logf("webhook", "No guest session found for identity %s (may be a Matrix user)", identity)
		return
	}

	// Clear the call.member state event
	if err := ClearCallMember(ctx, svc.Client, id.RoomID(session.RoomID), session.StateKey); err != nil {
		logf("webhook", "Failed to clear call.member for session %s: %v", session.ID, err)
	} else {
		logf("webhook", "Cleared call.member for session %s (room=%s)", session.ID, session.RoomID)
	}

	if err := DeleteSession(svc.DB, session.ID); err != nil {
		logf("webhook", "Failed to delete session %s: %v", session.ID, err)
	} else {
		logf("webhook", "Deleted guest session %s", session.ID)
	}
}

func (svc *Service) handleRoomFinished(ctx context.Context, evt *livekit.WebhookEvent) {
	if evt.GetRoom() == nil {
		return
	}
	roomName := evt.GetRoom().GetName()
	logf("webhook", "room_finished: lk_room=%s", roomName)

	sessions, err := GetSessionsByLKRoom(svc.DB, roomName)
	if err != nil {
		logf("webhook", "Error looking up sessions for LK room: %v", err)
		return
	}
	if len(sessions) == 0 {
		logf("webhook", "No guest sessions found for LiveKit room %s", roomName)
		return
	}

	for _, session := range sessions {
		if err := ClearCallMember(ctx, svc.Client, id.RoomID(session.RoomID), session.StateKey); err != nil {
			logf("webhook", "Failed to clear call.member for session %s: %v", session.ID, err)
		}
		if err := DeleteSession(svc.DB, session.ID); err != nil {
			logf("webhook", "Failed to delete session %s: %v", session.ID, err)
		}
	}

	logf("webhook", "Cleaned up %d guest sessions for LiveKit room %s", len(sessions), roomName)
}
