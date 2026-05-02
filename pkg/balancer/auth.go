package balancer

import (
	"FlakyOllama/pkg/balancer/models"
	"FlakyOllama/pkg/shared/auth"
	"FlakyOllama/pkg/shared/logging"
	models2 "FlakyOllama/pkg/shared/models"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
)

// formatDuration renders a duration as a human-readable approximate string,
// e.g. "~45m", "~3h 12m", "~2d 4h".
func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "soon"
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("~%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("~%dh %dm", hours, mins)
	default:
		return fmt.Sprintf("~%dm", mins+1)
	}
}

// quotaResetHint returns a short "resets in ~X" or "resets on DATE" string for
// the given quota window. Called only when a quota limit has been exceeded.
func (b *Balancer) quotaResetHint(userID, window string) string {
	now := time.Now().UTC()
	switch window {
	case "daily":
		oldest := b.Storage.GetWindowOldestTimestamp(userID, "daily")
		if oldest.IsZero() {
			return "resets within 24h"
		}
		return "resets in " + formatDuration(time.Until(oldest.Add(24*time.Hour)))
	case "weekly":
		oldest := b.Storage.GetWindowOldestTimestamp(userID, "weekly")
		if oldest.IsZero() {
			return "resets within 7 days"
		}
		return "resets in " + formatDuration(time.Until(oldest.Add(7*24*time.Hour)))
	case "monthly":
		nextMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
		return "resets on " + nextMonth.Format("Jan 2")
	default:
		return ""
	}
}

type Claims struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Admin bool   `json:"admin"`
	jwt.RegisteredClaims
}

func (b *Balancer) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Check Session Cookie (Signed JWT)
		cookie, err := r.Cookie("flaky_session")
		if err == nil {
			token, err := jwt.ParseWithClaims(cookie.Value, &Claims{}, func(t *jwt.Token) (interface{}, error) {
				return []byte(b.Config.JWTSecret), nil
			})

			if err == nil && token.Valid {
				if claims, ok := token.Claims.(*Claims); ok {
					user, err := b.Storage.GetUserBySub(claims.Sub)
					if err == nil {
						// Ensure Admin status is synced from JWT if it changed
						if user.IsAdmin != claims.Admin {
							user.IsAdmin = claims.Admin
							if updateErr := b.Storage.UpdateUser(user); updateErr != nil {
								logging.Global.Errorf("Failed to update user admin status: %v", updateErr)
							}
						}

						ctx := context.WithValue(r.Context(), auth.ContextKeyUser, user)
						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}
				}
			}
		}

		// 2. Fall back to Token Middleware (API Keys / Master Token)
		auth.BearerAuthMiddleware([]string{b.Config.AuthToken, b.Config.RemoteToken}, b.Storage, next.ServeHTTP).ServeHTTP(w, r)
	})
}

func (b *Balancer) getOIDCProvider(ctx context.Context) (*oidc.Provider, error) {
	b.oidcMu.Lock()
	defer b.oidcMu.Unlock()

	if b.oidcProvider != nil {
		return b.oidcProvider, nil
	}

	provider, err := oidc.NewProvider(ctx, b.Config.OIDC.Issuer)
	if err != nil {
		return nil, err
	}

	b.oidcProvider = provider
	return provider, nil
}

func (b *Balancer) HandleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	if !b.Config.OIDC.Enabled {
		http.Error(w, "OIDC disabled", http.StatusForbidden)
		return
	}

	provider, err := b.getOIDCProvider(r.Context())
	if err != nil {
		http.Error(w, "Failed to get provider: "+err.Error(), http.StatusInternalServerError)
		return
	}

	oauth2Config := oauth2.Config{
		ClientID:     b.Config.OIDC.ClientID,
		ClientSecret: b.Config.OIDC.ClientSecret,
		RedirectURL:  b.Config.OIDC.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	state := b.generateState()
	http.SetCookie(w, &http.Cookie{
		Name:     "oidc_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		MaxAge:   300,
	})

	http.Redirect(w, r, oauth2Config.AuthCodeURL(state), http.StatusFound)
}

func (b *Balancer) HandleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie("oidc_state")
	if err != nil || r.URL.Query().Get("state") != stateCookie.Value {
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}

	provider, err := b.getOIDCProvider(r.Context())
	if err != nil {
		http.Error(w, "Failed to get provider: "+err.Error(), http.StatusInternalServerError)
		return
	}

	oauth2Config := oauth2.Config{
		ClientID:     b.Config.OIDC.ClientID,
		ClientSecret: b.Config.OIDC.ClientSecret,
		RedirectURL:  b.Config.OIDC.RedirectURL,
		Endpoint:     provider.Endpoint(),
	}

	token, err := oauth2Config.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "Failed to exchange token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "No id_token", http.StatusInternalServerError)
		return
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: b.Config.OIDC.ClientID})
	idToken, err := verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		http.Error(w, "Failed to verify ID token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var rawClaims map[string]interface{}
	if err := idToken.Claims(&rawClaims); err != nil {
		http.Error(w, "Failed to parse claims: "+err.Error(), http.StatusInternalServerError)
		return
	}

	sub, ok := rawClaims["sub"].(string)
	if !ok {
		http.Error(w, "Missing or invalid sub claim", http.StatusInternalServerError)
		return
	}

	var email, name string
	if e, ok := rawClaims["email"].(string); ok {
		email = e
	}
	if n, ok := rawClaims["name"].(string); ok {
		name = n
	}

	// Check Admin Status
	isAdmin := false
	if b.Config.OIDC.AdminClaim != "" {
		if val, ok := rawClaims[b.Config.OIDC.AdminClaim]; ok {
			if str, ok := val.(string); ok && str == b.Config.OIDC.AdminValue {
				isAdmin = true
			} else if slice, ok := val.([]interface{}); ok {
				for _, v := range slice {
					if s, ok := v.(string); ok && s == b.Config.OIDC.AdminValue {
						isAdmin = true
						break
					}
				}
			}
		}
	}

	// Sync User
	user, err := b.Storage.GetUserBySub(sub)
	if err != nil {
		user = models.User{
			ID:                "u_" + b.computeHash(sub)[:12],
			Sub:               sub,
			Email:             email,
			Name:              name,
			IsAdmin:           isAdmin,
			QuotaTier:         models.QuotaTierFree,
			QuotaLimit:        models.DefaultTiers[models.QuotaTierFree].Total,
			DailyQuotaLimit:   models.DefaultTiers[models.QuotaTierFree].Daily,
			WeeklyQuotaLimit:  models.DefaultTiers[models.QuotaTierFree].Weekly,
			MonthlyQuotaLimit: models.DefaultTiers[models.QuotaTierFree].Monthly,
		}
		err := b.Storage.CreateUser(user)
		if err != nil {
			logging.Global.Errorf("Failed to create user on login: %v", err)
			http.Error(w, "Failed to create user", http.StatusInternalServerError)
			return
		}
	} else {
		// Update existing user info
		user.Email = email
		user.Name = name
		user.IsAdmin = isAdmin
		if updateErr := b.Storage.UpdateUser(user); updateErr != nil {
			logging.Global.Errorf("Failed to update user on login: %v", updateErr)
		}
	}

	// Issue Signed Session Token
	sessionToken := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		Sub:   user.Sub,
		Email: user.Email,
		Name:  user.Name,
		Admin: user.IsAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})

	tokenString, err := sessionToken.SignedString([]byte(b.Config.JWTSecret))
	if err != nil {
		http.Error(w, "Failed to sign session", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "flaky_session",
		Value:    tokenString,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		MaxAge:   86400 * 7,
	})

	http.Redirect(w, r, "/", http.StatusFound)
}

func (b *Balancer) HandleOIDCLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "flaky_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (b *Balancer) generateState() string {
	b2 := make([]byte, 16)
	_, err := rand.Read(b2)
	if err != nil {
		panic(err)
	}
	return base64.URLEncoding.EncodeToString(b2)
}

type contextKey string

const ContextKeyForceOwnNode contextKey = "force_own_node"

// InferenceQuotaMiddleware enforces per-key and per-user quotas for inference
// endpoints only. Agent credits earned by the user are subtracted from their
// effective usage, so compute contributors can use their own supplied capacity
// without hitting the quota.
//
// If a user's route_preference is "quality_fallback" and their quota is exhausted,
// a ForceOwnNode flag is set in the context instead of returning 429 — requests
// then fall back to that user's own agent nodes for free.
func (b *Balancer) InferenceQuotaMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Client key sub-quota
		if ck, ok := auth.GetClientDataFromContext(r.Context()); ok {
			if ck.QuotaLimit != -1 && ck.QuotaUsed >= ck.QuotaLimit {
				b.respondError(w, r, http.StatusTooManyRequests, "API key quota exceeded")
				return
			}
		}

		// 2. User global quota, offset by quota earned via contributed compute
		if val := r.Context().Value(auth.ContextKeyUser); val != nil {
			if u, ok := val.(models.User); ok && u.ID != "" {
				usage, err := b.Storage.GetUserQuotaUsage(u.ID)
				if err != nil {
					logging.Global.Errorf("InferenceQuotaMiddleware: failed to get quota usage for user %s: %v", u.ID, err)
					// fail open — don't block requests on a storage error
				} else {
					offset := int64(usage.AgentCreditsEarned)
					exceeded := false
					var reason, window string
					switch {
					case u.DailyQuotaLimit != -1 && max(0, usage.DailyUsed-offset) >= u.DailyQuotaLimit:
						exceeded, reason, window = true, "daily quota exceeded", "daily"
					case u.WeeklyQuotaLimit != -1 && max(0, usage.WeeklyUsed-offset) >= u.WeeklyQuotaLimit:
						exceeded, reason, window = true, "weekly quota exceeded", "weekly"
					case u.MonthlyQuotaLimit != -1 && max(0, usage.MonthlyUsed-offset) >= u.MonthlyQuotaLimit:
						exceeded, reason, window = true, "monthly quota exceeded", "monthly"
					case u.QuotaLimit != -1 && max(0, u.QuotaUsed-offset) >= u.QuotaLimit:
						exceeded, reason, window = true, "account quota exceeded", ""
					}
					if exceeded {
						if u.RoutePreference == "quality_fallback" {
							hasOwnNode := false
							b.State.View(func(s ClusterState) {
								for _, node := range s.Agents {
									if node.UserID == u.ID && node.State != models2.StateBroken && !node.Draining {
										hasOwnNode = true
										break
									}
								}
							})
							if hasOwnNode {
								ctx := context.WithValue(r.Context(), ContextKeyForceOwnNode, true)
								next.ServeHTTP(w, r.WithContext(ctx))
								return
							}
						}
						if hint := b.quotaResetHint(u.ID, window); hint != "" {
							reason = reason + " (" + hint + ")"
						}
						b.respondError(w, r, http.StatusTooManyRequests, reason)
						return
					}
				}
			}
		}

		next.ServeHTTP(w, r)
	})
}

func (b *Balancer) AdminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if val := r.Context().Value(auth.ContextKeyUser); val != nil {
			if user, ok := val.(models.User); ok {
				if user.IsAdmin {
					next.ServeHTTP(w, r)
					return
				}
				logging.Global.Warnf("AdminOnly: Access denied for user %s (Not an Admin)", user.Email)
			}
		}
		http.Error(w, "Administrator privileges required", http.StatusForbidden)
	})
}
