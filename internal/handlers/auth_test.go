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
	t.Cleanup(func() { middleware.ResetStore() })
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
	t.Cleanup(func() { middleware.ResetStore() })
	h := testHandler(t)
	seedUser(t, h, "alice", "hunter2")

	wSetup := httptest.NewRecorder()
	token := middleware.CreatePreSessionToken(wSetup)
	preSessionCookie := wSetup.Result().Cookies()[0]

	form := url.Values{"username": {"alice"}, "password": {"hunter2"}, "csrf_token": {token}}
	r := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(preSessionCookie)
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
	t.Cleanup(func() { middleware.ResetStore() })
	h := testHandler(t)
	seedUser(t, h, "alice", "hunter2")

	wSetup := httptest.NewRecorder()
	token := middleware.CreatePreSessionToken(wSetup)
	preSessionCookie := wSetup.Result().Cookies()[0]

	form := url.Values{"username": {"alice"}, "password": {"wrong"}, "csrf_token": {token}}
	r := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(preSessionCookie)
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
	t.Cleanup(func() { middleware.ResetStore() })
	h := testHandler(t)

	wSetup := httptest.NewRecorder()
	token := middleware.CreatePreSessionToken(wSetup)
	preSessionCookie := wSetup.Result().Cookies()[0]

	form := url.Values{"username": {"nobody"}, "password": {"anything"}, "csrf_token": {token}}
	r := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(preSessionCookie)
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

func TestLoginPage_SetsPreSessionCookie(t *testing.T) {
	t.Cleanup(func() { middleware.ResetStore() })
	h := testHandler(t)

	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()
	h.LoginPage(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Error("expected a session cookie to be set by LoginPage")
	}
}

func TestLoginSubmit_MissingCSRFToken_RedirectsToLogin(t *testing.T) {
	t.Cleanup(func() { middleware.ResetStore() })
	h := testHandler(t)

	wSetup := httptest.NewRecorder()
	middleware.CreatePreSessionToken(wSetup)
	preSessionCookie := wSetup.Result().Cookies()[0]

	// Submit with no csrf_token field.
	body := strings.NewReader("username=admin&password=secret")
	req := httptest.NewRequest("POST", "/login", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(preSessionCookie)
	w := httptest.NewRecorder()
	h.LoginSubmit(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("want 303 redirect to /login on CSRF failure, got %d", w.Code)
	}
	if w.Header().Get("Location") != "/login" {
		t.Errorf("want redirect to /login, got %s", w.Header().Get("Location"))
	}
}

func TestLoginSubmit_WrongCSRFToken_RedirectsToLogin(t *testing.T) {
	t.Cleanup(func() { middleware.ResetStore() })
	h := testHandler(t)

	wSetup := httptest.NewRecorder()
	middleware.CreatePreSessionToken(wSetup)
	preSessionCookie := wSetup.Result().Cookies()[0]

	body := strings.NewReader("username=admin&password=secret&csrf_token=wrong")
	req := httptest.NewRequest("POST", "/login", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(preSessionCookie)
	w := httptest.NewRecorder()
	h.LoginSubmit(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("want 303 redirect to /login on CSRF failure, got %d", w.Code)
	}
	if w.Header().Get("Location") != "/login" {
		t.Errorf("want redirect to /login, got %s", w.Header().Get("Location"))
	}
}

func TestLoginPage_AlreadyAuthenticated_RedirectsToHome(t *testing.T) {
	t.Cleanup(func() { middleware.ResetStore() })
	h := testHandler(t)

	// Create a real session for "alice"
	wSession := httptest.NewRecorder()
	middleware.CreateSession(wSession, "alice")
	sessionCookie := wSession.Result().Cookies()[0]

	// Hit GET /login through the LoadSession wrapper chain
	req := httptest.NewRequest("GET", "/login", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	// Manually apply LoadSession to the handler (simulating the routing)
	handler := middleware.LoadSession(http.HandlerFunc(h.LoginPage))
	handler.ServeHTTP(w, req)

	// Should redirect to home
	if w.Code != http.StatusSeeOther {
		t.Errorf("want 303, got %d", w.Code)
	}
	if w.Header().Get("Location") != "/" {
		t.Errorf("want Location /, got %s", w.Header().Get("Location"))
	}
}
