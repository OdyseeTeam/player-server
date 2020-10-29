package player

import (
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/lbryio/lbry.go/v2/stream"
)

const (
	// MaxChunkSize is the max size of decrypted blob.
	MaxChunkSize = stream.MaxBlobSize - 1

	// DefaultPrefetchLen is how many blobs we should prefetch ahead.
	// 3 should be enough to deliver 2 x 4 = 8MB/s streams.
	DefaultPrefetchLen = 3
)

// ReadableChunk is a chunk object that Stream can Read() from.
type ReadableChunk []byte

// Read is called by stream.Read.
func (b ReadableChunk) Read(offset, n int64, dest []byte) (int, error) {
	if offset+n > int64(len(b)) {
		n = int64(len(b)) - offset
	}

	read := copy(dest, b[offset:offset+n])
	return read, nil
}

// streamRange provides handy blob calculations for a requested stream range.
type streamRange struct {
	FirstChunkIdx    int64 // index of chunk that contains start of range
	LastChunkIdx     int64 // index of chunk that contains end of range
	FirstChunkOffset int64 // offset from start of first chunk to start of range
	LastChunkReadLen int64 // number of bytes read from the last chunk that's read
	LastChunkOffset  int64 // offset from start of last chunk to end of range (only set if the whole range is in a single chunk)
}

// getRange returns a streamRange for the given start offset and reader buffer length.
func getRange(offset int64, readLen int) streamRange {
	r := streamRange{}

	rangeStartBytes := offset
	rangeEndBytes := offset + int64(readLen)

	r.FirstChunkIdx = rangeStartBytes / MaxChunkSize
	r.LastChunkIdx = rangeEndBytes / MaxChunkSize
	r.FirstChunkOffset = offset - r.FirstChunkIdx*MaxChunkSize
	if r.FirstChunkIdx == r.LastChunkIdx {
		r.LastChunkOffset = offset - r.LastChunkIdx*MaxChunkSize
	}
	r.LastChunkReadLen = rangeEndBytes - r.LastChunkIdx*MaxChunkSize - r.LastChunkOffset

	return r
}

func (r streamRange) String() string {
	return fmt.Sprintf("B%v[%v:]-B%v[%v:%v]", r.FirstChunkIdx, r.FirstChunkOffset, r.LastChunkIdx, r.LastChunkOffset, r.LastChunkReadLen)
}

func (r streamRange) ByteRangeForChunk(i int64) (int64, int64) {
	switch {
	case i == r.FirstChunkIdx:
		return r.FirstChunkOffset, MaxChunkSize - r.FirstChunkOffset
	case i == r.LastChunkIdx:
		return r.LastChunkOffset, r.LastChunkReadLen
	case r.FirstChunkIdx == r.LastChunkIdx:
		return r.FirstChunkOffset, r.LastChunkReadLen
	default:
		// Andrey says this never happens because apparently http.ServeContent never reads deeper
		// than 32KB from Stream.Read so the case when i is in the middle just doesn't happen
		return 0, 0
	}
}

// chunkGetter is an object for retrieving blobs from BlobStore or optionally from local cache.
type chunkGetter struct {
	hotCache *HotCache
	sdBlob   *stream.SDBlob
	prefetch bool
}

// Get returns a Blob object that can be Read() from.
func (b *chunkGetter) Get(n int) (ReadableChunk, error) {
	if n > len(b.sdBlob.BlobInfos) {
		return nil, errors.New("blob index out of bounds")
	}

	bi := b.sdBlob.BlobInfos[n]
	hash := hex.EncodeToString(bi.BlobHash)

	chunk, err := b.hotCache.GetChunk(hash, b.sdBlob.Key, bi.IV)
	if err != nil || chunk == nil {
		return nil, err
	}

	if b.prefetch {
		go b.prefetchToCache(n + 1)
	}
	return chunk, nil
}

func (b *chunkGetter) prefetchToCache(chunkIdx int) {
	prefetchLen := DefaultPrefetchLen
	chunksLeft := len(b.sdBlob.BlobInfos) - chunkIdx - 1 // Last blob is empty
	if chunksLeft < DefaultPrefetchLen {
		prefetchLen = chunksLeft
	}
	if prefetchLen <= 0 {
		return
	}

	Logger.Debugf("prefetching %v chunks to local cache", prefetchLen)
	for _, bi := range b.sdBlob.BlobInfos[chunkIdx : chunkIdx+prefetchLen] {
		hash := hex.EncodeToString(bi.BlobHash)

		if !b.hotCache.IsChunkCached(hash) {
			Logger.Debugf("chunk %v found in cache, not prefetching", hash)
			continue
		}

		Logger.Debugf("prefetching chunk %v", hash)
		_, err := b.hotCache.GetChunk(hash, b.sdBlob.Key, bi.IV)
		if err != nil {
			Logger.Errorf("failed to prefetch chunk %v: %v", hash, err)
			return
		}
	}
}
