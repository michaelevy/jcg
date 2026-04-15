package handlers

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
	"jcg/internal/db"
)

// testHandler returns a Handler backed by an in-memory SQLite database.
// Inline templates avoid filesystem dependency — they test behavior, not rendering.
func testHandler(t *testing.T) *Handler {
	t.Helper()
	database, err := db.Open("file::memory:?cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	tmpl := template.Must(template.New("root").Parse(`
		{{define "login"}}LOGIN{{if .Error}}:{{.Error}}{{end}}{{end}}
		{{define "home"}}HOME{{end}}
	`))

	return New(database, tmpl)
}

func seedUser(t *testing.T, h *Handler, username, password string) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.Exec(`INSERT INTO users (username, password_hash) VALUES (?, ?)`, username, string(hash)); err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

func TestLoginPage_ReturnsOK(t *testing.T) {
	h := testHandler(t)
	r := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()

	h.LoginPage(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "LOGIN") {
		t.Errorf("want LOGIN in body, got: %s", w.Body.String())
	}
}

func TestLoginSubmit_ValidCredentials_RedirectsAndSetsCookie(t *testing.T) {
	h := testHandler(t)
	seedUser(t, h, "alice", "hunter2")

	form := url.Values{"username": {"alice"}, "password": {"hunter2"}}
	r := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.LoginSubmit(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("want 303, got %d", w.Code)
	}
	if w.Header().Get("Location") != "/" {
		t.Errorf("want Location /, got %s", w.Header().Get("Location"))
	}
	var hasCookie bool
	for _, c := range w.Result().Cookies() {
		if c.Name == "jcg_session" {
			hasCookie = true
		}
	}
	if !hasCookie {
		t.Error("want jcg_session cookie in response, got none")
	}
}

func TestLoginSubmit_WrongPassword_ReRendersWithError(t *testing.T) {
	h := testHandler(t)
	seedUser(t, h, "alice", "hunter2")

	form := url.Values{"username": {"alice"}, "password": {"wrong"}}
	r := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.LoginSubmit(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 (re-render), got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Invalid") {
		t.Errorf("want error message in body, got: %s", w.Body.String())
	}
}

func TestLoginSubmit_UnknownUser_ReRendersWithError(t *testing.T) {
	h := testHandler(t)

	form := url.Values{"username": {"nobody"}, "password": {"anything"}}
	r := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.LoginSubmit(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 (re-render), got %d", w.Code)
	}
}

func TestLogout_RedirectsToLogin(t *testing.T) {
	h := testHandler(t)

	r := httptest.NewRequest("POST", "/logout", nil)
	w := httptest.NewRecorder()

	h.Logout(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("want 303, got %d", w.Code)
	}
	if w.Header().Get("Location") != "/login" {
		t.Errorf("want Location /login, got %s", w.Header().Get("Location"))
	}
}
