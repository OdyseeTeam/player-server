package config

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/lbryio/lbrytv-player/player"

	"github.com/gorilla/mux"
)

var UserName string
var Password string

func InstallConfigRoute(r *mux.Router) {
	r.Handle("/config/throttle", basicAuthWrapper(throttle()))
}

//throttle allows for live configuration of the throttle scalar for the player (MB/s)
//http://localhost:8080/config/throttle?scale=1.2    //postman to add basic auth or temporarily remove basic auth
func throttle() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enabledStr := r.FormValue("enabled")
		if enabledStr != "" {
			enabled, err := strconv.ParseBool(enabledStr)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, "failed to parse enabled %s", enabledStr)
			} else {
				player.ThrottleSwitch = enabled
			}
		}
		scaleStr := r.FormValue("scale")
		if scaleStr != "" {
			scale, err := strconv.ParseFloat(scaleStr, 64)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, "failed to parse scale %s", scaleStr)
			} else {
				player.ThrottleScale = scale
			}
		}
	})
}

//basicAuthWrapper is a wrapper function to ensure some authentication for accessing an API
func basicAuthWrapper(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok {
			http.Error(w, "authentication required", http.StatusBadRequest)
			return
		}
		if user == UserName && pass == Password {
			h.ServeHTTP(w, r)
		} else {
			http.Error(w, "invalid username or password", http.StatusForbidden)
		}
	})
}
