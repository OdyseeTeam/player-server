package player

import (
	"encoding/hex"
	"errors"
	"io"
	"math"
	"mime"
	"time"

	"github.com/lbryio/lbrytv-player/internal/metrics"

	ljsonrpc "github.com/lbryio/lbry.go/v2/extras/jsonrpc"
	"github.com/lbryio/lbry.go/v2/stream"
	pb "github.com/lbryio/types/v2/go"
)

// Stream provides an io.ReadSeeker interface to a stream of blobs to be used by standard http library for range requests,
// as well as some stream metadata.
type Stream struct {
	URI         string
	Size        uint64
	ContentType string
	hash        string

	player         *Player
	claim          *ljsonrpc.Claim
	source         *pb.Source
	resolvedStream *pb.Stream
	sdBlob         *stream.SDBlob
	seekOffset     int64
}

func NewStream(p *Player, uri string, claim *ljsonrpc.Claim) *Stream {
	stream := claim.Value.GetStream()
	source := stream.GetSource()
	return &Stream{
		URI:         uri,
		ContentType: patchMediaType(source.MediaType),
		Size:        source.GetSize(),

		player:         p,
		claim:          claim,
		source:         source,
		resolvedStream: stream,
		hash:           hex.EncodeToString(source.SdHash),
	}
}

// Filename detects name of the original file, suitable for saving under on the filesystem.
func (s *Stream) Filename() string {
	name := s.source.GetName()
	if name != "" {
		return name
	}
	name = s.claim.NormalizedName
	exts, err := mime.ExtensionsByType(s.ContentType)
	if err != nil {
		return name
	}
	return name + exts[0]
}

// PrepareForReading downloads stream description from the reflector and tries to determine stream size
// using several methods, including legacy ones for streams that do not have metadata.
func (s *Stream) PrepareForReading() error {
	sdBlob, err := s.player.blobSource.GetSDBlob(s.hash)
	if err != nil {
		return err
	}

	s.sdBlob = &sdBlob

	s.setSize(sdBlob.BlobInfos)

	return nil
}

func (s *Stream) setSize(blobs []stream.BlobInfo) {
	if s.Size > 0 {
		return
	}

	if s.source.GetSize() > 0 {
		s.Size = s.source.GetSize()
	}

	size, err := s.claim.GetStreamSizeByMagic()

	if err != nil {
		Logger.Infof("couldn't figure out stream %v size by magic: %v", s.URI, err)
		for _, blob := range blobs {
			if blob.Length == stream.MaxBlobSize {
				size += MaxChunkSize
			} else {
				size += uint64(blob.Length - 1)
			}
		}
		// last padding is unguessable
		size -= 16
	}

	s.Size = size
}

// Timestamp returns stream creation timestamp, used in HTTP response header.
func (s *Stream) Timestamp() time.Time {
	return time.Unix(int64(s.claim.Timestamp), 0)
}

// Seek implements io.ReadSeeker interface and is meant to be called by http.ServeContent.
func (s *Stream) Seek(offset int64, whence int) (int64, error) {
	var newOffset int64

	if s.Size == 0 {
		return 0, errStreamSizeZero
	} else if uint64(math.Abs(float64(offset))) > s.Size {
		return 0, errOutOfBounds
	}

	switch whence {
	case io.SeekStart:
		newOffset = offset
	case io.SeekCurrent:
		newOffset = s.seekOffset + offset
	case io.SeekEnd:
		newOffset = int64(s.Size) - offset
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

func (s *Stream) readFromChunks(sr streamRange, dest []byte) (int, error) {
	var read int

	for i := sr.FirstChunkIdx; i < sr.LastChunkIdx+1; i++ {
		offset, readLen := sr.ByteRangeForChunk(i)

		b, err := s.GetChunk(int(i))
		if err != nil {
			return read, err
		}

		n, err := b.Read(offset, readLen, dest[read:])
		read += n
		if err != nil {
			return read, err
		}
	}

	return read, nil
}

// GetChunk returns the nth ReadableChunk of the stream.
func (s *Stream) GetChunk(chunkIdx int) (ReadableChunk, error) {
	if chunkIdx > len(s.sdBlob.BlobInfos) {
		return nil, errors.New("blob index out of bounds")
	}

	bi := s.sdBlob.BlobInfos[chunkIdx]
	hash := hex.EncodeToString(bi.BlobHash)

	chunk, err := s.player.blobSource.GetChunk(hash, s.sdBlob.Key, bi.IV)
	if err != nil || chunk == nil {
		return nil, err
	}

	if s.player.prefetch {
		go s.prefetchChunk(chunkIdx + 1)
	}
	return chunk, nil
}

func (s *Stream) prefetchChunk(chunkIdx int) {
	prefetchLen := DefaultPrefetchLen
	chunksLeft := len(s.sdBlob.BlobInfos) - chunkIdx - 1 // Last blob is empty
	if chunksLeft < DefaultPrefetchLen {
		prefetchLen = chunksLeft
	}
	if prefetchLen <= 0 {
		return
	}

	Logger.Debugf("prefetching %v chunks to local cache", prefetchLen)
	for _, bi := range s.sdBlob.BlobInfos[chunkIdx : chunkIdx+prefetchLen] {
		hash := hex.EncodeToString(bi.BlobHash)

		if !s.player.blobSource.IsCached(hash) {
			Logger.Debugf("chunk %v found in cache, not prefetching", hash)
			continue
		}

		Logger.Debugf("prefetching chunk %v", hash)
		_, err := s.player.blobSource.GetChunk(hash, s.sdBlob.Key, bi.IV)
		if err != nil {
			Logger.Errorf("failed to prefetch chunk %v: %v", hash, err)
			return
		}
	}
}
