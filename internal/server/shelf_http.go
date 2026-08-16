package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"libshelf/internal/auth"
)

func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) *auth.User {
	u := userFrom(r.Context())
	if u == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil
	}
	return u
}

func (s *Server) handleShelf(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/shelf"), "/")

	switch r.Method {
	case http.MethodGet:
		if path == "" || path == "continue" {
			s.listShelf(w, r, u, path == "continue")
			return
		}
		bookID, err := strconv.ParseInt(path, 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		entry, err := s.auth.GetShelfEntry(u.ID, bookID)
		if err != nil {
			httpError(w, err, 500)
			return
		}
		writeJSON(w, entry)
	case http.MethodPut:
		bookID, err := strconv.ParseInt(path, 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		var body struct {
			Status *string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		status := ""
		if body.Status != nil {
			status = strings.TrimSpace(*body.Status)
		}
		if err := s.auth.SetShelfStatus(u.ID, bookID, status); err != nil {
			httpError(w, err, 400)
			return
		}
		entry, err := s.auth.GetShelfEntry(u.ID, bookID)
		if err != nil {
			httpError(w, err, 500)
			return
		}
		writeJSON(w, entry)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listShelf(w http.ResponseWriter, r *http.Request, u *auth.User, cont bool) {
	status := r.URL.Query().Get("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	var (
		items []auth.ShelfItem
		err   error
	)
	if cont {
		items, err = s.auth.ListContinue(u.ID, limit)
	} else {
		items, err = s.auth.ListShelf(u.ID, status, limit)
	}
	if err != nil {
		httpError(w, err, 400)
		return
	}
	ids := make([]int64, len(items))
	for i, it := range items {
		ids[i] = it.BookID
	}
	books, err := s.store.BooksByIDs(ids)
	if err != nil {
		httpError(w, err, 500)
		return
	}
	s.decorateBooks(books)
	byID := make(map[int64]any, len(books))
	for i := range books {
		byID[books[i].ID] = books[i]
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		b, ok := byID[it.BookID]
		if !ok {
			continue
		}
		out = append(out, map[string]any{
			"book":     b,
			"status":   it.Status,
			"progress": it.Progress,
		})
	}
	writeJSON(w, map[string]any{"items": out})
}
