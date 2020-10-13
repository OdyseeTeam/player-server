package player

import (
	"time"

	"github.com/karlseguin/ccache/v2"
)

type HotCache struct {
	cache *ccache.Cache
	ttl   time.Duration
}

var hc *HotCache

func Init(size int64, ttl time.Duration) *HotCache {
	if hc == nil {
		hc = &HotCache{cache: ccache.New(ccache.Configure().MaxSize(size)), ttl: ttl}
	}
	return hc
}

func (h *HotCache) Get(key string) *reflectedChunk {
	cachedItem := h.cache.Get(key)
	if cachedItem != nil && !cachedItem.Expired() {
		item := cachedItem.Value().(reflectedChunk)
		return &item
	}
	return nil
}

func (h *HotCache) Set(key string, chunk *reflectedChunk) {
	h.cache.Set(key, *chunk, h.ttl)
}

func (h *HotCache) Fetch(key string, fetchFunc func() (interface{}, error)) (*reflectedChunk, error) {
	fetchedItem, err := h.cache.Fetch(key, h.ttl, fetchFunc)
	if err != nil {
		return nil, err
	}
	if fetchedItem != nil {
		item := fetchedItem.Value().(reflectedChunk)
		return &item, nil
	}
	return nil, nil
}
