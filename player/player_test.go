package player

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	ljsonrpc "github.com/lbryio/lbry.go/v2/extras/jsonrpc"
	"github.com/lbryio/reflector.go/store"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ybbus/jsonrpc"
)

// An MP4 file, size: 158433824 bytes, blobs: 77
const claimID = "6769855a9aa43b67086f9ff3c1a5bacb5698a27a"

// An MP4 file, size: 128791189 bytes, blobs: 63
const knownSizeClaimID = "0590f924bbee6627a2e79f7f2ff7dfb50bf2877c"

type knownStream struct {
	uri      string
	size     int64
	blobsNum int
}

var knownStreams = []knownStream{
	{uri: claimID, size: 158433824, blobsNum: 77},
	{uri: knownSizeClaimID, size: 128791189, blobsNum: 63},
}

func randomString(n int) string {
	var letter = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

	b := make([]rune, n)
	for i := range b {
		b[i] = letter[rand.Intn(len(letter))]
	}
	return string(b)
}

func getTestPlayer() *Player {
	origin := store.NewHttpStore("source.odycdn.com:5569", "")
	ds := NewDecryptedCache(origin)
	return NewPlayer(
		NewHotCache(*ds, 100000000),
		WithDownloads(true),
		WithEdgeToken(testEdgeToken),
	)
}

func loadResponseFixture(t *testing.T, f string) jsonrpc.RPCResponse {
	var r jsonrpc.RPCResponse

	absPath, _ := filepath.Abs(filepath.Join("./testdata", f))
	rawJSON, err := ioutil.ReadFile(absPath)
	require.NoError(t, err)
	err = json.Unmarshal(rawJSON, &r)
	require.NoError(t, err)
	return r
}

func TestPlayerResolveStream(t *testing.T) {
	p := getTestPlayer()
	s, err := p.ResolveStream("389ba57c9f76b859c2763c4b9a419bd78b1a8dd0")
	require.NoError(t, err)
	err = s.PrepareForReading()
	require.NoError(t, err)
}

func TestPlayerResolveStreamNotFound(t *testing.T) {
	p := getTestPlayer()
	s, err := p.ResolveStream(randomString(20))
	assert.Equal(t, ErrClaimNotFound, err)
	assert.Nil(t, s)
}

func TestStreamSeek(t *testing.T) {
	p := getTestPlayer()

	for _, stream := range knownStreams {
		s, err := p.ResolveStream(stream.uri)
		require.NoError(t, err)
		err = s.PrepareForReading()
		require.NoError(t, err)

		// Seeking to the end
		n, err := s.Seek(0, io.SeekEnd)
		require.NoError(t, err)
		assert.EqualValues(t, stream.size, n)

		// Seeking to the middle of the stream
		n, err = s.Seek(stream.size/2, io.SeekStart)
		require.NoError(t, err)
		assert.EqualValues(t, stream.size/2, n)

		// Seeking back to the beginning of the stream
		n, err = s.Seek(-stream.size/2, io.SeekCurrent)
		require.NoError(t, err)
		assert.EqualValues(t, 0, n)

		n, err = s.Seek(0, io.SeekStart)
		require.NoError(t, err)
		assert.EqualValues(t, 0, n)

		s.Seek(0, io.SeekEnd)
		n, err = s.Seek(-999999999, io.SeekEnd)
		assert.EqualValues(t, 0, n)
		assert.Equal(t, ErrSeekOutOfBounds, err)

		n, err = s.Seek(-99, io.SeekStart)
		assert.EqualValues(t, 0, n)
		assert.Equal(t, ErrSeekBeforeStart, err)

		n, err = s.Seek(999999999, io.SeekStart)
		assert.EqualValues(t, 0, n)
		assert.Equal(t, ErrSeekOutOfBounds, err)
	}
}

func TestStreamRead(t *testing.T) {
	p := getTestPlayer()
	s, err := p.ResolveStream(claimID)
	require.NoError(t, err)

	err = s.PrepareForReading()
	require.NoError(t, err)

	n, err := s.Seek(4000000, io.SeekStart)
	require.NoError(t, err)
	require.EqualValues(t, 4000000, n)

	readData := make([]byte, 105)
	readNum, err := s.Read(readData)
	require.NoError(t, err)
	assert.Equal(t, 105, readNum)
	expectedData, err := hex.DecodeString(
		"6E81C93A90DD3A322190C8D608E29AA929867407596665097B5AE780412" +
			"61638A51C10BC26770AFFEF1533715FBD1428DCADEDC7BEA5D7A9C7D170" +
			"B71EF38E7138D24B0C7E86D791695EDAE1B88EDBE54F95C98EF3DCFD91D" +
			"A025C284EE37D8FEEA2EA84B76B9A22D3")
	require.NoError(t, err)
	assert.Equal(t, expectedData, readData)
}

func TestStreamFilenameOldMime(t *testing.T) {
	r := loadResponseFixture(t, "old_mime.json")
	res := &ljsonrpc.ResolveResponse{}
	ljsonrpc.Decode(r.Result, res)
	uri := "lbry://@Deterrence-Dispensed#2/Ivans100DIY30rdAR-15MagazineV10-DeterrenceDispensed#1"
	claim := (*res)[uri]
	s := NewStream(&Player{}, &claim)
	assert.Equal(t, "ivans100diy30rdar-15magazinev10-deterrencedispensed.zip", s.Filename())
}

func TestStreamFilenameNew(t *testing.T) {
	r := loadResponseFixture(t, "new_stream.json")
	res := &ljsonrpc.ResolveResponse{}
	ljsonrpc.Decode(r.Result, res)
	uri := "what"
	claim := (*res)[uri]
	s := NewStream(&Player{}, &claim)
	assert.Equal(t, "1 dog and chicken.mp4", s.Filename())
}

func TestStreamReadHotCache(t *testing.T) {
	p := getTestPlayer()

	s1, err := p.ResolveStream(claimID)
	require.NoError(t, err)

	assert.EqualValues(t, 0, p.blobSource.cache.Len(false))

	err = s1.PrepareForReading()
	require.NoError(t, err)

	assert.EqualValues(t, 2, p.blobSource.cache.Len(false)) // 2 because it gets the sd blob and the last blob when setting stream size

	// Warm up the cache
	n, err := s1.Seek(4000000, io.SeekStart)
	require.NoError(t, err)
	require.EqualValues(t, 4000000, n)

	readData := make([]byte, 105)
	readNum, err := s1.Read(readData)
	require.NoError(t, err)
	assert.Equal(t, 105, readNum)

	assert.EqualValues(t, 3, p.blobSource.cache.Len(false))

	// Re-get the stream

	s2, err := p.ResolveStream(claimID)
	require.NoError(t, err)

	err = s2.PrepareForReading()
	require.NoError(t, err)

	assert.EqualValues(t, 3, p.blobSource.cache.Len(false))

	for i := 0; i < 2; i++ {
		n, err := s2.Seek(4000000, io.SeekStart)
		require.NoError(t, err)
		require.EqualValues(t, 4000000, n)

		readData := make([]byte, 105)
		readNum, err := s2.Read(readData)
		require.NoError(t, err)
		assert.Equal(t, 105, readNum)
		expectedData, err := hex.DecodeString(
			"6E81C93A90DD3A322190C8D608E29AA929867407596665097B5AE780412" +
				"61638A51C10BC26770AFFEF1533715FBD1428DCADEDC7BEA5D7A9C7D170" +
				"B71EF38E7138D24B0C7E86D791695EDAE1B88EDBE54F95C98EF3DCFD91D" +
				"A025C284EE37D8FEEA2EA84B76B9A22D3")
		require.NoError(t, err)
		assert.Equal(t, expectedData, readData)
	}

	// no new blobs should have been fetched because they are all cached
	assert.EqualValues(t, 3, p.blobSource.cache.Len(false))

	n, err = s2.Seek(2000000, io.SeekCurrent)
	require.NoError(t, err)
	require.EqualValues(t, 6000105, n)

	readData = make([]byte, 105)
	readNum, err = s2.Read(readData)
	require.NoError(t, err)
	assert.Equal(t, 105, readNum)
	require.NoError(t, err)

	assert.EqualValues(t, 4, p.blobSource.cache.Len(false))
}

func TestStreamReadOutOfBounds(t *testing.T) {
	p := getTestPlayer()
	s, err := p.ResolveStream(claimID)
	require.NoError(t, err)

	err = s.PrepareForReading()
	require.NoError(t, err)

	n, err := s.Seek(4000000, io.SeekStart)
	require.NoError(t, err)
	require.EqualValues(t, 4000000, n)

	readData := make([]byte, 105)
	readNum, err := s.Read(readData)
	require.NoError(t, err)
	assert.Equal(t, 105, readNum)
	expectedData, err := hex.DecodeString(
		"6E81C93A90DD3A322190C8D608E29AA929867407596665097B5AE780412" +
			"61638A51C10BC26770AFFEF1533715FBD1428DCADEDC7BEA5D7A9C7D170" +
			"B71EF38E7138D24B0C7E86D791695EDAE1B88EDBE54F95C98EF3DCFD91D" +
			"A025C284EE37D8FEEA2EA84B76B9A22D3")
	require.NoError(t, err)
	assert.Equal(t, expectedData, readData)
}

func TestVerifyAccess(t *testing.T) {
	restrictedUrls := []string{
		"lbry://@gifprofile#7/rental1#8",
		"lbry://@gifprofile#7/purchase1#2",
		"lbry://@gifprofile#7/members-only#7",
	}

	gin.SetMode(gin.TestMode)
	p := getTestPlayer()

	for _, url := range restrictedUrls {
		t.Run(url, func(t *testing.T) {
			ctx, e := gin.CreateTestContext(httptest.NewRecorder())
			req, _ := http.NewRequest(http.MethodGet, "", nil)
			ctx.Request = req
			e.HandleContext(ctx)

			s, err := p.ResolveStream(url)
			require.NoError(t, err)
			err = p.VerifyAccess(s, ctx)
			require.ErrorIs(t, err, ErrEdgeCredentialsMissing)
		})
	}

	for _, url := range restrictedUrls {
		t.Run(url, func(t *testing.T) {
			ctx, e := gin.CreateTestContext(httptest.NewRecorder())
			req, _ := http.NewRequest(http.MethodGet, "", nil)
			req.Header.Set(edgeTokenHeader, testEdgeToken)
			ctx.Request = req
			e.HandleContext(ctx)

			s, err := p.ResolveStream(url)
			require.NoError(t, err)
			err = p.VerifyAccess(s, ctx)
			require.NoError(t, err)
		})
	}
}
