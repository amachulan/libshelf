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
	if !bytes.Contains(listOut, []byte(`"admin"`)) {
		t.Fatalf("expected admin user in list: %s", listOut)
	}

	// Second create must 409, list must still include both.
	req2, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/users", bytes.NewReader(createBody))
	req2.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req2.AddCookie(c)
	}
	res3, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	res3.Body.Close()
	if res3.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate create want 409, got %d", res3.StatusCode)
	}
}

func TestLoginSwitchDropsStaleSessionCookie(t *testing.T) {
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
	if _, err := a.CreateUser("test", "testpass", auth.RoleReader); err != nil {
		t.Fatal(err)
	}

	srv := server.New(server.Options{
		Store: st, Auth: a, AuthRequired: true, LibDir: dir, CoverDir: dir,
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	adminLogin, _ := json.Marshal(map[string]string{"username": "admin", "password": "adminpass"})
	adminRes, err := http.Post(ts.URL+"/api/login", "application/json", bytes.NewReader(adminLogin))
	if err != nil {
		t.Fatal(err)
	}
	adminRes.Body.Close()
	var adminCookie *http.Cookie
	for _, c := range adminRes.Cookies() {
		if c.Name == auth.CookieName() && c.Value != "" {
			adminCookie = c
			break
		}
	}
	if adminCookie == nil {
		t.Fatal("no admin cookie")
	}

	testLogin, _ := json.Marshal(map[string]string{"username": "test", "password": "testpass"})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/login", bytes.NewReader(testLogin))
	req.Header.Set("Content-Type", "application/json")
	// Simulate a sticky previous cookie still attached while switching users.
	req.AddCookie(adminCookie)
	req.AddCookie(&http.Cookie{Name: auth.LegacyCookieName(), Value: adminCookie.Value})
	loginRes, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	loginBody, _ := io.ReadAll(loginRes.Body)
	loginRes.Body.Close()
	if loginRes.StatusCode != http.StatusOK {
		t.Fatalf("login %d: %s", loginRes.StatusCode, loginBody)
	}

	var testCookie *http.Cookie
	for _, c := range loginRes.Cookies() {
		if c.Name == auth.CookieName() && c.Value != "" && c.MaxAge >= 0 {
			testCookie = c
		}
	}
	if testCookie == nil {
		t.Fatal("no test session cookie")
	}

	meReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/me", nil)
	// Browser may still send the old cookie alongside the new one; last valid wins.
	meReq.AddCookie(adminCookie)
	meReq.AddCookie(testCookie)
	meRes, err := http.DefaultClient.Do(meReq)
	if err != nil {
		t.Fatal(err)
	}
	meBody, _ := io.ReadAll(meRes.Body)
	meRes.Body.Close()
	if meRes.StatusCode != http.StatusOK {
		t.Fatalf("me %d: %s", meRes.StatusCode, meBody)
	}
	if !bytes.Contains(meBody, []byte(`"username":"test"`)) {
		t.Fatalf("expected session user test, got %s", meBody)
	}
}
