package fb2

import (
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"strings"

	"golang.org/x/net/html/charset"
)

// Person is an FB2 author/translator name.
type Person struct {
	First    string
	Middle   string
	Last     string
	Nickname string
}

func (p Person) Display() string {
	parts := make([]string, 0, 3)
	for _, s := range []string{p.Last, p.First, p.Middle} {
		if t := strings.TrimSpace(s); t != "" {
			parts = append(parts, t)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	return strings.TrimSpace(p.Nickname)
}

// Meta is useful title/publish metadata from an FB2 description.
type Meta struct {
	Annotation  string   `json:"annotation,omitempty"`
	Translators []string `json:"translators,omitempty"`
	Publisher   string   `json:"publisher,omitempty"`
	City        string   `json:"city,omitempty"`
	Year        string   `json:"year,omitempty"`
	ISBN        string   `json:"isbn,omitempty"`
}

// ExtractAnnotation returns sanitized HTML of the FB2 annotation, if any.
func ExtractAnnotation(r io.Reader) (string, error) {
	m, err := ExtractMeta(r)
	if err != nil {
		return "", err
	}
	return m.Annotation, nil
}

// ExtractMeta reads annotation, translators and publish-info from FB2.
func ExtractMeta(r io.Reader) (*Meta, error) {
	dec := xml.NewDecoder(r)
	dec.CharsetReader = charset.NewReaderLabel
	dec.Strict = false

	var path []string
	in := func(elems ...string) bool {
		if len(path) < len(elems) {
			return false
		}
		tail := path[len(path)-len(elems):]
		for i := range elems {
			if !strings.EqualFold(tail[i], elems[i]) {
				return false
			}
		}
		return true
	}

	meta := &Meta{}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return meta, nil
		}
		if err != nil {
			return nil, fmt.Errorf("fb2: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			switch {
			case name == "annotation" && in("description", "title-info"):
				ann, err := annotationToHTML(dec)
				if err != nil {
					return nil, err
				}
				meta.Annotation = ann
			case name == "translator" && in("description", "title-info"):
				p, err := readPerson(dec)
				if err != nil {
					return nil, err
				}
				if d := p.Display(); d != "" {
					meta.Translators = append(meta.Translators, d)
				}
			case name == "publisher" && in("description", "publish-info"):
				text, err := readText(dec)
				if err != nil {
					return nil, err
				}
				meta.Publisher = text
			case name == "city" && in("description", "publish-info"):
				text, err := readText(dec)
				if err != nil {
					return nil, err
				}
				meta.City = text
			case name == "year" && in("description", "publish-info"):
				text, err := readText(dec)
				if err != nil {
					return nil, err
				}
				meta.Year = text
			case name == "isbn" && in("description", "publish-info"):
				text, err := readText(dec)
				if err != nil {
					return nil, err
				}
				meta.ISBN = text
			case name == "body":
				return meta, nil
			default:
				path = append(path, name)
			}
		case xml.EndElement:
			if len(path) > 0 {
				path = path[:len(path)-1]
			}
			if strings.EqualFold(t.Name.Local, "description") {
				return meta, nil
			}
		}
	}
}

func readPerson(dec *xml.Decoder) (Person, error) {
	var p Person
	depth := 1
	var field string
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return p, fmt.Errorf("person: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			field = strings.ToLower(t.Name.Local)
		case xml.EndElement:
			depth--
			field = ""
		case xml.CharData:
			text := strings.TrimSpace(string(t))
			if text == "" || field == "" {
				continue
			}
			switch field {
			case "first-name":
				p.First = text
			case "middle-name":
				p.Middle = text
			case "last-name":
				p.Last = text
			case "nickname":
				p.Nickname = text
			}
		}
	}
	return p, nil
}

func readText(dec *xml.Decoder) (string, error) {
	var sb strings.Builder
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return "", fmt.Errorf("text: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			sb.Write(t)
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

var annotationTags = map[string]string{
	"p":             "p",
	"emphasis":      "em",
	"strong":        "strong",
	"strikethrough": "s",
	"sub":           "sub",
	"sup":           "sup",
	"code":          "code",
	"cite":          "blockquote",
	"poem":          "blockquote",
	"stanza":        "p",
	"subtitle":      "h4",
}

func annotationToHTML(dec *xml.Decoder) (string, error) {
	var sb strings.Builder
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return "", fmt.Errorf("annotation: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch t.Name.Local {
			case "empty-line":
				sb.WriteString("<br>")
			case "v":
			default:
				if tag, ok := annotationTags[t.Name.Local]; ok {
					sb.WriteString("<" + tag + ">")
				}
			}
		case xml.EndElement:
			depth--
			if depth == 0 {
				break
			}
			switch t.Name.Local {
			case "empty-line":
			case "v":
				sb.WriteString("<br>")
			default:
				if tag, ok := annotationTags[t.Name.Local]; ok {
					sb.WriteString("</" + tag + ">")
				}
			}
		case xml.CharData:
			sb.WriteString(html.EscapeString(string(t)))
		}
	}
	return strings.TrimSpace(sb.String()), nil
}
