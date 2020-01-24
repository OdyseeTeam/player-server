package server

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

var Logger = logger.NewModuleLogger("server")

// Server holds entities that can be used to control the web server
type Server struct {
	DefaultHeaders map[string]string
	Router         *mux.Router

	stopChan chan os.Signal
	stopWait time.Duration
	address  string
	listener *http.Server
}

// Opts holds basic web server settings.
type Opts struct {
	Address         string
	StopWaitSeconds int
}

// NewServer returns a server initialized with settings from supplied Opts.
func NewServer(opts Opts) *Server {
	s := &Server{
		stopChan:       make(chan os.Signal),
		DefaultHeaders: make(map[string]string),
		address:        opts.Address,
	}
	if opts.StopWaitSeconds != 0 {
		s.stopWait = time.Second * time.Duration(opts.StopWaitSeconds)
	} else {
		s.stopWait = time.Second * 15
	}
	s.DefaultHeaders["Server"] = "lbrytv media player"

	s.Router = s.newRouter()
	s.listener = s.newListener()

	return s
}

func (s *Server) newListener() *http.Server {
	return &http.Server{
		Addr:    s.address,
		Handler: s.Router,
		// Can't have WriteTimeout set for streaming endpoints
		WriteTimeout:      time.Second * 0,
		IdleTimeout:       time.Second * 0,
		ReadHeaderTimeout: time.Second * 10,
	}
}

func (s *Server) newRouter() *mux.Router {
	r := mux.NewRouter()
	r.Use(s.defaultHeadersMiddleware)
	return r
}

func (s *Server) defaultHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range s.DefaultHeaders {
			w.Header().Set(k, v)
		}
		next.ServeHTTP(w, r)
	})
}

// Start starts a http server and returns immediately.
func (s *Server) Start() error {
	go func() {
		if err := s.listener.ListenAndServe(); err != nil {
			if err.Error() != "http: Server closed" {
				Logger.Log().Error(err)
			}
		}
	}()
	Logger.Log().Infof("http server listening on %v", s.address)
	return nil
}

// ServeUntilShutdown blocks until a shutdown signal is received, then shuts down the http server.
func (s *Server) ServeUntilShutdown() {
	signal.Notify(s.stopChan, os.Interrupt, os.Kill, syscall.SIGTERM, syscall.SIGINT)
	sig := <-s.stopChan

	Logger.Log().Printf("caught a signal (%v), shutting down http server...", sig)

	err := s.Shutdown()
	if err != nil {
		Logger.Log().Error("error shutting down server: ", err)
	} else {
		Logger.Log().Info("http server shut down")
	}
}

// Shutdown gracefully shuts down the peer server.
func (s *Server) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.stopWait)
	defer cancel()
	err := s.listener.Shutdown(ctx)
	return err
}
