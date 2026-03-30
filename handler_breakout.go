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
		SetCORS(w, r, allowedOrigins)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}

	if errMsg := ValidateRoomID(body.RoomID); errMsg != "" {
		SetCORS(w, r, allowedOrigins)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
		return
	}

	topic := body.Topic
	if topic == "" {
		SetCORS(w, r, allowedOrigins)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "topic is required"})
		return
	}

	// Check breakout capacity
	active, err := CountActiveBreakouts(svc.DB, body.RoomID)
	if err != nil {
		log.Printf("Error counting breakouts: %v", err)
		SetCORS(w, r, allowedOrigins)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal error"})
		return
	}
	if active >= svc.Config.MaxBreakoutsPerRoom {
		SetCORS(w, r, allowedOrigins)
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
		SetCORS(w, r, allowedOrigins)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal error"})
		return
	}

	SetCORS(w, r, allowedOrigins)
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
		SetCORS(w, r, allowedOrigins)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}

	if body.SessionID == "" || body.BreakoutID == "" {
		SetCORS(w, r, allowedOrigins)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_id and breakout_id are required"})
		return
	}

	result, err := svc.moveToBreakout(body.SessionID, body.BreakoutID)
	if err != nil {
		SetCORS(w, r, allowedOrigins)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	SetCORS(w, r, allowedOrigins)
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
	lkIdent := LiveKitIdentity(session.BotUserID, session.DeviceID)

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
		"livekit_url":  svc.Config.LiveKitURL,
		"livekit_room": lkRoom,
		"ec_url":       ecURL,
	}, nil
}

// HandleBreakoutJoin lets an authenticated Matrix user get a JWT for a breakout room.
// POST /guest/breakout/join — body: { breakout_id, openid_token: { access_token, ... }, device_id }
func (svc *Service) HandleBreakoutJoin(w http.ResponseWriter, r *http.Request) {
	SetCORS(w, r, svc.Config.AllowedOrigins)

	var body struct {
		BreakoutID  string `json:"breakout_id"`
		DeviceID    string `json:"device_id"`
		OpenIDToken struct {
			AccessToken      string `json:"access_token"`
			TokenType        string `json:"token_type"`
			MatrixServerName string `json:"matrix_server_name"`
			ExpiresIn        int    `json:"expires_in"`
		} `json:"openid_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}

	if body.BreakoutID == "" || body.OpenIDToken.AccessToken == "" || body.DeviceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "breakout_id, device_id, and openid_token are required"})
		return
	}

	// Verify the OpenID token with the user's homeserver
	verifyURL := fmt.Sprintf("https://%s/_matrix/federation/v1/openid/userinfo?access_token=%s",
		body.OpenIDToken.MatrixServerName, url.QueryEscape(body.OpenIDToken.AccessToken))
	resp, err := http.Get(verifyURL)
	if err != nil || resp.StatusCode != 200 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Failed to verify OpenID token"})
		return
	}
	defer resp.Body.Close()
	var userInfo struct {
		Sub string `json:"sub"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil || userInfo.Sub == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid OpenID token"})
		return
	}

	// Look up breakout
	breakout, err := GetBreakout(svc.DB, body.BreakoutID)
	if err != nil || breakout == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Breakout not found"})
		return
	}
	if breakout.EndedAt.Valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Breakout has ended"})
		return
	}

	// Create JWT with the user's real identity
	lkIdent := LiveKitIdentity(userInfo.Sub, body.DeviceID)
	displayName := userInfo.Sub // Best we have without profile lookup
	jwtToken, err := MakeGuestJWT(
		svc.Config.LiveKitAPIKey, svc.Config.LiveKitAPISecret,
		breakout.LKAlias, lkIdent, displayName, svc.Config.GuestTokenTTL,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create JWT"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"jwt":          jwtToken,
		"livekit_url":  svc.Config.LiveKitURL,
		"livekit_room": breakout.LKAlias,
		"breakout_id":  body.BreakoutID,
		"user_id":      userInfo.Sub,
	})
}

// HandleBreakoutList handles GET /guest/breakout/list/{roomID}.
func (svc *Service) HandleBreakoutList(w http.ResponseWriter, r *http.Request) {
	allowedOrigins := svc.Config.AllowedOrigins

	rawRoomID := r.PathValue("roomID")
	if rawRoomID == "" {
		SetCORS(w, r, allowedOrigins)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing room ID"})
		return
	}
	roomID, err := url.PathUnescape(rawRoomID)
	if err != nil {
		roomID = rawRoomID
	}

	breakouts, err := GetActiveBreakouts(svc.DB, roomID)
	if err != nil {
		SetCORS(w, r, allowedOrigins)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal error"})
		return
	}

	var result []map[string]interface{}
	for _, br := range breakouts {
		sessions, _ := GetSessionsForBreakout(svc.DB, br.ID)
		topic := ""
		if br.Topic.Valid {
			topic = br.Topic.String
		}
		result = append(result, map[string]interface{}{
			"id":           br.ID,
			"topic":        topic,
			"created_by":   br.CreatedBy,
			"created_at":   br.CreatedAt,
			"participants": len(sessions),
		})
	}

	if result == nil {
		result = []map[string]interface{}{}
	}

	SetCORS(w, r, allowedOrigins)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"room_id":   roomID,
		"breakouts": result,
	})
}

// HandleBreakoutEnd handles POST /guest/breakout/end.
func (svc *Service) HandleBreakoutEnd(w http.ResponseWriter, r *http.Request) {
	allowedOrigins := svc.Config.AllowedOrigins

	var body struct {
		BreakoutID string `json:"breakout_id"`
		UserID     string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		SetCORS(w, r, allowedOrigins)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}

	if body.BreakoutID == "" {
		SetCORS(w, r, allowedOrigins)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "breakout_id is required"})
		return
	}

	breakout, err := GetBreakout(svc.DB, body.BreakoutID)
	if err != nil || breakout == nil {
		SetCORS(w, r, allowedOrigins)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Breakout not found"})
		return
	}
	if breakout.EndedAt.Valid {
		SetCORS(w, r, allowedOrigins)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Breakout already ended"})
		return
	}

	// Clean up guest sessions
	ctx := context.Background()
	sessions, _ := GetSessionsForBreakout(svc.DB, body.BreakoutID)
	for _, session := range sessions {
		ClearCallMember(ctx, svc.Client, id.RoomID(session.RoomID), session.StateKey)
		DeleteSession(svc.DB, session.ID)
	}

	EndBreakoutDB(svc.DB, body.BreakoutID)

	SetCORS(w, r, allowedOrigins)
	writeJSON(w, http.StatusOK, map[string]string{
		"status":      "ended",
		"breakout_id": body.BreakoutID,
	})
}
