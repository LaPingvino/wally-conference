// Wally Conference — standalone Go service for guest access & breakout rooms
// in Matrix voice/video calls powered by Element Call and LiveKit.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: wally-conference <config.yaml>")
		os.Exit(1)
	}

	cfg, err := LoadConfig(os.Args[1])
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Open SQLite database
	db, err := OpenDB(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if err := MigrateDB(db); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	// Create Matrix client
	client, err := mautrix.NewClient(cfg.Homeserver, id.UserID(cfg.UserID), cfg.AccessToken)
	if err != nil {
		log.Fatalf("Failed to create Matrix client: %v", err)
	}

	// If password is set and no access token, do login
	if cfg.AccessToken == "" && cfg.Password != "" {
		resp, err := client.Login(context.Background(), &mautrix.ReqLogin{
			Type: mautrix.AuthTypePassword,
			Identifier: mautrix.UserIdentifier{
				Type: mautrix.IdentifierTypeUser,
				User: cfg.UserID,
			},
			Password: cfg.Password,
		})
		if err != nil {
			log.Fatalf("Failed to login: %v", err)
		}
		client.AccessToken = resp.AccessToken
		log.Printf("Logged in as %s", resp.UserID)
	}

	// Verify whoami
	whoami, err := client.Whoami(context.Background())
	if err != nil {
		log.Fatalf("Failed to verify credentials (whoami): %v", err)
	}
	log.Printf("Authenticated as %s", whoami.UserID)
	botUserID := whoami.UserID.String()

	// Create the bot service
	svc := &Service{
		Config:    cfg,
		DB:        db,
		Client:    client,
		BotUserID: botUserID,
		Limiter:   NewRateLimiter(cfg.RateLimitPerMinute),
	}

	// Register Matrix event handler for bot commands
	syncer := client.Syncer.(*mautrix.DefaultSyncer)
	syncer.OnEventType(event.EventMessage, func(ctx context.Context, evt *event.Event) {
		svc.HandleMatrixMessage(ctx, evt)
	})

	// Auto-join on invite
	if cfg.AutoJoinInvites {
		syncer.OnEventType(event.StateMember, func(ctx context.Context, evt *event.Event) {
			if evt.GetStateKey() == botUserID {
				content := evt.Content.AsMember()
				if content.Membership == event.MembershipInvite {
					_, err := client.JoinRoomByID(ctx, evt.RoomID)
					if err != nil {
						log.Printf("Failed to auto-join room %s: %v", evt.RoomID, err)
					} else {
						log.Printf("Auto-joined room %s", evt.RoomID)
						svc.onRoomJoin(ctx, evt.RoomID)
					}
				}
			}
		})
	}

	// Set up HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("GET /guest/{roomID}", svc.HandleGuestPage)
	mux.HandleFunc("POST /guest/join", svc.HandleJoin)
	mux.HandleFunc("OPTIONS /guest/join", svc.HandleCORSPreflight)
	mux.HandleFunc("POST /guest/breakout/create", svc.HandleBreakoutCreate)
	mux.HandleFunc("OPTIONS /guest/breakout/create", svc.HandleCORSPreflight)
	mux.HandleFunc("POST /guest/breakout/move", svc.HandleBreakoutMove)
	mux.HandleFunc("OPTIONS /guest/breakout/move", svc.HandleCORSPreflight)
	// Keep legacy /join for backwards compat
	mux.HandleFunc("POST /join", svc.HandleJoin)
	mux.HandleFunc("OPTIONS /join", svc.HandleCORSPreflight)
	mux.HandleFunc("POST /webhook", svc.HandleWebhook)
	mux.HandleFunc("GET /health", svc.HandleHealth)

	httpServer := &http.Server{
		Addr:    cfg.ListenAddress,
		Handler: mux,
	}

	// Start background cleanup goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go CleanupLoop(ctx, svc)

	// Start Matrix sync in background
	go func() {
		log.Println("Starting Matrix sync...")
		if err := client.SyncWithContext(ctx); err != nil && ctx.Err() == nil {
			log.Printf("Matrix sync error: %v", err)
		}
	}()

	// Start HTTP server in background
	go func() {
		log.Printf("HTTP server listening on %s", cfg.ListenAddress)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	httpServer.Shutdown(shutdownCtx)
	client.StopSync()

	log.Println("Shutdown complete")
}

// Service holds all shared state for the bot.
type Service struct {
	Config    *Config
	DB        *sql.DB
	Client    *mautrix.Client
	BotUserID string
	Limiter   *RateLimiter
}
