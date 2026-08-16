package store

import (
	"fmt"
	"strings"
	"unicode"
)

// SearchQuery is a text/year filter for catalog search.
// Empty text fields are ignored. Year/Added ranges are inclusive; 0 means unset.
type SearchQuery struct {
	Q         string
	Author    string
	Title     string
	YearFrom  int // publication year (INPX YEAR)
	YearTo    int
	AddedFrom int // year book was added to the dump (INPX DATE → books.added)
	AddedTo   int
}

func (q SearchQuery) hasText() bool {
	return strings.TrimSpace(q.Q) != "" ||
		strings.TrimSpace(q.Author) != "" ||
		strings.TrimSpace(q.Title) != ""
}

func (q SearchQuery) hasYear() bool {
	return q.YearFrom > 0 || q.YearTo > 0
}

func (q SearchQuery) hasAdded() bool {
	return q.AddedFrom > 0 || q.AddedTo > 0
}

func (q SearchQuery) Empty() bool {
	return !q.hasText() && !q.hasYear() && !q.hasAdded()
}

func normalizeYearRange(from, to *int) {
	const yearMax = 9999
	if *from > 0 && *to == 0 {
		*to = yearMax
	}
	if *to > 0 && *from == 0 {
		*from = 1
	}
	if *from > 0 && *to > 0 && *from > *to {
		*from, *to = *to, *from
	}
}

// NormalizeYear clamps inverted / open publication-year and added-year ranges.
// Only From → from that year onward; only To → up to that year.
func (q *SearchQuery) NormalizeYear() {
	normalizeYearRange(&q.YearFrom, &q.YearTo)
	normalizeYearRange(&q.AddedFrom, &q.AddedTo)
}

// ftsQuery builds an FTS5 expression where every token must match
// somewhere in the row (title OR authors OR series), not a single column.
// Token order does not matter (AND), so "кинг стивен" ≡ "steven кинг".
func ftsQuery(input string) string {
	input = foldYo(input)
	var terms []string
	for _, tok := range strings.FieldsFunc(input, func(r rune) bool {
		return !isWordRune(r)
	}) {
		tok = strings.ReplaceAll(tok, `"`, "")
		if tok == "" {
			continue
		}
		terms = append(terms, `"`+tok+`"*`)
	}
	return strings.Join(terms, " AND ")
}

func isWordRune(r rune) bool {
	return r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// ftsMatch builds a column-aware FTS5 MATCH string from structured fields.
func ftsMatch(q SearchQuery) string {
	var parts []string
	if t := ftsQuery(q.Q); t != "" {
		parts = append(parts, "("+t+")")
	}
	if t := ftsQuery(q.Title); t != "" {
		parts = append(parts, "title: ("+t+")")
	}
	if t := ftsQuery(q.Author); t != "" {
		parts = append(parts, "authors: ("+t+")")
	}
	return strings.Join(parts, " AND ")
}

// Search returns works matching a free-text query (all FTS columns).
func (s *Store) Search(query string, limit, offset int) ([]Book, int, error) {
	return s.SearchBy(SearchQuery{Q: query}, limit, offset)
}

// SearchBy returns works matching structured filters. total is the work count.
func (s *Store) SearchBy(q SearchQuery, limit, offset int) ([]Book, int, error) {
	if limit <= 0 {
		limit = 40
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	q.NormalizeYear()
	if q.Empty() {
		return nil, 0, nil
	}

	match := ftsMatch(q)
	lang, langArgs := s.langPred("b.lang")

	var (
		sb   strings.Builder
		args []any
	)
	sb.WriteString(`SELECT` + bookColumns + `
FROM `)
	if match != "" {
		sb.WriteString(`book_search
JOIN books b ON b.id = book_search.rowid
`)
	} else {
		sb.WriteString(`books b
`)
	}
	sb.WriteString(`LEFT JOIN series s ON s.id = b.series_id
WHERE b.deleted = 0` + lang)
	args = append(args, langArgs...)
	if match != "" {
		sb.WriteString(`
  AND book_search MATCH ?`)
		args = append(args, match)
	}
	if q.hasYear() {
		sb.WriteString(`
  AND coalesce(b.year, 0) >= ? AND coalesce(b.year, 0) <= ?`)
		args = append(args, q.YearFrom, q.YearTo)
	}
	if q.hasAdded() {
		// books.added is INPX DATE text, usually YYYY-MM-DD.
		sb.WriteString(`
  AND length(b.added) >= 4
  AND CAST(substr(b.added, 1, 4) AS INTEGER) >= ?
  AND CAST(substr(b.added, 1, 4) AS INTEGER) <= ?`)
		args = append(args, q.AddedFrom, q.AddedTo)
	}
	if match != "" {
		sb.WriteString(`
ORDER BY bm25(book_search, 2.0, 8.0, 4.0), b.title
LIMIT ?`)
		args = append(args, searchCandidateCap)
	} else if q.hasAdded() {
		sb.WriteString(`
ORDER BY b.added DESC, b.title
LIMIT ?`)
		args = append(args, filterCandidateCap)
	} else {
		sb.WriteString(`
ORDER BY coalesce(b.year, 0) DESC, b.title
LIMIT ?`)
		args = append(args, filterCandidateCap)
	}

	rows, err := s.db.Query(sb.String(), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()

	var candidates []Book
	for rows.Next() {
		b, err := s.scanBook(rows)
		if err != nil {
			return nil, 0, err
		}
		candidates = append(candidates, b)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if len(candidates) == 0 {
		return nil, 0, nil
	}
	return s.groupAndPaginate(candidates, limit, offset)
}

// SearchAuthors returns authors whose name parts match all query tokens (any order).
// Ranked by visible book count, then limited — unlike catalog prefix browse,
// which pages alphabetically and must not pre-cut by popularity.
func (s *Store) SearchAuthors(query string, limit int) ([]CatalogPerson, error) {
	tokens := strings.Fields(foldYo(strings.TrimSpace(query)))
	if len(tokens) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 24
	}
	if limit > 40 {
		limit = 40
	}

	lang, langArgs := s.langPred("b.lang")

	var (
		where []string
		args  []any
	)
	args = append(args, langArgs...)
	for _, tok := range tokens {
		pats := likePrefixPatterns(tok)
		wLast, aLast := likeOrColumn("a.last_name", pats)
		wFirst, aFirst := likeOrColumn("a.first_name", pats)
		wMid, aMid := likeOrColumn("a.middle_name", pats)
		where = append(where, `(`+wLast+` OR `+wFirst+` OR `+wMid+`)`)
		args = append(args, aLast...)
		args = append(args, aFirst...)
		args = append(args, aMid...)
	}
	args = append(args, limit)

	rows, err := s.db.Query(`
SELECT a.id,
       trim(a.last_name || ' ' || a.first_name || ' ' || a.middle_name) AS name,
       count(*)
FROM authors a
JOIN book_authors ba ON ba.author_id = a.id
JOIN books b ON b.id = ba.book_id AND b.deleted = 0`+lang+`
WHERE `+strings.Join(where, " AND ")+`
GROUP BY a.id
ORDER BY 3 DESC, a.last_name, a.first_name
LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCatalogPeople(rows)
}
