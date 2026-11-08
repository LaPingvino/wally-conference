package main

import (
	"context"
	"encoding/json"
	"fmt"
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

// TokenMeta carries additional metadata from token generation for debug/observability.
type TokenMeta struct {
	FocusSource   string // "local" or "remote:<url>"
	ActiveMembers int
}

// GetGuestToken issues a guest's LiveKit JWT on the bot's OWN SFU.
//
// Multi-SFU (Element Call's default since v0.21; it dropped "oldest membership" in v0.25):
// every member publishes on its own homeserver's SFU. This used to follow the oldest member's
// focus and, when that was another server's, ask THAT lk-jwt-service — which gives users of
// other servers a subscribe-only token, so the guest couldn't be heard (audit 2026-11-08).
// The active-membership count is still read for RequireActiveCall.
func (svc *Service) GetGuestToken(
	ctx context.Context,
	matrixRoomID string,
	roomID id.RoomID,
	deviceID string,
	sessionID string,
	displayName string,
	ttlSeconds int,
) (jwt, livekitURL, lkRoom string, meta TokenMeta, err error) {
	if active, aerr := svc.getActiveCallMemberships(ctx, roomID); aerr == nil {
		meta.ActiveMembers = len(active)
	} else {
		logf("focus", "Could not count call memberships in %s: %v", roomID, aerr)
	}
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
