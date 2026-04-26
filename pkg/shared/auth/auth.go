package auth

import (
	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/models"
	"context"
	"net/http"
	"strings"
)

type contextKey string

const (
	ContextKeyToken      contextKey = "token"
	ContextKeyClientData contextKey = "client_data"
	ContextKeyUser       contextKey = "user"
)

type KeyManager interface {
	GetClientKey(key string) (models.ClientKey, error)
	GetAgentKey(key string) (models.AgentKey, error)
	GetUserByID(id string) (models.User, error)
}

// Middleware checks for a Bearer token in the Authorization header or query param.
// It prioritizes OIDC sessions from SessionMiddleware if present.
func Middleware(token string, km KeyManager, next http.HandlerFunc) http.HandlerFunc {
	token = strings.TrimSpace(token)
	return func(w http.ResponseWriter, r *http.Request) {
		// Always allow OPTIONS to pass through (CORS)
		if r.Method == "OPTIONS" {
			next(w, r)
			return
		}

		if token == "" && km == nil {
			next(w, r)
			return
		}

		// 0. Check if we have a user from SessionMiddleware (OIDC)
		if val := r.Context().Value(ContextKeyUser); val != nil {
			if u, ok := val.(models.User); ok {
				// Check User-global Quota
				if u.QuotaLimit != -1 && u.QuotaUsed >= u.QuotaLimit {
					http.Error(w, "Account-wide quota exceeded", http.StatusForbidden)
					return
				}
				next(w, r)
				return
			}
			logging.Global.Warnf("Auth: Context user has wrong type: %T for %s", val, r.URL.Path)
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
			logging.Global.Warnf("Auth failure: No session or token provided for %s %s", r.Method, r.URL.Path)
			http.Error(w, "Authorization required", http.StatusUnauthorized)
			return
		}

		// 1. Check against master token
		if token != "" && receivedToken == token {
			ctx := context.WithValue(r.Context(), ContextKeyToken, receivedToken)
			// Master admin gets max priority
			masterKey := models.ClientKey{Key: token, Label: "Master Admin", Credits: 999999999, QuotaLimit: -1, Active: true}
			ctx = context.WithValue(ctx, ContextKeyClientData, masterKey)
			next(w, r.WithContext(ctx))
			return
		}

		// 2. Check against KeyManager (database)
		if km != nil {
			ck, err := km.GetClientKey(receivedToken)
			if err == nil && ck.Active {
				// 1. Check Key-specific Sub-quota
				if ck.QuotaLimit != -1 && ck.QuotaUsed >= ck.QuotaLimit {
					http.Error(w, "API Key quota exceeded", http.StatusForbidden)
					return
				}

				// 2. Check User-global Quota
				if ck.UserID != "" {
					u, err := km.GetUserByID(ck.UserID)
					if err == nil {
						if u.QuotaLimit != -1 && u.QuotaUsed >= u.QuotaLimit {
							http.Error(w, "Account-wide quota exceeded", http.StatusForbidden)
							return
						}
					}
				}

				ctx := context.WithValue(r.Context(), ContextKeyToken, receivedToken)
				ctx = context.WithValue(ctx, ContextKeyClientData, ck)
				next(w, r.WithContext(ctx))
				return
			}

			// Check if it's an Agent Key
			ak, err := km.GetAgentKey(receivedToken)
			if err == nil && ak.Active {
				ctx := context.WithValue(r.Context(), ContextKeyToken, receivedToken)
				// Agents get priority based on their earnings
				agentAsClient := models.ClientKey{Key: ak.Key, Label: ak.Label, Credits: ak.CreditsEarned, QuotaLimit: -1, Active: true}
				ctx = context.WithValue(ctx, ContextKeyClientData, agentAsClient)
				next(w, r.WithContext(ctx))
				return
			}
		}

		logging.Global.Warnf("Auth failure: Invalid token for %s %s", r.Method, r.URL.Path)
		http.Error(w, "Invalid or missing token", http.StatusUnauthorized)
	}
}

func WithKeyManager(km KeyManager, next http.HandlerFunc) http.HandlerFunc {
	return Middleware("", km, next)
}

func GetTokenFromContext(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(ContextKeyToken).(string)
	return val, ok
}

func GetClientDataFromContext(ctx context.Context) (models.ClientKey, bool) {
	val, ok := ctx.Value(ContextKeyClientData).(models.ClientKey)
	return val, ok
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
