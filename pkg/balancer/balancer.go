package balancer

import (
	"FlakyOllama/pkg/balancer/adapters"
	"FlakyOllama/pkg/balancer/adapters/openai"
	"FlakyOllama/pkg/balancer/config"
	"FlakyOllama/pkg/balancer/hash"
	"FlakyOllama/pkg/balancer/queue"
	"FlakyOllama/pkg/shared/logging"
	"context"
	"crypto/tls"
	"net/http"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type metricEntry struct {
	nodeID  string
	model   string
	latency time.Duration
	success bool
}

type tokenUsageEntry struct {
	nodeID     string
	model      string
	input      int
	output     int
	reward     float64
	cost       float64
	ttft       int64
	duration   int64
	clientKey  string
	userID     string
	selfServed bool // true when the user's own agent served the request
}

type Balancer struct {
	addr      string
	startTime time.Time
	Config    *config.Config
	configMu  sync.RWMutex
	Storage   *SQLiteStorage
	State     *Actor
	Queue     *queue.RequestQueue
	Jobs      *JobManager

	// Caches
	perfCache map[string]struct {
		AvgTTFT, AvgDuration float64
		Requests             int
	}
	policyCache map[string]map[string]struct{ Banned, Pinned, Persistent bool } // [model][nodeID]
	cacheMu     sync.RWMutex

	server     *http.Server
	httpClient *http.Client
	MetricCh   chan metricEntry
	TokenCh    chan tokenUsageEntry
	LogCh      chan logging.LogEntry
	stopCh     chan struct{}

	logMu  sync.RWMutex
	logChs map[chan string]bool

	oidcProvider *oidc.Provider
	oidcMu       sync.Mutex
}

func NewBalancer(addr, dbPath string, cfg *config.Config) (*Balancer, error) {
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		return nil, err
	}

	b := &Balancer{
		addr:      addr,
		startTime: time.Now(),
		Config:    cfg,
		Storage:   s,
		State:     NewActor(),
		Queue:     queue.NewRequestQueue(),
		Jobs:      NewJobManager(),
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: cfg.TLS.InsecureSkipVerify,
				},
			},
			Timeout: 30 * time.Minute, // Long timeout for large models
		},
		MetricCh: make(chan metricEntry, 1000),
		TokenCh:  make(chan tokenUsageEntry, 1000),
		LogCh:    make(chan logging.LogEntry, 1000),
		stopCh:   make(chan struct{}),
		logChs:   make(map[chan string]bool),
		perfCache: make(map[string]struct {
			AvgTTFT, AvgDuration float64
			Requests             int
		}),
		policyCache: make(map[string]map[string]struct{ Banned, Pinned, Persistent bool }),
	}

	b.Init()
	return b, nil
}

func (b *Balancer) Init() {
	// Security Audit
	if b.Config.JWTSecret == "flakyollama-secret-change-me-immediately" {
		logging.Global.Warnf("****************************************************************")
		logging.Global.Warnf("SECURITY WARNING: Using default JWT_SECRET!")
		logging.Global.Warnf("OIDC session cookies can be easily forged by attackers.")
		logging.Global.Warnf("Please set a unique JWT_SECRET in your environment immediately.")
		logging.Global.Warnf("****************************************************************")
	}

	b.StartMetricProcessor()
	b.StartPerfCacheRefresher()
	b.StartLogBroadcaster()
	b.StartBackgroundTasks()
	b.StartTelemetryPoller()
}

func (b *Balancer) SetupRoutes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// Ollama Routes
	r.Route("/api", func(r chi.Router) {
		r.Use(b.AuthMiddleware)
		r.Get("/tags", b.HandleV1Tags)
		r.Post("/tags", b.HandleV1Tags)
		r.Post("/pull", b.HandleV1ModelPull)
		r.Delete("/delete", b.HandleV1ModelDelete)
		r.Post("/log/collect", b.HandleV1LogCollect)
		r.Group(func(r chi.Router) {
			r.Use(b.InferenceQuotaMiddleware)
			r.Post("/generate", b.HandleGenerate)
			r.Post("/chat", b.HandleChat)
			r.Post("/embeddings", b.HandleOllamaEmbeddings)
		})
	})

	// Management API
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(b.AuthMiddleware)

		r.Get("/catalog", b.HandleV1Catalog)

		r.Get("/me", b.HandleV1Me)
		r.Get("/status", b.HandleV1ClusterStatus)
		r.Get("/nodes", b.HandleV1Nodes)
		r.Post("/nodes/register", b.HandleV1Register)

		// User self-service settings
		r.Patch("/user/settings", b.HandleV1UserUpdateSettings)

		// User self-service key management (ownership enforced, no admin required)
		r.Route("/user/keys", func(r chi.Router) {
			r.Route("/clients", func(r chi.Router) {
				r.Post("/", b.HandleV1UserClientKeyCreate)
				r.Delete("/{key}", b.HandleV1UserClientKeyDelete)
				r.Patch("/{key}/settings", b.HandleV1UserClientKeyUpdateSettings)
			})
			r.Route("/agents", func(r chi.Router) {
				r.Post("/", b.HandleV1UserAgentKeyCreate)
				r.Delete("/{key}", b.HandleV1UserAgentKeyDelete)
				r.Post("/{key}/rotate", b.HandleV1UserAgentKeyRotate)
				r.Patch("/{key}/settings", b.HandleV1UserAgentKeyUpdateSettings)
			})
		})

		r.Group(func(r chi.Router) {
			r.Use(b.AdminOnly)
			r.Get("/logs", b.HandleV1Logs)
			r.Get("/logs/history", b.HandleV1LogHistory)
			r.Route("/nodes", func(r chi.Router) {
				r.Post("/{id}/drain", b.HandleV1NodeDrain)
				r.Post("/{id}/undrain", b.HandleV1NodeUndrain)
				r.Delete("/{id}", b.HandleV1NodeDelete)
				r.Post("/{id}/test", b.HandleV1TestInference)
				r.Post("/test", b.HandleV1TestInference)
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
					r.Delete("/{key}", b.HandleV1ClientKeyDelete)
					r.Patch("/{key}/settings", b.HandleV1ClientKeyUpdateSettings)
				})
				r.Route("/agents", func(r chi.Router) {
					r.Get("/", b.HandleV1AgentKeysList)
					r.Post("/", b.HandleV1AgentKeyCreate)
					r.Post("/{key}/rotate", b.HandleV1AgentKeyRotate)
					r.Delete("/{key}", b.HandleV1AgentKeyDelete)
					r.Patch("/{key}/settings", b.HandleV1AgentKeyUpdateSettings)
				})
			})
			r.Route("/users", func(r chi.Router) {
				r.Get("/", b.HandleV1UsersList)
				r.Post("/{id}/quota", b.HandleV1UserUpdateQuota)
				r.Delete("/{id}", b.HandleV1UserDelete)
				r.Get("/{id}/policies", b.HandleV1UserModelPoliciesList)
				r.Post("/policies", b.HandleV1UserModelPolicySet)
			})
			r.Route("/queue", func(r chi.Router) {
				r.Get("/", b.HandleV1QueueList)
				r.Delete("/{id}", b.HandleV1QueueCancel)
			})
			r.Get("/jobs/{id}", b.HandleV1JobStatus)
			r.Route("/config", func(r chi.Router) {
				r.Get("/", b.HandleV1ConfigGet)
				r.Post("/", b.HandleV1ConfigUpdate)
			})
		})
	})

	// Auth Endpoints
	r.Get("/auth/login", b.HandleOIDCLogin)
	r.Get("/auth/callback", b.HandleOIDCCallback)
	r.Get("/auth/logout", b.HandleOIDCLogout)

	// Public (no auth)
	r.Get("/api/public/info", b.HandlePublicInfo)
	r.Get("/api/public/catalog", b.HandleV1Catalog)

	// Adapter Routes
	for _, adapter := range []adapters.Adapter{
		openai.NewOpenAIAdapter(),
		// Register more adapters here
	} {
		adapter.RegisterRoutes(r, b)
	}

	return r
}
func (b *Balancer) Serve() error {
	b.server = &http.Server{
		Addr:    b.addr,
		Handler: b.SetupRoutes(),
	}
	logging.Global.Infof("Balancer listening on %s", b.addr)
	return b.server.ListenAndServe()
}

func (b *Balancer) Stop() {
	close(b.stopCh)
	if b.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		b.server.Shutdown(ctx)
	}
	if b.Storage != nil {
		b.Storage.Close()
	}
}

// Ship LogSink implementation
func (b *Balancer) Ship(entry logging.LogEntry) {
	select {
	case b.LogCh <- entry:
	default:
	}
}

func (b *Balancer) computeHash(input string) string {
	return hash.ComputeHash(input)
}
