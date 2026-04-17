package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

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
	gameNumberStr := r.FormValue("game_number")

	if seasonIDStr == "" || gameTitle == "" || gameNumberStr == "" {
		http.Error(w, "season, game, and game number are required", http.StatusBadRequest)
		return
	}

	seasonID, err := strconv.ParseInt(seasonIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid season", http.StatusBadRequest)
		return
	}

	gameNumber, err := strconv.Atoi(gameNumberStr)
	if err != nil || gameNumber < 1 {
		http.Error(w, "game number must be a positive integer", http.StatusBadRequest)
		return
	}

	gameID, err := db.CreateGame(h.db, gameTitle)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	// Parse per-player placements from form fields named "place_<playerID>".
	placements := map[int64]int{}
	for key, vals := range r.Form {
		if strings.HasPrefix(key, "place_") && len(vals) > 0 && vals[0] != "" {
			playerIDStr := key[6:] // Extract portion after "place_"
			playerID, err := strconv.ParseInt(playerIDStr, 10, 64)
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid player ID in %s", key), http.StatusBadRequest)
				return
			}
			place, err := strconv.Atoi(vals[0])
			if err != nil || place < 1 {
				http.Error(w, fmt.Sprintf("invalid placement for player %d", playerID), http.StatusBadRequest)
				return
			}
			placements[playerID] = place
		}
	}

	if len(placements) < 2 {
		http.Error(w, "enter placements for at least 2 players", http.StatusBadRequest)
		return
	}

	scored := db.PlacementsToScores(placements)

	if err := db.InsertGameResult(h.db, seasonID, gameID, gameNumber, scored); err != nil {
		if errors.Is(err, db.ErrDuplicateGameNumber) {
			http.Error(w, fmt.Sprintf("game #%d already exists for this season", gameNumber), http.StatusBadRequest)
			return
		}
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// NextGameNumber handles the HTMX request to auto-fill the game number field
// when a season is selected. Returns the game-number-input template fragment.
func (h *Handler) NextGameNumber(w http.ResponseWriter, r *http.Request) {
	seasonIDStr := r.URL.Query().Get("season_id")
	seasonID, err := strconv.ParseInt(seasonIDStr, 10, 64)
	if err != nil || seasonID <= 0 {
		h.render(w, "game-number-input", map[string]any{"NextGameNumber": 0})
		return
	}

	next, err := db.NextGameNumber(h.db, seasonID)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	h.render(w, "game-number-input", map[string]any{"NextGameNumber": next})
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
