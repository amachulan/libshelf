package store

import (
	"fmt"
	"strings"
	"unicode"
)

// SearchQuery is a text/year filter for catalog search.
// Empty text fields are ignored. YearFrom/YearTo are inclusive; 0 means unset.
// If only YearFrom is set, it matches that exact year.
type SearchQuery struct {
	Q        string
	Author   string
	Title    string
	YearFrom int
	YearTo   int
}

func (q SearchQuery) hasText() bool {
	return strings.TrimSpace(q.Q) != "" ||
		strings.TrimSpace(q.Author) != "" ||
		strings.TrimSpace(q.Title) != ""
}

func (q SearchQuery) hasYear() bool {
	return q.YearFrom > 0 || q.YearTo > 0
}

func (q SearchQuery) Empty() bool {
	return !q.hasText() && !q.hasYear()
}

// NormalizeYear clamps inverted ranges and turns a lone YearFrom into an exact year.
func (q *SearchQuery) NormalizeYear() {
	if q.YearFrom > 0 && q.YearTo == 0 {
		q.YearTo = q.YearFrom
	}
	if q.YearTo > 0 && q.YearFrom == 0 {
		q.YearFrom = 1
	}
	if q.YearFrom > 0 && q.YearTo > 0 && q.YearFrom > q.YearTo {
		q.YearFrom, q.YearTo = q.YearTo, q.YearFrom
	}
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
	if match != "" {
		sb.WriteString(`
ORDER BY bm25(book_search, 2.0, 8.0, 4.0), b.title
`)
	} else {
		sb.WriteString(`
ORDER BY coalesce(b.year, 0) DESC, b.title
`)
	}
	sb.WriteString(`LIMIT ?`)
	args = append(args, searchCandidateCap)

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
