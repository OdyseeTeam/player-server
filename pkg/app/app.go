package app

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/OdyseeTeam/player-server/pkg/logger"

	"github.com/lbryio/lbry.go/v2/extras/errors"
	reflectorHttp "github.com/lbryio/reflector.go/server/http"
	"github.com/lbryio/reflector.go/server/http3"
	"github.com/lbryio/reflector.go/server/peer"
	"github.com/lbryio/reflector.go/store"

	nice "github.com/ekyoung/gin-nice-recovery"
	"github.com/gin-gonic/gin"
)

var (
	Logger         = logger.GetLogger()
	connContextKey = &contextKey{"conn"}
	ReadTimeout    = uint(10)
	WriteTimeout   = uint(15)
)

type contextKey struct {
	key string
}

// App holds entities that can be used to control the web server
type App struct {
	DefaultHeaders map[string]string
	Router         *gin.Engine
	Address        string
	BlobStore      store.BlobStore

	stopChan    chan os.Signal
	stopWait    time.Duration
	server      *http.Server
	peerServer  *peer.Server
	http3Server *http3.Server
	httpServer  *reflectorHttp.Server
}

// Opts holds basic web server settings.
type Opts struct {
	Address         string
	StopWaitSeconds int
	Listener        *http.Server
	BlobStore       store.BlobStore
	EdgeToken       string
}

// New returns a new App HTTP server initialized with settings from supplied Opts.
func New(opts Opts) *App {
	a := &App{
		stopChan: make(chan os.Signal),
		DefaultHeaders: map[string]string{
			"Access-Control-Allow-Origin": "*",
			"Server":                      "Odysee media player",
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
		a.http3Server = http3.NewServer(a.BlobStore, 200)
		a.httpServer = reflectorHttp.NewServer(a.BlobStore, 200, opts.EdgeToken)
	}

	return a
}

func (a *App) newServer() *http.Server {
	return &http.Server{
		Addr:    a.Address,
		Handler: a.Router,
		// Can't have WriteTimeout set for streaming endpoints
		ReadTimeout:       time.Duration(ReadTimeout) * time.Second,
		WriteTimeout:      time.Duration(WriteTimeout) * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       10 * time.Second,
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			return context.WithValue(ctx, connContextKey, c)
		},
	}
}

func (a *App) newRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger())
	// Install nice.Recovery, passing the handler to call after recovery
	r.Use(nice.Recovery(func(c *gin.Context, err interface{}) {
		c.JSON(500, gin.H{
			"title": "Error",
			"err":   err,
		})
	}))

	r.Use(a.defaultHeadersMiddleware())
	r.Use(logger.SentryHandler)
	return r
}

func (a *App) defaultHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		for k, v := range a.DefaultHeaders {
			c.Header(k, v)
		}
		c.Next()
	}
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
		err := a.peerServer.Start(":5567")
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			Logger.Fatal(err)
		}
	}

	if a.http3Server != nil {
		err := a.http3Server.Start(":5568")
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			Logger.Fatal(err)
		}
	}

	if a.httpServer != nil {
		err := a.httpServer.Start(":5569")
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			Logger.Fatal(err)
		}
	}
}

// ServeUntilShutdown blocks until a shutdown signal is received, then shuts down the HTTP server.
func (a *App) ServeUntilShutdown() {
	signal.Notify(a.stopChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
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

	if a.http3Server != nil {
		a.http3Server.Shutdown()
	}

	if a.httpServer != nil {
		a.httpServer.Shutdown()
	}
}

// Shutdown gracefully shuts down the HTTP server.
func (a *App) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), a.stopWait)
	defer cancel()
	err := a.server.Shutdown(ctx)
	return err
}

// GetConnection returns the connection for the request.
func GetConnection(r *http.Request) (net.Conn, error) {
	c, ok := r.Context().Value(connContextKey).(net.Conn)
	if !ok {
		return nil, errors.Err("cannot retrieve connection from request")
	}
	return c, nil
}
