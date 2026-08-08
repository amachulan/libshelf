package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"libshelf/internal/server"
	"libshelf/internal/store"
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
  libshelf serve  --library-dir DIR --data-dir DIR [--addr HOST:PORT]

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
	_ = fs.Parse(args)
	if *libDir == "" || *dataDir == "" {
		fs.Usage()
		os.Exit(2)
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
	srv := server.New(st, *libDir, coverDir)
	log.Printf("listening on http://%s (%d books)", *addr, n)
	if err := srv.ListenAndServe(*addr); err != nil {
		log.Fatal(err)
	}
}
