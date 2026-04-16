package db

import (
	"testing"
)

func TestComputePlacements_BasicRanking(t *testing.T) {
	scores := map[int64]int{
		1: 100,
		2: 80,
		3: 60,
		4: 40,
	}
	results := ComputePlacements(scores)

	byPlayer := map[int64]PlayerScore{}
	for _, r := range results {
		byPlayer[r.PlayerID] = r
	}

	cases := []struct {
		playerID  int64
		wantPlace int
		wantPts   int
	}{
		{1, 1, 3},
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

func TestComputePlacements_TiedScores_SharePlacementAndPoints(t *testing.T) {
	scores := map[int64]int{
		1: 100,
		2: 100, // tied for 1st with player 1
		3: 60,
	}
	results := ComputePlacements(scores)

	byPlayer := map[int64]PlayerScore{}
	for _, r := range results {
		byPlayer[r.PlayerID] = r
	}

	// Both tied players share placement 1 and receive 3 season points.
	if byPlayer[1].Placement != 1 || byPlayer[1].SeasonPoints != 3 {
		t.Errorf("player 1 (tied 1st): want placement=1 pts=3, got placement=%d pts=%d",
			byPlayer[1].Placement, byPlayer[1].SeasonPoints)
	}
	if byPlayer[2].Placement != 1 || byPlayer[2].SeasonPoints != 3 {
		t.Errorf("player 2 (tied 1st): want placement=1 pts=3, got placement=%d pts=%d",
			byPlayer[2].Placement, byPlayer[2].SeasonPoints)
	}
	// Player 3 is 3rd place (positions 1 and 2 are both occupied by the tie).
	if byPlayer[3].Placement != 3 || byPlayer[3].SeasonPoints != 1 {
		t.Errorf("player 3 (3rd after tie): want placement=3 pts=1, got placement=%d pts=%d",
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
		{PlayerID: 1, Score: 90, Placement: 1, SeasonPoints: 3},
		{PlayerID: 2, Score: 70, Placement: 2, SeasonPoints: 2},
	}
	if err := InsertGameResult(database, 1, 1, "2026-04-12", scores); err != nil {
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
	// Alice wins (3pts), Bob 2nd (2pts), Carol 3rd (1pt).
	database.Exec(`INSERT INTO game_results (id, season_id, game_id, played_at) VALUES (1, 1, 1, '2026-04-12')`)
	database.Exec(`INSERT INTO player_scores (result_id, player_id, score, placement, season_points)
		VALUES (1, 1, 100, 1, 3), (1, 2, 80, 2, 2), (1, 3, 60, 3, 1)`)

	rows, err := Leaderboard(database, 1)
	if err != nil {
		t.Fatalf("Leaderboard: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(rows))
	}
	if rows[0].PlayerName != "Alice" || rows[0].TotalPoints != 3 || rows[0].Wins != 1 {
		t.Errorf("1st place: want Alice 3pts 1win, got %+v", rows[0])
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
