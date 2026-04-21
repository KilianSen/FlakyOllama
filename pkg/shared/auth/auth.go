package auth

import (
	"FlakyOllama/pkg/shared/logging"
	"net/http"
	"strings"
)

// Middleware checks for a Bearer token in the Authorization header.
func Middleware(token string, next http.HandlerFunc) http.HandlerFunc {
	token = strings.TrimSpace(token)
	return func(w http.ResponseWriter, r *http.Request) {
		// Always allow OPTIONS to pass through (CORS)
		if r.Method == "OPTIONS" {
			next(w, r)
			return
		}

		if token == "" {
			next(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		receivedToken := ""
		if authHeader != "" {
			parts := strings.Fields(authHeader)
			if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
				receivedToken = parts[1]
			}
		}

		if receivedToken == "" {
			receivedToken = r.URL.Query().Get("token")
		}

		if receivedToken == "" {
			logging.Global.Warnf("Auth failure: No token provided for %s %s", r.Method, r.URL.Path)
			http.Error(w, "Authorization required", http.StatusUnauthorized)
			return
		}

		if receivedToken != token {
			logging.Global.Warnf("Auth failure: Token mismatch for %s %s (received: %s...)", r.Method, r.URL.Path, receivedToken[:min(len(receivedToken), 5)])
			http.Error(w, "Invalid or missing token", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
