package store

import "testing"

func TestNormalizeTitle(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Оно", "оно"},
		{"ОНО....", "оно"},
		{"  Тёмная   башня  ", "темная башня"},
		{"Бегущий человек....", "бегущий человек"},
	}
	for _, c := range cases {
		if got := normalizeTitle(c.in); got != c.want {
			t.Fatalf("%q: got %q want %q", c.in, got, c.want)
		}
	}
}

func TestGroupBooksToWorks(t *testing.T) {
	books := []Book{
		{ID: 1, Title: "Оно", Size: 100, Year: 1993},
		{ID: 2, Title: "ОНО....", Size: 200, Year: 2011},
		{ID: 3, Title: "Сияние", Size: 50, Year: 1980},
	}
	auth := map[int64][]int64{1: {10}, 2: {10}, 3: {10}}
	got := groupBooksToWorks(books, auth)
	if len(got) != 2 {
		t.Fatalf("works=%d want 2: %+v", len(got), got)
	}
	if got[0].EditionCount != 2 || got[0].ID != 2 {
		t.Fatalf("first work: %+v", got[0])
	}
	if got[1].ID != 3 || got[1].EditionCount != 1 {
		t.Fatalf("second work: %+v", got[1])
	}
}

func TestGroupBooksToWorksKeepsBestRate(t *testing.T) {
	books := []Book{
		{ID: 1, Title: "Оно", Size: 100, Rate: 4.8},
		{ID: 2, Title: "Оно", Size: 300, Rate: 0},
	}
	auth := map[int64][]int64{1: {10}, 2: {10}}
	got := groupBooksToWorks(books, auth)
	if len(got) != 1 {
		t.Fatalf("works=%d", len(got))
	}
	if got[0].ID != 2 {
		t.Fatalf("want larger edition, got %+v", got[0])
	}
	if got[0].Rate != 4.8 {
		t.Fatalf("rate=%v want 4.8", got[0].Rate)
	}
}
