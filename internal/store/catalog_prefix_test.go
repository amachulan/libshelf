package store

import "testing"

func TestLikePrefix(t *testing.T) {
	got := likePrefix(" ёж%_\\ ")
	want := `ЕЖ\%\_\\%`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
