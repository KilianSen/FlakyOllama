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

var jwtKey = []byte("flakyollama-secret-key-change-me")

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
	logging.Global.Infof("OIDC: Login initiated from %s", r.RemoteAddr)
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

	sub := claims["sub"].(string)
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
			ID:      fmt.Sprintf("u_%d", time.Now().Unix()),
			Sub:     sub,
			Email:   email,
			Name:    name,
			IsAdmin: isAdmin,
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
			QuotaLimit: 1000000,
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
	tokenString, err := jwtToken.SignedString(jwtKey)
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
	var clientKey models.ClientKey
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
				clientKey = models.ClientKey{Key: tkn, Label: "Master Token", Credits: 999999, QuotaLimit: -1, Active: true}
			} else if ck, ok := r.Context().Value(auth.ContextKeyClientData).(models.ClientKey); ok {
				user = models.User{ID: "token-user", Name: ck.Label, Email: "client@token", IsAdmin: false}
				clientKey = ck
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
		ck, err := b.Storage.GetClientKeyByUserID(user.ID)
		if err != nil {
			ck = models.ClientKey{
				Key:        fmt.Sprintf("sk-%s", randString(32)),
				Label:      fmt.Sprintf("Personal Key for %s", user.Name),
				QuotaLimit: 1000000,
				QuotaUsed:  0,
				Credits:    10.0,
				Active:     true,
				UserID:     user.ID,
			}
			b.Storage.CreateClientKey(ck)
		}
		clientKey = ck

		ak, err := b.Storage.GetAgentKeysByUserID(user.ID)
		if err != nil || len(ak) == 0 {
			newAgentKey := models.AgentKey{
				Key:           fmt.Sprintf("ak-%s", randString(32)),
				Label:         fmt.Sprintf("Default Agent for %s", user.Name),
				CreditsEarned: 0,
				Reputation:    1.0,
				Active:        true,
				UserID:        user.ID,
			}
			b.Storage.CreateAgentKey(newAgentKey)
			agentKeys = []models.AgentKey{newAgentKey}
		} else {
			agentKeys = ak
		}
	}

	resp := struct {
		User      models.User       `json:"user"`
		ClientKey models.ClientKey  `json:"client_key"`
		AgentKeys []models.AgentKey `json:"agent_keys"`
	}{
		User:      user,
		ClientKey: clientKey,
		AgentKeys: agentKeys,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (b *Balancer) SessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("session_token")
		if err != nil {
			if err != http.ErrNoCookie {
				logging.Global.Errorf("SessionMiddleware: Error reading cookie: %v", err)
			}
			next.ServeHTTP(w, r)
			return
		}

		tknStr := c.Value
		claims := &Claims{}

		tkn, err := jwt.ParseWithClaims(tknStr, claims, func(token *jwt.Token) (interface{}, error) {
			return jwtKey, nil
		})

		if err != nil || !tkn.Valid {
			logging.Global.Errorf("SessionMiddleware: Invalid token: %v. Clearing cookie.", err)
			setCookie(r, w, "session_token", "", -1)
			next.ServeHTTP(w, r)
			return
		}

		user, err := b.Storage.GetUserByID(claims.UserID)
		if err != nil {
			logging.Global.Errorf("SessionMiddleware: User %s not found in DB", claims.UserID)
			next.ServeHTTP(w, r)
			return
		}

		logging.Global.Infof("Session: Authenticated user %s (%s)", user.Name, user.ID)
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
