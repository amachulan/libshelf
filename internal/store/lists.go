package store

import (
	"database/sql"
	"strings"
)

type AuthorRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type GenreRef struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type NamedList struct {
	ID     int64           `json:"id"`
	Name   string          `json:"name"`
	Code   string          `json:"code,omitempty"`
	Sort   string          `json:"sort,omitempty"`
	Books  []Book          `json:"books"`
	Total  int             `json:"total"`
	Series []CatalogSeries `json:"series,omitempty"`
}

// NormalizeListSort maps UI/API sort aliases. Empty and unknown → popular.
func NormalizeListSort(sort string) string {
	switch strings.ToLower(strings.TrimSpace(sort)) {
	case "new", "added", "date":
		return "new"
	case "title", "alpha", "name":
		return "title"
	default:
		return "popular"
	}
}

// AuthorSeries returns series that contain this author's visible books.
func (s *Store) AuthorSeries(authorID int64) ([]CatalogSeries, error) {
	lang, langArgs := s.langPred("b.lang")
	args := append(append([]any{}, langArgs...), authorID)
	rows, err := s.db.Query(`
SELECT s.id, s.title, count(*)
FROM series s
JOIN books b ON b.series_id = s.id AND b.deleted = 0`+lang+`
JOIN book_authors ba ON ba.book_id = b.id AND ba.author_id = ?
GROUP BY s.id
ORDER BY s.title
LIMIT 200`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CatalogSeries
	for rows.Next() {
		var it CatalogSeries
		if err := rows.Scan(&it.ID, &it.Title, &it.Books); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) AuthorBooks(authorID int64, limit, offset int) (*NamedList, error) {
	if limit <= 0 {
		limit = 60
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	var name string
	err := s.db.QueryRow(`
SELECT trim(last_name || ' ' || first_name || ' ' || middle_name)
FROM authors WHERE id = ?`, authorID).Scan(&name)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	lang, langArgs := s.langPred("b.lang")
	args := append(append([]any{}, langArgs...), authorID, listGroupCap)
	rows, err := s.db.Query(`
SELECT`+bookColumns+`
FROM books b
LEFT JOIN series s ON s.id = b.series_id
WHERE b.deleted = 0`+lang+`
  AND b.id IN (SELECT book_id FROM book_authors WHERE author_id = ?)
ORDER BY coalesce(s.title, ''), b.series_num, b.title
LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	books, err := scanBooks(rows)
	if err != nil {
		return nil, err
	}
	page, total, err := s.groupAndPaginate(books, limit, offset)
	if err != nil {
		return nil, err
	}
	series, err := s.AuthorSeries(authorID)
	if err != nil {
		return nil, err
	}
	return &NamedList{
		ID:     authorID,
		Name:   strings.TrimSpace(name),
		Books:  page,
		Total:  total,
		Series: series,
	}, nil
}

func (s *Store) SeriesBooks(seriesID int64, limit, offset int) (*NamedList, error) {
	if limit <= 0 {
		limit = 60
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	var title string
	err := s.db.QueryRow(`SELECT title FROM series WHERE id = ?`, seriesID).Scan(&title)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	lang, langArgs := s.langPred("b.lang")
	args := append(append([]any{}, langArgs...), seriesID, listGroupCap)
	rows, err := s.db.Query(`
SELECT`+bookColumns+`
FROM books b
LEFT JOIN series s ON s.id = b.series_id
WHERE b.deleted = 0`+lang+` AND b.series_id = ?
ORDER BY b.series_num, b.title
LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	books, err := scanBooks(rows)
	if err != nil {
		return nil, err
	}
	page, total, err := s.groupAndPaginate(books, limit, offset)
	if err != nil {
		return nil, err
	}
	return &NamedList{ID: seriesID, Name: title, Books: page, Total: total}, nil
}

func scanBooks(rows *sql.Rows) ([]Book, error) {
	var out []Book
	for rows.Next() {
		var b Book
		if err := rows.Scan(&b.ID, &b.Title, &b.Authors, &b.Series, &b.SeriesNum,
			&b.Year, &b.Ext, &b.Size, &b.Lang, &b.Rate); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// BooksByIDs returns books for the given ids, preserving input order when possible.
func (s *Store) BooksByIDs(ids []int64) ([]Book, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	lang, langArgs := s.langPred("b.lang")
	q := `
SELECT` + bookColumns + `
FROM books b
LEFT JOIN series s ON s.id = b.series_id
WHERE b.deleted = 0` + lang + ` AND b.id IN (` + strings.Join(placeholders, ",") + `)`
	rows, err := s.db.Query(q, append(langArgs, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := make(map[int64]Book, len(ids))
	for rows.Next() {
		var b Book
		if err := rows.Scan(&b.ID, &b.Title, &b.Authors, &b.Series, &b.SeriesNum,
			&b.Year, &b.Ext, &b.Size, &b.Lang, &b.Rate); err != nil {
			return nil, err
		}
		byID[b.ID] = b
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Book, 0, len(ids))
	for _, id := range ids {
		if b, ok := byID[id]; ok {
			out = append(out, b)
		}
	}
	return out, nil
}
