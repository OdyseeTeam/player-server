package player

import (
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/OdyseeTeam/player-server/pkg/paid"
	"github.com/Pallinder/go-randomdata"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type rangeHeader struct {
	start, end, knownLen int
}

func makeRequest(t *testing.T, router *gin.Engine, method, uri string, rng *rangeHeader) *http.Response {
	if router == nil {
		router = gin.New()
		InstallPlayerRoutes(router, getTestPlayer())
	}

	r, err := http.NewRequest(method, uri, nil)
	require.NoError(t, err)
	r.Header.Add("Referer", "https://odysee.com")
	if rng != nil {
		if rng.start == 0 {
			r.Header.Add("Range", fmt.Sprintf("bytes=0-%v", rng.end))
		} else if rng.end == 0 {
			r.Header.Add("Range", fmt.Sprintf("bytes=%v-", rng.start))
		} else {
			r.Header.Add("Range", fmt.Sprintf("bytes=%v-%v", rng.start, rng.end))
		}
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, r)
	return rr.Result()
}

func TestHandleGet(t *testing.T) {
	player := getTestPlayer()
	router := gin.New()
	router.Any("/content/claims/:claim_name/:claim_id/:filename", NewRequestHandler(player).Handle)

	type testInput struct {
		name, uri string
		rng       *rangeHeader
	}
	type testCase struct {
		input  testInput
		output string
	}
	testCases := []testCase{
		{
			testInput{"MiddleBytes", "/content/claims/what/6769855a9aa43b67086f9ff3c1a5bacb5698a27a/stream.mp4", &rangeHeader{start: 156, end: 259}},
			"00000001D39A07E8D39A07E80000000100000000008977680000" +
				"0000000000000000000000000000000100000000000000000000" +
				"0000000000010000000000000000000000000000400000000780" +
				"00000438000000000024656474730000001C656C737400000000",
		},
		{
			testInput{"FirstBytes", "/content/claims/what/6769855a9aa43b67086f9ff3c1a5bacb5698a27a/stream.mp4", &rangeHeader{start: 0, end: 52}},
			"00000018667479706D703432000000006D7034326D7034310000" +
				"C4EA6D6F6F760000006C6D76686400000000D39A07E8D39A07F200",
		},
		{
			testInput{"BytesFromSecondBlob", "/content/claims/what/6769855a9aa43b67086f9ff3c1a5bacb5698a27a/stream.mp4", &rangeHeader{start: 4000000, end: 4000104}},
			"6E81C93A90DD3A322190C8D608E29AA929867407596665097B5AE780412" +
				"61638A51C10BC26770AFFEF1533715FBD1428DCADEDC7BEA5D7A9C7D170" +
				"B71EF38E7138D24B0C7E86D791695EDAE1B88EDBE54F95C98EF3DCFD91D" +
				"A025C284EE37D8FEEA2EA84B76B9A22D3",
		},
		{
			testInput{"LastBytes", "/content/claims/known-size/0590f924bbee6627a2e79f7f2ff7dfb50bf2877c/stream", &rangeHeader{start: 128791089, knownLen: 100}},
			"2505CA36CB47B0B14CA023203410E965657B6314F6005D51E992D073B8090419D49E28E99306C95CF2DDB9" +
				"51DA5FE6373AC542CC2D83EB129548FFA0B4FFE390EB56600AD72F0D517236140425E323FDFC649FDEB80F" +
				"A429227D149FD493FBCA2042141F",
		},
		{
			testInput{"BetweenBlobs", "/content/claims/known-size/0590f924bbee6627a2e79f7f2ff7dfb50bf2877c/stream",
				&rangeHeader{start: 2097149, end: 2097191}},
			"6BD50FA7383B3760C5CE5DFC2F73BD5EE7D3591C986758A5E43D8F3712A59861898F349BC0FA25CDED91DB",
		},
		{
			testInput{"SecondBLob", "/content/claims/known-size/0590f924bbee6627a2e79f7f2ff7dfb50bf2877c/stream",
				&rangeHeader{start: 2097151, end: 2097191}},
			"0FA7383B3760C5CE5DFC2F73BD5EE7D3591C986758A5E43D8F3712A59861898F349BC0FA25CDED91DB",
		},
	}

	for _, row := range testCases {
		t.Run(row.input.name, func(t *testing.T) {
			var expectedLen int
			response := makeRequest(t, router, http.MethodGet, row.input.uri, row.input.rng)

			if row.input.rng.knownLen > 0 {
				expectedLen = row.input.rng.knownLen
			} else {
				expectedLen = row.input.rng.end - row.input.rng.start + 1
			}
			require.Equal(t, http.StatusPartialContent, response.StatusCode)
			assert.Equal(t, fmt.Sprintf("%v", expectedLen), response.Header.Get("Content-Length"))
			assert.Equal(t, "bytes", response.Header.Get("Accept-Ranges"))
			assert.Equal(t, "video/mp4", response.Header.Get("Content-Type"))
			assert.Equal(t, "", response.Header.Get("Content-Disposition"))
			assert.Equal(t, "public, max-age=31536000", response.Header.Get("Cache-Control"))

			responseStream := make([]byte, expectedLen)
			_, err := response.Body.Read(responseStream)
			require.NoError(t, err)
			assert.Equal(t, strings.ToLower(row.output), hex.EncodeToString(responseStream))
		})
	}
}

func TestHandleUnpaid(t *testing.T) {
	response := makeRequest(t, nil, http.MethodGet, "/content/claims/iOS-13-AdobeXD/9cd2e93bfc752dd6560e43623f36d0c3504dbca6/stream.mp4", nil)
	assert.Equal(t, http.StatusPaymentRequired, response.StatusCode)
}

func TestHandleHead(t *testing.T) {
	response := makeRequest(t, nil, http.MethodHead, "/content/claims/what/6769855a9aa43b67086f9ff3c1a5bacb5698a27a/stream.mp4", nil)

	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "video/mp4", response.Header.Get("Content-Type"))
	assert.Equal(t, "Fri, 17 Nov 2017 17:19:50 GMT", response.Header.Get("Last-Modified"))
	assert.Equal(t, "158433824", response.Header.Get("Content-Length"))
}

func TestHandleHeadErrors(t *testing.T) {
	r := makeRequest(t, nil, http.MethodHead, "/content/claims/completely/ef/stream", nil)
	require.Equal(t, http.StatusNotFound, r.StatusCode)
}

func TestHandleNotFound(t *testing.T) {
	r := makeRequest(t, nil, http.MethodGet, "/content/claims/completely/ef/stream", nil)
	require.Equal(t, http.StatusNotFound, r.StatusCode)
}

func TestHandleOutOfBounds(t *testing.T) {
	r := makeRequest(t, nil, http.MethodGet, "/content/claims/known-size/0590f924bbee6627a2e79f7f2ff7dfb50bf2877c/stream", &rangeHeader{start: 999999999})

	require.Equal(t, http.StatusRequestedRangeNotSatisfiable, r.StatusCode)
}

func TestHandleDownloadableFile(t *testing.T) {
	checkRequest := func(r *http.Response) {
		assert.Equal(t, http.StatusOK, r.StatusCode)
		assert.Equal(t, `attachment; filename="861382668_228248581_tenor.gif"; filename*=UTF-8''861382668_228248581_tenor.gif`, r.Header.Get("Content-Disposition"))
		assert.Equal(t, "8722934", r.Header.Get("Content-Length"))
	}

	testCases := []struct {
		url, method string
	}{
		{
			"/content/claims/scalable-test2/0a15a743ac078a83a02cc086fbb8b566e912b7c5/stream?download=true",
			http.MethodGet,
		},
		{
			"/content/claims/scalable-test2/0a15a743ac078a83a02cc086fbb8b566e912b7c5/stream?download=true",
			http.MethodHead,
		},
		{
			"/content/claims/scalable-test2/0a15a743ac078a83a02cc086fbb8b566e912b7c5/stream?download=1",
			http.MethodGet,
		},
		{
			"/content/claims/scalable-test2/0a15a743ac078a83a02cc086fbb8b566e912b7c5/stream?download=1",
			http.MethodHead,
		},
	}
	for _, c := range testCases {
		r := makeRequest(t, nil, c.method, c.url, nil)
		checkRequest(r)
	}
}

func TestUTF8Filename(t *testing.T) {
	_ = ` 【大苑子APP宣傳影片】分享新鮮＿精彩生活-20181106.mp4` // original filename, just for reference
	r := makeRequest(t, nil, http.MethodHead, "/content/claims/"+url.PathEscape(`-【大苑子APP宣傳影片】分享新鮮＿精彩生活-20181106`)+"/e9bbe7a0ffe8bb1070ffe41b342e93b054641b6c/stream?download=1", nil)
	assert.Equal(t, http.StatusOK, r.StatusCode)
	assert.Equal(t, `attachment; filename=" 大苑子APP宣傳影片分享新鮮精彩生活-20181106.mp4"; filename*=UTF-8''%20%E5%A4%A7%E8%8B%91%E5%AD%90APP%E5%AE%A3%E5%82%B3%E5%BD%B1%E7%89%87%E5%88%86%E4%BA%AB%E6%96%B0%E9%AE%AE%E7%B2%BE%E5%BD%A9%E7%94%9F%E6%B4%BB-20181106.mp4`, r.Header.Get("Content-Disposition"))
	assert.Equal(t, "294208625", r.Header.Get("Content-Length"))

	_ = `"Bitcoin je scam" - informujú média.mp4` // original filename, just for reference
	r = makeRequest(t, nil, http.MethodHead, "/content/claims/"+url.PathEscape(`-Bitcoin-je-scam----informujú-média`)+"/554c23406b0821c5e2a101ea0e865e35948b632c/stream?download=1", nil)
	assert.Equal(t, http.StatusOK, r.StatusCode)
	assert.Equal(t, `attachment; filename="Bitcoin je scam - informuju media.mp4"; filename*=UTF-8''Bitcoin%20je%20scam%20-%20informuju%20media.mp4`, r.Header.Get("Content-Disposition"))
	assert.Equal(t, "504872011", r.Header.Get("Content-Length"))
}

func TestHandleHeadStreamsV2(t *testing.T) {
	r, err := http.Get("https://api.na-backend.odysee.com/api/v1/paid/pubkey")
	require.NoError(t, err)
	rawKey, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	err = paid.InitPubKey(rawKey)
	require.NoError(t, err)

	r = makeRequest(t, nil, http.MethodHead, "/api/v2/streams/paid/iOS-13-AdobeXD/9cd2e93bfc752dd6560e43623f36d0c3504dbca6/eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9", nil)
	body, _ := io.ReadAll(r.Body)
	assert.Equal(t, http.StatusUnauthorized, r.StatusCode, string(body))

	r = makeRequest(t, nil, http.MethodHead, "/api/v2/streams/free/iOS-13-AdobeXD/9cd2e93bfc752dd6560e43623f36d0c3504dbca6", nil)
	body, _ = io.ReadAll(r.Body)
	assert.Equal(t, http.StatusPaymentRequired, r.StatusCode, string(body))

	paid.GeneratePrivateKey()
	expiredToken, err := paid.CreateToken("iOS-13-AdobeXD/9cd2e93bfc752dd6560e43623f36d0c3504dbca6", "000", 120_000_000, func(uint64) int64 { return 1 })
	require.NoError(t, err)

	r = makeRequest(t, nil, http.MethodHead, "/api/v2/streams/paid/iOS-13-AdobeXD/9cd2e93bfc752dd6560e43623f36d0c3504dbca6/"+expiredToken, nil)
	body, _ = io.ReadAll(r.Body)
	assert.Equal(t, http.StatusGone, r.StatusCode, string(body))

	validToken, err := paid.CreateToken("iOS-13-AdobeXD/9cd2e93bfc752dd6560e43623f36d0c3504dbca6", "000", 120_000_000, paid.ExpTenSecPer100MB)
	require.NoError(t, err)

	r = makeRequest(t, nil, http.MethodHead, "/api/v2/streams/paid/iOS-13-AdobeXD/9cd2e93bfc752dd6560e43623f36d0c3504dbca6/"+validToken, nil)
	body, _ = io.ReadAll(r.Body)
	assert.Equal(t, http.StatusOK, r.StatusCode, string(body))
}

func TestHandleHeadStreamsV3(t *testing.T) {
	r, err := http.Get("https://api.na-backend.odysee.com/api/v1/paid/pubkey")
	require.NoError(t, err)
	rawKey, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	err = paid.InitPubKey(rawKey)
	require.NoError(t, err)

	r = makeRequest(t, nil, http.MethodHead, "/api/v3/streams/paid/iOS-13-AdobeXD/9cd2e93bfc752dd6560e43623f36d0c3504dbca6/abcdef/eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9", nil)
	body, _ := io.ReadAll(r.Body)
	assert.Equal(t, http.StatusUnauthorized, r.StatusCode, string(body))

	r = makeRequest(t, nil, http.MethodHead, "/api/v3/streams/free/iOS-13-AdobeXD/9cd2e93bfc752dd6560e43623f36d0c3504dbca6/abcdef", nil)
	body, _ = io.ReadAll(r.Body)
	assert.Equal(t, http.StatusPaymentRequired, r.StatusCode, string(body))

	paid.GeneratePrivateKey()
	expiredToken, err := paid.CreateToken("iOS-13-AdobeXD/9cd2e93bfc752dd6560e43623f36d0c3504dbca6", "000", 120_000_000, func(uint64) int64 { return 1 })
	require.NoError(t, err)

	r = makeRequest(t, nil, http.MethodHead, "/api/v3/streams/paid/iOS-13-AdobeXD/9cd2e93bfc752dd6560e43623f36d0c3504dbca6/abcdef/"+expiredToken, nil)
	body, _ = io.ReadAll(r.Body)
	assert.Equal(t, http.StatusGone, r.StatusCode, string(body))

	validToken, err := paid.CreateToken("iOS-13-AdobeXD/9cd2e93bfc752dd6560e43623f36d0c3504dbca6", "000", 120_000_000, paid.ExpTenSecPer100MB)
	require.NoError(t, err)

	r = makeRequest(t, nil, http.MethodHead, "/api/v3/streams/paid/iOS-13-AdobeXD/9cd2e93bfc752dd6560e43623f36d0c3504dbca6/abcdef/"+validToken, nil)
	body, _ = io.ReadAll(r.Body)
	assert.Equal(t, http.StatusOK, r.StatusCode, string(body))
}

func TestHandleSpeech(t *testing.T) {
	r := makeRequest(t, nil, http.MethodGet, "/speech/8b2d4d78e9c8120e:f.jpg", nil)
	assert.Equal(t, http.StatusOK, r.StatusCode)
	assert.Equal(t, "image/jpeg", r.Header.Get("content-type"))
}

func Test_getPlaylistURL(t *testing.T) {
	stream := &Stream{URL: "lbryStreamURL"}
	// This is the pattern transcoder client should return.
	tcURL := "claimID/SDhash/master.m3u8"

	t.Run("v4", func(t *testing.T) {
		assert.Equal(t,
			"/api/v4/streams/tc/lbryStreamURL/claimID/SDhash/master.m3u8",
			getPlaylistURL("/api/v4/streams/free/lbryStreamURL/claimID/SDhash/", url.Values{}, tcURL, stream),
		)
	})
	t.Run("v5", func(t *testing.T) {
		assert.Equal(t,
			"/v5/streams/hls/claimID/SDhash/master.m3u8",
			getPlaylistURL("/v5/streams/start/claimID/SDhash/", url.Values{}, tcURL, stream),
		)
	})
	t.Run("v5 signed", func(t *testing.T) {
		q := url.Values{}
		h := randomdata.Alphanumeric(32)
		ip := randomdata.IpV4Address()
		q.Add(paramHashHLS, h)
		q.Add(paramClientIP, ip)
		assert.Equal(t,
			fmt.Sprintf("/v5/streams/hls/claimID/SDhash/master.m3u8?ip=%s&hash=%s", ip, h),
			getPlaylistURL("/v5/streams/start/claimID/SDhash/", q, tcURL, stream),
		)
	})
	t.Run("v6", func(t *testing.T) {
		assert.Equal(t,
			"/v6/streams/claimID/SDhash/master.m3u8",
			getPlaylistURL("/v6/streams/claimID/SDhash/start", url.Values{}, tcURL, stream),
		)
	})
	t.Run("v6 with hash", func(t *testing.T) {
		q := url.Values{}
		h := "abc,89898"
		q.Add(paramHash77, h)
		assert.Equal(t,
			"/abc,89898/v6/streams/claimID/SDhash/master.m3u8",
			getPlaylistURL("/v6/streams/claimID/SDhash/start", q, tcURL, stream),
		)
	})
}

func Test_fitForTranscoder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var r *http.Request
	p := getTestPlayer()

	w := httptest.NewRecorder()
	c, e := gin.CreateTestContext(w)

	InstallPlayerRoutes(e, p)

	r, _ = http.NewRequest(http.MethodGet, "https://cdn.lbryplayer.xyz/api/v4/streams/free/claimname/abc/sdhash", nil)
	r.Header.Add("referer", "https://odysee.com/")
	c.Request = r
	e.HandleContext(c)

	s, err := p.ResolveStream("6769855a9aa43b67086f9ff3c1a5bacb5698a27a")
	require.NoError(t, err)
	assert.True(t, fitForTranscoder(c, s))

	r, _ = http.NewRequest(http.MethodGet, "https://cdn.lbryplayer.xyz/api/v4/streams/free/claimname/abc/sdhash", nil)
	c.Request = r
	e.HandleContext(c)
	s, err = p.ResolveStream("9cd2e93bfc752dd6560e43623f36d0c3504dbca6")
	require.NoError(t, err)
	assert.False(t, fitForTranscoder(c, s))

	r, _ = http.NewRequest(http.MethodGet, "https://cdn.lbryplayer.xyz/api/v4/streams/free/claimname/abc/sdhash", nil)
	c.Request = r
	e.HandleContext(c)
	r.Header.Add("Range", "bytes=12121-")
	s, err = p.ResolveStream("9cd2e93bfc752dd6560e43623f36d0c3504dbca6")
	require.NoError(t, err)
	assert.False(t, fitForTranscoder(c, s))

	r, _ = http.NewRequest(http.MethodGet, "https://cdn.lbryplayer.xyz/api/v3/streams/free/claimname/abc/sdhash", nil)
	c.Request = r
	e.HandleContext(c)
	s, err = p.ResolveStream("6769855a9aa43b67086f9ff3c1a5bacb5698a27a")
	require.NoError(t, err)
	assert.False(t, fitForTranscoder(c, s))

	r, _ = http.NewRequest(http.MethodGet, "https://cdn.lbryplayer.xyz/api/v5/streams/original/6769855a9aa43b67086f9ff3c1a5bacb5698a27a/abcabc", nil)
	c.Request = r
	e.HandleContext(c)
	s, err = p.ResolveStream("6769855a9aa43b67086f9ff3c1a5bacb5698a27a")
	require.NoError(t, err)
	assert.False(t, fitForTranscoder(c, s))

	r, _ = http.NewRequest(http.MethodGet, "https://cdn.lbryplayer.xyz/v5/streams/start/6769855a9aa43b67086f9ff3c1a5bacb5698a27a/abcabc", nil)
	c.Request = r
	e.HandleContext(c)
	s, err = p.ResolveStream("6769855a9aa43b67086f9ff3c1a5bacb5698a27a")
	require.NoError(t, err)
	assert.False(t, fitForTranscoder(c, s))

	r, _ = http.NewRequest(http.MethodHead, "https://cdn.lbryplayer.xyz/v5/streams/start/6769855a9aa43b67086f9ff3c1a5bacb5698a27a/abcabc", nil)
	c.Request = r
	e.HandleContext(c)
	s, err = p.ResolveStream("6769855a9aa43b67086f9ff3c1a5bacb5698a27a")
	require.NoError(t, err)
	assert.True(t, fitForTranscoder(c, s))

	r, _ = http.NewRequest(http.MethodHead, "https://cdn.lbryplayer.xyz/v5/streams/start/6769855a9aa43b67086f9ff3c1a5bacb5698a27a/abcabc", nil)
	c.Request = r
	r.Header.Add("Range", "bytes=0-13337")
	e.HandleContext(c)
	s, err = p.ResolveStream("6769855a9aa43b67086f9ff3c1a5bacb5698a27a")
	require.NoError(t, err)
	assert.True(t, fitForTranscoder(c, s))

	r, _ = http.NewRequest(http.MethodGet, "https://cdn.lbryplayer.xyz/v5/streams/original/6769855a9aa43b67086f9ff3c1a5bacb5698a27a/abcabc", nil)
	c.Request = r
	e.HandleContext(c)
	s, err = p.ResolveStream("6769855a9aa43b67086f9ff3c1a5bacb5698a27a")
	require.NoError(t, err)
	assert.False(t, fitForTranscoder(c, s))

	r, _ = http.NewRequest(http.MethodHead, "https://cdn.lbryplayer.xyz/v6/streams/6769855a9aa43b67086f9ff3c1a5bacb5698a27a/abcabc", nil)
	c.Request = r
	e.HandleContext(c)
	s, err = p.ResolveStream("6769855a9aa43b67086f9ff3c1a5bacb5698a27a")
	require.NoError(t, err)
	assert.True(t, fitForTranscoder(c, s))

	r, _ = http.NewRequest(http.MethodHead, "https://cdn.lbryplayer.xyz/v6/streams/6769855a9aa43b67086f9ff3c1a5bacb5698a27a/abcabc.mp4", nil)
	c.Request = r
	e.HandleContext(c)
	s, err = p.ResolveStream("6769855a9aa43b67086f9ff3c1a5bacb5698a27a")
	require.NoError(t, err)
	assert.True(t, fitForTranscoder(c, s))

	r, _ = http.NewRequest(http.MethodGet, "https://cdn.lbryplayer.xyz/v6/streams/6769855a9aa43b67086f9ff3c1a5bacb5698a27a/abcabc", nil)
	c.Request = r
	e.HandleContext(c)
	s, err = p.ResolveStream("6769855a9aa43b67086f9ff3c1a5bacb5698a27a")
	require.NoError(t, err)
	assert.False(t, fitForTranscoder(c, s))

	r, _ = http.NewRequest(http.MethodGet, "https://cdn.lbryplayer.xyz/v6/streams/6769855a9aa43b67086f9ff3c1a5bacb5698a27a/abcabc.mp4", nil)
	c.Request = r
	e.HandleContext(c)
	s, err = p.ResolveStream("6769855a9aa43b67086f9ff3c1a5bacb5698a27a")
	require.NoError(t, err)
	assert.False(t, fitForTranscoder(c, s))
}
