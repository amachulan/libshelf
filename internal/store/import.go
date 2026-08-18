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
	Skipped  int
	Authors  int
	Series   int
	Genres   int
	Duration time.Duration
}

type ImportOptions struct {
	Paths   []string
	Replace bool
	// Append skips records whose LIBID already exists (first / DB wins).
	Append bool
}

const importBatchSize = 25_000

type authorKey struct {
	Last, First, Middle string
}

func (s *Store) ImportINPX(path string, replace bool) (ImportStats, error) {
	return s.ImportCatalog(ImportOptions{Paths: []string{path}, Replace: replace})
}

func (s *Store) ImportCatalog(opts ImportOptions) (ImportStats, error) {
	if len(opts.Paths) == 0 {
		return ImportStats{}, fmt.Errorf("no inpx paths")
	}
	if opts.Replace && opts.Append {
		return ImportStats{}, fmt.Errorf("replace and append are mutually exclusive")
	}

	n, err := s.TotalBookCount()
	if err != nil {
		return ImportStats{}, err
	}
	if n > 0 && !opts.Replace && !opts.Append {
		return ImportStats{}, fmt.Errorf("library is not empty (%d books); use --replace or --append", n)
	}
	if opts.Replace {
		if err := s.clearCatalog(); err != nil {
			return ImportStats{}, err
		}
		n = 0
	}

	skip := map[string]struct{}{}
	if opts.Append && n > 0 {
		skip, err = s.LibIDs(false)
		if err != nil {
			return ImportStats{}, err
		}
		log.Printf("append mode: %d existing lib ids will be skipped", len(skip))
	}

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
		seen:    make(map[string]struct{}, 1<<20),
		skip:    skip,
		start:   start,
	}
	if n > 0 && opts.Append {
		if err := im.preloadLookups(); err != nil {
			return ImportStats{}, err
		}
	}
	if err := im.begin(); err != nil {
		return ImportStats{}, err
	}

	var lastName string
	for _, path := range opts.Paths {
		f, err := inpx.Open(path)
		if err != nil {
			im.abort()
			return ImportStats{}, err
		}
		log.Printf("importing %q (%s)", f.CollectionName(), path)
		err = f.Records(func(rec *inpx.Record) error {
			return im.add(rec)
		})
		name := f.CollectionName()
		_ = f.Close()
		if err != nil {
			im.abort()
			return ImportStats{}, err
		}
		if name != "" {
			lastName = name
		}
	}

	stats, err := im.finish()
	if err != nil {
		return ImportStats{}, err
	}
	if lastName != "" {
		_, _ = s.db.Exec(`INSERT OR REPLACE INTO meta(key, value) VALUES('collection', ?)`, lastName)
	}
	s.invalidateCatalogCache()
	return stats, nil
}

func (s *Store) clearCatalog() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		`DELETE FROM meta WHERE key = 'search_index_version'`,
		`DELETE FROM meta WHERE key = 'search_index_langs'`,
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
	if err := tx.Commit(); err != nil {
		return err
	}
	// Contentless FTS5 rejects DELETE — drop/recreate outside the catalog tx.
	return s.resetSearchTable()
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
	seen    map[string]struct{} // lib ids added in this import run
	skip    map[string]struct{} // preexisting lib ids (append)
	stats   ImportStats
}

type importStmts struct {
	folder, series, author, genre, book, ba, bg *sql.Stmt
}

func (im *importSession) preloadLookups() error {
	if err := scanIDMap(im.s.db, `SELECT id, name FROM folders`, im.folders); err != nil {
		return err
	}
	if err := scanIDMap(im.s.db, `SELECT id, title FROM series`, im.series); err != nil {
		return err
	}
	if err := scanIDMap(im.s.db, `SELECT id, code FROM genres`, im.genres); err != nil {
		return err
	}
	rows, err := im.s.db.Query(`SELECT id, last_name, first_name, middle_name FROM authors`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var key authorKey
		if err := rows.Scan(&id, &key.Last, &key.First, &key.Middle); err != nil {
			return err
		}
		im.authors[key] = id
	}
	return rows.Err()
}

func scanIDMap(db *sql.DB, q string, dst map[string]int64) error {
	rows, err := db.Query(q)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var key string
		if err := rows.Scan(&id, &key); err != nil {
			return err
		}
		dst[key] = id
	}
	return rows.Err()
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
	if rec.LibID != "" {
		if _, ok := im.skip[rec.LibID]; ok {
			im.stats.Skipped++
			return nil
		}
		if _, ok := im.seen[rec.LibID]; ok {
			im.stats.Skipped++
			return nil
		}
		im.seen[rec.LibID] = struct{}{}
	}

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
	if err := im.s.RebuildSearchIndex(); err != nil {
		return im.stats, fmt.Errorf("search index: %w", err)
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
