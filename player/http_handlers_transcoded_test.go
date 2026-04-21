package player

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OdyseeTeam/player-server/firewall"
	"github.com/OdyseeTeam/player-server/internal/metrics"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func newTranscodedFirewallRouter(t *testing.T) *gin.Engine {
	t.Helper()

	sdk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"jsonrpc":"2.0","error":{"code":-32000,"message":"not found"},"id":1}`)
	}))
	t.Cleanup(sdk.Close)

	p := NewPlayer(nil, WithLbrynetServer(sdk.URL))
	p.TCVideoPath = t.TempDir()

	router := gin.New()
	InstallPlayerRoutes(router, p)
	return router
}

const emptyBlacklist = `{"blacklisted_asn":[],"blacklisted_ips":[]}`

// writeTestBlacklist swaps in a test-scoped blacklist.json for the duration of
// the test. Cleanup overwrites the file with an empty blacklist and reloads
// before chdir-ing back, because ReloadBlacklist is additive/overwrite in
// practice but is a no-op if its configured file is absent.
func writeTestBlacklist(t *testing.T, blacklist string) {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	dir := t.TempDir()
	path := filepath.Join(dir, "blacklist.json")
	require.NoError(t, os.WriteFile(path, []byte(blacklist), 0o600))
	require.NoError(t, os.Chdir(dir))
	firewall.ReloadBlacklist()
	t.Cleanup(func() {
		_ = os.WriteFile(path, []byte(emptyBlacklist), 0o600)
		firewall.ReloadBlacklist()
		_ = os.Chdir(cwd)
	})
}

func fireTranscodedRequest(t *testing.T, router *gin.Engine, ip, claimID string) *httptest.ResponseRecorder {
	t.Helper()
	uri := fmt.Sprintf("/v6/streams/%s/abcabc/v0_s000001.ts", claimID)
	r, err := http.NewRequest(http.MethodGet, uri, nil)
	require.NoError(t, err)
	r.RemoteAddr = ip + ":12345"
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, r)
	return rr
}

func paddedClaimID(suffix byte) string {
	return strings.Repeat("a", 39) + string(suffix)
}

func TestHandleTranscodedFragment_IPBanReturns429(t *testing.T) {
	writeTestBlacklist(t, `{"blacklisted_asn":[],"blacklisted_ips":["203.0.113.7/32"]}`)
	router := newTranscodedFirewallRouter(t)

	before := testutil.ToFloat64(metrics.FirewallBlocked.WithLabelValues(metrics.FirewallReasonIPBan))
	rr := fireTranscodedRequest(t, router, "203.0.113.7", paddedClaimID('1'))

	require.Equal(t, http.StatusTooManyRequests, rr.Code)
	require.InDelta(t, 1.0, testutil.ToFloat64(metrics.FirewallBlocked.WithLabelValues(metrics.FirewallReasonIPBan))-before, 0, "IP-ban counter must increment by exactly 1")
}

func TestHandleTranscodedFragment_ASNBanReturns429(t *testing.T) {
	if os.Getenv("MAXMIND_KEY") == "" {
		t.Skip("MAXMIND_KEY not set")
	}
	writeTestBlacklist(t, `{"blacklisted_asn":[13335],"blacklisted_ips":[]}`)
	router := newTranscodedFirewallRouter(t)

	before := testutil.ToFloat64(metrics.FirewallBlocked.WithLabelValues(metrics.FirewallReasonASNBan))
	rr := fireTranscodedRequest(t, router, "1.1.1.1", paddedClaimID('2'))

	require.Equal(t, http.StatusTooManyRequests, rr.Code)
	require.InDelta(t, 1.0, testutil.ToFloat64(metrics.FirewallBlocked.WithLabelValues(metrics.FirewallReasonASNBan))-before, 0, "ASN-ban counter must increment by exactly 1")
}

func TestHandleTranscodedFragment_ClaimDiversityBlocksAt11(t *testing.T) {
	writeTestBlacklist(t, emptyBlacklist)
	router := newTranscodedFirewallRouter(t)

	ip := "203.0.113.50"
	blockedBefore := testutil.ToFloat64(metrics.FirewallBlocked.WithLabelValues(metrics.FirewallReasonRateLimit))
	flaggedBefore := testutil.ToFloat64(metrics.FirewallRateLimitHits.WithLabelValues(metrics.FirewallOutcomeFlagged))

	for i := 0; i < 10; i++ {
		claim := paddedClaimID(byte('0' + i))
		rr := fireTranscodedRequest(t, router, ip, claim)
		require.NotEqual(t, http.StatusTooManyRequests, rr.Code, "request %d must not 429 from firewall yet", i+1)
	}

	afterTen := testutil.ToFloat64(metrics.FirewallRateLimitHits.WithLabelValues(metrics.FirewallOutcomeFlagged))
	require.Greater(t, afterTen, flaggedBefore, "flagged counter should increment once the count exceeds MaxStringsPerIp")
	require.Equal(t, blockedBefore, testutil.ToFloat64(metrics.FirewallBlocked.WithLabelValues(metrics.FirewallReasonRateLimit)), "no rate_limit block until count > 10")

	rr := fireTranscodedRequest(t, router, ip, paddedClaimID('A'))
	require.Equal(t, http.StatusTooManyRequests, rr.Code, "11th distinct claim must 429")
	require.InDelta(t, 1.0, testutil.ToFloat64(metrics.FirewallBlocked.WithLabelValues(metrics.FirewallReasonRateLimit))-blockedBefore, 0)
}

func TestHandleTranscodedFragment_CleanRequestPassesFirewall(t *testing.T) {
	writeTestBlacklist(t, emptyBlacklist)
	router := newTranscodedFirewallRouter(t)

	ip := "203.0.113.99"
	reasons := []string{
		metrics.FirewallReasonIPBan,
		metrics.FirewallReasonASNBan,
		metrics.FirewallReasonRateLimit,
		metrics.FirewallReasonASNBandwidthLimit,
	}
	before := make(map[string]float64, len(reasons))
	for _, r := range reasons {
		before[r] = testutil.ToFloat64(metrics.FirewallBlocked.WithLabelValues(r))
	}

	rr := fireTranscodedRequest(t, router, ip, paddedClaimID('f'))

	require.NotEqual(t, http.StatusTooManyRequests, rr.Code, "non-banned IP must not be firewall-blocked")
	for _, r := range reasons {
		require.Equal(t, before[r], testutil.ToFloat64(metrics.FirewallBlocked.WithLabelValues(r)), "no FirewallBlocked{%s} increment", r)
	}
}
