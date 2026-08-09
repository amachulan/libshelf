package store

import (
	"sort"
	"strings"
)

// langs empty means all languages are visible.
// Default before SetLanguages is ["ru"] (set in Open).

func NormalizeLanguages(langs []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, raw := range langs {
		l := strings.ToLower(strings.TrimSpace(raw))
		if l == "" {
			continue
		}
		if l == "*" || l == "all" {
			return nil // all languages
		}
		if _, ok := seen[l]; ok {
			continue
		}
		seen[l] = struct{}{}
		out = append(out, l)
	}
	return out
}

// SetLanguages configures which book languages are visible in the catalog.
// Pass nil/empty after NormalizeLanguages (e.g. ["*"]) for all languages.
func (s *Store) SetLanguages(langs []string) {
	s.langs = NormalizeLanguages(langs)
	s.invalidateCatalogCache()
}

func (s *Store) Languages() []string {
	return append([]string(nil), s.langs...)
}

func (s *Store) langsKey() string {
	if len(s.langs) == 0 {
		return "*"
	}
	cp := append([]string(nil), s.langs...)
	sort.Strings(cp)
	return strings.Join(cp, ",")
}

// langPred returns a SQL fragment starting with AND, and args for a language column.
func (s *Store) langPred(column string) (string, []any) {
	if len(s.langs) == 0 {
		return "", nil
	}
	if len(s.langs) == 1 {
		return " AND " + column + " = ?", []any{s.langs[0]}
	}
	ph := make([]string, len(s.langs))
	args := make([]any, len(s.langs))
	for i, l := range s.langs {
		ph[i] = "?"
		args[i] = l
	}
	return " AND " + column + " IN (" + strings.Join(ph, ",") + ")", args
}
