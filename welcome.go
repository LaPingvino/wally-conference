package main

import (
	"context"
	"log"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// wallyConferenceEventType is the custom state event that advertises
// the bot's presence and capabilities in a room.
var wallyConferenceEventType = event.Type{
	Type:  "eu.kiefte.wally.conference",
	Class: event.StateEventType,
}

const welcomeBody = `**Wally Conference** is now active in this room.

Available commands:
- ` + "`!wc status`" + ` — Show active guests and breakout rooms
- ` + "`!wc link`" + ` — Generate a guest join link
- ` + "`!wc breakout create <topic>`" + ` — Create a breakout room
- ` + "`!wc breakout list`" + ` — List active breakouts
- ` + "`!wc breakout end <id>`" + ` — End a breakout
- ` + "`!wc kick <session-id>`" + ` — Remove a guest

Type ` + "`!wc`" + ` for help.`

const welcomeHTML = `<p><strong>Wally Conference</strong> is now active in this room.</p>
<p>Available commands:</p>
<ul>
<li><code>!wc status</code> — Show active guests and breakout rooms</li>
<li><code>!wc link</code> — Generate a guest join link</li>
<li><code>!wc breakout create &lt;topic&gt;</code> — Create a breakout room</li>
<li><code>!wc breakout list</code> — List active breakouts</li>
<li><code>!wc breakout end &lt;id&gt;</code> — End a breakout</li>
<li><code>!wc kick &lt;session-id&gt;</code> — Remove a guest</li>
</ul>
<p>Type <code>!wc</code> for help.</p>`

// onRoomJoin is called after the bot successfully joins a room.
// It sends a welcome message and a capability state event.
func (svc *Service) onRoomJoin(ctx context.Context, roomID id.RoomID) {
	// Send welcome message (formatted with HTML)
	_, err := svc.Client.SendMessageEvent(ctx, roomID, event.EventMessage, &event.MessageEventContent{
		MsgType:       event.MsgText,
		Body:          welcomeBody,
		Format:        event.FormatHTML,
		FormattedBody: welcomeHTML,
	})
	if err != nil {
		log.Printf("Failed to send welcome message to %s: %v", roomID, err)
	}

	// Build feature list
	features := []string{"guest_access", "breakout_rooms"}

	// Determine endpoint URL
	endpoint := svc.Config.PublicURL
	if endpoint == "" {
		// Fall back to listen address (not very useful externally, but non-empty)
		endpoint = "http://localhost" + svc.Config.ListenAddress
	}

	// Send capability state event
	stateContent := map[string]interface{}{
		"version":     "0.1.0",
		"endpoint":    endpoint,
		"features":    features,
		"bot_user_id": svc.BotUserID,
	}

	_, err = svc.Client.SendStateEvent(ctx, roomID, wallyConferenceEventType, "", stateContent)
	if err != nil {
		log.Printf("Failed to send capability state event to %s: %v", roomID, err)
	}
}
