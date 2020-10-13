package player

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/lbryio/lbrytv-player/pkg/logger"
	"github.com/lbryio/lbrytv-player/pkg/paid"
	"github.com/lbryio/reflector.go/peer"
	"github.com/lbryio/reflector.go/store"
	pb "github.com/lbryio/types/v2/go"
	"github.com/sirupsen/logrus"

	ljsonrpc "github.com/lbryio/lbry.go/v2/extras/jsonrpc"
	"github.com/lbryio/lbry.go/v2/stream"
	"github.com/lbryio/reflector.go/peer/http3"
)

var Logger = logger.GetLogger()

const (
	// ChunkSize is a size of decrypted blob.
	ChunkSize = stream.MaxBlobSize - 1

	// DefaultPrefetchLen is how many blobs we should prefetch ahead.
	// 3 should be enough to deliver 2 x 4 = 8MB/s streams.
	DefaultPrefetchLen = 3

	// RetrieverSourceL2Cache is for labeling cache speed sourced from level 2 chunk cache (LFU)
	RetrieverSourceL2Cache = "l2_cache"

	// RetrieverSourceReflector is for labeling cache speed sourced from reflector
	RetrieverSourceReflector = "reflector"
)

// Player is an entry-point object to the new player package.
type Player struct {
	lbrynetClient     *ljsonrpc.Client
	chunkGetter       chunkGetter
	localCache        ChunkCache
	enablePrefetch    bool
	reflectorProtocol string

	reflectorAddress string
	reflectorTimeout time.Duration
}

// Opts are options to be set for Player instance.
type Opts struct {
	EnablePrefetch    bool
	LocalCache        ChunkCache
	ReflectorAddress  string
	ReflectorTimeout  time.Duration
	LbrynetAddress    string
	ReflectorProtocol string
}

var defaultOpts = Opts{
	LbrynetAddress:    "http://localhost:5279",
	ReflectorAddress:  "reflector.lbry.com:5568",
	ReflectorTimeout:  30 * time.Second,
	ReflectorProtocol: "http3",
}

// Stream provides an io.ReadSeeker interface to a stream of blobs to be used by standard http library for range requests,
// as well as some stream metadata.
type Stream struct {
	URI            string
	Hash           string
	sdBlob         *stream.SDBlob
	Size           int64
	ContentType    string
	Claim          *ljsonrpc.Claim
	seekOffset     int64
	chunkGetter    chunkGetter
	resolvedStream *pb.Stream
}

// chunkGetter is an object for retrieving blobs from BlobStore or optionally from local cache.
type chunkGetter struct {
	localCache     ChunkCache
	sdBlob         *stream.SDBlob
	seenChunks     []ReadableChunk
	enablePrefetch bool
	getBlobStore   func() store.BlobStore
}

// ReadableChunk interface describes generic chunk object that Stream can Read() from.
type ReadableChunk interface {
	Read(offset, n int, dest []byte) (int, error)
	Size() int
}

type reflectedChunk struct {
	body []byte
}

// NewPlayer initializes an instance with optional BlobStore.
func NewPlayer(opts *Opts) *Player {
	if opts == nil {
		opts = &defaultOpts
	}
	if opts.LbrynetAddress == "" {
		opts.LbrynetAddress = defaultOpts.LbrynetAddress
	}
	if opts.ReflectorAddress == "" {
		opts.ReflectorAddress = defaultOpts.ReflectorAddress
	}
	if opts.ReflectorTimeout == 0 {
		opts.ReflectorTimeout = defaultOpts.ReflectorTimeout
	}
	p := &Player{
		reflectorAddress:  opts.ReflectorAddress,
		reflectorTimeout:  opts.ReflectorTimeout,
		lbrynetClient:     ljsonrpc.NewClient(opts.LbrynetAddress),
		reflectorProtocol: opts.ReflectorProtocol,
		localCache:        opts.LocalCache,
		enablePrefetch:    opts.EnablePrefetch,
	}

	return p
}

func (p *Player) getBlobStore() store.BlobStore {
	switch p.reflectorProtocol {
	case "tcp":
		return peer.NewStore(peer.StoreOpts{
			Address: p.reflectorAddress,
			Timeout: p.reflectorTimeout,
		})
	case "http3":
		return http3.NewStore(http3.StoreOpts{
			Address: p.reflectorAddress,
			Timeout: p.reflectorTimeout,
		})
	default:
		log.Fatalf("specified protocol is not supported: %s", p.reflectorProtocol)
	}
	return nil
}

// Play delivers requested URI onto the supplied http.ResponseWriter.
func (p *Player) Play(s *Stream, w http.ResponseWriter, r *http.Request) error {
	MtrStreamsRunning.Inc()
	defer MtrStreamsRunning.Dec()
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
	bStore := p.getBlobStore()
	blob, err := bStore.Get(s.Hash)
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
		sdBlob:         &sdBlob,
		localCache:     p.localCache,
		enablePrefetch: p.enablePrefetch,
		seenChunks:     make([]ReadableChunk, len(sdBlob.BlobInfos)-1),
		getBlobStore:   func() store.BlobStore { return p.getBlobStore() },
	}

	return nil
}

func (s *Stream) setSize(blobs *[]stream.BlobInfo) {
	if s.Size > 0 {
		return
	}

	size, err := s.Claim.GetStreamSizeByMagic()

	if err != nil {
		Logger.Infof("couldn't figure out stream %v size by magic: %v", s.URI, err)
		for _, blob := range *blobs {
			if blob.Length == stream.MaxBlobSize {
				size += ChunkSize
			} else {
				size += uint64(blob.Length - 1)
			}
		}
		// last padding is unguessable
		size -= 16
	}

	s.Size = int64(size)
}

// Timestamp returns stream creation timestamp, used in HTTP response header.
func (s *Stream) Timestamp() time.Time {
	return time.Unix(int64(s.Claim.Timestamp), 0)
}

// Seek implements io.ReadSeeker interface and is meant to be called by http.ServeContent.
func (s *Stream) Seek(offset int64, whence int) (int64, error) {
	var newOffset int64

	if s.Size == 0 {
		return 0, errStreamSizeZero
	} else if int64(math.Abs(float64(offset))) > s.Size {
		return 0, errOutOfBounds
	}

	switch whence {
	case io.SeekStart:
		newOffset = offset
	case io.SeekCurrent:
		newOffset = s.seekOffset + offset
	case io.SeekEnd:
		newOffset = s.Size - offset
	default:
		return 0, errors.New("invalid seek whence argument")
	}

	if newOffset < 0 {
		return 0, errSeekingBeforeStart
	}

	s.seekOffset = newOffset
	return newOffset, nil
}

// Read implements io.ReadSeeker interface and is meant to be called by http.ServeContent.
// Actual chunk retrieval and delivery happens in s.readFromChunks().
func (s *Stream) Read(dest []byte) (n int, err error) {
	calc := newChunkCalculator(s.Size, s.seekOffset, len(dest))

	n, err = s.readFromChunks(calc, dest)
	s.seekOffset += int64(n)

	MtrOutBytes.Add(float64(n))

	if err != nil {
		Logger.Errorf("failed to read from stream %v at offset %v: %v", s.URI, s.seekOffset, err)
	}

	return n, err
}

func (s *Stream) readFromChunks(calc chunkCalculator, dest []byte) (int, error) {
	var b ReadableChunk
	var err error
	var read int

	for i := calc.FirstChunkIdx; i < calc.LastChunkIdx+1; i++ {
		var start, readLen int

		if i == calc.FirstChunkIdx {
			start = calc.FirstChunkOffset
			readLen = ChunkSize - calc.FirstChunkOffset
		} else if i == calc.LastChunkIdx {
			start = calc.LastChunkOffset
			readLen = calc.LastChunkReadLen
		} else if calc.FirstChunkIdx == calc.LastChunkIdx {
			start = calc.FirstChunkOffset
			readLen = calc.LastChunkReadLen
		}

		b, err = s.chunkGetter.Get(i)
		if err != nil {
			return read, err
		}

		n, err := b.Read(start, readLen, dest[read:])
		read += n
		if err != nil {
			return read, err
		}
	}

	return read, nil
}

// Get returns a Blob object that can be Read() from.
// It first tries to get it from the local cache, and if it is not found, fetches it from the reflector.
func (b *chunkGetter) Get(n int) (ReadableChunk, error) {
	var (
		//cChunk   ReadableChunk
		rChunk *reflectedChunk
		//cacheHit bool
		err error
	)
	if n > len(b.sdBlob.BlobInfos) {
		return nil, errors.New("blob index out of bounds")
	}

	if b.seenChunks[n] != nil {
		return b.seenChunks[n], nil
	}

	bi := b.sdBlob.BlobInfos[n]
	hash := hex.EncodeToString(bi.BlobHash)

	MtrCacheMissCount.Inc()
	hotCache := Init(1000, 5*time.Minute)
	rChunk, err = hotCache.Fetch(hash, func() (interface{}, error) {
		logrus.Infof("fetching from source: %s", hash)
		timerReflector := TimerStart()
		item, err := b.getChunkFromReflector(hash, b.sdBlob.Key, bi.IV)
		if err != nil {
			return nil, err
		}
		if item == nil {
			return nil, nil
		}
		timerReflector.Done()
		rate := float64(item.Size()) / (1024 * 1024) / timerReflector.Duration * 8
		MtrRetrieverSpeed.With(map[string]string{MtrLabelSource: RetrieverSourceReflector}).Set(rate)
		return *item, nil
	})
	if err != nil {
		return nil, err
	}
	b.saveToHotCache(n, rChunk)
	go b.prefetchToCache(n+1, hotCache)

	return rChunk, nil
}

func (b *chunkGetter) saveToHotCache(n int, chunk ReadableChunk) {
	// Save chunk in the hot cache so next Get() / Read() goes to it
	b.seenChunks[n] = chunk
	// Remove already read chunks to preserve memory
	if n > 0 {
		b.seenChunks[n-1] = nil
	}
}

func (b *chunkGetter) prefetchToCache(startN int, hotCache *HotCache) {
	if !b.enablePrefetch {
		return
	}

	prefetchLen := DefaultPrefetchLen
	chunksLeft := len(b.sdBlob.BlobInfos) - startN - 1 // Last blob is empty
	if chunksLeft <= 0 {
		return
	} else if chunksLeft < DefaultPrefetchLen {
		prefetchLen = chunksLeft
	}

	Logger.Debugf("prefetching %v chunks to local cache", prefetchLen)
	for _, bi := range b.sdBlob.BlobInfos[startN : startN+prefetchLen] {
		hash := hex.EncodeToString(bi.BlobHash)

		if hotCache.Get(hash) != nil {
			Logger.Debugf("chunk %v found in cache, not prefetching", hash)
			continue
		}
		Logger.Debugf("prefetching chunk %v", hash)
		timerReflector := TimerStart()
		reflected, err := b.getChunkFromReflector(hash, b.sdBlob.Key, bi.IV)
		if err != nil {
			Logger.Errorf("failed to prefetch chunk %v: %v", hash, err)
			return
		}
		timerReflector.Done()
		rate := float64(reflected.Size()) / (1024 * 1024) / timerReflector.Duration * 8
		MtrRetrieverSpeed.With(map[string]string{MtrLabelSource: RetrieverSourceReflector}).Set(rate)
		hotCache.Set(hash, reflected)
	}
}

func (b *chunkGetter) getChunkFromReflector(hash string, key, iv []byte) (*reflectedChunk, error) {
	bStore := b.getBlobStore()
	blob, err := bStore.Get(hash)
	if err != nil {
		return nil, err
	}

	MtrInBytes.Add(float64(len(blob)))

	body, err := stream.DecryptBlob(blob, key, iv)
	if err != nil {
		return nil, err
	}

	chunk := &reflectedChunk{body}
	return chunk, nil
}

// Read is called by stream.Read.
func (b *reflectedChunk) Read(offset, n int, dest []byte) (int, error) {
	if offset+n > len(b.body) {
		n = len(b.body) - offset
	}

	read := copy(dest, b.body[offset:offset+n])
	return read, nil
}

func (b *reflectedChunk) Size() int {
	return len(b.body)
}

// chunkCalculator provides handy blob calculations for a requested stream range.
type chunkCalculator struct {
	Offset           int64
	ReadLen          int
	FirstChunkIdx    int
	LastChunkIdx     int
	FirstChunkOffset int
	LastChunkReadLen int
	LastChunkOffset  int
}

// newChunkCalculator initializes chunkCalculator with provided stream size, start offset and reader buffer length.
func newChunkCalculator(size, offset int64, readLen int) chunkCalculator {
	bc := chunkCalculator{Offset: offset, ReadLen: readLen}

	bc.FirstChunkIdx = int(offset / int64(ChunkSize))
	bc.LastChunkIdx = int((offset + int64(readLen)) / int64(ChunkSize))
	bc.FirstChunkOffset = int(offset - int64(bc.FirstChunkIdx*ChunkSize))
	if bc.FirstChunkIdx == bc.LastChunkIdx {
		bc.LastChunkOffset = int(offset - int64(bc.LastChunkIdx*ChunkSize))
	}
	bc.LastChunkReadLen = int((offset + int64(readLen)) - int64(bc.LastChunkOffset) - int64(ChunkSize)*int64(bc.LastChunkIdx))

	return bc
}

func (c chunkCalculator) String() string {
	return fmt.Sprintf("B%v[%v:]-B%v[%v:%v]", c.FirstChunkIdx, c.FirstChunkOffset, c.LastChunkIdx, c.LastChunkOffset, c.LastChunkReadLen)
}
