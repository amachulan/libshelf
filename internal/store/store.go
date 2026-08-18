package store

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	_ "modernc.org/sqlite"
)

type Store struct {
	db         *sql.DB
	langs      []string // empty = all languages; default ["ru"]
	catCache   catalogCache
	genreCache genreListCache
}

func Open(path string) (*Store, error) {
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, langs: []string{"ru"}}
	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
	for _, idx := range schemaIndexes {
		if _, err := db.Exec(idx); err != nil {
			db.Close()
			return nil, fmt.Errorf("index: %w", err)
		}
	}
	return s, nil
}

func (s *Store) invalidateCatalogCache() {
	s.catCache.mu.Lock()
	s.catCache.authorLetters = nil
	s.catCache.seriesLetters = nil
	s.catCache.genres = nil
	s.catCache.mu.Unlock()

	s.genreCache.mu.Lock()
	s.genreCache.gen++
	s.genreCache.lists = nil
	s.genreCache.mu.Unlock()
}

// InvalidateCatalogCache drops in-memory catalog indexes and genre work lists.
func (s *Store) InvalidateCatalogCache() {
	s.invalidateCatalogCache()
}

// EnsureSearchIndexAsync rebuilds the FTS index in the background so serve can
// bind the HTTP port immediately (health checks must not wait on reindex).
func (s *Store) EnsureSearchIndexAsync() {
	go func() {
		if err := s.EnsureSearchIndex(); err != nil {
			log.Printf("search index: %v", err)
		}
	}()
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Exec(query string, args ...any) error {
	_, err := s.db.Exec(query, args...)
	return err
}

func (s *Store) BookCount() (int, error) {
	q := `SELECT count(*) FROM books WHERE deleted = 0`
	lang, args := s.langPred("lang")
	var n int
	err := s.db.QueryRow(q+lang, args...).Scan(&n)
	return n, err
}

func (s *Store) TotalBookCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT count(*) FROM books`).Scan(&n)
	return n, err
}

type Book struct {
	ID            int64   `json:"id"`
	Title         string  `json:"title"`
	Authors       string  `json:"authors"`
	Series        string  `json:"series"`
	SeriesNum     int     `json:"seriesNum"`
	Year          int     `json:"year"`
	Ext           string  `json:"ext"`
	Size          int64   `json:"size"`
	Lang          string  `json:"lang"`
	Rate          float64 `json:"rate"`
	CoverURL      string  `json:"coverUrl"`
	DownloadURL   string  `json:"downloadUrl"`
	EditionCount  int     `json:"editionCount,omitempty"`
	FantLabRate   float64 `json:"fantlabRate,omitempty"`
	FantLabVoters int     `json:"fantlabVoters,omitempty"`
	Progress      float64 `json:"progress,omitempty"`
}

type BookDetails struct {
	Book
	SeriesID    int64        `json:"seriesId,omitempty"`
	AuthorList  []AuthorRef  `json:"authorList"`
	GenreList   []GenreRef   `json:"genreList"`
	Genres      []string     `json:"genres"` // legacy: genre codes
	Annotation  string       `json:"annotation,omitempty"`
	Translators []string     `json:"translators,omitempty"`
	Publisher   string       `json:"publisher,omitempty"`
	City        string       `json:"city,omitempty"`
	PubYear     string       `json:"pubYear,omitempty"`
	ISBN        string       `json:"isbn,omitempty"`
	ShelfStatus string       `json:"shelfStatus,omitempty"`
	File        string       `json:"file"`
	Folder      string       `json:"folder"`
	Editions    []EditionRef `json:"editions,omitempty"`
}

type BookFile struct {
	ID     int64
	Title  string
	Folder string
	File   string
	Ext    string
	Size   int64
}

const bookColumns = `
 b.id,
 b.title,
 coalesce((SELECT group_concat(trim(a.last_name || ' ' || a.first_name), ', ')
           FROM book_authors ba JOIN authors a ON a.id = ba.author_id
           WHERE ba.book_id = b.id), ''),
 coalesce(s.title, ''),
 coalesce(b.series_num, 0),
 coalesce(b.year, 0),
 b.ext,
 b.size,
 b.lang,
 b.lib_rate`

func (s *Store) scanBook(sc interface{ Scan(dest ...any) error }) (Book, error) {
	var b Book
	err := sc.Scan(&b.ID, &b.Title, &b.Authors, &b.Series, &b.SeriesNum,
		&b.Year, &b.Ext, &b.Size, &b.Lang, &b.Rate)
	return b, err
}

func (s *Store) GetBook(id int64) (*BookDetails, error) {
	lang, langArgs := s.langPred("b.lang")
	args := append([]any{id}, langArgs...)
	row := s.db.QueryRow(`
SELECT`+bookColumns+`, b.file, f.name, coalesce(b.series_id, 0)
FROM books b
LEFT JOIN series s ON s.id = b.series_id
JOIN folders f ON f.id = b.folder_id
WHERE b.id = ? AND b.deleted = 0`+lang, args...)

	var d BookDetails
	err := row.Scan(&d.ID, &d.Title, &d.Authors, &d.Series, &d.SeriesNum,
		&d.Year, &d.Ext, &d.Size, &d.Lang, &d.Rate, &d.File, &d.Folder, &d.SeriesID)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	arows, err := s.db.Query(`
SELECT a.id, trim(a.last_name || ' ' || a.first_name || ' ' || a.middle_name)
FROM book_authors ba JOIN authors a ON a.id = ba.author_id
WHERE ba.book_id = ?
ORDER BY a.last_name, a.first_name`, id)
	if err != nil {
		return nil, err
	}
	defer arows.Close()
	for arows.Next() {
		var a AuthorRef
		if err := arows.Scan(&a.ID, &a.Name); err != nil {
			return nil, err
		}
		a.Name = strings.TrimSpace(a.Name)
		d.AuthorList = append(d.AuthorList, a)
	}
	if err := arows.Err(); err != nil {
		return nil, err
	}

	one := []Book{d.Book}
	if err := s.AttachFantLab(one); err != nil {
		return nil, err
	}
	d.Book = one[0]

	rows, err := s.db.Query(`
SELECT g.code FROM book_genres bg
JOIN genres g ON g.id = bg.genre_id
WHERE bg.book_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		d.Genres = append(d.Genres, code)
	}
	return &d, rows.Err()
}

func (s *Store) BookFile(id int64) (*BookFile, error) {
	f := &BookFile{}
	lang, langArgs := s.langPred("b.lang")
	args := append([]any{id}, langArgs...)
	err := s.db.QueryRow(`
SELECT b.id, b.title, fo.name, b.file, b.ext, b.size
FROM books b JOIN folders fo ON fo.id = b.folder_id
WHERE b.id = ? AND b.deleted = 0`+lang, args...).
		Scan(&f.ID, &f.Title, &f.Folder, &f.File, &f.Ext, &f.Size)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return f, err
}

var ErrNotFound = fmt.Errorf("not found")
