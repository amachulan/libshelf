package store

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// INPX has LIBRATES (0–5) and no vote counts, so a raw 5.0 dump is just
// alphabet soup: quotes, ellipses, "Crime story № 1". Treat edition count as
// a stand-in for votes (more files ≈ more circulation) and break remaining
// ties with a title key that prefers Cyrillic over Latin/digits/junk.
const (
	popularPriorStrength = 4.0
	popularPriorMean     = 3.5
)

func popularScore(rate float64, editions int) float64 {
	if editions < 1 {
		editions = 1
	}
	v := float64(editions)
	return (rate*v + popularPriorStrength*popularPriorMean) / (v + popularPriorStrength)
}

func trimTitleDecor(s string) string {
	return strings.TrimLeftFunc(s, func(r rune) bool {
		if unicode.IsSpace(r) {
			return true
		}
		switch r {
		case '«', '»', '"', '\'', '`', '„', '“', '”', '…', '—', '–', '-', '·', '•',
			'.', ',', ';', ':', '*', '#', '(', '[', '{':
			return true
		}
		return false
	})
}

// popularTitleKey: 0|cyrillic, 1|latin, 2|digit, 3|other — then folded title.
func popularTitleKey(title string) string {
	s := foldYo(trimTitleDecor(strings.TrimSpace(title)))
	if s == "" {
		return "3|"
	}
	var b strings.Builder
	b.Grow(len(s) + 2)
	for _, r := range s {
		b.WriteRune(unicode.ToLower(r))
	}
	s = b.String()
	r, _ := utf8.DecodeRuneInString(s)
	bucket := byte('3')
	switch {
	case r >= 'а' && r <= 'я':
		bucket = '0'
	case r >= 'a' && r <= 'z':
		bucket = '1'
	case unicode.IsDigit(r):
		bucket = '2'
	}
	return string(bucket) + "|" + s
}

func sortWorksPopular(works []Book) {
	sort.SliceStable(works, func(i, j int) bool {
		si := popularScore(works[i].Rate, works[i].EditionCount)
		sj := popularScore(works[j].Rate, works[j].EditionCount)
		if si != sj {
			return si > sj
		}
		ki, kj := popularTitleKey(works[i].Title), popularTitleKey(works[j].Title)
		if ki != kj {
			return ki < kj
		}
		return works[i].ID < works[j].ID
	})
}
