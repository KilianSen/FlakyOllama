package balancer

import (
	"FlakyOllama/pkg/shared/auth"
	"FlakyOllama/pkg/shared/models"
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

func (b *Balancer) AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Check Session Cookie (OIDC)
		cookie, err := r.Cookie("flaky_session")
		if err == nil {
			userSub := cookie.Value
			user, err := b.Storage.GetUserBySub(userSub)
			if err == nil {
				// Quota Check for OIDC user
				if user.QuotaLimit != -1 && user.QuotaUsed >= user.QuotaLimit {
					http.Error(w, "Account-wide quota exceeded", http.StatusForbidden)
					return
				}
				ctx := context.WithValue(r.Context(), auth.ContextKeyUser, user)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// 2. Fall back to Token Middleware (API Keys)
		auth.Middleware(b.Config.AuthToken, b.Storage, next).ServeHTTP(w, r)
	}
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
		Secure:   false, // Set to true if using HTTPS
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

	var claims struct {
		Email string `json:"email"`
		Sub   string `json:"sub"`
		Name  string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		http.Error(w, "Failed to parse claims: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Sync User
	user, err := b.Storage.GetUserBySub(claims.Sub)
	if err != nil {
		user = models.User{
			ID:         "u_" + b.computeHash(claims.Sub)[:12],
			Sub:        claims.Sub,
			Email:      claims.Email,
			Name:       claims.Name,
			QuotaLimit: 1000000, // Default 1M tokens
		}
		b.Storage.CreateUser(user)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "flaky_session",
		Value:    user.Sub,
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		MaxAge:   86400 * 7,
	})

	http.Redirect(w, r, "/", http.StatusFound)
}

func (b *Balancer) generateState() string {
	b2 := make([]byte, 16)
	rand.Read(b2)
	return base64.URLEncoding.EncodeToString(b2)
}
