package handlers

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"

	"jcg/internal/db"
	"jcg/internal/middleware"
)

func (h *Handler) Leaderboard(w http.ResponseWriter, r *http.Request) {
	var seasonID int64
	if raw := r.URL.Query().Get("season"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			http.Error(w, "invalid season", http.StatusBadRequest)
			return
		}
		seasonID = parsed
	}
	if seasonID == 0 {
		var err error
		seasonID, err = db.CurrentSeasonID(h.db)
		if err != nil {
			http.Error(w, "something has gone wrong which I haven't bothered to write a proper error message for", http.StatusInternalServerError)
			return
		}
	}

	seasons, err := db.ListSeasons(h.db)
	if err != nil {
		http.Error(w, "something has gone wrong which I haven't bothered to write a proper error message for", http.StatusInternalServerError)
		return
	}

	var rows []db.LeaderboardRow
	var currentSeason db.Season
	graphJSON := template.JS("null")
	if seasonID > 0 {
		rows, err = db.Leaderboard(h.db, seasonID)
		if err != nil {
			http.Error(w, "something has gone wrong which I haven't bothered to write a proper error message for", http.StatusInternalServerError)
			return
		}
		currentSeason, err = db.GetSeason(h.db, seasonID)
		if err != nil {
			http.Error(w, "something has gone wrong which I haven't bothered to write a proper error message for", http.StatusInternalServerError)
			return
		}
		cumulative, err := db.CumulativePoints(h.db, seasonID)
		if err != nil {
			http.Error(w, "something has gone wrong which I haven't bothered to write a proper error message for", http.StatusInternalServerError)
			return
		}
		// json.Marshal cannot fail on []CumulativePointsRow (no channels, funcs, or cyclic refs).
		b, _ := json.Marshal(cumulative)
		graphJSON = template.JS(b)
	}

	data := map[string]any{
		"Title":         "Leaderboard",
		"Username":      middleware.UsernameFromContext(r),
		"Seasons":       seasons,
		"CurrentSeason": currentSeason,
		"Rows":          rows,
		"SeasonID":      seasonID,
		"GraphJSON":     graphJSON,
	}

	// HTMX requests get only the table fragment (for the season selector swap).
	if r.Header.Get("HX-Request") == "true" {
		h.render(w, r, "leaderboard-table", data)
		return
	}

	h.render(w, r, "leaderboard", data)
}
