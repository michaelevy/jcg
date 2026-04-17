package handlers

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jcg/internal/db"
)

func historyTestHandler(t *testing.T) *Handler {
	t.Helper()
	database, err := db.Open("file::memory:?cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	database.Exec(`INSERT INTO players (id, name) VALUES (1, 'Alice'), (2, 'Bob')`)
	// Only one season so CurrentSeasonID returns 1 and the default-season test works.
	database.Exec(`INSERT INTO seasons (id, name) VALUES (1, 'Season 1')`)
	database.Exec(`INSERT INTO games (id, title) VALUES (1, 'Wingspan')`)
	database.Exec(`INSERT INTO game_results (id, season_id, game_id, game_number) VALUES (1, 1, 1, 1)`)
	database.Exec(`INSERT INTO player_scores (result_id, player_id, placement, season_points)
		VALUES (1, 1, 1, 4), (1, 2, 2, 2)`)

	tmpl := template.Must(template.New("").Funcs(template.FuncMap{
		"add": func(a, b int) int { return a + b },
	}).Parse(`
		{{define "history"}}FULL:{{range .Games}}{{.Title}};{{end}}{{end}}
		{{define "history-table"}}TABLE:{{range .Games}}{{.Title}};{{end}}{{end}}
	`))
	return New(database, tmpl)
}

func TestSeasonGames_FullPage(t *testing.T) {
	h := historyTestHandler(t)
	r := httptest.NewRequest("GET", "/history?season=1", nil)
	w := httptest.NewRecorder()
	h.SeasonGames(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	if !strings.HasPrefix(w.Body.String(), "FULL:") {
		t.Errorf("want full-page template, got: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Wingspan") {
		t.Errorf("want Wingspan in response, got: %s", w.Body.String())
	}
}

func TestSeasonGames_HTMXReturnsFragment(t *testing.T) {
	h := historyTestHandler(t)
	r := httptest.NewRequest("GET", "/history?season=1", nil)
	r.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	h.SeasonGames(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	if !strings.HasPrefix(w.Body.String(), "TABLE:") {
		t.Errorf("want table fragment for HTMX, got: %s", w.Body.String())
	}
}

func TestSeasonGames_DefaultsToCurrentSeason(t *testing.T) {
	h := historyTestHandler(t)
	r := httptest.NewRequest("GET", "/history", nil)
	w := httptest.NewRecorder()
	h.SeasonGames(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 with no season param, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Wingspan") {
		t.Errorf("want default season content, got: %s", w.Body.String())
	}
}

func TestSeasonGames_InvalidParam_Returns400(t *testing.T) {
	h := historyTestHandler(t)
	r := httptest.NewRequest("GET", "/history?season=bad", nil)
	w := httptest.NewRecorder()
	h.SeasonGames(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}
