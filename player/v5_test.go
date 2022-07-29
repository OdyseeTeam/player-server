package player

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/lbryio/reflector.go/store"

	"github.com/Pallinder/go-randomdata"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	rentalClaim   = "81b1749f773bad5b9b53d21508051560f2746cdc"
	purchaseClaim = "2742f9e8eea0c4654ea8b51507dbb7f23f1f5235"
)

var testEdgeToken = randomdata.Alphanumeric(32)

type httpTest struct {
	Name string

	Method string
	URL    string

	ReqBody   io.Reader
	ReqHeader map[string]string

	Code        int
	ResBody     string
	ResHeader   map[string]string
	ResContains string
}

type apiV5Suite struct {
	suite.Suite

	player *Player
	router *gin.Engine
}

func (s *apiV5Suite) SetupSuite() {
	et, ok := os.LookupEnv("TEST_EDGE_TOKEN")
	if !ok {
		s.T().Skip("TEST_EDGE_TOKEN not set, skipping")
	}
	origin := store.NewHttpStore("source.odycdn.com:5569", et)
	ds := NewDecryptedCache(origin)
	p := NewPlayer(NewHotCache(*ds, 100000000), WithDownloads(true), WithEdgeToken(testEdgeToken))
	s.player = p

	s.router = gin.New()

	InstallPlayerRoutes(s.router, s.player)
}

func (s *apiV5Suite) TestMissingEdgeToken() {
	(&httpTest{
		Method:      http.MethodGet,
		URL:         "/api/v3/streams/free/randomstring/2742f9e8eea0c4654ea8b51507dbb7f23f1f5235/abcabc",
		Code:        http.StatusUnauthorized,
		ResContains: "edge credentials missing",
	}).Run(s.router, s.T())
	(&httpTest{
		Method:      http.MethodGet,
		URL:         "/api/v4/streams/free/randomstring/2742f9e8eea0c4654ea8b51507dbb7f23f1f5235/abcabc",
		Code:        http.StatusUnauthorized,
		ResContains: "edge credentials missing",
	}).Run(s.router, s.T())
	(&httpTest{
		Method:      http.MethodGet,
		URL:         "/v5/streams/original/2742f9e8eea0c4654ea8b51507dbb7f23f1f5235/abcdef",
		Code:        http.StatusUnauthorized,
		ResContains: "edge credentials missing",
	}).Run(s.router, s.T())
}

func (s *apiV5Suite) TestValidEdgeToken() {
	(&httpTest{
		Method: http.MethodGet,
		URL:    "/v5/streams/start/2742f9e8eea0c4654ea8b51507dbb7f23f1f5235/abcdef",
		Code:   http.StatusOK,
		ReqHeader: map[string]string{
			"Authorization": "Token " + testEdgeToken,
		},
	}).Run(s.router, s.T())
	(&httpTest{
		Method: http.MethodGet,
		URL:    "/v5/streams/original/2742f9e8eea0c4654ea8b51507dbb7f23f1f5235/abcdef",
		Code:   http.StatusOK,
		ReqHeader: map[string]string{
			"Authorization": "Token " + testEdgeToken,
		},
	}).Run(s.router, s.T())
}

func (test *httpTest) Run(handler http.Handler, t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	req, err := http.NewRequest(test.Method, test.URL, test.ReqBody)
	require.NoError(t, err)
	// req.RequestURI = test.URL

	// Add headers
	for key, value := range test.ReqHeader {
		req.Header.Set(key, value)
	}

	req.Host = "odysee.com"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != test.Code {
		t.Errorf("Expected %v %s as status code (got %v %s)", test.Code, http.StatusText(test.Code), w.Code, http.StatusText(w.Code))
	}

	for key, value := range test.ResHeader {
		header := w.Header().Get(key)

		if value != header {
			t.Errorf("Expected '%s' as '%s' (got '%s')", value, key, header)
		}
	}

	if test.ResBody != "" && w.Body.String() != test.ResBody {
		t.Errorf("Expected '%s' as body (got '%s'", test.ResBody, w.Body.String())
	}

	if test.ResContains != "" && !strings.Contains(w.Body.String(), test.ResContains) {
		t.Errorf("Expected '%s' to be present in response (got '%s'", test.ResContains, w.Body.String())
	}

	return w
}

func TestAPIV5Suite(t *testing.T) {
	suite.Run(t, new(apiV5Suite))
}
