package balancer

import (
	"FlakyOllama/pkg/balancer/storage"
	"FlakyOllama/pkg/shared/auth"
	"FlakyOllama/pkg/shared/config"
	"FlakyOllama/pkg/shared/models"
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type metricEntry struct {
	nodeID, model string
	latency       time.Duration
	success       bool
}

// Balancer manages multiple agents and routes requests.
type Balancer struct {
	Address         string
	Agents          map[string]*models.NodeStatus
	Storage         *storage.SQLiteStorage
	Config          *config.Config
	PendingRequests map[string]int       // model_name -> count
	ModelLastUsed   map[string]time.Time // "node_id:model_name" -> last_time
	InProgressPulls map[string]time.Time // model_name -> start_time
	Queue           *RequestQueue
	NodeWorkloads   map[string]int                       // agent_addr -> count
	ClientAffinity  map[string]string                    // client_ip -> agent_id
	PerfCache       map[string]storage.PerformanceMetric // "node_id:model" -> metric
	perfMu          sync.RWMutex
	MetricCh        chan metricEntry
	LogCh           chan models.LogEntry
	Mu              sync.RWMutex
	stopCh          chan struct{}
	logChs          map[chan string]bool
	logMu           sync.Mutex
	httpClient      *http.Client
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
		Address:         address,
		Agents:          make(map[string]*models.NodeStatus),
		Storage:         s,
		Config:          cfg,
		PendingRequests: make(map[string]int),
		ModelLastUsed:   make(map[string]time.Time),
		InProgressPulls: make(map[string]time.Time),
		Queue:           NewRequestQueue(),
		NodeWorkloads:   make(map[string]int),
		ClientAffinity:  make(map[string]string),
		PerfCache:       make(map[string]storage.PerformanceMetric),
		MetricCh:        make(chan metricEntry, 1000),
		LogCh:           make(chan models.LogEntry, 1000),
		stopCh:          make(chan struct{}),
		logChs:          make(map[chan string]bool),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: cfg.TLS.InsecureSkipVerify,
				},
			},
		},
	}

	// Intercept log output
	log.SetOutput(b)

	return b, nil
}

func (b *Balancer) Write(p []byte) (n int, err error) {
	msg := string(p)
	os.Stderr.Write(p) // Also write to stderr

	// Send structured entry to LogCh
	entry := models.LogEntry{
		Timestamp: time.Now(),
		NodeID:    "balancer",
		Level:     models.LevelInfo,
		Component: "core",
		Message:   strings.TrimSpace(msg),
	}
	select {
	case b.LogCh <- entry:
	default:
	}

	return len(p), nil
}

func (b *Balancer) Close() error {
	close(b.stopCh)
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
	log.Printf("Balancer listening on %s (TLS: %v)", b.Address, b.Config.TLS.Enabled)
	if b.Config.TLS.Enabled {
		return http.ListenAndServeTLS(b.Address, b.Config.TLS.CertFile, b.Config.TLS.KeyFile, b.CORS(b.NewMux()))
	}
	return http.ListenAndServe(b.Address, b.CORS(b.NewMux()))
}

// NewMux returns a mux with the balancer's handlers registered.
func (b *Balancer) NewMux() *http.ServeMux {
	token := os.Getenv("BALANCER_TOKEN")
	mux := http.NewServeMux()

	// Base
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	// Ollama Layer
	mux.HandleFunc("/api/generate", auth.Middleware(token, b.HandleGenerate))
	mux.HandleFunc("/api/chat", auth.Middleware(token, b.HandleChat))
	mux.HandleFunc("/api/show", auth.Middleware(token, b.HandleShow))
	mux.HandleFunc("/api/tags", auth.Middleware(token, b.HandleTags))
	mux.HandleFunc("/api/embeddings", auth.Middleware(token, b.HandleEmbed))
	mux.HandleFunc("/api/version", b.HandleVersion)
	mux.HandleFunc("/api/ps", auth.Middleware(token, b.HandlePS))
	mux.HandleFunc("/api/pull", auth.Middleware(token, b.HandlePull))
	mux.HandleFunc("/api/delete", auth.Middleware(token, b.HandleDelete))
	mux.HandleFunc("/api/create", auth.Middleware(token, b.HandleCreate))
	mux.HandleFunc("/api/copy", auth.Middleware(token, b.HandleCopy))
	mux.HandleFunc("/api/push", auth.Middleware(token, b.HandlePush))

	// OpenAI Layer
	mux.HandleFunc("/v1/chat/completions", auth.Middleware(token, b.HandleOpenAIChat))
	mux.HandleFunc("/v1/completions", auth.Middleware(token, b.HandleOpenAICompletions))
	mux.HandleFunc("/v1/models", auth.Middleware(token, b.HandleOpenAIModels))
	mux.HandleFunc("/v1/embeddings", auth.Middleware(token, b.HandleOpenAIEmbeddings))

	// Management Layer
	mux.HandleFunc("/register", b.HandleRegister)
	mux.HandleFunc("/api/status", auth.Middleware(token, b.HandleAPIStatus))
	mux.HandleFunc("/api/logs", b.HandleLogs)
	mux.HandleFunc("/api/log/collect", b.HandleLogCollect)
	mux.HandleFunc("/api/manage/node/drain", auth.Middleware(token, b.HandleNodeDrain))
	mux.HandleFunc("/api/manage/node/undrain", auth.Middleware(token, b.HandleNodeUndrain))
	mux.HandleFunc("/api/manage/model/unload", auth.Middleware(token, b.HandleModelUnload))
	mux.HandleFunc("/api/manage/model/pull", auth.Middleware(token, b.HandleModelPull))
	mux.HandleFunc("/api/manage/model/delete", auth.Middleware(token, b.HandleModelDelete))
	mux.HandleFunc("/api/manage/test", auth.Middleware(token, b.HandleTestInference))

	return mux
}
