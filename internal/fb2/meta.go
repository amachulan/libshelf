package fb2

import (
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"strings"

	"golang.org/x/net/html/charset"
)

// ExtractAnnotation returns sanitized HTML of the FB2 annotation, if any.
func ExtractAnnotation(r io.Reader) (string, error) {
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
			if tail[i] != elems[i] {
				return false
			}
		}
		return true
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return "", nil
		}
		if err != nil {
			return "", fmt.Errorf("fb2: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local
			if name == "annotation" && in("description", "title-info") {
				return annotationToHTML(dec)
			}
			if name == "body" {
				return "", nil
			}
			path = append(path, name)
		case xml.EndElement:
			if len(path) > 0 {
				path = path[:len(path)-1]
			}
			if t.Name.Local == "description" {
				return "", nil
			}
		}
	}
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
