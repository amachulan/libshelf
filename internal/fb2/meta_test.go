package fb2

import (
	"strings"
	"testing"
)

func TestExtractMeta(t *testing.T) {
	const sample = `<?xml version="1.0" encoding="utf-8"?>
<FictionBook xmlns="http://www.gribuser.ru/xml/fictionbook/2.0">
  <description>
    <title-info>
      <author><first-name>Стивен</first-name><last-name>Кинг</last-name></author>
      <book-title>Безнадега</book-title>
      <annotation><p>Текст <emphasis>аннотации</emphasis>.</p></annotation>
      <lang>ru</lang>
      <translator>
        <first-name>Виктор</first-name>
        <last-name>Вебер</last-name>
      </translator>
    </title-info>
    <publish-info>
      <publisher>АСТ</publisher>
      <city>Москва</city>
      <year>2015</year>
      <isbn>978-5-17-123456-7</isbn>
    </publish-info>
  </description>
  <body><section><p>hi</p></section></body>
</FictionBook>`

	m, err := ExtractMeta(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.Annotation, "аннотации") || !strings.Contains(m.Annotation, "<em>") {
		t.Fatalf("annotation: %q", m.Annotation)
	}
	if len(m.Translators) != 1 || m.Translators[0] != "Вебер Виктор" {
		t.Fatalf("translators: %#v", m.Translators)
	}
	if m.Publisher != "АСТ" || m.City != "Москва" || m.Year != "2015" {
		t.Fatalf("publish: %+v", m)
	}
	if m.ISBN != "978-5-17-123456-7" {
		t.Fatalf("isbn: %q", m.ISBN)
	}
}
