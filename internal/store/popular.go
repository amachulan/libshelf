package store

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

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

// FantLab ratings are ~1–10. A lone 10.0 must not beat a well-voted 8.5.
const (
	fantlabPriorStrength = 20.0
	fantlabPriorMean     = 7.0
)

func fantlabScore(rate float64, voters int) float64 {
	if voters <= 0 || rate <= 0 {
		return 0
	}
	v := float64(voters)
	return (rate*v + fantlabPriorStrength*fantlabPriorMean) / (v + fantlabPriorStrength)
}

func sortWorksPopular(works []Book) {
	sort.SliceStable(works, func(i, j int) bool {
		si := fantlabScore(works[i].FantLabRate, works[i].FantLabVoters)
		sj := fantlabScore(works[j].FantLabRate, works[j].FantLabVoters)
		if si != sj {
			return si > sj
		}
		if works[i].FantLabVoters != works[j].FantLabVoters {
			return works[i].FantLabVoters > works[j].FantLabVoters
		}
		ki, kj := popularTitleKey(works[i].Title), popularTitleKey(works[j].Title)
		if ki != kj {
			return ki < kj
		}
		return works[i].ID < works[j].ID
	})
}
