package server

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"libshelf/internal/appconfig"
	"libshelf/internal/auth"
	"libshelf/internal/nativedialog"
	"libshelf/internal/store"
)

type setupPhase string

const (
	setupNeedConfig setupPhase = "setup"
	setupImporting  setupPhase = "importing"
	setupReady      setupPhase = "ready"
	setupError      setupPhase = "error"
)

type setupRuntime struct {
	mu            sync.Mutex
	phase         setupPhase
	message       string
	books         int
	adminUser     string
	adminPassword string
}

func (s *Server) initSetupRuntime() {
	if s.setup == nil {
		s.setup = &setupRuntime{phase: setupNeedConfig}
	}
}

func (s *Server) handleSetupAPI(w http.ResponseWriter, r *http.Request) {
	if !s.setupMode.Load() && (s.setup == nil || s.setup.phase != setupReady) {
		http.Error(w, "setup disabled", http.StatusNotFound)
		return
	}
	if !isLocalRequest(r) {
		http.Error(w, "setup is only available from this computer", http.StatusForbidden)
		return
	}
	s.initSetupRuntime()

	switch {
	case r.URL.Path == "/api/setup" && r.Method == http.MethodGet:
		s.writeSetupStatus(w)
	case r.URL.Path == "/api/setup" && r.Method == http.MethodPost:
		s.handleSetupStart(w, r)
	case r.URL.Path == "/api/setup/browse" && r.Method == http.MethodPost:
		s.handleSetupBrowse(w, r)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) writeSetupStatus(w http.ResponseWriter) {
	s.setup.mu.Lock()
	defer s.setup.mu.Unlock()
	cfg := s.config
	if cfg.Addr == "" {
		cfg = appconfig.Defaults()
	}
	writeJSON(w, map[string]any{
		"phase":         s.setup.phase,
		"message":       s.setup.message,
		"books":         s.setup.books,
		"adminUser":     s.setup.adminUser,
		"adminPassword": s.setup.adminPassword,
		"canBrowse":     nativedialog.Available(),
		"defaults": map[string]string{
			"addr":        cfg.Addr,
			"library_dir": cfg.LibraryDir,
			"data_dir":    cfg.DataDir,
			"inpx":        cfg.INPX,
			"auth":        cfg.Auth,
		},
	})
}

func (s *Server) handleSetupBrowse(w http.ResponseWriter, r *http.Request) {
	if !nativedialog.Available() {
		http.Error(w, "native dialogs unavailable", http.StatusNotImplemented)
		return
	}
	var req struct {
		Kind string `json:"kind"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	var path string
	var err error
	switch strings.ToLower(strings.TrimSpace(req.Kind)) {
	case "dir", "library", "data":
		path, err = nativedialog.Folder("Выберите папку")
	case "inpx", "file":
		path, err = nativedialog.File("Выберите файл каталога .inpx", "INPX catalog (*.inpx)|*.inpx|All files (*.*)|*.*")
	default:
		http.Error(w, "kind must be dir or inpx", http.StatusBadRequest)
		return
	}
	if err != nil {
		if errors.Is(err, nativedialog.ErrCanceled) {
			writeJSON(w, map[string]any{"canceled": true})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"path": path})
}

func (s *Server) handleSetupStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		INPX       string `json:"inpx"`
		LibraryDir string `json:"library_dir"`
		DataDir    string `json:"data_dir"`
		Auth       string `json:"auth"`
		Replace    bool   `json:"replace"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	req.INPX = strings.TrimSpace(req.INPX)
	req.LibraryDir = strings.TrimSpace(req.LibraryDir)
	req.DataDir = strings.TrimSpace(req.DataDir)
	req.Auth = strings.ToLower(strings.TrimSpace(req.Auth))
	if req.Auth == "" {
		req.Auth = "users"
	}
	if req.Auth != "users" && req.Auth != "none" {
		http.Error(w, "auth must be users or none", http.StatusBadRequest)
		return
	}
	if req.INPX == "" || req.LibraryDir == "" || req.DataDir == "" {
		http.Error(w, "inpx, library_dir and data_dir are required", http.StatusBadRequest)
		return
	}
	if st, err := os.Stat(req.INPX); err != nil || st.IsDir() {
		http.Error(w, "inpx file not found", http.StatusBadRequest)
		return
	}
	if st, err := os.Stat(req.LibraryDir); err != nil || !st.IsDir() {
		http.Error(w, "library_dir not found", http.StatusBadRequest)
		return
	}

	s.setup.mu.Lock()
	if s.setup.phase == setupImporting {
		s.setup.mu.Unlock()
		http.Error(w, "import already running", http.StatusConflict)
		return
	}
	s.setup.phase = setupImporting
	s.setup.message = "Импорт каталога… Это может занять несколько минут."
	s.setup.adminUser = ""
	s.setup.adminPassword = ""
	s.setup.mu.Unlock()

	writeJSON(w, map[string]any{"phase": setupImporting})
	go s.runSetupImport(req.INPX, req.LibraryDir, req.DataDir, req.Auth, req.Replace)
}

func (s *Server) runSetupImport(inpxPath, libraryDir, dataDir, authMode string, replace bool) {
	fail := func(msg string) {
		s.setup.mu.Lock()
		s.setup.phase = setupError
		s.setup.message = msg
		s.setup.mu.Unlock()
		log.Printf("setup failed: %s", msg)
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		fail(err.Error())
		return
	}
	coverDir := filepath.Join(dataDir, "covers")
	if err := os.MkdirAll(coverDir, 0o755); err != nil {
		fail(err.Error())
		return
	}

	s.mu.Lock()
	curData := s.config.DataDir
	curStore := s.store
	s.mu.Unlock()

	dbPath := filepath.Join(dataDir, "libshelf.db")
	st := curStore
	openedNew := false
	if st == nil || filepath.Clean(dataDir) != filepath.Clean(curData) {
		var err error
		st, err = store.Open(dbPath)
		if err != nil {
			fail(err.Error())
			return
		}
		openedNew = true
	}

	stats, err := st.ImportINPX(inpxPath, replace)
	if err != nil {
		if openedNew {
			_ = st.Close()
		}
		fail(err.Error())
		return
	}

	var auther *auth.Auth
	var adminUser, adminPass string
	if authMode == "users" {
		auther, err = auth.Open(filepath.Join(dataDir, "users.db"))
		if err != nil {
			if openedNew {
				_ = st.Close()
			}
			fail(err.Error())
			return
		}
		user, pass := auth.EnvBootstrap()
		u, generated, err := auther.BootstrapAdmin(user, pass)
		if err != nil {
			_ = auther.Close()
			if openedNew {
				_ = st.Close()
			}
			fail(err.Error())
			return
		}
		if u != nil {
			adminUser = u.Username
			if generated != "" {
				adminPass = generated
				log.Printf("created bootstrap admin %q password=%s (change it after login)", u.Username, generated)
			} else {
				log.Printf("created bootstrap admin %q from env", u.Username)
			}
		}
	}

	cfg := s.config
	cfg.INPX = inpxPath
	cfg.LibraryDir = libraryDir
	cfg.DataDir = dataDir
	cfg.Auth = authMode
	if cfg.Addr == "" {
		cfg.Addr = appconfig.Defaults().Addr
	}
	cfg.OpenBrowser = true
	if s.configPath != "" {
		if err := cfg.Save(s.configPath); err != nil {
			log.Printf("warning: could not save config: %v", err)
		}
	}

	s.mu.Lock()
	// Keep the previous store/auth open if main() owns them via defer;
	// only swap pointers so requests use the imported library.
	s.store = st
	s.auth = auther
	s.authRequired = authMode == "users"
	s.libDirs = normalizeLibDirs(nil, libraryDir)
	s.coverDir = coverDir
	s.config = cfg
	s.setupMode.Store(false)
	s.mu.Unlock()

	s.setup.mu.Lock()
	s.setup.phase = setupReady
	s.setup.message = "Готово"
	s.setup.books = stats.Books
	s.setup.adminUser = adminUser
	s.setup.adminPassword = adminPass
	s.setup.mu.Unlock()

	log.Printf("setup complete: books=%d authors=%d series=%d genres=%d in %s",
		stats.Books, stats.Authors, stats.Series, stats.Genres, stats.Duration)
}

func isLocalRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isSetupPublicPath(path string) bool {
	switch path {
	case "/setup.html", "/setup.js",
		"/api/setup", "/api/setup/browse",
		"/health",
		"/favicon.ico", "/favicon.svg",
		"/apple-touch-icon.png", "/icon-512.png",
		"/style.css", "/theme.js":
		return true
	default:
		return false
	}
}
