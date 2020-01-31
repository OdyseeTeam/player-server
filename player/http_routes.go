package player

import (
	"net/http"
	"net/http/pprof"

	"github.com/gorilla/mux"
)

const ProfileRoutePath = "/superdebug/pprof"

func InstallPlayerRoutes(r *mux.Router, p *Player) {
	playerHandler := NewRequestHandler(p)
	playerRouter := r.Path("/content/claims/{uri}/{claim}/{filename}").Subrouter()
	playerRouter.HandleFunc("", playerHandler.Handle).Methods(http.MethodGet)
	playerRouter.HandleFunc("", playerHandler.HandleHead).Methods(http.MethodHead)
}

func InstallProfilingRoutes(r *mux.Router) {
	debugRouter := r.PathPrefix(ProfileRoutePath).Subrouter()
	debugRouter.HandleFunc("/", pprof.Index)
	debugRouter.HandleFunc("/cmdline", pprof.Cmdline)
	debugRouter.HandleFunc("/profile", pprof.Profile)
	debugRouter.HandleFunc("/symbol", pprof.Symbol)
	debugRouter.HandleFunc("/trace", pprof.Trace)
	debugRouter.Handle("/heap", pprof.Handler("heap"))
}
