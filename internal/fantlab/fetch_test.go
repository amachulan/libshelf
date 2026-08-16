package fantlab

import (
	"context"
	"path/filepath"
	"testing"

	"libshelf/internal/store"
)

type fakeSearch struct {
	hits []Hit
}

func (f fakeSearch) SearchWorks(context.Context, string) ([]Hit, error) {
	return f.hits, nil
}

func TestFetchMatchesAndSkips(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "libshelf.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Exec(`INSERT INTO folders(id, name) VALUES(1, 'f')`); err != nil {
		t.Fatal(err)
	}
	if err := st.Exec(`INSERT INTO authors(id, last_name, first_name) VALUES(1, 'Симмонс', 'Дэн')`); err != nil {
		t.Fatal(err)
	}
	if err := st.Exec(`INSERT INTO books(id, title, folder_id, file, lang) VALUES(1, 'Гиперион', 1, 'h.fb2', 'ru')`); err != nil {
		t.Fatal(err)
	}
	if err := st.Exec(`INSERT INTO book_authors(book_id, author_id) VALUES(1, 1)`); err != nil {
		t.Fatal(err)
	}

	stats, err := Fetch(context.Background(), st, Options{
		Delay: 1,
		Searcher: fakeSearch{hits: []Hit{{
			WorkID:    1,
			RusName:   "Гиперион",
			Authors:   []string{"Дэн Симмонс"},
			Midmark:   8.64,
			MarkCount: 10528,
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Matched != 1 || stats.Looked != 1 {
		t.Fatalf("stats=%+v", stats)
	}
	books := []store.Book{{ID: 1, Title: "Гиперион"}}
	if err := st.AttachFantLab(books); err != nil {
		t.Fatal(err)
	}
	if books[0].FantLabVoters != 10528 {
		t.Fatalf("book=%+v", books[0])
	}
}
