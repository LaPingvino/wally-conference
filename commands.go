package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/google/uuid"
)

// HandleMatrixMessage processes incoming Matrix messages and dispatches !wc commands.
func (svc *Service) HandleMatrixMessage(ctx context.Context, evt *event.Event) {
	content := evt.Content.AsMessage()
	if content == nil || content.Body == "" {
		return
	}

	body := strings.TrimSpace(content.Body)
	if !strings.HasPrefix(body, "!wc") {
		return
	}

	parts := strings.Fields(body)
	if len(parts) < 2 {
		svc.sendReply(ctx, evt, cmdHelp)
		return
	}

	roomID := evt.RoomID
	sender := evt.Sender

	switch parts[1] {
	case "status":
		svc.cmdStatus(ctx, evt, roomID)
	case "link":
		svc.cmdLink(ctx, evt, roomID)
	case "invite":
		if len(parts) < 3 {
			svc.sendReply(ctx, evt, "Usage: `!wc invite <room_id>`")
			return
		}
		svc.cmdInvite(ctx, evt, roomID, sender, parts[2])
	case "leave":
		if len(parts) < 3 {
			svc.sendReply(ctx, evt, "Usage: `!wc leave <room_id>`")
			return
		}
		svc.cmdLeave(ctx, evt, roomID, sender, parts[2])
	case "kick":
		if len(parts) < 3 {
			svc.sendReply(ctx, evt, "Usage: `!wc kick <session_id>`")
			return
		}
		svc.cmdKick(ctx, evt, roomID, sender, parts[2])
	case "activate":
		svc.cmdActivate(ctx, evt, roomID)
	case "config":
		svc.cmdConfig(ctx, evt, roomID, sender)
	case "breakout":
		svc.handleBreakoutCommand(ctx, evt, roomID, sender, parts[2:])
	default:
		svc.sendReply(ctx, evt, cmdHelp)
	}
}

const cmdHelp = `Wally Conference commands:
- ` + "`!wc status`" + ` — show bot status
- ` + "`!wc link`" + ` — generate guest join link
- ` + "`!wc invite <room_id>`" + ` — bot joins a room
- ` + "`!wc leave <room_id>`" + ` — bot leaves a room
- ` + "`!wc kick <session_id>`" + ` — remove a guest
- ` + "`!wc activate`" + ` — re-send welcome message and state event
- ` + "`!wc config`" + ` — show configuration
- ` + "`!wc breakout create <topic>`" + ` — create breakout
- ` + "`!wc breakout list`" + ` — list active breakouts
- ` + "`!wc breakout end <id>`" + ` — end breakout
- ` + "`!wc breakout move <session_id> <breakout_id>`" + ` — move guest`

func (svc *Service) cmdActivate(ctx context.Context, evt *event.Event, roomID id.RoomID) {
	svc.onRoomJoin(ctx, roomID)
}

func (svc *Service) cmdStatus(ctx context.Context, evt *event.Event, roomID id.RoomID) {
	guests, _ := CountAllActiveSessions(svc.DB)
	breakouts, _ := CountAllActiveBreakouts(svc.DB)
	lkConfigured := svc.Config.LiveKitAPIKey != "" && svc.Config.LiveKitAPISecret != ""
	roomGuests, _ := CountActiveSessions(svc.DB, string(roomID))
	roomBreakouts, _ := CountActiveBreakouts(svc.DB, string(roomID))

	msg := fmt.Sprintf(
		"**Wally Conference Status**\n\n"+
			"Global: %d active guests, %d active breakouts\n"+
			"This room: %d guests, %d breakouts\n"+
			"LiveKit configured: %v\n"+
			"Bot user: `%s`",
		guests, breakouts, roomGuests, roomBreakouts,
		lkConfigured, svc.BotUserID,
	)
	svc.sendReply(ctx, evt, msg)
}

func (svc *Service) cmdLink(ctx context.Context, evt *event.Event, roomID id.RoomID) {
	endpoint := svc.Config.PublicURL
	if endpoint == "" {
		svc.sendReply(ctx, evt, "Error: `public_url` is not configured. Set it in config.yaml and restart.")
		return
	}

	guestLink := fmt.Sprintf("%s/%s",
		strings.TrimRight(endpoint, "/"),
		url.PathEscape(string(roomID)),
	)

	msg := fmt.Sprintf(
		"**Join this call as a guest:**\n%s\n\nShare this link — no Matrix account needed.",
		guestLink,
	)
	svc.sendReply(ctx, evt, msg)
}

func (svc *Service) cmdInvite(ctx context.Context, evt *event.Event, roomID id.RoomID, sender id.UserID, targetRoom string) {
	if !svc.isModerator(ctx, roomID, sender) {
		svc.sendReply(ctx, evt, "Permission denied: moderator power level required.")
		return
	}

	if errMsg := ValidateRoomID(targetRoom); errMsg != "" {
		svc.sendReply(ctx, evt, fmt.Sprintf("Invalid room ID: %s", errMsg))
		return
	}

	_, err := svc.Client.JoinRoomByID(ctx, id.RoomID(targetRoom))
	if err != nil {
		svc.sendReply(ctx, evt, fmt.Sprintf("Failed to join room: %v", err))
		return
	}
	svc.sendReply(ctx, evt, fmt.Sprintf("Joined room `%s`.", targetRoom))
	svc.onRoomJoin(ctx, id.RoomID(targetRoom))
}

func (svc *Service) cmdLeave(ctx context.Context, evt *event.Event, roomID id.RoomID, sender id.UserID, targetRoom string) {
	if !svc.isModerator(ctx, roomID, sender) {
		svc.sendReply(ctx, evt, "Permission denied: moderator power level required.")
		return
	}

	if errMsg := ValidateRoomID(targetRoom); errMsg != "" {
		svc.sendReply(ctx, evt, fmt.Sprintf("Invalid room ID: %s", errMsg))
		return
	}

	_, err := svc.Client.LeaveRoom(ctx, id.RoomID(targetRoom))
	if err != nil {
		svc.sendReply(ctx, evt, fmt.Sprintf("Failed to leave room: %v", err))
		return
	}
	svc.sendReply(ctx, evt, fmt.Sprintf("Left room `%s`.", targetRoom))
}

func (svc *Service) cmdKick(ctx context.Context, evt *event.Event, roomID id.RoomID, sender id.UserID, sessionID string) {
	if !svc.isModerator(ctx, roomID, sender) {
		svc.sendReply(ctx, evt, "Permission denied: moderator power level required.")
		return
	}

	session, err := GetSession(svc.DB, sessionID)
	if err != nil || session == nil {
		svc.sendReply(ctx, evt, fmt.Sprintf("Session `%s` not found.", sessionID))
		return
	}

	if err := ClearCallMember(ctx, svc.Client, id.RoomID(session.RoomID), session.StateKey); err != nil {
		log.Printf("Failed to clear call.member for kicked session: %v", err)
	}

	if err := DeleteSession(svc.DB, sessionID); err != nil {
		log.Printf("Failed to delete kicked session: %v", err)
	}

	svc.sendReply(ctx, evt, fmt.Sprintf("Kicked guest `%s` (session `%s`).", session.DisplayName, sessionID))
}

func (svc *Service) cmdConfig(ctx context.Context, evt *event.Event, roomID id.RoomID, sender id.UserID) {
	if !svc.isAdminRoom(string(roomID)) {
		svc.sendReply(ctx, evt, "This command is only available in admin rooms.")
		return
	}
	if !svc.isModerator(ctx, roomID, sender) {
		svc.sendReply(ctx, evt, "Permission denied: moderator power level required.")
		return
	}

	apiKey := "(not set)"
	if svc.Config.LiveKitAPIKey != "" {
		apiKey = svc.Config.LiveKitAPIKey[:4] + "****"
	}
	apiSecret := "(not set)"
	if svc.Config.LiveKitAPISecret != "" {
		apiSecret = "****"
	}
	adminRooms := "(all rooms)"
	if len(svc.Config.AdminRooms) > 0 {
		adminRooms = strings.Join(svc.Config.AdminRooms, ", ")
	}

	msg := fmt.Sprintf(
		"**Wally Conference Configuration**\n\n"+
			"- `livekit_url`: `%s`\n"+
			"- `livekit_api_key`: `%s`\n"+
			"- `livekit_api_secret`: `%s`\n"+
			"- `livekit_service_url`: `%s`\n"+
			"- `guest_token_ttl`: `%d`\n"+
			"- `max_guests_per_room`: `%d`\n"+
			"- `allowed_origins`: `%s`\n"+
			"- `rate_limit_per_minute`: `%d`\n"+
			"- `auto_join_invites`: `%v`\n"+
			"- `admin_rooms`: `%s`\n"+
			"- `ec_base_url`: `%s`\n"+
			"- `cleanup_interval`: `%d`\n"+
			"- `max_breakouts_per_room`: `%d`",
		svc.Config.LiveKitURL, apiKey, apiSecret,
		svc.Config.LiveKitServiceURL, svc.Config.GuestTokenTTL,
		svc.Config.MaxGuestsPerRoom, svc.Config.AllowedOrigins,
		svc.Config.RateLimitPerMinute, svc.Config.AutoJoinInvites,
		adminRooms, svc.Config.ECBaseURL, svc.Config.CleanupInterval,
		svc.Config.MaxBreakoutsPerRoom,
	)
	svc.sendReply(ctx, evt, msg)
}

func (svc *Service) handleBreakoutCommand(ctx context.Context, evt *event.Event, roomID id.RoomID, sender id.UserID, args []string) {
	if len(args) == 0 {
		svc.sendReply(ctx, evt,
			"Breakout commands:\n"+
				"- `!wc breakout create <topic>` — create a breakout room\n"+
				"- `!wc breakout list` — list active breakouts\n"+
				"- `!wc breakout end <id>` — end a breakout room\n"+
				"- `!wc breakout move <session_id> <breakout_id>` — move guest to breakout",
		)
		return
	}

	switch args[0] {
	case "create":
		if len(args) < 2 {
			svc.sendReply(ctx, evt, "Usage: `!wc breakout create <topic>`")
			return
		}
		svc.cmdBreakoutCreate(ctx, evt, roomID, sender, strings.Join(args[1:], " "))
	case "list":
		svc.cmdBreakoutList(ctx, evt, roomID)
	case "end":
		if len(args) < 2 {
			svc.sendReply(ctx, evt, "Usage: `!wc breakout end <id>`")
			return
		}
		svc.cmdBreakoutEnd(ctx, evt, roomID, sender, args[1])
	case "move":
		if len(args) < 3 {
			svc.sendReply(ctx, evt, "Usage: `!wc breakout move <session_id> <breakout_id>`")
			return
		}
		svc.cmdBreakoutMove(ctx, evt, roomID, sender, args[1], args[2])
	default:
		svc.sendReply(ctx, evt, "Unknown breakout subcommand. Use `!wc breakout` for help.")
	}
}

func (svc *Service) cmdBreakoutCreate(ctx context.Context, evt *event.Event, roomID id.RoomID, sender id.UserID, topic string) {
	if !svc.isModerator(ctx, roomID, sender) {
		svc.sendReply(ctx, evt, "Permission denied: moderator power level required.")
		return
	}

	topic = strings.TrimSpace(topic)
	if topic == "" {
		svc.sendReply(ctx, evt, "Usage: `!wc breakout create <topic>`")
		return
	}

	active, _ := CountActiveBreakouts(svc.DB, string(roomID))
	if active >= svc.Config.MaxBreakoutsPerRoom {
		svc.sendReply(ctx, evt, fmt.Sprintf("Breakout capacity reached (%d per room).", svc.Config.MaxBreakoutsPerRoom))
		return
	}

	breakoutID := uuid.New().String()[:8]
	lkAlias := LiveKitBreakoutAlias(string(roomID), breakoutID)

	br := &BreakoutRoom{
		ID:           breakoutID,
		MatrixRoomID: string(roomID),
		Topic:        sql.NullString{String: topic, Valid: true},
		LKAlias:      lkAlias,
		CreatedBy:    string(sender),
		CreatedAt:    time.Now().Unix(),
	}
	if err := CreateBreakoutRoom(svc.DB, br); err != nil {
		log.Printf("Failed to create breakout: %v", err)
		svc.sendReply(ctx, evt, "Failed to create breakout room.")
		return
	}

	svc.sendReply(ctx, evt, fmt.Sprintf(
		"**Breakout room created**\n\n"+
			"ID: `%s`\n"+
			"Topic: %s\n"+
			"LiveKit room: `%s...`\n\n"+
			"Use `!wc breakout move <session_id> %s` to move guests.",
		breakoutID, topic, lkAlias[:16], breakoutID,
	))
}

func (svc *Service) cmdBreakoutList(ctx context.Context, evt *event.Event, roomID id.RoomID) {
	breakouts, err := GetActiveBreakouts(svc.DB, string(roomID))
	if err != nil {
		svc.sendReply(ctx, evt, "Error fetching breakout rooms.")
		return
	}

	if len(breakouts) == 0 {
		svc.sendReply(ctx, evt, "No active breakout rooms in this room.")
		return
	}

	lines := []string{"**Active Breakout Rooms**\n"}
	for _, br := range breakouts {
		topic := "(no topic)"
		if br.Topic.Valid {
			topic = br.Topic.String
		}
		lines = append(lines, fmt.Sprintf("- `%s` — %s (created by `%s`)", br.ID, topic, br.CreatedBy))
	}
	svc.sendReply(ctx, evt, strings.Join(lines, "\n"))
}

func (svc *Service) cmdBreakoutEnd(ctx context.Context, evt *event.Event, roomID id.RoomID, sender id.UserID, breakoutID string) {
	if !svc.isModerator(ctx, roomID, sender) {
		svc.sendReply(ctx, evt, "Permission denied: moderator power level required.")
		return
	}

	breakout, err := GetBreakout(svc.DB, breakoutID)
	if err != nil || breakout == nil {
		svc.sendReply(ctx, evt, fmt.Sprintf("Breakout `%s` not found.", breakoutID))
		return
	}
	if breakout.EndedAt.Valid {
		svc.sendReply(ctx, evt, fmt.Sprintf("Breakout `%s` already ended.", breakoutID))
		return
	}

	// Clear all guest sessions in this breakout
	sessions, _ := GetSessionsForBreakout(svc.DB, breakoutID)
	for _, session := range sessions {
		ClearCallMember(ctx, svc.Client, id.RoomID(session.RoomID), session.StateKey)
		DeleteSession(svc.DB, session.ID)
	}

	EndBreakoutDB(svc.DB, breakoutID)
	svc.sendReply(ctx, evt, fmt.Sprintf("Breakout `%s` ended. Cleaned up %d guest session(s).", breakoutID, len(sessions)))
}

func (svc *Service) cmdBreakoutMove(ctx context.Context, evt *event.Event, roomID id.RoomID, sender id.UserID, sessionID, breakoutID string) {
	if !svc.isModerator(ctx, roomID, sender) {
		svc.sendReply(ctx, evt, "Permission denied: moderator power level required.")
		return
	}

	result, err := svc.moveToBreakout(sessionID, breakoutID)
	if err != nil {
		svc.sendReply(ctx, evt, fmt.Sprintf("Error: %v", err))
		return
	}

	svc.sendReply(ctx, evt, fmt.Sprintf(
		"Moved session `%s` to breakout `%s`.\nNew EC URL: %s",
		sessionID, breakoutID, result["ec_url"],
	))
}

// ── Helpers ──────────────────────────────────────────────

func (svc *Service) isAdminRoom(roomID string) bool {
	if len(svc.Config.AdminRooms) == 0 {
		return true // no restriction
	}
	for _, ar := range svc.Config.AdminRooms {
		if ar == roomID {
			return true
		}
	}
	return false
}

func (svc *Service) isModerator(ctx context.Context, roomID id.RoomID, userID id.UserID) bool {
	var pl event.PowerLevelsEventContent
	err := svc.Client.StateEvent(ctx, roomID, event.StatePowerLevels, "", &pl)
	if err != nil {
		return false
	}
	return pl.GetUserLevel(userID) >= 50
}

func (svc *Service) sendReply(ctx context.Context, evt *event.Event, body string) {
	_, err := svc.Client.SendMessageEvent(ctx, evt.RoomID, event.EventMessage, &event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    body,
	})
	if err != nil {
		log.Printf("Failed to send reply: %v", err)
	}
}
