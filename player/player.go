package player

import (
	"net/http"
	"strings"

	"github.com/lbryio/lbrytv-player/internal/metrics"
	"github.com/lbryio/lbrytv-player/pkg/logger"
	"github.com/lbryio/lbrytv-player/pkg/paid"

	ljsonrpc "github.com/lbryio/lbry.go/v2/extras/jsonrpc"
)

var Logger = logger.GetLogger()

// Player is an entry-point object to the new player package.
type Player struct {
	lbrynetClient  *ljsonrpc.Client
	hotCache       *HotCache
	chunkGetter    chunkGetter
	enablePrefetch bool
}

// NewPlayer initializes an instance with optional BlobStore.
func NewPlayer(hotCache *HotCache, lbrynetAddress string) *Player {
	if lbrynetAddress == "" {
		lbrynetAddress = "http://localhost:5279"
	}

	return &Player{
		lbrynetClient: ljsonrpc.NewClient(lbrynetAddress),
		hotCache:      hotCache,
	}
}

func (p *Player) SetPrefech(enabled bool) {
	p.enablePrefetch = enabled
}

// Play delivers requested URI onto the supplied http.ResponseWriter.
func (p *Player) Play(s *Stream, w http.ResponseWriter, r *http.Request) error {
	metrics.StreamsRunning.Inc()
	defer metrics.StreamsRunning.Dec()
	ServeStream(w, r, s)
	return nil
}

// ResolveStream resolves provided URI by calling the SDK.
func (p *Player) ResolveStream(uri string) (*Stream, error) {
	resolved, err := p.lbrynetClient.Resolve(uri)
	if err != nil {
		return nil, err
	}

	claim := (*resolved)[uri]
	if claim.CanonicalURL == "" {
		return nil, errStreamNotFound
	}

	return NewStream(p, uri, &claim), nil
}

// VerifyAccess checks if the stream is paid and the token supplied matched the stream
func (p *Player) VerifyAccess(s *Stream, token string) error {
	if s.resolvedStream.Fee == nil || s.resolvedStream.Fee.Amount <= 0 {
		return nil
	}

	Logger.WithField("uri", s.URI).Info("paid stream requested")
	if token == "" {
		return errPaidStream
	}
	if err := paid.VerifyStreamAccess(strings.Replace(s.URI, "#", "/", 1), token); err != nil {
		return err
	}
	return nil
}
