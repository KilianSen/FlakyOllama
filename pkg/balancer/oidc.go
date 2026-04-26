package balancer

import (
	"FlakyOllama/pkg/shared/auth"
	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/models"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
)

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
	if !b.Config.OIDC.Enabled {
		logging.Global.Warnf("OIDC: Login attempt while OIDC is disabled")
		http.Error(w, "OIDC is disabled in cluster configuration", http.StatusForbidden)
		return
	}
	logging.Global.Infof("OIDC: Login initiated from %s", r.RemoteAddr)
	provider, oauth2Config, err := b.initOIDC()
	if err != nil || provider == nil {
		http.Error(w, "OIDC Provider Error", http.StatusInternalServerError)
		return
	}

	state := randString(16)
	setCookie(r, w, "oidc_state", state, 15*time.Minute)

	http.Redirect(w, r, oauth2Config.AuthCodeURL(state), http.StatusFound)
}

func (b *Balancer) HandleCallback(w http.ResponseWriter, r *http.Request) {
	logging.Global.Infof("OIDC: Callback received")
	state, err := r.Cookie("oidc_state")
	if err != nil {
		logging.Global.Errorf("OIDC: State cookie missing")
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}
	if r.URL.Query().Get("state") != state.Value {
		logging.Global.Errorf("OIDC: State mismatch. Expected %s, got %s", state.Value, r.URL.Query().Get("state"))
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}

	provider, oauth2Config, err := b.initOIDC()
	if err != nil {
		logging.Global.Errorf("OIDC: Provider init failed: %v", err)
		http.Error(w, "OIDC Provider Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	token, err := oauth2Config.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		logging.Global.Errorf("OIDC: Token exchange failed: %v", err)
		http.Error(w, "Failed to exchange token", http.StatusInternalServerError)
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		logging.Global.Errorf("OIDC: No id_token in exchange response")
		http.Error(w, "No id_token", http.StatusInternalServerError)
		return
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: b.Config.OIDC.ClientID})
	idToken, err := verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		logging.Global.Errorf("OIDC: ID Token verification failed: %v", err)
		http.Error(w, "Failed to verify ID Token", http.StatusInternalServerError)
		return
	}

	var claims map[string]interface{}
	if err := idToken.Claims(&claims); err != nil {
		logging.Global.Errorf("OIDC: Failed to parse claims: %v", err)
		http.Error(w, "Failed to parse claims", http.StatusInternalServerError)
		return
	}

	sub, ok := claims["sub"].(string)
	if !ok || sub == "" {
		logging.Global.Errorf("OIDC: Missing 'sub' claim")
		http.Error(w, "Invalid token claims", http.StatusBadRequest)
		return
	}
	email, _ := claims["email"].(string)
	name, _ := claims["name"].(string)

	isAdmin := false
	if b.Config.OIDC.AdminClaim != "" {
		if val, ok := claims[b.Config.OIDC.AdminClaim]; ok {
			target := strings.ToLower(b.Config.OIDC.AdminValue)
			switch v := val.(type) {
			case string:
				isAdmin = strings.ToLower(v) == target
			case []interface{}:
				for _, item := range v {
					if s, ok := item.(string); ok && strings.ToLower(s) == target {
						isAdmin = true
						break
					}
				}
			}
		}
	}
	logging.Global.Infof("OIDC: User %s (admin: %v)", name, isAdmin)

	// Find or create user
	user, err := b.Storage.GetUserBySub(sub)
	if err != nil {
		// New user
		user = models.User{
			ID:         fmt.Sprintf("u_%d", time.Now().Unix()),
			Sub:        sub,
			Email:      email,
			Name:       name,
			IsAdmin:    isAdmin,
			QuotaLimit: 10000000, // 10M default global
			QuotaUsed:  0,
		}
		if err := b.Storage.CreateUser(user); err != nil {
			logging.Global.Errorf("OIDC: Failed to create user in DB: %v", err)
			http.Error(w, "Failed to register user", http.StatusInternalServerError)
			return
		}

		// Create a personal client key for them
		personalKey := models.ClientKey{
			Key:        fmt.Sprintf("sk-%s", randString(32)),
			Label:      fmt.Sprintf("Personal Key for %s", user.Name),
			QuotaLimit: 1000000, // 1M tokens sub-quota default
			QuotaUsed:  0,
			Credits:    10.0,
			Active:     true,
			UserID:     user.ID,
		}
		if err := b.Storage.CreateClientKey(personalKey); err != nil {
			logging.Global.Errorf("OIDC: Failed to create client key for user: %v", err)
		}
	} else {
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
	tokenString, err := jwtToken.SignedString([]byte(b.Config.JWTSecret))
	if err != nil {
		logging.Global.Errorf("OIDC: Failed to sign JWT: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	logging.Global.Infof("OIDC: Setting session_token cookie for user %s", user.ID)
	setCookie(r, w, "session_token", tokenString, 24*time.Hour)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (b *Balancer) HandleLogout(w http.ResponseWriter, r *http.Request) {
	logging.Global.Infof("OIDC: Logging out")
	setCookie(r, w, "session_token", "", -1)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (b *Balancer) HandleV1Me(w http.ResponseWriter, r *http.Request) {
	var user models.User
	var clientKeys []models.ClientKey
	var agentKeys []models.AgentKey

	val := r.Context().Value(auth.ContextKeyUser)
	if u, ok := val.(models.User); ok {
		user = u
	} else if u, ok := val.(*models.User); ok {
		user = *u
	} else {
		// Try token-based auth fallback
		if tkn, ok := r.Context().Value(auth.ContextKeyToken).(string); ok {
			if tkn == b.Config.AuthToken {
				user = models.User{ID: "master", Name: "Master Admin", Email: "admin@local", IsAdmin: true}
				clientKeys = []models.ClientKey{{Key: tkn, Label: "Master Token", Credits: 999999, QuotaLimit: -1, Active: true}}
			} else if ck, ok := r.Context().Value(auth.ContextKeyClientData).(models.ClientKey); ok {
				user = models.User{ID: "token-user", Name: ck.Label, Email: "client@token", IsAdmin: false}
				clientKeys = []models.ClientKey{ck}
			}
		}
	}

	if user.ID == "" {
		logging.Global.Warnf("HandleV1Me: User context missing or wrong type. Got: %T", val)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// If we still need to fetch details for OIDC user
	if user.ID != "master" && user.ID != "token-user" {
		// Fetch fresh user to get latest quotas
		u, err := b.Storage.GetUserByID(user.ID)
		if err == nil {
			user = u
		}

		cks, err := b.Storage.GetClientKeysByUserID(user.ID)
		if err != nil || len(cks) == 0 {
			defaultKey := models.ClientKey{
				Key:        fmt.Sprintf("sk-%s", randString(32)),
				Label:      fmt.Sprintf("Personal Key for %s", user.Name),
				QuotaLimit: 1000000,
				QuotaUsed:  0,
				Credits:    10.0,
				Active:     true,
				UserID:     user.ID,
			}
			b.Storage.CreateClientKey(defaultKey)
			clientKeys = []models.ClientKey{defaultKey}
		} else {
			clientKeys = cks
		}

		ak, err := b.Storage.GetAgentKeysByUserID(user.ID)
		if err == nil {
			agentKeys = ak
		}
	}

	resp := struct {
		User       models.User       `json:"user"`
		ClientKeys []models.ClientKey `json:"client_keys"`
		AgentKeys  []models.AgentKey `json:"agent_keys"`
	}{
		User:       user,
		ClientKeys: clientKeys,
		AgentKeys:  agentKeys,
	}

	b.jsonResponse(w, http.StatusOK, resp)
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
			return []byte(b.Config.JWTSecret), nil
		})

		if err != nil || !tkn.Valid {
			setCookie(r, w, "session_token", "", -1)
			next.ServeHTTP(w, r)
			return
		}

		user, err := b.Storage.GetUserByID(claims.UserID)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		ctx := context.WithValue(r.Context(), auth.ContextKeyUser, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func randString(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func setCookie(r *http.Request, w http.ResponseWriter, name, value string, duration time.Duration) {
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}

	isLocal := strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1")
	secure := (r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https") && !isLocal

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
