# JCG — Critical Path Implementation Plan

**Goal:** Add authenticated data entry for recording game results — selecting/creating a season, picking a board game, and entering per-player scores with automatic placement and season-point calculation.

**Architecture:** Two handlers (`GET /enter` shows the form, `POST /enter` processes it) wrapped individually in `RequireAuth` — no nested mux. The server computes placement (rank by descending score) and season points (3/2/1/0) from submitted scores. DB query helpers live in `internal/db/queries.go`. An HTMX-powered inline form lets users add a new season without leaving the page.

**Tech Stack:** net/http, html/template, HTMX (inline season creation), database/sql

**Scope:** Phase 3 of 4 (F3 from design) — builds on Phases 1 and 2

**Codebase verified:** 2026-04-12 — schema has players, seasons, games, game_results, player_scores tables; RequireAuth middleware is implemented; `InjectUsername` helper is in the middleware package

---

<!-- START_SUBCOMPONENT_A (tasks 1-2) -->
<!-- START_TASK_1 -->
### Task 1: DB Query Helpers

**Files:**
- Create: `internal/db/queries.go`
- Create: `internal/db/queries_test.go`

**Step 1: Write the query helpers**

Create `internal/db/queries.go`:
```go
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
```

**Step 2: Write tests for ComputePlacements and InsertGameResult**

Create `internal/db/queries_test.go`:
```go
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

	database.Exec(`INSERT INTO players (id, name) VALUES (1, 'Alice'), (2, 'Bob')`)
	database.Exec(`INSERT INTO seasons (id, name) VALUES (1, 'Season 1')`)
	database.Exec(`INSERT INTO games (id, title) VALUES (1, 'Wingspan')`)

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
```

**Step 3: Run the tests**

```bash
go test ./internal/db/ -v
```

Expected: All tests PASS.

**Step 4: Commit**

```bash
git add internal/db/queries.go internal/db/queries_test.go
git commit -m "feat: add DB query helpers for players, seasons, games, and result insertion"
```
<!-- END_TASK_1 -->

<!-- START_TASK_2 -->
### Task 2: Entry Form Handlers and Template

**Files:**
- Create: `internal/handlers/entry.go`
- Create: `cmd/server/templates/entry.html`
- Modify: `cmd/server/main.go` (replace placeholder /enter route with real handlers)

**Step 1: Create entry handler**

Create `internal/handlers/entry.go`:
```go
package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"jcg/internal/db"
	"jcg/internal/middleware"
)

func (h *Handler) EntryPage(w http.ResponseWriter, r *http.Request) {
	players, err := db.ListPlayers(h.db)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	seasons, err := db.ListSeasons(h.db)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	games, err := db.ListGames(h.db)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	h.render(w, "entry", map[string]any{
		"Title":    "Record Game Result",
		"Username": middleware.UsernameFromContext(r),
		"Players":  players,
		"Seasons":  seasons,
		"Games":    games,
		"Today":    time.Now().Format("2006-01-02"),
	})
}

func (h *Handler) EntrySubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	seasonIDStr := r.FormValue("season_id")
	gameTitle := r.FormValue("game_title")
	playedAt := r.FormValue("played_at")

	if seasonIDStr == "" || gameTitle == "" || playedAt == "" {
		http.Error(w, "season, game, and date are required", http.StatusBadRequest)
		return
	}

	seasonID, err := strconv.ParseInt(seasonIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid season", http.StatusBadRequest)
		return
	}

	gameID, err := db.CreateGame(h.db, gameTitle)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	// Parse per-player scores from form fields named "score_<playerID>".
	rawScores := map[int64]int{}
	for key, vals := range r.Form {
		var playerID int64
		if n, _ := fmt.Sscanf(key, "score_%d", &playerID); n == 1 && len(vals) > 0 && vals[0] != "" {
			score, err := strconv.Atoi(vals[0])
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid score for player %d", playerID), http.StatusBadRequest)
				return
			}
			rawScores[playerID] = score
		}
	}

	if len(rawScores) < 2 {
		http.Error(w, "enter scores for at least 2 players", http.StatusBadRequest)
		return
	}

	scored := db.ComputePlacements(rawScores)

	if err := db.InsertGameResult(h.db, seasonID, gameID, playedAt, scored); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// CreateSeason handles the HTMX inline season-creation sub-form.
// Returns an updated <select> options fragment for the season dropdown.
func (h *Handler) CreateSeason(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("season_name")
	if name == "" {
		http.Error(w, "season name is required", http.StatusBadRequest)
		return
	}

	id, err := db.CreateSeason(h.db, name)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	seasons, err := db.ListSeasons(h.db)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	// Return just the <option> elements so HTMX can swap them into the <select>.
	h.render(w, "season-options", map[string]any{
		"Seasons":          seasons,
		"SelectedSeasonID": id,
	})
}
```

**Step 2: Create entry template**

Create `cmd/server/templates/entry.html`:
```html
{{define "entry"}}
<!DOCTYPE html>
<html lang="en">
<head>{{template "head" .}}</head>
<body>
  {{template "nav" .}}
  <main>
    <h1>Record Game Result</h1>

    <form method="POST" action="/enter" class="form-narrow">

      <label for="season_id">Season</label>
      <select id="season_id" name="season_id" required>
        <option value="">— select —</option>
        {{template "season-options" .}}
      </select>

      <details style="margin-top: 0.5rem;">
        <summary style="cursor:pointer; font-size: 0.9rem; color: #0066cc;">+ New season</summary>
        <div style="margin-top: 0.5rem; display: flex; gap: 0.5rem;">
          <input id="season_name" name="season_name" type="text" placeholder="Season name"
                 style="flex: 1;">
          <button type="button"
                  hx-post="/enter/season"
                  hx-target="#season_id"
                  hx-swap="innerHTML"
                  hx-include="#season_name">Add</button>
        </div>
      </details>

      <label for="game_title">Game</label>
      <input id="game_title" name="game_title" list="games-list" type="text" required placeholder="Start typing...">
      <datalist id="games-list">
        {{range .Games}}<option value="{{.Title}}">{{end}}
      </datalist>

      <label for="played_at">Date played</label>
      <input id="played_at" name="played_at" type="date" value="{{.Today}}" required>

      <fieldset style="margin-top: 1.5rem; border: 1px solid #ccc; border-radius: 4px; padding: 1rem;">
        <legend>Scores <span style="font-weight: normal; font-size: 0.85rem;">(leave blank for absent players)</span></legend>
        {{range .Players}}
        <div style="display: flex; align-items: center; gap: 1rem; margin-top: 0.5rem;">
          <label style="min-width: 120px; font-weight: normal;">{{.Name}}</label>
          <input type="number" name="score_{{.ID}}" min="0" placeholder="score"
                 style="width: 100px;">
        </div>
        {{end}}
      </fieldset>

      <button type="submit" class="btn-primary">Save result</button>
    </form>
  </main>
</body>
</html>
{{end}}

{{define "season-options"}}
{{range .Seasons}}
<option value="{{.ID}}" {{if eq .ID $.SelectedSeasonID}}selected{{end}}>{{.Name}}</option>
{{end}}
{{end}}
```

**Step 3: Replace the placeholder /enter route in main.go**

Find and remove the placeholder /enter handler added in Phase 2. It looks like:
```go
// /enter is protected — placeholder until Phase 3 replaces it.
mux.Handle("GET /enter", middleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    http.Error(w, "entry coming in phase 3", http.StatusNotImplemented)
})))
```

Replace it with individual protected routes (no nested mux — each route is wrapped separately):
```go
mux.Handle("GET /enter", middleware.RequireAuth(http.HandlerFunc(h.EntryPage)))
mux.Handle("POST /enter", middleware.RequireAuth(http.HandlerFunc(h.EntrySubmit)))
mux.Handle("POST /enter/season", middleware.RequireAuth(http.HandlerFunc(h.CreateSeason)))
```

**Step 4: Verify the entry form loads**

```bash
go run ./cmd/server &
sleep 1
```

Test that unauthenticated /enter redirects:
```bash
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/enter
```
Expected: `303`

Login and verify the form loads (adjust credentials to match Phase 2 seeding):
```bash
curl -s -c /tmp/jcg-cookies.txt -d "username=admin&password=changeme" -L http://localhost:8080/login
curl -s -b /tmp/jcg-cookies.txt http://localhost:8080/enter | grep "Record Game Result"
```
Expected: Returns HTML containing "Record Game Result".

```bash
kill %1
```

**Step 5: Commit**

```bash
git add internal/handlers/entry.go cmd/server/templates/entry.html cmd/server/main.go
git commit -m "feat: add authenticated game result entry form with HTMX season creation"
```
<!-- END_TASK_2 -->
<!-- END_SUBCOMPONENT_A -->

<!-- START_TASK_3 -->
### Task 3: Entry Handler Tests

**Files:**
- Create: `internal/handlers/entry_test.go`

**Step 1: Write the tests**

Create `internal/handlers/entry_test.go`:
```go
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

	database.Exec(`INSERT INTO players (id, name) VALUES (1, 'Alice'), (2, 'Bob'), (3, 'Carol'), (4, 'Dan')`)
	database.Exec(`INSERT INTO seasons (id, name) VALUES (1, 'Season 1')`)
	database.Exec(`INSERT INTO games (id, title) VALUES (1, 'Wingspan')`)

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
		"season_id":  {"1"},
		"game_title": {"Wingspan"},
		"played_at":  {"2026-04-12"},
		"score_1":    {"90"},
		"score_2":    {"70"},
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
		// missing game_title and played_at
	}
	r := authenticatedRequest("POST", "/enter", form.Encode())
	w := httptest.NewRecorder()

	h.EntrySubmit(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestEntrySubmit_OnlyOneScore_Returns400(t *testing.T) {
	h := entryTestHandler(t)

	form := url.Values{
		"season_id":  {"1"},
		"game_title": {"Wingspan"},
		"played_at":  {"2026-04-12"},
		"score_1":    {"90"},
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
```

**Step 2: Run the tests**

```bash
go test ./internal/handlers/ -v -run "TestEntry|TestCreateSeason"
```

Expected: All tests PASS.

**Step 3: Run full test suite**

```bash
go test ./...
```

Expected: All tests pass across all packages.

**Step 4: Commit**

```bash
git add internal/handlers/entry_test.go
git commit -m "test: add entry form handler tests"
```
<!-- END_TASK_3 -->
