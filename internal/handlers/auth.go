package handlers

import (
	"net/http"

	"golang.org/x/crypto/bcrypt"
	"jcg/internal/middleware"
)

func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, "login", map[string]any{
		"Title": "Login",
	})
}

func (h *Handler) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	var hash string
	err := h.db.QueryRow(`SELECT password_hash FROM users WHERE username = ?`, username).Scan(&hash)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		h.render(w, "login", map[string]any{
			"Title": "Login",
			"Error": "Invalid username or password.",
		})
		return
	}

	middleware.CreateSession(w, username)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	middleware.DeleteSession(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
