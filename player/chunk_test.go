package player

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBlobCalculator(t *testing.T) {
	type testCase struct {
		offset   int64
		readLen  int
		expected streamRange
	}

	testCases := []testCase{
		{0, 512, streamRange{158433824, 0, 0, 0, 0, 512, 0}},
		{2450019, 64000, streamRange{158433824, 2450019, 1, 1, 352867 + 1, 64000, 352868}},
		{128791089, 99, streamRange{128791189, 128791089, 61, 61, 864817 + 61, 99, 864878}},
		{0, 128791189, streamRange{0, 128791189, 0, 61, 0, 864978, 0}},
		{2097149, 43, streamRange{2097149, 43, 0, 1, 2097149, 41, 0}},
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
