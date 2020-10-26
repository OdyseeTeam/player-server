package metrics

import (
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func InstallRoute(r *mux.Router) {
	r.Handle("/metrics", promhttp.Handler())
}

const (
	ns     = "player"
	Source = "source"
)

var (
	StreamsRunning = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Subsystem: "streams",
		Name:      "running",
		Help:      "Number of streams currently playing",
	})
	RetrieverSpeed = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: ns,
		Subsystem: "retriever",
		Name:      "speed_mbps",
		Help:      "Speed of blob/chunk retrieval",
	}, []string{Source})

	InBytes = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: ns,
		Name:      "in_bytes",
		Help:      "Total number of bytes downloaded",
	})
	OutBytes = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: ns,
		Name:      "out_bytes",
		Help:      "Total number of bytes streamed out",
	})

	CacheHitCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: ns,
		Subsystem: "cache",
		Name:      "hit_count",
		Help:      "Total number of blobs found in the local cache",
	})
	CacheMissCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: ns,
		Subsystem: "cache",
		Name:      "miss_count",
		Help:      "Total number of blobs that were not in the local cache",
	})
	CacheErrorCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: ns,
		Subsystem: "cache",
		Name:      "error_count",
		Help:      "Total number of errors retrieving blobs from the local cache",
	})

	CacheSize = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Subsystem: "cache",
		Name:      "size",
		Help:      "Current size of cache",
	})
	CacheDroppedCount = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Subsystem: "cache",
		Name:      "dropped_count",
		Help:      "Total number of blobs dropped at the admission time",
	})
	CacheRejectedCount = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Subsystem: "cache",
		Name:      "rejected_count",
		Help:      "Total number of blobs rejected at the admission time",
	})
)
