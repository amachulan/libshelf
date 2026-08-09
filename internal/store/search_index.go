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

func (s *Store) searchLangsMeta() string {
	var v string
	_ = s.db.QueryRow(`SELECT value FROM meta WHERE key = 'search_index_langs'`).Scan(&v)
	return v
}

func (s *Store) setSearchLangsMeta(v string) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO meta(key, value) VALUES('search_index_langs', ?)`, v)
	return err
}

// EnsureSearchIndex rebuilds FTS if missing the current normalization rules or language filter.
func (s *Store) EnsureSearchIndex() error {
	if s.searchIndexMeta() == searchIndexVersion && s.searchLangsMeta() == s.langsKey() {
		return nil
	}
	log.Printf("rebuilding search index (version %s, langs=%s) …", searchIndexVersion, s.langsKey())
	if err := s.RebuildSearchIndex(); err != nil {
		return err
	}
	log.Printf("search index ready")
	return nil
}

type searchBookRow struct {
	id            int64
	title, series string
}

// RebuildSearchIndex refreshes book_search for visible non-deleted books.
// Books are loaded first, then written — SQLite is limited to one connection,
// so overlapping Query+Begin deadlocks. Contentless FTS5 cannot DELETE rows,
// so the table is dropped and recreated.
func (s *Store) RebuildSearchIndex() error {
	books, err := s.loadSearchBooks()
	if err != nil {
		return err
	}

	if err := s.resetSearchTable(); err != nil {
		return err
	}

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

	for i, b := range books {
		authors, err := authorSearchBlob(authStmt, b.id)
		if err != nil {
			return err
		}
		if _, err := ins.Exec(b.id, foldYo(b.title), authors, foldYo(b.series)); err != nil {
			return err
		}
		if (i+1)%50_000 == 0 {
			log.Printf("search index books=%d/%d", i+1, len(books))
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	if _, err := s.db.Exec(`ANALYZE`); err != nil {
		return err
	}
	if err := s.setSearchIndexMeta(searchIndexVersion); err != nil {
		return err
	}
	return s.setSearchLangsMeta(s.langsKey())
}

func (s *Store) resetSearchTable() error {
	if _, err := s.db.Exec(`DROP TABLE IF EXISTS book_search`); err != nil {
		return fmt.Errorf("drop book_search: %w", err)
	}
	if _, err := s.db.Exec(bookSearchDDL); err != nil {
		return fmt.Errorf("create book_search: %w", err)
	}
	return nil
}

func (s *Store) loadSearchBooks() ([]searchBookRow, error) {
	lang, langArgs := s.langPred("b.lang")
	rows, err := s.db.Query(`
SELECT b.id, b.title, coalesce(s.title, '')
FROM books b
LEFT JOIN series s ON s.id = b.series_id
WHERE b.deleted = 0`+lang, langArgs...)
	if err != nil {
		return nil, fmt.Errorf("search scan: %w", err)
	}
	defer rows.Close()

	books := make([]searchBookRow, 0, 1024)
	for rows.Next() {
		var b searchBookRow
		if err := rows.Scan(&b.id, &b.title, &b.series); err != nil {
			return nil, err
		}
		books = append(books, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return books, nil
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
