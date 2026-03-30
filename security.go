package main

import (
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
)

// roomIDRe matches valid Matrix room IDs: !<localpart>:<server>
var roomIDRe = regexp.MustCompile(`^![A-Za-z0-9._=\-/]+:[A-Za-z0-9.\-]+(:[0-9]+)?$`)

// RateLimiter implements a sliding-window per-key rate limiter (in-memory).
type RateLimiter struct {
	maxPerMinute int
	mu           sync.Mutex
	windows      map[string][]time.Time
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(maxPerMinute int) *RateLimiter {
	return &RateLimiter{
		maxPerMinute: maxPerMinute,
		windows:      make(map[string][]time.Time),
	}
}

// Check returns true if the request is allowed, false if rate-limited.
// If allowed, the current timestamp is recorded.
func (rl *RateLimiter) Check(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-time.Minute)

	// Filter to recent timestamps
	var recent []time.Time
	for _, t := range rl.windows[key] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}

	if len(recent) >= rl.maxPerMinute {
		rl.windows[key] = recent
		return false
	}

	rl.windows[key] = append(recent, now)
	return true
}

// Cleanup removes stale keys to prevent unbounded memory growth.
func (rl *RateLimiter) Cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-time.Minute)
	for k, times := range rl.windows {
		hasRecent := false
		for _, t := range times {
			if t.After(cutoff) {
				hasRecent = true
				break
			}
		}
		if !hasRecent {
			delete(rl.windows, k)
		}
	}
}

// ValidateRoomID validates a Matrix room ID. Returns an error string if invalid.
func ValidateRoomID(roomID string) string {
	if roomID == "" {
		return "room_id is required"
	}
	if !roomIDRe.MatchString(roomID) {
		return "Invalid room_id format"
	}
	return ""
}

// ValidateDisplayName validates and sanitizes a guest display name.
// Returns (sanitizedName, errorMessage). If errorMessage is non-empty, name was rejected.
func ValidateDisplayName(name string) (string, string) {
	if name == "" {
		return "", "display_name is required"
	}

	// Strip control characters
	var b strings.Builder
	for _, r := range name {
		if !unicode.IsControl(r) {
			b.WriteRune(r)
		}
	}
	sanitized := strings.TrimSpace(b.String())

	if sanitized == "" {
		return "", "display_name must contain visible characters"
	}
	if len(sanitized) > 50 {
		return "", "display_name must be 50 characters or fewer"
	}

	return sanitized, ""
}

// matchOrigin checks if the request Origin is allowed and returns it,
// or returns the allowedOrigins string if it's "*" or a single match.
func matchOrigin(r *http.Request, allowedOrigins string) string {
	if allowedOrigins == "*" {
		return "*"
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return allowedOrigins
	}
	for _, allowed := range strings.Split(allowedOrigins, ",") {
		if strings.TrimSpace(allowed) == origin {
			return origin
		}
	}
	// Default to first origin if no match (backwards compat)
	return strings.TrimSpace(strings.Split(allowedOrigins, ",")[0])
}

// AddCORSHeaders adds CORS headers to a response.
// Kept for backwards compat — callers without *http.Request.
func AddCORSHeaders(w http.ResponseWriter, allowedOrigins string) {
	w.Header().Set("Access-Control-Allow-Origin", allowedOrigins)
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

// SetCORS adds CORS headers, dynamically matching the request Origin
// against a comma-separated allowedOrigins list (or "*").
func SetCORS(w http.ResponseWriter, r *http.Request, allowedOrigins string) {
	w.Header().Set("Access-Control-Allow-Origin", matchOrigin(r, allowedOrigins))
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

// CORSPreflight writes a 204 CORS preflight response.
func CORSPreflight(w http.ResponseWriter, allowedOrigins string) {
	w.Header().Set("Access-Control-Allow-Origin", allowedOrigins)
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.WriteHeader(http.StatusNoContent)
}
