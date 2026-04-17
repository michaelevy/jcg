package handlers

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"jcg/internal/db"
	"jcg/internal/middleware"
)

func entryTestHandler(t *testing.T) *Handler {
	t.Helper()
	database, err := db.Open("file::memory:?cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if _, err := database.Exec(`INSERT INTO players (id, name) VALUES (1, 'Alice'), (2, 'Bob'), (3, 'Carol'), (4, 'Dan')`); err != nil {
		t.Fatalf("seed players: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO seasons (id, name) VALUES (1, 'Season 1')`); err != nil {
		t.Fatalf("seed seasons: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO games (id, title) VALUES (1, 'Wingspan')`); err != nil {
		t.Fatalf("seed games: %v", err)
	}

	tmpl := template.Must(template.New("root").Parse(`
		{{define "entry"}}ENTRY{{end}}
		{{define "season-options"}}{{range .Seasons}}<option value="{{.ID}}">{{.Name}}</option>{{end}}{{end}}
		{{define "home"}}HOME{{end}}
	`))

	return New(database, tmpl)
}

// authenticatedRequest creates a request with a username injected into context,
// simulating what RequireAuth would do for authenticated requests.
func authenticatedRequest(method, path, body string) *http.Request {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	return r.WithContext(middleware.InjectUsername(r.Context(), "admin"))
}

func TestEntryPage_ReturnsOK(t *testing.T) {
	h := entryTestHandler(t)
	r := authenticatedRequest("GET", "/enter", "")
	w := httptest.NewRecorder()

	h.EntryPage(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ENTRY") {
		t.Errorf("want ENTRY in body, got: %s", w.Body.String())
	}
}

func TestEntrySubmit_ValidResult_RedirectsAndPersists(t *testing.T) {
	h := entryTestHandler(t)

	form := url.Values{
		"season_id":   {"1"},
		"game_number": {"1"},
		"game_title":  {"Wingspan"},

		"place_1":    {"1"},
		"place_2":    {"2"},
	}
	r := authenticatedRequest("POST", "/enter", form.Encode())
	w := httptest.NewRecorder()

	h.EntrySubmit(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("want 303, got %d", w.Code)
	}

	var resultCount, scoreCount int
	h.db.QueryRow(`SELECT COUNT(*) FROM game_results`).Scan(&resultCount)
	h.db.QueryRow(`SELECT COUNT(*) FROM player_scores`).Scan(&scoreCount)
	if resultCount != 1 {
		t.Errorf("want 1 game_results row, got %d", resultCount)
	}
	if scoreCount != 2 {
		t.Errorf("want 2 player_scores rows, got %d", scoreCount)
	}
}

func TestEntrySubmit_MissingRequiredFields_Returns400(t *testing.T) {
	h := entryTestHandler(t)

	form := url.Values{
		"season_id": {"1"},
		// missing game_title
	}
	r := authenticatedRequest("POST", "/enter", form.Encode())
	w := httptest.NewRecorder()

	h.EntrySubmit(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestEntrySubmit_OnlyOnePlacement_Returns400(t *testing.T) {
	h := entryTestHandler(t)

	form := url.Values{
		"season_id":   {"1"},
		"game_number": {"1"},
		"game_title":  {"Wingspan"},
		"place_1":     {"1"},
	}
	r := authenticatedRequest("POST", "/enter", form.Encode())
	w := httptest.NewRecorder()

	h.EntrySubmit(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestEntrySubmit_NegativeScore_Returns400(t *testing.T) {
	h := entryTestHandler(t)

	form := url.Values{
		"season_id":   {"1"},
		"game_number": {"1"},
		"game_title":  {"Wingspan"},

		"place_1":    {"-5"},
		"score_2":    {"10"},
	}
	r := authenticatedRequest("POST", "/enter", form.Encode())
	w := httptest.NewRecorder()

	h.EntrySubmit(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestEntrySubmit_WhitespaceGameTitle_Returns400(t *testing.T) {
	h := entryTestHandler(t)

	form := url.Values{
		"season_id":  {"1"},
		"game_title": {"   "},

		"place_1":    {"1"},
		"place_2":    {"2"},
	}
	r := authenticatedRequest("POST", "/enter", form.Encode())
	w := httptest.NewRecorder()

	h.EntrySubmit(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestEntrySubmit_MalformedScoreKey_Returns400(t *testing.T) {
	h := entryTestHandler(t)

	form := url.Values{
		"season_id":   {"1"},
		"game_number": {"1"},
		"game_title":  {"Wingspan"},

		"place_abc":  {"1"},
		"place_def":  {"2"},
	}
	r := authenticatedRequest("POST", "/enter", form.Encode())
	w := httptest.NewRecorder()

	h.EntrySubmit(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestCreateSeason_CreatesSeasonAndReturnsOptions(t *testing.T) {
	h := entryTestHandler(t)

	form := url.Values{"season_name": {"Season 2"}}
	r := authenticatedRequest("POST", "/enter/season", form.Encode())
	w := httptest.NewRecorder()

	h.CreateSeason(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Season 1") || !strings.Contains(body, "Season 2") {
		t.Errorf("want both seasons in options fragment, got: %s", body)
	}
}
