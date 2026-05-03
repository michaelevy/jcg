package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"jcg/internal/db"
	"jcg/internal/middleware"
)

func (h *Handler) GetEditGameResult(w http.ResponseWriter, r *http.Request) {
	raw := r.PathValue("id")
	resultID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || resultID <= 0 {
		http.Error(w, "invalid result id", http.StatusBadRequest)
		return
	}

	detail, err := db.GetGameResult(h.db, resultID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "game result not found", http.StatusNotFound)
			return
		}
		http.Error(w, "something has gone wrong which I haven't bothered to write a proper error message for", http.StatusInternalServerError)
		return
	}

	seasons, err := db.ListSeasons(h.db)
	if err != nil {
		http.Error(w, "something has gone wrong which I haven't bothered to write a proper error message for", http.StatusInternalServerError)
		return
	}
	games, err := db.ListGames(h.db)
	if err != nil {
		http.Error(w, "something has gone wrong which I haven't bothered to write a proper error message for", http.StatusInternalServerError)
		return
	}

	placements := make(map[int64]int, len(detail.Placements))
	for _, p := range detail.Placements {
		placements[p.PlayerID] = p.Placement
	}

	h.render(w, r, "game_result_edit", map[string]any{
		"Title":            "Edit Game Result",
		"Username":         middleware.UsernameFromContext(r),
		"Detail":           detail,
		"Seasons":          seasons,
		"Games":            games,
		"SelectedSeasonID": detail.SeasonID,
		"GameTitle":        detail.GameTitle,
		"GameNumber":       detail.GameNumber,
		"Placements":       placements,
	})
}

func (h *Handler) PostEditGameResult(w http.ResponseWriter, r *http.Request) {
	raw := r.PathValue("id")
	resultID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || resultID <= 0 {
		http.Error(w, "invalid result id", http.StatusBadRequest)
		return
	}

	// Load detail upfront to validate player IDs later.
	detail, err := db.GetGameResult(h.db, resultID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "game result not found", http.StatusNotFound)
			return
		}
		http.Error(w, "something has gone wrong which I haven't bothered to write a proper error message for", http.StatusInternalServerError)
		return
	}

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

	placements := map[int64]int{}
	for key, vals := range r.Form {
		if strings.HasPrefix(key, "place_") && len(vals) > 0 && vals[0] != "" {
			playerIDStr := key[6:]
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

	// Validate that all submitted player IDs belong to the original game result.
	validPlayerIDs := make(map[int64]bool)
	for _, p := range detail.Placements {
		validPlayerIDs[p.PlayerID] = true
	}
	for playerID := range placements {
		if !validPlayerIDs[playerID] {
			http.Error(w, fmt.Sprintf("player %d was not in this game", playerID), http.StatusBadRequest)
			return
		}
	}

	// NOTE: CreateGame is called before UpdateGameResult. If UpdateGameResult fails
	// (e.g., on duplicate game number), the new game row persists as tech debt.
	// This is a known pattern shared with entry.go; refactoring to fold CreateGame
	// into UpdateGameResult is out of scope. Future readers: this is intentional.
	gameID, err := db.CreateGame(h.db, gameTitle)
	if err != nil {
		http.Error(w, "something has gone wrong which I haven't bothered to write a proper error message for", http.StatusInternalServerError)
		return
	}

	scores := db.PlacementsToScores(placements)

	if err := db.UpdateGameResult(h.db, resultID, seasonID, gameID, gameNumber, scores); err != nil {
		if errors.Is(err, db.ErrDuplicateGameNumber) {
			seasons, dbErr := db.ListSeasons(h.db)
			if dbErr != nil {
				http.Error(w, "something has gone wrong which I haven't bothered to write a proper error message for", http.StatusInternalServerError)
				return
			}
			games, dbErr := db.ListGames(h.db)
			if dbErr != nil {
				http.Error(w, "something has gone wrong which I haven't bothered to write a proper error message for", http.StatusInternalServerError)
				return
			}
			h.render(w, r, "game_result_edit", map[string]any{
				"Title":            "Edit Game Result",
				"Username":         middleware.UsernameFromContext(r),
				"Detail":           detail,
				"Seasons":          seasons,
				"Games":            games,
				"SelectedSeasonID": seasonID,
				"GameTitle":        gameTitle,
				"GameNumber":       gameNumber,
				"Placements":       placements,
				"Error":            fmt.Sprintf("game #%d already exists for this season", gameNumber),
			})
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "game result not found", http.StatusNotFound)
			return
		}
		http.Error(w, "something has gone wrong which I haven't bothered to write a proper error message for", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/game-results/%d", resultID), http.StatusSeeOther)
}
