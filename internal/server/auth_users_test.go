package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"libshelf/internal/auth"
	"libshelf/internal/server"
	"libshelf/internal/store"
)

func TestCreateUserAsAdmin(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "libshelf.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	a, err := auth.Open(filepath.Join(dir, "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })

	if _, _, err := a.BootstrapAdmin("admin", "adminpass"); err != nil {
		t.Fatal(err)
	}

	srv := server.New(server.Options{
		Store:        st,
		Auth:         a,
		AuthRequired: true,
		LibDir:       dir,
		CoverDir:     dir,
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	loginBody, _ := json.Marshal(map[string]string{"username": "admin", "password": "adminpass"})
	res, err := http.Post(ts.URL+"/api/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login %d: %s", res.StatusCode, body)
	}
	cookies := res.Cookies()
	if len(cookies) == 0 {
		t.Fatal("no session cookie")
	}

	createBody, _ := json.Marshal(map[string]string{
		"username": "test",
		"password": "testpass",
		"role":     "reader",
	})
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/users", bytes.NewReader(createBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	res2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(res2.Body)
	res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("create %d: %s", res2.StatusCode, out)
	}

	listReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/users", nil)
	for _, c := range cookies {
		listReq.AddCookie(c)
	}
	listRes, err := http.DefaultClient.Do(listReq)
	if err != nil {
		t.Fatal(err)
	}
	listOut, _ := io.ReadAll(listRes.Body)
	listRes.Body.Close()
	if listRes.StatusCode != http.StatusOK {
		t.Fatalf("list %d: %s", listRes.StatusCode, listOut)
	}
	if !bytes.Contains(listOut, []byte(`"test"`)) {
		t.Fatalf("expected test user in list: %s", listOut)
	}
}
