package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"libshelf/internal/archive"
	"libshelf/internal/auth"
	"libshelf/internal/fb2"
	"libshelf/internal/genres"
	"libshelf/internal/store"
	"libshelf/internal/version"
	"libshelf/web"
)

type Server struct {
	store        *store.Store
	auth         *auth.Auth
	authRequired bool
	libDir       string
	coverDir     string
	assetVer     string
	mux          *http.ServeMux
	coverMu      sync.Mutex
}

type Options struct {
	Store        *store.Store
	Auth         *auth.Auth
	AuthRequired bool
	LibDir       string
	CoverDir     string
}

func New(opts Options) *Server {
	s := &Server{
		store:        opts.Store,
		auth:         opts.Auth,
		authRequired: opts.AuthRequired,
		libDir:       opts.LibDir,
		coverDir:     opts.CoverDir,
		assetVer:     staticAssetVersion(web.FS),
		mux:          http.NewServeMux(),
	}
	s.routes()
	return s
}

func staticAssetVersion(fsys fs.FS) string {
	h := sha256.New()
	for _, name := range []string{
		"index.html", "login.html",
		"style.css", "app.js", "login.js", "theme.js",
	} {
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			continue
		}
		_, _ = h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

func (s *Server) Handler() http.Handler {
	return s.authMiddleware(s.mux)
}

func (s *Server) ListenAndServe(addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return srv.ListenAndServe()
}

func (s *Server) routes() {
	s.mux.HandleFunc("/api/login", s.handleLogin)
	s.mux.HandleFunc("/api/logout", s.handleLogout)
	s.mux.HandleFunc("/api/me", s.handleMe)
	s.mux.HandleFunc("/api/users", s.handleUsers)
	s.mux.HandleFunc("/api/users/", s.handleUsers)
	s.mux.HandleFunc("/api/search", s.handleSearch)
	s.mux.HandleFunc("/api/book/", s.handleBook)
	s.mux.HandleFunc("/api/shelf", s.handleShelf)
	s.mux.HandleFunc("/api/shelf/", s.handleShelf)
	s.mux.HandleFunc("/api/author/", s.handleAuthor)
	s.mux.HandleFunc("/api/series/", s.handleSeries)
	s.mux.HandleFunc("/api/catalog", s.handleCatalog)
	s.mux.HandleFunc("/api/catalog/", s.handleCatalog)
	s.mux.HandleFunc("/api/stats", s.handleStats)
	s.mux.HandleFunc("/opds", s.handleOPDS)
	s.mux.HandleFunc("/opds/", s.handleOPDS)
	s.mux.HandleFunc("/cover/", s.handleCover)
	s.mux.HandleFunc("/download/", s.handleDownload)
	s.mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok " + version.Commit + "\n"))
	})
	s.mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	static, err := fs.Sub(web.FS, ".")
	if err != nil {
		log.Fatal(err)
	}
	fileServer := http.FileServer(http.FS(static))
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch path {
		case "/", "/index.html":
			s.serveHTML(w, static, "index.html")
			return
		case "/login.html":
			s.serveHTML(w, static, "login.html")
			return
		}
		if path != "/" && !strings.Contains(strings.TrimPrefix(path, "/"), "/") {
			name := strings.TrimPrefix(path, "/")
			if f, err := static.Open(name); err == nil {
				f.Close()
				// Fingerprinted URLs (?v=...) can be cached hard; bare ones revalidate.
				if r.URL.RawQuery != "" {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		if path == "/" || !strings.Contains(path, ".") {
			s.serveHTML(w, static, "index.html")
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func (s *Server) serveHTML(w http.ResponseWriter, fsys fs.FS, name string) {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	v := s.assetVer
	html := string(b)
	for _, asset := range []string{"/style.css", "/app.js", "/login.js", "/theme.js"} {
		html = strings.ReplaceAll(html, `"`+asset+`"`, `"`+asset+`?v=`+v+`"`)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(html))
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.BookCount()
	if err != nil {
		httpError(w, err, 500)
		return
	}
	writeJSON(w, map[string]any{"books": n})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	books, err := s.store.Search(q, limit)
	if err != nil {
		httpError(w, err, 500)
		return
	}
	decorateBooks(books)
	writeJSON(w, map[string]any{"query": q, "books": books})
}

func (s *Server) handleBook(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/book/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		httpError(w, err, 400)
		return
	}
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleBookDetails(w, r, id)
		return
	}
	switch parts[1] {
	case "read":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleBookRead(w, r, id)
	case "progress":
		if r.Method != http.MethodPut {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleBookProgress(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleBookDetails(w http.ResponseWriter, r *http.Request, id int64) {
	d, err := s.store.GetBook(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpError(w, err, 404)
			return
		}
		httpError(w, err, 500)
		return
	}
	d.CoverURL = "/cover/" + strconv.FormatInt(d.ID, 10)
	d.DownloadURL = "/download/" + strconv.FormatInt(d.ID, 10)
	for _, code := range d.Genres {
		d.GenreList = append(d.GenreList, store.GenreRef{
			Code: code,
			Name: genres.Name(code),
		})
	}
	if meta, err := s.bookMeta(id); err == nil && meta != nil {
		d.Annotation = meta.Annotation
		d.Translators = meta.Translators
		d.Publisher = meta.Publisher
		d.City = meta.City
		d.PubYear = meta.Year
		d.ISBN = meta.ISBN
	}
	if u := userFrom(r.Context()); u != nil && s.auth != nil {
		if entry, err := s.auth.GetShelfEntry(u.ID, id); err == nil {
			d.ShelfStatus = entry.Status
			d.Progress = entry.Progress
		}
	}
	writeJSON(w, d)
}

func (s *Server) handleBookRead(w http.ResponseWriter, r *http.Request, id int64) {
	doc, err := s.readerDoc(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpError(w, err, 404)
			return
		}
		httpError(w, err, 404)
		return
	}
	out := map[string]any{
		"id":       id,
		"title":    doc.Title,
		"html":     doc.HTML,
		"chapters": doc.Chapters,
	}
	if u := userFrom(r.Context()); u != nil && s.auth != nil {
		_ = s.auth.MarkReading(u.ID, id)
		if pos, err := s.auth.GetProgress(u.ID, id); err == nil {
			out["position"] = pos
		}
		if entry, err := s.auth.GetShelfEntry(u.ID, id); err == nil {
			out["shelfStatus"] = entry.Status
		}
	}
	writeJSON(w, out)
}

func (s *Server) handleBookProgress(w http.ResponseWriter, r *http.Request, id int64) {
	if s.auth == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	var body struct {
		Position float64 `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if _, err := s.store.BookFile(id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpError(w, err, 404)
			return
		}
		httpError(w, err, 500)
		return
	}
	if err := s.auth.SetProgress(u.ID, id, body.Position); err != nil {
		httpError(w, err, 500)
		return
	}
	_ = s.auth.MarkReading(u.ID, id)
	writeJSON(w, map[string]any{"position": body.Position})
}

func (s *Server) readerDoc(id int64) (*fb2.ReaderDoc, error) {
	cachePath := filepath.Join(s.coverDir, strconv.FormatInt(id, 10)+".read.json")
	if data, err := os.ReadFile(cachePath); err == nil {
		var doc fb2.ReaderDoc
		if json.Unmarshal(data, &doc) == nil && doc.HTML != "" {
			return &doc, nil
		}
	}
	bf, err := s.store.BookFile(id)
	if err != nil {
		return nil, err
	}
	raw, err := archive.OpenBook(s.libDir, bf.Folder, bf.File, bf.Ext)
	if err != nil {
		return nil, err
	}
	doc, err := fb2.ExtractReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	if data, err := json.Marshal(doc); err == nil {
		s.coverMu.Lock()
		_ = os.WriteFile(cachePath, data, 0o644)
		s.coverMu.Unlock()
	}
	return doc, nil
}

func (s *Server) handleAuthor(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(strings.TrimPrefix(r.URL.Path, "/api/author/"))
	if err != nil {
		httpError(w, err, 400)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	list, err := s.store.AuthorBooks(id, limit, offset)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpError(w, err, 404)
			return
		}
		httpError(w, err, 500)
		return
	}
	decorateBooks(list.Books)
	writeJSON(w, list)
}

func (s *Server) handleSeries(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(strings.TrimPrefix(r.URL.Path, "/api/series/"))
	if err != nil {
		httpError(w, err, 400)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	list, err := s.store.SeriesBooks(id, limit, offset)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpError(w, err, 404)
			return
		}
		httpError(w, err, 500)
		return
	}
	decorateBooks(list.Books)
	writeJSON(w, list)
}

func decorateBooks(books []store.Book) {
	for i := range books {
		books[i].CoverURL = "/cover/" + strconv.FormatInt(books[i].ID, 10)
		books[i].DownloadURL = "/download/" + strconv.FormatInt(books[i].ID, 10)
	}
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(strings.TrimPrefix(r.URL.Path, "/download/"))
	if err != nil {
		httpError(w, err, 400)
		return
	}
	bf, err := s.store.BookFile(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpError(w, err, 404)
			return
		}
		httpError(w, err, 500)
		return
	}
	data, err := archive.OpenBook(s.libDir, bf.Folder, bf.File, bf.Ext)
	if err != nil {
		httpError(w, err, 404)
		return
	}
	name := archive.SafeFilename(bf.Title, bf.Ext)
	w.Header().Set("Content-Type", "application/fb2+xml")
	w.Header().Set("Content-Disposition", archive.ContentDisposition(name))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

func (s *Server) handleCover(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(strings.TrimPrefix(r.URL.Path, "/cover/"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	cachePath := filepath.Join(s.coverDir, strconv.FormatInt(id, 10)+".img")
	metaPath := cachePath + ".mime"

	if data, err := os.ReadFile(cachePath); err == nil {
		mime := "image/jpeg"
		if m, err := os.ReadFile(metaPath); err == nil && len(m) > 0 {
			mime = string(m)
		}
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(data)
		return
	}

	cover, err := s.extractCover(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.coverMu.Lock()
	_ = os.WriteFile(cachePath, cover.Data, 0o644)
	_ = os.WriteFile(metaPath, []byte(cover.Mime), 0o644)
	s.coverMu.Unlock()

	w.Header().Set("Content-Type", cover.Mime)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(cover.Data)
}

func (s *Server) extractCover(id int64) (*fb2.Cover, error) {
	bf, err := s.store.BookFile(id)
	if err != nil {
		return nil, err
	}
	data, err := archive.OpenBook(s.libDir, bf.Folder, bf.File, bf.Ext)
	if err != nil {
		return nil, err
	}
	return fb2.ExtractCover(bytes.NewReader(data))
}

func (s *Server) bookMeta(id int64) (*fb2.Meta, error) {
	cachePath := filepath.Join(s.coverDir, strconv.FormatInt(id, 10)+".meta.json")
	if data, err := os.ReadFile(cachePath); err == nil {
		var meta fb2.Meta
		if json.Unmarshal(data, &meta) == nil {
			return &meta, nil
		}
	}
	bf, err := s.store.BookFile(id)
	if err != nil {
		return nil, err
	}
	raw, err := archive.OpenBook(s.libDir, bf.Folder, bf.File, bf.Ext)
	if err != nil {
		return nil, err
	}
	meta, err := fb2.ExtractMeta(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	if data, err := json.Marshal(meta); err == nil {
		s.coverMu.Lock()
		_ = os.WriteFile(cachePath, data, 0o644)
		s.coverMu.Unlock()
	}
	return meta, nil
}

func parseID(s string) (int64, error) {
	s = strings.Trim(s, "/")
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	return strconv.ParseInt(s, 10, 64)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Vary", "Cookie")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func httpError(w http.ResponseWriter, err error, code int) {
	http.Error(w, err.Error(), code)
}
