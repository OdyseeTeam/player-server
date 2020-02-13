package app

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lbryio/lbrytv-player/pkg/logger"

	"github.com/gorilla/mux"
)

var Logger = logger.GetLogger()

// App holds entities that can be used to control the web server
type App struct {
	DefaultHeaders map[string]string
	Router         *mux.Router
	Address        string

	stopChan chan os.Signal
	stopWait time.Duration
	server   *http.Server
}

// Opts holds basic web server settings.
type Opts struct {
	Address         string
	StopWaitSeconds int
	Listener        *http.Server
}

// New returns a new App HTTP server initialized with settings from supplied Opts.
func New(opts Opts) *App {
	a := &App{
		stopChan:       make(chan os.Signal),
		DefaultHeaders: make(map[string]string),
		Address:        opts.Address,
	}
	if opts.StopWaitSeconds != 0 {
		a.stopWait = time.Second * time.Duration(opts.StopWaitSeconds)
	} else {
		a.stopWait = time.Second * 15
	}

	a.DefaultHeaders["Server"] = "lbrytv media player"
	a.DefaultHeaders["Access-Control-Allow-Origin"] = "*"

	a.Router = a.newRouter()

	a.server = a.newServer()

	return a
}

func (a *App) newServer() *http.Server {
	return &http.Server{
		Addr:    a.Address,
		Handler: a.Router,
		// Can't have WriteTimeout set for streaming endpoints
		WriteTimeout:      time.Second * 0,
		IdleTimeout:       time.Second * 0,
		ReadHeaderTimeout: time.Second * 10,
	}
}

func (a *App) newRouter() *mux.Router {
	r := mux.NewRouter()
	r.Use(a.defaultHeadersMiddleware)
	r.Use(logger.SentryHandler.Handle)
	return r
}

func (a *App) defaultHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range a.DefaultHeaders {
			w.Header().Set(k, v)
		}
		next.ServeHTTP(w, r)
	})
}

// Start starts a HTTP server and returns immediately.
func (a *App) Start() {
	go func() {
		Logger.Infof("starting app server on %v", a.Address)
		if err := a.server.ListenAndServe(); err != nil {
			if err.Error() != "http: Server closed" {
				Logger.Fatal(err)
			}
		}
	}()
}

// ServeUntilShutdown blocks until a shutdown signal is received, then shuts down the HTTP server.
func (a *App) ServeUntilShutdown() {
	signal.Notify(a.stopChan, os.Interrupt, os.Kill, syscall.SIGTERM, syscall.SIGINT)
	sig := <-a.stopChan

	Logger.Printf("caught a signal (%v), shutting down http server...", sig)

	err := a.Shutdown()
	if err != nil {
		Logger.Error("error shutting down http server: ", err)
	} else {
		Logger.Info("http server shut down")
	}
}

// Shutdown gracefully shuts down the HTTP server.
func (a *App) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), a.stopWait)
	defer cancel()
	err := a.server.Shutdown(ctx)
	return err
}
