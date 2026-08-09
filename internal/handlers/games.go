package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"jcg/internal/db"
	"jcg/internal/middleware"
)

func (h *Handler) Games(w http.ResponseWriter, r *http.Request) {
	seasons, matrix, err := db.GameSeasonMatrix(h.db)
	if err != nil {
		http.Error(w, "something has gone wrong which I haven't bothered to write a proper error message for", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "games", map[string]any{
		"Title":    "Games",
		"Username": middleware.UsernameFromContext(r),
		"Seasons":  seasons,
		"Matrix":   matrix,
	})
}

func (h *Handler) GameDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	game, err := db.GetGame(h.db, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "game not found", http.StatusNotFound)
			return
		}
		http.Error(w, "something has gone wrong which I haven't bothered to write a proper error message for", http.StatusInternalServerError)
		return
	}

	totals, err := db.GameSeasonPointsTotals(h.db, id)
	if err != nil {
		http.Error(w, "something has gone wrong which I haven't bothered to write a proper error message for", http.StatusInternalServerError)
		return
	}

	playHistory, err := db.GamePlayHistory(h.db, id)
	if err != nil {
		http.Error(w, "something has gone wrong which I haven't bothered to write a proper error message for", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "game", map[string]any{
		"Title":       game.Title + " — Game Detail",
		"Username":    middleware.UsernameFromContext(r),
		"Game":        game,
		"Totals":      totals,
		"PlayHistory": playHistory,
	})
}

func (h *Handler) DeleteGame(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := db.DeleteGame(h.db, id); err == sql.ErrNoRows {
		http.Error(w, "not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "cannot delete game (may have recorded results)", http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusOK)
}
