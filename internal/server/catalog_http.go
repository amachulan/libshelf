package server

import (
	"net/http"
	"strconv"
	"strings"

	"libshelf/internal/genres"
	"libshelf/internal/store"
)

func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/catalog"), "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, map[string]any{
			"sections": []string{"authors", "genres", "series"},
		})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	letter := r.URL.Query().Get("letter")

	switch parts[0] {
	case "authors":
		if letter == "" {
			letters, err := s.store.AuthorLetters()
			if err != nil {
				httpError(w, err, 500)
				return
			}
			writeJSON(w, map[string]any{"letters": letters})
			return
		}
		items, err := s.store.AuthorsByLetter(letter, limit, offset)
		if err != nil {
			httpError(w, err, 500)
			return
		}
		writeJSON(w, map[string]any{"letter": letter, "authors": items})
	case "series":
		if letter == "" {
			letters, err := s.store.SeriesLetters()
			if err != nil {
				httpError(w, err, 500)
				return
			}
			writeJSON(w, map[string]any{"letters": letters})
			return
		}
		items, err := s.store.SeriesByLetter(letter, limit, offset)
		if err != nil {
			httpError(w, err, 500)
			return
		}
		writeJSON(w, map[string]any{"letter": letter, "series": items})
	case "genres":
		if len(parts) == 1 {
			items, err := s.store.ListGenres()
			if err != nil {
				httpError(w, err, 500)
				return
			}
			for i := range items {
				items[i].Name = genres.Name(items[i].Code)
			}
			writeJSON(w, map[string]any{"genres": items})
			return
		}
		code := parts[1]
		list, err := s.store.GenreBooks(code, limit, offset)
		if err != nil {
			if err == store.ErrNotFound {
				httpError(w, err, 404)
				return
			}
			httpError(w, err, 500)
			return
		}
		list.Name = genres.Name(code)
		decorateBooks(list.Books)
		writeJSON(w, list)
	default:
		http.NotFound(w, r)
	}
}
