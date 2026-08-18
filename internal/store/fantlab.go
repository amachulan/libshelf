package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const (
	FantLabOK        = "ok"
	FantLabNone      = "none"
	FantLabAmbiguous = "ambiguous"
)

type FantLabRating struct {
	WorkKey      string
	Status       string
	FantLabID    int64
	Rating       float64
	Voters       int
	MatchedTitle string
	FetchedAt    string
}

type FantLabWork struct {
	Key         string
	Title       string
	AuthorLasts []string
	Year        int
}

type authorMeta struct {
	IDs   []int64
	Lasts []string
}

func (s *Store) UpsertFantLab(r FantLabRating) error {
	if r.WorkKey == "" {
		return fmt.Errorf("empty work key")
	}
	if r.FetchedAt == "" {
		r.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.db.Exec(`
INSERT INTO fantlab_ratings(work_key, status, fantlab_id, rating, voters, matched_title, fetched_at)
VALUES(?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(work_key) DO UPDATE SET
  status=excluded.status,
  fantlab_id=excluded.fantlab_id,
  rating=excluded.rating,
  voters=excluded.voters,
  matched_title=excluded.matched_title,
  fetched_at=excluded.fetched_at`,
		r.WorkKey, r.Status, r.FantLabID, r.Rating, r.Voters, r.MatchedTitle, r.FetchedAt)
	return err
}

func (s *Store) ClearFailedFantLab() (int64, error) {
	return s.ClearFantLabStatus(FantLabNone, FantLabAmbiguous)
}

func (s *Store) ClearFantLabStatus(statuses ...string) (int64, error) {
	if len(statuses) == 0 {
		return 0, nil
	}
	ph := make([]string, len(statuses))
	args := make([]any, len(statuses))
	for i, st := range statuses {
		ph[i] = "?"
		args[i] = st
	}
	res, err := s.db.Exec(`DELETE FROM fantlab_ratings WHERE status IN (`+strings.Join(ph, ",")+`)`, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) FantLabKnownKeys() (map[string]struct{}, error) {
	rows, err := s.db.Query(`SELECT work_key FROM fantlab_ratings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]struct{}, 1024)
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out[k] = struct{}{}
	}
	return out, rows.Err()
}

func (s *Store) FantLabByKeys(keys []string) (map[string]FantLabRating, error) {
	out := make(map[string]FantLabRating, len(keys))
	if len(keys) == 0 {
		return out, nil
	}
	for start := 0; start < len(keys); start += sqliteMaxVars {
		end := start + sqliteMaxVars
		if end > len(keys) {
			end = len(keys)
		}
		chunk := keys[start:end]
		ph := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, k := range chunk {
			ph[i] = "?"
			args[i] = k
		}
		rows, err := s.db.Query(`
SELECT work_key, status, fantlab_id, rating, voters, matched_title, fetched_at
FROM fantlab_ratings
WHERE work_key IN (`+strings.Join(ph, ",")+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var r FantLabRating
			if err := rows.Scan(&r.WorkKey, &r.Status, &r.FantLabID, &r.Rating, &r.Voters, &r.MatchedTitle, &r.FetchedAt); err != nil {
				rows.Close()
				return nil, err
			}
			out[r.WorkKey] = r
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) AttachFantLab(books []Book) error {
	if len(books) == 0 {
		return nil
	}
	ids := make([]int64, len(books))
	for i := range books {
		ids[i] = books[i].ID
	}
	meta, err := s.authorMetaForBooks(ids)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(books))
	seen := make(map[string]struct{}, len(books))
	bookKey := make([]string, len(books))
	for i, b := range books {
		key := workKey(b.Title, meta[b.ID].IDs)
		bookKey[i] = key
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
	}
	ratings, err := s.FantLabByKeys(keys)
	if err != nil {
		return err
	}
	for i := range books {
		r, ok := ratings[bookKey[i]]
		if !ok || r.Status != FantLabOK || r.Voters <= 0 || r.Rating <= 0 {
			continue
		}
		books[i].FantLabRate = r.Rating
		books[i].FantLabVoters = r.Voters
	}
	return nil
}

func (s *Store) PendingFantLabWorks(genreCode string, limit int) ([]FantLabWork, error) {
	known, err := s.FantLabKnownKeys()
	if err != nil {
		return nil, err
	}
	lang, langArgs := s.langPred("b.lang")
	var rows *sql.Rows
	if genreCode != "" {
		var genreID int64
		err = s.db.QueryRow(`SELECT id FROM genres WHERE code = ?`, genreCode).Scan(&genreID)
		if err != nil {
			return nil, err
		}
		args := append([]any{genreID}, langArgs...)
		rows, err = s.db.Query(`
SELECT b.id, b.title
FROM books b
JOIN book_genres bg ON bg.book_id = b.id AND bg.genre_id = ?
WHERE b.deleted = 0`+lang, args...)
	} else {
		rows, err = s.db.Query(`SELECT b.id, b.title FROM books b WHERE b.deleted = 0`+lang, langArgs...)
	}
	if err != nil {
		return nil, err
	}
	slims, err := scanSlimBooks(rows)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(slims))
	for i, b := range slims {
		ids[i] = b.id
	}
	meta, err := s.authorMetaForBooks(ids)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(slims))
	out := make([]FantLabWork, 0, 256)
	for _, b := range slims {
		m := meta[b.id]
		key := workKey(b.title, m.IDs)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if _, ok := known[key]; ok {
			continue
		}
		out = append(out, FantLabWork{Key: key, Title: b.title, AuthorLasts: m.Lasts})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

type slimBook struct {
	id    int64
	title string
}

func scanSlimBooks(rows *sql.Rows) ([]slimBook, error) {
	defer rows.Close()
	var out []slimBook
	for rows.Next() {
		var b slimBook
		if err := rows.Scan(&b.id, &b.title); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) authorMetaForBooks(bookIDs []int64) (map[int64]authorMeta, error) {
	out := make(map[int64]authorMeta, len(bookIDs))
	if len(bookIDs) == 0 {
		return out, nil
	}
	for start := 0; start < len(bookIDs); start += sqliteMaxVars {
		end := start + sqliteMaxVars
		if end > len(bookIDs) {
			end = len(bookIDs)
		}
		chunk := bookIDs[start:end]
		ph := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, id := range chunk {
			ph[i] = "?"
			args[i] = id
		}
		rows, err := s.db.Query(`
SELECT ba.book_id, a.id, a.last_name
FROM book_authors ba
JOIN authors a ON a.id = ba.author_id
WHERE ba.book_id IN (`+strings.Join(ph, ",")+`)
ORDER BY ba.book_id, a.id`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var bookID, authorID int64
			var last string
			if err := rows.Scan(&bookID, &authorID, &last); err != nil {
				rows.Close()
				return nil, err
			}
			m := out[bookID]
			m.IDs = append(m.IDs, authorID)
			if last = strings.TrimSpace(last); last != "" {
				m.Lasts = append(m.Lasts, last)
			}
			out[bookID] = m
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
