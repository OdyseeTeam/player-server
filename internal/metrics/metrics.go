package metrics

import (
	"fmt"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func InstallRoute(r *mux.Router) {
	r.Handle("/metrics", promhttp.Handler())
}

const (
	ns = "player"

	StreamOriginal   = "original"
	StreamTranscoded = "transcoded"
)

var (
	StreamsRunning = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: ns,
		Subsystem: "streams",
		Name:      "running",
		Help:      "Number of streams currently playing",
	}, []string{"variant"})

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

	HotCacheSize = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Subsystem: "hotcache",
		Name:      "items_size",
		Help:      "Size of items in cache",
	})
	HotCacheItems = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Subsystem: "hotcache",
		Name:      "items_count",
		Help:      "Number of items in cache",
	})
	HotCacheRequestCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: ns,
		Subsystem: "hotcache",
		Name:      "request_total",
		Help:      "Total number of blobs requested from hot cache",
	}, []string{"blob_type", "result"})
	HotCacheEvictions = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: ns,
		Subsystem: "hotcache",
		Name:      "evictions_total",
		Help:      "Total number of items evicted from the cache",
	})

	playerInfo = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: ns,
		Subsystem: "hotcache",
		Name:      "info",
		Help:      "Info about cache",
	}, []string{"max_size"})
)

func PlayerCacheInfo(cacheSize uint64) {
	playerInfo.With(prometheus.Labels{
		"max_size": fmt.Sprintf("%d", cacheSize),
	}).Set(1.0)
}
