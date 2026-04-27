package middleware

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
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
const preSessionTTL = time.Hour

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

// CreatePreSessionToken creates a short-lived, unauthenticated session entry solely
// to carry a CSRF token into POST /login before a real session exists.
// Sets the session cookie and returns the CSRF token to embed in the login form.
func CreatePreSessionToken(w http.ResponseWriter) string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	id := base64.URLEncoding.EncodeToString(b)

	cb := make([]byte, 32)
	if _, err := rand.Read(cb); err != nil {
		panic(err)
	}
	csrfToken := base64.URLEncoding.EncodeToString(cb)

	store.Store(id, sessionEntry{
		username:  "",
		csrfToken: csrfToken,
		expires:   time.Now().Add(preSessionTTL),
	})

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(preSessionTTL.Seconds()),
		Secure:   secureFlag.Load(),
	})

	return csrfToken
}

// ValidateAndConsumePreSession checks the CSRF token submitted with the login form
// against the pre-session entry, deletes the entry (preventing replay), and returns
// whether the token was valid. Returns false if there is no pre-session cookie,
// if the entry is not a pre-session (non-empty username), or if the token mismatches.
func ValidateAndConsumePreSession(r *http.Request) bool {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}

	val, ok := store.Load(cookie.Value)
	if !ok {
		return false
	}

	entry := val.(sessionEntry)
	if entry.username != "" {
		return false
	}

	// Delete regardless of token match to prevent replay attacks.
	store.Delete(cookie.Value)

	formToken := r.FormValue("csrf_token")
	return subtle.ConstantTimeCompare([]byte(entry.csrfToken), []byte(formToken)) == 1
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

// RequireCSRF validates the CSRF token on the incoming request against the token
// stored in context by LoadSession or RequireAuth. Checks the X-CSRF-Token header
// first (used by HTMX), then falls back to the csrf_token form field.
// Returns 403 if the token is missing or does not match.
func RequireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contextToken := CSRFTokenFromContext(r)

		var requestToken string
		if h := r.Header.Get("X-CSRF-Token"); h != "" {
			requestToken = h
		} else {
			requestToken = r.FormValue("csrf_token")
		}

		if subtle.ConstantTimeCompare([]byte(contextToken), []byte(requestToken)) != 1 {
			http.Error(w, "invalid CSRF token", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
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
