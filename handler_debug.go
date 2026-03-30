package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// HandleDebug shows LiveKit server state for a given room — active rooms,
// participants, computed aliases. Only for development/debugging.
func (svc *Service) HandleDebug(w http.ResponseWriter, r *http.Request) {
	rawRoomID := r.PathValue("roomID")
	if rawRoomID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing room ID"})
		return
	}
	roomID, err := url.PathUnescape(rawRoomID)
	if err != nil {
		roomID = rawRoomID
	}

	// Compute aliases in both modes
	aliasHash := LiveKitRoomAlias(roomID)
	aliasRaw := roomID
	activeMode := svc.Config.LiveKitRoomAliasMode
	activeAlias := LiveKitRoomAliasForMode(roomID, activeMode)

	result := map[string]interface{}{
		"matrix_room_id":  roomID,
		"active_mode":     activeMode,
		"active_alias":    activeAlias,
		"alias_hash_mode": aliasHash,
		"alias_raw_mode":  aliasRaw,
		"alias_input":     roomID + "|m.call#ROOM",
		"livekit_url":     svc.Config.LiveKitURL,
		"lk_service_url":  svc.Config.LiveKitServiceURL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	matrixRoomID := id.RoomID(roomID)

	// ── call.member state events ──
	stateMap, stateErr := svc.Client.State(ctx, matrixRoomID)
	if stateErr != nil {
		result["call_member_error"] = fmt.Sprintf("fetch room state: %v", stateErr)
	} else {
		callMemberType := event.Type{Type: "org.matrix.msc3401.call.member", Class: event.StateEventType}
		memberEvents := stateMap[callMemberType]
		now := time.Now().UnixMilli()

		var memberList []map[string]interface{}
		for stateKey, evt := range memberEvents {
			var content callMemberContent
			raw, _ := json.Marshal(evt.Content.Raw)
			if err := json.Unmarshal(raw, &content); err != nil {
				memberList = append(memberList, map[string]interface{}{
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

			entry := map[string]interface{}{
				"state_key":  stateKey,
				"sender":     evt.Sender.String(),
				"device_id":  content.DeviceID,
				"created_ts": created,
				"expires":    expires,
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
						"livekit_alias":       f.LiveKitAlias,
					})
				}
				entry["foci_preferred"] = foci
			}

			memberList = append(memberList, entry)
		}
		result["call_members"] = memberList
		result["call_members_count"] = len(memberList)
	}

	// ── Focus resolution trace ──
	focusTrace, traceErr := svc.ResolveActiveFocusTrace(ctx, matrixRoomID)
	if traceErr != nil {
		result["focus_trace_error"] = traceErr.Error()
	}
	result["focus_trace"] = focusTrace

	// ── Active guest sessions ──
	sessions, sessErr := GetAllSessionsInRoom(svc.DB, roomID)
	if sessErr != nil {
		result["guest_sessions_error"] = sessErr.Error()
	} else {
		var sessList []map[string]interface{}
		for _, s := range sessions {
			sessList = append(sessList, map[string]interface{}{
				"session_id":   s.ID,
				"device_id":    s.DeviceID,
				"display_name": s.DisplayName,
				"lk_identity":  s.LKIdentity,
				"lk_room":      s.LKRoom,
				"state_key":    s.StateKey,
				"created_at":   s.CreatedAt,
				"expires_at":   s.ExpiresAt,
			})
		}
		result["guest_sessions"] = sessList
		result["guest_sessions_count"] = len(sessList)
	}

	// ── Alias comparison (hash vs lk-jwt-service) ──
	hashAlias := LiveKitRoomAlias(roomID)
	rawAlias := roomID
	match := "MATCH"
	if hashAlias != activeAlias {
		// If current mode is raw, the hash won't match the active alias
		if activeMode == "raw" {
			match = "MISMATCH (mode=raw, lk-jwt-service uses hash)"
		} else {
			match = "MATCH"
		}
	}
	if activeMode == "hash" && hashAlias == activeAlias {
		match = "MATCH"
	}
	result["alias_comparison"] = map[string]interface{}{
		"lk_jwt_service_would_use": hashAlias,
		"bot_active_alias":         activeAlias,
		"bot_alias_mode":           activeMode,
		"raw_alias":                rawAlias,
		"verdict":                  match,
	}

	// ── LiveKit rooms from server ──
	lkURL := svc.Config.LiveKitURL
	httpURL := strings.Replace(lkURL, "wss://", "https://", 1)
	httpURL = strings.Replace(httpURL, "ws://", "http://", 1)

	roomClient := lksdk.NewRoomServiceClient(httpURL, svc.Config.LiveKitAPIKey, svc.Config.LiveKitAPISecret)

	rooms, err := roomClient.ListRooms(ctx, nil)
	if err != nil {
		result["lk_error"] = fmt.Sprintf("ListRooms failed: %v", err)
	} else {
		var roomList []map[string]interface{}
		for _, rm := range rooms.GetRooms() {
			rmInfo := map[string]interface{}{
				"name":                rm.Name,
				"sid":                 rm.Sid,
				"num_participants":    rm.NumParticipants,
				"num_publishers":      rm.NumPublishers,
				"created_at":          rm.CreationTime,
				"matches_active_alias": rm.Name == activeAlias,
				"matches_hash":        rm.Name == aliasHash,
				"matches_raw":         rm.Name == aliasRaw,
			}

			if rm.Name == activeAlias || rm.NumParticipants > 0 {
				parts, err := roomClient.ListParticipants(ctx, &livekit.ListParticipantsRequest{Room: rm.Name})
				if err == nil {
					var partList []map[string]interface{}
					for _, p := range parts.GetParticipants() {
						tracks := []string{}
						for _, t := range p.Tracks {
							tracks = append(tracks, fmt.Sprintf("%s:%s(muted=%v)", t.Source, t.Type, t.Muted))
						}
						partList = append(partList, map[string]interface{}{
							"identity":     p.Identity,
							"name":         p.Name,
							"sid":          p.Sid,
							"state":        p.State.String(),
							"joined_at":    p.JoinedAt,
							"tracks":       tracks,
							"is_publisher": p.IsPublisher,
						})
					}
					rmInfo["participants"] = partList
				} else {
					rmInfo["participants_error"] = err.Error()
				}
			}

			roomList = append(roomList, rmInfo)
		}
		result["lk_rooms"] = roomList
		result["lk_rooms_count"] = len(roomList)
	}

	AddCORSHeaders(w, svc.Config.AllowedOrigins)
	writeJSON(w, http.StatusOK, result)
}

// HandleJoinTrace simulates a guest join WITHOUT creating a session or sending
// state events. Returns what would happen if a guest joined right now.
func (svc *Service) HandleJoinTrace(w http.ResponseWriter, r *http.Request) {
	rawRoomID := r.PathValue("roomID")
	if rawRoomID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing room ID"})
		return
	}
	roomID, err := url.PathUnescape(rawRoomID)
	if err != nil {
		roomID = rawRoomID
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	matrixRoomID := id.RoomID(roomID)
	result := map[string]interface{}{
		"matrix_room_id": roomID,
		"simulated":      true,
	}

	// Check bot is in room
	members, err := svc.Client.JoinedMembers(ctx, matrixRoomID)
	if err != nil {
		result["bot_in_room"] = false
		result["bot_error"] = fmt.Sprintf("could not check room membership: %v", err)
		AddCORSHeaders(w, svc.Config.AllowedOrigins)
		writeJSON(w, http.StatusOK, result)
		return
	}
	if _, ok := members.Joined[id.UserID(svc.BotUserID)]; !ok {
		result["bot_in_room"] = false
		AddCORSHeaders(w, svc.Config.AllowedOrigins)
		writeJSON(w, http.StatusOK, result)
		return
	}
	result["bot_in_room"] = true

	// Guest capacity
	activeCount, err := CountActiveSessions(svc.DB, roomID)
	if err != nil {
		result["guest_count_error"] = err.Error()
	} else {
		result["guest_count"] = activeCount
		result["guest_max"] = svc.Config.MaxGuestsPerRoom
		result["would_be_blocked_by_capacity"] = activeCount >= svc.Config.MaxGuestsPerRoom
	}

	// require_active_call check
	if svc.Config.RequireActiveCall {
		activeMems, err := svc.getActiveCallMemberships(ctx, matrixRoomID)
		if err != nil {
			result["require_active_call_error"] = err.Error()
		} else {
			result["require_active_call"] = true
			result["active_call_members"] = len(activeMems)
			result["would_be_blocked_by_require_active_call"] = len(activeMems) == 0
		}
	} else {
		result["require_active_call"] = false
		result["would_be_blocked_by_require_active_call"] = false
	}

	// Focus resolution
	focusTrace, _ := svc.ResolveActiveFocusTrace(ctx, matrixRoomID)
	result["focus_resolution"] = focusTrace

	// Simulate identifiers
	fakeSessionID := uuid.New().String()
	fakeDeviceID := fmt.Sprintf("GUEST_%s", uuid.New().String()[:8])
	fakeStateKey := fmt.Sprintf("_%s_%s", svc.BotUserID, fakeDeviceID)
	lkIdent := LiveKitIdentity(svc.BotUserID, fakeDeviceID, fakeSessionID)

	// Compute LK room alias
	lkRoomHash := LiveKitRoomAlias(roomID)
	lkRoomRaw := roomID
	lkRoom := LiveKitRoomAliasForMode(roomID, svc.Config.LiveKitRoomAliasMode)

	result["would_use"] = map[string]interface{}{
		"lk_room":      lkRoom,
		"lk_room_hash": lkRoomHash,
		"lk_room_raw":  lkRoomRaw,
		"alias_mode":   svc.Config.LiveKitRoomAliasMode,
		"lk_identity":  lkIdent,
		"device_id":    fakeDeviceID,
		"state_key":    fakeStateKey,
	}

	// Compute what call.member content would be
	ttlSeconds := svc.Config.GuestTokenTTL
	expiresMS := ttlSeconds * 1000
	callMemberContent := map[string]interface{}{
		"application": "m.call",
		"call_id":     "",
		"scope":       "m.room",
		"device_id":   fakeDeviceID,
		"expires":     expiresMS,
		"created_ts":  time.Now().UnixMilli(),
		"focus_active": map[string]interface{}{
			"type":            "livekit",
			"focus_selection": "oldest_membership",
		},
		"foci_preferred": []map[string]interface{}{
			{
				"type":               "livekit",
				"livekit_alias":      roomID,
				"livekit_service_url": svc.Config.LiveKitServiceURL,
			},
		},
	}
	result["would_send_call_member"] = callMemberContent

	// Compute EC URL
	parentURL := svc.Config.PublicURL
	if parentURL == "" {
		parentURL = "https://localhost"
	}

	// Determine livekit URL based on focus
	livekitURL := svc.Config.LiveKitURL
	if selectedURL, ok := focusTrace["selected_service_url"].(string); ok && selectedURL != svc.Config.LiveKitServiceURL {
		livekitURL = "(would be fetched from remote: " + selectedURL + ")"
	}

	ecParams := url.Values{
		"embed":              {"true"},
		"widgetId":           {fmt.Sprintf("guest-%s", fakeSessionID[:8])},
		"parentUrl":          {parentURL},
		"roomId":             {roomID},
		"livekitToken":       {"(would be generated)"},
		"livekitRoom":        {lkRoom},
		"livekitUrl":         {livekitURL},
		"displayName":        {"(guest name)"},
		"skipLobby":          {"true"},
		"header":             {"none"},
		"perParticipantE2EE": {"false"},
	}
	result["would_generate_ec_url"] = fmt.Sprintf("%s?%s", svc.Config.ECBaseURL, ecParams.Encode())

	AddCORSHeaders(w, svc.Config.AllowedOrigins)
	writeJSON(w, http.StatusOK, result)
}

// LiveKitRoomAlias2 computes alias with a custom slot suffix (for debug comparison).
func LiveKitRoomAlias2(matrixRoomID, slot string) string {
	return hashUnpaddedBase64(matrixRoomID + "|" + slot)
}
