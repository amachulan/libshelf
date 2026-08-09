package store

import "testing"

func TestFtsQueryCrossField(t *testing.T) {
	got := ftsQuery("кинг сияние")
	want := `"кинг"* AND "сияние"*`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFtsQueryEmpty(t *testing.T) {
	if ftsQuery("   ") != "" {
		t.Fatal("expected empty")
	}
}

func TestFtsMatchAuthorAndTitle(t *testing.T) {
	got := ftsMatch(SearchQuery{Author: "кинг", Title: "сияние"})
	want := `title: ("сияние"*) AND authors: ("кинг"*)`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFtsMatchGeneralAndAuthor(t *testing.T) {
	got := ftsMatch(SearchQuery{Q: "сияние", Author: "кинг"})
	want := `("сияние"*) AND authors: ("кинг"*)`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSearchQueryNormalizeYear(t *testing.T) {
	q := SearchQuery{YearFrom: 2026}
	q.NormalizeYear()
	if q.YearFrom != 2026 || q.YearTo != 2026 {
		t.Fatalf("exact: %+v", q)
	}
	q = SearchQuery{YearTo: 2020}
	q.NormalizeYear()
	if q.YearFrom != 1 || q.YearTo != 2020 {
		t.Fatalf("open from: %+v", q)
	}
	q = SearchQuery{YearFrom: 2020, YearTo: 2010}
	q.NormalizeYear()
	if q.YearFrom != 2010 || q.YearTo != 2020 {
		t.Fatalf("swap: %+v", q)
	}
}

func TestSearchQueryEmpty(t *testing.T) {
	if !(SearchQuery{}).Empty() {
		t.Fatal("expected empty")
	}
	if (SearchQuery{YearFrom: 2026}).Empty() {
		t.Fatal("year-only should not be empty")
	}
	if (SearchQuery{Author: "кинг"}).Empty() {
		t.Fatal("author should not be empty")
	}
}
