package store

import (
	"database/sql"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"libshelf/internal/genres"
)

type LetterCount struct {
	Letter string `json:"letter"`
	Count  int    `json:"count"`
}

type CatalogPerson struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Books int    `json:"books"`
}

type CatalogGenre struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	Books int    `json:"books"`
}

type CatalogSeries struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Books int    `json:"books"`
}

type catalogCache struct {
	mu            sync.Mutex
	authorLetters []LetterCount
	seriesLetters []LetterCount
	genres        []CatalogGenre
}

func firstLetter(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "#"
	}
	r, _ := utf8.DecodeRuneInString(strings.ToUpper(name))
	if r == 'Ё' {
		r = 'Е'
	}
	if (r >= 'А' && r <= 'Я') || (r >= 'A' && r <= 'Z') {
		return string(r)
	}
	return "#"
}

func normalizeLetter(letter string) string {
	letter = strings.ToUpper(strings.TrimSpace(letter))
	if letter == "Ё" {
		return "Е"
	}
	return letter
}

func authorHasRUBooksSQL() string {
	return `EXISTS (
  SELECT 1 FROM book_authors ba
  JOIN books b ON b.id = ba.book_id AND b.deleted = 0 AND b.lang = 'ru'
  WHERE ba.author_id = a.id
)`
}

func (s *Store) AuthorLetters() ([]LetterCount, error) {
	s.catCache.mu.Lock()
	defer s.catCache.mu.Unlock()
	if s.catCache.authorLetters != nil {
		return s.catCache.authorLetters, nil
	}

	rows, err := s.db.Query(`
SELECT upper(replace(substr(trim(a.last_name), 1, 1), 'Ё', 'Е')), count(*)
FROM authors a
WHERE ` + authorHasRUBooksSQL() + `
GROUP BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	merged := map[string]int{}
	for rows.Next() {
		var raw sql.NullString
		var n int
		if err := rows.Scan(&raw, &n); err != nil {
			return nil, err
		}
		merged[firstLetter(raw.String)] += n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := sortLetters(merged)
	s.catCache.authorLetters = out
	return out, nil
}

func sortLetters(merged map[string]int) []LetterCount {
	var cyr, lat []string
	hasHash := false
	for l := range merged {
		switch {
		case l == "#":
			hasHash = true
		case l >= "А" && l <= "Я":
			cyr = append(cyr, l)
		default:
			lat = append(lat, l)
		}
	}
	sort.Strings(cyr)
	sort.Strings(lat)
	out := make([]LetterCount, 0, len(merged))
	for _, l := range cyr {
		out = append(out, LetterCount{Letter: l, Count: merged[l]})
	}
	for _, l := range lat {
		out = append(out, LetterCount{Letter: l, Count: merged[l]})
	}
	if hasHash {
		out = append(out, LetterCount{Letter: "#", Count: merged["#"]})
	}
	return out
}

func (s *Store) AuthorsByLetter(letter string, limit, offset int) ([]CatalogPerson, error) {
	letter = normalizeLetter(letter)
	if limit <= 0 {
		limit = 100
	}
	if limit > 300 {
		limit = 300
	}
	if offset < 0 {
		offset = 0
	}

	bookCount := `(
  SELECT count(*) FROM book_authors ba
  JOIN books b ON b.id = ba.book_id AND b.deleted = 0 AND b.lang = 'ru'
  WHERE ba.author_id = a.id
)`
	q := `
SELECT a.id,
       trim(a.last_name || ' ' || a.first_name || ' ' || a.middle_name),
       ` + bookCount + `
FROM authors a
WHERE ` + authorHasRUBooksSQL() + `
`
	var (
		rows *sql.Rows
		err  error
	)
	if letter == "#" {
		rows, err = s.db.Query(q+`
AND (
  trim(a.last_name) = ''
  OR (
    upper(replace(substr(trim(a.last_name),1,1),'Ё','Е')) NOT BETWEEN 'A' AND 'Z'
    AND upper(replace(substr(trim(a.last_name),1,1),'Ё','Е')) NOT BETWEEN 'А' AND 'Я'
  )
)
ORDER BY a.last_name, a.first_name
LIMIT ? OFFSET ?`, limit, offset)
	} else {
		rows, err = s.db.Query(q+`
AND upper(replace(substr(trim(a.last_name),1,1),'Ё','Е')) = ?
ORDER BY a.last_name, a.first_name
LIMIT ? OFFSET ?`, letter, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCatalogPeople(rows)
}

func (s *Store) SeriesLetters() ([]LetterCount, error) {
	s.catCache.mu.Lock()
	defer s.catCache.mu.Unlock()
	if s.catCache.seriesLetters != nil {
		return s.catCache.seriesLetters, nil
	}

	rows, err := s.db.Query(`
SELECT upper(replace(substr(trim(s.title), 1, 1), 'Ё', 'Е')), count(*)
FROM series s
WHERE EXISTS (
  SELECT 1 FROM books b
  WHERE b.series_id = s.id AND b.deleted = 0 AND b.lang = 'ru'
)
GROUP BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	merged := map[string]int{}
	for rows.Next() {
		var raw sql.NullString
		var n int
		if err := rows.Scan(&raw, &n); err != nil {
			return nil, err
		}
		merged[firstLetter(raw.String)] += n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := sortLetters(merged)
	s.catCache.seriesLetters = out
	return out, nil
}

func (s *Store) SeriesByLetter(letter string, limit, offset int) ([]CatalogSeries, error) {
	letter = normalizeLetter(letter)
	if limit <= 0 {
		limit = 100
	}
	if limit > 300 {
		limit = 300
	}
	if offset < 0 {
		offset = 0
	}
	bookCount := `(
  SELECT count(*) FROM books b
  WHERE b.series_id = s.id AND b.deleted = 0 AND b.lang = 'ru'
)`
	base := `
SELECT s.id, s.title, ` + bookCount + `
FROM series s
WHERE EXISTS (
  SELECT 1 FROM books b
  WHERE b.series_id = s.id AND b.deleted = 0 AND b.lang = 'ru'
)
`
	var (
		rows *sql.Rows
		err  error
	)
	if letter == "#" {
		rows, err = s.db.Query(base+`
AND (
  trim(s.title) = ''
  OR (
    upper(replace(substr(trim(s.title),1,1),'Ё','Е')) NOT BETWEEN 'A' AND 'Z'
    AND upper(replace(substr(trim(s.title),1,1),'Ё','Е')) NOT BETWEEN 'А' AND 'Я'
  )
)
ORDER BY s.title
LIMIT ? OFFSET ?`, limit, offset)
	} else {
		rows, err = s.db.Query(base+`
AND upper(replace(substr(trim(s.title),1,1),'Ё','Е')) = ?
ORDER BY s.title
LIMIT ? OFFSET ?`, letter, limit, offset)
	}
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

func (s *Store) ListGenres() ([]CatalogGenre, error) {
	s.catCache.mu.Lock()
	defer s.catCache.mu.Unlock()
	if s.catCache.genres != nil {
		return s.catCache.genres, nil
	}

	rows, err := s.db.Query(`
SELECT g.code, count(*)
FROM genres g
JOIN book_genres bg ON bg.genre_id = g.id
JOIN books b ON b.id = bg.book_id AND b.deleted = 0 AND b.lang = 'ru'
GROUP BY g.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CatalogGenre
	for rows.Next() {
		var g CatalogGenre
		if err := rows.Scan(&g.Code, &g.Books); err != nil {
			return nil, err
		}
		g.Name = genres.Name(g.Code)
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	s.catCache.genres = out
	return out, nil
}

func (s *Store) GenreBooks(code string, limit, offset int) (*NamedList, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, ErrNotFound
	}
	if limit <= 0 {
		limit = 60
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	var genreID int64
	err := s.db.QueryRow(`SELECT id FROM genres WHERE code = ?`, code).Scan(&genreID)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var total int
	if err := s.db.QueryRow(`
SELECT count(*) FROM book_genres bg
JOIN books b ON b.id = bg.book_id
WHERE bg.genre_id = ? AND b.deleted = 0 AND b.lang = 'ru'`, genreID).Scan(&total); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
SELECT`+bookColumns+`
FROM books b
LEFT JOIN series s ON s.id = b.series_id
JOIN book_genres bg ON bg.book_id = b.id AND bg.genre_id = ?
WHERE b.deleted = 0 AND b.lang = 'ru'
ORDER BY b.title
LIMIT ? OFFSET ?`, genreID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	books, err := scanBooks(rows)
	if err != nil {
		return nil, err
	}
	return &NamedList{ID: genreID, Name: code, Books: books, Total: total}, nil
}

func scanCatalogPeople(rows *sql.Rows) ([]CatalogPerson, error) {
	var out []CatalogPerson
	for rows.Next() {
		var p CatalogPerson
		if err := rows.Scan(&p.ID, &p.Name, &p.Books); err != nil {
			return nil, err
		}
		p.Name = strings.TrimSpace(p.Name)
		out = append(out, p)
	}
	return out, rows.Err()
}
