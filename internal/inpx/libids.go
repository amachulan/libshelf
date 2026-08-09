package inpx

import "fmt"

// CollectLibIDs returns every LIBID from an .inpx catalog.
// If aliveOnly is true, records with DEL=1 are skipped.
func CollectLibIDs(path string, aliveOnly bool) (map[string]struct{}, error) {
	f, err := Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ids := make(map[string]struct{}, 1<<20)
	err = f.Records(func(rec *Record) error {
		if rec.LibID == "" {
			return nil
		}
		if aliveOnly && rec.Deleted {
			return nil
		}
		ids[rec.LibID] = struct{}{}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("collect lib ids: %w", err)
	}
	return ids, nil
}
