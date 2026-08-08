package store

import (
	"fmt"
	"strings"
	"unicode"
)

// ftsQuery builds an FTS5 expression where every token must match
// somewhere in the row (title OR authors OR series), not a single column.
func ftsQuery(input string) string {
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
	return strings.Join(terms, " ")
}

func isWordRune(r rune) bool {
	return r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func (s *Store) Search(query string, limit int) ([]Book, error) {
	if limit <= 0 {
		limit = 40
	}
	if limit > 100 {
		limit = 100
	}
	q := ftsQuery(query)
	if q == "" {
		return nil, nil
	}

	rows, err := s.db.Query(`
SELECT`+bookColumns+`
FROM books b
LEFT JOIN series s ON s.id = b.series_id
WHERE b.deleted = 0 AND b.lang = 'ru'
  AND b.id IN (
    SELECT rowid FROM book_search
    WHERE book_search MATCH ?
    ORDER BY rank
    LIMIT ?
  )
ORDER BY b.title`, q, limit)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()

	var out []Book
	for rows.Next() {
		b, err := s.scanBook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
