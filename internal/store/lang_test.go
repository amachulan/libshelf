package store

import "testing"

func TestNormalizeLanguages(t *testing.T) {
	if got := NormalizeLanguages([]string{"RU", "en", "ru"}); len(got) != 2 || got[0] != "ru" || got[1] != "en" {
		t.Fatalf("got %#v", got)
	}
	if got := NormalizeLanguages([]string{"*"}); got != nil {
		t.Fatalf("all marker: %#v", got)
	}
	if got := NormalizeLanguages([]string{"all"}); got != nil {
		t.Fatalf("all word: %#v", got)
	}
}

func TestLangPred(t *testing.T) {
	s := &Store{langs: []string{"ru"}}
	pred, args := s.langPred("b.lang")
	if pred != " AND b.lang = ?" || len(args) != 1 || args[0] != "ru" {
		t.Fatalf("%q %#v", pred, args)
	}
	s.langs = nil
	pred, args = s.langPred("b.lang")
	if pred != "" || args != nil {
		t.Fatalf("all langs: %q %#v", pred, args)
	}
	s.langs = []string{"ru", "en"}
	pred, args = s.langPred("lang")
	if pred != " AND lang IN (?,?)" || len(args) != 2 {
		t.Fatalf("%q %#v", pred, args)
	}
}
