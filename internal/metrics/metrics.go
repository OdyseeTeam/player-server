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
	ns = "player"
)

var (
	StreamsRunning = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Subsystem: "streams",
		Name:      "running",
		Help:      "Number of streams currently playing",
	})

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

	CacheSize = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Subsystem: "cache",
		Name:      "size",
		Help:      "Current size of cache",
	})
)
