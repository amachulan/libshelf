package store

import (
	"path/filepath"
	"testing"
)

func TestGenreBooksCache(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "libshelf.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Exec(`INSERT INTO folders(id, name) VALUES(1, 'f')`); err != nil {
		t.Fatal(err)
	}
	if err := s.Exec(`INSERT INTO authors(id, last_name, first_name) VALUES(1, 'Кинг', 'Стивен')`); err != nil {
		t.Fatal(err)
	}
	if err := s.Exec(`INSERT INTO genres(id, code) VALUES(1, 'sf_action')`); err != nil {
		t.Fatal(err)
	}
	insertGenreBook(t, s, 10, "Оно", 1)
	insertGenreBook(t, s, 11, "Оно", 1)
	insertGenreBook(t, s, 12, "Сияние", 1)

	first, err := s.GenreBooks("sf_action", 60, 0, "title")
	if err != nil {
		t.Fatal(err)
	}
	if first.Total != 2 {
		t.Fatalf("total=%d want 2", first.Total)
	}
	if len(first.Books) != 2 {
		t.Fatalf("books=%d", len(first.Books))
	}
	var ono *Book
	for i := range first.Books {
		if first.Books[i].Title == "Оно" {
			ono = &first.Books[i]
			break
		}
	}
	if ono == nil || ono.EditionCount != 2 {
		t.Fatalf("оно: %+v", ono)
	}

	s.genreCache.mu.Lock()
	cached := len(s.genreCache.lists)
	s.genreCache.mu.Unlock()
	if cached != 1 {
		t.Fatalf("cache entries=%d want 1", cached)
	}

	second, err := s.GenreBooks("sf_action", 1, 1, "title")
	if err != nil {
		t.Fatal(err)
	}
	if second.Total != 2 || len(second.Books) != 1 {
		t.Fatalf("page 2: total=%d n=%d", second.Total, len(second.Books))
	}

	insertGenreBook(t, s, 13, "Кэрри", 1)
	stale, err := s.GenreBooks("sf_action", 60, 0, "title")
	if err != nil {
		t.Fatal(err)
	}
	if stale.Total != 2 {
		t.Fatalf("stale cache total=%d want 2", stale.Total)
	}
	s.InvalidateCatalogCache()
	fresh, err := s.GenreBooks("sf_action", 60, 0, "title")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Total != 3 {
		t.Fatalf("after invalidate total=%d want 3", fresh.Total)
	}

	if _, err := s.GenreBooks("sf_action", 60, 0, "popular"); err != nil {
		t.Fatal(err)
	}
	s.genreCache.mu.Lock()
	nkeys := len(s.genreCache.lists)
	s.genreCache.mu.Unlock()
	if nkeys != 2 {
		t.Fatalf("sort keys=%d want 2", nkeys)
	}
}

func insertGenreBook(t *testing.T, s *Store, id int64, title string, genreID int64) {
	t.Helper()
	if err := s.Exec(
		`INSERT INTO books(id, title, folder_id, file, lang) VALUES(?, ?, 1, ?, 'ru')`,
		id, title, title+".fb2",
	); err != nil {
		t.Fatal(err)
	}
	if err := s.Exec(`INSERT INTO book_authors(book_id, author_id) VALUES(?, 1)`, id); err != nil {
		t.Fatal(err)
	}
	if err := s.Exec(`INSERT INTO book_genres(book_id, genre_id) VALUES(?, ?)`, id, genreID); err != nil {
		t.Fatal(err)
	}
}
