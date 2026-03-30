package main

import (
	"context"
	"time"

	"maunium.net/go/mautrix/id"
)

// CleanupLoop periodically expires stale guest sessions.
// It runs until the context is cancelled.
func CleanupLoop(ctx context.Context, svc *Service) {
	interval := time.Duration(svc.Config.CleanupInterval) * time.Second
	logf("cleanup", "Cleanup task started (interval=%s)", interval)

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
