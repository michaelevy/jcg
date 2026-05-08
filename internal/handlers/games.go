package handlers

import (
	"net/http"

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
