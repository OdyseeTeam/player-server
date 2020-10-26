package player

import (
	"errors"
	"io"
	"math"
	"time"

	"github.com/lbryio/lbrytv-player/internal/metrics"

	ljsonrpc "github.com/lbryio/lbry.go/v2/extras/jsonrpc"
	"github.com/lbryio/lbry.go/v2/stream"
	pb "github.com/lbryio/types/v2/go"
)

// Stream provides an io.ReadSeeker interface to a stream of blobs to be used by standard http library for range requests,
// as well as some stream metadata.
type Stream struct {
	URI            string
	Hash           string
	sdBlob         *stream.SDBlob
	Size           int64
	ContentType    string
	Claim          *ljsonrpc.Claim
	seekOffset     int64
	chunkGetter    chunkGetter
	resolvedStream *pb.Stream
}

func (s *Stream) setSize(blobs *[]stream.BlobInfo) {
	if s.Size > 0 {
		return
	}

	size, err := s.Claim.GetStreamSizeByMagic()

	if err != nil {
		Logger.Infof("couldn't figure out stream %v size by magic: %v", s.URI, err)
		for _, blob := range *blobs {
			if blob.Length == stream.MaxBlobSize {
				size += ChunkSize
			} else {
				size += uint64(blob.Length - 1)
			}
		}
		// last padding is unguessable
		size -= 16
	}

	s.Size = int64(size)
}

// Timestamp returns stream creation timestamp, used in HTTP response header.
func (s *Stream) Timestamp() time.Time {
	return time.Unix(int64(s.Claim.Timestamp), 0)
}

// Seek implements io.ReadSeeker interface and is meant to be called by http.ServeContent.
func (s *Stream) Seek(offset int64, whence int) (int64, error) {
	var newOffset int64

	if s.Size == 0 {
		return 0, errStreamSizeZero
	} else if int64(math.Abs(float64(offset))) > s.Size {
		return 0, errOutOfBounds
	}

	switch whence {
	case io.SeekStart:
		newOffset = offset
	case io.SeekCurrent:
		newOffset = s.seekOffset + offset
	case io.SeekEnd:
		newOffset = s.Size - offset
	default:
		return 0, errors.New("invalid seek whence argument")
	}

	if newOffset < 0 {
		return 0, errSeekingBeforeStart
	}

	s.seekOffset = newOffset
	return newOffset, nil
}

// Read implements io.ReadSeeker interface and is meant to be called by http.ServeContent.
// Actual chunk retrieval and delivery happens in s.readFromChunks().
func (s *Stream) Read(dest []byte) (n int, err error) {
	n, err = s.readFromChunks(getRange(s.seekOffset, len(dest)), dest)
	s.seekOffset += int64(n)

	metrics.OutBytes.Add(float64(n))

	if err != nil {
		Logger.Errorf("failed to read from stream %v at offset %v: %v", s.URI, s.seekOffset, err)
	}

	return n, err
}

func (s *Stream) readFromChunks(calc streamRange, dest []byte) (int, error) {
	var b ReadableChunk
	var err error
	var read int

	for i := calc.FirstChunkIdx; i < calc.LastChunkIdx+1; i++ {
		var start, readLen int64

		if i == calc.FirstChunkIdx {
			start = calc.FirstChunkOffset
			readLen = ChunkSize - calc.FirstChunkOffset
		} else if i == calc.LastChunkIdx {
			start = calc.LastChunkOffset
			readLen = calc.LastChunkReadLen
		} else if calc.FirstChunkIdx == calc.LastChunkIdx {
			start = calc.FirstChunkOffset
			readLen = calc.LastChunkReadLen
		}

		b, err = s.chunkGetter.Get(int(i))
		if err != nil {
			return read, err
		}

		n, err := b.Read(start, readLen, dest[read:])
		read += n
		if err != nil {
			return read, err
		}
	}

	return read, nil
}
