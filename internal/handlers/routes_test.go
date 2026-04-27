package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"jcg/internal/db"
	"jcg/internal/handlers"
	"jcg/internal/middleware"
)

// TestPostLoginWithMiddlewareChain simulates the actual middleware chain from main.go
// for POST /login and verifies it doesn't 403 with valid pre-session credentials.
func TestPostLoginWithMiddlewareChain(t *testing.T) {
	t.Cleanup(func() { middleware.ResetStore() })

	database, err := db.Open("file::memory:?cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer database.Close()

	// Seed a test user
	hash, err := bcrypt.GenerateFromPassword([]byte("hunter2"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO users (username, password_hash) VALUES (?, ?)`, "alice", string(hash)); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	h := handlers.New(database, nil) // template.Template not needed for this test

	// Create a pre-session (as done by LoginPage)
	wSetup := httptest.NewRecorder()
	csrfToken := middleware.CreatePreSessionToken(wSetup)
	preSessionCookie := wSetup.Result().Cookies()[0]

	// Apply the middleware chain from main.go:
	// mux.Handle("POST /login", middleware.LoadSession(http.HandlerFunc(h.LoginSubmit)))
	handler := middleware.LoadSession(http.HandlerFunc(h.LoginSubmit))

	// POST to login with the pre-session cookie and CSRF token
	form := strings.NewReader("username=alice&password=hunter2&csrf_token=" + csrfToken)
	req := httptest.NewRequest("POST", "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(preSessionCookie)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code == http.StatusForbidden {
		t.Errorf("POST /login with valid pre-session and CSRF token returned 403 (CSRF check failed)")
		t.Logf("This indicates RequireCSRF is rejecting the middleware chain improperly")
	}
	if w.Code != http.StatusSeeOther {
		t.Errorf("POST /login should redirect (303) on success, got %d", w.Code)
	}
}

// TestPostLogoutWithMiddlewareChain verifies POST /logout works with the full middleware chain.
func TestPostLogoutWithMiddlewareChain(t *testing.T) {
	t.Cleanup(func() { middleware.ResetStore() })

	database, err := db.Open("file::memory:?cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer database.Close()

	h := handlers.New(database, nil)

	// Create a real session
	wSession := httptest.NewRecorder()
	middleware.CreateSession(wSession, "alice")
	sessionCookie := wSession.Result().Cookies()[0]

	// Store a session with a known CSRF token for this test
	const testCSRFToken = "test-token"
	middleware.StoreTestCSRFSession(sessionCookie.Value, "alice", testCSRFToken, time.Now().Add(24*time.Hour))
	csrfToken := testCSRFToken

	// Apply the middleware chain from main.go:
	// mux.Handle("POST /logout", middleware.LoadSession(middleware.RequireCSRF(http.HandlerFunc(h.Logout))))
	handler := middleware.LoadSession(middleware.RequireCSRF(http.HandlerFunc(h.Logout)))

	// POST to logout with valid session cookie and CSRF token
	form := strings.NewReader("csrf_token=" + csrfToken)
	req := httptest.NewRequest("POST", "/logout", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code == http.StatusForbidden {
		t.Errorf("POST /logout with valid session and CSRF token returned 403")
	}
	if w.Code != http.StatusSeeOther {
		t.Errorf("POST /logout should redirect (303), got %d", w.Code)
	}
}

// TestPostLogoutWithWrongCSRFToken verifies POST /logout rejects wrong CSRF tokens.
func TestPostLogoutWithWrongCSRFToken(t *testing.T) {
	t.Cleanup(func() { middleware.ResetStore() })

	database, err := db.Open("file::memory:?cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer database.Close()

	h := handlers.New(database, nil)

	// Create a real session with a known CSRF token
	middleware.StoreTestCSRFSession("session-id", "alice", "correct-token", time.Now().Add(24*time.Hour))

	// Apply the middleware chain from main.go
	handler := middleware.LoadSession(middleware.RequireCSRF(http.HandlerFunc(h.Logout)))

	// POST to logout with wrong CSRF token
	form := strings.NewReader("csrf_token=wrong-token")
	req := httptest.NewRequest("POST", "/logout", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "jcg_session", Value: "session-id"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("POST /logout with wrong CSRF token should return 403, got %d", w.Code)
	}

	// Verify the inner Logout handler was NOT reached by checking for the error message
	if !strings.Contains(w.Body.String(), "invalid CSRF token") {
		t.Error("response should contain 'invalid CSRF token' error message from RequireCSRF")
	}
}
