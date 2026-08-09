package store

import (
	"database/sql"
	"fmt"
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
	// Catalog is ru-only for now: Latin initials collapse into "#".
	if r >= 'А' && r <= 'Я' {
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

func isCyrillicLetter(letter string) bool {
	letter = normalizeLetter(letter)
	return letter >= "А" && letter <= "Я"
}

// nonCyrillicInitialSQL is true when the first letter is not А–Я (Latin, digits, empty, …).
func nonCyrillicInitialSQL(column string) string {
	return `(
  trim(` + column + `) = ''
  OR upper(replace(substr(trim(` + column + `),1,1),'Ё','Е')) NOT BETWEEN 'А' AND 'Я'
)`
}

func (s *Store) authorHasVisibleBooksSQL() (string, []any) {
	lang, args := s.langPred("b.lang")
	return `EXISTS (
  SELECT 1 FROM book_authors ba
  JOIN books b ON b.id = ba.book_id AND b.deleted = 0` + lang + `
  WHERE ba.author_id = a.id
)`, args
}

func (s *Store) AuthorLetters() ([]LetterCount, error) {
	s.catCache.mu.Lock()
	defer s.catCache.mu.Unlock()
	if s.catCache.authorLetters != nil {
		return s.catCache.authorLetters, nil
	}

	has, hasArgs := s.authorHasVisibleBooksSQL()
	rows, err := s.db.Query(`
SELECT upper(replace(substr(trim(a.last_name), 1, 1), 'Ё', 'Е')), count(*)
FROM authors a
WHERE `+has+`
GROUP BY 1`, hasArgs...)
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
	var cyr []string
	hash := 0
	for l, n := range merged {
		switch {
		case l == "#":
			hash += n
		case l >= "А" && l <= "Я":
			cyr = append(cyr, l)
		default:
			hash += n
		}
	}
	sort.Strings(cyr)
	out := make([]LetterCount, 0, len(cyr)+1)
	for _, l := range cyr {
		out = append(out, LetterCount{Letter: l, Count: merged[l]})
	}
	if hash > 0 {
		out = append(out, LetterCount{Letter: "#", Count: hash})
	}
	return out
}

func likePrefix(s string) string {
	s = strings.ToUpper(foldYo(strings.TrimSpace(s)))
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(s) + "%"
}

// AuthorsByPrefix returns authors whose name starts with query (last/first/full).
func (s *Store) AuthorsByPrefix(query string, limit int) ([]CatalogPerson, error) {
	tokens := strings.Fields(foldYo(strings.TrimSpace(query)))
	if len(tokens) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	lang, langArgs := s.langPred("b.lang")
	has, hasArgs := s.authorHasVisibleBooksSQL()
	bookCount := `(
  SELECT count(*) FROM book_authors ba
  JOIN books b ON b.id = ba.book_id AND b.deleted = 0` + lang + `
  WHERE ba.author_id = a.id
)`
	nameFold := `upper(replace(replace(%s,'ё','е'),'Ё','Е'))`
	last := fmt.Sprintf(nameFold, "a.last_name")
	first := fmt.Sprintf(nameFold, "a.first_name")
	full := fmt.Sprintf(nameFold, `trim(a.last_name || ' ' || a.first_name || ' ' || a.middle_name)`)

	var (
		where string
		args  []any
	)
	args = append(args, langArgs...)
	args = append(args, hasArgs...)
	if len(tokens) >= 2 {
		p0, p1 := likePrefix(tokens[0]), likePrefix(tokens[1])
		where = `(` + last + ` LIKE ? ESCAPE '\' AND ` + first + ` LIKE ? ESCAPE '\')`
		args = append(args, p0, p1)
	} else {
		p := likePrefix(tokens[0])
		where = `(` + last + ` LIKE ? ESCAPE '\' OR ` + first + ` LIKE ? ESCAPE '\' OR ` + full + ` LIKE ? ESCAPE '\')`
		args = append(args, p, p, p)
	}
	args = append(args, limit)

	rows, err := s.db.Query(`
SELECT a.id,
       trim(a.last_name || ' ' || a.first_name || ' ' || a.middle_name),
       `+bookCount+`
FROM authors a
WHERE `+has+`
  AND `+where+`
ORDER BY a.last_name, a.first_name
LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCatalogPeople(rows)
}

func (s *Store) AuthorsByLetter(letter string, limit, offset int) ([]CatalogPerson, error) {
	letter = normalizeLetter(letter)
	if letter != "#" && !isCyrillicLetter(letter) {
		return nil, nil
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 300 {
		limit = 300
	}
	if offset < 0 {
		offset = 0
	}

	lang, langArgs := s.langPred("b.lang")
	has, hasArgs := s.authorHasVisibleBooksSQL()
	bookCount := `(
  SELECT count(*) FROM book_authors ba
  JOIN books b ON b.id = ba.book_id AND b.deleted = 0` + lang + `
  WHERE ba.author_id = a.id
)`
	q := `
SELECT a.id,
       trim(a.last_name || ' ' || a.first_name || ' ' || a.middle_name),
       ` + bookCount + `
FROM authors a
WHERE ` + has + `
`
	var (
		rows *sql.Rows
		err  error
	)
	if letter == "#" {
		args := append(append([]any{}, langArgs...), hasArgs...)
		args = append(args, limit, offset)
		rows, err = s.db.Query(q+`
AND `+nonCyrillicInitialSQL("a.last_name")+`
ORDER BY a.last_name, a.first_name
LIMIT ? OFFSET ?`, args...)
	} else {
		args := append(append([]any{}, langArgs...), hasArgs...)
		args = append(args, letter, limit, offset)
		rows, err = s.db.Query(q+`
AND upper(replace(substr(trim(a.last_name),1,1),'Ё','Е')) = ?
ORDER BY a.last_name, a.first_name
LIMIT ? OFFSET ?`, args...)
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

	lang, langArgs := s.langPred("b.lang")
	rows, err := s.db.Query(`
SELECT upper(replace(substr(trim(s.title), 1, 1), 'Ё', 'Е')), count(*)
FROM series s
WHERE EXISTS (
  SELECT 1 FROM books b
  WHERE b.series_id = s.id AND b.deleted = 0`+lang+`
)
GROUP BY 1`, langArgs...)
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

// SeriesByPrefix returns series whose title starts with query.
func (s *Store) SeriesByPrefix(query string, limit int) ([]CatalogSeries, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	lang, langArgs := s.langPred("b.lang")
	bookCount := `(
  SELECT count(*) FROM books b
  WHERE b.series_id = s.id AND b.deleted = 0` + lang + `
)`
	titleFold := `upper(replace(replace(s.title,'ё','е'),'Ё','Е'))`
	pattern := likePrefix(query)
	args := append(append([]any{}, langArgs...), langArgs...)
	args = append(args, pattern, limit)
	rows, err := s.db.Query(`
SELECT s.id, s.title, `+bookCount+`
FROM series s
WHERE EXISTS (
  SELECT 1 FROM books b
  WHERE b.series_id = s.id AND b.deleted = 0`+lang+`
)
  AND `+titleFold+` LIKE ? ESCAPE '\'
ORDER BY s.title
LIMIT ?`, args...)
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

func (s *Store) SeriesByLetter(letter string, limit, offset int) ([]CatalogSeries, error) {
	letter = normalizeLetter(letter)
	if letter != "#" && !isCyrillicLetter(letter) {
		return nil, nil
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 300 {
		limit = 300
	}
	if offset < 0 {
		offset = 0
	}
	lang, langArgs := s.langPred("b.lang")
	bookCount := `(
  SELECT count(*) FROM books b
  WHERE b.series_id = s.id AND b.deleted = 0` + lang + `
)`
	base := `
SELECT s.id, s.title, ` + bookCount + `
FROM series s
WHERE EXISTS (
  SELECT 1 FROM books b
  WHERE b.series_id = s.id AND b.deleted = 0` + lang + `
)
`
	var (
		rows *sql.Rows
		err  error
	)
	if letter == "#" {
		args := append(append([]any{}, langArgs...), langArgs...)
		args = append(args, limit, offset)
		rows, err = s.db.Query(base+`
AND `+nonCyrillicInitialSQL("s.title")+`
ORDER BY s.title
LIMIT ? OFFSET ?`, args...)
	} else {
		args := append(append([]any{}, langArgs...), langArgs...)
		args = append(args, letter, limit, offset)
		rows, err = s.db.Query(base+`
AND upper(replace(substr(trim(s.title),1,1),'Ё','Е')) = ?
ORDER BY s.title
LIMIT ? OFFSET ?`, args...)
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

	lang, langArgs := s.langPred("b.lang")
	rows, err := s.db.Query(`
SELECT g.code, count(*)
FROM genres g
JOIN book_genres bg ON bg.genre_id = g.id
JOIN books b ON b.id = bg.book_id AND b.deleted = 0`+lang+`
GROUP BY g.id`, langArgs...)
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
		if !genres.Known(g.Code) {
			continue
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
	lang, langArgs := s.langPred("b.lang")
	args := append([]any{genreID}, append(langArgs, listGroupCap)...)
	rows, err := s.db.Query(`
SELECT`+bookColumns+`
FROM books b
LEFT JOIN series s ON s.id = b.series_id
JOIN book_genres bg ON bg.book_id = b.id AND bg.genre_id = ?
WHERE b.deleted = 0`+lang+`
ORDER BY b.title
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
	return &NamedList{ID: genreID, Name: code, Books: page, Total: total}, nil
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
