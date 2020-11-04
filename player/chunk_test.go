package player

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStreamRange(t *testing.T) {
	type testCase struct {
		offset   int64
		readLen  int
		expected streamRange
	}

	// size: 128791189, has blobs: 62 + padding, last blob index: 61
	testCases := []testCase{
		// read 512 bytes from the start of a single chunk
		{0, 512, streamRange{0, 0, 0, 512, 0}},
		// read 64KiB from the middle of the 2nd chunk
		{2450019, 64000, streamRange{1, 1, 352867 + 1, 64000, 352868}},
		// read 99 bytes from the middle of the 61st chunk
		{128791089, 99, streamRange{61, 61, 864817 + 61, 99, 864878}},
		// read across 61 chunks, ending mid-chunk.
		{0, 128791189, streamRange{0, 61, 0, 864978, 0}},
		// read 43 bytes across one chunk boundary
		{2097149, 43, streamRange{0, 1, 2097149, 41, 0}},
	}

	for n, row := range testCases {
		t.Run(fmt.Sprintf("row:%v", n), func(t *testing.T) {
			bc := getRange(row.offset, row.readLen)
			assert.Equal(t, row.expected.FirstChunkIdx, bc.FirstChunkIdx)
			assert.Equal(t, row.expected.LastChunkIdx, bc.LastChunkIdx)
			assert.Equal(t, row.expected.FirstChunkOffset, bc.FirstChunkOffset)
			assert.Equal(t, row.expected.LastChunkReadLen, bc.LastChunkReadLen)
			assert.Equal(t, row.expected.LastChunkOffset, bc.LastChunkOffset)
		})
	}
}
