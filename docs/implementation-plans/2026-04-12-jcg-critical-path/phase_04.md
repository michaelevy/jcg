# JCG — Critical Path Implementation Plan

**Goal:** Build the season leaderboard — the public home page showing ranked players with wins and total points, plus an HTMX-powered season selector.

**Architecture:** The leaderboard handler runs a single aggregation SQL query for a given season ID. The season selector uses HTMX to swap only the leaderboard table (no full-page reload). The current season defaults to the most recently created one. The placeholder `Home` handler and `home.html` template from Phase 1 are removed.

**Tech Stack:** net/http, html/template, HTMX (hx-get + hx-swap for season selector), database/sql

**Scope:** Phase 4 of 4 (F4 from design) — builds on Phases 1–3

**Codebase verified:** 2026-04-12 — player_scores and game_results tables exist; seasons, games, players tables exist; handlers package has render() helper

---

<!-- START_SUBCOMPONENT_A (tasks 1-2) -->
<!-- START_TASK_1 -->
### Task 1: Leaderboard DB Query

**Files:**
- Modify: `internal/db/queries.go`
- Modify: `internal/db/queries_test.go`

**Step 1: Add leaderboard types and query functions**

Append the following to `internal/db/queries.go` (after the existing code):
```go
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
```

**Step 2: Add leaderboard tests**

Append to `internal/db/queries_test.go`:
```go
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
	database.Exec(`INSERT INTO game_results (id, season_id, game_id, played_at) VALUES (1, 1, 1, '2026-04-12')`)
	database.Exec(`INSERT INTO player_scores (result_id, player_id, score, placement, season_points)
		VALUES (1, 1, 100, 1, 4), (1, 2, 80, 2, 2), (1, 3, 60, 3, 1)`)

	rows, err := Leaderboard(database, 1)
	if err != nil {
		t.Fatalf("Leaderboard: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(rows))
	}
	if rows[0].PlayerName != "Alice" || rows[0].TotalPoints != 3 || rows[0].Wins != 1 {
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
```

**Step 3: Run the tests**

```bash
go test ./internal/db/ -v
```

Expected: All tests PASS, including the new leaderboard and CurrentSeasonID tests.

**Step 4: Commit**

```bash
git add internal/db/queries.go internal/db/queries_test.go
git commit -m "feat: add leaderboard query, CurrentSeasonID, and GetSeason DB helpers"
```
<!-- END_TASK_1 -->

<!-- START_TASK_2 -->
### Task 2: Leaderboard Handler and Template

**Files:**
- Create: `internal/handlers/views.go`
- Create: `cmd/server/templates/leaderboard.html`
- Modify: `internal/handlers/handler.go` (remove placeholder Home method)
- Modify: `cmd/server/main.go` (register `add` template func, swap Home route for Leaderboard)

**Step 1: Create views handler**

Create `internal/handlers/views.go`:
```go
package handlers

import (
	"net/http"
	"strconv"

	"jcg/internal/db"
	"jcg/internal/middleware"
)

func (h *Handler) Leaderboard(w http.ResponseWriter, r *http.Request) {
	var seasonID int64
	if raw := r.URL.Query().Get("season"); raw != "" {
		seasonID, _ = strconv.ParseInt(raw, 10, 64)
	}
	if seasonID == 0 {
		var err error
		seasonID, err = db.CurrentSeasonID(h.db)
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
	}

	seasons, err := db.ListSeasons(h.db)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	var rows []db.LeaderboardRow
	var currentSeason db.Season
	if seasonID > 0 {
		rows, err = db.Leaderboard(h.db, seasonID)
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		currentSeason, err = db.GetSeason(h.db, seasonID)
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
	}

	data := map[string]any{
		"Title":         "Leaderboard",
		"Username":      middleware.UsernameFromContext(r),
		"Seasons":       seasons,
		"CurrentSeason": currentSeason,
		"Rows":          rows,
		"SeasonID":      seasonID,
	}

	// HTMX requests get only the table fragment (for the season selector swap).
	if r.Header.Get("HX-Request") == "true" {
		h.render(w, "leaderboard-table", data)
		return
	}

	h.render(w, "leaderboard", data)
}
```

**Step 2: Create leaderboard template**

Create `cmd/server/templates/leaderboard.html`:
```html
{{define "leaderboard"}}
<!DOCTYPE html>
<html lang="en">
<head>{{template "head" .}}</head>
<body>
  {{template "nav" .}}
  <main>
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem;">
      <h1>
        {{if .CurrentSeason.Name}}{{.CurrentSeason.Name}}{{else}}Leaderboard{{end}}
      </h1>

      {{if .Seasons}}
      <select id="season-selector"
              hx-get="/"
              hx-target="#leaderboard-section"
              hx-swap="innerHTML"
              hx-include="#season-selector"
              name="season"
              style="padding: 0.4rem 0.6rem; border: 1px solid #ccc; border-radius: 4px;">
        {{range .Seasons}}
        <option value="{{.ID}}" {{if eq .ID $.SeasonID}}selected{{end}}>{{.Name}}</option>
        {{end}}
      </select>
      {{end}}
    </div>

    {{if .Username}}
    <p style="margin-bottom: 1.5rem;">
      <a href="/enter">Record a game result</a>
    </p>
    {{end}}

    <div id="leaderboard-section">
      {{template "leaderboard-table" .}}
    </div>
  </main>
</body>
</html>
{{end}}

{{define "leaderboard-table"}}
{{if not .Rows}}
  <p>No results yet for this season.
    {{if .Username}}<a href="/enter">Record the first game</a>.{{end}}
  </p>
{{else}}
<table>
  <thead>
    <tr>
      <th>#</th>
      <th>Player</th>
      <th>Points</th>
      <th>Wins</th>
      <th>Games</th>
    </tr>
  </thead>
  <tbody>
    {{range $i, $r := .Rows}}
    <tr>
      <td>{{add $i 1}}</td>
      <td>{{$r.PlayerName}}</td>
      <td><strong>{{$r.TotalPoints}}</strong></td>
      <td>{{$r.Wins}}</td>
      <td>{{$r.GamesPlayed}}</td>
    </tr>
    {{end}}
  </tbody>
</table>
{{end}}
{{end}}
```

**Step 3: Remove the placeholder Home method from handler.go**

In `internal/handlers/handler.go`, delete the Home method and remove the now-unused `middleware` import:

Remove this entire method:
```go
// Home is a placeholder until the leaderboard is built in Phase 4.
func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
	h.render(w, "home", map[string]any{
		"Title":    "Home",
		"Username": middleware.UsernameFromContext(r),
	})
}
```

Update the imports in `internal/handlers/handler.go` — remove `"jcg/internal/middleware"` if it is no longer used in that file (it's used in `auth.go` and `views.go`, just not `handler.go` after this change):
```go
import (
	"database/sql"
	"html/template"
	"log"
	"net/http"
)
```

**Step 4: Delete the obsolete home template**

```bash
rm cmd/server/templates/home.html
```

**Step 5: Register the `add` template function and swap routes in main.go**

In `cmd/server/main.go`, replace:
```go
tmpl := template.Must(template.ParseFS(templateFS, "templates/*.html"))
```
With:
```go
tmpl := template.Must(
    template.New("").Funcs(template.FuncMap{
        "add": func(a, b int) int { return a + b },
    }).ParseFS(templateFS, "templates/*.html"),
)
```

Replace the home route:
```go
mux.HandleFunc("GET /{$}", h.Home)
```
With:
```go
mux.HandleFunc("GET /{$}", h.Leaderboard)
```

**Step 6: Verify build and server**

```bash
go build ./...
go run ./cmd/server &
sleep 1
curl -s http://localhost:8080/ | grep "Leaderboard"
kill %1
```

Expected: curl returns HTML containing "Leaderboard". If you see a template parse error about the `add` function, check that the FuncMap is registered before `ParseFS`.

**Step 7: Commit**

```bash
git add internal/handlers/views.go internal/handlers/handler.go cmd/server/templates/leaderboard.html cmd/server/main.go
git rm cmd/server/templates/home.html
git commit -m "feat: add leaderboard view with HTMX season selector; remove placeholder home template"
```
<!-- END_TASK_2 -->
<!-- END_SUBCOMPONENT_A -->

<!-- START_TASK_3 -->
### Task 3: Leaderboard Handler Tests

**Files:**
- Create: `internal/handlers/views_test.go`

**Step 1: Write the tests**

Create `internal/handlers/views_test.go`:
```go
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
`1`
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
```

**Step 2: Run the tests**

```bash
go test ./internal/handlers/ -v -run TestLeaderboard
```

Expected: All tests PASS.

**Step 3: Run the full test suite**

```bash
go test ./...
```

Expected: All tests pass across all packages.

**Step 4: Commit**

```bash
git add internal/handlers/views_test.go
git commit -m "test: add leaderboard handler tests"
```
<!-- END_TASK_3 -->

<!-- START_TASK_4 -->
### Task 4: Seed Players and Smoke Test

**Files:** No new files — this task uses the seeder from Phase 2.

**Step 1: Seed the four players**

Run once for each player (use the real names of the group members):
```bash
go run ./cmd/seed -player "Player One"
go run ./cmd/seed -player "Player Two"
go run ./cmd/seed -player "Player Three"
go run ./cmd/seed -player "Player Four"
```

Expected: Each prints `Player "[name]" added (or already exists).`

Verify:
```bash
sqlite3 jcg.db "SELECT id, name FROM players;"
```

Expected: 4 rows.

**Step 2: Manual smoke test**

```bash
go run ./cmd/server &
sleep 1
```

Open http://localhost:8080 in a browser and verify:

1. Leaderboard page loads — shows 4 players with 0 points each (no seasons yet)
2. Click Login → log in with admin account → nav shows username + Logout
3. "Record a game result" link appears on the leaderboard
4. Go to /enter → form shows all 4 players and a season selector
5. Click "+ New season" → type a name → click Add → season appears in the dropdown
6. Select the season, enter a board game name, enter scores for 2+ players, submit
7. Redirects to / → leaderboard now shows points, correct player ranking
8. Use the season selector dropdown → leaderboard updates without a full page reload (HTMX swap)

```bash
kill %1
```

**Step 3: Docker smoke test**

```bash
docker compose build
docker compose up -d
sleep 3
curl -s http://localhost:8080/ | grep "Leaderboard"
docker compose down
```

Expected: curl returns HTML containing "Leaderboard".

**Phase 4 complete. The critical path (F1–F4) is done.**

The app has:
- F1 Foundation: Go+SQLite+HTMX+Docker
- F2 Auth: session-based login/logout with bcrypt
- F3 Data entry: authenticated game result recording with HTMX season creation
- F4 Leaderboard: ranked by season points with HTMX season selector

Next phases (F5–F9) add game history, player profiles, game detail views, points graph, and historical import.
<!-- END_TASK_4 -->
