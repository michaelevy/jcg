package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"jcg/internal/middleware"
)

func TestRequireAuth_NoCookie_RedirectsToLogin(t *testing.T) {
	t.Cleanup(func() { middleware.ResetStore() })

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("reached"))
	})
	handler := middleware.RequireAuth(inner)

	r := httptest.NewRequest("GET", "/enter", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("want 303, got %d", w.Code)
	}
	if w.Header().Get("Location") != "/login" {
		t.Errorf("want redirect to /login, got %s", w.Header().Get("Location"))
	}
	if strings.Contains(w.Body.String(), "reached") {
		t.Error("inner handler should NOT have been called")
	}
}

func TestRequireAuth_ValidSession_PassesThrough(t *testing.T) {
	t.Cleanup(func() { middleware.ResetStore() })

	// Create a session by calling CreateSession and extracting the resulting cookie.
	wSetup := httptest.NewRecorder()
	middleware.CreateSession(wSetup, "alice")
	cookie := wSetup.Result().Cookies()[0]

	var capturedUsername string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUsername = middleware.UsernameFromContext(r)
		w.Write([]byte("reached"))
	})
	handler := middleware.RequireAuth(inner)

	r := httptest.NewRequest("GET", "/enter", nil)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "reached") {
		t.Error("inner handler should have been called")
	}
	if capturedUsername != "alice" {
		t.Errorf("want username alice in context, got %q", capturedUsername)
	}
}

func TestRequireAuth_InvalidSessionID_RedirectsToLogin(t *testing.T) {
	t.Cleanup(func() { middleware.ResetStore() })

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("reached"))
	})
	handler := middleware.RequireAuth(inner)

	r := httptest.NewRequest("GET", "/enter", nil)
	r.AddCookie(&http.Cookie{Name: "jcg_session", Value: "bogus-session-id"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("want 303, got %d", w.Code)
	}
}

func TestRequireAuth_ExpiredSession_RedirectsAndDeletesSession(t *testing.T) {
	t.Cleanup(func() { middleware.ResetStore() })

	// Create an expired session
	sessionID := "expired-session-id"
	middleware.StoreTestSession(sessionID, "alice", time.Now().Add(-1*time.Hour))

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("reached"))
	})
	handler := middleware.RequireAuth(inner)

	r := httptest.NewRequest("GET", "/enter", nil)
	r.AddCookie(&http.Cookie{Name: "jcg_session", Value: sessionID})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	// Should redirect to login
	if w.Code != http.StatusSeeOther {
		t.Errorf("want 303, got %d", w.Code)
	}
	if w.Header().Get("Location") != "/login" {
		t.Errorf("want redirect to /login, got %s", w.Header().Get("Location"))
	}

	// Inner handler should NOT have been called
	if strings.Contains(w.Body.String(), "reached") {
		t.Error("inner handler should NOT have been called for expired session")
	}
}
