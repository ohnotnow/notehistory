package notehistory

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const timeLayout = "2006-01-02 15:04:05"

// Store is the history database.
type Store struct {
	db *sql.DB
}

// OpenStore opens (creating if needed) the history database at path.
func OpenStore(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	for _, p := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
	} {
		if _, err := db.ExecContext(ctx, p); err != nil {
			db.Close()
			return nil, err
		}
	}
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

type migration struct {
	version     int
	description string
	apply       func(ctx context.Context, tx *sql.Tx) error
}

// The baseline uses IF NOT EXISTS so a history file written by the Python
// script is adopted as-is.
var migrations = []migration{
	{1, "baseline schema", func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS notifications (
			uuid TEXT PRIMARY KEY,
			delivered_at TEXT,
			app TEXT,
			title TEXT,
			subtitle TEXT,
			body TEXT,
			recorded_at TEXT
		)`)
		return err
	}},
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		version INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO schema_version (id, version) VALUES (1, 0)`); err != nil {
		return err
	}
	var current int
	if err := s.db.QueryRowContext(ctx, `SELECT version FROM schema_version WHERE id = 1`).Scan(&current); err != nil {
		return err
	}
	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if err := m.apply(ctx, tx); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d (%s): %w", m.version, m.description, err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE schema_version SET version = ? WHERE id = 1`, m.version); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Record inserts any notifications not already present and returns those it
// added, in the order given.
func (s *Store) Record(ctx context.Context, rows []Notification) ([]Notification, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := time.Now().Format(timeLayout)
	var added []Notification
	for _, n := range rows {
		res, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO notifications (uuid, delivered_at, app, title, subtitle, body, recorded_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			n.UUID, n.DeliveredAt, n.App, n.Title, n.Subtitle, n.Body, now)
		if err != nil {
			return nil, err
		}
		if c, _ := res.RowsAffected(); c > 0 {
			n.RecordedAt = now
			added = append(added, n)
		}
	}
	return added, tx.Commit()
}

// Recent returns the most recent limit notifications, newest first.
func (s *Store) Recent(ctx context.Context, limit int) ([]Notification, error) {
	return s.query(ctx, `SELECT uuid, delivered_at, app, title, subtitle, body, recorded_at
		FROM notifications ORDER BY delivered_at DESC LIMIT ?`, limit)
}

func (s *Store) query(ctx context.Context, q string, args ...any) ([]Notification, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Notification
	for rows.Next() {
		var n Notification
		var delivered, app, title, subtitle, body, recorded sql.NullString
		if err := rows.Scan(&n.UUID, &delivered, &app, &title, &subtitle, &body, &recorded); err != nil {
			return nil, err
		}
		n.DeliveredAt, n.App, n.Title = delivered.String, app.String, title.String
		n.Subtitle, n.Body, n.RecordedAt = subtitle.String, body.String, recorded.String
		out = append(out, n)
	}
	return out, rows.Err()
}

// All returns every recorded notification, newest first.
func (s *Store) All(ctx context.Context) ([]Notification, error) {
	return s.query(ctx, `SELECT uuid, delivered_at, app, title, subtitle, body, recorded_at
		FROM notifications ORDER BY delivered_at DESC`)
}

// Prune deletes notifications delivered before cutoff and returns how many
// went. Rows with no delivered time are kept.
func (s *Store) Prune(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM notifications WHERE delivered_at != '' AND delivered_at < ?`,
		cutoff.Format(timeLayout))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
