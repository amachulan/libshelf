package server

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"libshelf/internal/genres"
	"libshelf/internal/store"
)

const (
	atomNS   = "http://www.w3.org/2005/Atom"
	opdsNS   = "http://opds-spec.org/2010/catalog"
	opdsType = "application/atom+xml;profile=opds-catalog;kind=navigation"
	opdsAcq  = "application/atom+xml;profile=opds-catalog;kind=acquisition"
)

type atomFeed struct {
	XMLName xml.Name     `xml:"feed"`
	Xmlns   string       `xml:"xmlns,attr"`
	XmlnsOD string       `xml:"xmlns:opds,attr"`
	ID      string       `xml:"id"`
	Title   string       `xml:"title"`
	Updated string       `xml:"updated"`
	Links   []atomLink   `xml:"link"`
	Entries []atomEntry  `xml:"entry"`
}

type atomEntry struct {
	ID        string     `xml:"id"`
	Title     string     `xml:"title"`
	Updated   string     `xml:"updated"`
	Authors   []atomAuthor `xml:"author,omitempty"`
	Content   *atomContent `xml:"content,omitempty"`
	Summary   string     `xml:"summary,omitempty"`
	Links     []atomLink `xml:"link"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

type atomContent struct {
	Type string `xml:"type,attr"`
	Body string `xml:",chardata"`
}

type atomLink struct {
	Rel    string `xml:"rel,attr"`
	Href   string `xml:"href,attr"`
	Type   string `xml:"type,attr,omitempty"`
	Title  string `xml:"title,attr,omitempty"`
}

func (s *Server) handleOPDS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	base := s.publicBase(r)
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/opds"), "/")
	parts := strings.Split(path, "/")
	if path == "" || parts[0] == "" {
		s.opdsRoot(w, base)
		return
	}
	switch parts[0] {
	case "search":
		s.opdsSearch(w, r, base)
	case "authors":
		if len(parts) == 1 {
			s.opdsAuthorLetters(w, r, base)
			return
		}
		if parts[1] == "letter" && len(parts) >= 3 {
			s.opdsAuthorsByLetter(w, r, base, parts[2])
			return
		}
		http.NotFound(w, r)
	case "author":
		if len(parts) < 2 {
			http.NotFound(w, r)
			return
		}
		id, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		s.opdsAuthorBooks(w, r, base, id)
	case "series":
		if len(parts) == 1 {
			s.opdsSeriesLetters(w, r, base)
			return
		}
		if parts[1] == "letter" && len(parts) >= 3 {
			s.opdsSeriesByLetter(w, r, base, parts[2])
			return
		}
		id, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		s.opdsSeriesBooks(w, r, base, id)
	case "genres":
		if len(parts) == 1 {
			s.opdsGenres(w, base)
			return
		}
		s.opdsGenreBooks(w, r, base, parts[1])
	case "book":
		if len(parts) < 2 {
			http.NotFound(w, r)
			return
		}
		id, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		s.opdsBook(w, r, base, id)
	case "opensearch.xml":
		s.opdsOpenSearch(w, base)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) publicBase(r *http.Request) string {
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return strings.TrimRight(proto+"://"+host, "/")
}

func (s *Server) opdsRoot(w http.ResponseWriter, base string) {
	now := time.Now().UTC().Format(time.RFC3339)
	feed := atomFeed{
		Xmlns:   atomNS,
		XmlnsOD: opdsNS,
		ID:      base + "/opds",
		Title:   "Моя полка",
		Updated: now,
		Links: []atomLink{
			{Rel: "self", Href: base + "/opds", Type: opdsType},
			{Rel: "start", Href: base + "/opds", Type: opdsType},
			{Rel: "search", Href: base + "/opds/opensearch.xml", Type: "application/opensearchdescription+xml"},
		},
		Entries: []atomEntry{
			navEntry(base+"/opds/search", "Поиск", "Поиск по автору, названию и серии", now),
			navEntry(base+"/opds/authors", "Авторы", "Каталог авторов по алфавиту", now),
			navEntry(base+"/opds/genres", "Жанры", "Книги по жанрам", now),
			navEntry(base+"/opds/series", "Серии", "Каталог серий по алфавиту", now),
		},
	}
	// search entry should be acquisition search - use link with search rel on feed is enough
	writeAtom(w, feed, opdsType)
}

func navEntry(href, title, summary, updated string) atomEntry {
	return atomEntry{
		ID:      href,
		Title:   title,
		Updated: updated,
		Summary: summary,
		Links: []atomLink{
			{Rel: "subsection", Href: href, Type: opdsType},
		},
	}
}

func (s *Server) opdsOpenSearch(w http.ResponseWriter, base string) {
	w.Header().Set("Content-Type", "application/opensearchdescription+xml; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<OpenSearchDescription xmlns="http://a9.com/-/spec/opensearch/1.1/">
  <ShortName>Моя полка</ShortName>
  <Description>Поиск книг</Description>
  <Url type="application/atom+xml;profile=opds-catalog;kind=acquisition"
       template=%q/>
</OpenSearchDescription>
`, base+"/opds/search?q={searchTerms}")
}

func (s *Server) opdsSearch(w http.ResponseWriter, r *http.Request, base string) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		q = strings.TrimSpace(r.URL.Query().Get("query"))
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 40
	}
	books, err := s.store.Search(q, limit)
	if err != nil {
		httpError(w, err, 500)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	feed := atomFeed{
		Xmlns:   atomNS,
		XmlnsOD: opdsNS,
		ID:      base + "/opds/search?q=" + url.QueryEscape(q),
		Title:   "Поиск: " + q,
		Updated: now,
		Links: []atomLink{
			{Rel: "self", Href: base + "/opds/search?q=" + url.QueryEscape(q), Type: opdsAcq},
			{Rel: "start", Href: base + "/opds", Type: opdsType},
			{Rel: "search", Href: base + "/opds/opensearch.xml", Type: "application/opensearchdescription+xml"},
		},
	}
	for _, b := range books {
		feed.Entries = append(feed.Entries, s.bookEntry(base, b, now))
	}
	writeAtom(w, feed, opdsAcq)
}

func (s *Server) opdsAuthorLetters(w http.ResponseWriter, r *http.Request, base string) {
	letters, err := s.store.AuthorLetters()
	if err != nil {
		httpError(w, err, 500)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	feed := atomFeed{
		Xmlns: atomNS, XmlnsOD: opdsNS,
		ID: base + "/opds/authors", Title: "Авторы", Updated: now,
		Links: []atomLink{
			{Rel: "self", Href: base + "/opds/authors", Type: opdsType},
			{Rel: "start", Href: base + "/opds", Type: opdsType},
		},
	}
	for _, l := range letters {
		href := base + "/opds/authors/letter/" + url.PathEscape(l.Letter)
		feed.Entries = append(feed.Entries, atomEntry{
			ID: href, Title: l.Letter, Updated: now,
			Summary: fmt.Sprintf("%d авторов", l.Count),
			Links:   []atomLink{{Rel: "subsection", Href: href, Type: opdsType}},
		})
	}
	writeAtom(w, feed, opdsType)
}

func (s *Server) opdsAuthorsByLetter(w http.ResponseWriter, r *http.Request, base, letter string) {
	letter, _ = url.PathUnescape(letter)
	items, err := s.store.AuthorsByLetter(letter, 200, 0)
	if err != nil {
		httpError(w, err, 500)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	feed := atomFeed{
		Xmlns: atomNS, XmlnsOD: opdsNS,
		ID: base + "/opds/authors/letter/" + url.PathEscape(letter),
		Title: "Авторы · " + letter, Updated: now,
		Links: []atomLink{
			{Rel: "self", Href: base + "/opds/authors/letter/" + url.PathEscape(letter), Type: opdsType},
			{Rel: "start", Href: base + "/opds", Type: opdsType},
			{Rel: "up", Href: base + "/opds/authors", Type: opdsType},
		},
	}
	for _, a := range items {
		href := base + "/opds/author/" + strconv.FormatInt(a.ID, 10)
		feed.Entries = append(feed.Entries, atomEntry{
			ID: href, Title: a.Name, Updated: now,
			Summary: fmt.Sprintf("%d книг", a.Books),
			Links:   []atomLink{{Rel: "subsection", Href: href, Type: opdsAcq}},
		})
	}
	writeAtom(w, feed, opdsType)
}

func (s *Server) opdsAuthorBooks(w http.ResponseWriter, r *http.Request, base string, id int64) {
	list, err := s.store.AuthorBooks(id, 100, 0)
	if err != nil {
		if err == store.ErrNotFound {
			http.NotFound(w, r)
			return
		}
		httpError(w, err, 500)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	feed := atomFeed{
		Xmlns: atomNS, XmlnsOD: opdsNS,
		ID: base + "/opds/author/" + strconv.FormatInt(id, 10),
		Title: list.Name, Updated: now,
		Links: []atomLink{
			{Rel: "self", Href: base + "/opds/author/" + strconv.FormatInt(id, 10), Type: opdsAcq},
			{Rel: "start", Href: base + "/opds", Type: opdsType},
		},
	}
	for _, b := range list.Books {
		feed.Entries = append(feed.Entries, s.bookEntry(base, b, now))
	}
	writeAtom(w, feed, opdsAcq)
}

func (s *Server) opdsSeriesLetters(w http.ResponseWriter, r *http.Request, base string) {
	letters, err := s.store.SeriesLetters()
	if err != nil {
		httpError(w, err, 500)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	feed := atomFeed{
		Xmlns: atomNS, XmlnsOD: opdsNS,
		ID: base + "/opds/series", Title: "Серии", Updated: now,
		Links: []atomLink{
			{Rel: "self", Href: base + "/opds/series", Type: opdsType},
			{Rel: "start", Href: base + "/opds", Type: opdsType},
		},
	}
	for _, l := range letters {
		href := base + "/opds/series/letter/" + url.PathEscape(l.Letter)
		feed.Entries = append(feed.Entries, atomEntry{
			ID: href, Title: l.Letter, Updated: now,
			Summary: fmt.Sprintf("%d серий", l.Count),
			Links:   []atomLink{{Rel: "subsection", Href: href, Type: opdsType}},
		})
	}
	writeAtom(w, feed, opdsType)
}

func (s *Server) opdsSeriesByLetter(w http.ResponseWriter, r *http.Request, base, letter string) {
	letter, _ = url.PathUnescape(letter)
	items, err := s.store.SeriesByLetter(letter, 200, 0)
	if err != nil {
		httpError(w, err, 500)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	feed := atomFeed{
		Xmlns: atomNS, XmlnsOD: opdsNS,
		ID: base + "/opds/series/letter/" + url.PathEscape(letter),
		Title: "Серии · " + letter, Updated: now,
		Links: []atomLink{
			{Rel: "self", Href: base + "/opds/series/letter/" + url.PathEscape(letter), Type: opdsType},
			{Rel: "start", Href: base + "/opds", Type: opdsType},
			{Rel: "up", Href: base + "/opds/series", Type: opdsType},
		},
	}
	for _, it := range items {
		href := base + "/opds/series/" + strconv.FormatInt(it.ID, 10)
		feed.Entries = append(feed.Entries, atomEntry{
			ID: href, Title: it.Title, Updated: now,
			Summary: fmt.Sprintf("%d книг", it.Books),
			Links:   []atomLink{{Rel: "subsection", Href: href, Type: opdsAcq}},
		})
	}
	writeAtom(w, feed, opdsType)
}

func (s *Server) opdsSeriesBooks(w http.ResponseWriter, r *http.Request, base string, id int64) {
	list, err := s.store.SeriesBooks(id, 100, 0)
	if err != nil {
		if err == store.ErrNotFound {
			http.NotFound(w, r)
			return
		}
		httpError(w, err, 500)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	feed := atomFeed{
		Xmlns: atomNS, XmlnsOD: opdsNS,
		ID: base + "/opds/series/" + strconv.FormatInt(id, 10),
		Title: list.Name, Updated: now,
		Links: []atomLink{
			{Rel: "self", Href: base + "/opds/series/" + strconv.FormatInt(id, 10), Type: opdsAcq},
			{Rel: "start", Href: base + "/opds", Type: opdsType},
		},
	}
	for _, b := range list.Books {
		feed.Entries = append(feed.Entries, s.bookEntry(base, b, now))
	}
	writeAtom(w, feed, opdsAcq)
}

func (s *Server) opdsGenres(w http.ResponseWriter, base string) {
	items, err := s.store.ListGenres()
	if err != nil {
		httpError(w, err, 500)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	feed := atomFeed{
		Xmlns: atomNS, XmlnsOD: opdsNS,
		ID: base + "/opds/genres", Title: "Жанры", Updated: now,
		Links: []atomLink{
			{Rel: "self", Href: base + "/opds/genres", Type: opdsType},
			{Rel: "start", Href: base + "/opds", Type: opdsType},
		},
	}
	for _, g := range items {
		name := genres.Name(g.Code)
		href := base + "/opds/genres/" + url.PathEscape(g.Code)
		feed.Entries = append(feed.Entries, atomEntry{
			ID: href, Title: name, Updated: now,
			Summary: fmt.Sprintf("%d книг", g.Books),
			Links:   []atomLink{{Rel: "subsection", Href: href, Type: opdsAcq}},
		})
	}
	writeAtom(w, feed, opdsType)
}

func (s *Server) opdsGenreBooks(w http.ResponseWriter, r *http.Request, base, code string) {
	code, _ = url.PathUnescape(code)
	list, err := s.store.GenreBooks(code, 100, 0)
	if err != nil {
		if err == store.ErrNotFound {
			http.NotFound(w, r)
			return
		}
		httpError(w, err, 500)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	title := genres.Name(code)
	feed := atomFeed{
		Xmlns: atomNS, XmlnsOD: opdsNS,
		ID: base + "/opds/genres/" + url.PathEscape(code),
		Title: title, Updated: now,
		Links: []atomLink{
			{Rel: "self", Href: base + "/opds/genres/" + url.PathEscape(code), Type: opdsAcq},
			{Rel: "start", Href: base + "/opds", Type: opdsType},
			{Rel: "up", Href: base + "/opds/genres", Type: opdsType},
		},
	}
	for _, b := range list.Books {
		feed.Entries = append(feed.Entries, s.bookEntry(base, b, now))
	}
	writeAtom(w, feed, opdsAcq)
}

func (s *Server) opdsBook(w http.ResponseWriter, r *http.Request, base string, id int64) {
	d, err := s.store.GetBook(id)
	if err != nil {
		if err == store.ErrNotFound {
			http.NotFound(w, r)
			return
		}
		httpError(w, err, 500)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	feed := atomFeed{
		Xmlns: atomNS, XmlnsOD: opdsNS,
		ID: base + "/opds/book/" + strconv.FormatInt(id, 10),
		Title: d.Title, Updated: now,
		Links: []atomLink{
			{Rel: "self", Href: base + "/opds/book/" + strconv.FormatInt(id, 10), Type: opdsAcq},
			{Rel: "start", Href: base + "/opds", Type: opdsType},
		},
		Entries: []atomEntry{s.bookEntry(base, d.Book, now)},
	}
	writeAtom(w, feed, opdsAcq)
}

func (s *Server) bookEntry(base string, b store.Book, updated string) atomEntry {
	id := base + "/opds/book/" + strconv.FormatInt(b.ID, 10)
	cover := base + "/cover/" + strconv.FormatInt(b.ID, 10)
	dl := base + "/download/" + strconv.FormatInt(b.ID, 10)
	e := atomEntry{
		ID:      id,
		Title:   b.Title,
		Updated: updated,
		Links: []atomLink{
			{Rel: "http://opds-spec.org/image", Href: cover, Type: "image/jpeg"},
			{Rel: "http://opds-spec.org/image/thumbnail", Href: cover, Type: "image/jpeg"},
			{Rel: "http://opds-spec.org/acquisition", Href: dl, Type: "application/fb2+xml", Title: "FB2"},
			{Rel: "alternate", Href: base + "/?book=" + strconv.FormatInt(b.ID, 10), Type: "text/html"},
		},
	}
	if b.Authors != "" {
		for _, name := range strings.Split(b.Authors, ",") {
			name = strings.TrimSpace(name)
			if name != "" {
				e.Authors = append(e.Authors, atomAuthor{Name: name})
			}
		}
	}
	var bits []string
	if b.Series != "" {
		bits = append(bits, b.Series)
	}
	if b.Year > 0 {
		bits = append(bits, strconv.Itoa(b.Year))
	}
	if len(bits) > 0 {
		e.Summary = strings.Join(bits, " · ")
	}
	return e
}

func writeAtom(w http.ResponseWriter, feed atomFeed, contentType string) {
	w.Header().Set("Content-Type", contentType+"; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	_ = enc.Encode(feed)
}
