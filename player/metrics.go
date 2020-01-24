package player

import (
	"fmt"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func InstallMetricsRoutes(r *mux.Router) {
	r.Handle("/metrics", promhttp.Handler())
}

var (
	nsPlayer       = "player"
	MetLabelSource = "source"

	MetStreamsRunning = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: nsPlayer,
		Subsystem: "streams",
		Name:      "running",
		Help:      "Number of streams currently playing",
	})
	MetRetrieverSpeed = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: nsPlayer,
		Subsystem: "retriever",
		Name:      "speed_mbps",
		Help:      "Speed of blob/chunk retrieval",
	}, []string{MetLabelSource})

	MetInBytes = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: nsPlayer,
		Name:      "in_bytes",
		Help:      "Total number of bytes downloaded",
	})
	MetOutBytes = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: nsPlayer,
		Name:      "out_bytes",
		Help:      "Total number of bytes streamed out",
	})

	MetCacheHitCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: nsPlayer,
		Subsystem: "cache",
		Name:      "hit_count",
		Help:      "Total number of blobs found in the local cache",
	})
	MetCacheMissCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: nsPlayer,
		Subsystem: "cache",
		Name:      "miss_count",
		Help:      "Total number of blobs that were not in the local cache",
	})
	MetCacheErrorCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: nsPlayer,
		Subsystem: "cache",
		Name:      "error_count",
		Help:      "Total number of errors retrieving blobs from the local cache",
	})

	MetCacheSize = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: nsPlayer,
		Subsystem: "cache",
		Name:      "size",
		Help:      "Current size of cache",
	})
	MetCacheDroppedCount = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: nsPlayer,
		Subsystem: "cache",
		Name:      "dropped_count",
		Help:      "Total number of blobs dropped at the admission time",
	})
	MetCacheRejectedCount = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: nsPlayer,
		Subsystem: "cache",
		Name:      "rejected_count",
		Help:      "Total number of blobs rejected at the admission time",
	})
)

type Timer struct {
	Started  time.Time
	Duration float64
	hist     prometheus.Histogram
}

func TimerStart() *Timer {
	return &Timer{Started: time.Now()}
}

func (t *Timer) Observe(hist prometheus.Histogram) *Timer {
	t.hist = hist
	return t
}

func (t *Timer) Done() float64 {
	if t.Duration == 0 {
		t.Duration = time.Since(t.Started).Seconds()
		if t.hist != nil {
			t.hist.Observe(t.Duration)
		}
	}
	return t.Duration
}

func (t *Timer) String() string {
	return fmt.Sprintf("%.2f", t.Duration)
}
