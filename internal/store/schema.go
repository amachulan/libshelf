package store

const schemaSQL = `
PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
PRAGMA foreign_keys=ON;

CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS folders (
  id   INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS series (
  id    INTEGER PRIMARY KEY,
  title TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS authors (
  id          INTEGER PRIMARY KEY,
  last_name   TEXT NOT NULL DEFAULT '',
  first_name  TEXT NOT NULL DEFAULT '',
  middle_name TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS genres (
  id   INTEGER PRIMARY KEY,
  code TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS books (
  id         INTEGER PRIMARY KEY,
  lib_id     TEXT NOT NULL DEFAULT '',
  title      TEXT NOT NULL,
  series_id  INTEGER REFERENCES series(id),
  series_num INTEGER,
  folder_id  INTEGER NOT NULL REFERENCES folders(id),
  file       TEXT NOT NULL,
  ext        TEXT NOT NULL DEFAULT 'fb2',
  size       INTEGER NOT NULL DEFAULT 0,
  lang       TEXT NOT NULL DEFAULT '',
  year       INTEGER,
  added      TEXT NOT NULL DEFAULT '',
  lib_rate   REAL NOT NULL DEFAULT 0,
  deleted    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS book_authors (
  book_id   INTEGER NOT NULL REFERENCES books(id),
  author_id INTEGER NOT NULL REFERENCES authors(id),
  PRIMARY KEY (book_id, author_id)
);

CREATE TABLE IF NOT EXISTS book_genres (
  book_id  INTEGER NOT NULL REFERENCES books(id),
  genre_id INTEGER NOT NULL REFERENCES genres(id),
  PRIMARY KEY (book_id, genre_id)
);

` + bookSearchDDL + `;
`

// bookSearchDDL creates the contentless FTS5 index. Contentless tables reject
// DELETE/UPDATE — wipe via DROP + recreate (see RebuildSearchIndex).
const bookSearchDDL = `CREATE VIRTUAL TABLE IF NOT EXISTS book_search USING fts5(
  title,
  authors,
  series,
  content='',
  tokenize='unicode61'
)`

var schemaIndexes = []string{
	`CREATE INDEX IF NOT EXISTS idx_books_lang_deleted ON books(lang, deleted)`,
	`CREATE INDEX IF NOT EXISTS idx_books_folder ON books(folder_id)`,
	`CREATE INDEX IF NOT EXISTS idx_books_series ON books(series_id)`,
	`CREATE INDEX IF NOT EXISTS idx_book_authors_author ON book_authors(author_id)`,
	`CREATE INDEX IF NOT EXISTS idx_book_genres_genre ON book_genres(genre_id)`,
	`CREATE INDEX IF NOT EXISTS idx_authors_name ON authors(last_name, first_name)`,
}
