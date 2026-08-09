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

	// Order by FTS rank (authors weighted highest), not title. Title sort + LIMIT
	// hid most of a prolific author: "steven кинг" never reached "Тёмная башня".
	rows, err := s.db.Query(`
SELECT`+bookColumns+`
FROM book_search
JOIN books b ON b.id = book_search.rowid
LEFT JOIN series s ON s.id = b.series_id
WHERE b.deleted = 0 AND b.lang = 'ru'
  AND book_search MATCH ?
ORDER BY bm25(book_search, 2.0, 8.0, 4.0), b.title
LIMIT ?`, q, limit)
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
