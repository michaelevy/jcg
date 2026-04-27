package handlers

import (
	"net/http"

	"golang.org/x/crypto/bcrypt"
	"jcg/internal/middleware"
)

// Dummy hash to equalize timing when user doesn't exist.
// This is a well-formed bcrypt hash that will never match any real password.
const dummyHash = "$2a$10$yLcxRVJO5Cl5rBE5W1yE.eQj3rVFZ1P9VrBv0lDNM.FjRGJ/HKnhi"

func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	token := middleware.CreatePreSessionToken(w)
	h.render(w, r, "login", map[string]any{
		"Title":     "Login",
		"CSRFToken": token,
	})
}

func (h *Handler) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	if !middleware.ValidateAndConsumePreSession(r) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	var hash string
	err := h.db.QueryRow(`SELECT password_hash FROM users WHERE username = ?`, username).Scan(&hash)
	if err != nil {
		// User not found: use dummy hash to equalize timing with bcrypt.CompareHashAndPassword
		hash = dummyHash
	}

	// Always perform bcrypt comparison regardless of whether user was found.
	// This ensures the timing is constant regardless of user existence.
	pwErr := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))

	// Reject if user doesn't exist (err != nil) OR password doesn't match
	if err != nil || pwErr != nil {
		token := middleware.CreatePreSessionToken(w)
		h.render(w, r, "login", map[string]any{
			"Title":     "Login",
			"Error":     "Invalid username or password.",
			"CSRFToken": token,
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
