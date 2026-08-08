package fb2

import (
	"strings"
	"testing"
)

func TestExtractReader(t *testing.T) {
	const sample = `<?xml version="1.0" encoding="utf-8"?>
<FictionBook xmlns="http://www.gribuser.ru/xml/fictionbook/2.0" xmlns:l="http://www.w3.org/1999/xlink">
  <description>
    <title-info>
      <book-title>Тест</book-title>
      <lang>ru</lang>
    </title-info>
  </description>
  <body>
    <section>
      <title><p>Глава 1</p></title>
      <p>Раз <emphasis>два</emphasis>.</p>
      <image l:href="#pic1"/>
    </section>
    <section>
      <title><p>Глава 2</p></title>
      <p>Три.</p>
    </section>
  </body>
  <binary id="pic1" content-type="image/png">iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==</binary>
</FictionBook>`

	doc, err := ExtractReader(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Title != "Тест" {
		t.Fatalf("title: %q", doc.Title)
	}
	if len(doc.Chapters) != 2 || doc.Chapters[0].Title != "Глава 1" {
		t.Fatalf("chapters: %#v", doc.Chapters)
	}
	if !strings.Contains(doc.HTML, "два") || !strings.Contains(doc.HTML, "<em>") {
		t.Fatalf("html markup: %s", doc.HTML)
	}
	if !strings.Contains(doc.HTML, `data:image/png;base64,`) {
		t.Fatalf("missing image: %s", doc.HTML)
	}
	if !strings.Contains(doc.HTML, `id="ch-1"`) {
		t.Fatalf("missing chapter id: %s", doc.HTML)
	}
}
