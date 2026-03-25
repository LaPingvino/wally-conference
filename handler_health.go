package main

import (
	"log"
	"net/http"
)

// HandleHealth handles GET /health — health check endpoint.
func (svc *Service) HandleHealth(w http.ResponseWriter, r *http.Request) {
	guests, err := CountAllActiveSessions(svc.DB)
	if err != nil {
		log.Printf("Error counting sessions: %v", err)
	}

	breakouts, err := CountAllActiveBreakouts(svc.DB)
	if err != nil {
		log.Printf("Error counting breakouts: %v", err)
	}

	lkOK := svc.Config.LiveKitAPIKey != "" && svc.Config.LiveKitAPISecret != ""

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":             "ok",
		"active_guests":      guests,
		"active_breakouts":   breakouts,
		"matrix_connected":   true,
		"livekit_configured": lkOK,
	})
}
