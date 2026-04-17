# JCG - Board Game Score Tracker

Last verified: 2026-04-17

## Tech Stack
- Language: Go 1.25.5
- Database: SQLite (go-sqlite3, WAL mode, foreign keys on)
- Frontend: Server-rendered HTML templates (html/template) + HTMX
- Auth: bcrypt (golang.org/x/crypto)
- Deployment: Multi-stage Docker build + docker-compose

## Commands
- `go run ./cmd/server` - Start web server (default :8080, flag -db for DSN, -addr for listen)
- `go run ./cmd/seed` - Seed users/players into DB
- `go test ./...` - Run all tests
- `docker compose up --build` - Full containerized run

## Project Structure
- `cmd/server/` - HTTP entrypoint, embedded templates + static assets
- `cmd/seed/` - CLI to seed users and players
- `internal/db/` - SQLite schema, Open(), all query/write helpers
- `internal/handlers/` - HTTP handlers (Handler struct holds *sql.DB + *template.Template)
- `internal/middleware/` - In-memory session store, RequireAuth middleware

## Routes
- `GET /` - Leaderboard (public, default season = latest)
- `GET /login`, `POST /login`, `POST /logout` - Auth
- `GET /enter`, `POST /enter` - Game result entry (auth required)
- `POST /enter/season` - HTMX inline season creation (auth required)

## Conventions
- All DB functions are package-level (`db.Leaderboard(db, id)`) not methods on a struct
- Templates use `embed.FS` from cmd/server; named blocks for HTMX partial swaps
- Template FuncMap includes `add` (for 1-indexed display from 0-indexed loops)
- Tests use in-memory SQLite (`file::memory:?cache=shared&_foreign_keys=on`)
- Season points: 3/2/1/0 for placements 1st/2nd/3rd/4th+; ties share placement

## Boundaries
- Safe to edit: `internal/`, `cmd/`
- Schema changes: `internal/db/schema.sql` (embedded, applied on Open())
- Templates: `cmd/server/templates/` (embedded at compile time)

## Known TODOs
- CSRF token protection on POST /login, POST /logout
- Session store background expiry sweep (currently unbounded sync.Map)
- Secure cookie flag for production (currently HTTP-only, no Secure)
