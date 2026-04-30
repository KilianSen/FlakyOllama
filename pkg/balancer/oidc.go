package balancer

import (
	"FlakyOllama/pkg/shared/auth"
	"FlakyOllama/pkg/shared/models"
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
)

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
						// Multi-window quota check with credit offset
						if usage, err := b.Storage.GetUserQuotaUsage(user.ID); err == nil {
							creditOffset := int64(usage.AgentCreditsEarned)
							effectiveOf := func(raw int64) int64 {
								if e := raw - creditOffset; e > 0 {
									return e
								}
								return 0
							}
							var quotaMsg string
							switch {
							case user.DailyQuotaLimit != -1 && effectiveOf(usage.DailyUsed) >= user.DailyQuotaLimit:
								quotaMsg = "Daily quota exceeded"
							case user.WeeklyQuotaLimit != -1 && effectiveOf(usage.WeeklyUsed) >= user.WeeklyQuotaLimit:
								quotaMsg = "Weekly quota exceeded"
							case user.MonthlyQuotaLimit != -1 && effectiveOf(usage.MonthlyUsed) >= user.MonthlyQuotaLimit:
								quotaMsg = "Monthly quota exceeded"
							case user.QuotaLimit != -1 && effectiveOf(user.QuotaUsed) >= user.QuotaLimit:
								quotaMsg = "Total quota exceeded"
							}
							if quotaMsg != "" {
								http.Error(w, quotaMsg, http.StatusForbidden)
								return
							}
						}
						// Ensure Admin status is synced from JWT if it changed
						if user.IsAdmin != claims.Admin {
							user.IsAdmin = claims.Admin
							_ = b.Storage.UpdateUser(user)
						}

						ctx := context.WithValue(r.Context(), auth.ContextKeyUser, user)
						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}
				}
			}
		}

		// 2. Fall back to Token Middleware (API Keys / Master Token)
		auth.Middleware([]string{b.Config.AuthToken, b.Config.RemoteToken}, b.Storage, next.ServeHTTP).ServeHTTP(w, r)
	})
}

func (b *Balancer) HandleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	if !b.Config.OIDC.Enabled {
		http.Error(w, "OIDC disabled", http.StatusForbidden)
		return
	}

	provider, err := oidc.NewProvider(r.Context(), b.Config.OIDC.Issuer)
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

	provider, _ := oidc.NewProvider(r.Context(), b.Config.OIDC.Issuer)
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

	sub := rawClaims["sub"].(string)
	email, _ := rawClaims["email"].(string)
	name, _ := rawClaims["name"].(string)

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
		b.Storage.CreateUser(user)
	} else {
		// Update existing user info
		user.Email = email
		user.Name = name
		user.IsAdmin = isAdmin
		_ = b.Storage.UpdateUser(user)
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
	rand.Read(b2)
	return base64.URLEncoding.EncodeToString(b2)
}
