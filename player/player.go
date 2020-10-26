package player

import (
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/lbryio/lbrytv-player/internal/metrics"
	"github.com/lbryio/lbrytv-player/pkg/logger"
	"github.com/lbryio/lbrytv-player/pkg/paid"

	ljsonrpc "github.com/lbryio/lbry.go/v2/extras/jsonrpc"
	"github.com/lbryio/lbry.go/v2/stream"
	"github.com/lbryio/reflector.go/store"
)

var Logger = logger.GetLogger()

const (
	// ChunkSize is a size of decrypted blob.
	ChunkSize = stream.MaxBlobSize - 1

	// DefaultPrefetchLen is how many blobs we should prefetch ahead.
	// 3 should be enough to deliver 2 x 4 = 8MB/s streams.
	DefaultPrefetchLen = 3

	// RetrieverSourceReflector is for labeling cache speed sourced from reflector
	RetrieverSourceReflector = "reflector"
)

// Player is an entry-point object to the new player package.
type Player struct {
	lbrynetClient  *ljsonrpc.Client
	blobSource     store.BlobStore
	hotCache       *HotCache
	chunkGetter    chunkGetter
	enablePrefetch bool
}

// Opts are options to be set for Player instance.
type Opts struct {
	EnablePrefetch bool
	BlobSource     store.BlobStore // source of encrypted blobs
	HotCache       *HotCache       // cache for decrypted blobs
	LbrynetAddress string
}

var defaultOpts = Opts{
	LbrynetAddress: "http://localhost:5279",
}

// NewPlayer initializes an instance with optional BlobStore.
func NewPlayer(opts *Opts) *Player {
	if opts == nil {
		opts = &defaultOpts
	}
	if opts.LbrynetAddress == "" {
		opts.LbrynetAddress = defaultOpts.LbrynetAddress
	}

	return &Player{
		lbrynetClient:  ljsonrpc.NewClient(opts.LbrynetAddress),
		blobSource:     opts.BlobSource,
		hotCache:       opts.HotCache,
		enablePrefetch: opts.EnablePrefetch,
	}
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
	s := &Stream{URI: uri}

	r, err := p.lbrynetClient.Resolve(uri)
	if err != nil {
		return nil, err
	}

	claim := (*r)[uri]
	if claim.CanonicalURL == "" {
		return nil, errStreamNotFound
	}

	stream := claim.Value.GetStream()

	s.Claim = &claim
	s.Hash = hex.EncodeToString(stream.Source.SdHash)
	s.ContentType = patchMediaType(stream.Source.MediaType)
	s.Size = int64(stream.Source.Size)
	s.resolvedStream = stream

	return s, nil
}

// VerifyAccess checks if the stream is paid and the token supplied matched the stream
func (p *Player) VerifyAccess(s *Stream, token string) error {
	if s.resolvedStream.Fee != nil && s.resolvedStream.Fee.Amount > 0 {
		Logger.WithField("uri", s.URI).Info("paid stream requested")
		if token == "" {
			return errPaidStream
		}
		if err := paid.VerifyStreamAccess(strings.Replace(s.URI, "#", "/", 1), token); err != nil {
			return err
		}
	}
	return nil
}

// RetrieveStream downloads stream description from the reflector and tries to determine stream size
// using several methods, including legacy ones for streams that do not have metadata.
func (p *Player) RetrieveStream(s *Stream) error {
	sdBlob := stream.SDBlob{}
	blob, err := p.blobSource.Get(s.Hash)
	if err != nil {
		return err
	}

	err = sdBlob.FromBlob(blob)
	if err != nil {
		return err
	}

	s.setSize(&sdBlob.BlobInfos)
	s.sdBlob = &sdBlob
	s.chunkGetter = chunkGetter{
		blobSource:     p.blobSource,
		hotCache:       p.hotCache,
		sdBlob:         &sdBlob,
		enablePrefetch: p.enablePrefetch,
		seenChunks:     make([]ReadableChunk, len(sdBlob.BlobInfos)-1),
	}

	return nil
}
