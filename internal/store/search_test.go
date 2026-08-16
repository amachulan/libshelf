package store

import (
	"path/filepath"
	"strconv"
	"testing"
)

func TestFtsQueryCrossField(t *testing.T) {
	got := ftsQuery("кинг сияние")
	want := `"кинг"* AND "сияние"*`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFtsQueryEmpty(t *testing.T) {
	if ftsQuery("   ") != "" {
		t.Fatal("expected empty")
	}
}

func TestFtsMatchAuthorAndTitle(t *testing.T) {
	got := ftsMatch(SearchQuery{Author: "кинг", Title: "сияние"})
	want := `title: ("сияние"*) AND authors: ("кинг"*)`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFtsMatchGeneralAndAuthor(t *testing.T) {
	got := ftsMatch(SearchQuery{Q: "сияние", Author: "кинг"})
	want := `("сияние"*) AND authors: ("кинг"*)`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSearchQueryNormalizeYear(t *testing.T) {
	q := SearchQuery{YearFrom: 2025}
	q.NormalizeYear()
	if q.YearFrom != 2025 || q.YearTo != 9999 {
		t.Fatalf("open to: %+v", q)
	}
	q = SearchQuery{YearTo: 2020}
	q.NormalizeYear()
	if q.YearFrom != 1 || q.YearTo != 2020 {
		t.Fatalf("open from: %+v", q)
	}
	q = SearchQuery{YearFrom: 2025, YearTo: 2026}
	q.NormalizeYear()
	if q.YearFrom != 2025 || q.YearTo != 2026 {
		t.Fatalf("range: %+v", q)
	}
	q = SearchQuery{YearFrom: 2020, YearTo: 2010}
	q.NormalizeYear()
	if q.YearFrom != 2010 || q.YearTo != 2020 {
		t.Fatalf("swap: %+v", q)
	}
	q = SearchQuery{AddedFrom: 2025}
	q.NormalizeYear()
	if q.AddedFrom != 2025 || q.AddedTo != 9999 {
		t.Fatalf("added open to: %+v", q)
	}
}

func TestSearchQueryEmpty(t *testing.T) {
	if !(SearchQuery{}).Empty() {
		t.Fatal("expected empty")
	}
	if (SearchQuery{YearFrom: 2026}).Empty() {
		t.Fatal("year-only should not be empty")
	}
	if (SearchQuery{AddedFrom: 2025}).Empty() {
		t.Fatal("added-only should not be empty")
	}
	if (SearchQuery{Author: "кинг"}).Empty() {
		t.Fatal("author should not be empty")
	}
}

func TestSearchAuthorsRanksByBookCountBeforeLimit(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "libshelf.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := s.db.Exec(`INSERT INTO folders(id, name) VALUES(1, 'f')`); err != nil {
		t.Fatal(err)
	}

	// First 8 alphabetically would hide Стивен if LIMIT is applied before ranking.
	early := []string{"Ада", "Алексей", "Брайан", "Бретт", "Валери", "Вики", "Генри", "Говард"}
	for _, first := range early {
		insertAuthorBooks(t, s, "Кинг", first, 1)
	}
	kingID := insertAuthorBooks(t, s, "Кинг", "Стивен", 20)

	got, err := s.SearchAuthors("кинг", 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 8 {
		t.Fatalf("got %d authors, want 8", len(got))
	}
	if got[0].ID != kingID || got[0].Books != 20 {
		t.Fatalf("first author = %+v, want Кинг Стивен with 20 books (id %d)", got[0], kingID)
	}
}

func insertAuthorBooks(t *testing.T, s *Store, last, first string, n int) int64 {
	t.Helper()
	res, err := s.db.Exec(`INSERT INTO authors(last_name, first_name) VALUES(?, ?)`, last, first)
	if err != nil {
		t.Fatal(err)
	}
	authorID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		res, err = s.db.Exec(
			`INSERT INTO books(title, folder_id, file, lang) VALUES(?, 1, ?, 'ru')`,
			first, first+"-"+strconv.Itoa(i)+".fb2",
		)
		if err != nil {
			t.Fatal(err)
		}
		bookID, err := res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`INSERT INTO book_authors(book_id, author_id) VALUES(?, ?)`, bookID, authorID); err != nil {
			t.Fatal(err)
		}
	}
	return authorID
}
