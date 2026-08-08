package archive

import (
	"strings"
	"testing"
)

func TestContentDispositionUTF8(t *testing.T) {
	name := SafeFilename("Полчаса телеги", "fb2")
	h := ContentDisposition(name)
	if !strings.Contains(h, "filename*=UTF-8''") {
		t.Fatalf("missing filename*: %s", h)
	}
	if !strings.Contains(h, "%D0%9F") { // 'П' in UTF-8
		t.Fatalf("expected percent-encoded Cyrillic in %s", h)
	}
	if strings.Contains(h, `filename="Пол`) {
		t.Fatalf("raw UTF-8 must not be in quoted filename: %s", h)
	}
}
