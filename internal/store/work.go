package store

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	// FTS already ranks; keep a window before edition grouping.
	searchCandidateCap = 2000
	// Year/added-only filters have no rank — need enough rows to group editions.
	filterCandidateCap = 250_000
	maxEditions        = 40
	listGroupCap       = 5000
)

// EditionRef is one file/edition of a work (same title + authors).
type EditionRef struct {
	ID          int64    `json:"id"`
	Year        int      `json:"year,omitempty"`
	Size        int64    `json:"size,omitempty"`
	Series      string   `json:"series,omitempty"`
	SeriesNum   int      `json:"seriesNum,omitempty"`
	Translators []string `json:"translators,omitempty"`
	Publisher   string   `json:"publisher,omitempty"`
	City        string   `json:"city,omitempty"`
	PubYear     string   `json:"pubYear,omitempty"`
	Current     bool     `json:"current,omitempty"`
}

func workKey(title string, authorIDs []int64) string {
	ids := append([]int64(nil), authorIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return normalizeTitle(title) + "\x00" + strings.Join(parts, ",")
}

func betterEdition(a, b Book) bool {
	if a.Size != b.Size {
		return a.Size > b.Size
	}
	if a.Year != b.Year {
		return a.Year > b.Year
	}
	return a.ID < b.ID
}

// groupBooksToWorks collapses editions sharing title+authors. Order follows
// first occurrence in books (e.g. FTS rank). Representative prefers larger file.
func groupBooksToWorks(books []Book, authorIDs map[int64][]int64) []Book {
	if len(books) == 0 {
		return nil
	}
	type acc struct {
		book  Book
		count int
	}
	byKey := make(map[string]*acc, len(books))
	order := make([]string, 0, len(books))
	for _, b := range books {
		key := workKey(b.Title, authorIDs[b.ID])
		if a, ok := byKey[key]; ok {
			a.count++
			if betterEdition(b, a.book) {
				a.book = b
			}
			continue
		}
		byKey[key] = &acc{book: b, count: 1}
		order = append(order, key)
	}
	out := make([]Book, 0, len(order))
	for _, key := range order {
		a := byKey[key]
		a.book.EditionCount = a.count
		out = append(out, a.book)
	}
	return out
}

func paginateBooks(books []Book, limit, offset int) ([]Book, int) {
	total := len(books)
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 60
	}
	if offset >= total {
		return nil, total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return books[offset:end], total
}

func (s *Store) authorIDsForBooks(bookIDs []int64) (map[int64][]int64, error) {
	out := make(map[int64][]int64, len(bookIDs))
	if len(bookIDs) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(bookIDs))
	args := make([]any, len(bookIDs))
	for i, id := range bookIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := s.db.Query(`
SELECT book_id, author_id FROM book_authors
WHERE book_id IN (`+strings.Join(placeholders, ",")+`)
ORDER BY book_id, author_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var bookID, authorID int64
		if err := rows.Scan(&bookID, &authorID); err != nil {
			return nil, err
		}
		out[bookID] = append(out[bookID], authorID)
	}
	return out, rows.Err()
}

func (s *Store) groupAndPaginate(books []Book, limit, offset int) ([]Book, int, error) {
	ids := make([]int64, len(books))
	for i := range books {
		ids[i] = books[i].ID
	}
	auth, err := s.authorIDsForBooks(ids)
	if err != nil {
		return nil, 0, err
	}
	works := groupBooksToWorks(books, auth)
	page, total := paginateBooks(works, limit, offset)
	return page, total, nil
}

// EditionsForBook returns editions of the same work (title + authors), up to maxEditions.
func (s *Store) EditionsForBook(bookID int64) ([]EditionRef, error) {
	var title string
	lang, langArgs := s.langPred("lang")
	args := append([]any{bookID}, langArgs...)
	err := s.db.QueryRow(`
SELECT title FROM books WHERE id = ? AND deleted = 0`+lang, args...).Scan(&title)
	if err != nil {
		return nil, err
	}
	authMap, err := s.authorIDsForBooks([]int64{bookID})
	if err != nil {
		return nil, err
	}
	authorIDs := authMap[bookID]
	key := workKey(title, authorIDs)

	var rowsBooks []Book
	if len(authorIDs) == 0 {
		b, err := s.BooksByIDs([]int64{bookID})
		if err != nil {
			return nil, err
		}
		rowsBooks = b
	} else {
		// Candidate pool: books that share the first author, then filter by workKey.
		lang, langArgs := s.langPred("b.lang")
		qargs := append(append([]any{}, langArgs...), authorIDs[0], listGroupCap)
		rows, err := s.db.Query(`
SELECT`+bookColumns+`
FROM books b
LEFT JOIN series s ON s.id = b.series_id
WHERE b.deleted = 0`+lang+`
  AND b.id IN (SELECT book_id FROM book_authors WHERE author_id = ?)
ORDER BY b.size DESC, coalesce(b.year, 0) DESC, b.id
LIMIT ?`, qargs...)
		if err != nil {
			return nil, fmt.Errorf("editions: %w", err)
		}
		defer rows.Close()
		rowsBooks, err = scanBooks(rows)
		if err != nil {
			return nil, err
		}
	}

	ids := make([]int64, len(rowsBooks))
	for i := range rowsBooks {
		ids[i] = rowsBooks[i].ID
	}
	allAuth, err := s.authorIDsForBooks(ids)
	if err != nil {
		return nil, err
	}

	out := make([]EditionRef, 0, 8)
	seen := false
	for _, b := range rowsBooks {
		if workKey(b.Title, allAuth[b.ID]) != key {
			continue
		}
		if b.ID == bookID {
			seen = true
		}
		out = append(out, EditionRef{
			ID:        b.ID,
			Year:      b.Year,
			Size:      b.Size,
			Series:    b.Series,
			SeriesNum: b.SeriesNum,
			Current:   b.ID == bookID,
		})
		if len(out) >= maxEditions {
			break
		}
	}
	if !seen {
		out = append([]EditionRef{{ID: bookID, Current: true}}, out...)
		if len(out) > maxEditions {
			out = out[:maxEditions]
		}
	}
	return out, nil
}
