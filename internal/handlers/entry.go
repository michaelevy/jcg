package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
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
		"Title":            "Record Game Result",
		"Username":         middleware.UsernameFromContext(r),
		"Players":          players,
		"Seasons":          seasons,
		"Games":            games,
		"Today":            time.Now().Format("2006-01-02"),
		"SelectedSeasonID": int64(0),
	})
}

func (h *Handler) EntrySubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	seasonIDStr := r.FormValue("season_id")
	gameTitle := strings.TrimSpace(r.FormValue("game_title"))
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
		if strings.HasPrefix(key, "score_") && len(vals) > 0 && vals[0] != "" {
			playerIDStr := key[6:] // Extract portion after "score_"
			playerID, err := strconv.ParseInt(playerIDStr, 10, 64)
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid player ID in %s", key), http.StatusBadRequest)
				return
			}
			score, err := strconv.Atoi(vals[0])
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid score for player %d", playerID), http.StatusBadRequest)
				return
			}
			if score < 0 {
				http.Error(w, fmt.Sprintf("score for player %d cannot be negative", playerID), http.StatusBadRequest)
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
