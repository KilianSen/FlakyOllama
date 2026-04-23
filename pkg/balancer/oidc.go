package balancer

import (
	"FlakyOllama/pkg/shared/auth"
	"FlakyOllama/pkg/shared/models"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
)

var jwtKey = []byte("flakyollama-secret-key-change-me") // In production, this should be in config

type Claims struct {
	UserID  string `json:"user_id"`
	IsAdmin bool   `json:"is_admin"`
	jwt.RegisteredClaims
}

func (b *Balancer) initOIDC() (*oidc.Provider, oauth2.Config, error) {
	if !b.Config.OIDC.Enabled {
		return nil, oauth2.Config{}, nil
	}

	provider, err := oidc.NewProvider(context.Background(), b.Config.OIDC.Issuer)
	if err != nil {
		return nil, oauth2.Config{}, err
	}

	oauth2Config := oauth2.Config{
		ClientID:     b.Config.OIDC.ClientID,
		ClientSecret: b.Config.OIDC.ClientSecret,
		RedirectURL:  b.Config.OIDC.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	return provider, oauth2Config, nil
}

func (b *Balancer) HandleLogin(w http.ResponseWriter, r *http.Request) {
	_, oauth2Config, err := b.initOIDC()
	if err != nil {
		http.Error(w, "OIDC Provider Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	state := randString(16)
	setCookie(r, w, "oidc_state", state, 15*time.Minute)

	http.Redirect(w, r, oauth2Config.AuthCodeURL(state), http.StatusFound)
}

func (b *Balancer) HandleCallback(w http.ResponseWriter, r *http.Request) {
	state, err := r.Cookie("oidc_state")
	if err != nil || r.URL.Query().Get("state") != state.Value {
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}

	provider, oauth2Config, err := b.initOIDC()
	if err != nil {
		http.Error(w, "OIDC Provider Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	token, err := oauth2Config.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "Failed to exchange token", http.StatusInternalServerError)
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
		http.Error(w, "Failed to verify ID Token", http.StatusInternalServerError)
		return
	}

	var claims map[string]interface{}
	if err := idToken.Claims(&claims); err != nil {
		http.Error(w, "Failed to parse claims", http.StatusInternalServerError)
		return
	}

	sub := claims["sub"].(string)
	email, _ := claims["email"].(string)
	name, _ := claims["name"].(string)

	isAdmin := false
	if b.Config.OIDC.AdminClaim != "" {
		if val, ok := claims[b.Config.OIDC.AdminClaim]; ok {
			switch v := val.(type) {
			case string:
				isAdmin = v == b.Config.OIDC.AdminValue
			case []interface{}:
				for _, item := range v {
					if s, ok := item.(string); ok && s == b.Config.OIDC.AdminValue {
						isAdmin = true
						break
					}
				}
			}
		}
	}

	// Find or create user
	user, err := b.Storage.GetUserBySub(sub)
	if err != nil {
		// New user
		user = models.User{
			ID:      fmt.Sprintf("u_%d", time.Now().Unix()),
			Sub:     sub,
			Email:   email,
			Name:    name,
			IsAdmin: isAdmin,
		}
		b.Storage.CreateUser(user)

		// Create a personal client key for them
		personalKey := models.ClientKey{
			Key:        fmt.Sprintf("sk-%s", randString(32)),
			Label:      fmt.Sprintf("Personal Key for %s", user.Name),
			QuotaLimit: 1000000, // 1M tokens default
			QuotaUsed:  0,
			Credits:    10.0,
			Active:     true,
			UserID:     user.ID,
		}
		b.Storage.CreateClientKey(personalKey)
	} else {
		// Update user info
		user.Email = email
		user.Name = name
		user.IsAdmin = isAdmin
		b.Storage.UpdateUser(user)
	}

	// Create session JWT
	expirationTime := time.Now().Add(24 * time.Hour)
	jwtClaims := &Claims{
		UserID:  user.ID,
		IsAdmin: user.IsAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		},
	}

	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims)
	tokenString, err := jwtToken.SignedString(jwtKey)
	if err != nil {
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	setCookie(r, w, "session_token", tokenString, 24*time.Hour)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (b *Balancer) HandleLogout(w http.ResponseWriter, r *http.Request) {
	setCookie(r, w, "session_token", "", -1)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (b *Balancer) HandleV1Me(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(auth.ContextKeyUser).(models.User)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	key, err := b.Storage.GetClientKeyByUserID(user.ID)
	if err != nil {
		// If they don't have a key, create one (safety fallback)
		key = models.ClientKey{
			Key:        fmt.Sprintf("sk-%s", randString(32)),
			Label:      fmt.Sprintf("Personal Key for %s", user.Name),
			QuotaLimit: 1000000,
			QuotaUsed:  0,
			Credits:    10.0,
			Active:     true,
			UserID:     user.ID,
		}
		b.Storage.CreateClientKey(key)
	}

	resp := struct {
		User models.User      `json:"user"`
		Key  models.ClientKey `json:"key"`
	}{
		User: user,
		Key:  key,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (b *Balancer) SessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("session_token")
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		tknStr := c.Value
		claims := &Claims{}

		tkn, err := jwt.ParseWithClaims(tknStr, claims, func(token *jwt.Token) (interface{}, error) {
			return jwtKey, nil
		})

		if err != nil || !tkn.Valid {
			next.ServeHTTP(w, r)
			return
		}

		user, err := b.Storage.GetUserByID(claims.UserID)
		if err == nil {
			ctx := context.WithValue(r.Context(), auth.ContextKeyUser, user)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func randString(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

func setCookie(r *http.Request, w http.ResponseWriter, name, value string, duration time.Duration) {
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Expires:  time.Now().Add(duration),
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}
