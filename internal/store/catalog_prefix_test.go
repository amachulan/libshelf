package store

import (
	"strings"
	"testing"
)

func TestLikePrefixPatternsIncludesCyrillicCases(t *testing.T) {
	pats := likePrefixPatterns(" азимов ")
	joined := strings.Join(pats, " ")
	for _, want := range []string{"азимов%", "Азимов%", "АЗИМОВ%"} {
		found := false
		for _, p := range pats {
			if p == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing %q in %q", want, joined)
		}
	}
}

func TestLikeEscape(t *testing.T) {
	got := likeEscape(`a%_b\c`)
	want := `a\%\_b\\c`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNormalizeListSort(t *testing.T) {
	cases := map[string]string{
		"":        "popular",
		"POPULAR": "popular",
		"rate":    "popular",
		"new":     "new",
		"added":   "new",
		"title":   "title",
		"alpha":   "title",
		"nope":    "popular",
	}
	for in, want := range cases {
		if got := NormalizeListSort(in); got != want {
			t.Fatalf("%q: got %q want %q", in, got, want)
		}
	}
}
