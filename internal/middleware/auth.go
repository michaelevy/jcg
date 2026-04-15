package middleware

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"
)

type contextKey string

// CtxKeyUsername is the context key for the authenticated username.
const CtxKeyUsername contextKey = "username"

const cookieName = "jcg_session"
const sessionTTL = 24 * time.Hour

type sessionEntry struct {
	username string
	expires  time.Time
}

var store sync.Map // map[string]sessionEntry
// TODO: add background expiry sweep to prevent unbounded memory growth

// CreateSession generates a session ID, stores it server-side, and sets the session cookie on w.
func CreateSession(w http.ResponseWriter, username string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failure is not recoverable
	}
	id := base64.URLEncoding.EncodeToString(b)

	store.Store(id, sessionEntry{username: username, expires: time.Now().Add(sessionTTL)})

	// TODO: set Secure: true in production (currently HTTP-only dev environment)
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
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

// InjectUsername injects a username into a context. Used in tests only —
// production code goes through RequireAuth instead.
func InjectUsername(ctx context.Context, username string) context.Context {
	return context.WithValue(ctx, CtxKeyUsername, username)
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
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// StoreTestSession inserts a session into the store for testing purposes.
// Only exported for testing; not for production use.
func StoreTestSession(id string, username string, expires time.Time) {
	store.Store(id, sessionEntry{username: username, expires: expires})
}

// ResetStore clears all sessions from the store. Used in test cleanup to prevent
// state leakage between tests.
func ResetStore() {
	store.Range(func(key, value interface{}) bool {
		store.Delete(key)
		return true
	})
}
