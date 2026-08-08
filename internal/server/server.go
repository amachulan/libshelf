package server

import (
	"bytes"
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
	"libshelf/internal/fb2"
	"libshelf/internal/store"
	"libshelf/web"
)

type Server struct {
	store    *store.Store
	libDir   string
	coverDir string
	mux      *http.ServeMux
	coverMu  sync.Mutex
}

func New(st *store.Store, libDir, coverDir string) *Server {
	s := &Server{
		store:    st,
		libDir:   libDir,
		coverDir: coverDir,
		mux:      http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) ListenAndServe(addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return srv.ListenAndServe()
}

func (s *Server) routes() {
	s.mux.HandleFunc("/api/search", s.handleSearch)
	s.mux.HandleFunc("/api/book/", s.handleBook)
	s.mux.HandleFunc("/api/stats", s.handleStats)
	s.mux.HandleFunc("/cover/", s.handleCover)
	s.mux.HandleFunc("/download/", s.handleDownload)
	s.mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	static, err := fs.Sub(web.FS, ".")
	if err != nil {
		log.Fatal(err)
	}
	fileServer := http.FileServer(http.FS(static))
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && !strings.Contains(strings.TrimPrefix(r.URL.Path, "/"), "/") {
			// try static file first
			name := strings.TrimPrefix(r.URL.Path, "/")
			if f, err := static.Open(name); err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		if r.URL.Path == "/" || !strings.Contains(r.URL.Path, ".") {
			http.ServeFileFS(w, r, static, "index.html")
			return
		}
		fileServer.ServeHTTP(w, r)
	})
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
	for i := range books {
		books[i].CoverURL = "/cover/" + strconv.FormatInt(books[i].ID, 10)
		books[i].DownloadURL = "/download/" + strconv.FormatInt(books[i].ID, 10)
	}
	writeJSON(w, map[string]any{"query": q, "books": books})
}

func (s *Server) handleBook(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(strings.TrimPrefix(r.URL.Path, "/api/book/"))
	if err != nil {
		httpError(w, err, 400)
		return
	}
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
	writeJSON(w, d)
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
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
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

func parseID(s string) (int64, error) {
	s = strings.Trim(s, "/")
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	return strconv.ParseInt(s, 10, 64)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func httpError(w http.ResponseWriter, err error, code int) {
	http.Error(w, err.Error(), code)
}
