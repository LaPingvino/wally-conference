package main

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

// GuestSession represents a row in the guest_session table.
type GuestSession struct {
	ID          string
	RoomID      string
	BotUserID   string
	DeviceID    string
	DisplayName string
	LKIdentity  string
	LKRoom      string
	BreakoutID  sql.NullString
	CreatedAt   int64
	ExpiresAt   int64
	StateKey    string
}

// BreakoutRoom represents a row in the breakout_room table.
type BreakoutRoom struct {
	ID            string
	MatrixRoomID  string
	Topic         sql.NullString
	LKAlias       string
	CreatedBy     string
	CreatedAt     int64
	EndedAt       sql.NullInt64
}

// OpenDB opens a SQLite database at the given path.
func OpenDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// Enable WAL mode for better concurrency
	_, err = db.Exec("PRAGMA journal_mode=WAL")
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// MigrateDB creates the database schema if it doesn't exist.
func MigrateDB(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS guest_session (
			id           TEXT PRIMARY KEY,
			room_id      TEXT NOT NULL,
			bot_user_id  TEXT NOT NULL,
			device_id    TEXT NOT NULL,
			display_name TEXT NOT NULL,
			lk_identity  TEXT NOT NULL,
			lk_room      TEXT NOT NULL,
			breakout_id  TEXT,
			created_at   INTEGER NOT NULL,
			expires_at   INTEGER NOT NULL,
			state_key    TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS breakout_room (
			id              TEXT PRIMARY KEY,
			matrix_room_id  TEXT NOT NULL,
			topic           TEXT,
			lk_alias        TEXT NOT NULL,
			created_by      TEXT NOT NULL,
			created_at      INTEGER NOT NULL,
			ended_at        INTEGER
		);
	`)
	return err
}

// CreateSession inserts a new guest session row.
func CreateSession(db *sql.DB, s *GuestSession) error {
	_, err := db.Exec(`
		INSERT INTO guest_session
			(id, room_id, bot_user_id, device_id, display_name,
			 lk_identity, lk_room, breakout_id, created_at, expires_at, state_key)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.RoomID, s.BotUserID, s.DeviceID, s.DisplayName,
		s.LKIdentity, s.LKRoom, s.BreakoutID, s.CreatedAt, s.ExpiresAt, s.StateKey,
	)
	return err
}

// GetSessionByIdentity looks up a guest session by its LiveKit identity hash.
func GetSessionByIdentity(db *sql.DB, lkIdentity string) (*GuestSession, error) {
	s := &GuestSession{}
	err := db.QueryRow(
		"SELECT id, room_id, bot_user_id, device_id, display_name, lk_identity, lk_room, breakout_id, created_at, expires_at, state_key FROM guest_session WHERE lk_identity = ?",
		lkIdentity,
	).Scan(&s.ID, &s.RoomID, &s.BotUserID, &s.DeviceID, &s.DisplayName, &s.LKIdentity, &s.LKRoom, &s.BreakoutID, &s.CreatedAt, &s.ExpiresAt, &s.StateKey)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// GetSessionByStateKey looks up a guest session by its call.member state key.
func GetSessionByStateKey(db *sql.DB, stateKey string) (*GuestSession, error) {
	s := &GuestSession{}
	err := db.QueryRow(
		"SELECT id, room_id, bot_user_id, device_id, display_name, lk_identity, lk_room, breakout_id, created_at, expires_at, state_key FROM guest_session WHERE state_key = ?",
		stateKey,
	).Scan(&s.ID, &s.RoomID, &s.BotUserID, &s.DeviceID, &s.DisplayName, &s.LKIdentity, &s.LKRoom, &s.BreakoutID, &s.CreatedAt, &s.ExpiresAt, &s.StateKey)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// GetSession looks up a guest session by its primary key.
func GetSession(db *sql.DB, sessionID string) (*GuestSession, error) {
	s := &GuestSession{}
	err := db.QueryRow(
		"SELECT id, room_id, bot_user_id, device_id, display_name, lk_identity, lk_room, breakout_id, created_at, expires_at, state_key FROM guest_session WHERE id = ?",
		sessionID,
	).Scan(&s.ID, &s.RoomID, &s.BotUserID, &s.DeviceID, &s.DisplayName, &s.LKIdentity, &s.LKRoom, &s.BreakoutID, &s.CreatedAt, &s.ExpiresAt, &s.StateKey)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// DeleteSession removes a guest session.
func DeleteSession(db *sql.DB, sessionID string) error {
	_, err := db.Exec("DELETE FROM guest_session WHERE id = ?", sessionID)
	return err
}

// CountActiveSessions counts active (non-expired) guest sessions in a room.
func CountActiveSessions(db *sql.DB, roomID string) (int, error) {
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM guest_session WHERE room_id = ? AND expires_at > ?",
		roomID, time.Now().Unix(),
	).Scan(&count)
	return count, err
}

// CountAllActiveSessions counts all active (non-expired) guest sessions across all rooms.
func CountAllActiveSessions(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM guest_session WHERE expires_at > ?",
		time.Now().Unix(),
	).Scan(&count)
	return count, err
}

// GetSessionsByLKRoom returns all guest sessions for a given LiveKit room alias.
func GetSessionsByLKRoom(db *sql.DB, lkRoom string) ([]*GuestSession, error) {
	rows, err := db.Query(
		"SELECT id, room_id, bot_user_id, device_id, display_name, lk_identity, lk_room, breakout_id, created_at, expires_at, state_key FROM guest_session WHERE lk_room = ?",
		lkRoom,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessions(rows)
}

// GetExpiredSessions returns all sessions whose expiry has passed.
func GetExpiredSessions(db *sql.DB) ([]*GuestSession, error) {
	rows, err := db.Query(
		"SELECT id, room_id, bot_user_id, device_id, display_name, lk_identity, lk_room, breakout_id, created_at, expires_at, state_key FROM guest_session WHERE expires_at <= ?",
		time.Now().Unix(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessions(rows)
}

// GetAllSessionsInRoom returns all active guest sessions for a given Matrix room ID.
func GetAllSessionsInRoom(db *sql.DB, roomID string) ([]*GuestSession, error) {
	rows, err := db.Query(
		"SELECT id, room_id, bot_user_id, device_id, display_name, lk_identity, lk_room, breakout_id, created_at, expires_at, state_key FROM guest_session WHERE room_id = ? AND expires_at > ?",
		roomID, time.Now().Unix(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessions(rows)
}

// ── Breakout helpers ────────────────────────────────────

// CreateBreakoutRoom inserts a new breakout room row.
func CreateBreakoutRoom(db *sql.DB, br *BreakoutRoom) error {
	_, err := db.Exec(`
		INSERT INTO breakout_room
			(id, matrix_room_id, topic, lk_alias, created_by, created_at, ended_at)
		VALUES (?, ?, ?, ?, ?, ?, NULL)`,
		br.ID, br.MatrixRoomID, br.Topic, br.LKAlias, br.CreatedBy, br.CreatedAt,
	)
	return err
}

// GetBreakout looks up a breakout room by ID.
func GetBreakout(db *sql.DB, breakoutID string) (*BreakoutRoom, error) {
	br := &BreakoutRoom{}
	err := db.QueryRow(
		"SELECT id, matrix_room_id, topic, lk_alias, created_by, created_at, ended_at FROM breakout_room WHERE id = ?",
		breakoutID,
	).Scan(&br.ID, &br.MatrixRoomID, &br.Topic, &br.LKAlias, &br.CreatedBy, &br.CreatedAt, &br.EndedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return br, err
}

// CountActiveBreakouts counts active (non-ended) breakout rooms for a parent room.
func CountActiveBreakouts(db *sql.DB, matrixRoomID string) (int, error) {
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM breakout_room WHERE matrix_room_id = ? AND ended_at IS NULL",
		matrixRoomID,
	).Scan(&count)
	return count, err
}

// CountAllActiveBreakouts counts all active (non-ended) breakout rooms.
func CountAllActiveBreakouts(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM breakout_room WHERE ended_at IS NULL",
	).Scan(&count)
	return count, err
}

// GetActiveBreakouts returns all active breakout rooms for a parent room.
func GetActiveBreakouts(db *sql.DB, matrixRoomID string) ([]*BreakoutRoom, error) {
	rows, err := db.Query(
		"SELECT id, matrix_room_id, topic, lk_alias, created_by, created_at, ended_at FROM breakout_room WHERE matrix_room_id = ? AND ended_at IS NULL",
		matrixRoomID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*BreakoutRoom
	for rows.Next() {
		br := &BreakoutRoom{}
		if err := rows.Scan(&br.ID, &br.MatrixRoomID, &br.Topic, &br.LKAlias, &br.CreatedBy, &br.CreatedAt, &br.EndedAt); err != nil {
			return nil, err
		}
		result = append(result, br)
	}
	return result, rows.Err()
}

// EndBreakoutDB marks a breakout room as ended.
func EndBreakoutDB(db *sql.DB, breakoutID string) error {
	_, err := db.Exec(
		"UPDATE breakout_room SET ended_at = ? WHERE id = ?",
		time.Now().Unix(), breakoutID,
	)
	return err
}

// GetSessionsForBreakout returns all guest sessions in a given breakout room.
func GetSessionsForBreakout(db *sql.DB, breakoutID string) ([]*GuestSession, error) {
	rows, err := db.Query(
		"SELECT id, room_id, bot_user_id, device_id, display_name, lk_identity, lk_room, breakout_id, created_at, expires_at, state_key FROM guest_session WHERE breakout_id = ?",
		breakoutID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessions(rows)
}

// UpdateSessionBreakout moves a session to a breakout room.
func UpdateSessionBreakout(db *sql.DB, sessionID, breakoutID, lkRoom string, expiresAt int64) error {
	_, err := db.Exec(
		"UPDATE guest_session SET breakout_id = ?, lk_room = ?, expires_at = ? WHERE id = ?",
		breakoutID, lkRoom, expiresAt, sessionID,
	)
	return err
}

// scanSessions scans multiple guest session rows.
func scanSessions(rows *sql.Rows) ([]*GuestSession, error) {
	var result []*GuestSession
	for rows.Next() {
		s := &GuestSession{}
		if err := rows.Scan(&s.ID, &s.RoomID, &s.BotUserID, &s.DeviceID, &s.DisplayName, &s.LKIdentity, &s.LKRoom, &s.BreakoutID, &s.CreatedAt, &s.ExpiresAt, &s.StateKey); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}
