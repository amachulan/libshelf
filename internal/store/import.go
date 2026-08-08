package store

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"libshelf/internal/inpx"
)

type ImportStats struct {
	Books    int
	Authors  int
	Series   int
	Genres   int
	Duration time.Duration
}

const importBatchSize = 25_000

type authorKey struct {
	Last, First, Middle string
}

func (s *Store) ImportINPX(path string, replace bool) (ImportStats, error) {
	n, err := s.TotalBookCount()
	if err != nil {
		return ImportStats{}, err
	}
	if n > 0 {
		if !replace {
			return ImportStats{}, fmt.Errorf("library is not empty (%d books); use --replace to reimport", n)
		}
		if err := s.clearCatalog(); err != nil {
			return ImportStats{}, err
		}
	}

	f, err := inpx.Open(path)
	if err != nil {
		return ImportStats{}, err
	}
	defer f.Close()
	log.Printf("collection %q", f.CollectionName())

	for _, idx := range schemaIndexes {
		name := indexName(idx)
		if name != "" {
			_, _ = s.db.Exec(`DROP INDEX IF EXISTS ` + name)
		}
	}

	start := time.Now()
	im := &importSession{
		s:       s,
		folders: make(map[string]int64, 1024),
		series:  make(map[string]int64, 1<<16),
		authors: make(map[authorKey]int64, 1<<17),
		genres:  make(map[string]int64, 256),
		start:   start,
	}
	if err := im.begin(); err != nil {
		return ImportStats{}, err
	}

	err = f.Records(func(rec *inpx.Record) error {
		return im.add(rec)
	})
	if err != nil {
		im.abort()
		return ImportStats{}, err
	}
	stats, err := im.finish()
	if err != nil {
		return ImportStats{}, err
	}
	_, _ = s.db.Exec(`INSERT OR REPLACE INTO meta(key, value) VALUES('collection', ?)`, f.CollectionName())
	return stats, nil
}

func (s *Store) clearCatalog() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		`DELETE FROM book_search`,
		`DELETE FROM book_genres`,
		`DELETE FROM book_authors`,
		`DELETE FROM books`,
		`DELETE FROM genres`,
		`DELETE FROM authors`,
		`DELETE FROM series`,
		`DELETE FROM folders`,
	} {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type importSession struct {
	s       *Store
	tx      *sql.Tx
	stmts   importStmts
	batch   int
	start   time.Time
	folders map[string]int64
	series  map[string]int64
	authors map[authorKey]int64
	genres  map[string]int64
	stats   ImportStats
}

type importStmts struct {
	folder, series, author, genre, book, ba, bg *sql.Stmt
}

func (im *importSession) begin() error {
	tx, err := im.s.db.Begin()
	if err != nil {
		return err
	}
	im.tx = tx
	var prepErr error
	prep := func(q string) *sql.Stmt {
		if prepErr != nil {
			return nil
		}
		st, err := tx.Prepare(q)
		if err != nil {
			prepErr = err
			return nil
		}
		return st
	}
	im.stmts = importStmts{
		folder: prep(`INSERT INTO folders (name) VALUES (?)`),
		series: prep(`INSERT INTO series (title) VALUES (?)`),
		author: prep(`INSERT INTO authors (last_name, first_name, middle_name) VALUES (?, ?, ?)`),
		genre:  prep(`INSERT INTO genres (code) VALUES (?)`),
		book: prep(`INSERT INTO books (lib_id, title, series_id, series_num, folder_id, file, ext, size, lang, year, added, lib_rate, deleted)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		ba: prep(`INSERT OR IGNORE INTO book_authors (book_id, author_id) VALUES (?, ?)`),
		bg: prep(`INSERT OR IGNORE INTO book_genres (book_id, genre_id) VALUES (?, ?)`),
	}
	return prepErr
}

func (im *importSession) commitBatch() error {
	if err := im.tx.Commit(); err != nil {
		return err
	}
	im.batch = 0
	return im.begin()
}

func (im *importSession) abort() {
	if im.tx != nil {
		_ = im.tx.Rollback()
	}
}

func lookup(cache map[string]int64, key string, ins *sql.Stmt) (int64, error) {
	if id, ok := cache[key]; ok {
		return id, nil
	}
	res, err := ins.Exec(key)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	cache[key] = id
	return id, nil
}

func (im *importSession) add(rec *inpx.Record) error {
	folderID, err := lookup(im.folders, rec.Folder, im.stmts.folder)
	if err != nil {
		return err
	}
	var seriesID any
	if rec.Series != "" {
		id, err := lookup(im.series, rec.Series, im.stmts.series)
		if err != nil {
			return err
		}
		seriesID = id
	}
	var seriesNum any
	if rec.SeriesNum > 0 {
		seriesNum = rec.SeriesNum
	}
	var year any
	if rec.Year > 0 {
		year = rec.Year
	}
	ext := rec.Ext
	if ext == "" {
		ext = "fb2"
	}
	deleted := 0
	if rec.Deleted {
		deleted = 1
	}

	res, err := im.stmts.book.Exec(rec.LibID, rec.Title, seriesID, seriesNum, folderID,
		rec.File, ext, rec.Size, rec.Lang, year, rec.Date, rec.Rate, deleted)
	if err != nil {
		return fmt.Errorf("book %q: %w", rec.Title, err)
	}
	bookID, err := res.LastInsertId()
	if err != nil {
		return err
	}

	for _, a := range rec.Authors {
		key := authorKey{a.Last, a.First, a.Middle}
		id, ok := im.authors[key]
		if !ok {
			res, err := im.stmts.author.Exec(a.Last, a.First, a.Middle)
			if err != nil {
				return err
			}
			id, err = res.LastInsertId()
			if err != nil {
				return err
			}
			im.authors[key] = id
		}
		if _, err := im.stmts.ba.Exec(bookID, id); err != nil {
			return err
		}
	}
	for _, g := range rec.Genres {
		id, err := lookup(im.genres, g, im.stmts.genre)
		if err != nil {
			return err
		}
		if _, err := im.stmts.bg.Exec(bookID, id); err != nil {
			return err
		}
	}

	im.stats.Books++
	im.batch++
	if im.stats.Books%50_000 == 0 {
		log.Printf("importing books=%d", im.stats.Books)
	}
	if im.batch >= importBatchSize {
		return im.commitBatch()
	}
	return nil
}

func (im *importSession) finish() (ImportStats, error) {
	if err := im.tx.Commit(); err != nil {
		return im.stats, err
	}
	for _, idx := range schemaIndexes {
		if _, err := im.s.db.Exec(idx); err != nil {
			return im.stats, fmt.Errorf("index: %w", err)
		}
	}
	if _, err := im.s.db.Exec(`DELETE FROM book_search`); err != nil {
		return im.stats, err
	}
	// Index only russian non-deleted books for search.
	if _, err := im.s.db.Exec(`
INSERT INTO book_search (rowid, title, authors, series)
SELECT b.id,
       b.title,
       coalesce((SELECT group_concat(trim(a.last_name || ' ' || a.first_name || ' ' || a.middle_name), ' ')
                 FROM book_authors ba JOIN authors a ON a.id = ba.author_id
                 WHERE ba.book_id = b.id), ''),
       coalesce(s.title, '')
FROM books b
LEFT JOIN series s ON s.id = b.series_id
WHERE b.deleted = 0 AND b.lang = 'ru'`); err != nil {
		return im.stats, fmt.Errorf("search index: %w", err)
	}
	if _, err := im.s.db.Exec(`ANALYZE`); err != nil {
		return im.stats, err
	}
	im.stats.Authors = len(im.authors)
	im.stats.Series = len(im.series)
	im.stats.Genres = len(im.genres)
	im.stats.Duration = time.Since(im.start)
	return im.stats, nil
}

func indexName(createStmt string) string {
	fields := strings.Fields(createStmt)
	for i, f := range fields {
		if strings.EqualFold(f, "EXISTS") && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return ""
}
