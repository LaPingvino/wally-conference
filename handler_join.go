package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"maunium.net/go/mautrix/id"

	"github.com/google/uuid"
)

// HandleJoin handles POST /join — guest join request.
func (svc *Service) HandleJoin(w http.ResponseWriter, r *http.Request) {
	allowedOrigins := svc.Config.AllowedOrigins

	// Rate limit by IP
	remoteIP := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		remoteIP = forwarded
	}
	if !svc.Limiter.Check(remoteIP) {
		AddCORSHeaders(w, allowedOrigins)
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "Rate limited"})
		return
	}

	// Parse body
	var body struct {
		RoomID      string `json:"room_id"`
		DisplayName string `json:"display_name"`
		BreakoutID  string `json:"breakout_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		AddCORSHeaders(w, allowedOrigins)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}

	// Validate room_id
	if errMsg := ValidateRoomID(body.RoomID); errMsg != "" {
		AddCORSHeaders(w, allowedOrigins)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
		return
	}

	// Validate display_name
	displayName, errMsg := ValidateDisplayName(body.DisplayName)
	if errMsg != "" {
		AddCORSHeaders(w, allowedOrigins)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
		return
	}

	// Check bot is in the room
	roomID := id.RoomID(body.RoomID)
	ctx := context.Background()
	members, err := svc.Client.JoinedMembers(ctx, roomID)
	if err != nil {
		AddCORSHeaders(w, allowedOrigins)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Bot not in room"})
		return
	}
	if _, ok := members.Joined[id.UserID(svc.BotUserID)]; !ok {
		AddCORSHeaders(w, allowedOrigins)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Bot not in room"})
		return
	}

	// Check guest capacity
	activeCount, err := CountActiveSessions(svc.DB, body.RoomID)
	if err != nil {
		logf("join", "Error counting sessions: %v", err)
		AddCORSHeaders(w, allowedOrigins)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal error"})
		return
	}
	if activeCount >= svc.Config.MaxGuestsPerRoom {
		AddCORSHeaders(w, allowedOrigins)
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "Guest capacity reached"})
		return
	}

	// Generate identifiers
	sessionID := uuid.New().String()
	deviceID := fmt.Sprintf("GUEST_%s", uuid.New().String()[:8])

	// Compute state key: _@bot:server_DEVICE_ID
	stateKey := fmt.Sprintf("_%s_%s", svc.BotUserID, deviceID)

	// Token TTL
	ttlSeconds := svc.Config.GuestTokenTTL
	expiresMS := ttlSeconds * 1000

	// Resolve active focus and get LiveKit JWT
	jwtToken, livekitURL, lkRoom, tokenMeta, err := svc.GetGuestToken(
		ctx, body.RoomID, roomID, deviceID, sessionID, displayName, ttlSeconds,
	)
	if err != nil {
		logf("join", "Failed to get guest token: %v", err)
		AddCORSHeaders(w, allowedOrigins)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create JWT"})
		return
	}
	lkIdent := LiveKitIdentity(svc.BotUserID, deviceID)

	// Compute expiry timestamp
	expiresAt := time.Now().Unix() + int64(ttlSeconds)

	// Store session in DB
	session := &GuestSession{
		ID:          sessionID,
		RoomID:      body.RoomID,
		BotUserID:   svc.BotUserID,
		DeviceID:    deviceID,
		DisplayName: displayName,
		LKIdentity:  lkIdent,
		LKRoom:      lkRoom,
		BreakoutID:  sql.NullString{String: body.BreakoutID, Valid: body.BreakoutID != ""},
		CreatedAt:   time.Now().Unix(),
		ExpiresAt:   expiresAt,
		StateKey:    stateKey,
	}
	if err := CreateSession(svc.DB, session); err != nil {
		logf("join", "Failed to create session: %v", err)
		AddCORSHeaders(w, allowedOrigins)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal error"})
		return
	}

	// Send call.member state event
	if err := SendCallMember(
		ctx, svc.Client, roomID, stateKey,
		deviceID, sessionID,
		svc.Config.LiveKitServiceURL, body.RoomID, expiresMS,
	); err != nil {
		logf("join", "Failed to send call.member event: %v", err)
		// Still return the JWT so the guest can connect to LiveKit
	}

	// Build EC URL
	parentURL := svc.Config.PublicURL
	if parentURL == "" {
		parentURL = "https://localhost"
	}
	ecParams := url.Values{
		"embed":              {"true"},
		"widgetId":           {fmt.Sprintf("guest-%s", sessionID[:8])},
		"parentUrl":          {parentURL},
		"roomId":             {body.RoomID},
		"livekitToken":       {jwtToken},
		"livekitRoom":        {lkRoom},
		"livekitUrl":         {livekitURL},
		"displayName":        {displayName},
		"skipLobby":          {"true"},
		"header":             {"none"},
		"perParticipantE2EE": {"false"},
	}
	ecURL := fmt.Sprintf("%s?%s", svc.Config.ECBaseURL, ecParams.Encode())

	AddCORSHeaders(w, allowedOrigins)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"jwt":          jwtToken,
		"livekit_url":  livekitURL,
		"livekit_room": lkRoom,
		"session_id":   sessionID,
		"ec_url":       ecURL,
		"expires_at":   expiresAt,
		// Debug info
		"debug": map[string]interface{}{
			"matrix_room_id":      body.RoomID,
			"lk_room_alias":       lkRoom,
			"lk_identity":         lkIdent,
			"device_id":           deviceID,
			"state_key":           stateKey,
			"lk_service_url":      svc.Config.LiveKitServiceURL,
			"alias_input":         body.RoomID + "|m.call#ROOM",
			"focus_source":        tokenMeta.FocusSource,
			"alias_mode":          svc.Config.LiveKitRoomAliasMode,
			"alias_hash":          LiveKitRoomAlias(body.RoomID),
			"alias_raw":           body.RoomID,
			"call_members_count":  tokenMeta.ActiveMembers,
		},
	})
}

// HandleCORSPreflight handles OPTIONS requests for CORS preflight.
func (svc *Service) HandleCORSPreflight(w http.ResponseWriter, r *http.Request) {
	CORSPreflight(w, svc.Config.AllowedOrigins)
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
