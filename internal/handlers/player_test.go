package handlers

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jcg/internal/db"
)

func playerTestHandler(t *testing.T) *Handler {
	t.Helper()
	database, err := db.Open("file::memory:?cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	database.Exec(`INSERT INTO players (id, name) VALUES (1, 'Alice'), (2, 'Bob')`)
	database.Exec(`INSERT INTO seasons (id, name) VALUES (1, 'Season 1')`)
	database.Exec(`INSERT INTO games (id, title) VALUES (1, 'Wingspan')`)
	database.Exec(`INSERT INTO game_results (id, season_id, game_id, game_number) VALUES (1, 1, 1, 1)`)
	database.Exec(`INSERT INTO player_scores (result_id, player_id, placement, season_points)
		VALUES (1, 1, 1, 4), (1, 2, 2, 2)`)

	tmpl := template.Must(template.New("").Funcs(template.FuncMap{
		"add": func(a, b int) int { return a + b },
	}).Parse(`
		{{define "player"}}PLAYER:{{.Player.Name}}:{{range .SeasonStats}}{{.SeasonName}}={{.TotalPoints}};{{end}}{{end}}
	`))
	return New(database, tmpl)
}

func TestPlayerProfile_RendersPlayerData(t *testing.T) {
	h := playerTestHandler(t)
	r := httptest.NewRequest("GET", "/players/1", nil)
	r.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.PlayerProfile(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.HasPrefix(body, "PLAYER:") {
		t.Errorf("want player template, got: %s", body)
	}
	if !strings.Contains(body, "Alice") {
		t.Errorf("want Alice in response, got: %s", body)
	}
	if !strings.Contains(body, "Season 1") {
		t.Errorf("want Season 1 stats, got: %s", body)
	}
}

func TestPlayerProfile_InvalidID_Returns400(t *testing.T) {
	h := playerTestHandler(t)
	r := httptest.NewRequest("GET", "/players/bad", nil)
	r.SetPathValue("id", "bad")
	w := httptest.NewRecorder()
	h.PlayerProfile(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestPlayerProfile_NotFound_Returns404(t *testing.T) {
	h := playerTestHandler(t)
	r := httptest.NewRequest("GET", "/players/999", nil)
	r.SetPathValue("id", "999")
	w := httptest.NewRecorder()
	h.PlayerProfile(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}
