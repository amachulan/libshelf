package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"libshelf/internal/auth"
	"libshelf/internal/server"
	"libshelf/internal/store"
	"libshelf/internal/version"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "import":
		runImport(os.Args[2:])
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
	fmt.Fprintf(os.Stderr, `libshelf — personal Flibusta catalog

Usage:
  libshelf import --inpx FILE --library-dir DIR --data-dir DIR [--replace]
  libshelf serve  --library-dir DIR --data-dir DIR [--addr HOST:PORT] [--auth users|none]
  libshelf user add --data-dir DIR --username NAME --password PASS [--role admin|reader]
  libshelf version

`)
}

func runImport(args []string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	inpxPath := fs.String("inpx", "", "path to .inpx catalog")
	libDir := fs.String("library-dir", "", "directory with book archives")
	dataDir := fs.String("data-dir", "", "directory for SQLite database")
	replace := fs.Bool("replace", false, "reimport even if database is not empty")
	_ = fs.Parse(args)
	if *inpxPath == "" || *libDir == "" || *dataDir == "" {
		fs.Usage()
		os.Exit(2)
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
	stats, err := st.ImportINPX(*inpxPath, *replace)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("import done: books=%d authors=%d series=%d genres=%d in %s",
		stats.Books, stats.Authors, stats.Series, stats.Genres, stats.Duration)
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	libDir := fs.String("library-dir", "", "directory with book archives")
	dataDir := fs.String("data-dir", "", "directory for SQLite database and cover cache")
	addr := fs.String("addr", "127.0.0.1:12380", "listen address")
	authMode := fs.String("auth", "users", "auth mode: users (login required) or none")
	_ = fs.Parse(args)
	if *libDir == "" || *dataDir == "" {
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
	n, err := st.BookCount()
	if err != nil {
		log.Fatal(err)
	}
	if n == 0 {
		log.Fatal("database is empty; run: libshelf import ...")
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
		LibDir:       *libDir,
		CoverDir:     coverDir,
	})
	log.Printf("listening on http://%s (%d books, auth=%s, commit=%s)", *addr, n, mode, version.Short())
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
