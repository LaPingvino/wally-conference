package main

import (
	"context"
	"log"
	"time"

	"maunium.net/go/mautrix/id"
)

// CleanupLoop periodically expires stale guest sessions.
// It runs until the context is cancelled.
func CleanupLoop(ctx context.Context, svc *Service) {
	interval := time.Duration(svc.Config.CleanupInterval) * time.Second
	log.Printf("Cleanup task started (interval=%s)", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Cleanup task cancelled")
			return
		case <-ticker.C:
			runCleanup(ctx, svc)
		}
	}
}

func runCleanup(ctx context.Context, svc *Service) {
	expired, err := GetExpiredSessions(svc.DB)
	if err != nil {
		log.Printf("Error fetching expired sessions: %v", err)
		return
	}

	if len(expired) == 0 {
		return
	}

	log.Printf("Cleaning up %d expired guest session(s)", len(expired))

	for _, session := range expired {
		if err := ClearCallMember(ctx, svc.Client, id.RoomID(session.RoomID), session.StateKey); err != nil {
			log.Printf("Failed to clear call.member for expired session %s: %v", session.ID, err)
		} else {
			log.Printf("Cleared call.member for expired session %s (room=%s)", session.ID, session.RoomID)
		}

		if err := DeleteSession(svc.DB, session.ID); err != nil {
			log.Printf("Failed to delete expired session %s: %v", session.ID, err)
		}
	}

	log.Printf("Expired session cleanup complete: removed %d session(s)", len(expired))

	// Also clean up rate limiter memory
	svc.Limiter.Cleanup()
}
