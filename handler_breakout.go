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
		RoomID      string      `json:"room_id"`
		Topic       string      `json:"topic"`
		OpenIDToken OpenIDToken `json:"openid_token"`
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

	// Who is asking, and are they in the room? (Audit 2026-11-08: this took a bare user_id, so
	// anyone who could reach the endpoint created breakouts in any room the bot is in.)
	ctx := r.Context()
	userID, ok := svc.authorizeMember(ctx, w, r, body.OpenIDToken, id.RoomID(body.RoomID))
	if !ok {
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

	createdBy := userID.String()

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

	// Publish as state event
	svc.sendBreakoutStateEvent(context.Background(), id.RoomID(body.RoomID), breakoutID, map[string]interface{}{
		"topic":      topic,
		"lk_alias":   lkAlias,
		"created_by": createdBy,
		"active":     true,
	})

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
		BreakoutID  string      `json:"breakout_id"`
		DeviceID    string      `json:"device_id"`
		OpenIDToken OpenIDToken `json:"openid_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}

	if body.BreakoutID == "" || body.OpenIDToken.AccessToken == "" || body.DeviceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "breakout_id, device_id, and openid_token are required"})
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

	// Only members of the breakout's room (audit 2026-11-08: any Matrix user could join any
	// breakout, and /breakout/list hands out the IDs).
	userID, ok := svc.authorizeMember(r.Context(), w, r, body.OpenIDToken, id.RoomID(breakout.MatrixRoomID))
	if !ok {
		return
	}

	// Create JWT with the user's real identity
	lkIdent := LiveKitIdentity(userID.String(), body.DeviceID)
	displayName := userID.String() // Best we have without profile lookup
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
		"user_id":      userID.String(),
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
		BreakoutID  string      `json:"breakout_id"`
		OpenIDToken OpenIDToken `json:"openid_token"`
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

	// Its creator, or someone whose power level could set breakout state anyway (audit
	// 2026-11-08: anyone could end any breakout by ID).
	userID, ok := svc.authorizeMember(r.Context(), w, r, body.OpenIDToken, id.RoomID(breakout.MatrixRoomID))
	if !ok {
		return
	}
	if userID.String() != breakout.CreatedBy && !svc.canManageBreakouts(r.Context(), id.RoomID(breakout.MatrixRoomID), userID) {
		SetCORS(w, r, allowedOrigins)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Only its creator or a room moderator can end this breakout"})
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

	// Clear state event
	svc.sendBreakoutStateEvent(ctx, id.RoomID(breakout.MatrixRoomID), body.BreakoutID, map[string]interface{}{
		"active": false,
	})

	SetCORS(w, r, allowedOrigins)
	writeJSON(w, http.StatusOK, map[string]string{
		"status":      "ended",
		"breakout_id": body.BreakoutID,
	})
}

// authorizeMember verifies the OpenID token and that its user is joined to room. On failure it
// has written the response (with CORS) and returns ok=false.
func (svc *Service) authorizeMember(ctx context.Context, w http.ResponseWriter, r *http.Request, tok OpenIDToken, room id.RoomID) (id.UserID, bool) {
	userID, err := VerifyOpenID(ctx, tok)
	if err != nil {
		log.Printf("breakout: OpenID verification failed: %v", err)
		SetCORS(w, r, svc.Config.AllowedOrigins)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Could not verify your Matrix identity"})
		return "", false
	}
	joined, err := svc.isJoinedMember(ctx, room, userID)
	if err != nil {
		log.Printf("breakout: membership check for %s in %s failed: %v", userID, room, err)
		SetCORS(w, r, svc.Config.AllowedOrigins)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not check room membership"})
		return "", false
	}
	if !joined {
		SetCORS(w, r, svc.Config.AllowedOrigins)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "You are not in this room"})
		return "", false
	}
	return userID, true
}
