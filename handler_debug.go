package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
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
		"matrix_room_id":   roomID,
		"active_mode":      activeMode,
		"active_alias":     activeAlias,
		"alias_hash_mode":  aliasHash,
		"alias_raw_mode":   aliasRaw,
		"alias_input":      roomID + "|m.call#ROOM",
		"livekit_url":      svc.Config.LiveKitURL,
		"lk_service_url":   svc.Config.LiveKitServiceURL,
	}

	// Try to query LiveKit server for active rooms
	lkURL := svc.Config.LiveKitURL
	// Convert wss:// to https:// for API calls
	httpURL := strings.Replace(lkURL, "wss://", "https://", 1)
	httpURL = strings.Replace(httpURL, "ws://", "http://", 1)

	roomClient := lksdk.NewRoomServiceClient(httpURL, svc.Config.LiveKitAPIKey, svc.Config.LiveKitAPISecret)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// List all active rooms
	rooms, err := roomClient.ListRooms(ctx, nil)
	if err != nil {
		result["lk_error"] = fmt.Sprintf("ListRooms failed: %v", err)
	} else {
		var roomList []map[string]interface{}
		for _, rm := range rooms.GetRooms() {
			rmInfo := map[string]interface{}{
				"name":             rm.Name,
				"sid":              rm.Sid,
				"num_participants": rm.NumParticipants,
				"num_publishers":   rm.NumPublishers,
				"created_at":       rm.CreationTime,
				"matches_active_alias": rm.Name == activeAlias,
				"matches_hash":        rm.Name == aliasHash,
				"matches_raw":         rm.Name == aliasRaw,
			}

			// If this room matches our alias, list participants
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
							"identity":   p.Identity,
							"name":       p.Name,
							"sid":        p.Sid,
							"state":      p.State.String(),
							"joined_at":  p.JoinedAt,
							"tracks":     tracks,
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

// LiveKitRoomAlias2 computes alias with a custom slot suffix (for debug comparison).
func LiveKitRoomAlias2(matrixRoomID, slot string) string {
	return hashUnpaddedBase64(matrixRoomID + "|" + slot)
}
