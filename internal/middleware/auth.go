package middleware

import "net/http"

// UsernameFromContext returns the authenticated username from the request context.
// Stub — real implementation added in Phase 2.
func UsernameFromContext(_ *http.Request) string {
	return ""
}
