package db

import (
	"database/sql"
	"fmt"
	"sort"
	"time"
)

// --- Data types ---

type Player struct {
	ID   int64
	Name string
}

type Season struct {
	ID        int64
	Name      string
	StartDate *time.Time
	EndDate   *time.Time
}

type Game struct {
	ID    int64
	Title string
}

// PlayerScore is one player's result within a game result entry.
type PlayerScore struct {
	PlayerID     int64
	Score        int
	Placement    int // 1 = winner
	SeasonPoints int // 3/2/1/0 for placements 1/2/3/4+
}

// --- List helpers ---

func ListPlayers(db *sql.DB) ([]Player, error) {
	rows, err := db.Query(`SELECT id, name FROM players ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Player
	for rows.Next() {
		var p Player
		if err := rows.Scan(&p.ID, &p.Name); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func ListSeasons(db *sql.DB) ([]Season, error) {
	rows, err := db.Query(`SELECT id, name, start_date, end_date FROM seasons ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Season
	for rows.Next() {
		var s Season
		if err := rows.Scan(&s.ID, &s.Name, &s.StartDate, &s.EndDate); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func ListGames(db *sql.DB) ([]Game, error) {
	rows, err := db.Query(`SELECT id, title FROM games ORDER BY title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Game
	for rows.Next() {
		var g Game
		if err := rows.Scan(&g.ID, &g.Title); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// --- Write helpers ---

// CreateSeason inserts a new season and returns its ID.
func CreateSeason(db *sql.DB, name string) (int64, error) {
	res, err := db.Exec(`INSERT INTO seasons (name) VALUES (?)`, name)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CreateGame inserts a new game title if it doesn't exist, returning its ID either way.
func CreateGame(db *sql.DB, title string) (int64, error) {
	_, err := db.Exec(`INSERT OR IGNORE INTO games (title) VALUES (?)`, title)
	if err != nil {
		return 0, err
	}
	var id int64
	if err := db.QueryRow(`SELECT id FROM games WHERE title = ?`, title).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

// ComputePlacements ranks scores highest-first and assigns placements (1-indexed).
// Ties share the same placement (e.g. two players tied for 1st both get placement 1
// and both receive 3 season points — the position below them is skipped accordingly).
// SeasonPoints: 3/2/1/0 for placements 1/2/3/4+.
func ComputePlacements(scores map[int64]int) []PlayerScore {
	type pair struct {
		playerID int64
		score    int
	}
	pairs := make([]pair, 0, len(scores))
	for pid, s := range scores {
		pairs = append(pairs, pair{pid, s})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].score > pairs[j].score // descending
	})

	results := make([]PlayerScore, len(pairs))
	for i, p := range pairs {
		placement := i + 1
		// Ties share the same placement as the previous player.
		if i > 0 && p.score == pairs[i-1].score {
			placement = results[i-1].Placement
		}
		results[i] = PlayerScore{
			PlayerID:     p.playerID,
			Score:        p.score,
			Placement:    placement,
			SeasonPoints: seasonPoints(placement),
		}
	}
	return results
}

func seasonPoints(placement int) int {
	switch placement {
	case 1:
		return 3
	case 2:
		return 2
	case 3:
		return 1
	default:
		return 0
	}
}

// InsertGameResult writes a game_result row and its player_scores in a transaction.
func InsertGameResult(db *sql.DB, seasonID, gameID int64, playedAt string, scores []PlayerScore) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`INSERT INTO game_results (season_id, game_id, played_at) VALUES (?, ?, ?)`,
		seasonID, gameID, playedAt,
	)
	if err != nil {
		return fmt.Errorf("insert game_result: %w", err)
	}
	resultID, _ := res.LastInsertId()

	for _, s := range scores {
		_, err = tx.Exec(
			`INSERT INTO player_scores (result_id, player_id, score, placement, season_points) VALUES (?, ?, ?, ?, ?)`,
			resultID, s.PlayerID, s.Score, s.Placement, s.SeasonPoints,
		)
		if err != nil {
			return fmt.Errorf("insert player_score: %w", err)
		}
	}

	return tx.Commit()
}
