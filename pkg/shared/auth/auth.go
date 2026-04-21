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
			http.Error(w, "Authorization required", http.StatusUnauthorized)
			return
		}

		if receivedToken != token {
			logging.Global.Warnf("Auth failure from %s: token mismatch", r.RemoteAddr)
			http.Error(w, "Invalid or missing token", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}
