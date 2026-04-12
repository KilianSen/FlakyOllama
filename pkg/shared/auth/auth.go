package auth

import (
	"log"
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
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		parts := strings.Fields(authHeader)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" || parts[1] != token {
			log.Printf("Auth failure from %s: invalid or missing token", r.RemoteAddr)
			http.Error(w, "Invalid or missing token", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}
