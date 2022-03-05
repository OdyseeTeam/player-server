package player

import (
	"time"
	"unsafe"

	"github.com/OdyseeTeam/player-server/internal/metrics"
	"github.com/lbryio/lbry.go/v2/stream"

	"github.com/bluele/gcache"
	"golang.org/x/sync/singleflight"
)

const longTTL = 365 * 24 * time.Hour

// HotCache is basically an in-memory BlobStore but it stores the blobs decrypted
// You have to know which blobs you expect to be sd blobs when using HotCache
type HotCache struct {
	origin DecryptedCache
	cache  gcache.Cache
	sf     *singleflight.Group
}

func NewHotCache(origin DecryptedCache, maxSizeInBytes int64) *HotCache {
	h := &HotCache{
		origin: origin,
		cache: gcache.New(int(maxSizeInBytes / stream.MaxBlobSize)).ARC().EvictedFunc(func(key interface{}, value interface{}) {
			metrics.HotCacheEvictions.Add(1)
		}).Build(),
		sf: new(singleflight.Group),
	}

	go func() {
		for {
			<-time.After(15 * time.Second)
			metrics.HotCacheSize.Set(float64(maxSizeInBytes))
			metrics.HotCacheItems.Set(float64(h.cache.Len(false)))
		}
	}()

	return h
}

// GetSDBlob gets an sd blob. If it's not in the cache, it is fetched from the origin and cached.
// store.ErrBlobNotFound is returned if blob is not found.
func (h *HotCache) GetSDBlob(hash string) (*stream.SDBlob, error) {
	cached, err := h.cache.Get(hash)
	if err == nil && cached != nil {
		metrics.HotCacheRequestCount.WithLabelValues("sd", "hit").Inc()
		return cached.(sizedSD).sd, nil
	}

	metrics.HotCacheRequestCount.WithLabelValues("sd", "miss").Inc()
	return h.getSDFromOrigin(hash)
}

// getSDFromOrigin gets the blob from the origin, caches it, and returns it
func (h *HotCache) getSDFromOrigin(hash string) (*stream.SDBlob, error) {
	blob, err, _ := h.sf.Do(hash, func() (interface{}, error) {
		sd, err := h.origin.GetSDBlob(hash)
		if err != nil {
			return nil, err
		}
		_ = h.cache.Set(hash, sizedSD{sd})

		return sd, nil
	})

	if err != nil || blob == nil {
		return nil, err
	}

	return blob.(*stream.SDBlob), nil
}

// GetChunk gets a decrypted stream chunk. If chunk is not cached, it is fetched from origin
// and decrypted.
func (h *HotCache) GetChunk(hash string, key, iv []byte) (ReadableChunk, error) {
	item, err := h.cache.Get(hash)
	if err == nil {
		metrics.HotCacheRequestCount.WithLabelValues("chunk", "hit").Inc()
		return ReadableChunk(item.(sizedSlice)[:]), nil
	}

	metrics.HotCacheRequestCount.WithLabelValues("chunk", "miss").Inc()
	return h.getChunkFromOrigin(hash, key, iv)
}

// getChunkFromOrigin gets the chunk from the origin, decrypts it, caches it, and returns it
func (h *HotCache) getChunkFromOrigin(hash string, key, iv []byte) (ReadableChunk, error) {
	chunk, err, _ := h.sf.Do(hash, func() (interface{}, error) {
		chunk, err := h.origin.GetChunk(hash, key, iv)
		if err != nil {
			return nil, err
		}
		metrics.InBytes.Add(float64(len(chunk)))
		_ = h.cache.Set(hash, sizedSlice(chunk))

		return chunk, nil
	})

	if err != nil || chunk == nil {
		return nil, err
	}

	return chunk.(ReadableChunk)[:], nil
}

func (h *HotCache) IsCached(hash string) bool {
	return h.cache.Has(hash)
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
