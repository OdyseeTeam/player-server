package metrics

import (
	"fmt"

	"github.com/Depado/ginprom"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

func InstallRoute(r *gin.Engine) {
	p := ginprom.New(
		ginprom.Engine(r),
		ginprom.Subsystem("gin"),
		ginprom.Path("/metrics"),
	)
	r.Use(p.Instrument())
}

const (
	ns = "player"

	StreamOriginal   = "original"
	StreamTranscoded = "transcoded"

	ResolveSource = "source"
	ResolveKind   = "kind"

	ResolveSourceCache          = "cache"
	ResolveSourceOApi           = "oapi"
	ResolveFailureGeneral       = "general"
	ResolveFailureClaimNotFound = "claim_not_found"
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
	TcOutBytes = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: ns,
		Name:      "tc_out_bytes",
		Help:      "Total number of bytes streamed out via transcoded content",
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

	ResolveFailures = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: ns,
		Subsystem: "resolve",
		Name:      "failures",
		Help:      "Total number of failed SDK resolves",
	}, []string{ResolveSource, ResolveKind})
	ResolveFailuresDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: ns,
		Subsystem: "resolve",
		Name:      "failures_duration",
		Help:      "Failed resolves durations",
	}, []string{ResolveSource, ResolveKind})

	ResolveSuccesses = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: ns,
		Subsystem: "resolve",
		Name:      "successes",
		Help:      "Total number of succeeded resolves",
	}, []string{ResolveSource})
	ResolveSuccessesDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: ns,
		Subsystem: "resolve",
		Name:      "successes_duration",
		Help:      "Successful resolves durations",
	}, []string{ResolveSource})

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
