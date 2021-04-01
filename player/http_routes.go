package player

import (
	"net/http"
	"net/http/pprof"

	"github.com/lbryio/lbrytv-player/internal/metrics"

	"github.com/gorilla/mux"
)

const ProfileRoutePath = "/superdebug/pprof"

func InstallPlayerRoutes(r *mux.Router, p *Player) {
	playerHandler := NewRequestHandler(p)

	v1Router := r.Path("/content/claims/{claim_name}/{claim_id}/{filename}").Subrouter()
	v1Router.HandleFunc("", playerHandler.Handle).Methods(http.MethodGet, http.MethodHead)

	v2Router := r.PathPrefix("/api/v2").Subrouter()
	v2Router.Path("/streams/free/{claim_name}/{claim_id}").HandlerFunc(playerHandler.Handle).Methods(http.MethodGet, http.MethodHead)
	v2Router.Path("/streams/paid/{claim_name}/{claim_id}/{token}").HandlerFunc(playerHandler.Handle).Methods(http.MethodGet, http.MethodHead)

	v3Router := r.PathPrefix("/api/v3").Subrouter()
	v3Router.Path("/streams/free/{claim_name}/{claim_id}/{sd_hash}").HandlerFunc(playerHandler.Handle).Methods(http.MethodGet, http.MethodHead)
	v3Router.Path("/streams/paid/{claim_name}/{claim_id}/{sd_hash}/{token}").HandlerFunc(playerHandler.Handle).Methods(http.MethodGet, http.MethodHead)

	v4Router := r.PathPrefix("/api/v4").Subrouter()
	v4Router.Path("/streams/free/{claim_name}/{claim_id}/{sd_hash}").HandlerFunc(playerHandler.Handle).Methods(http.MethodGet, http.MethodHead)
	v4Router.Path("/status").HandlerFunc(playerHandler.Status).Methods(http.MethodGet, http.MethodPost)

	r.PathPrefix(SpeechPrefix).HandlerFunc(playerHandler.Handle).Methods(http.MethodGet, http.MethodHead)

	if p.TCVideoPath != "" {
		fs := http.FileServer(http.Dir(p.TCVideoPath))
		v4Router.PathPrefix("/streams/t").Handler(
			func(h http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					addPoweredByHeaders(w)
					metrics.StreamsRunning.WithLabelValues(metrics.StreamTranscoded).Inc()
					defer metrics.StreamsRunning.WithLabelValues(metrics.StreamTranscoded).Dec()
					h.ServeHTTP(w, r)
				})
			}(http.StripPrefix("/api/v4/streams/t", fs)))
	}
}

func InstallProfilingRoutes(r *mux.Router) {
	debugRouter := r.PathPrefix(ProfileRoutePath).Subrouter()
	debugRouter.HandleFunc("/", pprof.Index)
	debugRouter.Handle("/goroutine", pprof.Handler("goroutine"))
	debugRouter.HandleFunc("/cmdline", pprof.Cmdline)
	debugRouter.HandleFunc("/profile", pprof.Profile)
	debugRouter.HandleFunc("/symbol", pprof.Symbol)
	debugRouter.HandleFunc("/trace", pprof.Trace)
	debugRouter.Handle("/heap", pprof.Handler("heap"))
}
