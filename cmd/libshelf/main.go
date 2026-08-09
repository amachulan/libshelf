package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"libshelf/internal/appconfig"
	"libshelf/internal/auth"
	"libshelf/internal/inpx"
	"libshelf/internal/server"
	"libshelf/internal/store"
	"libshelf/internal/version"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	if len(os.Args) < 2 {
		runStart(nil)
		return
	}
	switch os.Args[1] {
	case "start":
		runStart(os.Args[2:])
	case "import":
		runImport(os.Args[2:])
	case "dedupe":
		runDedupe(os.Args[2:])
	case "serve":
		runServe(os.Args[2:])
	case "user":
		runUser(os.Args[2:])
	case "version", "-version", "--version":
		fmt.Println(version.Commit)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `libshelf — personal INPX/FB2 library catalog

Usage:
  libshelf                 same as start (double-click friendly)
  libshelf start           [--config FILE] [--addr HOST:PORT] [--no-browser]
  libshelf import          --inpx FILE [--inpx FILE ...] --library-dir DIR --data-dir DIR [--replace|--append]
  libshelf dedupe          --incoming FILE --out FILE (--base FILE | --base-db DIR/FILE) [options]
  libshelf serve           --library-dir DIR [--library-dir DIR ...] --data-dir DIR [--addr HOST:PORT] [--auth users|none] [--open]
  libshelf user add        --data-dir DIR --username NAME --password PASS [--role admin|reader]
  libshelf version

Dedupe example (old library stays untouched; clean a newly obtained dump):
  libshelf dedupe \
    --base-db /opt/libshelf/data/libshelf.db \
    --incoming /data/books-new/catalog.inpx \
    --out /data/books-new/catalog.unique.inpx \
    --library-dir /data/books-new \
    --prune-empty-archives

Then append the cleaned catalog (archives may stay in the new folder):
  libshelf import --append --inpx /data/books-new/catalog.unique.inpx \
    --library-dir /opt/libshelf/library --data-dir /opt/libshelf/data
  # serve with both roots:
  libshelf serve --library-dir /opt/libshelf/library --library-dir /data/books-new \
    --data-dir /opt/libshelf/data

Windows: download libshelf-windows-amd64.exe, double-click it, finish setup in the browser.

`)
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func runStart(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	configPath := fs.String("config", "", "path to libshelf.json (default: next to the executable)")
	addrFlag := fs.String("addr", "", "override listen address")
	noBrowser := fs.Bool("no-browser", false, "do not open the browser")
	_ = fs.Parse(args)

	path := *configPath
	if path == "" {
		path = appconfig.DefaultPath()
	}
	cfg, err := appconfig.Load(path)
	if err != nil {
		log.Fatal(err)
	}
	if *addrFlag != "" {
		cfg.Addr = *addrFlag
	}
	if *noBrowser {
		cfg.OpenBrowser = false
	}

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatal(err)
	}
	coverDir := filepath.Join(cfg.DataDir, "covers")
	if err := os.MkdirAll(coverDir, 0o755); err != nil {
		log.Fatal(err)
	}

	dbPath := filepath.Join(cfg.DataDir, "libshelf.db")
	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()
	st.EnsureSearchIndexAsync()

	n, err := st.TotalBookCount()
	if err != nil {
		log.Fatal(err)
	}
	needSetup := n == 0

	var auther *auth.Auth
	authRequired := false
	if !needSetup {
		mode := strings.ToLower(strings.TrimSpace(cfg.Auth))
		if mode == "" {
			mode = "users"
		}
		if mode != "users" && mode != "none" {
			log.Fatal("config auth must be users or none")
		}
		authRequired = mode == "users"
		if authRequired {
			auther, err = auth.Open(filepath.Join(cfg.DataDir, "users.db"))
			if err != nil {
				log.Fatal(err)
			}
			defer auther.Close()
			user, pass := auth.EnvBootstrap()
			u, generated, err := auther.BootstrapAdmin(user, pass)
			if err != nil {
				log.Fatal(err)
			}
			if u != nil {
				if generated != "" {
					log.Printf("created bootstrap admin %q password=%s (change it after login)", u.Username, generated)
				} else {
					log.Printf("created bootstrap admin %q from env", u.Username)
				}
			}
		}
		if len(cfg.AllLibraryDirs()) == 0 {
			log.Fatal("library_dir is empty; edit libshelf.json or delete the database to run setup again")
		}
	}

	// Persist defaults so the next double-click finds the same paths.
	_ = cfg.Save(path)

	srv := server.New(server.Options{
		Store:        st,
		Auth:         auther,
		AuthRequired: authRequired,
		LibDirs:      cfg.AllLibraryDirs(),
		CoverDir:     coverDir,
		SetupMode:    needSetup,
		ConfigPath:   path,
		Config:       cfg,
	})

	url := "http://" + cfg.Addr + "/"
	if needSetup {
		url = "http://" + cfg.Addr + "/setup.html"
		log.Printf("first run: open setup wizard at %s", url)
		log.Printf("keep this window open while LibShelf is running")
	} else {
		log.Printf("listening on %s (%d books, auth=%s, commit=%s)", url, n, cfg.Auth, version.Short())
		log.Printf("keep this window open while LibShelf is running")
	}

	if cfg.OpenBrowser {
		go func() {
			time.Sleep(350 * time.Millisecond)
			if err := openBrowser(url); err != nil {
				log.Printf("could not open browser: %v (open %s manually)", err, url)
			}
		}()
	}

	if err := srv.ListenAndServe(cfg.Addr); err != nil {
		log.Fatal(err)
	}
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

func runImport(args []string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	var inpxPaths stringList
	fs.Var(&inpxPaths, "inpx", "path to .inpx catalog (repeatable)")
	libDir := fs.String("library-dir", "", "directory with book archives")
	dataDir := fs.String("data-dir", "", "directory for SQLite database")
	replace := fs.Bool("replace", false, "wipe catalog and reimport")
	appendMode := fs.Bool("append", false, "add only books whose LIBID is not already in the database")
	_ = fs.Parse(args)
	if len(inpxPaths) == 0 || *libDir == "" || *dataDir == "" {
		fs.Usage()
		os.Exit(2)
	}
	if *replace && *appendMode {
		log.Fatal("--replace and --append cannot be used together")
	}
	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatal(err)
	}
	dbPath := filepath.Join(*dataDir, "libshelf.db")
	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()
	stats, err := st.ImportCatalog(store.ImportOptions{
		Paths:   []string(inpxPaths),
		Replace: *replace,
		Append:  *appendMode,
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("import done: books=%d skipped=%d authors=%d series=%d genres=%d in %s",
		stats.Books, stats.Skipped, stats.Authors, stats.Series, stats.Genres, stats.Duration)
}

func runDedupe(args []string) {
	fs := flag.NewFlagSet("dedupe", flag.ExitOnError)
	baseINPX := fs.String("base", "", "reference .inpx (older dump; left untouched)")
	baseDB := fs.String("base-db", "", "reference libshelf.db or its data-dir (preferred if library already imported)")
	incoming := fs.String("incoming", "", "newly downloaded .inpx to clean")
	out := fs.String("out", "", "output cleaned .inpx")
	libDir := fs.String("library-dir", "", "directory of the NEW dump archives (for prune)")
	prune := fs.Bool("prune-empty-archives", false, "delete NEW archives no longer referenced after dedupe")
	dryRun := fs.Bool("dry-run", false, "with --prune-empty-archives: only print what would be removed")
	aliveOnly := fs.Bool("base-alive-only", false, "ignore deleted=1 rows when building the base LIBID set")
	_ = fs.Parse(args)

	if *incoming == "" || *out == "" || (*baseINPX == "" && *baseDB == "") {
		fs.Usage()
		os.Exit(2)
	}
	if *prune && *libDir == "" {
		log.Fatal("--prune-empty-archives requires --library-dir")
	}

	base := map[string]struct{}{}
	if *baseDB != "" {
		dbPath := *baseDB
		if st, err := os.Stat(dbPath); err == nil && st.IsDir() {
			dbPath = filepath.Join(dbPath, "libshelf.db")
		}
		st, err := store.Open(dbPath)
		if err != nil {
			log.Fatal(err)
		}
		ids, err := st.LibIDs(*aliveOnly)
		_ = st.Close()
		if err != nil {
			log.Fatal(err)
		}
		base = ids
		log.Printf("base-db: %d lib ids from %s", len(base), dbPath)
	}
	if *baseINPX != "" {
		ids, err := inpx.CollectLibIDs(*baseINPX, *aliveOnly)
		if err != nil {
			log.Fatal(err)
		}
		for id := range ids {
			base[id] = struct{}{}
		}
		log.Printf("base inpx: %d lib ids from %s (union size now %d)", len(ids), *baseINPX, len(base))
	}

	stats, err := inpx.FilterINPX(*incoming, *out, base)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("dedupe: incoming=%d kept=%d dropped=%d no_libid=%d -> %s",
		stats.IncomingTotal, stats.Kept, stats.Dropped, stats.SkippedNoID, *out)
	log.Printf("folders: incoming=%d kept=%d", len(stats.IncomingFolders), len(stats.KeptFolders))

	if *prune {
		removed, err := inpx.PruneArchives(*libDir, stats.KeptFolders, stats.IncomingFolders, *dryRun)
		if err != nil {
			log.Fatal(err)
		}
		if *dryRun {
			log.Printf("dry-run prune: %d archives would be removed", len(removed))
		} else {
			log.Printf("pruned %d archives from %s", len(removed), *libDir)
		}
		for _, p := range removed {
			log.Printf("  %s", p)
		}
	}
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	var libDirs stringList
	fs.Var(&libDirs, "library-dir", "directory with book archives (repeatable)")
	dataDir := fs.String("data-dir", "", "directory for SQLite database and cover cache")
	addr := fs.String("addr", "127.0.0.1:12380", "listen address")
	authMode := fs.String("auth", "users", "auth mode: users (login required) or none")
	open := fs.Bool("open", false, "open the library in a browser")
	_ = fs.Parse(args)
	if len(libDirs) == 0 || *dataDir == "" {
		fs.Usage()
		os.Exit(2)
	}
	mode := strings.ToLower(strings.TrimSpace(*authMode))
	if mode != "users" && mode != "none" {
		log.Fatal("--auth must be users or none")
	}

	dbPath := filepath.Join(*dataDir, "libshelf.db")
	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()
	st.EnsureSearchIndexAsync()
	n, err := st.BookCount()
	if err != nil {
		log.Fatal(err)
	}
	if n == 0 {
		log.Fatal("database is empty; run: libshelf start   or   libshelf import ...")
	}
	coverDir := filepath.Join(*dataDir, "covers")
	if err := os.MkdirAll(coverDir, 0o755); err != nil {
		log.Fatal(err)
	}

	var auther *auth.Auth
	authRequired := mode == "users"
	if authRequired {
		auther, err = auth.Open(filepath.Join(*dataDir, "users.db"))
		if err != nil {
			log.Fatal(err)
		}
		defer auther.Close()
		user, pass := auth.EnvBootstrap()
		u, generated, err := auther.BootstrapAdmin(user, pass)
		if err != nil {
			log.Fatal(err)
		}
		if u != nil {
			if generated != "" {
				log.Printf("created bootstrap admin %q password=%s (change it after login)", u.Username, generated)
			} else {
				log.Printf("created bootstrap admin %q from env", u.Username)
			}
		}
	}

	srv := server.New(server.Options{
		Store:        st,
		Auth:         auther,
		AuthRequired: authRequired,
		LibDirs:      []string(libDirs),
		CoverDir:     coverDir,
	})
	url := "http://" + *addr + "/"
	log.Printf("listening on %s (%d books, auth=%s, lib_dirs=%d, commit=%s)",
		url, n, mode, len(libDirs), version.Short())
	if *open {
		go func() {
			time.Sleep(350 * time.Millisecond)
			if err := openBrowser(url); err != nil {
				log.Printf("could not open browser: %v", err)
			}
		}()
	}
	if err := srv.ListenAndServe(*addr); err != nil {
		log.Fatal(err)
	}
}

func runUser(args []string) {
	if len(args) < 1 || args[0] != "add" {
		fmt.Fprintf(os.Stderr, "usage: libshelf user add --data-dir DIR --username NAME --password PASS [--role admin|reader]\n")
		os.Exit(2)
	}
	fs := flag.NewFlagSet("user add", flag.ExitOnError)
	dataDir := fs.String("data-dir", "", "data directory")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	role := fs.String("role", auth.RoleReader, "admin or reader")
	_ = fs.Parse(args[1:])
	if *dataDir == "" || *username == "" || *password == "" {
		fs.Usage()
		os.Exit(2)
	}
	a, err := auth.Open(filepath.Join(*dataDir, "users.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()
	u, err := a.CreateUser(*username, *password, *role)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("created user %q role=%s id=%d", u.Username, u.Role, u.ID)
}
