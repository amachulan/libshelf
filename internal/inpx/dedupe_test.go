package inpx

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func writeTestINPX(t *testing.T, path string, lines []string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create(structureEntry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(defaultStructure + "\n")); err != nil {
		t.Fatal(err)
	}
	w, err = zw.Create("f.fb2-000001-000010.inp")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range lines {
		if _, err := w.Write([]byte(line + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func inpLine(libID, file, title string, deleted bool) string {
	del := "0"
	if deleted {
		del = "1"
	}
	// AUTHOR;GENRE;TITLE;SERIES;SERNO;FILE;SIZE;LIBID;DEL;EXT;DATE;LANG;LIBRATE;KEYWORDS;YEAR;SOURCELIB
	fields := []string{
		"Author,A,", "sf", title, "", "", file, "100", libID, del, "fb2", "2024-01-01", "ru", "0", "", "2020", "",
	}
	return joinFields(fields)
}

func joinFields(fields []string) string {
	out := fields[0]
	sep := string(fieldsSeparator)
	for i := 1; i < len(fields); i++ {
		out += sep + fields[i]
	}
	return out
}

func TestFilterINPX(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "base.inpx")
	inPath := filepath.Join(dir, "new.inpx")
	outPath := filepath.Join(dir, "out.inpx")

	writeTestINPX(t, basePath, []string{
		inpLine("1", "1", "Old One", false),
		inpLine("2", "2", "Old Two", true),
	})
	writeTestINPX(t, inPath, []string{
		inpLine("1", "1", "Dup One", false),
		inpLine("2", "2", "Dup Two", false),
		inpLine("3", "3", "Brand New", false),
	})

	base, err := CollectLibIDs(basePath, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(base) != 2 {
		t.Fatalf("base ids=%d", len(base))
	}

	stats, err := FilterINPX(inPath, outPath, base)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Kept != 1 || stats.Dropped != 2 {
		t.Fatalf("stats kept=%d dropped=%d total=%d", stats.Kept, stats.Dropped, stats.IncomingTotal)
	}

	kept, err := CollectLibIDs(outPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := kept["3"]; !ok || len(kept) != 1 {
		t.Fatalf("kept=%v", kept)
	}
}

func TestPruneArchives(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("keep.zip")
	mustWrite("drop.zip")
	mustWrite("other.zip")

	kept := map[string]struct{}{"keep.zip": {}}
	incoming := map[string]struct{}{"keep.zip": {}, "drop.zip": {}}
	removed, err := PruneArchives(dir, kept, incoming, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || filepath.Base(removed[0]) != "drop.zip" {
		t.Fatalf("removed=%v", removed)
	}
	if _, err := os.Stat(filepath.Join(dir, "drop.zip")); !os.IsNotExist(err) {
		t.Fatal("drop.zip should be gone")
	}
	if _, err := os.Stat(filepath.Join(dir, "other.zip")); err != nil {
		t.Fatal("other.zip must stay")
	}
}
