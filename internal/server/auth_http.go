package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"libshelf/internal/auth"
)

type ctxKey int

const userCtxKey ctxKey = 1

func userFrom(ctx context.Context) *auth.User {
	u, _ := ctx.Value(userCtxKey).(*auth.User)
	return u
}

func (s *Server) withUser(r *http.Request, u *auth.User) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userCtxKey, u))
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil || !s.authRequired {
			next.ServeHTTP(w, r)
			return
		}
		if isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if u := s.resolveUser(r); u != nil {
			next.ServeHTTP(w, s.withUser(r, u))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/opds") {
			w.Header().Set("WWW-Authenticate", `Basic realm="libshelf"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if wantsJSON(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/login.html", http.StatusFound)
	})
}

func (s *Server) resolveUser(r *http.Request) *auth.User {
	if s.auth == nil {
		return nil
	}
	// Cookie session wins for the web UI.
	if c, err := r.Cookie(auth.CookieName()); err == nil && c.Value != "" {
		if u, err := s.auth.UserByToken(c.Value); err == nil {
			return u
		}
	}
	// Basic Auth is only for OPDS clients. Browsers often cache realm credentials
	// after a 401 and would otherwise silently re-login as that user on / and /api/*.
	if allowBasicAuth(r.URL.Path) {
		if user, pass, ok := r.BasicAuth(); ok {
			if u, err := s.auth.Authenticate(user, pass); err == nil {
				return u
			}
		}
	}
	return nil
}

func allowBasicAuth(path string) bool {
	return strings.HasPrefix(path, "/opds") ||
		strings.HasPrefix(path, "/download/") ||
		strings.HasPrefix(path, "/cover/")
}

func isPublicPath(path string) bool {
	switch {
	case path == "/health", path == "/api/login", path == "/login.html", path == "/favicon.ico":
		return true
	case path == "/style.css", path == "/app.js", path == "/login.js", path == "/theme.js":
		return true
	default:
		return false
	}
}

func wantsJSON(r *http.Request) bool {
	if strings.HasPrefix(r.URL.Path, "/api/") ||
		strings.HasPrefix(r.URL.Path, "/cover/") ||
		strings.HasPrefix(r.URL.Path, "/download/") {
		return true
	}
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json")
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) *auth.User {
	u := userFrom(r.Context())
	if u == nil || u.Role != auth.RoleAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}
	return u
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.auth == nil {
		http.Error(w, "auth disabled", http.StatusBadRequest)
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	// Drop previous browser session before issuing a new one (user switch).
	if c, err := r.Cookie(auth.CookieName()); err == nil && c.Value != "" {
		s.auth.Logout(c.Value)
	}
	token, user, err := s.auth.Login(body.Username, body.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCreds) {
			http.Error(w, "invalid username or password", http.StatusUnauthorized)
			return
		}
		httpError(w, err, 500)
		return
	}
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName(),
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		Expires:  time.Now().Add(30 * 24 * time.Hour),
	})
	writeJSON(w, user)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if c, err := r.Cookie(auth.CookieName()); err == nil && s.auth != nil {
		s.auth.Logout(c.Value)
	}
	// Must match Secure/SameSite of the login cookie or the browser keeps the old one on HTTPS.
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName(),
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil || !s.authRequired {
		writeJSON(w, map[string]any{"auth": false, "user": nil})
		return
	}
	u := userFrom(r.Context())
	if u == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, map[string]any{"auth": true, "user": u})
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/users")
	path = strings.Trim(path, "/")

	switch r.Method {
	case http.MethodGet:
		if path != "" {
			http.NotFound(w, r)
			return
		}
		users, err := s.auth.ListUsers()
		if err != nil {
			httpError(w, err, 500)
			return
		}
		writeJSON(w, map[string]any{"users": users})

	case http.MethodPost:
		if path != "" {
			http.NotFound(w, r)
			return
		}
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Role     string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if body.Role == "" {
			body.Role = auth.RoleReader
		}
		u, err := s.auth.CreateUser(body.Username, body.Password, body.Role)
		if err != nil {
			if errors.Is(err, auth.ErrExists) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, u)

	case http.MethodDelete:
		id, err := strconv.ParseInt(path, 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		me := userFrom(r.Context())
		if me != nil && me.ID == id {
			http.Error(w, "cannot delete yourself", http.StatusBadRequest)
			return
		}
		if err := s.auth.DeleteUser(id); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	case http.MethodPatch:
		id, err := strconv.ParseInt(path, 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		var body struct {
			Password *string `json:"password"`
			Role     *string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if body.Password != nil {
			if err := s.auth.SetPassword(id, *body.Password); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if body.Role != nil {
			if err := s.auth.SetRole(id, *body.Role); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
