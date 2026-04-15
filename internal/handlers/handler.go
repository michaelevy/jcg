package handlers

import (
	"database/sql"
	"html/template"
	"log"
	"net/http"

	"jcg/internal/middleware"
)

// Handler holds shared dependencies for all HTTP handlers.
type Handler struct {
	db   *sql.DB
	tmpl *template.Template
}

func New(db *sql.DB, tmpl *template.Template) *Handler {
	return &Handler{db: db, tmpl: tmpl}
}

func (h *Handler) render(w http.ResponseWriter, name string, data any) {
	if err := h.tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template %q: %v", name, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// Home is a placeholder until the leaderboard is built in Phase 4.
func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
	h.render(w, "home", map[string]any{
		"Title":    "Home",
		"Username": middleware.UsernameFromContext(r),
	})
}
