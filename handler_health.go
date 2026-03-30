package main

import (
	"net/http"
	"time"
)

// HandleHealth handles GET /health — health check endpoint.
func (svc *Service) HandleHealth(w http.ResponseWriter, r *http.Request) {
	guests, err := CountAllActiveSessions(svc.DB)
	if err != nil {
		logf("health", "Error counting sessions: %v", err)
	}

	breakouts, err := CountAllActiveBreakouts(svc.DB)
	if err != nil {
		logf("health", "Error counting breakouts: %v", err)
	}

	lkOK := svc.Config.LiveKitAPIKey != "" && svc.Config.LiveKitAPISecret != ""

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":                 "ok",
		"active_guests":          guests,
		"active_breakouts":       breakouts,
		"matrix_connected":       true,
		"livekit_configured":     lkOK,
		"livekit_room_alias_mode": svc.Config.LiveKitRoomAliasMode,
		"livekit_url":            svc.Config.LiveKitURL,
		"lk_service_url":         svc.Config.LiveKitServiceURL,
		"ec_base_url":            svc.Config.ECBaseURL,
		"uptime_seconds":         int(time.Since(svc.StartedAt).Seconds()),
		"bot_user_id":            svc.BotUserID,
	})
}
