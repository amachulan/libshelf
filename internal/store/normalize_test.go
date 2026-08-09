package store

import (
	"strings"
	"testing"
)

func TestFoldYo(t *testing.T) {
	if got := foldYo("Головачёв"); got != "Головачев" {
		t.Fatalf("got %q", got)
	}
	if got := foldYo("Ёлка"); got != "Елка" {
		t.Fatalf("got %q", got)
	}
	if got := foldYo("King"); got != "King" {
		t.Fatalf("got %q", got)
	}
}

func TestAuthorSearchTextOrders(t *testing.T) {
	got := authorSearchText("Кинг", "Стивен", "")
	if !strings.Contains(got, "Кинг Стивен") || !strings.Contains(got, "Стивен Кинг") {
		t.Fatalf("got %q", got)
	}
}

func TestFtsQueryFoldsYo(t *testing.T) {
	got := ftsQuery("Головачёв")
	want := `"Головачев"*`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFtsQueryNameOrderSameTokens(t *testing.T) {
	a := ftsQuery("кинг стивен")
	b := ftsQuery("стивен кинг")
	if a != `"кинг"* AND "стивен"*` {
		t.Fatalf("a=%q", a)
	}
	if b != `"стивен"* AND "кинг"*` {
		t.Fatalf("b=%q", b)
	}
}
