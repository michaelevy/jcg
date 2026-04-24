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

func TestSweepExpiredSessions_RemovesOnlyExpiredEntries(t *testing.T) {
	t.Cleanup(func() { middleware.ResetStore() })

	middleware.StoreTestSession("expired-id", "alice", time.Now().Add(-time.Hour))
	middleware.StoreTestSession("valid-id", "bob", time.Now().Add(time.Hour))

	middleware.SweepExpiredSessions()

	// Expired session should not load into context after sweep.
	var expiredUsername string
	expiredReq := httptest.NewRequest("GET", "/", nil)
	expiredReq.AddCookie(&http.Cookie{Name: "jcg_session", Value: "expired-id"})
	middleware.LoadSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expiredUsername = middleware.UsernameFromContext(r)
	})).ServeHTTP(httptest.NewRecorder(), expiredReq)
	if expiredUsername != "" {
		t.Errorf("expired session should not load after sweep, got %q", expiredUsername)
	}

	// Valid session should still load.
	var validUsername string
	validReq := httptest.NewRequest("GET", "/", nil)
	validReq.AddCookie(&http.Cookie{Name: "jcg_session", Value: "valid-id"})
	middleware.LoadSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		validUsername = middleware.UsernameFromContext(r)
	})).ServeHTTP(httptest.NewRecorder(), validReq)
	if validUsername != "bob" {
		t.Errorf("valid session should survive sweep, got %q", validUsername)
	}
}

func TestCreateSession_SecureFlagOff_CookieNotSecure(t *testing.T) {
	t.Cleanup(func() {
		middleware.ResetStore()
		middleware.SetSecure(false)
	})

	w := httptest.NewRecorder()
	middleware.CreateSession(w, "alice")

	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected a session cookie to be set")
	}
	if cookies[0].Secure {
		t.Error("expected Secure=false when secure flag is off")
	}
}

func TestCreateSession_SecureFlagOn_CookieIsSecure(t *testing.T) {
	t.Cleanup(func() {
		middleware.ResetStore()
		middleware.SetSecure(false)
	})

	middleware.SetSecure(true)
	w := httptest.NewRecorder()
	middleware.CreateSession(w, "alice")

	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected a session cookie to be set")
	}
	if !cookies[0].Secure {
		t.Error("expected Secure=true when secure flag is on")
	}
}

func TestCreateSession_GeneratesNonEmptyCSRFToken(t *testing.T) {
	t.Cleanup(func() { middleware.ResetStore() })

	// Create a session and recover its cookie.
	wSetup := httptest.NewRecorder()
	middleware.CreateSession(wSetup, "alice")
	cookie := wSetup.Result().Cookies()[0]

	// LoadSession should inject a non-empty CSRF token into context.
	var capturedToken string
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)
	middleware.LoadSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedToken = middleware.CSRFTokenFromContext(r)
	})).ServeHTTP(httptest.NewRecorder(), req)

	if capturedToken == "" {
		t.Error("expected a non-empty CSRF token in context after LoadSession")
	}
}

func TestRequireAuth_InjectsCSRFToken(t *testing.T) {
	t.Cleanup(func() { middleware.ResetStore() })

	wSetup := httptest.NewRecorder()
	middleware.CreateSession(wSetup, "alice")
	cookie := wSetup.Result().Cookies()[0]

	var capturedToken string
	req := httptest.NewRequest("GET", "/enter", nil)
	req.AddCookie(cookie)
	middleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedToken = middleware.CSRFTokenFromContext(r)
	})).ServeHTTP(httptest.NewRecorder(), req)

	if capturedToken == "" {
		t.Error("expected a non-empty CSRF token in context after RequireAuth")
	}
}

func TestStoreTestCSRFSession_TokenRoundtrips(t *testing.T) {
	t.Cleanup(func() { middleware.ResetStore() })

	middleware.StoreTestCSRFSession("my-id", "bob", "known-token", time.Now().Add(time.Hour))

	var capturedToken string
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "jcg_session", Value: "my-id"})
	middleware.LoadSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedToken = middleware.CSRFTokenFromContext(r)
	})).ServeHTTP(httptest.NewRecorder(), req)

	if capturedToken != "known-token" {
		t.Errorf("expected CSRF token %q, got %q", "known-token", capturedToken)
	}
}
