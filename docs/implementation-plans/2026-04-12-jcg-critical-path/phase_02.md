# JCG — Critical Path Implementation Plan

**Goal:** Add session-based authentication — login/logout, protected routes, and a CLI for creating user accounts.

**Architecture:** In-memory session store (sync.Map) keyed by a 256-bit random session ID stored in an HttpOnly cookie. `RequireAuth` middleware redirects unauthenticated requests to /login. Passwords stored as bcrypt hashes in the `users` table. A seed CLI (`cmd/seed`) creates user accounts outside the web process.

**Tech Stack:** golang.org/x/crypto (bcrypt), crypto/rand, net/http cookie API, sync

**Scope:** Phase 2 of 4 (F2 from design) — builds on Phase 1

**Codebase verified:** 2026-04-12 — Phase 1 complete; `users` table exists in schema, `internal/middleware/auth.go` is a package stub

---

<!-- START_TASK_1 -->
### Task 1: bcrypt Dependency and User Seeder CLI

**Files:**
- Modify: `go.mod` (via `go get`)
- Modify: `cmd/seed/main.go`

**Step 1: Add bcrypt**

```bash
go get golang.org/x/crypto
go mod tidy
```

Expected: `go.mod` updated with `golang.org/x/crypto`. `go.sum` updated. No errors.

**Step 2: Implement seed command**

Replace `cmd/seed/main.go`:
```go
package main

import (
	"flag"
	"fmt"
	"log"

	"golang.org/x/crypto/bcrypt"
	"jcg/internal/db"
)

func main() {
	dsn := flag.String("db", "file:./jcg.db?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000", "SQLite DSN")
	username := flag.String("u", "", "create/update a user account (requires -p)")
	password := flag.String("p", "", "password for the user (used with -u)")
	player := flag.String("player", "", "add a player by name")
	flag.Parse()

	if *username == "" && *player == "" {
		log.Fatal("provide either -u username -p password, or -player name")
	}

	database, err := db.Open(*dsn)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()

	if *player != "" {
		_, err = database.Exec(`INSERT OR IGNORE INTO players (name) VALUES (?)`, *player)
		if err != nil {
			log.Fatalf("insert player: %v", err)
		}
		fmt.Printf("Player %q added (or already exists).\n", *player)
	}

	if *username != "" {
		if *password == "" {
			log.Fatal("-p password is required when using -u")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
		if err != nil {
			log.Fatalf("bcrypt: %v", err)
		}
		_, err = database.Exec(
			`INSERT OR REPLACE INTO users (username, password_hash) VALUES (?, ?)`,
			*username, string(hash),
		)
		if err != nil {
			log.Fatalf("insert user: %v", err)
		}
		fmt.Printf("User %q created/updated.\n", *username)
	}
}
```

**Step 3: Verify compilation**

```bash
go build ./...
```

Expected: Exits silently.

**Step 4: Create the first admin user**

```bash
go run ./cmd/seed -u admin -p changeme
```

Expected:
```
User "admin" created/updated.
```

**Step 5: Commit**

```bash
git add go.mod go.sum cmd/seed/
git commit -m "feat: add bcrypt dependency and user/player seeder CLI"
```
<!-- END_TASK_1 -->

<!-- START_TASK_2 -->
### Task 2: Session Store and Auth Middleware

**Files:**
- Modify: `internal/middleware/auth.go`

**Step 1: Implement session middleware**

Replace `internal/middleware/auth.go`:
```go
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

// CreateSession generates a session ID, stores it server-side, and sets the session cookie on w.
func CreateSession(w http.ResponseWriter, username string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failure is not recoverable
	}
	id := base64.URLEncoding.EncodeToString(b)

	store.Store(id, sessionEntry{username: username, expires: time.Now().Add(sessionTTL)})

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
```

**Step 2: Verify compilation**

```bash
go build ./...
```

Expected: Exits silently.

**Step 3: Commit**

```bash
git add internal/middleware/auth.go
git commit -m "feat: add in-memory session store and RequireAuth middleware"
```
<!-- END_TASK_2 -->

<!-- START_TASK_3 -->
### Task 3: Auth Handlers and Login Template

**Files:**
- Create: `internal/handlers/auth.go`
- Create: `cmd/server/templates/login.html`
- Modify: `cmd/server/main.go` (add login/logout routes and entry placeholder)

**Step 1: Create auth handlers**

Create `internal/handlers/auth.go`:
```go
package handlers

import (
	"net/http"

	"golang.org/x/crypto/bcrypt"
	"jcg/internal/middleware"
)

func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, "login", map[string]any{
		"Title": "Login",
	})
}

func (h *Handler) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	var hash string
	err := h.db.QueryRow(`SELECT password_hash FROM users WHERE username = ?`, username).Scan(&hash)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		h.render(w, "login", map[string]any{
			"Title": "Login",
			"Error": "Invalid username or password.",
		})
		return
	}

	middleware.CreateSession(w, username)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	middleware.DeleteSession(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
```

**Step 2: Create login template**

Create `cmd/server/templates/login.html`:
```html
{{define "login"}}
<!DOCTYPE html>
<html lang="en">
<head>{{template "head" .}}</head>
<body>
  {{template "nav" .}}
  <main>
    <h1>Login</h1>
    {{if .Error}}
      <p class="error">{{.Error}}</p>
    {{end}}
    <form method="POST" action="/login" class="form-narrow">
      <label for="username">Username</label>
      <input id="username" name="username" type="text" required autofocus>

      <label for="password">Password</label>
      <input id="password" name="password" type="password" required>

      <button type="submit">Login</button>
    </form>
  </main>
</body>
</html>
{{end}}
```

**Step 3: Add auth routes to main.go**

In `cmd/server/main.go`, add to the import block:
```go
"jcg/internal/middleware"
```

Add these routes after the existing `mux.HandleFunc("GET /{$}", h.Home)` line:
```go
mux.HandleFunc("GET /login", h.LoginPage)
mux.HandleFunc("POST /login", h.LoginSubmit)
mux.HandleFunc("POST /logout", h.Logout)

// /enter is protected — placeholder until Phase 3 replaces it.
mux.Handle("GET /enter", middleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    http.Error(w, "entry coming in phase 3", http.StatusNotImplemented)
})))
```

**Step 4: Verify login flow works end-to-end**

```bash
go run ./cmd/server &
sleep 1
curl -s http://localhost:8080/login | grep "Login"
```
Expected: Returns HTML containing "Login".

```bash
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/enter
```
Expected: `303` (redirect to /login).

```bash
kill %1
```

**Step 5: Commit**

```bash
git add internal/handlers/auth.go cmd/server/templates/login.html cmd/server/main.go
git commit -m "feat: add login/logout handlers, session cookies, and protected route placeholder"
```
<!-- END_TASK_3 -->

<!-- START_TASK_4 -->
### Task 4: Auth Handler Tests and RequireAuth Middleware Tests

**Files:**
- Create: `internal/handlers/auth_test.go`
- Create: `internal/middleware/auth_test.go`

**Step 1: Write auth handler tests**

Create `internal/handlers/auth_test.go`:
```go
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
```

**Step 2: Write RequireAuth middleware tests**

Create `internal/middleware/auth_test.go`:
```go
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jcg/internal/middleware"
)

func TestRequireAuth_NoCookie_RedirectsToLogin(t *testing.T) {
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
```

**Step 3: Run all tests**

```bash
go test ./...
```

Expected: All tests pass. Fix any failures before proceeding.

**Step 4: Commit**

```bash
git add internal/handlers/auth_test.go internal/middleware/auth_test.go
git commit -m "test: add auth handler tests and RequireAuth middleware tests"
```
<!-- END_TASK_4 -->
