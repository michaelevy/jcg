package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"jcg/internal/db"
	"jcg/internal/middleware"
)

func (h *Handler) GameResultDetail(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	playHistory, err := db.GamePlayHistory(h.db, detail.GameID)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	data := map[string]any{
		"Title":       detail.GameTitle + " — Game Detail",
		"Username":    middleware.UsernameFromContext(r),
		"Detail":      detail,
		"PlayHistory": playHistory,
	}
	h.render(w, "game_result", data)
}
