package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// OpenIDToken is what a Matrix client gets from /openid/request_token and hands to a service
// to prove who it is.
type OpenIDToken struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	MatrixServerName string `json:"matrix_server_name"`
	ExpiresIn        int    `json:"expires_in"`
}

// A Matrix server name: a DNS name or IP literal, optional port. No paths, no userinfo, no
// query — it goes into a URL we fetch.
var serverNameRe = regexp.MustCompile(`^(?:[A-Za-z0-9.-]+|\[[0-9A-Fa-f:.]+\])(?::[0-9]{1,5})?$`)

var openIDHTTP = &http.Client{Timeout: 10 * time.Second}

// verifyOpenIDURL is swapped in tests.
var verifyOpenIDURL = func(serverName, accessToken string) string {
	return fmt.Sprintf("https://%s/_matrix/federation/v1/openid/userinfo?access_token=%s",
		serverName, url.QueryEscape(accessToken))
}

// VerifyOpenID asks the token's homeserver who it belongs to.
//
// Audit 2026-11-08: the server answering is the one the CLIENT names, so its answer only counts
// for its own users. Before, any server could vouch for any user ID: run a server that answers
// {"sub":"@joop:kiefte.eu"} and you got a LiveKit token under Joop's name.
func VerifyOpenID(ctx context.Context, tok OpenIDToken) (id.UserID, error) {
	if tok.AccessToken == "" {
		return "", errors.New("openid_token is required")
	}
	if !serverNameRe.MatchString(tok.MatrixServerName) {
		return "", errors.New("invalid matrix_server_name")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, verifyOpenIDURL(tok.MatrixServerName, tok.AccessToken), nil)
	if err != nil {
		return "", err
	}
	resp, err := openIDHTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("verify OpenID token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("verify OpenID token: HTTP %d", resp.StatusCode)
	}
	var info struct {
		Sub string `json:"sub"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil || info.Sub == "" {
		return "", errors.New("invalid OpenID userinfo")
	}
	userID := id.UserID(info.Sub)
	_, server, err := userID.Parse()
	if err != nil || !strings.EqualFold(server, tok.MatrixServerName) {
		return "", fmt.Errorf("%s does not belong to %s", info.Sub, tok.MatrixServerName)
	}
	return userID, nil
}

// isJoinedMember reports whether user is joined to room, as the bot sees it.
func (svc *Service) isJoinedMember(ctx context.Context, roomID id.RoomID, user id.UserID) (bool, error) {
	members, err := svc.Client.JoinedMembers(ctx, roomID)
	if err != nil {
		return false, err
	}
	_, ok := members.Joined[user]
	return ok, nil
}

// canManageBreakouts: the user's power level reaches what the room asks for setting breakout
// state — the level at which they could end a breakout by hand anyway.
func (svc *Service) canManageBreakouts(ctx context.Context, roomID id.RoomID, user id.UserID) bool {
	var pl event.PowerLevelsEventContent
	if err := svc.Client.StateEvent(ctx, roomID, event.StatePowerLevels, "", &pl); err != nil {
		return false
	}
	return pl.GetUserLevel(user) >= pl.GetEventLevel(breakoutEventType)
}
