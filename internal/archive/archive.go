package archive

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/bodgit/sevenzip"
)

// OpenBook reads a book file from a Flibusta archive (zip or 7z).
// folder is the archive name (e.g. f.fb2-000001-025000.zip),
// file is the entry name without extension, ext is e.g. "fb2".
func OpenBook(libraryDir, folder, file, ext string) ([]byte, error) {
	return OpenBookDirs([]string{libraryDir}, folder, file, ext)
}

// OpenBookDirs looks for the archive in each library directory (first hit wins).
func OpenBookDirs(libraryDirs []string, folder, file, ext string) ([]byte, error) {
	if len(libraryDirs) == 0 {
		return nil, fmt.Errorf("no library directories configured")
	}
	entryName := file
	if ext != "" && !strings.HasSuffix(strings.ToLower(file), "."+strings.ToLower(ext)) {
		entryName = file + "." + ext
	}
	alt := strings.TrimSuffix(file, filepath.Ext(file)) + "." + ext

	var lastErr error
	for _, libraryDir := range libraryDirs {
		if strings.TrimSpace(libraryDir) == "" {
			continue
		}
		archivePath := filepath.Join(libraryDir, folder)
		data, err := readEntry(archivePath, entryName)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if alt != entryName {
			if data2, err2 := readEntry(archivePath, alt); err2 == nil {
				return data2, nil
			}
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("archive %s not found in library dirs", folder)
	}
	return nil, lastErr
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

// SafeFilename builds a download filename from title (UTF-8, OS-safe).
func SafeFilename(title, ext string) string {
	var b bytes.Buffer
	for _, r := range title {
		switch {
		case r < 32 || !unicode.IsPrint(r):
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
	runes := []rune(name)
	if len(runes) > 80 {
		name = string(runes[:80])
	}
	if ext == "" {
		ext = "fb2"
	}
	return name + "." + ext
}

// asciiFilename is a legacy-compatible fallback for old browsers.
func asciiFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r < 128 && r >= 32 && !strings.ContainsRune(`<>:"/\|?*`, r) {
			b.WriteRune(r)
		} else if r >= 128 {
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), " ._")
	if out == "" || strings.HasPrefix(out, ".") {
		out = "book.fb2"
	}
	return out
}

// ContentDisposition returns an RFC 6266 / 5987 header value so Cyrillic
// titles download with a correct filename in modern browsers.
func ContentDisposition(filename string) string {
	ascii := asciiFilename(filename)
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, ascii, rfc5987(filename))
}

func rfc5987(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '.' || c == '_' || c == '~' {
			b.WriteByte(c)
		} else {
			b.WriteString(fmt.Sprintf("%%%02X", c))
		}
	}
	return b.String()
}
