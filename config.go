package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds the YAML configuration for the service.
type Config struct {
	// Matrix homeserver URL
	Homeserver string `yaml:"homeserver"`
	// Matrix bot user ID (e.g. @wally-conference:yourserver.com)
	UserID string `yaml:"user_id"`
	// Matrix access token (mutually exclusive with password)
	AccessToken string `yaml:"access_token"`
	// Matrix password (used if access_token is empty)
	Password string `yaml:"password"`

	// HTTP listen address (e.g. ":8080" or "127.0.0.1:8080")
	ListenAddress string `yaml:"listen_address"`

	// Externally reachable URL for the bot's HTTP API
	// Used in state events and guest join links
	PublicURL string `yaml:"public_url"`

	// SQLite database path
	Database string `yaml:"database"`

	// LiveKit
	LiveKitURL        string `yaml:"livekit_url"`
	LiveKitAPIKey     string `yaml:"livekit_api_key"`
	LiveKitAPISecret  string `yaml:"livekit_api_secret"`
	LiveKitServiceURL string `yaml:"livekit_service_url"`

	// LiveKit room alias mode: "raw" (use Matrix room ID directly, older lk-jwt-service)
	// or "hash" (SHA256 hash per MSC4195, newer lk-jwt-service)
	LiveKitRoomAliasMode string `yaml:"livekit_room_alias_mode"`

	// Guest settings
	GuestTokenTTL     int `yaml:"guest_token_ttl"`      // seconds
	MaxGuestsPerRoom  int `yaml:"max_guests_per_room"`
	RequireActiveCall bool `yaml:"require_active_call"`

	// Security
	AllowedOrigins     string `yaml:"allowed_origins"`
	RateLimitPerMinute int    `yaml:"rate_limit_per_minute"`

	// Bot behavior
	AutoJoinInvites bool     `yaml:"auto_join_invites"`
	AdminRooms      []string `yaml:"admin_rooms"`

	// Element Call URL
	ECBaseURL string `yaml:"ec_base_url"`

	// Cleanup
	CleanupInterval int `yaml:"cleanup_interval"` // seconds

	// Breakout rooms
	MaxBreakoutsPerRoom int `yaml:"max_breakouts_per_room"`
}

// LoadConfig reads and parses a YAML config file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &Config{
		// Defaults
		ListenAddress:      ":8080",
		Database:           "/var/lib/wally-conference/wally-conference.db",
		GuestTokenTTL:      7200,
		MaxGuestsPerRoom:   20,
		RequireActiveCall:  true,
		LiveKitRoomAliasMode: "raw",
		AllowedOrigins:     "*",
		RateLimitPerMinute: 5,
		AutoJoinInvites:    true,
		CleanupInterval:    300,
		MaxBreakoutsPerRoom: 10,
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Validate required fields
	if cfg.Homeserver == "" {
		return nil, fmt.Errorf("homeserver is required")
	}
	if cfg.UserID == "" {
		return nil, fmt.Errorf("user_id is required")
	}
	if cfg.AccessToken == "" && cfg.Password == "" {
		return nil, fmt.Errorf("either access_token or password is required")
	}
	if cfg.LiveKitAPIKey == "" {
		return nil, fmt.Errorf("livekit_api_key is required")
	}
	if cfg.LiveKitAPISecret == "" {
		return nil, fmt.Errorf("livekit_api_secret is required")
	}

	return cfg, nil
}
