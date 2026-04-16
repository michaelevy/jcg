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
