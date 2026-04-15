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
	"jcg/internal/middleware"
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
	if !strings.Contains(w.Body.String(), "Invalid") {
		t.Errorf("want error message in body, got: %s", w.Body.String())
	}
}

func TestLogout_RedirectsToLogin(t *testing.T) {
	h := testHandler(t)
	t.Cleanup(func() { middleware.ResetStore() })

	// Create a session
	wSession := httptest.NewRecorder()
	middleware.CreateSession(wSession, "alice")
	sessionCookie := wSession.Result().Cookies()[0]

	// Call logout with the session cookie
	r := httptest.NewRequest("POST", "/logout", nil)
	r.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	h.Logout(w, r)

	// Should redirect to /login
	if w.Code != http.StatusSeeOther {
		t.Errorf("want 303, got %d", w.Code)
	}
	if w.Header().Get("Location") != "/login" {
		t.Errorf("want Location /login, got %s", w.Header().Get("Location"))
	}

	// Response should have a Set-Cookie header that clears the session (MaxAge <= 0)
	var hasClearingCookie bool
	for _, c := range w.Result().Cookies() {
		if c.Name == "jcg_session" && c.MaxAge < 0 {
			hasClearingCookie = true
		}
	}
	if !hasClearingCookie {
		t.Error("want Set-Cookie header with MaxAge < 0 to clear session cookie")
	}

	// Verify that a subsequent request with the same cookie redirects to /login
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("reached"))
	})
	protected := middleware.RequireAuth(inner)

	rCheck := httptest.NewRequest("GET", "/enter", nil)
	rCheck.AddCookie(sessionCookie)
	wCheck := httptest.NewRecorder()

	protected.ServeHTTP(wCheck, rCheck)

	if wCheck.Code != http.StatusSeeOther {
		t.Errorf("subsequent request should redirect, got %d", wCheck.Code)
	}
	if wCheck.Header().Get("Location") != "/login" {
		t.Errorf("subsequent request should redirect to /login, got %s", wCheck.Header().Get("Location"))
	}
}
