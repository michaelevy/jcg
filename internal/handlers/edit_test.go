package handlers

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"jcg/internal/db"
)

func editTestHandler(t *testing.T) *Handler {
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
	database.Exec(`INSERT INTO player_scores (result_id, player_id, placement, season_points) VALUES (1, 1, 1, 4), (1, 2, 2, 2)`)

	tmpl := template.Must(template.New("root").Funcs(template.FuncMap{
		"add": func(a, b int) int { return a + b },
	}).Parse(`
		{{define "game_result_edit"}}EDIT:{{.GameTitle}}:{{if .Error}}ERROR:{{.Error}}{{end}}{{end}}
	`))
	return New(database, tmpl)
}

func TestGetEditGameResult_RendersForm(t *testing.T) {
	h := editTestHandler(t)
	r := authenticatedRequest("GET", "/game-results/1/edit", "")
	r.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.GetEditGameResult(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Wingspan") {
		t.Errorf("want Wingspan in body, got: %s", body)
	}
}

func TestGetEditGameResult_InvalidID_Returns400(t *testing.T) {
	h := editTestHandler(t)
	r := authenticatedRequest("GET", "/game-results/bad/edit", "")
	r.SetPathValue("id", "bad")
	w := httptest.NewRecorder()
	h.GetEditGameResult(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestGetEditGameResult_NotFound_Returns404(t *testing.T) {
	h := editTestHandler(t)
	r := authenticatedRequest("GET", "/game-results/999/edit", "")
	r.SetPathValue("id", "999")
	w := httptest.NewRecorder()
	h.GetEditGameResult(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}

func TestPostEditGameResult_ValidSubmit_RedirectsAndUpdates(t *testing.T) {
	h := editTestHandler(t)

	form := url.Values{
		"season_id":   {"1"},
		"game_title":  {"Wingspan"},
		"game_number": {"1"},
		"place_1":     {"2"},
		"place_2":     {"1"},
	}
	r := authenticatedRequest("POST", "/game-results/1/edit", form.Encode())
	r.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.PostEditGameResult(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("want 303, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/game-results/1" {
		t.Errorf("want redirect to /game-results/1, got: %s", loc)
	}

	var p1Placement int
	h.db.QueryRow(`SELECT placement FROM player_scores WHERE result_id=1 AND player_id=1`).Scan(&p1Placement)
	if p1Placement != 2 {
		t.Errorf("want player 1 placement=2 after edit, got %d", p1Placement)
	}
}

func TestPostEditGameResult_DuplicateGameNumber_RerendersWithError(t *testing.T) {
	h := editTestHandler(t)
	// Add a second result so game_number=2 is taken.
	h.db.Exec(`INSERT INTO game_results (id, season_id, game_id, game_number) VALUES (2, 1, 1, 2)`)
	h.db.Exec(`INSERT INTO player_scores (result_id, player_id, placement, season_points) VALUES (2, 1, 1, 4), (2, 2, 2, 2)`)

	form := url.Values{
		"season_id":   {"1"},
		"game_title":  {"Wingspan"},
		"game_number": {"2"}, // conflicts with result 2
		"place_1":     {"1"},
		"place_2":     {"2"},
	}
	r := authenticatedRequest("POST", "/game-results/1/edit", form.Encode())
	r.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.PostEditGameResult(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 (form re-render), got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ERROR") {
		t.Errorf("want error message in re-rendered form, got: %s", w.Body.String())
	}
}

func TestPostEditGameResult_NotFound_Returns404(t *testing.T) {
	h := editTestHandler(t)

	form := url.Values{
		"season_id":   {"1"},
		"game_title":  {"Wingspan"},
		"game_number": {"1"},
		"place_1":     {"1"},
		"place_2":     {"2"},
	}
	r := authenticatedRequest("POST", "/game-results/999/edit", form.Encode())
	r.SetPathValue("id", "999")
	w := httptest.NewRecorder()
	h.PostEditGameResult(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}
