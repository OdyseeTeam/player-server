package app

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lbryio/lbrytv-player/pkg/logger"

	"github.com/lbryio/lbry.go/v2/extras/errors"
	"github.com/lbryio/reflector.go/peer"
	"github.com/lbryio/reflector.go/peer/http3"
	"github.com/lbryio/reflector.go/store"

	"github.com/gorilla/mux"
)

var Logger = logger.GetLogger()

// App holds entities that can be used to control the web server
type App struct {
	DefaultHeaders map[string]string
	Router         *mux.Router
	Address        string
	BlobStore      store.BlobStore

	stopChan    chan os.Signal
	stopWait    time.Duration
	server      *http.Server
	peerServer  *peer.Server
	peer3Server *http3.Server
}

// Opts holds basic web server settings.
type Opts struct {
	Address         string
	StopWaitSeconds int
	Listener        *http.Server
	BlobStore       store.BlobStore
}

// New returns a new App HTTP server initialized with settings from supplied Opts.
func New(opts Opts) *App {
	a := &App{
		stopChan: make(chan os.Signal),
		DefaultHeaders: map[string]string{
			"Access-Control-Allow-Origin": "*",
			"Server":                      "lbrytv media player",
		},
		Address:   opts.Address,
		BlobStore: opts.BlobStore,
		stopWait:  15 * time.Second,
	}

	if opts.StopWaitSeconds != 0 {
		a.stopWait = time.Second * time.Duration(opts.StopWaitSeconds)
	}

	a.Router = a.newRouter()
	a.server = a.newServer()

	if a.BlobStore != nil {
		a.peerServer = peer.NewServer(a.BlobStore)
		a.peer3Server = http3.NewServer(a.BlobStore)
	}

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

	if a.peerServer != nil {
		err := a.peerServer.Start(":5667")
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			Logger.Fatal(err)
		}
	}

	if a.peer3Server != nil {
		err := a.peer3Server.Start(":5668")
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			Logger.Fatal(err)
		}
	}
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

	if a.peerServer != nil {
		a.peerServer.Shutdown()
	}

	if a.peer3Server != nil {
		a.peer3Server.Shutdown()
	}
}

// Shutdown gracefully shuts down the HTTP server.
func (a *App) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), a.stopWait)
	defer cancel()
	err := a.server.Shutdown(ctx)
	return err
}
