package handlers

import (
	"database/sql"
	"html/template"
	"log"
	"net/http"
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
