package main

import (
	"time"

	"github.com/livekit/protocol/auth"
)

// MakeGuestJWT generates a LiveKit access token (JWT) for a guest participant.
func MakeGuestJWT(
	apiKey, apiSecret string,
	livekitRoom string,
	participantIdentity string,
	participantName string,
	ttlSeconds int,
) (string, error) {
	at := auth.NewAccessToken(apiKey, apiSecret)
	grant := &auth.VideoGrant{
		Room:                 livekitRoom,
		RoomJoin:             true,
		CanPublish:           boolPtr(true),
		CanSubscribe:         boolPtr(true),
		CanPublishData:       boolPtr(true),
		CanUpdateOwnMetadata: boolPtr(true),
	}

	at.SetIdentity(participantIdentity).
		SetName(participantName).
		SetVideoGrant(grant).
		SetValidFor(time.Duration(ttlSeconds) * time.Second)

	return at.ToJWT()
}

func boolPtr(b bool) *bool {
	return &b
}
