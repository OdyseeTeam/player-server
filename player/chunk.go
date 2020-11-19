package player

import (
	"fmt"

	"github.com/lbryio/lbry.go/v2/stream"
)

const (
	// MaxChunkSize is the max size of decrypted blob.
	MaxChunkSize = stream.MaxBlobSize - 1

	// DefaultPrefetchLen is how many blobs we should prefetch ahead.
	// 3 should be enough to deliver 2 x 4 = 8MB/s streams.
	// however since we can't keep up, let's see if 1 works
	DefaultPrefetchLen = 1
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

// Size returns the chunk size. Used by ccache to track size of cache
// DISABLED FOR NOW. we're making some guesses in cmd/main.go instead
//func (b ReadableChunk) Size() int64 {
//	return int64(len(b))
//}

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
