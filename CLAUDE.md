# JCG - Board Game Score Tracker

Last verified: 2026-04-27 (Phase 5 complete: CSRF/secure-cookie/session-sweep security hardening)

## Tech Stack
- Language: Go 1.25.5
- Database: SQLite (go-sqlite3, WAL mode, foreign keys on)
- Frontend: Server-rendered HTML templates (html/template) + HTMX + Chart.js (CDN, leaderboard graph)
- Auth: bcrypt (golang.org/x/crypto)
- Deployment: Multi-stage Docker build + docker-compose

## Commands
- `go run ./cmd/server` - Start web server (default :8080, flag -db for DSN, -addr for listen)
- `go run ./cmd/seed` - Seed users/players into DB
- `go test ./...` - Run all tests
- `docker compose up --build` - Full containerized run

## Environment
- `JCG_SECURE_COOKIE=true` - Set on session cookie in production (HTTPS); defaults to off so local HTTP works
- `JCG_SEED_PLAYERS`, `JCG_SEED_USERS` - Read by `applySeed` on startup (comma-separated; users are `name:password`)

## Project Structure
- `cmd/server/` - HTTP entrypoint, embedded templates + static assets
- `cmd/seed/` - CLI to seed users and players
- `internal/db/` - SQLite schema, Open(), all query/write helpers
- `internal/handlers/` - HTTP handlers (Handler struct holds *sql.DB + *template.Template)
- `internal/middleware/` - In-memory session store, RequireAuth middleware

## Routes
- `GET /` - Leaderboard with cumulative points graph (public, default season = latest)
- `GET /history` - Season game history (public, season param, HTMX partial support)
- `GET /players/{id}` - Player profile with per-season stats and game history (public)
- `GET /game-results/{id}` - Game result detail with play history for that game (public)
- `GET /login`, `POST /login`, `POST /logout` - Auth
- `GET /enter`, `POST /enter` - Game result entry (auth required)
- `POST /enter/season` - HTMX inline season creation (auth required)

## Conventions
- All DB functions are package-level (`db.Leaderboard(db, id)`) not methods on a struct
- Templates use `embed.FS` from cmd/server; named blocks for HTMX partial swaps
- Template FuncMap includes `add` (for 1-indexed display from 0-indexed loops)
- Tests use in-memory SQLite (`file::memory:?cache=shared&_foreign_keys=on`)
- Season points: 4/2/1/0 for placements 1st/2nd/3rd/4th+; ties share placement

## Boundaries
- Safe to edit: `internal/`, `cmd/`
- Schema changes: `internal/db/schema.sql` (embedded, applied on Open())
- Templates: `cmd/server/templates/` (embedded at compile time)

## Security
- CSRF: per-session token issued on login, validated on POST /enter, POST /enter/season, POST /logout via `middleware.RequireCSRF`. Pre-session token (cookie + form) protects POST /login. Tokens are auto-injected into templates by `Handler.render` from request context; HTMX requests pick up the token via the `htmx:configRequest` listener reading the `csrf-token` meta tag.
- Sessions: 24h TTL, deleted on access if expired, swept every hour by `middleware.StartSessionSweep`. Cookie is HttpOnly + SameSite=Lax; `Secure` flag toggled by `JCG_SECURE_COOKIE`.

## Known TODOs
- (none open from initial security hardening; revisit if multi-instance deployment is needed - sync.Map session store is single-process only)
