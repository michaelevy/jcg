# Handlers Package

Last verified: 2026-04-19

## Purpose
HTTP handler layer. Translates requests into DB calls and template renders. No business logic beyond input parsing.

## Contracts
- **Exposes**: `Handler` struct via `New(db, tmpl)`, methods: Leaderboard, SeasonGames, PlayerProfile, GameResultDetail, LoginPage, LoginSubmit, Logout, EntryPage, EntrySubmit, CreateSeason
- **Guarantees**: Auth handlers use constant-time comparison (dummy bcrypt hash on missing user). Leaderboard defaults to latest season if none specified and now includes CumulativePoints graph data as `template.JS`. HTMX requests (`HX-Request: true`) get partial template fragments. Read-only views (history, player, game-result) are public (no auth required).
- **Expects**: Templates pre-parsed with named blocks: `login`, `leaderboard`, `leaderboard-table`, `entry`, `season-options`, `history`, `history-table`, `player`, `game_result`. Template FuncMap must include `add`.

## Dependencies
- **Uses**: `internal/db` (all data access), `internal/middleware` (session create/delete/context)
- **Used by**: `cmd/server/main.go` (route registration)

## Key Decisions
- Handler is a thin struct (db + tmpl only), no interface: simple enough not to need one
- Entry form scores parsed from `score_<playerID>` form fields; empty fields skipped, minimum 2 players required
- CreateSeason returns an HTMX fragment (season-options) for inline dropdown update
- SeasonGames supports HTMX partial swap (history-table block) for season selector
- PlayerProfile uses path param `{id}`; GameResultDetail uses path param `{id}`
- Leaderboard passes GraphJSON (CumulativePoints as JSON) to template for Chart.js rendering

## Gotchas
- render() logs template errors and returns 500; does not panic
- LoginSubmit always runs bcrypt.CompareHashAndPassword even on missing user (timing attack mitigation)
