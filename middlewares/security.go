package middlewares

import (
	"log/slog"
	"net/http"
)

// AuthMiddleware is a middleware for authentication verification.
// It checks for the presence of an "Authorization" header in the incoming HTTP request.
// If the header is missing, it responds with a 401 Unauthorized status.
//
// Parameters:
// - next: The next HTTP handler to be called if the request is authorized.
//
// Returns:
// - http.Handler: The HTTP handler that performs the authentication check.
func AuthMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Here you could verify an authentication token, for example
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
