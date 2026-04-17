# JCG — Critical Path Implementation Plan

**Goal:** Bootstrap a working Go+SQLite+HTMX web server with Docker support.

**Architecture:** Single Go binary serving server-rendered HTML via net/http. SQLite via mattn/go-sqlite3 for persistence (CGO required). Templates and static assets embedded into the binary at build time via `embed.FS` — they live under `cmd/server/` so the `//go:embed` directive works (Go embed does not allow `..` path components). Multi-stage Debian Docker image, dynamically linked (simpler and more reliable with glibc CGO than static linking).

**Tech Stack:** Go 1.22+, mattn/go-sqlite3, net/http, html/template, embed, Docker/docker-compose

**Scope:** Phase 1 of 4 (F1 from design)

**Codebase verified:** 2026-04-12 — blank slate, only design.md exists

---

<!-- START_TASK_1 -->
### Task 1: Go Module, Dependencies, and Project Skeleton

**Files:**
- Create: `go.mod` (via `go mod init`)
- Create: `.gitignore`
- Create: directory tree `cmd/server/`, `cmd/server/templates/`, `cmd/server/static/`, `cmd/seed/`, `internal/db/`, `internal/handlers/`, `internal/middleware/`

**Step 1: Initialize the module**

> **Windows note:** mattn/go-sqlite3 requires CGO and a C compiler (gcc). Install [TDM-GCC](https://jmeubank.github.io/tdm-gcc/) or develop inside WSL or Docker to avoid this requirement. On Linux/Mac, gcc is typically available.

```bash
go mod init jcg
mkdir -p cmd/server/templates cmd/server/static cmd/seed internal/db internal/handlers internal/middleware
```

Expected: `go.mod` created with `module jcg` and current Go version. Directories created.

**Step 2: Add SQLite driver**

```bash
go get github.com/mattn/go-sqlite3
go mod tidy
```

Expected: `go.mod` updated with `require github.com/mattn/go-sqlite3`. `go.sum` created. No errors.

**Step 3: Create .gitignore**

Create `.gitignore`:
```
*.db
*.db-wal
*.db-shm
/server
/cmd/server/server
.env
.DS_Store
```

**Step 4: Create placeholder Go files so the module builds**

Create `cmd/server/main.go`:
```go
package main

func main() {}
```

Create `cmd/seed/main.go`:
```go
package main

func main() {}
```

Create `internal/db/db.go`:
```go
package db
```

Create `internal/handlers/handler.go`:
```go
package handlers
```

Create `internal/middleware/auth.go`:
```go
package middleware
```

**Step 5: Add a placeholder static file so the static/ dir is tracked**

Create `cmd/server/static/.gitkeep`:
```
```
(empty file)

**Step 6: Verify module compiles**

```bash
go build ./...
```

Expected: Exits silently (success). Any error indicates a problem — fix before continuing.

**Step 7: Commit**

```bash
git add go.mod go.sum .gitignore cmd/ internal/
git commit -m "chore: initialize Go module, project layout, and .gitignore"
```
<!-- END_TASK_1 -->

<!-- START_TASK_2 -->
### Task 2: Database Package and Schema

**Files:**
- Create: `internal/db/schema.sql`
- Modify: `internal/db/db.go`

**Step 1: Create the schema**

Create `internal/db/schema.sql`:
```sql
CREATE TABLE IF NOT EXISTS players (
    id   INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS seasons (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL UNIQUE,
    start_date DATE,
    end_date   DATE
);

-- Board game catalog (Wingspan, Catan, etc.)
CREATE TABLE IF NOT EXISTS games (
    id    INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL UNIQUE
);

-- One row per session a board game was played within a season.
CREATE TABLE IF NOT EXISTS game_results (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    season_id INTEGER NOT NULL REFERENCES seasons(id),
    game_id   INTEGER NOT NULL REFERENCES games(id),
    played_at DATE NOT NULL
);

-- Per-player scores for each game_result.
-- placement: 1 = winner, 2 = second, etc.
-- season_points: league points awarded (convention: 3/2/1/0 for placements 1/2/3/4).
CREATE TABLE IF NOT EXISTS player_scores (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    result_id     INTEGER NOT NULL REFERENCES game_results(id),
    player_id     INTEGER NOT NULL REFERENCES players(id),
    score         INTEGER NOT NULL,
    placement     INTEGER NOT NULL,
    season_points INTEGER NOT NULL,
    UNIQUE(result_id, player_id)
);

-- Admin users who can enter game data.
CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL  -- bcrypt hash
);
```

**Step 2: Implement db.Open**

Replace `internal/db/db.go`:
```go
package db

import (
	_ "embed"
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schema string

// Open opens (or creates) the SQLite database at dsn and applies the schema.
//
// Production DSN:  "file:/data/jcg.db?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000"
// Test DSN:        "file::memory:?cache=shared&_foreign_keys=on"
func Open(dsn string) (*sql.DB, error) {
	database, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	// Single writer avoids SQLITE_BUSY under concurrent requests.
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)

	if _, err := database.Exec(schema); err != nil {
		database.Close()
		return nil, err
	}

	return database, nil
}
```

**Step 3: Verify compilation**

```bash
go build ./...
```

Expected: Exits silently. If a CGO/gcc error appears, see the Windows note in Task 1.

**Step 4: Commit**

```bash
git add internal/db/
git commit -m "feat: add database package with SQLite schema"
```
<!-- END_TASK_2 -->

<!-- START_TASK_3 -->
### Task 3: HTTP Server, Templates, and Static Assets

Templates and static assets live under `cmd/server/` so Go's embed can reference them from `cmd/server/main.go` without `..` path components.

**Files:**
- Create: `cmd/server/templates/layout.html`
- Create: `cmd/server/templates/home.html`
- Create: `cmd/server/static/style.css`
- Modify: `internal/handlers/handler.go`
- Modify: `cmd/server/main.go`

**Step 1: Create layout sub-templates**

Create `cmd/server/templates/layout.html`:
```html
{{define "head"}}
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{.Title}} – Jacksonian Gaming Council</title>
<script src="https://cdn.jsdelivr.net/npm/htmx.org@2.0.8/dist/htmx.min.js"
        integrity="sha384-/TgkGk7p307TH7EXJDuUlgG3Ce1UVolAOFopFekQkkXihi5u/6OCvVKyz1W+idaz"
        crossorigin="anonymous"></script>
<link rel="stylesheet" href="/static/style.css">
{{end}}

{{define "nav"}}
<nav>
  <a href="/" class="brand">Jacksonian Gaming Council</a>
  <div class="nav-links">
    {{if .Username}}
      <span class="nav-user">{{.Username}}</span>
      <form method="POST" action="/logout" style="display:inline">
        <button type="submit" class="btn-link">Logout</button>
      </form>
    {{else}}
      <a href="/login">Login</a>
    {{end}}
  </div>
</nav>
{{end}}
```

**Step 2: Create home template (placeholder, replaced in Phase 4)**

Create `cmd/server/templates/home.html`:
```html
{{define "home"}}
<!DOCTYPE html>
<html lang="en">
<head>{{template "head" .}}</head>
<body>
  {{template "nav" .}}
  <main>
    <h1>Season Leaderboard</h1>
    <p>No seasons yet.</p>
  </main>
</body>
</html>
{{end}}
```

**Step 3: Create base stylesheet**

Create `cmd/server/static/style.css`:
```css
*, *::before, *::after { box-sizing: border-box; }

body {
  font-family: system-ui, sans-serif;
  max-width: 960px;
  margin: 0 auto;
  padding: 1rem;
  color: #1a1a1a;
}

nav {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.75rem 0;
  border-bottom: 1px solid #e0e0e0;
  margin-bottom: 2rem;
}

.brand { font-weight: bold; text-decoration: none; color: inherit; }
.nav-links { display: flex; gap: 1rem; align-items: center; }
.nav-user { color: #555; font-size: 0.9rem; }

.btn-link {
  background: none;
  border: none;
  cursor: pointer;
  color: #0066cc;
  font-size: inherit;
  padding: 0;
  text-decoration: underline;
}

table { width: 100%; border-collapse: collapse; }
th, td { padding: 0.5rem 0.75rem; text-align: left; border-bottom: 1px solid #e0e0e0; }
th { font-weight: 600; }

.form-narrow { max-width: 360px; }
.form-narrow label { display: block; margin-top: 1rem; font-weight: 600; }
.form-narrow input, .form-narrow select {
  display: block; width: 100%; padding: 0.4rem 0.6rem;
  margin-top: 0.25rem; border: 1px solid #ccc; border-radius: 4px;
}
.form-narrow button, .btn-primary {
  margin-top: 1rem; padding: 0.5rem 1.5rem;
  background: #0066cc; color: white;
  border: none; border-radius: 4px; cursor: pointer;
}
.error { color: #c00; background: #fff0f0; padding: 0.5rem; border-radius: 4px; margin-bottom: 1rem; }
```

**Step 4: Implement Handler struct**

Replace `internal/handlers/handler.go`:
```go
package handlers

import (
	"database/sql"
	"html/template"
	"log"
	"net/http"

	"jcg/internal/middleware"
)

// Handler holds shared dependencies for all HTTP handlers.
type Handler struct {
	db   *sql.DB
	tmpl *template.Template
}

func New(db *sql.DB, tmpl *template.Template) *Handler {
	return &Handler{db: db, tmpl: tmpl}
}

func (h *Handler) render(w http.ResponseWriter, name string, data any) {
	if err := h.tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template %q: %v", name, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// Home is a placeholder until the leaderboard is built in Phase 4.
func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
	h.render(w, "home", map[string]any{
		"Title":    "Home",
		"Username": middleware.UsernameFromContext(r),
	})
}
```

**Step 5: Implement main.go**

Replace `cmd/server/main.go`:
```go
package main

import (
	"embed"
	"flag"
	"html/template"
	"io/fs"
	"log"
	"net/http"

	"jcg/internal/db"
	"jcg/internal/handlers"
)

// Templates and static assets live alongside main.go under cmd/server/
// so //go:embed can reference them without forbidden ".." path components.
//go:embed templates
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

func main() {
	dsn := flag.String("db", "file:./jcg.db?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000", "SQLite DSN")
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	database, err := db.Open(*dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()

	tmpl := template.Must(template.ParseFS(templateFS, "templates/*.html"))

	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal(err)
	}

	h := handlers.New(database, tmpl)

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))
	mux.HandleFunc("GET /{$}", h.Home)

	log.Printf("listening on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}
```

**Step 6: Verify the server starts and serves HTML**

```bash
go run ./cmd/server &
sleep 1
curl -s http://localhost:8080/ | grep "Jacksonian Gaming Council"
kill %1
```

Expected: curl returns HTML containing "Jacksonian Gaming Council".

**Step 7: Commit**

```bash
git add cmd/server/main.go cmd/server/templates/ cmd/server/static/ internal/handlers/handler.go
git commit -m "feat: add HTTP server skeleton with embedded templates and static assets"
```
<!-- END_TASK_3 -->

<!-- START_TASK_4 -->
### Task 4: Docker Setup

**Files:**
- Create: `Dockerfile`
- Create: `docker-compose.yml`
- Modify: `.dockerignore`

**Step 1: Create Dockerfile**

Use dynamic linking (not static) against glibc. The builder and runtime both use Debian, so the dynamically linked binary runs without issues. This is simpler and more reliable than static linking with glibc.

Create `Dockerfile`:
```dockerfile
# Stage 1: Build
FROM golang:1.22-bookworm AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=1 required for mattn/go-sqlite3.
# Dynamic linking against glibc — runtime image must also be glibc-based (debian).
RUN CGO_ENABLED=1 GOOS=linux go build -o server ./cmd/server

# Stage 2: Run
FROM debian:bookworm-slim

WORKDIR /app
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/server .

VOLUME ["/data"]
EXPOSE 8080
CMD ["./server", \
     "-db=file:/data/jcg.db?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000", \
     "-addr=:8080"]
```

**Step 2: Create docker-compose.yml**

Create `docker-compose.yml`:
```yaml
services:
  app:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - jcg_data:/data
    restart: unless-stopped

volumes:
  jcg_data:
```

**Step 3: Update .dockerignore**

Replace `.dockerignore` with:
```
.git
*.db
*.db-wal
*.db-shm
docs/
.worktrees/
```

**Step 4: Build Docker image**

```bash
docker compose build
```

Expected: Build completes successfully. The Go binary is compiled inside the container — no local gcc needed for this step.

**Step 5: Run with Docker and verify**

```bash
docker compose up -d
sleep 2
curl -s http://localhost:8080/ | grep "Jacksonian Gaming Council"
docker compose down
```

Expected: curl returns HTML containing "Jacksonian Gaming Council".

**Step 6: Commit**

```bash
git add Dockerfile docker-compose.yml .dockerignore
git commit -m "chore: add multi-stage Docker build and compose configuration"
```
<!-- END_TASK_4 -->
