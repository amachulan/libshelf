package fb2

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"strconv"
	"strings"

	"golang.org/x/net/html/charset"
)

type Chapter struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type ReaderDoc struct {
	Title    string    `json:"title"`
	HTML     string    `json:"html"`
	Chapters []Chapter `json:"chapters"`
}

var bodyTags = map[string]string{
	"p":             "p",
	"emphasis":      "em",
	"strong":        "strong",
	"strikethrough": "s",
	"sub":           "sub",
	"sup":           "sup",
	"code":          "code",
	"cite":          "blockquote",
	"poem":          "blockquote",
	"stanza":        "div",
	"subtitle":      "h3",
	"text-author":   "p",
	"epigraph":      "blockquote",
}

// ExtractReader converts the main FB2 body into HTML with a chapter TOC.
func ExtractReader(r io.Reader) (*ReaderDoc, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	binaries, title := collectBinaries(bytes.NewReader(raw))
	doc, err := renderBody(bytes.NewReader(raw), binaries)
	if err != nil {
		return nil, err
	}
	if doc.Title == "" {
		doc.Title = title
	}
	return doc, nil
}

func collectBinaries(r io.Reader) (map[string]string, string) {
	dec := xml.NewDecoder(r)
	dec.CharsetReader = charset.NewReaderLabel
	dec.Strict = false
	out := map[string]string{}
	var title string
	for {
		tok, err := dec.Token()
		if err != nil {
			return out, title
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		name := strings.ToLower(se.Name.Local)
		switch name {
		case "book-title":
			if title == "" {
				if t, err := readText(dec); err == nil {
					title = t
				}
			}
		case "binary":
			var id, ctype string
			for _, a := range se.Attr {
				switch a.Name.Local {
				case "id":
					id = a.Value
				case "content-type":
					ctype = a.Value
				}
			}
			var b64 string
			if err := dec.DecodeElement(&b64, &se); err != nil {
				continue
			}
			if id == "" {
				continue
			}
			data, err := base64.StdEncoding.DecodeString(strings.Map(dropSpace, b64))
			if err != nil || len(data) == 0 {
				continue
			}
			if ctype == "" {
				ctype = "image/jpeg"
			}
			out[id] = "data:" + ctype + ";base64," + base64.StdEncoding.EncodeToString(data)
		}
	}
}

func renderBody(r io.Reader, binaries map[string]string) (*ReaderDoc, error) {
	dec := xml.NewDecoder(r)
	dec.CharsetReader = charset.NewReaderLabel
	dec.Strict = false

	var title string
	var htmlBuilder strings.Builder
	var chapters []Chapter
	sectionDepth := 0
	chapterN := 0
	inBody := false
	inTitle := false
	var titleBuf strings.Builder
	bodyCount := 0

	flushTitle := func() {
		t := strings.Join(strings.Fields(titleBuf.String()), " ")
		titleBuf.Reset()
		inTitle = false
		if t == "" {
			return
		}
		if title == "" {
			title = t
		}
		if sectionDepth == 1 {
			chapterN++
			id := "ch-" + strconv.Itoa(chapterN)
			chapters = append(chapters, Chapter{ID: id, Title: t})
			htmlBuilder.WriteString(`<h2 class="chapter" id="` + html.EscapeString(id) + `">`)
			htmlBuilder.WriteString(html.EscapeString(t))
			htmlBuilder.WriteString("</h2>")
			return
		}
		htmlBuilder.WriteString("<h3>")
		htmlBuilder.WriteString(html.EscapeString(t))
		htmlBuilder.WriteString("</h3>")
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("fb2 reader: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			switch name {
			case "book-title":
				text, err := readText(dec)
				if err != nil {
					return nil, err
				}
				if title == "" {
					title = text
				}
			case "body":
				bodyCount++
				nameAttr := attr(t, "name")
				if bodyCount > 1 || strings.EqualFold(nameAttr, "notes") || strings.EqualFold(nameAttr, "comments") {
					if err := dec.Skip(); err != nil {
						return nil, err
					}
					continue
				}
				inBody = true
			case "section":
				if inBody {
					sectionDepth++
				}
			case "title":
				if inBody && sectionDepth > 0 {
					inTitle = true
					titleBuf.Reset()
				}
			case "image":
				if !inBody {
					continue
				}
				href := imageHref(t)
				if src, ok := binaries[href]; ok {
					htmlBuilder.WriteString(`<img class="fb2-img" src="` + src + `" alt="">`)
				}
			case "empty-line":
				if inBody && !inTitle {
					htmlBuilder.WriteString("<br>")
				}
			case "binary":
				if err := dec.Skip(); err != nil {
					return nil, err
				}
			default:
				if !inBody || inTitle {
					continue
				}
				if tag, ok := bodyTags[name]; ok {
					htmlBuilder.WriteString("<" + tag + ">")
				}
			}
		case xml.EndElement:
			name := strings.ToLower(t.Name.Local)
			switch name {
			case "body":
				inBody = false
			case "section":
				if sectionDepth > 0 {
					sectionDepth--
				}
			case "title":
				if inTitle {
					flushTitle()
				}
			case "v":
				if inBody && !inTitle {
					htmlBuilder.WriteString("<br>")
				}
			default:
				if !inBody || inTitle {
					continue
				}
				if tag, ok := bodyTags[name]; ok {
					htmlBuilder.WriteString("</" + tag + ">")
				}
			}
		case xml.CharData:
			if inTitle {
				titleBuf.WriteString(string(t))
				continue
			}
			if inBody {
				htmlBuilder.WriteString(html.EscapeString(string(t)))
			}
		}
	}

	doc := &ReaderDoc{
		Title:    title,
		HTML:     strings.TrimSpace(htmlBuilder.String()),
		Chapters: chapters,
	}
	if doc.HTML == "" {
		return nil, fmt.Errorf("empty fb2 body")
	}
	return doc, nil
}

func imageHref(t xml.StartElement) string {
	for _, a := range t.Attr {
		if a.Name.Local == "href" {
			return strings.TrimPrefix(a.Value, "#")
		}
	}
	return ""
}

func attr(t xml.StartElement, local string) string {
	for _, a := range t.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}
