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
