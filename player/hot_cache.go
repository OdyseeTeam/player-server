package player

import (
	"time"

	"github.com/karlseguin/ccache/v2"
)

const longTTL = 365 * 24 * time.Hour

type HotCache struct {
	cache *ccache.Cache
}

func NewHotCache(size int64) *HotCache {
	return &HotCache{cache: ccache.New(ccache.Configure().MaxSize(size))}
}

func (h *HotCache) Get(hash string) ReadableChunk {
	cachedItem := h.cache.Get(hash)
	if cachedItem == nil {
		return nil
	}
	return cachedItem.Value().(ReadableChunk)
}

func (h *HotCache) Set(hash string, chunk ReadableChunk) {
	h.cache.Set(hash, chunk, longTTL)
}

func (h *HotCache) Fetch(hash string, fetchFunc func() (ReadableChunk, error)) (ReadableChunk, error) {
	value, err := fetchFunc()
	if err != nil || value == nil {
		return nil, err
	}

	h.Set(hash, value)
	return value, nil
}
