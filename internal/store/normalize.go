package store

import (
	"strings"
	"unicode"
)

const searchIndexVersion = "2" // ё→е + author name order variants

// foldYo maps ё/Ё to е/Е so search treats them as the same letter.
func foldYo(s string) string {
	if s == "" || !strings.ContainsAny(s, "ёЁ") {
		return s
	}
	return strings.Map(func(r rune) rune {
		switch r {
		case 'ё':
			return 'е'
		case 'Ё':
			return 'Е'
		default:
			return r
		}
	}, s)
}

// FoldTitle is the comparison form for titles (decor stripped, ё→е, lower).
func FoldTitle(s string) string {
	return normalizeTitle(trimTitleDecor(s))
}

// FoldName lowercases and folds ё for author last-name matching.
func FoldName(s string) string {
	return normalizeTitle(s)
}

// normalizeTitle folds ё/е, lowercases, trims trailing ellipsis/dots, collapses spaces.
func normalizeTitle(s string) string {
	s = foldYo(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		b.WriteRune(unicode.ToLower(r))
	}
	s = strings.TrimRight(b.String(), " .…·•")
	return strings.Join(strings.Fields(s), " ")
}

// authorSearchText builds FTS text for one author: "Last First Middle" and
// "First Last Middle" so both "кинг стивен" and "стивен кинг" match.
func authorSearchText(last, first, middle string) string {
	last, first, middle = foldYo(last), foldYo(first), foldYo(middle)
	a := strings.TrimSpace(last + " " + first + " " + middle)
	if first == "" || last == "" {
		return a
	}
	b := strings.TrimSpace(first + " " + last + " " + middle)
	if a == b {
		return a
	}
	return a + " " + b
}
