package handlers

import (
	"net/http"
	"strconv"

	"jcg/internal/db"
	"jcg/internal/middleware"
)

func (h *Handler) SeasonGames(w http.ResponseWriter, r *http.Request) {
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

	var games []db.GameSummary
	var season db.Season
	if seasonID > 0 {
		season, err = db.GetSeason(h.db, seasonID)
		if err != nil {
			http.Error(w, "something has gone wrong which I haven't bothered to write a proper error message for", http.StatusInternalServerError)
			return
		}
		games, err = db.SeasonHistory(h.db, seasonID)
		if err != nil {
			http.Error(w, "something has gone wrong which I haven't bothered to write a proper error message for", http.StatusInternalServerError)
			return
		}
	}

	data := map[string]any{
		"Title":    "Game History",
		"Username": middleware.UsernameFromContext(r),
		"Season":   season,
		"SeasonID": seasonID,
		"Seasons":  seasons,
		"Games":    games,
	}

	if r.Header.Get("HX-Request") == "true" {
		h.render(w, "history-table", data)
		return
	}
	h.render(w, "history", data)
}
