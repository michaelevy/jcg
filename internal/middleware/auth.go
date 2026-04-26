package middleware

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type contextKey string

const (
	// CtxKeyUsername is the context key for the authenticated username.
	CtxKeyUsername contextKey = "username"
	// CtxKeyCSRFToken is the context key for the per-session CSRF token.
	CtxKeyCSRFToken contextKey = "csrf_token"
)

const cookieName = "jcg_session"
const sessionTTL = 24 * time.Hour

type sessionEntry struct {
	username  string
	csrfToken string
	expires   time.Time
}

var store sync.Map // map[string]sessionEntry

var secureFlag atomic.Bool

// SetSecure controls whether the Secure attribute is set on session cookies.
// Call with true at startup when serving over HTTPS.
func SetSecure(v bool) { secureFlag.Store(v) }

// SweepExpiredSessions removes all expired entries from the session store.
// Called periodically by StartSessionSweep; also exported for direct use in tests.
func SweepExpiredSessions() {
	now := time.Now()
	store.Range(func(key, value any) bool {
		if now.After(value.(sessionEntry).expires) {
			store.Delete(key)
		}
		return true
	})
}

// StartSessionSweep launches a background goroutine that calls SweepExpiredSessions
// on the given interval. Call once at startup; runs for the lifetime of the process.
func StartSessionSweep(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			SweepExpiredSessions()
		}
	}()
}

// CreateSession generates a session ID, stores it server-side, and sets the session cookie on w.
func CreateSession(w http.ResponseWriter, username string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failure is not recoverable
	}
	id := base64.URLEncoding.EncodeToString(b)

	cb := make([]byte, 32)
	if _, err := rand.Read(cb); err != nil {
		panic(err)
	}
	csrfToken := base64.URLEncoding.EncodeToString(cb)

	store.Store(id, sessionEntry{username: username, csrfToken: csrfToken, expires: time.Now().Add(sessionTTL)})

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
		Secure:   secureFlag.Load(),
	})
}

// DeleteSession removes the session from the store and clears the cookie.
func DeleteSession(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(cookieName); err == nil {
		store.Delete(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:   cookieName,
		Path:   "/",
		MaxAge: -1,
	})
}

// UsernameFromContext returns the authenticated username injected by RequireAuth,
// or empty string if the request is unauthenticated.
func UsernameFromContext(r *http.Request) string {
	v, _ := r.Context().Value(CtxKeyUsername).(string)
	return v
}

// CSRFTokenFromContext returns the CSRF token injected by LoadSession or RequireAuth,
// or empty string if the request has no valid session.
func CSRFTokenFromContext(r *http.Request) string {
	v, _ := r.Context().Value(CtxKeyCSRFToken).(string)
	return v
}

// InjectUsername injects a username into a context. Used in tests only —
// production code goes through RequireAuth instead.
func InjectUsername(ctx context.Context, username string) context.Context {
	return context.WithValue(ctx, CtxKeyUsername, username)
}

// LoadSession injects the username into context if a valid session cookie exists,
// but does not redirect unauthenticated requests. Use for public routes that
// want to know who's logged in without enforcing auth.
func LoadSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(cookieName); err == nil {
			if val, ok := store.Load(cookie.Value); ok {
				entry := val.(sessionEntry)
				if !time.Now().After(entry.expires) {
					ctx := context.WithValue(r.Context(), CtxKeyUsername, entry.username)
					ctx = context.WithValue(ctx, CtxKeyCSRFToken, entry.csrfToken)
					r = r.WithContext(ctx)
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAuth is middleware that redirects unauthenticated requests to /login.
// Authenticated requests have the username injected into context via CtxKeyUsername.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(cookieName)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		val, ok := store.Load(cookie.Value)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		entry := val.(sessionEntry)
		if time.Now().After(entry.expires) {
			store.Delete(cookie.Value)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), CtxKeyUsername, entry.username)
		ctx = context.WithValue(ctx, CtxKeyCSRFToken, entry.csrfToken)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// StoreTestSession inserts a session into the store for testing purposes.
// Only exported for testing; not for production use.
func StoreTestSession(id string, username string, expires time.Time) {
	store.Store(id, sessionEntry{username: username, expires: expires})
}

// StoreTestCSRFSession inserts a session with a known CSRF token for testing.
// Only exported for testing; not for production use.
func StoreTestCSRFSession(id, username, csrfToken string, expires time.Time) {
	store.Store(id, sessionEntry{username: username, csrfToken: csrfToken, expires: expires})
}

// ResetStore clears all sessions from the store. Used in test cleanup to prevent
// state leakage between tests.
func ResetStore() {
	store.Range(func(key, value interface{}) bool {
		store.Delete(key)
		return true
	})
}
