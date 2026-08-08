package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

const (
	RoleAdmin  = "admin"
	RoleReader = "reader"

	sessionTTL   = 30 * 24 * time.Hour
	cookieName   = "libshelf_session"
	bcryptCost   = 10
)

var (
	ErrInvalidCreds = errors.New("invalid username or password")
	ErrForbidden    = errors.New("forbidden")
	ErrNotFound     = errors.New("not found")
	ErrExists       = errors.New("user already exists")
)

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

type Auth struct {
	db *sql.DB
}

func CookieName() string { return cookieName }

func Open(path string) (*Auth, error) {
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	a := &Auth{db: db}
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY,
  username TEXT NOT NULL UNIQUE COLLATE NOCASE,
  pass_hash TEXT NOT NULL,
  role TEXT NOT NULL CHECK(role IN ('admin','reader')),
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  token TEXT PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_exp ON sessions(expires_at);
`); err != nil {
		db.Close()
		return nil, err
	}
	if err := a.ensureShelfSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return a, nil
}

func (a *Auth) Close() error { return a.db.Close() }

func (a *Auth) UserCount() (int, error) {
	var n int
	err := a.db.QueryRow(`SELECT count(*) FROM users`).Scan(&n)
	return n, err
}

func ValidRole(role string) bool {
	return role == RoleAdmin || role == RoleReader
}

func (a *Auth) BootstrapAdmin(username, password string) (*User, string, error) {
	n, err := a.UserCount()
	if err != nil {
		return nil, "", err
	}
	if n > 0 {
		return nil, "", nil
	}
	username = strings.TrimSpace(username)
	if username == "" {
		username = "admin"
	}
	generated := ""
	if password == "" {
		generated, err = randomPassword(12)
		if err != nil {
			return nil, "", err
		}
		password = generated
	}
	u, err := a.CreateUser(username, password, RoleAdmin)
	if err != nil {
		return nil, "", err
	}
	return u, generated, nil
}

func (a *Auth) CreateUser(username, password, role string) (*User, error) {
	username = strings.TrimSpace(username)
	role = strings.ToLower(strings.TrimSpace(role))
	if username == "" || password == "" {
		return nil, fmt.Errorf("username and password required")
	}
	if !ValidRole(role) {
		return nil, fmt.Errorf("invalid role")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return nil, err
	}
	res, err := a.db.Exec(
		`INSERT INTO users(username, pass_hash, role, created_at) VALUES(?,?,?,?)`,
		username, string(hash), role, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrExists
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &User{ID: id, Username: username, Role: role}, nil
}

func (a *Auth) ListUsers() ([]User, error) {
	rows, err := a.db.Query(`SELECT id, username, role FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (a *Auth) DeleteUser(id int64) error {
	var role string
	err := a.db.QueryRow(`SELECT role FROM users WHERE id = ?`, id).Scan(&role)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if role == RoleAdmin {
		var admins int
		if err := a.db.QueryRow(`SELECT count(*) FROM users WHERE role = 'admin'`).Scan(&admins); err != nil {
			return err
		}
		if admins <= 1 {
			return fmt.Errorf("cannot delete the last admin")
		}
	}
	res, err := a.db.Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (a *Auth) SetPassword(id int64, password string) error {
	if password == "" {
		return fmt.Errorf("password required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return err
	}
	res, err := a.db.Exec(`UPDATE users SET pass_hash = ? WHERE id = ?`, string(hash), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	_, _ = a.db.Exec(`DELETE FROM sessions WHERE user_id = ?`, id)
	return nil
}

func (a *Auth) SetRole(id int64, role string) error {
	if !ValidRole(role) {
		return fmt.Errorf("invalid role")
	}
	var cur string
	err := a.db.QueryRow(`SELECT role FROM users WHERE id = ?`, id).Scan(&cur)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if cur == RoleAdmin && role != RoleAdmin {
		var admins int
		if err := a.db.QueryRow(`SELECT count(*) FROM users WHERE role = 'admin'`).Scan(&admins); err != nil {
			return err
		}
		if admins <= 1 {
			return fmt.Errorf("cannot demote the last admin")
		}
	}
	_, err = a.db.Exec(`UPDATE users SET role = ? WHERE id = ?`, role, id)
	return err
}

// Authenticate checks username/password without creating a session (for HTTP Basic / OPDS).
func (a *Auth) Authenticate(username, password string) (*User, error) {
	var u User
	var hash string
	err := a.db.QueryRow(
		`SELECT id, username, role, pass_hash FROM users WHERE username = ? COLLATE NOCASE`,
		strings.TrimSpace(username),
	).Scan(&u.ID, &u.Username, &u.Role, &hash)
	if err == sql.ErrNoRows {
		return nil, ErrInvalidCreds
	}
	if err != nil {
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return nil, ErrInvalidCreds
	}
	return &u, nil
}

func (a *Auth) Login(username, password string) (token string, user *User, err error) {
	u, err := a.Authenticate(username, password)
	if err != nil {
		return "", nil, err
	}
	token, err = randomToken(32)
	if err != nil {
		return "", nil, err
	}
	exp := time.Now().Add(sessionTTL).Unix()
	if _, err := a.db.Exec(`INSERT INTO sessions(token, user_id, expires_at) VALUES(?,?,?)`, token, u.ID, exp); err != nil {
		return "", nil, err
	}
	return token, u, nil
}

func (a *Auth) Logout(token string) {
	if token == "" {
		return
	}
	_, _ = a.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
}

func (a *Auth) UserByToken(token string) (*User, error) {
	if token == "" {
		return nil, ErrInvalidCreds
	}
	now := time.Now().Unix()
	_, _ = a.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, now)
	var u User
	err := a.db.QueryRow(`
SELECT u.id, u.username, u.role
FROM sessions s JOIN users u ON u.id = s.user_id
WHERE s.token = ? AND s.expires_at >= ?`, token, now).Scan(&u.ID, &u.Username, &u.Role)
	if err == sql.ErrNoRows {
		return nil, ErrInvalidCreds
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func randomPassword(n int) (string, error) {
	const alphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b), nil
}

// EnvBootstrap reads LIBSHELF_ADMIN_USER / LIBSHELF_ADMIN_PASS.
func EnvBootstrap() (user, pass string) {
	return os.Getenv("LIBSHELF_ADMIN_USER"), os.Getenv("LIBSHELF_ADMIN_PASS")
}
