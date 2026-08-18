package store

import (
	"strconv"
	"sync"
)

type genreWorkRef struct {
	ID           int64
	EditionCount int
}

type genreBuild struct {
	done chan struct{}
	list []genreWorkRef
	err  error
}

// genreListCache holds ordered work ids per genre+sort+lang. Built lazily;
// dropped on import, language change, and after FantLab fetch.
type genreListCache struct {
	mu       sync.Mutex
	gen      uint64
	lists    map[string][]genreWorkRef
	inflight map[string]*genreBuild
}

func (s *Store) genreListKey(genreID int64, sortName string) string {
	return strconv.FormatInt(genreID, 10) + "|" + sortName + "|" + s.langsKey()
}

func (s *Store) cachedGenreWorks(genreID int64, sortName string) ([]genreWorkRef, error) {
	key := s.genreListKey(genreID, sortName)
	s.genreCache.mu.Lock()
	if s.genreCache.lists != nil {
		if list, ok := s.genreCache.lists[key]; ok {
			s.genreCache.mu.Unlock()
			return list, nil
		}
	}
	if s.genreCache.inflight != nil {
		if inf, ok := s.genreCache.inflight[key]; ok {
			s.genreCache.mu.Unlock()
			<-inf.done
			return inf.list, inf.err
		}
	}
	inf := &genreBuild{done: make(chan struct{})}
	if s.genreCache.inflight == nil {
		s.genreCache.inflight = make(map[string]*genreBuild)
	}
	s.genreCache.inflight[key] = inf
	gen := s.genreCache.gen
	s.genreCache.mu.Unlock()

	list, err := s.buildGenreWorks(genreID, sortName)

	s.genreCache.mu.Lock()
	delete(s.genreCache.inflight, key)
	if err == nil && s.genreCache.gen == gen {
		if s.genreCache.lists == nil {
			s.genreCache.lists = make(map[string][]genreWorkRef)
		}
		s.genreCache.lists[key] = list
	}
	inf.list = list
	inf.err = err
	close(inf.done)
	s.genreCache.mu.Unlock()
	return list, err
}

func (s *Store) buildGenreWorks(genreID int64, sortName string) ([]genreWorkRef, error) {
	order := `b.lib_rate DESC, b.title`
	switch sortName {
	case "new":
		order = `b.added DESC, coalesce(b.year, 0) DESC, b.title`
	case "title":
		order = `b.title`
	}
	lang, langArgs := s.langPred("b.lang")
	args := append([]any{genreID}, append(langArgs, genreGroupCap)...)
	rows, err := s.db.Query(`
SELECT`+bookColumns+`
FROM books b
LEFT JOIN series s ON s.id = b.series_id
JOIN book_genres bg ON bg.book_id = b.id AND bg.genre_id = ?
WHERE b.deleted = 0`+lang+`
ORDER BY `+order+`
LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	books, err := scanBooks(rows)
	if err != nil {
		return nil, err
	}
	works, err := s.groupWorks(books)
	if err != nil {
		return nil, err
	}
	if err := s.AttachFantLab(works); err != nil {
		return nil, err
	}
	if sortName == "popular" {
		sortWorksPopular(works)
	}
	out := make([]genreWorkRef, len(works))
	for i, b := range works {
		out[i] = genreWorkRef{ID: b.ID, EditionCount: b.EditionCount}
	}
	return out, nil
}

func paginateGenreRefs(list []genreWorkRef, limit, offset int) ([]genreWorkRef, int) {
	total := len(list)
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 60
	}
	if offset >= total {
		return nil, total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return list[offset:end], total
}

func (s *Store) booksFromGenreRefs(refs []genreWorkRef) ([]Book, error) {
	ids := make([]int64, len(refs))
	for i, r := range refs {
		ids[i] = r.ID
	}
	books, err := s.BooksByIDs(ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]Book, len(books))
	for _, b := range books {
		byID[b.ID] = b
	}
	out := make([]Book, 0, len(refs))
	for _, r := range refs {
		b, ok := byID[r.ID]
		if !ok {
			continue
		}
		b.EditionCount = r.EditionCount
		out = append(out, b)
	}
	if err := s.AttachFantLab(out); err != nil {
		return nil, err
	}
	return out, nil
}
