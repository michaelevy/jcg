package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"jcg/internal/db"
	"jcg/internal/middleware"
)

func (h *Handler) PlayerProfile(w http.ResponseWriter, r *http.Request) {
	raw := r.PathValue("id")
	playerID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || playerID <= 0 {
		http.Error(w, "invalid player id", http.StatusBadRequest)
		return
	}

	player, err := db.GetPlayer(h.db, playerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "player not found", http.StatusNotFound)
			return
		}
		http.Error(w, "something has gone wrong which I haven't bothered to write a proper error message for", http.StatusInternalServerError)
		return
	}

	seasonStats, err := db.PlayerSeasonStats(h.db, playerID)
	if err != nil {
		http.Error(w, "something has gone wrong which I haven't bothered to write a proper error message for", http.StatusInternalServerError)
		return
	}

	gameHistory, err := db.PlayerGameHistory(h.db, playerID)
	if err != nil {
		http.Error(w, "something has gone wrong which I haven't bothered to write a proper error message for", http.StatusInternalServerError)
		return
	}

	data := map[string]any{
		"Title":       player.Name + " — Profile",
		"Username":    middleware.UsernameFromContext(r),
		"Player":      player,
		"SeasonStats": seasonStats,
		"GameHistory": gameHistory,
	}
	h.render(w, "player", data)
}
