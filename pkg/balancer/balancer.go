package balancer

import (
	"FlakyOllama/pkg/balancer/jobs"
	"FlakyOllama/pkg/balancer/state"
	"FlakyOllama/pkg/balancer/storage"
	"FlakyOllama/pkg/shared/auth"
	"FlakyOllama/pkg/shared/config"
	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/models"
	"crypto/tls"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type metricEntry struct {
	nodeID, model string
	latency       time.Duration
	success       bool
}

// Balancer manages multiple agents and routes requests.
type Balancer struct {
	Address        string
	Storage        *storage.SQLiteStorage
	Config         *config.Config
	State          *state.ClusterStateActor
	Jobs           *jobs.JobManager
	Queue          *RequestQueue
	ClientAffinity map[string]string // client_ip -> agent_id
	affinityMu     sync.RWMutex
	PerfCache      map[string]storage.PerformanceMetric // "node_id:model" -> metric
	perfMu         sync.RWMutex
	MetricCh       chan metricEntry
	LogCh          chan models.LogEntry
	stopCh         chan struct{}
	logChs         map[chan string]bool
	logMu          sync.Mutex
	httpClient     *http.Client
	StartTime      time.Time
}

func NewBalancer(address string, dbPath string, cfg *config.Config) (*Balancer, error) {
	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		return nil, err
	}

	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	b := &Balancer{
		Address:        address,
		Storage:        s,
		Config:         cfg,
		State:          state.NewClusterStateActor(),
		Jobs:           jobs.NewJobManager(),
		Queue:          NewRequestQueue(),
		ClientAffinity: make(map[string]string),
		PerfCache:      make(map[string]storage.PerformanceMetric),
		MetricCh:       make(chan metricEntry, 1000),
		LogCh:          make(chan models.LogEntry, 1000),
		stopCh:         make(chan struct{}),
		logChs:         make(map[chan string]bool),
		StartTime:      time.Now(),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: cfg.TLS.InsecureSkipVerify,
				},
			},
		},
	}

	b.State.Start()
	return b, nil
}

func (b *Balancer) Ship(entry models.LogEntry) {
	select {
	case b.LogCh <- entry:
	default:
	}
}

func (b *Balancer) Close() error {
	close(b.stopCh)
	b.Queue.Close()
	return b.Storage.Close()
}

func (b *Balancer) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	promhttp.Handler().ServeHTTP(w, r)
}

func (b *Balancer) CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Origin, Accept")
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
		return http.ListenAndServeTLS(b.Address, b.Config.TLS.CertFile, b.Config.TLS.KeyFile, b.CORS(b.NewMux()))
	}
	return http.ListenAndServe(b.Address, b.CORS(b.NewMux()))
}

// NewMux returns a mux with the balancer's handlers registered.
func (b *Balancer) NewMux() *chi.Mux {
	token := b.Config.AuthToken
	remoteToken := b.Config.RemoteToken
	r := chi.NewRouter()

	r.Use(b.CORS)

	// Base
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/metrics", b.HandleMetrics)

	// Legacy Ollama Layer
	r.Group(func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return auth.Middleware(token, next.ServeHTTP)
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
			return auth.Middleware(token, next.ServeHTTP)
		})
		r.Post("/v1/chat/completions", b.HandleOpenAIChat)
		r.Post("/v1/completions", b.HandleOpenAICompletions)
		r.Get("/v1/models", b.HandleOpenAIModels)
		r.Post("/v1/embeddings", b.HandleOpenAIEmbeddings)
	})

	// Management Layer (Legacy compatibility)
	r.Post("/register", auth.Middleware(remoteToken, b.HandleV1Register))
	r.Post("/api/log/collect", auth.Middleware(remoteToken, b.HandleV1LogCollect))
	r.Get("/api/status", auth.Middleware(token, b.HandleV1ClusterStatus))
	r.Get("/api/logs", auth.Middleware(token, b.HandleV1Logs))

	// Management API (V1 Structured)
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return auth.Middleware(token, next.ServeHTTP)
		})

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

		r.Get("/jobs/{id}", b.HandleV1JobStatus)
		r.Post("/test", b.HandleV1TestInference)

		r.Route("/config", func(r chi.Router) {
			r.Get("/", b.HandleV1ConfigGet)
			r.Post("/", b.HandleV1ConfigUpdate)
		})
	})

	return r
}
