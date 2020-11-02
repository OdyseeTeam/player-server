package player

import (
	"time"

	"github.com/lbryio/lbrytv-player/internal/metrics"

	"github.com/lbryio/lbry.go/v2/stream"
	"github.com/lbryio/reflector.go/store"

	"github.com/karlseguin/ccache/v2"
)

const longTTL = 365 * 24 * time.Hour

// HotCache is basically an in-memory BlobStore but it stores the blobs decrypted
// You have to know which blobs you expect to be sd blobs when using HotCache
type HotCache struct {
	// Origin for both sd blobs and content blobs
	origin store.BlobStore
	// Content blobs are stored here in decrypted form
	chunkCache *ccache.Cache
	// SD blobs are stored here as they are
	sdCache *ccache.Cache
}

func NewHotCache(origin store.BlobStore, maxChunks, maxSDBlobs int) *HotCache {
	h := &HotCache{
		origin:     store.WithSingleFlight("hotcache", origin),
		chunkCache: ccache.New(ccache.Configure().MaxSize(int64(maxChunks))),
		sdCache:    ccache.New(ccache.Configure().MaxSize(int64(maxSDBlobs))),
	}

	go func() {
		for {
			<-time.After(15 * time.Second)
			metrics.CacheSize.WithLabelValues("chunk").Set(float64(h.chunkCache.ItemCount()))
			metrics.CacheSize.WithLabelValues("sd").Set(float64(h.sdCache.ItemCount()))
			metrics.CacheEvictions.WithLabelValues("chunk").Add(float64(h.chunkCache.GetDropped()))
			metrics.CacheEvictions.WithLabelValues("sd").Add(float64(h.sdCache.GetDropped()))
		}
	}()

	return h
}

// GetSDBlob gets an sd blob. If it's not in the cache, it is fetched from the origin and cached.
// store.ErrBlobNotFound is returned if blob is not found.
func (h *HotCache) GetSDBlob(hash string) (stream.SDBlob, error) {
	cached := h.sdCache.Get(hash)
	if cached != nil {
		return cached.Value().(stream.SDBlob), nil
	}

	blob, err := h.origin.Get(hash)
	if err != nil {
		return stream.SDBlob{}, err
	}

	var sdBlob stream.SDBlob
	err = sdBlob.FromBlob(blob)
	if err != nil {
		return sdBlob, err
	}

	h.sdCache.Set(hash, sdBlob, longTTL)

	return sdBlob, nil
}

// GetChunk gets a decrypted stream chunk. If chunk is not cached, it is fetched from origin
// and decrypted.
func (h *HotCache) GetChunk(hash string, key, iv []byte) (ReadableChunk, error) {
	item := h.chunkCache.Get(hash)
	if item != nil {
		return item.Value().(ReadableChunk), nil
	}

	blob, err := h.origin.Get(hash)
	if err != nil {
		return nil, err
	}

	metrics.InBytes.Add(float64(len(blob)))

	chunk, err := stream.DecryptBlob(blob, key, iv)
	if err != nil {
		return nil, err
	}

	h.chunkCache.Set(hash, ReadableChunk(chunk), longTTL)

	return chunk, nil
}

func (h *HotCache) IsChunkCached(hash string) bool {
	return h.chunkCache.Get(hash) != nil
}
