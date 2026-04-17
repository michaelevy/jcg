package db

import (
	"testing"
)

func TestPlacementsToScores_AssignsSeasonPoints(t *testing.T) {
	placements := map[int64]int{
		1: 1,
		2: 2,
		3: 3,
		4: 4,
	}
	results := PlacementsToScores(placements)

	byPlayer := map[int64]PlayerScore{}
	for _, r := range results {
		byPlayer[r.PlayerID] = r
	}

	cases := []struct {
		playerID  int64
		wantPlace int
		wantPts   int
	}{
		{1, 1, 4},
		{2, 2, 2},
		{3, 3, 1},
		{4, 4, 0},
	}
	for _, c := range cases {
		got := byPlayer[c.playerID]
		if got.Placement != c.wantPlace || got.SeasonPoints != c.wantPts {
			t.Errorf("player %d: want placement=%d pts=%d, got placement=%d pts=%d",
				c.playerID, c.wantPlace, c.wantPts, got.Placement, got.SeasonPoints)
		}
	}
}

func TestPlacementsToScores_TiedPlacements_BothGetSamePoints(t *testing.T) {
	placements := map[int64]int{
		1: 1,
		2: 1, // tied for 1st
		3: 3,
	}
	results := PlacementsToScores(placements)

	byPlayer := map[int64]PlayerScore{}
	for _, r := range results {
		byPlayer[r.PlayerID] = r
	}

	if byPlayer[1].Placement != 1 || byPlayer[1].SeasonPoints != 4 {
		t.Errorf("player 1 (tied 1st): want placement=1 pts=4, got placement=%d pts=%d",
			byPlayer[1].Placement, byPlayer[1].SeasonPoints)
	}
	if byPlayer[2].Placement != 1 || byPlayer[2].SeasonPoints != 4 {
		t.Errorf("player 2 (tied 1st): want placement=1 pts=4, got placement=%d pts=%d",
			byPlayer[2].Placement, byPlayer[2].SeasonPoints)
	}
	if byPlayer[3].Placement != 3 || byPlayer[3].SeasonPoints != 1 {
		t.Errorf("player 3 (3rd): want placement=3 pts=1, got placement=%d pts=%d",
			byPlayer[3].Placement, byPlayer[3].SeasonPoints)
	}
}

func TestInsertGameResult_PersistsData(t *testing.T) {
	database, err := Open("file::memory:?cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	if _, err := database.Exec(`INSERT INTO players (id, name) VALUES (1, 'Alice'), (2, 'Bob')`); err != nil {
		t.Fatalf("seed players: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO seasons (id, name) VALUES (1, 'Season 1')`); err != nil {
		t.Fatalf("seed seasons: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO games (id, title) VALUES (1, 'Wingspan')`); err != nil {
		t.Fatalf("seed games: %v", err)
	}

	scores := []PlayerScore{
		{PlayerID: 1, Placement: 1, SeasonPoints: 4},
		{PlayerID: 2, Placement: 2, SeasonPoints: 2},
	}
	if err := InsertGameResult(database, 1, 1, 1, scores); err != nil {
		t.Fatalf("InsertGameResult: %v", err)
	}

	var resultCount, scoreCount int
	database.QueryRow(`SELECT COUNT(*) FROM game_results`).Scan(&resultCount)
	database.QueryRow(`SELECT COUNT(*) FROM player_scores`).Scan(&scoreCount)

	if resultCount != 1 {
		t.Errorf("want 1 game_results row, got %d", resultCount)
	}
	if scoreCount != 2 {
		t.Errorf("want 2 player_scores rows, got %d", scoreCount)
	}
}

func TestLeaderboard_RanksPlayers(t *testing.T) {
	database, err := Open("file::memory:?cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	database.Exec(`INSERT INTO players (id, name) VALUES (1, 'Alice'), (2, 'Bob'), (3, 'Carol')`)
	database.Exec(`INSERT INTO seasons (id, name) VALUES (1, 'Season 1')`)
	database.Exec(`INSERT INTO games (id, title) VALUES (1, 'Wingspan')`)
	// Alice wins (4pts), Bob 2nd (2pts), Carol 3rd (1pt).
	database.Exec(`INSERT INTO game_results (id, season_id, game_id, game_number) VALUES (1, 1, 1, 1)`)
	database.Exec(`INSERT INTO player_scores (result_id, player_id, placement, season_points)
		VALUES (1, 1, 1, 4), (1, 2, 2, 2), (1, 3, 3, 1)`)

	rows, err := Leaderboard(database, 1)
	if err != nil {
		t.Fatalf("Leaderboard: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(rows))
	}
	if rows[0].PlayerName != "Alice" || rows[0].TotalPoints != 4 || rows[0].Wins != 1 {
		t.Errorf("1st place: want Alice 4pts 1win, got %+v", rows[0])
	}
	if rows[1].PlayerName != "Bob" || rows[1].TotalPoints != 2 || rows[1].Wins != 0 {
		t.Errorf("2nd place: want Bob 2pts 0wins, got %+v", rows[1])
	}
}

func TestLeaderboard_PlayersWithNoResults_AppearWithZeros(t *testing.T) {
	database, err := Open("file::memory:?cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	database.Exec(`INSERT INTO players (id, name) VALUES (1, 'Alice'), (2, 'Bob')`)
	database.Exec(`INSERT INTO seasons (id, name) VALUES (1, 'Season 1')`)
	// No game results.

	rows, err := Leaderboard(database, 1)
	if err != nil {
		t.Fatalf("Leaderboard: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("want 2 rows (all players appear), got %d", len(rows))
	}
	for _, r := range rows {
		if r.TotalPoints != 0 || r.GamesPlayed != 0 {
			t.Errorf("player with no results should have zero stats, got %+v", r)
		}
	}
}

func TestCurrentSeasonID_ReturnsLatestOrZero(t *testing.T) {
	database, err := Open("file::memory:?cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	id, err := CurrentSeasonID(database)
	if err != nil {
		t.Fatalf("CurrentSeasonID: %v", err)
	}
	if id != 0 {
		t.Errorf("want 0 when no seasons, got %d", id)
	}

	database.Exec(`INSERT INTO seasons (id, name) VALUES (1, 'S1'), (2, 'S2')`)
	id, _ = CurrentSeasonID(database)
	if id != 2 {
		t.Errorf("want 2 (latest), got %d", id)
	}
}

func TestLeaderboard_MultiSeasonIsolation_RegressionTest(t *testing.T) {
	database, err := Open("file::memory:?cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	// Setup: 2 seasons, 2 players, 2 games (one per season)
	database.Exec(`INSERT INTO players (id, name) VALUES (1, 'Player1'), (2, 'Player2')`)
	database.Exec(`INSERT INTO seasons (id, name) VALUES (1, 'Season 1'), (2, 'Season 2')`)
	database.Exec(`INSERT INTO games (id, title) VALUES (1, 'Game1'), (2, 'Game2')`)

	// Season 1: Player 1 wins with 4 points
	database.Exec(`INSERT INTO game_results (id, season_id, game_id, game_number) VALUES (1, 1, 1, 1)`)
	database.Exec(`INSERT INTO player_scores (result_id, player_id, placement, season_points)
		VALUES (1, 1, 1, 4), (1, 2, 2, 2)`)

	// Season 2: Player 2 wins with 4 points
	database.Exec(`INSERT INTO game_results (id, season_id, game_id, game_number) VALUES (2, 2, 2, 1)`)
	database.Exec(`INSERT INTO player_scores (result_id, player_id, placement, season_points)
		VALUES (2, 2, 1, 4), (2, 1, 2, 2)`)

	// Test Season 1 leaderboard: Player 1 has 4 points, Player 2 has 2 points
	rows1, err := Leaderboard(database, 1)
	if err != nil {
		t.Fatalf("Leaderboard(season 1): %v", err)
	}

	if len(rows1) != 2 {
		t.Fatalf("Season 1: want 2 rows, got %d", len(rows1))
	}

	// Season 1 leaderboard should be ranked: Player 1 (4pts), Player 2 (2pts)
	if rows1[0].PlayerName != "Player1" || rows1[0].TotalPoints != 4 {
		t.Errorf("Season 1, 1st place: want Player1 with 4 points, got %s with %d points",
			rows1[0].PlayerName, rows1[0].TotalPoints)
	}
	if rows1[1].PlayerName != "Player2" || rows1[1].TotalPoints != 2 {
		t.Errorf("Season 1, 2nd place: want Player2 with 2 points, got %s with %d points",
			rows1[1].PlayerName, rows1[1].TotalPoints)
	}

	// Test Season 2 leaderboard: Player 2 has 3 points, Player 1 has 2 points
	rows2, err := Leaderboard(database, 2)
	if err != nil {
		t.Fatalf("Leaderboard(season 2): %v", err)
	}

	if len(rows2) != 2 {
		t.Fatalf("Season 2: want 2 rows, got %d", len(rows2))
	}

	// Season 2 leaderboard should be ranked: Player 2 (4pts), Player 1 (2pts)
	if rows2[0].PlayerName != "Player2" || rows2[0].TotalPoints != 4 {
		t.Errorf("Season 2, 1st place: want Player2 with 4 points, got %s with %d points",
			rows2[0].PlayerName, rows2[0].TotalPoints)
	}
	if rows2[1].PlayerName != "Player1" || rows2[1].TotalPoints != 2 {
		t.Errorf("Season 2, 2nd place: want Player1 with 2 points, got %s with %d points",
			rows2[1].PlayerName, rows2[1].TotalPoints)
	}
}

func TestSeasonHistory_ReturnsSortedByGameNumber(t *testing.T) {
	database, err := Open("file::memory:?cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	database.Exec(`INSERT INTO players (id, name) VALUES (1, 'Alice'), (2, 'Bob')`)
	database.Exec(`INSERT INTO seasons (id, name) VALUES (1, 'S1')`)
	database.Exec(`INSERT INTO games (id, title) VALUES (1, 'Wingspan'), (2, 'Catan')`)
	database.Exec(`INSERT INTO game_results (id, season_id, game_id, game_number) VALUES (1, 1, 1, 1), (2, 1, 2, 2)`)
	database.Exec(`INSERT INTO player_scores (result_id, player_id, placement, season_points)
		VALUES (1, 1, 1, 4), (1, 2, 2, 2), (2, 2, 1, 4), (2, 1, 2, 2)`)

	summaries, err := SeasonHistory(database, 1)
	if err != nil {
		t.Fatalf("SeasonHistory: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("want 2 summaries, got %d", len(summaries))
	}
	if summaries[0].GameNumber != 1 || summaries[0].Title != "Wingspan" {
		t.Errorf("first: want game 1 Wingspan, got game %d %q", summaries[0].GameNumber, summaries[0].Title)
	}
	if len(summaries[0].Placements) != 2 {
		t.Errorf("want 2 placements for game 1, got %d", len(summaries[0].Placements))
	}
	if summaries[0].Placements[0].PlayerName != "Alice" || summaries[0].Placements[0].Placement != 1 {
		t.Errorf("first placement: want Alice 1st, got %s %d",
			summaries[0].Placements[0].PlayerName, summaries[0].Placements[0].Placement)
	}
	if summaries[1].Title != "Catan" {
		t.Errorf("second: want Catan, got %q", summaries[1].Title)
	}
}

func TestSeasonHistory_EmptySeason_ReturnsEmpty(t *testing.T) {
	database, err := Open("file::memory:?cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	database.Exec(`INSERT INTO seasons (id, name) VALUES (1, 'S1')`)

	summaries, err := SeasonHistory(database, 1)
	if err != nil {
		t.Fatalf("SeasonHistory: %v", err)
	}
	if len(summaries) != 0 {
		t.Errorf("want 0 summaries for empty season, got %d", len(summaries))
	}
}
