package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
		log.Printf("No active call memberships in %s, using own config", roomID)
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
			log.Printf("Active focus for %s: %s (from %s/%s, age %ds)",
				roomID, f.LiveKitServiceURL, oldest.Sender, oldest.DeviceID,
				(now-oldest.CreatedTS)/1000)
			return f.LiveKitServiceURL, f.LiveKitAlias, nil
		}
	}

	log.Printf("No livekit focus found in oldest membership for %s, using own config", roomID)
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
	log.Printf("Requesting SFU token from %s for room %s device %s", sfuURL, matrixRoomID, deviceID)

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

	log.Printf("Got SFU token from %s: url=%s", sfuURL, sfuResp.URL)
	return &sfuResp, nil
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
) (jwt, livekitURL, lkRoom string, err error) {
	// Resolve which SFU to use
	serviceURL, lkAlias, err := svc.ResolveActiveFocus(ctx, roomID)
	if err != nil {
		log.Printf("Failed to resolve focus, falling back to local: %v", err)
		serviceURL = svc.Config.LiveKitServiceURL
	}

	// If the active focus is our own service, issue JWT locally (faster, no round-trip)
	if serviceURL == svc.Config.LiveKitServiceURL {
		lkRoom = LiveKitRoomAliasForMode(matrixRoomID, svc.Config.LiveKitRoomAliasMode)
		lkIdent := LiveKitIdentity(svc.BotUserID, deviceID, sessionID)
		jwt, err = MakeGuestJWT(
			svc.Config.LiveKitAPIKey,
			svc.Config.LiveKitAPISecret,
			lkRoom,
			lkIdent,
			displayName,
			ttlSeconds,
		)
		if err != nil {
			return "", "", "", fmt.Errorf("create local JWT: %w", err)
		}
		return jwt, svc.Config.LiveKitURL, lkRoom, nil
	}

	// Active focus is on a different server — request token via their lk-jwt-service
	sfuResp, err := svc.RequestSFUToken(ctx, serviceURL, matrixRoomID, deviceID)
	if err != nil {
		return "", "", "", fmt.Errorf("request remote SFU token: %w", err)
	}

	// Use the alias from the state event if available, otherwise from the response
	if lkAlias == "" {
		lkAlias = matrixRoomID // fallback
	}

	return sfuResp.JWT, sfuResp.URL, lkAlias, nil
}
