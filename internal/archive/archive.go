package archive

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bodgit/sevenzip"
)

// OpenBook reads a book file from a Flibusta archive (zip or 7z).
// folder is the archive name (e.g. f.fb2-000001-025000.zip),
// file is the entry name without extension, ext is e.g. "fb2".
func OpenBook(libraryDir, folder, file, ext string) ([]byte, error) {
	archivePath := filepath.Join(libraryDir, folder)
	entryName := file
	if ext != "" && !strings.HasSuffix(strings.ToLower(file), "."+strings.ToLower(ext)) {
		entryName = file + "." + ext
	}
	data, err := readEntry(archivePath, entryName)
	if err != nil {
		// Some catalogs store bare id without matching extension casing.
		alt := strings.TrimSuffix(file, filepath.Ext(file)) + "." + ext
		if alt != entryName {
			if data2, err2 := readEntry(archivePath, alt); err2 == nil {
				return data2, nil
			}
		}
		return nil, err
	}
	return data, nil
}

func readEntry(archivePath, entryName string) ([]byte, error) {
	if _, err := os.Stat(archivePath); err != nil {
		return nil, fmt.Errorf("archive %s: %w", archivePath, err)
	}
	ext := strings.ToLower(filepath.Ext(archivePath))
	switch ext {
	case ".zip":
		return readZipEntry(archivePath, entryName)
	case ".7z":
		return read7zEntry(archivePath, entryName)
	default:
		// Try zip first, then 7z.
		if data, err := readZipEntry(archivePath, entryName); err == nil {
			return data, nil
		}
		return read7zEntry(archivePath, entryName)
	}
}

func readZipEntry(path, entryName string) ([]byte, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	want := normalizeName(entryName)
	for _, f := range zr.File {
		if normalizeName(f.Name) == want || strings.EqualFold(filepath.Base(f.Name), entryName) {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("entry %q not found in %s", entryName, path)
}

func read7zEntry(path, entryName string) ([]byte, error) {
	r, err := sevenzip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	want := normalizeName(entryName)
	for _, f := range r.File {
		if normalizeName(f.Name) == want || strings.EqualFold(filepath.Base(f.Name), entryName) {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			return data, err
		}
	}
	return nil, fmt.Errorf("entry %q not found in %s", entryName, path)
}

func normalizeName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	return strings.ToLower(filepath.Base(name))
}

// SafeFilename builds a download filename from title.
func SafeFilename(title, ext string) string {
	var b bytes.Buffer
	for _, r := range title {
		switch {
		case r < 32:
			continue
		case strings.ContainsRune(`<>:"/\|?*`, r):
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	name := strings.TrimSpace(b.String())
	if name == "" {
		name = "book"
	}
	if len(name) > 120 {
		name = name[:120]
	}
	if ext == "" {
		ext = "fb2"
	}
	return name + "." + ext
}
