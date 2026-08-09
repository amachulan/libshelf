package store

import (
	"fmt"
	"strings"
	"unicode"
)

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

// Search returns works (grouped editions) matching query. total is the work count.
func (s *Store) Search(query string, limit, offset int) ([]Book, int, error) {
	if limit <= 0 {
		limit = 40
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	q := ftsQuery(query)
	if q == "" {
		return nil, 0, nil
	}

	rows, err := s.db.Query(`
SELECT`+bookColumns+`
FROM book_search
JOIN books b ON b.id = book_search.rowid
LEFT JOIN series s ON s.id = b.series_id
WHERE b.deleted = 0 AND b.lang = 'ru'
  AND book_search MATCH ?
ORDER BY bm25(book_search, 2.0, 8.0, 4.0), b.title
LIMIT ?`, q, searchCandidateCap)
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
