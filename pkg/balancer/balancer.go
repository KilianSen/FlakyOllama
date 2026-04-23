package balancer

import (
	"FlakyOllama/pkg/balancer/jobs"
	"FlakyOllama/pkg/balancer/state"
	"FlakyOllama/pkg/balancer/storage"
	"FlakyOllama/pkg/shared/auth"
	"FlakyOllama/pkg/shared/config"
	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/models"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

type Balancer struct {
	Address string
	Config  *config.Config
	State   *state.Actor
	Storage *storage.SQLiteStorage
	Jobs    *jobs.Manager

	// Performance cache: model -> []PerformanceMetric
	perfMu    sync.RWMutex
	PerfCache map[string]*models.PerformanceStats

	// Client affinity: IP -> NodeID
	affinityMu     sync.RWMutex
	ClientAffinity map[string]string

	// Log shipping
	LogCh chan models.LogEntry
}

func NewBalancer(addr, dbPath string, cfg *config.Config) (*Balancer, error) {
	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		return nil, err
	}

	b := &Balancer{
		Address:        addr,
		Config:         cfg,
		State:          state.NewActor(),
		Storage:        s,
		Jobs:           jobs.NewManager(),
		PerfCache:      make(map[string]*models.PerformanceStats),
		ClientAffinity: make(map[string]string),
		LogCh:          make(chan models.LogEntry, 1000),
	}

	return b, nil
}

func (b *Balancer) CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, HEAD")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		// Browsers supporting wildcard headers will use it, others get the explicit list
		reqHeaders := r.Header.Get("Access-Control-Request-Headers")
		if reqHeaders != "" {
			w.Header().Set("Access-Control-Allow-Headers", reqHeaders)
		} else {
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
		}

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (b *Balancer) Serve() error {
	logging.Global.Infof("Balancer listening on %s (TLS: %v)", b.Address, b.Config.TLS.Enabled)
	if b.Config.TLS.Enabled {
		return http.ListenAndServeTLS(b.Address, b.Config.TLS.CertFile, b.Config.TLS.KeyFile, b.NewMux())
	}
	return http.ListenAndServe(b.Address, b.NewMux())
}

// NewMux returns a mux with the balancer's handlers registered.
func (b *Balancer) NewMux() *chi.Mux {
	token := b.Config.AuthToken
	remoteToken := b.Config.RemoteToken
	r := chi.NewRouter()

	r.Use(b.CORS)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logging.Global.Infof("%s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
			next.ServeHTTP(w, r)
		})
	})
	r.Use(b.SessionMiddleware)

	// OIDC Auth
	r.Get("/auth/login", b.HandleLogin)
	r.Get("/auth/callback", b.HandleCallback)
	r.Get("/auth/logout", b.HandleLogout)

	// Base
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/metrics", b.HandleMetrics)

	// Legacy Ollama Layer
	r.Group(func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return auth.Middleware(token, b.Storage, next.ServeHTTP)
		})
		r.Post("/api/generate", b.HandleGenerate)
		r.Post("/api/chat", b.HandleChat)
		r.Post("/api/show", b.HandleShow)
		r.Get("/api/tags", b.HandleTags)
		r.Post("/api/embeddings", b.HandleEmbed)
		r.Get("/api/version", b.HandleVersion)
		r.Get("/api/ps", b.HandlePS)
		r.Post("/api/pull", b.HandlePull)
		r.Post("/api/delete", b.HandleDelete)
		r.Post("/api/create", b.HandleCreate)
		r.Post("/api/copy", b.HandleCopy)
		r.Post("/api/push", b.HandlePush)
	})

	// OpenAI Layer
	r.Group(func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return auth.Middleware(token, b.Storage, next.ServeHTTP)
		})
		r.Post("/v1/chat/completions", b.HandleOpenAIChat)
		r.Post("/v1/completions", b.HandleOpenAICompletions)
		r.Get("/v1/models", b.HandleOpenAIModels)
		r.Post("/v1/embeddings", b.HandleOpenAIEmbeddings)
	})

	// Management Layer (Legacy compatibility)
	r.Post("/register", auth.Middleware(remoteToken, b.Storage, b.HandleV1Register))
	r.Post("/api/log/collect", auth.Middleware(remoteToken, b.Storage, b.HandleV1LogCollect))
	r.Get("/api/status", auth.Middleware(token, b.Storage, b.HandleV1ClusterStatus))
	r.Get("/api/logs", auth.Middleware(token, b.Storage, b.HandleV1Logs))

	// Management API (V1 Structured)
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return auth.Middleware(token, b.Storage, next.ServeHTTP)
		})

		r.Get("/catalog", b.HandleV1Catalog)
		r.Get("/me", b.HandleV1Me)

		// Gated Admin Routes
		r.Group(func(r chi.Router) {
			r.Use(b.AdminOnly)

			r.Get("/status", b.HandleV1ClusterStatus)
			r.Get("/logs", b.HandleV1Logs)

			r.Route("/nodes", func(r chi.Router) {
				r.Get("/", b.HandleV1Nodes)
				r.Post("/{id}/drain", b.HandleV1NodeDrain)
				r.Post("/{id}/undrain", b.HandleV1NodeUndrain)
			})

			r.Route("/models", func(r chi.Router) {
				r.Post("/pull", b.HandleV1ModelPull)
				r.Delete("/{name}", b.HandleV1ModelDelete)
				r.Post("/{name}/unload", b.HandleV1ModelUnload)
			})

			r.Route("/requests", func(r chi.Router) {
				r.Get("/", b.HandleV1ModelRequestsList)
				r.Post("/{id}/approve", b.HandleV1ModelRequestApprove)
				r.Post("/{id}/decline", b.HandleV1ModelRequestDecline)
			})

			r.Post("/policies", b.HandleV1ModelPolicySet)

			r.Route("/keys", func(r chi.Router) {
				r.Route("/clients", func(r chi.Router) {
					r.Get("/", b.HandleV1ClientKeysList)
					r.Post("/", b.HandleV1ClientKeyCreate)
				})
				r.Route("/agents", func(r chi.Router) {
					r.Get("/", b.HandleV1AgentKeysList)
					r.Post("/", b.HandleV1AgentKeyCreate)
				})
			})

			r.Get("/jobs/{id}", b.HandleV1JobStatus)
			r.Post("/test", b.HandleV1TestInference)

			r.Route("/config", func(r chi.Router) {
				r.Get("/", b.HandleV1ConfigGet)
				r.Post("/", b.HandleV1ConfigUpdate)
			})
		})
	})

	return r
}

func (b *Balancer) AdminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Check if authenticated via OIDC and is Admin
		if val := r.Context().Value(auth.ContextKeyUser); val != nil {
			if user, ok := val.(models.User); ok {
				if user.IsAdmin {
					next.ServeHTTP(w, r)
					return
				}
			}
		}

		// 2. Check if authenticated via Master Token
		if tkn, ok := r.Context().Value(auth.ContextKeyToken).(string); ok {
			if tkn != "" && tkn == b.Config.AuthToken {
				next.ServeHTTP(w, r)
				return
			}
		}

		http.Error(w, "Forbidden: Admin access required", http.StatusForbidden)
	})
}
