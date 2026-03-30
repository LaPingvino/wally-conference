package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// CleanupLoop periodically expires stale guest sessions.
// It runs until the context is cancelled.
func CleanupLoop(ctx context.Context, svc *Service) {
	interval := time.Duration(svc.Config.CleanupInterval) * time.Second
	logf("cleanup", "Cleanup task started (interval=%s)", interval)

	// On startup, clear any orphaned call.member state events from previous runs.
	// This handles the case where the bot crashed without cleaning up.
	startupCleanup(ctx, svc)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logf("cleanup", "Cleanup task cancelled")
			return
		case <-ticker.C:
			runCleanup(ctx, svc)
		}
	}
}

// startupCleanup clears stale call.member state events left by previous bot runs.
// It checks all joined rooms for call.member events sent by the bot that have no
// matching active guest session in the DB.
func startupCleanup(ctx context.Context, svc *Service) {
	logf("cleanup", "Running startup cleanup for orphaned call.member events")

	// Get all rooms the bot is in
	resp, err := svc.Client.JoinedRooms(ctx)
	if err != nil {
		logf("cleanup", "Failed to get joined rooms for startup cleanup: %v", err)
		return
	}

	callMemberType := event.Type{Type: "org.matrix.msc3401.call.member", Class: event.StateEventType}
	botPrefix := fmt.Sprintf("_%s_", svc.BotUserID)
	cleaned := 0

	for _, roomID := range resp.JoinedRooms {
		stateMap, err := svc.Client.State(ctx, roomID)
		if err != nil {
			continue
		}

		memberEvents := stateMap[callMemberType]
		for stateKey, evt := range memberEvents {
			// Only check state keys that belong to our bot (guest proxied events)
			if !strings.HasPrefix(stateKey, botPrefix) {
				continue
			}

			// Check if the content is non-empty (empty = already departed)
			var content callMemberContent
			raw, _ := json.Marshal(evt.Content.Raw)
			if err := json.Unmarshal(raw, &content); err != nil || content.DeviceID == "" {
				continue // already cleared or unparseable
			}

			// Clear all bot call.member events on startup — any leftover
			// from a previous run is stale (no delayed leave event support yet)
			logf("cleanup", "Clearing bot call.member in %s (state_key=%s)", roomID, stateKey)
			if err := ClearCallMember(ctx, svc.Client, roomID, stateKey); err != nil {
				logf("cleanup", "Failed to clear call.member: %v", err)
			} else {
				cleaned++
			}
		}
	}

	if cleaned > 0 {
		logf("cleanup", "Startup cleanup complete: cleared %d orphaned call.member event(s)", cleaned)
	} else {
		logf("cleanup", "Startup cleanup complete: no orphans found")
	}
}

func runCleanup(ctx context.Context, svc *Service) {
	expired, err := GetExpiredSessions(svc.DB)
	if err != nil {
		logf("cleanup", "Error fetching expired sessions: %v", err)
		return
	}

	if len(expired) == 0 {
		return
	}

	logf("cleanup", "Cleaning up %d expired guest session(s)", len(expired))

	for _, session := range expired {
		if err := ClearCallMember(ctx, svc.Client, id.RoomID(session.RoomID), session.StateKey); err != nil {
			logf("cleanup", "Failed to clear call.member for expired session %s: %v", session.ID, err)
		} else {
			logf("cleanup", "Cleared call.member for expired session %s (room=%s)", session.ID, session.RoomID)
		}

		if err := DeleteSession(svc.DB, session.ID); err != nil {
			logf("cleanup", "Failed to delete expired session %s: %v", session.ID, err)
		}
	}

	logf("cleanup", "Expired session cleanup complete: removed %d session(s)", len(expired))

	// Also clean up rate limiter memory
	svc.Limiter.Cleanup()
}
