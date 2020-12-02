package player

import (
	"time"
	"unsafe"

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
		cache:  ccache.New(ccache.Configure().MaxSize(maxSizeInBytes).GetsPerPromote(180)),
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
func (h *HotCache) GetSDBlob(hash string) (*stream.SDBlob, error) {
	cached := h.cache.Get(hash)
	if cached != nil {
		metrics.HotCacheRequestCount.WithLabelValues("sd", "hit").Inc()
		return cached.Value().(sizedSD).sd, nil
	}

	metrics.HotCacheRequestCount.WithLabelValues("sd", "miss").Inc()
	return h.getSDFromOrigin(hash)
}

// getSDFromOrigin gets the blob from the origin, caches it, and returns it
func (h *HotCache) getSDFromOrigin(hash string) (*stream.SDBlob, error) {
	blob, err, _ := h.sf.Do(hash, func() (interface{}, error) {
		blob, err := h.origin.Get(hash)
		if err != nil {
			return nil, err
		}

		var sd stream.SDBlob
		err = sd.FromBlob(blob)
		if err != nil {
			return nil, err
		}

		h.cache.Set(hash, sizedSD{&sd}, longTTL)

		return &sd, nil
	})

	if err != nil || blob == nil {
		return nil, err
	}

	return blob.(*stream.SDBlob), nil
}

// GetChunk gets a decrypted stream chunk. If chunk is not cached, it is fetched from origin
// and decrypted.
func (h *HotCache) GetChunk(hash string, key, iv []byte) (ReadableChunk, error) {
	item := h.cache.Get(hash)
	if item != nil {
		metrics.HotCacheRequestCount.WithLabelValues("chunk", "hit").Inc()
		return ReadableChunk(item.Value().(sizedSlice)[:]), nil
	}

	metrics.HotCacheRequestCount.WithLabelValues("chunk", "miss").Inc()
	return h.getChunkFromOrigin(hash, key, iv)
}

// getChunkFromOrigin gets the chunk from the origin, decrypts it, caches it, and returns it
func (h *HotCache) getChunkFromOrigin(hash string, key, iv []byte) (ReadableChunk, error) {
	chunk, err, _ := h.sf.Do(hash, func() (interface{}, error) {
		blob, err := h.origin.Get(hash)
		if err != nil {
			return nil, err
		}

		metrics.InBytes.Add(float64(len(blob)))

		chunk, err := stream.DecryptBlob(blob, key, iv)
		if err != nil {
			return nil, err
		}

		h.cache.Set(hash, sizedSlice(chunk), longTTL)

		return ReadableChunk(chunk), nil
	})

	if err != nil || chunk == nil {
		return nil, err
	}

	return chunk.(ReadableChunk)[:], nil
}

func (h *HotCache) IsCached(hash string) bool {
	return h.cache.Get(hash) != nil
}

type sizedSlice []byte

func (s sizedSlice) Size() int64 { return int64(len(s)) }

type sizedSD struct {
	sd *stream.SDBlob
}

func (s sizedSD) Size() int64 {
	total := int64(unsafe.Sizeof(s)) + int64(unsafe.Sizeof(&(s.sd)))
	for _, bi := range s.sd.BlobInfos {
		total += int64(unsafe.Sizeof(bi)) + int64(len(bi.BlobHash)+len(bi.IV))
	}
	return total + int64(len(s.sd.StreamName)+len(s.sd.StreamType)+len(s.sd.Key)+len(s.sd.SuggestedFileName)+len(s.sd.StreamHash))
}
