package inpx

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type FilterStats struct {
	IncomingTotal int
	Kept          int
	Dropped       int
	SkippedNoID   int
	KeptFolders   map[string]struct{}
	// IncomingFolders lists archive names seen in the incoming catalog.
	IncomingFolders map[string]struct{}
}

// FilterINPX writes a new .inpx containing only records whose LIBID is absent from base.
// Base is typically collected from an older dump or from libshelf.db.
func FilterINPX(incomingPath, outPath string, base map[string]struct{}) (FilterStats, error) {
	in, err := Open(incomingPath)
	if err != nil {
		return FilterStats{}, err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return FilterStats{}, err
	}
	tmp := outPath + ".tmp"
	outFile, err := os.Create(tmp)
	if err != nil {
		return FilterStats{}, err
	}
	zw := zip.NewWriter(outFile)

	stats := FilterStats{
		KeptFolders:     make(map[string]struct{}),
		IncomingFolders: make(map[string]struct{}),
	}

	// Preserve metadata / non-inp entries from the incoming catalog.
	for _, entry := range in.zr.File {
		name := entry.Name
		if strings.EqualFold(path.Ext(name), ".inp") {
			continue
		}
		if err := copyZipEntry(zw, entry); err != nil {
			_ = zw.Close()
			_ = outFile.Close()
			_ = os.Remove(tmp)
			return FilterStats{}, fmt.Errorf("copy %s: %w", name, err)
		}
	}

	wroteInp := false
	for _, inp := range in.inps {
		kept, err := filterInpEntry(zw, inp, in.fields, base, &stats)
		if err != nil {
			_ = zw.Close()
			_ = outFile.Close()
			_ = os.Remove(tmp)
			return FilterStats{}, fmt.Errorf("%s: %w", inp.Name, err)
		}
		if kept {
			wroteInp = true
		}
	}
	if !wroteInp {
		// Keep the zip a valid .inpx even when everything was a duplicate.
		w, err := zw.Create("empty.inp")
		if err != nil {
			_ = zw.Close()
			_ = outFile.Close()
			_ = os.Remove(tmp)
			return FilterStats{}, err
		}
		_, _ = w.Write(nil)
	}

	// Ensure structure.info exists (some dumps omit it).
	if !zipHas(in.zr, structureEntry) {
		w, err := zw.Create(structureEntry)
		if err != nil {
			_ = zw.Close()
			_ = outFile.Close()
			_ = os.Remove(tmp)
			return FilterStats{}, err
		}
		if _, err := io.WriteString(w, strings.Join(in.fields, ";")+"\n"); err != nil {
			_ = zw.Close()
			_ = outFile.Close()
			_ = os.Remove(tmp)
			return FilterStats{}, err
		}
	}

	if err := zw.Close(); err != nil {
		_ = outFile.Close()
		_ = os.Remove(tmp)
		return FilterStats{}, err
	}
	if err := outFile.Close(); err != nil {
		_ = os.Remove(tmp)
		return FilterStats{}, err
	}
	if err := os.Rename(tmp, outPath); err != nil {
		_ = os.Remove(tmp)
		return FilterStats{}, err
	}
	return stats, nil
}

func zipHas(zr *zip.ReadCloser, name string) bool {
	for _, e := range zr.File {
		if strings.EqualFold(e.Name, name) {
			return true
		}
	}
	return false
}

func copyZipEntry(zw *zip.Writer, entry *zip.File) error {
	w, err := zw.Create(entry.Name)
	if err != nil {
		return err
	}
	rc, err := entry.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	_, err = io.Copy(w, rc)
	return err
}

func filterInpEntry(zw *zip.Writer, inp *zip.File, fields []string, base map[string]struct{}, stats *FilterStats) (keptAny bool, err error) {
	rc, err := inp.Open()
	if err != nil {
		return false, err
	}
	defer rc.Close()

	baseName := strings.TrimSuffix(path.Base(inp.Name), path.Ext(inp.Name))
	defaultFolder := baseName + ".zip"

	var buf strings.Builder
	r := bufio.NewReaderSize(rc, 1<<20)
	for {
		line, err := r.ReadString('\n')
		if line != "" {
			raw := strings.TrimRight(line, "\r\n")
			if raw != "" {
				stats.IncomingTotal++
				rec := parseLine(raw+"\n", fields, defaultFolder)
				if rec == nil || rec.LibID == "" {
					stats.SkippedNoID++
					// Keep malformed/no-id lines out of the cleaned catalog.
				} else {
					stats.IncomingFolders[rec.Folder] = struct{}{}
					if _, known := base[rec.LibID]; known {
						stats.Dropped++
					} else {
						stats.Kept++
						stats.KeptFolders[rec.Folder] = struct{}{}
						buf.WriteString(raw)
						buf.WriteByte('\n')
						keptAny = true
					}
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, err
		}
	}

	if !keptAny {
		return false, nil
	}
	w, err := zw.Create(inp.Name)
	if err != nil {
		return false, err
	}
	_, err = io.WriteString(w, buf.String())
	return true, err
}

// PruneArchives removes archive files under libraryDir that appear in incomingFolders
// but are not referenced by keptFolders. Only top-level files are considered.
// dryRun reports candidates without deleting.
func PruneArchives(libraryDir string, keptFolders, incomingFolders map[string]struct{}, dryRun bool) (removed []string, err error) {
	entries, err := os.ReadDir(libraryDir)
	if err != nil {
		return nil, err
	}
	keptBase := make(map[string]struct{}, len(keptFolders))
	for f := range keptFolders {
		keptBase[filepath.Base(f)] = struct{}{}
	}
	incomingBase := make(map[string]struct{}, len(incomingFolders))
	for f := range incomingFolders {
		incomingBase[filepath.Base(f)] = struct{}{}
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if _, inIncoming := incomingBase[name]; !inIncoming {
			continue
		}
		if _, keep := keptBase[name]; keep {
			continue
		}
		full := filepath.Join(libraryDir, name)
		removed = append(removed, full)
		if dryRun {
			continue
		}
		if err := os.Remove(full); err != nil {
			return removed, fmt.Errorf("remove %s: %w", full, err)
		}
	}
	return removed, nil
}
