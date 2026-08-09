package store

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
)

func (s *Store) searchIndexMeta() string {
	var v string
	_ = s.db.QueryRow(`SELECT value FROM meta WHERE key = 'search_index_version'`).Scan(&v)
	return v
}

func (s *Store) setSearchIndexMeta(v string) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO meta(key, value) VALUES('search_index_version', ?)`, v)
	return err
}

// EnsureSearchIndex rebuilds FTS if missing the current normalization rules.
func (s *Store) EnsureSearchIndex() error {
	if s.searchIndexMeta() == searchIndexVersion {
		return nil
	}
	log.Printf("rebuilding search index (version %s) …", searchIndexVersion)
	if err := s.RebuildSearchIndex(); err != nil {
		return err
	}
	log.Printf("search index ready")
	return nil
}

// RebuildSearchIndex refreshes book_search for russian non-deleted books.
func (s *Store) RebuildSearchIndex() error {
	// Invalidate first so a crash mid-rebuild forces another pass on Open.
	_ = s.setSearchIndexMeta("")
	if _, err := s.db.Exec(`DELETE FROM book_search`); err != nil {
		return err
	}

	rows, err := s.db.Query(`
SELECT b.id, b.title, coalesce(s.title, '')
FROM books b
LEFT JOIN series s ON s.id = b.series_id
WHERE b.deleted = 0 AND b.lang = 'ru'`)
	if err != nil {
		return fmt.Errorf("search scan: %w", err)
	}
	defer rows.Close()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	ins, err := tx.Prepare(`INSERT INTO book_search(rowid, title, authors, series) VALUES(?,?,?,?)`)
	if err != nil {
		return err
	}
	defer ins.Close()

	authStmt, err := tx.Prepare(`
SELECT a.last_name, a.first_name, a.middle_name
FROM book_authors ba JOIN authors a ON a.id = ba.author_id
WHERE ba.book_id = ?
ORDER BY a.last_name, a.first_name`)
	if err != nil {
		return err
	}
	defer authStmt.Close()

	n := 0
	for rows.Next() {
		var id int64
		var title, series string
		if err := rows.Scan(&id, &title, &series); err != nil {
			return err
		}
		authors, err := authorSearchBlob(authStmt, id)
		if err != nil {
			return err
		}
		if _, err := ins.Exec(id, foldYo(title), authors, foldYo(series)); err != nil {
			return err
		}
		n++
		if n%50_000 == 0 {
			log.Printf("search index books=%d", n)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if _, err := s.db.Exec(`ANALYZE`); err != nil {
		return err
	}
	return s.setSearchIndexMeta(searchIndexVersion)
}

func authorSearchBlob(stmt *sql.Stmt, bookID int64) (string, error) {
	rows, err := stmt.Query(bookID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var parts []string
	for rows.Next() {
		var last, first, middle string
		if err := rows.Scan(&last, &first, &middle); err != nil {
			return "", err
		}
		if t := authorSearchText(last, first, middle); t != "" {
			parts = append(parts, t)
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return strings.Join(parts, " "), nil
}
