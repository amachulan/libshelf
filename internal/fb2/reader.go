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

func isNotesBodyName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "notes", "comments", "footnotes":
		return true
	default:
		return false
	}
}

type fb2Note struct {
	id   string
	html string
}

func renderBody(r io.Reader, binaries map[string]string) (*ReaderDoc, error) {
	dec := xml.NewDecoder(r)
	dec.CharsetReader = charset.NewReaderLabel
	dec.Strict = false

	var title string
	var htmlBuilder strings.Builder
	var noteBuf strings.Builder
	var notes []fb2Note
	var chapters []Chapter
	sectionDepth := 0
	chapterN := 0
	noteN := 0
	inBody := false
	inNotes := false
	inTitle := false
	noteID := ""
	var dest *strings.Builder
	var titleBuf strings.Builder
	bodyCount := 0

	syncDest := func() {
		if inTitle {
			dest = nil
			return
		}
		if inNotes {
			if noteID != "" {
				dest = &noteBuf
			} else {
				dest = nil
			}
			return
		}
		if inBody {
			dest = &htmlBuilder
			return
		}
		dest = nil
	}

	flushNote := func() {
		htmlFrag := strings.TrimSpace(noteBuf.String())
		noteBuf.Reset()
		id := noteID
		noteID = ""
		if id == "" || htmlFrag == "" {
			return
		}
		notes = append(notes, fb2Note{id: id, html: htmlFrag})
	}

	flushTitle := func() {
		t := strings.Join(strings.Fields(titleBuf.String()), " ")
		titleBuf.Reset()
		inTitle = false
		if t == "" {
			syncDest()
			return
		}
		if inNotes {
			if noteID != "" {
				noteBuf.WriteString(`<p class="fb2-note-label">`)
				noteBuf.WriteString(html.EscapeString(t))
				noteBuf.WriteString("</p>")
			}
			syncDest()
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
			syncDest()
			return
		}
		htmlBuilder.WriteString("<h3>")
		htmlBuilder.WriteString(html.EscapeString(t))
		htmlBuilder.WriteString("</h3>")
		syncDest()
	}

	writeOpen := func(t xml.StartElement) {
		if dest == nil {
			return
		}
		name := strings.ToLower(t.Name.Local)
		switch name {
		case "image":
			href := imageHref(t)
			if src, ok := binaries[href]; ok {
				dest.WriteString(`<img class="fb2-img" src="` + src + `" alt="">`)
			}
		case "empty-line":
			dest.WriteString("<br>")
		case "a":
			writeAnchor(dest, t)
		default:
			if tag, ok := bodyTags[name]; ok {
				dest.WriteString("<" + tag + ">")
			}
		}
	}

	writeClose := func(name string) {
		if dest == nil {
			return
		}
		switch name {
		case "a":
			dest.WriteString("</a>")
		case "v":
			dest.WriteString("<br>")
		default:
			if tag, ok := bodyTags[name]; ok {
				dest.WriteString("</" + tag + ">")
			}
		}
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
				if isNotesBodyName(nameAttr) {
					inNotes = true
					inBody = false
					sectionDepth = 0
					syncDest()
					continue
				}
				if bodyCount > 1 {
					if err := dec.Skip(); err != nil {
						return nil, err
					}
					continue
				}
				inBody = true
				inNotes = false
				syncDest()
			case "section":
				if inNotes {
					if sectionDepth == 0 {
						if noteID != "" {
							flushNote()
						}
						noteID = strings.TrimSpace(attr(t, "id"))
						if noteID == "" {
							noteN++
							noteID = "fb2-note-" + strconv.Itoa(noteN)
						}
						noteBuf.Reset()
					}
					sectionDepth++
					syncDest()
					continue
				}
				if inBody {
					sectionDepth++
					if id := strings.TrimSpace(attr(t, "id")); id != "" {
						htmlBuilder.WriteString(`<span class="fb2-anchor" id="` + html.EscapeString(id) + `"></span>`)
					}
				}
			case "title":
				if (inBody || inNotes) && sectionDepth > 0 {
					inTitle = true
					titleBuf.Reset()
					syncDest()
				}
			case "binary":
				if err := dec.Skip(); err != nil {
					return nil, err
				}
			default:
				writeOpen(t)
			}
		case xml.EndElement:
			name := strings.ToLower(t.Name.Local)
			switch name {
			case "body":
				if inNotes && noteID != "" {
					flushNote()
				}
				inBody = false
				inNotes = false
				noteID = ""
				syncDest()
			case "section":
				if inNotes {
					if sectionDepth == 1 {
						flushNote()
					}
					if sectionDepth > 0 {
						sectionDepth--
					}
					syncDest()
					continue
				}
				if sectionDepth > 0 {
					sectionDepth--
				}
			case "title":
				if inTitle {
					flushTitle()
				}
			default:
				writeClose(name)
			}
		case xml.CharData:
			if inTitle {
				titleBuf.WriteString(string(t))
				continue
			}
			if dest != nil {
				dest.WriteString(html.EscapeString(string(t)))
			}
		}
	}

	if len(notes) > 0 {
		htmlBuilder.WriteString(`<aside class="fb2-notes" hidden>`)
		for _, n := range notes {
			htmlBuilder.WriteString(`<section class="fb2-note" id="` + html.EscapeString(n.id) + `">`)
			htmlBuilder.WriteString(n.html)
			htmlBuilder.WriteString(`</section>`)
		}
		htmlBuilder.WriteString(`</aside>`)
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

func writeAnchor(b *strings.Builder, t xml.StartElement) {
	href := strings.TrimSpace(imageHref(t))
	typ := strings.ToLower(attr(t, "type"))
	low := strings.ToLower(href)
	if href != "" && (strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://")) {
		b.WriteString(`<a class="fb2-ext" href="` + html.EscapeString(href) + `" target="_blank" rel="noopener noreferrer">`)
		return
	}
	id := strings.TrimPrefix(href, "#")
	class := "fb2-ref"
	if typ == "note" || typ == "cite" {
		class = "fb2-note-ref"
	}
	if id == "" {
		b.WriteString(`<a class="` + class + `">`)
		return
	}
	b.WriteString(`<a class="` + class + `" href="#` + html.EscapeString(id) + `">`)
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
