package player

import (
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/lbryio/lbrytv-player/internal/metrics"

	"github.com/lbryio/lbry.go/v2/stream"
	"github.com/lbryio/reflector.go/store"

	"github.com/sirupsen/logrus"
)

// ReadableChunk interface describes generic chunk object that Stream can Read() from.
type ReadableChunk []byte

// Read is called by stream.Read.
func (b ReadableChunk) Read(offset, n int64, dest []byte) (int, error) {
	if offset+n > int64(len(b)) {
		n = int64(len(b)) - offset
	}

	read := copy(dest, b[offset:offset+n])
	return read, nil
}

func (b ReadableChunk) Size() int {
	return len(b)
}

// streamRange provides handy blob calculations for a requested stream range.
type streamRange struct {
	Offset           int64
	ReadLen          int64
	FirstChunkIdx    int64
	LastChunkIdx     int64
	FirstChunkOffset int64
	LastChunkReadLen int64
	LastChunkOffset  int64
}

// getRange returns a streamRange for the given start offset and reader buffer length.
func getRange(offset int64, readLen int) streamRange {
	bc := streamRange{Offset: offset, ReadLen: int64(readLen)}

	bc.FirstChunkIdx = offset / ChunkSize
	bc.LastChunkIdx = offset + int64(readLen)/ChunkSize
	bc.FirstChunkOffset = offset - bc.FirstChunkIdx*ChunkSize
	if bc.FirstChunkIdx == bc.LastChunkIdx {
		bc.LastChunkOffset = offset - bc.LastChunkIdx*ChunkSize
	}
	bc.LastChunkReadLen = (offset + int64(readLen)) - bc.LastChunkOffset - ChunkSize*bc.LastChunkIdx

	return bc
}

func (c streamRange) String() string {
	return fmt.Sprintf("B%v[%v:]-B%v[%v:%v]", c.FirstChunkIdx, c.FirstChunkOffset, c.LastChunkIdx, c.LastChunkOffset, c.LastChunkReadLen)
}

// chunkGetter is an object for retrieving blobs from BlobStore or optionally from local cache.
type chunkGetter struct {
	blobSource     store.BlobStore
	hotCache       *HotCache
	sdBlob         *stream.SDBlob
	seenChunks     []ReadableChunk
	enablePrefetch bool
}

// Get returns a Blob object that can be Read() from.
// It first tries to get it from the local cache, and if it is not found, fetches it from the reflector.
func (b *chunkGetter) Get(n int) (ReadableChunk, error) {
	var (
		//cChunk   ReadableChunk
		rChunk ReadableChunk
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

	metrics.CacheMissCount.Inc()
	rChunk, err = b.hotCache.Fetch(hash, func() (ReadableChunk, error) {
		logrus.Infof("fetching from source: %s", hash)
		startTime := time.Now()
		item, err := b.getChunkFromReflector(hash, b.sdBlob.Key, bi.IV)
		if err != nil || item == nil {
			return nil, err
		}

		rate := float64(item.Size()) / (1024 * 1024) / time.Since(startTime).Seconds() * 8
		metrics.RetrieverSpeed.With(map[string]string{metrics.Source: RetrieverSourceReflector}).Set(rate)
		return item, nil
	})
	if err != nil {
		return nil, err
	}
	b.saveToHotCache(n, rChunk)
	go b.prefetchToCache(n + 1)

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

func (b *chunkGetter) prefetchToCache(startN int) {
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

		if b.hotCache.Get(hash) != nil {
			Logger.Debugf("chunk %v found in cache, not prefetching", hash)
			continue
		}
		Logger.Debugf("prefetching chunk %v", hash)
		startTime := time.Now()
		reflected, err := b.getChunkFromReflector(hash, b.sdBlob.Key, bi.IV)
		if err != nil {
			Logger.Errorf("failed to prefetch chunk %v: %v", hash, err)
			return
		}
		rate := float64(reflected.Size()) / (1024 * 1024) / time.Since(startTime).Seconds() * 8
		metrics.RetrieverSpeed.With(map[string]string{metrics.Source: RetrieverSourceReflector}).Set(rate)
		b.hotCache.Set(hash, reflected)
	}
}

func (b *chunkGetter) getChunkFromReflector(hash string, key, iv []byte) (ReadableChunk, error) {
	blob, err := b.blobSource.Get(hash)
	if err != nil {
		return nil, err
	}

	metrics.InBytes.Add(float64(len(blob)))

	body, err := stream.DecryptBlob(blob, key, iv)
	if err != nil {
		return nil, err
	}

	return body, nil
}
