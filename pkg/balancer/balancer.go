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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Balancer struct {
	Address  string
	configMu sync.RWMutex
	Config   *config.Config
	State    *state.ClusterStateActor
	Storage *storage.SQLiteStorage
	Jobs    *jobs.JobManager
	Queue   *RequestQueue

	httpClient *http.Client
	StartTime  time.Time

	// Performance cache: node_id:model -> PerformanceMetric
	perfMu    sync.RWMutex
	PerfCache map[string]storage.PerformanceMetric

	// Client affinity: IP -> NodeID
	affinityMu      sync.RWMutex
	ClientAffinity  map[string]string
	ContextAffinity map[string]string

	// Channel for async metric processing
	MetricCh chan metricEntry
	TokenCh  chan tokenUsageEntry

	// Log shipping
	LogCh  chan models.LogEntry
	logMu  sync.Mutex
	logChs map[chan string]bool

	stopCh     chan struct{}
	httpServer *http.Server
}

type metricEntry struct {
	nodeID  string
	model   string
	latency time.Duration
	success bool
}

type tokenUsageEntry struct {
	nodeID    string
	model     string
	input     int
	output    int
	reward    float64
	cost      float64
	ttft      int64
	duration  int64
	clientKey string
}

func NewBalancer(addr, dbPath string, cfg *config.Config) (*Balancer, error) {
	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		return nil, err
	}

	b := &Balancer{
		Address: addr,
		Config:  cfg,
		State:   state.NewClusterStateActor(),
		Storage: s,
		Jobs:    jobs.NewJobManager(),
		Queue:   NewRequestQueue(),
		httpClient: &http.Client{
			Timeout: 10 * time.Minute,
		},
		StartTime:       time.Now(),
		PerfCache:       make(map[string]storage.PerformanceMetric),
		ClientAffinity:  make(map[string]string),
		ContextAffinity: make(map[string]string),
		MetricCh:        make(chan metricEntry, 1000),
		TokenCh:         make(chan tokenUsageEntry, 1000),
		LogCh:           make(chan models.LogEntry, 1000),
		logChs:          make(map[chan string]bool),
		stopCh:          make(chan struct{}),
	}

	b.State.Start()
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
	b.httpServer = &http.Server{
		Addr:    b.Address,
		Handler: b.NewMux(),
	}
	if b.Config.TLS.Enabled {
		return b.httpServer.ListenAndServeTLS(b.Config.TLS.CertFile, b.Config.TLS.KeyFile)
	}
	return b.httpServer.ListenAndServe()
}

func (b *Balancer) Stop() {
	logging.Global.Infof("Balancer shutting down...")
	close(b.stopCh)
	if b.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		b.httpServer.Shutdown(ctx)
	}
}

func (b *Balancer) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	promhttp.Handler().ServeHTTP(w, r)
}

func (b *Balancer) NewMux() *chi.Mux {
	token := b.Config.AuthToken
	remoteToken := b.Config.RemoteToken
	r := chi.NewRouter()

	r.Use(middleware.Compress(5))
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

	// Management API (V1 Structured)
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return auth.Middleware(token, b.Storage, next.ServeHTTP)
		})

		// Publicly available to all authenticated users
		r.Get("/catalog", b.HandleV1Catalog)
		r.Get("/me", b.HandleV1Me)
		r.Get("/status", b.HandleV1ClusterStatus)
		r.Get("/nodes", b.HandleV1Nodes)

		// Gated Admin Routes
		r.Group(func(r chi.Router) {
			r.Use(b.AdminOnly)

			r.Get("/logs", b.HandleV1Logs)
			r.Get("/logs/history", b.HandleV1LogHistory)

			r.Route("/nodes", func(r chi.Router) {
				r.Post("/{id}/drain", b.HandleV1NodeDrain)
				r.Post("/{id}/undrain", b.HandleV1NodeUndrain)
				r.Delete("/{id}", b.HandleV1NodeDelete)
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
				r.Post("/status", b.HandleV1KeySetStatus)
				r.Route("/clients", func(r chi.Router) {
					r.Get("/", b.HandleV1ClientKeysList)
					r.Post("/", b.HandleV1ClientKeyCreate)
				})
				r.Route("/agents", func(r chi.Router) {
					r.Get("/", b.HandleV1AgentKeysList)
					r.Post("/", b.HandleV1AgentKeyCreate)
				})
			})

			r.Route("/users", func(r chi.Router) {
				r.Get("/", b.HandleV1UsersList)
				r.Post("/{id}/quota", b.HandleV1UserUpdateQuota)
			})

			r.Route("/queue", func(r chi.Router) {
				r.Get("/", b.HandleV1QueueList)
				r.Delete("/{id}", b.HandleV1QueueCancel)
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
		if val := r.Context().Value(auth.ContextKeyUser); val != nil {
			if user, ok := val.(models.User); ok {
				if user.IsAdmin {
					next.ServeHTTP(w, r)
					return
				}
				logging.Global.Warnf("AdminOnly: Access denied for user %s (Not an Admin)", user.Email)
			}
		}

		if tkn, ok := r.Context().Value(auth.ContextKeyToken).(string); ok {
			if tkn != "" && tkn == b.Config.AuthToken {
				next.ServeHTTP(w, r)
				return
			}
		}

		http.Error(w, "Forbidden: Admin access required", http.StatusForbidden)
	})
}

func (b *Balancer) Ship(entry models.LogEntry) {
	select {
	case b.LogCh <- entry:
	default:
	}
}

func (b *Balancer) getRequestPriority(r *http.Request) int {
	if val := r.Context().Value(auth.ContextKeyUser); val != nil {
		if user, ok := val.(models.User); ok {
			if user.IsAdmin {
				return 100
			}
			cks, err := b.Storage.GetClientKeysByUserID(user.ID)
			if err == nil && len(cks) > 0 {
				return int(cks[0].Credits / 10)
			}
		}
	}
	if val := r.Context().Value(auth.ContextKeyClientData); val != nil {
		if ck, ok := val.(models.ClientKey); ok {
			return int(ck.Credits / 10)
		}
	}
	return 0
}

func (b *Balancer) computeHash(data string) string {
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// RESTORED HANDLERS

func (b *Balancer) HandleV1Register(w http.ResponseWriter, r *http.Request) {
	var req models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.ID == "" || req.Address == "" {
		b.jsonError(w, http.StatusBadRequest, "id and address are required")
		return
	}

	// Address fix for agents registering with 0.0.0.0 or empty address
	addr := req.Address
	if host, port, err := net.SplitHostPort(addr); err == nil {
		if host == "0.0.0.0" || host == "" {
			remoteHost, _, _ := net.SplitHostPort(r.RemoteAddr)
			addr = net.JoinHostPort(remoteHost, port)
		}
	}

	// Get authenticated token from context
	token, _ := r.Context().Value(auth.ContextKeyToken).(string)

	// Verify key if it's not the master remote token
	if token != b.Config.RemoteToken {
		if _, err := b.Storage.GetAgentKey(token); err != nil {
			logging.Global.Warnf("Unauthorized registration attempt from %s with invalid AgentKey", addr)
			b.jsonError(w, http.StatusUnauthorized, "invalid agent key")
			return
		}
	}

	b.State.Do(func(s *state.ClusterState) {
		existing, exists := s.Agents[addr]
		
		status := &models.NodeStatus{
			ID:       req.ID,
			AgentKey: token,
			Address:  addr,
			Tier:     req.Tier,
			HasGPU:   req.HasGPU,
			GPUModel: req.GPUModel,
			State:    models.StateHealthy,
			Errors:   0,
			LastSeen: time.Now(),
		}

		if exists {
			// Preserve sticky state
			status.Reputation = existing.Reputation
			status.InputTokens = existing.InputTokens
			status.OutputTokens = existing.OutputTokens
			status.TokenReward = existing.TokenReward
			status.Draining = existing.Draining
		} else {
			status.Reputation = 1.0 // Initial
		}

		s.Agents[addr] = status
	})

	logging.Global.Infof("Registered agent: %s at %s [Tier: %s, GPU: %v (%s)]", req.ID, addr, req.Tier, req.HasGPU, req.GPUModel)
	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "registered"})
}

func (b *Balancer) HandleV1LogCollect(w http.ResponseWriter, r *http.Request) {
	var entry models.LogEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		b.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	select {
	case b.LogCh <- entry:
	default:
	}
	w.WriteHeader(http.StatusNoContent)
}

func (b *Balancer) HandleV1ModelPull(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model  string `json:"model"`
		NodeID string `json:"node_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	jobID := generateJobID()
	job := b.Jobs.CreateJob(jobID, "model_pull")
	
	// Create request
	request := models.ModelRequest{
		ID:          jobID,
		Type:        models.RequestPull,
		Model:       req.Model,
		NodeID:      req.NodeID,
		Status:      models.StatusPending,
		RequestedAt: time.Now(),
	}

	if err := b.Storage.CreateModelRequest(request); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	b.jsonResponse(w, http.StatusAccepted, map[string]string{"job_id": jobID, "status": string(job.Status)})
}

func (b *Balancer) HandleV1ModelDelete(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	jobID := generateJobID()
	b.Jobs.CreateJob(jobID, "model_delete")

	request := models.ModelRequest{
		ID:          jobID,
		Type:        models.RequestDelete,
		Model:       name,
		Status:      models.StatusPending,
		RequestedAt: time.Now(),
	}

	if err := b.Storage.CreateModelRequest(request); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	b.jsonResponse(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

func (b *Balancer) HandleV1ModelUnload(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var req struct{ NodeID string `json:"node_id"` }
	json.NewDecoder(r.Body).Decode(&req)

	// Placeholder for actual unload logic
	logging.Global.Infof("Model unload requested: %s on node %s", name, req.NodeID)
	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "unload_initiated"})
}

func (b *Balancer) UpdateNodeByID(id string, update func(*models.NodeStatus)) {
	b.State.Do(func(s *state.ClusterState) {
		for _, a := range s.Agents {
			if a.ID == id {
				update(a)
				break
			}
		}
	})
}

func (b *Balancer) captureUsage(agentID, model string, body []byte, clientKey string, input, output int, surge float64) {
	// ... logic to record usage in TokenCh
}
