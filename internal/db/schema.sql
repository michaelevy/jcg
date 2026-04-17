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
    season_id   INTEGER NOT NULL REFERENCES seasons(id),
    game_id     INTEGER NOT NULL REFERENCES games(id),
    game_number INTEGER NOT NULL,
    UNIQUE(season_id, game_number)
);

-- Per-player scores for each game_result.
-- placement: 1 = winner, 2 = second, etc.
-- season_points: league points awarded (convention: 3/2/1/0 for placements 1/2/3/4).
CREATE TABLE IF NOT EXISTS player_scores (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    result_id     INTEGER NOT NULL REFERENCES game_results(id),
    player_id     INTEGER NOT NULL REFERENCES players(id),
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
