package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrDuplicateGameNumber is returned by InsertGameResult when the game_number
// is already taken for that season.
var ErrDuplicateGameNumber = errors.New("game number already used in this season")

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
	Placement    int // 1 = winner
	SeasonPoints int // 4/2/1/0 for placements 1/2/3/4+
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

// NextGameNumber returns the next available game_number for a season (max + 1, or 1 if none yet).
func NextGameNumber(db *sql.DB, seasonID int64) (int, error) {
	var next int
	err := db.QueryRow(
		`SELECT COALESCE(MAX(game_number), 0) + 1 FROM game_results WHERE season_id = ?`,
		seasonID,
	).Scan(&next)
	return next, err
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

// PlacementsToScores converts a map of playerID->placement into PlayerScore entries
// with season_points computed. Ties are supported: two players can share a placement.
func PlacementsToScores(placements map[int64]int) []PlayerScore {
	results := make([]PlayerScore, 0, len(placements))
	for pid, placement := range placements {
		results = append(results, PlayerScore{
			PlayerID:     pid,
			Placement:    placement,
			SeasonPoints: seasonPoints(placement),
		})
	}
	return results
}

func seasonPoints(placement int) int {
	switch placement {
	case 1:
		return 4
	case 2:
		return 2
	case 3:
		return 1
	default:
		return 0
	}
}

// LeaderboardRow is one player's aggregated stats for a season.
type LeaderboardRow struct {
	PlayerID    int64
	PlayerName  string
	GamesPlayed int
	Wins        int
	TotalPoints int
}

// CurrentSeasonID returns the ID of the most recently created season,
// or 0 if no seasons exist.
func CurrentSeasonID(db *sql.DB) (int64, error) {
	var id int64
	err := db.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM seasons`).Scan(&id)
	return id, err
}

// GetSeason returns a single season by ID.
func GetSeason(db *sql.DB, id int64) (Season, error) {
	var s Season
	err := db.QueryRow(`SELECT id, name, start_date, end_date FROM seasons WHERE id = ?`, id).
		Scan(&s.ID, &s.Name, &s.StartDate, &s.EndDate)
	return s, err
}

// Leaderboard returns all players ranked by season points for the given season.
// Players with no results in the season appear with zero stats.
func Leaderboard(db *sql.DB, seasonID int64) ([]LeaderboardRow, error) {
	const q = `
		SELECT
			p.id,
			p.name,
			COUNT(ps.id)                                             AS games_played,
			COALESCE(SUM(CASE WHEN ps.placement = 1 THEN 1 END), 0) AS wins,
			COALESCE(SUM(ps.season_points), 0)                       AS total_points
		FROM players p
		LEFT JOIN (
			SELECT ps.*
			FROM player_scores ps
			JOIN game_results gr ON gr.id = ps.result_id
			WHERE gr.season_id = ?
		) ps ON ps.player_id = p.id
		GROUP BY p.id, p.name
		ORDER BY total_points DESC, wins DESC, p.name ASC
	`
	rows, err := db.Query(q, seasonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LeaderboardRow
	for rows.Next() {
		var r LeaderboardRow
		if err := rows.Scan(&r.PlayerID, &r.PlayerName, &r.GamesPlayed, &r.Wins, &r.TotalPoints); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// InsertGameResult writes a game_result row and its player_scores in a transaction.
func InsertGameResult(db *sql.DB, seasonID, gameID int64, gameNumber int, scores []PlayerScore) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`INSERT INTO game_results (season_id, game_id, game_number) VALUES (?, ?, ?)`,
		seasonID, gameID, gameNumber,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrDuplicateGameNumber
		}
		return fmt.Errorf("insert game_result: %w", err)
	}
	resultID, _ := res.LastInsertId()

	for _, s := range scores {
		_, err = tx.Exec(
			`INSERT INTO player_scores (result_id, player_id, placement, season_points) VALUES (?, ?, ?, ?)`,
			resultID, s.PlayerID, s.Placement, s.SeasonPoints,
		)
		if err != nil {
			return fmt.Errorf("insert player_score: %w", err)
		}
	}

	return tx.Commit()
}

// UpdateGameResult updates an existing game_result row and its player_scores in a transaction.
// Scores must be pre-computed via PlacementsToScores. Returns ErrDuplicateGameNumber if the
// (season_id, game_number) pair conflicts with another result in the same season.
// Scores must reference players already attached to this result; mismatched player_ids are silently ignored.
func UpdateGameResult(db *sql.DB, resultID, seasonID, gameID int64, gameNumber int, scores []PlayerScore) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`UPDATE game_results SET season_id=?, game_id=?, game_number=? WHERE id=?`,
		seasonID, gameID, gameNumber, resultID,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrDuplicateGameNumber
		}
		return fmt.Errorf("update game_result: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("update game_result: %w", sql.ErrNoRows)
	}

	for _, s := range scores {
		_, err = tx.Exec(
			`UPDATE player_scores SET placement=?, season_points=? WHERE result_id=? AND player_id=?`,
			s.Placement, s.SeasonPoints, resultID, s.PlayerID,
		)
		if err != nil {
			return fmt.Errorf("update player_score: %w", err)
		}
	}

	return tx.Commit()
}

// PlacementRow is one player's result within a game session, used in history and detail views.
type PlacementRow struct {
	PlayerID   int64
	PlayerName string
	Placement  int
	Points     int
}

// GameSummary is one game session for the season history view.
type GameSummary struct {
	ResultID   int64
	GameNumber int
	GameID     int64 // Used by game detail view (Phase 3)
	Title      string
	Placements []PlacementRow
}

// SeasonHistory returns all game sessions in a season ordered by game_number,
// each with placements sorted by rank.
func SeasonHistory(db *sql.DB, seasonID int64) ([]GameSummary, error) {
	const q = `
		SELECT
			gr.id, gr.game_number, g.id, g.title,
			p.id, p.name, ps.placement, ps.season_points
		FROM game_results gr
		JOIN games g ON g.id = gr.game_id
		JOIN player_scores ps ON ps.result_id = gr.id
		JOIN players p ON p.id = ps.player_id
		WHERE gr.season_id = ?
		ORDER BY gr.game_number, ps.placement
	`
	rows, err := db.Query(q, seasonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []GameSummary
	index := map[int64]int{} // resultID -> slice index
	for rows.Next() {
		var (
			resultID, gameID, playerID    int64
			gameNumber, placement, points int
			title, playerName             string
		)
		if err := rows.Scan(&resultID, &gameNumber, &gameID, &title,
			&playerID, &playerName, &placement, &points); err != nil {
			return nil, err
		}
		if _, seen := index[resultID]; !seen {
			index[resultID] = len(summaries)
			summaries = append(summaries, GameSummary{
				ResultID:   resultID,
				GameNumber: gameNumber,
				GameID:     gameID,
				Title:      title,
			})
		}
		i := index[resultID]
		summaries[i].Placements = append(summaries[i].Placements, PlacementRow{
			PlayerID:   playerID,
			PlayerName: playerName,
			Placement:  placement,
			Points:     points,
		})
	}
	return summaries, rows.Err()
}

// GetPlayer returns a single player by ID.
func GetPlayer(db *sql.DB, id int64) (Player, error) {
	var p Player
	err := db.QueryRow(`SELECT id, name FROM players WHERE id = ?`, id).Scan(&p.ID, &p.Name)
	return p, err
}

// PlayerSeasonStat summarises a player's performance in one season.
type PlayerSeasonStat struct {
	SeasonID    int64
	SeasonName  string
	Position    int
	TotalPoints int
	Wins        int
}

// PlayerSeasonStats returns per-season aggregates for a player, newest season first.
// Position is the player's leaderboard rank within each season (ties share a rank).
func PlayerSeasonStats(db *sql.DB, playerID int64) ([]PlayerSeasonStat, error) {
	const q = `
		WITH player_season AS (
			SELECT
				gr.season_id,
				COALESCE(SUM(ps.season_points), 0) AS total_points,
				COALESCE(SUM(CASE WHEN ps.placement = 1 THEN 1 ELSE 0 END), 0) AS wins
			FROM player_scores ps
			JOIN game_results gr ON gr.id = ps.result_id
			WHERE ps.player_id = ?
			GROUP BY gr.season_id
		),
		all_season_totals AS (
			SELECT
				gr.season_id,
				ps.player_id,
				p.name,
				COALESCE(SUM(ps.season_points), 0) AS total_points,
				COALESCE(SUM(CASE WHEN ps.placement = 1 THEN 1 ELSE 0 END), 0) AS wins
			FROM player_scores ps
			JOIN game_results gr ON gr.id = ps.result_id
			JOIN players p ON p.id = ps.player_id
			GROUP BY gr.season_id, ps.player_id
		),
		ranked AS (
			SELECT
				season_id, player_id,
				RANK() OVER (
					PARTITION BY season_id
					ORDER BY total_points DESC, wins DESC, name ASC
				) AS position
			FROM all_season_totals
		)
		SELECT
			s.id, s.name,
			ps.total_points,
			ps.wins,
			r.position
		FROM player_season ps
		JOIN seasons s ON s.id = ps.season_id
		JOIN ranked r ON r.season_id = ps.season_id AND r.player_id = ?
		ORDER BY s.id DESC
	`
	rows, err := db.Query(q, playerID, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PlayerSeasonStat
	for rows.Next() {
		var r PlayerSeasonStat
		if err := rows.Scan(&r.SeasonID, &r.SeasonName, &r.TotalPoints, &r.Wins, &r.Position); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PlayerGameRow is one game entry in a player's cross-season history.
type PlayerGameRow struct {
	ResultID   int64
	GameNumber int
	GameID     int64
	GameTitle  string
	SeasonID   int64
	SeasonName string
	Placement  int
	Points     int
}

// PlayerGameHistory returns all games a player has participated in, newest first.
func PlayerGameHistory(db *sql.DB, playerID int64) ([]PlayerGameRow, error) {
	const q = `
		SELECT
			gr.id, gr.game_number, g.id, g.title,
			s.id, s.name, ps.placement, ps.season_points
		FROM player_scores ps
		JOIN game_results gr ON gr.id = ps.result_id
		JOIN games g ON g.id = gr.game_id
		JOIN seasons s ON s.id = gr.season_id
		WHERE ps.player_id = ?
		ORDER BY s.id DESC, gr.game_number DESC
	`
	rows, err := db.Query(q, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PlayerGameRow
	for rows.Next() {
		var r PlayerGameRow
		if err := rows.Scan(&r.ResultID, &r.GameNumber, &r.GameID, &r.GameTitle,
			&r.SeasonID, &r.SeasonName, &r.Placement, &r.Points); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GameResultDetail holds the full data for one game session (for the detail view).
type GameResultDetail struct {
	ResultID   int64
	GameNumber int
	GameID     int64
	GameTitle  string
	SeasonID   int64
	SeasonName string
	Placements []PlacementRow // PlacementRow defined in phase_01 (queries.go)
}

// GetGameResult returns the full detail for one game_result row including all placements.
// Returns sql.ErrNoRows if the result doesn't exist.
func GetGameResult(db *sql.DB, resultID int64) (GameResultDetail, error) {
	const q = `
		SELECT
			gr.id, gr.game_number, g.id, g.title,
			s.id, s.name,
			p.id, p.name, ps.placement, ps.season_points
		FROM game_results gr
		JOIN games g ON g.id = gr.game_id
		JOIN seasons s ON s.id = gr.season_id
		JOIN player_scores ps ON ps.result_id = gr.id
		JOIN players p ON p.id = ps.player_id
		WHERE gr.id = ?
		ORDER BY ps.placement
	`
	rows, err := db.Query(q, resultID)
	if err != nil {
		return GameResultDetail{}, err
	}
	defer rows.Close()

	var detail GameResultDetail
	found := false
	for rows.Next() {
		var (
			playerID, gameID, seasonID    int64
			gameNumber, placement, points int
			gameTitle, seasonName         string
			playerName                    string
		)
		if err := rows.Scan(&detail.ResultID, &gameNumber, &gameID, &gameTitle,
			&seasonID, &seasonName, &playerID, &playerName, &placement, &points); err != nil {
			return GameResultDetail{}, err
		}
		if !found {
			detail.GameNumber = gameNumber
			detail.GameID = gameID
			detail.GameTitle = gameTitle
			detail.SeasonID = seasonID
			detail.SeasonName = seasonName
			found = true
		}
		detail.Placements = append(detail.Placements, PlacementRow{
			PlayerID:   playerID,
			PlayerName: playerName,
			Placement:  placement,
			Points:     points,
		})
	}
	if err := rows.Err(); err != nil {
		return GameResultDetail{}, err
	}
	if !found {
		return GameResultDetail{}, sql.ErrNoRows
	}
	return detail, nil
}

// PlayHistoryRow is one entry in the all-time play history for a game title.
type PlayHistoryRow struct {
	ResultID   int64
	GameNumber int
	SeasonID   int64
	SeasonName string
	WinnerName string
	WinnerID   int64
}

// CumulativePointsRow is one data point in a player's running total for a season.
type CumulativePointsRow struct {
	GameNumber       int
	PlayerID         int64
	PlayerName       string
	CumulativePoints int
}

// GamePlayHistory returns all sessions a given game title has been played, newest first.
// Each row includes the session winner. Note: if two players tied for 1st place in a
// session, that session will appear once per co-winner in the results. This is acceptable
// given the 4-player format where ties are uncommon, and both winners are relevant to show.
func GamePlayHistory(db *sql.DB, gameID int64) ([]PlayHistoryRow, error) {
	const q = `
		SELECT
			gr.id, gr.game_number, s.id, s.name,
			p.id, p.name
		FROM game_results gr
		JOIN seasons s ON s.id = gr.season_id
		JOIN player_scores ps ON ps.result_id = gr.id AND ps.placement = 1
		JOIN players p ON p.id = ps.player_id
		WHERE gr.game_id = ?
		ORDER BY gr.id DESC
	`
	rows, err := db.Query(q, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PlayHistoryRow
	for rows.Next() {
		var r PlayHistoryRow
		if err := rows.Scan(&r.ResultID, &r.GameNumber,
			&r.SeasonID, &r.SeasonName, &r.WinnerID, &r.WinnerName); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CumulativePoints returns the running cumulative season points for each player
// at each game number within the season, ordered by game_number then player name.
// Uses a SQLite window function (requires SQLite ≥ 3.25).
func CumulativePoints(db *sql.DB, seasonID int64) ([]CumulativePointsRow, error) {
	const q = `
		SELECT
			gr.game_number,
			p.id,
			p.name,
			SUM(ps.season_points) OVER (
				PARTITION BY p.id
				ORDER BY gr.game_number
				ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW
			) AS cumulative_points
		FROM player_scores ps
		JOIN game_results gr ON gr.id = ps.result_id
		JOIN players p ON p.id = ps.player_id
		WHERE gr.season_id = ?
		ORDER BY gr.game_number, p.name
	`
	rows, err := db.Query(q, seasonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CumulativePointsRow
	for rows.Next() {
		var r CumulativePointsRow
		if err := rows.Scan(&r.GameNumber, &r.PlayerID, &r.PlayerName, &r.CumulativePoints); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
