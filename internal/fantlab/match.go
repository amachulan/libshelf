package fantlab

import (
	"strings"
	"unicode"

	"libshelf/internal/store"
)

const (
	statusOK        = store.FantLabOK
	statusNone      = store.FantLabNone
	statusAmbiguous = store.FantLabAmbiguous
)

type Match struct {
	Status string
	Hit    Hit
}

func SearchQuery(title string, authorLasts []string) string {
	t := strings.TrimSpace(store.FoldTitle(title))
	if t == "" {
		t = strings.TrimSpace(title)
	}
	if t == "" {
		return ""
	}
	// FoldTitle lowercases; FantLab search is fine with that. Prefer original
	// title text with decor stripped for readability of the query.
	cut := func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(`«»"'`+"`„“”…—–-·•.,;:*#([{)]}", r)
	}
	raw := strings.TrimSpace(strings.TrimRightFunc(strings.TrimLeftFunc(title, cut), cut))
	if raw == "" {
		raw = title
	}
	if len(authorLasts) > 0 && strings.TrimSpace(authorLasts[0]) != "" {
		return strings.TrimSpace(authorLasts[0] + " " + raw)
	}
	return raw
}

func PickMatch(title string, authorLasts []string, hits []Hit) Match {
	wantTitle := store.FoldTitle(title)
	if wantTitle == "" {
		return Match{Status: statusNone}
	}
	var good []Hit
	for _, h := range hits {
		if !titleMatches(wantTitle, h) {
			continue
		}
		if !authorMatches(authorLasts, h.Authors) {
			continue
		}
		good = append(good, h)
	}
	switch len(good) {
	case 0:
		return Match{Status: statusNone}
	case 1:
		return Match{Status: statusOK, Hit: good[0]}
	default:
		if hit, ok := pickNonCycle(good); ok {
			return Match{Status: statusOK, Hit: hit}
		}
		return Match{Status: statusAmbiguous}
	}
}

func isCycle(h Hit) bool {
	if strings.EqualFold(h.NameEng, "cycle") {
		return true
	}
	if h.NameShow == "цикл" {
		return true
	}
	return h.WorkType == 4
}

func pickNonCycle(hits []Hit) (Hit, bool) {
	var books []Hit
	for _, h := range hits {
		if !isCycle(h) {
			books = append(books, h)
		}
	}
	if len(books) == 1 {
		return books[0], true
	}
	return Hit{}, false
}

func titleMatches(want string, h Hit) bool {
	for _, cand := range []string{h.RusName, h.Name, h.AltName} {
		if store.FoldTitle(cand) == want {
			return true
		}
	}
	return false
}

func authorMatches(ours []string, theirs []string) bool {
	if len(ours) == 0 {
		return true
	}
	want := make(map[string]struct{}, len(ours))
	for _, n := range ours {
		n = store.FoldName(n)
		if n != "" {
			want[n] = struct{}{}
		}
	}
	if len(want) == 0 {
		return true
	}
	for _, raw := range theirs {
		for _, tok := range nameTokens(raw) {
			if _, ok := want[tok]; ok {
				return true
			}
		}
	}
	return false
}

func nameTokens(s string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || r == ',' || r == ';' || r == '/'
	}) {
		n := store.FoldName(part)
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}
