package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"
)

// HandleBreakoutCreate handles POST /breakout/create.
func (svc *Service) HandleBreakoutCreate(w http.ResponseWriter, r *http.Request) {
	allowedOrigins := svc.Config.AllowedOrigins

	var body struct {
		RoomID string `json:"room_id"`
		Topic  string `json:"topic"`
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		AddCORSHeaders(w, allowedOrigins)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}

	if errMsg := ValidateRoomID(body.RoomID); errMsg != "" {
		AddCORSHeaders(w, allowedOrigins)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
		return
	}

	topic := body.Topic
	if topic == "" {
		AddCORSHeaders(w, allowedOrigins)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "topic is required"})
		return
	}

	// Check breakout capacity
	active, err := CountActiveBreakouts(svc.DB, body.RoomID)
	if err != nil {
		log.Printf("Error counting breakouts: %v", err)
		AddCORSHeaders(w, allowedOrigins)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal error"})
		return
	}
	if active >= svc.Config.MaxBreakoutsPerRoom {
		AddCORSHeaders(w, allowedOrigins)
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "Breakout capacity reached"})
		return
	}

	breakoutID := uuid.New().String()[:8]
	lkAlias := LiveKitBreakoutAlias(body.RoomID, breakoutID)

	createdBy := body.UserID
	if createdBy == "" {
		createdBy = "http-api"
	}

	br := &BreakoutRoom{
		ID:           breakoutID,
		MatrixRoomID: body.RoomID,
		Topic:        sql.NullString{String: topic, Valid: topic != ""},
		LKAlias:      lkAlias,
		CreatedBy:    createdBy,
		CreatedAt:    time.Now().Unix(),
	}
	if err := CreateBreakoutRoom(svc.DB, br); err != nil {
		log.Printf("Failed to create breakout: %v", err)
		AddCORSHeaders(w, allowedOrigins)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal error"})
		return
	}

	AddCORSHeaders(w, allowedOrigins)
	writeJSON(w, http.StatusOK, map[string]string{
		"breakout_id":  breakoutID,
		"livekit_room": lkAlias,
		"topic":        topic,
	})
}

// HandleBreakoutMove handles POST /breakout/move.
func (svc *Service) HandleBreakoutMove(w http.ResponseWriter, r *http.Request) {
	allowedOrigins := svc.Config.AllowedOrigins

	var body struct {
		SessionID  string `json:"session_id"`
		BreakoutID string `json:"breakout_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		AddCORSHeaders(w, allowedOrigins)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}

	if body.SessionID == "" || body.BreakoutID == "" {
		AddCORSHeaders(w, allowedOrigins)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_id and breakout_id are required"})
		return
	}

	result, err := svc.moveToBreakout(body.SessionID, body.BreakoutID)
	if err != nil {
		AddCORSHeaders(w, allowedOrigins)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	AddCORSHeaders(w, allowedOrigins)
	writeJSON(w, http.StatusOK, result)
}

// moveToBreakout moves a guest session to a breakout room.
func (svc *Service) moveToBreakout(sessionID, breakoutID string) (map[string]string, error) {
	session, err := GetSession(svc.DB, sessionID)
	if err != nil || session == nil {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	breakout, err := GetBreakout(svc.DB, breakoutID)
	if err != nil || breakout == nil {
		return nil, fmt.Errorf("breakout %s not found", breakoutID)
	}
	if breakout.EndedAt.Valid {
		return nil, fmt.Errorf("breakout %s already ended", breakoutID)
	}

	lkRoom := breakout.LKAlias

	// Recompute identity (stays the same, only room changes)
	lkIdent := LiveKitIdentity(session.BotUserID, session.DeviceID, session.ID)

	jwtToken, err := MakeGuestJWT(
		svc.Config.LiveKitAPIKey,
		svc.Config.LiveKitAPISecret,
		lkRoom,
		lkIdent,
		session.DisplayName,
		svc.Config.GuestTokenTTL,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create JWT: %w", err)
	}

	// Clear the call.member from the main room
	ctx := context.Background()
	if err := ClearCallMember(ctx, svc.Client, id.RoomID(session.RoomID), session.StateKey); err != nil {
		log.Printf("Failed to clear call.member for moved session: %v", err)
	}

	// Update DB
	newExpires := time.Now().Unix() + int64(svc.Config.GuestTokenTTL)
	if err := UpdateSessionBreakout(svc.DB, sessionID, breakoutID, lkRoom, newExpires); err != nil {
		return nil, fmt.Errorf("failed to update session: %w", err)
	}

	// Build EC URL
	ecParams := url.Values{
		"embed":        {"true"},
		"widgetId":     {fmt.Sprintf("guest-%s", sessionID[:8])},
		"roomId":       {breakout.MatrixRoomID},
		"livekitToken": {jwtToken},
		"livekitRoom":  {lkRoom},
		"livekitUrl":   {svc.Config.LiveKitURL},
		"displayName":  {session.DisplayName},
		"skipLobby":    {"true"},
		"header":       {"none"},
	}
	ecURL := fmt.Sprintf("%s?%s", svc.Config.ECBaseURL, ecParams.Encode())

	return map[string]string{
		"jwt":          jwtToken,
		"livekit_room": lkRoom,
		"ec_url":       ecURL,
	}, nil
}
