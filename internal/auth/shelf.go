package auth

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const (
	StatusReading = "reading"
	StatusRead    = "read"
	StatusWant    = "want"
)

func ValidShelfStatus(status string) bool {
	switch status {
	case StatusReading, StatusRead, StatusWant:
		return true
	default:
		return false
	}
}

type ShelfItem struct {
	BookID    int64   `json:"bookId"`
	Status    string  `json:"status"`
	UpdatedAt string  `json:"updatedAt"`
	Progress  float64 `json:"progress,omitempty"`
}

type ShelfEntry struct {
	Status   string  `json:"status,omitempty"`
	Progress float64 `json:"progress"`
}

func (a *Auth) ensureShelfSchema() error {
	_, err := a.db.Exec(`
CREATE TABLE IF NOT EXISTS shelf (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  book_id INTEGER NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('reading','read','want')),
  updated_at TEXT NOT NULL,
  PRIMARY KEY (user_id, book_id)
);
CREATE INDEX IF NOT EXISTS idx_shelf_user_status ON shelf(user_id, status, updated_at DESC);

CREATE TABLE IF NOT EXISTS reading_progress (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  book_id INTEGER NOT NULL,
  position REAL NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (user_id, book_id)
);
CREATE INDEX IF NOT EXISTS idx_progress_user ON reading_progress(user_id, updated_at DESC);
`)
	return err
}

func (a *Auth) GetShelfEntry(userID, bookID int64) (*ShelfEntry, error) {
	var e ShelfEntry
	err := a.db.QueryRow(`SELECT status FROM shelf WHERE user_id = ? AND book_id = ?`, userID, bookID).Scan(&e.Status)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	_ = a.db.QueryRow(`SELECT position FROM reading_progress WHERE user_id = ? AND book_id = ?`, userID, bookID).Scan(&e.Progress)
	if e.Status == "" && e.Progress == 0 {
		return &ShelfEntry{}, nil
	}
	return &e, nil
}

func (a *Auth) SetShelfStatus(userID, bookID int64, status string) error {
	if status == "" {
		_, err := a.db.Exec(`DELETE FROM shelf WHERE user_id = ? AND book_id = ?`, userID, bookID)
		return err
	}
	if !ValidShelfStatus(status) {
		return fmt.Errorf("invalid status")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := a.db.Exec(`
INSERT INTO shelf(user_id, book_id, status, updated_at) VALUES(?,?,?,?)
ON CONFLICT(user_id, book_id) DO UPDATE SET status = excluded.status, updated_at = excluded.updated_at`,
		userID, bookID, status, now)
	return err
}

// MarkReading sets status to reading unless the book is already marked read.
func (a *Auth) MarkReading(userID, bookID int64) error {
	var cur string
	err := a.db.QueryRow(`SELECT status FROM shelf WHERE user_id = ? AND book_id = ?`, userID, bookID).Scan(&cur)
	if err == nil && cur == StatusRead {
		return nil
	}
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	return a.SetShelfStatus(userID, bookID, StatusReading)
}

func (a *Auth) SetProgress(userID, bookID int64, position float64) error {
	if position < 0 {
		position = 0
	}
	if position > 1 {
		position = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := a.db.Exec(`
INSERT INTO reading_progress(user_id, book_id, position, updated_at) VALUES(?,?,?,?)
ON CONFLICT(user_id, book_id) DO UPDATE SET position = excluded.position, updated_at = excluded.updated_at`,
		userID, bookID, position, now)
	return err
}

func (a *Auth) GetProgress(userID, bookID int64) (float64, error) {
	var pos float64
	err := a.db.QueryRow(`SELECT position FROM reading_progress WHERE user_id = ? AND book_id = ?`, userID, bookID).Scan(&pos)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return pos, err
}

// ListShelf returns shelf rows for a user, optionally filtered by status.
// When status is empty, returns all. Ordered by updated_at desc.
func (a *Auth) ListShelf(userID int64, status string, limit int) ([]ShelfItem, error) {
	if limit <= 0 {
		limit = 60
	}
	if limit > 200 {
		limit = 200
	}
	status = strings.TrimSpace(status)
	var (
		rows *sql.Rows
		err  error
	)
	if status == "" {
		rows, err = a.db.Query(`
SELECT s.book_id, s.status, s.updated_at, coalesce(p.position, 0)
FROM shelf s
LEFT JOIN reading_progress p ON p.user_id = s.user_id AND p.book_id = s.book_id
WHERE s.user_id = ?
ORDER BY s.updated_at DESC
LIMIT ?`, userID, limit)
	} else {
		if !ValidShelfStatus(status) {
			return nil, fmt.Errorf("invalid status")
		}
		rows, err = a.db.Query(`
SELECT s.book_id, s.status, s.updated_at, coalesce(p.position, 0)
FROM shelf s
LEFT JOIN reading_progress p ON p.user_id = s.user_id AND p.book_id = s.book_id
WHERE s.user_id = ? AND s.status = ?
ORDER BY s.updated_at DESC
LIMIT ?`, userID, status, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ShelfItem
	for rows.Next() {
		var it ShelfItem
		if err := rows.Scan(&it.BookID, &it.Status, &it.UpdatedAt, &it.Progress); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// ListContinue returns books currently being read (shelf status reading), newest first.
func (a *Auth) ListContinue(userID int64, limit int) ([]ShelfItem, error) {
	if limit <= 0 {
		limit = 6
	}
	return a.ListShelf(userID, StatusReading, limit)
}
