# Database Package

Last verified: 2026-04-17

## Purpose
Single source of truth for SQLite schema and all data access. Keeps SQL out of handlers.

## Contracts
- **Exposes**: `Open(dsn)`, list helpers (Players/Seasons/Games), write helpers (CreateSeason/CreateGame/InsertGameResult), `Leaderboard(db, seasonID)`, `CurrentSeasonID(db)`, `GetSeason(db, id)`, `ComputePlacements(scores)`
- **Guarantees**: Schema applied on Open(). InsertGameResult is transactional (game_result + all player_scores atomically). ComputePlacements handles ties (shared placement, both get same season points). Leaderboard includes all players (LEFT JOIN), even those with no results.
- **Expects**: DSN with `_foreign_keys=on`. Single-connection pool (SetMaxOpenConns(1)) to avoid SQLITE_BUSY.

## Dependencies
- **Uses**: go-sqlite3 driver, embedded schema.sql
- **Used by**: handlers (all DB access), seed CLI

## Key Decisions
- Package-level functions (not receiver methods on a DB wrapper): keeps it simple, db passed explicitly
- `CreateGame` uses INSERT OR IGNORE + SELECT: upsert pattern for game titles
- Placement scoring is 4/2/1/0; ties share placement and both receive the higher tier's points

## Invariants
- player_scores has UNIQUE(result_id, player_id): one score per player per game result
- Seasons ordered by id DESC (latest first) in ListSeasons
- Leaderboard ORDER BY: total_points DESC, wins DESC, name ASC

## Gotchas
- schema.sql is embedded at compile time; changes require rebuild
- Open() sets MaxOpenConns(1) -- do not override or you get SQLITE_BUSY under concurrency
