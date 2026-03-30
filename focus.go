package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// callMemberContent represents the content of a call.member state event.
type callMemberContent struct {
	Application   string        `json:"application"`
	CallID        string        `json:"call_id"`
	Scope         string        `json:"scope"`
	DeviceID      string        `json:"device_id"`
	Expires       int64         `json:"expires"`
	CreatedTS     int64         `json:"created_ts"`
	FocusActive   focusActive   `json:"focus_active"`
	FociPreferred []focusEntry  `json:"foci_preferred"`
}

type focusActive struct {
	Type           string `json:"type"`
	FocusSelection string `json:"focus_selection"`
}

type focusEntry struct {
	Type             string `json:"type"`
	LiveKitAlias     string `json:"livekit_alias"`
	LiveKitServiceURL string `json:"livekit_service_url"`
}

// activeMembership is a parsed, non-expired call membership.
type activeMembership struct {
	Sender    string
	DeviceID  string
	CreatedTS int64
	Focus     []focusEntry
}

// ResolveActiveFocus reads call.member state from the room, applies the
// "oldest_membership" algorithm, and returns the active focus's
// livekit_service_url and livekit_alias. Falls back to the bot's own config.
func (svc *Service) ResolveActiveFocus(ctx context.Context, roomID id.RoomID) (serviceURL, lkAlias string, err error) {
	stateMap, err := svc.Client.State(ctx, roomID)
	if err != nil {
		return "", "", fmt.Errorf("fetch room state: %w", err)
	}

	callMemberType := event.Type{Type: "org.matrix.msc3401.call.member", Class: event.StateEventType}
	memberEvents := stateMap[callMemberType]

	now := time.Now().UnixMilli()
	var active []activeMembership

	for _, evt := range memberEvents {
		var content callMemberContent
		raw, _ := json.Marshal(evt.Content.Raw)
		if err := json.Unmarshal(raw, &content); err != nil {
			continue
		}
		if content.DeviceID == "" || len(content.FociPreferred) == 0 {
			continue // empty (departed) or no focus info
		}

		created := content.CreatedTS
		if created == 0 {
			created = evt.Timestamp // fallback to event timestamp
		}
		expires := content.Expires
		if expires == 0 {
			expires = 7200000 // default 2h
		}

		if now-created < expires {
			active = append(active, activeMembership{
				Sender:    evt.Sender.String(),
				DeviceID:  content.DeviceID,
				CreatedTS: created,
				Focus:     content.FociPreferred,
			})
		}
	}

	if len(active) == 0 {
		logf("focus", "No active call memberships in %s, using own config", roomID)
		return svc.Config.LiveKitServiceURL, "", nil
	}

	// Find the oldest membership
	oldest := active[0]
	for _, m := range active[1:] {
		if m.CreatedTS < oldest.CreatedTS {
			oldest = m
		}
	}

	// Use the first livekit focus from the oldest member
	for _, f := range oldest.Focus {
		if f.Type == "livekit" && f.LiveKitServiceURL != "" {
			logf("focus", "Active focus for %s: %s (from %s/%s, age %ds)",
				roomID, f.LiveKitServiceURL, oldest.Sender, oldest.DeviceID,
				(now-oldest.CreatedTS)/1000)
			return f.LiveKitServiceURL, f.LiveKitAlias, nil
		}
	}

	logf("focus", "No livekit focus found in oldest membership for %s, using own config", roomID)
	return svc.Config.LiveKitServiceURL, "", nil
}

// sfuGetRequest is the request body for lk-jwt-service's /sfu/get endpoint.
type sfuGetRequest struct {
	Room        string        `json:"room"`
	OpenIDToken openIDToken   `json:"openid_token"`
	DeviceID    string        `json:"device_id"`
}

type openIDToken struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	MatrixServerName string `json:"matrix_server_name"`
	ExpiresIn        int    `json:"expires_in"`
}

// sfuGetResponse is the response from lk-jwt-service.
type sfuGetResponse struct {
	URL string `json:"url"`
	JWT string `json:"jwt"`
}

// RequestSFUToken requests a LiveKit JWT from a remote lk-jwt-service using
// the bot's OpenID token. This lets guests join calls on any federated SFU.
func (svc *Service) RequestSFUToken(ctx context.Context, serviceURL, matrixRoomID, deviceID string) (*sfuGetResponse, error) {
	// Get an OpenID token from our homeserver
	oidResp, err := svc.Client.RequestOpenIDToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("request openid token: %w", err)
	}

	// Build request for lk-jwt-service /sfu/get
	reqBody := sfuGetRequest{
		Room: matrixRoomID,
		OpenIDToken: openIDToken{
			AccessToken:      oidResp.AccessToken,
			TokenType:        oidResp.TokenType,
			MatrixServerName: oidResp.MatrixServerName,
			ExpiresIn:        oidResp.ExpiresIn,
		},
		DeviceID: deviceID,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal sfu request: %w", err)
	}

	// POST to the lk-jwt-service
	sfuURL := serviceURL + "/sfu/get"
	logf("focus", "Requesting SFU token from %s for room %s device %s", sfuURL, matrixRoomID, deviceID)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", sfuURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create sfu request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sfu request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read sfu response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sfu returned %d: %s", resp.StatusCode, string(respBody))
	}

	var sfuResp sfuGetResponse
	if err := json.Unmarshal(respBody, &sfuResp); err != nil {
		return nil, fmt.Errorf("parse sfu response: %w", err)
	}

	logf("focus", "Got SFU token from %s: url=%s", sfuURL, sfuResp.URL)
	return &sfuResp, nil
}

// TokenMeta carries additional metadata from token generation for debug/observability.
type TokenMeta struct {
	FocusSource   string // "local" or "remote:<url>"
	ActiveMembers int
}

// GetGuestToken resolves the active focus for a room and obtains a LiveKit JWT,
// either from the active focus's lk-jwt-service or by issuing one locally.
func (svc *Service) GetGuestToken(
	ctx context.Context,
	matrixRoomID string,
	roomID id.RoomID,
	deviceID string,
	sessionID string,
	displayName string,
	ttlSeconds int,
) (jwt, livekitURL, lkRoom string, meta TokenMeta, err error) {
	// Resolve which SFU to use
	serviceURL, lkAlias, activeMembers, err := svc.ResolveActiveFocusWithCount(ctx, roomID)
	if err != nil {
		logf("focus", "Failed to resolve focus, falling back to local: %v", err)
		serviceURL = svc.Config.LiveKitServiceURL
	}
	meta.ActiveMembers = activeMembers

	// If the active focus is our own service, issue JWT locally (faster, no round-trip)
	if serviceURL == svc.Config.LiveKitServiceURL {
		meta.FocusSource = "local"
		lkRoom = LiveKitRoomAliasForMode(matrixRoomID, svc.Config.LiveKitRoomAliasMode)
		lkIdent := LiveKitIdentity(svc.BotUserID, deviceID)
		jwt, err = MakeGuestJWT(
			svc.Config.LiveKitAPIKey,
			svc.Config.LiveKitAPISecret,
			lkRoom,
			lkIdent,
			displayName,
			ttlSeconds,
		)
		if err != nil {
			return "", "", "", meta, fmt.Errorf("create local JWT: %w", err)
		}
		return jwt, svc.Config.LiveKitURL, lkRoom, meta, nil
	}

	meta.FocusSource = "remote:" + serviceURL

	// Active focus is on a different server — request token via their lk-jwt-service
	sfuResp, err := svc.RequestSFUToken(ctx, serviceURL, matrixRoomID, deviceID)
	if err != nil {
		return "", "", "", meta, fmt.Errorf("request remote SFU token: %w", err)
	}

	// Use the alias from the state event if available, otherwise from the response
	if lkAlias == "" {
		lkAlias = matrixRoomID // fallback
	}

	return sfuResp.JWT, sfuResp.URL, lkAlias, meta, nil
}

// ResolveActiveFocusWithCount is like ResolveActiveFocus but also returns the
// count of active (non-expired) call memberships.
func (svc *Service) ResolveActiveFocusWithCount(ctx context.Context, roomID id.RoomID) (serviceURL, lkAlias string, activeCount int, err error) {
	active, err := svc.getActiveCallMemberships(ctx, roomID)
	if err != nil {
		return "", "", 0, err
	}
	activeCount = len(active)

	if len(active) == 0 {
		logf("focus", "No active call memberships in %s, using own config (via WithCount)", roomID)
		return svc.Config.LiveKitServiceURL, "", 0, nil
	}

	oldest := active[0]
	for _, m := range active[1:] {
		if m.CreatedTS < oldest.CreatedTS {
			oldest = m
		}
	}

	now := time.Now().UnixMilli()
	for _, f := range oldest.Focus {
		if f.Type == "livekit" && f.LiveKitServiceURL != "" {
			logf("focus", "Active focus for %s: %s (from %s/%s, age %ds) [WithCount: %d members]",
				roomID, f.LiveKitServiceURL, oldest.Sender, oldest.DeviceID,
				(now-oldest.CreatedTS)/1000, activeCount)
			return f.LiveKitServiceURL, f.LiveKitAlias, activeCount, nil
		}
	}

	logf("focus", "No livekit focus found in oldest membership for %s, using own config", roomID)
	return svc.Config.LiveKitServiceURL, "", activeCount, nil
}

// getActiveCallMemberships fetches and filters call.member state events in a room.
func (svc *Service) getActiveCallMemberships(ctx context.Context, roomID id.RoomID) ([]activeMembership, error) {
	stateMap, err := svc.Client.State(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("fetch room state: %w", err)
	}

	callMemberType := event.Type{Type: "org.matrix.msc3401.call.member", Class: event.StateEventType}
	memberEvents := stateMap[callMemberType]

	now := time.Now().UnixMilli()
	var active []activeMembership

	for _, evt := range memberEvents {
		var content callMemberContent
		raw, _ := json.Marshal(evt.Content.Raw)
		if err := json.Unmarshal(raw, &content); err != nil {
			continue
		}
		if content.DeviceID == "" || len(content.FociPreferred) == 0 {
			continue
		}

		created := content.CreatedTS
		if created == 0 {
			created = evt.Timestamp
		}
		expires := content.Expires
		if expires == 0 {
			expires = 7200000
		}

		if now-created < expires {
			active = append(active, activeMembership{
				Sender:    evt.Sender.String(),
				DeviceID:  content.DeviceID,
				CreatedTS: created,
				Focus:     content.FociPreferred,
			})
		}
	}

	return active, nil
}

// ResolveActiveFocusTrace runs the focus resolution algorithm with step-by-step
// trace output, returning a structured trace for debug endpoints.
func (svc *Service) ResolveActiveFocusTrace(ctx context.Context, roomID id.RoomID) (map[string]interface{}, error) {
	trace := map[string]interface{}{}

	stateMap, err := svc.Client.State(ctx, roomID)
	if err != nil {
		trace["error"] = fmt.Sprintf("fetch room state: %v", err)
		return trace, err
	}

	callMemberType := event.Type{Type: "org.matrix.msc3401.call.member", Class: event.StateEventType}
	memberEvents := stateMap[callMemberType]
	trace["total_call_member_events"] = len(memberEvents)

	now := time.Now().UnixMilli()
	var active []activeMembership
	var memberDetails []map[string]interface{}

	for stateKey, evt := range memberEvents {
		var content callMemberContent
		raw, _ := json.Marshal(evt.Content.Raw)
		if err := json.Unmarshal(raw, &content); err != nil {
			memberDetails = append(memberDetails, map[string]interface{}{
				"state_key":   stateKey,
				"parse_error": err.Error(),
			})
			continue
		}

		created := content.CreatedTS
		if created == 0 {
			created = evt.Timestamp
		}
		expires := content.Expires
		if expires == 0 {
			expires = 7200000
		}

		ageMs := now - created
		isExpired := ageMs >= expires

		detail := map[string]interface{}{
			"state_key":  stateKey,
			"sender":     evt.Sender.String(),
			"device_id":  content.DeviceID,
			"created_ts": created,
			"expires":    expires,
			"age_ms":     ageMs,
			"age_s":      ageMs / 1000,
			"is_expired": isExpired,
			"is_empty":   content.DeviceID == "" || len(content.FociPreferred) == 0,
		}

		if len(content.FociPreferred) > 0 {
			var foci []map[string]interface{}
			for _, f := range content.FociPreferred {
				foci = append(foci, map[string]interface{}{
					"type":               f.Type,
					"livekit_service_url": f.LiveKitServiceURL,
					"livekit_alias":      f.LiveKitAlias,
				})
			}
			detail["foci_preferred"] = foci
		}

		memberDetails = append(memberDetails, detail)

		if !isExpired && content.DeviceID != "" && len(content.FociPreferred) > 0 {
			active = append(active, activeMembership{
				Sender:    evt.Sender.String(),
				DeviceID:  content.DeviceID,
				CreatedTS: created,
				Focus:     content.FociPreferred,
			})
		}
	}

	trace["members"] = memberDetails
	trace["active_members_count"] = len(active)

	if len(active) == 0 {
		trace["resolution"] = "no active members, fallback to own config"
		trace["selected_service_url"] = svc.Config.LiveKitServiceURL
		trace["is_local"] = true
		return trace, nil
	}

	oldest := active[0]
	for _, m := range active[1:] {
		if m.CreatedTS < oldest.CreatedTS {
			oldest = m
		}
	}

	trace["oldest_member_sender"] = oldest.Sender
	trace["oldest_member_device_id"] = oldest.DeviceID
	trace["oldest_member_created_ts"] = oldest.CreatedTS
	trace["oldest_member_age_s"] = (now - oldest.CreatedTS) / 1000

	selectedURL := ""
	for _, f := range oldest.Focus {
		if f.Type == "livekit" && f.LiveKitServiceURL != "" {
			selectedURL = f.LiveKitServiceURL
			break
		}
	}

	if selectedURL == "" {
		trace["resolution"] = "oldest member has no livekit focus, fallback to own config"
		trace["selected_service_url"] = svc.Config.LiveKitServiceURL
		trace["is_local"] = true
	} else {
		isLocal := selectedURL == svc.Config.LiveKitServiceURL
		trace["selected_service_url"] = selectedURL
		trace["is_local"] = isLocal
		if isLocal {
			trace["resolution"] = "oldest member points to local SFU"
		} else {
			trace["resolution"] = fmt.Sprintf("oldest member points to remote SFU: %s", selectedURL)
		}
	}

	return trace, nil
}
