package player

import (
	"bytes"
	"encoding/gob"
	"time"

	"github.com/lbryio/lbrytv-player/internal/metrics"

	"github.com/lbryio/ccache/v2"
	"github.com/lbryio/lbry.go/v2/stream"
	"github.com/lbryio/reflector.go/store"

	"golang.org/x/sync/singleflight"
)

const longTTL = 365 * 24 * time.Hour

// HotCache is basically an in-memory BlobStore but it stores the blobs decrypted
// You have to know which blobs you expect to be sd blobs when using HotCache
type HotCache struct {
	origin store.BlobStore
	cache  *ccache.Cache
	sf     *singleflight.Group
}

func NewHotCache(origin store.BlobStore, maxSizeInBytes int64) *HotCache {
	h := &HotCache{
		origin: origin,
		cache:  ccache.New(ccache.Configure().MaxSize(maxSizeInBytes)),
		sf:     new(singleflight.Group),
	}

	go func() {
		for {
			<-time.After(15 * time.Second)
			metrics.HotCacheSize.Set(float64(h.cache.Size()))
			metrics.HotCacheItems.Set(float64(h.cache.ItemCount()))
			metrics.HotCacheEvictions.Add(float64(h.cache.GetDropped()))
		}
	}()

	return h
}

// GetSDBlob gets an sd blob. If it's not in the cache, it is fetched from the origin and cached.
// store.ErrBlobNotFound is returned if blob is not found.
func (h *HotCache) GetSDBlob(hash string) (stream.SDBlob, error) {
	blob, err, _ := h.sf.Do(hash, h.sdBlobGetter(hash))
	if err != nil {
		return stream.SDBlob{}, err
	}
	return blob.(stream.SDBlob), nil
}

// sdBlobGetter actually gets the sd blob
func (h *HotCache) sdBlobGetter(hash string) func() (interface{}, error) {
	return func() (interface{}, error) {
		cached := h.cache.Get(hash)
		if cached != nil {
			metrics.HotCacheRequestCount.WithLabelValues("sd", "hit").Inc()

			var sd stream.SDBlob
			err := gob.NewDecoder(bytes.NewBuffer(cached.Value().(sized))).Decode(&sd)
			return sd, err
		}

		metrics.HotCacheRequestCount.WithLabelValues("sd", "miss").Inc()
		blob, err := h.origin.Get(hash)
		if err != nil {
			return stream.SDBlob{}, err
		}

		var sdBlob stream.SDBlob
		err = sdBlob.FromBlob(blob)
		if err != nil {
			return sdBlob, err
		}

		encoded := new(bytes.Buffer)
		err = gob.NewEncoder(encoded).Encode(sdBlob)
		if err != nil {
			return stream.SDBlob{}, err
		}

		h.cache.Set(hash, sized(encoded.Bytes()), longTTL)

		return sdBlob, nil
	}
}

// GetChunk gets a decrypted stream chunk. If chunk is not cached, it is fetched from origin
// and decrypted.
func (h *HotCache) GetChunk(hash string, key, iv []byte) (ReadableChunk, error) {
	chunk, err, _ := h.sf.Do(hash, h.chunkGetter(hash, key, iv))
	if err != nil {
		return nil, err
	}
	return chunk.(ReadableChunk), nil
}

// chunkGetter actually gets the chunk
func (h *HotCache) chunkGetter(hash string, key, iv []byte) func() (interface{}, error) {
	return func() (interface{}, error) {
		item := h.cache.Get(hash)
		if item != nil {
			metrics.HotCacheRequestCount.WithLabelValues("chunk", "hit").Inc()
			return ReadableChunk(item.Value().(sized)), nil
		}

		metrics.HotCacheRequestCount.WithLabelValues("chunk", "miss").Inc()
		blob, err := h.origin.Get(hash)
		if err != nil {
			return nil, err
		}

		metrics.InBytes.Add(float64(len(blob)))

		chunk, err := stream.DecryptBlob(blob, key, iv)
		if err != nil {
			return nil, err
		}

		h.cache.Set(hash, sized(chunk), longTTL)

		return ReadableChunk(chunk), nil
	}
}

func (h *HotCache) IsCached(hash string) bool {
	return h.cache.Get(hash) != nil
}

type sized []byte

func (s sized) Size() int64 { return int64(len(s)) }
