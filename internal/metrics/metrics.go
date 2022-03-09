package metrics

import (
	"fmt"

	"github.com/chenjiandongx/ginprom"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func InstallRoute(r *gin.Engine) {
	r.Use(ginprom.PromMiddleware(nil))

	// register the `/metrics` route.
	r.GET("/metrics", ginprom.PromHandler(promhttp.Handler()))
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

	StreamsDelivered = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: ns,
		Subsystem: "streams",
		Name:      "delivered",
		Help:      "Total number streams delivered",
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
	DecryptedCacheRequestCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: ns,
		Subsystem: "decryptedcache",
		Name:      "request_total",
		Help:      "Total number of objects requested from decrypted cache",
	}, []string{"object_type", "result"})
	HotCacheEvictions = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: ns,
		Subsystem: "hotcache",
		Name:      "evictions_total",
		Help:      "Total number of items evicted from the cache",
	})
	ResolveFailures = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: ns,
		Subsystem: "resolve",
		Name:      "failures",
		Help:      "Total number of failed SDK resolves",
	})
	ResolveSuccesses = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: ns,
		Subsystem: "resolve",
		Name:      "successes",
		Help:      "Total number of succeeded SDK resolves",
	})
	ResolveTimeMS = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: ns,
		Subsystem: "resolve",
		Name:      "time",
		Help:      "Resolve times",
		Buckets:   []float64{1, 2, 5, 25, 50, 100, 250, 400, 1000},
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
