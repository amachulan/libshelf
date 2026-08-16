package store

import (
	"path/filepath"
	"testing"
)

func TestFantLabAttachAndPending(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "libshelf.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.db.Exec(`INSERT INTO folders(id, name) VALUES(1, 'f')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO authors(id, last_name, first_name) VALUES(1, 'Симмонс', 'Дэн')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO books(id, title, folder_id, file, lang) VALUES(1, 'Гиперион', 1, 'h.fb2', 'ru')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO book_authors(book_id, author_id) VALUES(1, 1)`); err != nil {
		t.Fatal(err)
	}

	pending, err := s.PendingFantLabWorks("", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Title != "Гиперион" {
		t.Fatalf("pending=%+v", pending)
	}
	if err := s.UpsertFantLab(FantLabRating{
		WorkKey: pending[0].Key,
		Status:  FantLabOK,
		Rating:  8.64,
		Voters:  10528,
	}); err != nil {
		t.Fatal(err)
	}
	pending, err = s.PendingFantLabWorks("", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("want no pending, got %+v", pending)
	}

	books := []Book{{ID: 1, Title: "Гиперион"}}
	if err := s.AttachFantLab(books); err != nil {
		t.Fatal(err)
	}
	if books[0].FantLabRate != 8.64 || books[0].FantLabVoters != 10528 {
		t.Fatalf("attached %+v", books[0])
	}
}
