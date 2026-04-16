package handlers

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jcg/internal/db"
)

func leaderboardTestHandler(t *testing.T) *Handler {
	t.Helper()
	database, err := db.Open("file::memory:?cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	database.Exec(`INSERT INTO players (id, name) VALUES (1, 'Alice'), (2, 'Bob')`)
	database.Exec(`INSERT INTO seasons (id, name) VALUES (1, 'Season 1')`)
	database.Exec(`INSERT INTO games (id, title) VALUES (1, 'Wingspan')`)
	database.Exec(`INSERT INTO game_results (id, season_id, game_id, played_at) VALUES (1, 1, 1, '2026-04-12')`)
	database.Exec(`INSERT INTO player_scores (result_id, player_id, score, placement, season_points)
		VALUES (1, 1, 100, 1, 4), (1, 2, 80, 2, 2)`)

	tmpl := template.Must(
		template.New("").Funcs(template.FuncMap{
			"add": func(a, b int) int { return a + b },
		}).Parse(`
			{{define "leaderboard"}}FULL:{{range .Rows}}{{.PlayerName}}={{.TotalPoints}};{{end}}{{end}}
			{{define "leaderboard-table"}}TABLE:{{range .Rows}}{{.PlayerName}}={{.TotalPoints}};{{end}}{{end}}
		`),
	)
	return New(database, tmpl)
}

func TestLeaderboard_FullPageRender(t *testing.T) {
	h := leaderboardTestHandler(t)

	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	h.Leaderboard(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.HasPrefix(body, "FULL:") {
		t.Errorf("want full-page template, got: %s", body)
	}
	if !strings.Contains(body, "Alice=4") {
		t.Errorf("want Alice with 4 points, got: %s", body)
	}
}

func TestLeaderboard_HTMXRequest_ReturnsTableFragment(t *testing.T) {
	h := leaderboardTestHandler(t)

	r := httptest.NewRequest("GET", "/?season=1", nil)
	r.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()

	h.Leaderboard(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.HasPrefix(body, "TABLE:") {
		t.Errorf("want table fragment for HTMX request, got: %s", body)
	}
}

func TestLeaderboard_DefaultsToCurrentSeason(t *testing.T) {
	h := leaderboardTestHandler(t)

	r := httptest.NewRequest("GET", "/", nil) // no ?season param
	w := httptest.NewRecorder()

	h.Leaderboard(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Alice") {
		t.Errorf("want Alice in default season leaderboard, got: %s", w.Body.String())
	}
}

func TestLeaderboard_NoSeasons_ReturnsOK(t *testing.T) {
	database, err := db.Open("file::memory:?cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	database.Exec(`INSERT INTO players (id, name) VALUES (1, 'Alice')`)
	// No seasons.

	tmpl := template.Must(
		template.New("").Funcs(template.FuncMap{
			"add": func(a, b int) int { return a + b },
		}).Parse(`
			{{define "leaderboard"}}EMPTY{{end}}
			{{define "leaderboard-table"}}EMPTY-TABLE{{end}}
		`),
	)

	h := New(database, tmpl)
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	h.Leaderboard(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 even with no seasons, got %d", w.Code)
	}
}
