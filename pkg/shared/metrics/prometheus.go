package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	InferenceRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "flakyollama_inference_requests_total",
		Help: "The total number of inference requests",
	}, []string{"model", "node", "status"})

	InferenceLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "flakyollama_inference_latency_seconds",
		Help:    "Histogram of inference latency",
		Buckets: prometheus.DefBuckets,
	}, []string{"model", "node"})

	NodeHealthStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "flakyollama_node_health_status",
		Help: "Health status of the node (0=broken, 1=degraded, 2=healthy)",
	}, []string{"node", "address"})

	QueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "flakyollama_queue_depth",
		Help: "Current number of requests in the priority queue",
	})
)
