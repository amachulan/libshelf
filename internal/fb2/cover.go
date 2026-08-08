package fb2

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"golang.org/x/net/html/charset"
)

type Cover struct {
	Data []byte
	Mime string
}

// ExtractCover reads cover image bytes from an FB2 stream.
func ExtractCover(r io.Reader) (*Cover, error) {
	dec := xml.NewDecoder(r)
	dec.CharsetReader = charset.NewReaderLabel
	dec.Strict = false

	var coverID string
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
			break
		}
		if err != nil {
			return nil, fmt.Errorf("fb2: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local
			switch {
			case name == "image" && in("description", "title-info", "coverpage"):
				for _, a := range t.Attr {
					if a.Name.Local == "href" {
						coverID = strings.TrimPrefix(a.Value, "#")
					}
				}
			case name == "binary":
				var id, ctype string
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "id":
						id = a.Value
					case "content-type":
						ctype = a.Value
					}
				}
				take := (coverID != "" && id == coverID) ||
					(coverID == "" && strings.HasPrefix(ctype, "image/"))
				if take {
					var b64 string
					if err := dec.DecodeElement(&b64, &t); err != nil {
						return nil, err
					}
					data, err := base64.StdEncoding.DecodeString(strings.Map(dropSpace, b64))
					if err != nil {
						return nil, fmt.Errorf("cover base64: %w", err)
					}
					if ctype == "" {
						ctype = "image/jpeg"
					}
					return &Cover{Data: data, Mime: ctype}, nil
				}
				if err := dec.Skip(); err != nil {
					return nil, err
				}
				continue
			}
			path = append(path, name)
		case xml.EndElement:
			if len(path) > 0 {
				path = path[:len(path)-1]
			}
		}
	}
	return nil, fmt.Errorf("no cover in fb2")
}

func dropSpace(r rune) rune {
	if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
		return -1
	}
	return r
}
