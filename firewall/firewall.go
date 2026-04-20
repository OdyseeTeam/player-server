package firewall

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/OdyseeTeam/player-server/internal/iapi"
	"github.com/OdyseeTeam/player-server/internal/metrics"
	"github.com/bluele/gcache"
	"github.com/gaissmai/bart"
	"github.com/lbryio/lbry.go/v2/extras/errors"
	"github.com/oschwald/maxminddb-golang"
	"github.com/puzpuzpuz/xsync/v3"
)

type blacklist struct {
	BlacklistedAsn []int    `json:"blacklisted_asn"`
	BlacklistedIPs []string `json:"blacklisted_ips"`
}

func init() {
	ReloadBlacklist()
	go trackIPCacheSize()
	asnBandwidthMultiplier.Store(10)
	asnBandwidthMinThreshold.Store(defaultMinASNThreshold)
	asnBandwidthWarmup.Store(defaultWarmupCycles)
}

func trackIPCacheSize() {
	ticker := time.NewTicker(15 * time.Second)
	for range ticker.C {
		metrics.FirewallTrackedIPs.Set(float64(resourcesForIPCache.Len(true)))
	}
}

func LogAbuseEvent(eventType, ip string, asn int, org, claimID, path string, count int) {
	args := []any{
		"component", "firewall",
		"event", "abuse",
		"type", eventType,
		"ip", ip,
	}
	if asn > 0 {
		args = append(args, "asn", asn, "org", org)
	}
	if claimID != "" {
		args = append(args, "claim_id", claimID)
	}
	if path != "" {
		args = append(args, "path", path)
	}
	if count > 0 {
		args = append(args, "count", count)
	}
	slog.Warn("abuse detected", args...)
}

func ReloadBlacklist() {
	f, err := os.ReadFile("blacklist.json")
	if err != nil {
		slog.Warn("no blacklist file found, skipping blacklist", "component", "firewall")
		return
	}

	var bl blacklist
	err = json.Unmarshal(f, &bl)
	if err != nil {
		slog.Error("failed to unmarshal blacklist", "component", "firewall", "error", err)
		return
	}
	blacklistedAsn.Clear()
	bannedIPs = &bart.Table[int]{}
	for _, v := range bl.BlacklistedAsn {
		blacklistedAsn.Store(v, true)
	}
	for _, v := range bl.BlacklistedIPs {
		parsedPrefix, err := netip.ParsePrefix(v)
		if err != nil {
			slog.Warn("error parsing IP in blacklist", "component", "firewall", "ip", v, "error", err)
			continue
		}
		bannedIPs.Insert(parsedPrefix, 1)
	}
}

var WindowSize = 120 * time.Second

const MaxStringsPerIp = 4

var resourcesForIPCache = gcache.New(1000).Simple().Build()
var whitelist = map[string]bool{
	"51.210.0.171": true,
}

var bannedIPs = &bart.Table[int]{}
var blacklistedAsn = xsync.NewMapOf[int, bool]()

type asnBandwidth struct {
	org   string
	bytes int64
}

type asnCacheEntry struct {
	asn int
	org string
}

type ipEntry struct {
	IP    string `json:"ip"`
	Bytes int64  `json:"bytes"`
}

type asnEntry struct {
	ASN   string `json:"asn"`
	Org   string `json:"org"`
	Bytes int64  `json:"bytes"`
}

var (
	bandwidthPerIP      atomic.Pointer[xsync.MapOf[string, *int64]]
	bandwidthPerASN     atomic.Pointer[xsync.MapOf[string, *asnBandwidth]]
	ipToASNCache        gcache.Cache
	bandwidthStop       chan struct{}
	bandwidthMu         sync.Mutex
	bandwidthRunning    bool
	asnLookupDisabled   atomic.Bool

	asnBandwidthThreshold    atomic.Int64
	asnBandwidthMultiplier   atomic.Int64
	asnBandwidthMinThreshold atomic.Int64
	asnBandwidthWarmup       atomic.Int32
)

const (
	defaultMinASNThreshold = 1 << 30 // 1GB
	minASNCount            = 5
	topASNsForMedian       = 10
	defaultWarmupCycles    = 3
)

func CheckBans(ip, path, claimID string) bool {
	parsedIp, err := netip.ParseAddr(ip)
	if err != nil {
		slog.Warn("error parsing IP", "component", "firewall", "ip", ip, "error", err)
		return false
	}
	_, ok := bannedIPs.Lookup(parsedIp)
	if ok {
		metrics.FirewallBlocked.WithLabelValues(metrics.FirewallReasonIPBan).Inc()
		LogAbuseEvent(metrics.FirewallReasonIPBan, ip, 0, "", claimID, path, 0)
		return true
	}
	org, asn, err := GetProviderForIP(ip)
	if err == nil {
		if _, found := blacklistedAsn.Load(asn); found {
			metrics.FirewallBlocked.WithLabelValues(metrics.FirewallReasonASNBan).Inc()
			metrics.FirewallASNBlocked.WithLabelValues(strings.ToLower(org)).Inc()
			LogAbuseEvent(metrics.FirewallReasonASNBan, ip, asn, org, claimID, path, 0)
			return true
		}
	}
	return false
}

func CheckAndRateLimitIp(ip string, endpoint string) (bool, int) {
	if ip == "" {
		return false, 0
	}
	if whitelist[ip] {
		return false, 0
	}
	resources, err := resourcesForIPCache.Get(ip)
	if errors.Is(err, gcache.KeyNotFoundError) {
		tokensMap := &sync.Map{}
		tokensMap.Store(endpoint, time.Now())
		err := resourcesForIPCache.SetWithExpire(ip, tokensMap, WindowSize*10)
		if err != nil {
			return false, 1
		}
		return false, 1
	}
	tokensForIP, _ := resources.(*sync.Map)
	currentTime := time.Now()
	tokensForIP.Store(endpoint, currentTime)
	resourcesCount := 0
	flagged := false
	tokensForIP.Range(func(k, v interface{}) bool {
		if currentTime.Sub(v.(time.Time)) > WindowSize {
			tokensForIP.Delete(k)
			return true
		}
		resourcesCount++
		if !flagged && resourcesCount > MaxStringsPerIp {
			flagged = true
		}
		return true
	})
	if flagged {
		metrics.FirewallRateLimitHits.WithLabelValues(metrics.FirewallOutcomeFlagged).Inc()
	}
	return flagged, resourcesCount
}

func IsStreamBlocked(claimId string, channelClaimId *string) bool {
	blocked, err := iapi.GetBlockedContent()
	if err == nil {
		if blocked[claimId] {
			return true
		}
	}

	if channelClaimId != nil && blocked[*channelClaimId] {
		return true
	}
	return false
}

var geoIpDbLocation = getGeoIPDbLocation()

func getGeoIPDbLocation() string {
	if dir := os.Getenv("GEOIP_DB_DIR"); dir != "" {
		return filepath.Join(dir, "GeoLite2-ASN.mmdb")
	}
	return filepath.Join(os.TempDir(), "GeoLite2-ASN.mmdb")
}

var providerDB *maxminddb.Reader
var providerDBInitOnce sync.Once
var providerDBInitErr error

func GetProviderForIP(ipStr string) (string, int, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "", 0, errors.Err("invalid ip")
	}
	providerDBInitOnce.Do(initProviderDB)
	if providerDBInitErr != nil {
		return "", 0, providerDBInitErr
	}
	if providerDB == nil {
		return "", 0, errors.Err("provider db not initialized")
	}
	var ASN struct {
		AutonomousSystemOrganization string `maxminddb:"autonomous_system_organization"`
		AutonomousSystemNumber       int    `maxminddb:"autonomous_system_number"`
	}

	err := providerDB.Lookup(ip, &ASN)
	if err != nil {
		return "", 0, errors.Err(err)
	}
	return ASN.AutonomousSystemOrganization, ASN.AutonomousSystemNumber, nil
}

func initProviderDB() {
	var err error
	providerDB, err = initISPGeoIPDB()
	if err != nil {
		providerDBInitErr = err
		asnLookupDisabled.Store(true)
	}
}

func initISPGeoIPDB() (*maxminddb.Reader, error) {
	key := os.Getenv("MAXMIND_KEY")
	if key == "" {
		return nil, errors.Err("MAXMIND_KEY not set")
	}
	info, err := os.Stat(geoIpDbLocation)
	if os.IsNotExist(err) || (err == nil && info.IsDir()) {
		resp, err := http.Get("https://download.maxmind.com/app/geoip_download?edition_id=GeoLite2-ASN&license_key=" + key + "&suffix=tar.gz")
		if err != nil {
			return nil, errors.Err(err)
		}
		defer resp.Body.Close()

		err = extractGeoIPDB(resp.Body, "GeoLite2-ASN.mmdb")
		if err != nil {
			return nil, errors.Err(err)
		}
	} else if err != nil {
		return nil, errors.Err(err)
	}

	providerDB, err := maxminddb.Open(geoIpDbLocation)
	if err != nil {
		return nil, errors.Err(err)
	}
	return providerDB, nil
}

func extractGeoIPDB(r io.Reader, dbName string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return errors.Err(err)
	}
	defer func() { _ = gzr.Close() }()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return errors.Err(err)
		}

		if header == nil {
			continue
		}

		target := filepath.Join(os.TempDir(), filepath.Base(header.Name))

		switch header.Typeflag {
		case tar.TypeReg:
			if filepath.Base(header.Name) == dbName {
				err := extractFile(tr, target, header)
				if err != nil {
					return errors.Err(err)
				}
			}

		case tar.TypeDir:
			err := os.MkdirAll(target, os.FileMode(header.Mode))
			if err != nil {
				return errors.Err(err)
			}
		}
	}
}

func extractFile(tr *tar.Reader, target string, header *tar.Header) error {
	err := os.MkdirAll(filepath.Dir(target), os.ModePerm)
	if err != nil {
		return errors.Err(err)
	}

	f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
	if err != nil {
		return errors.Err(err)
	}
	defer f.Close()

	_, err = io.Copy(f, tr)
	if err != nil {
		return errors.Err(err)
	}
	err = f.Sync()
	if err != nil {
		return errors.Err(err)
	}
	return nil
}

func initBandwidthMaps() {
	ipToASNCache = gcache.New(10000).LRU().Build()
	ipMap := xsync.NewMapOf[string, *int64]()
	asnMap := xsync.NewMapOf[string, *asnBandwidth]()
	bandwidthPerIP.Store(ipMap)
	bandwidthPerASN.Store(asnMap)
}

func TrackBandwidth(ip string, bytes int64) {
	if ip == "" || bytes <= 0 {
		return
	}

	ipMap := bandwidthPerIP.Load()
	if ipMap == nil {
		return
	}
	ipMap.Compute(ip, func(oldVal *int64, exists bool) (*int64, bool) {
		if exists {
			atomic.AddInt64(oldVal, bytes)
			return oldVal, false
		}
		val := new(int64)
		*val = bytes
		return val, false
	})

	if asnLookupDisabled.Load() {
		return
	}

	cache := ipToASNCache
	if cache == nil {
		return
	}

	var asn int
	var org string
	cached, err := cache.Get(ip)
	if err == nil {
		entry := cached.(asnCacheEntry)
		if entry.asn <= 0 {
			return
		}
		asn = entry.asn
		org = entry.org
	} else {
		org, asn, err = GetProviderForIP(ip)
		if err != nil {
			_ = cache.Set(ip, asnCacheEntry{asn: -1})
			return
		}
		_ = cache.Set(ip, asnCacheEntry{asn: asn, org: org})
	}

	asnKey := fmt.Sprintf("AS%d", asn)
	asnMap := bandwidthPerASN.Load()
	if asnMap == nil {
		return
	}
	asnMap.Compute(asnKey, func(oldVal *asnBandwidth, exists bool) (*asnBandwidth, bool) {
		if exists {
			atomic.AddInt64(&oldVal.bytes, bytes)
			return oldVal, false
		}
		return &asnBandwidth{org: org, bytes: bytes}, false
	})
}

func StartBandwidthReporter() {
	bandwidthMu.Lock()
	defer bandwidthMu.Unlock()

	if bandwidthRunning {
		return
	}

	initBandwidthMaps()
	bandwidthStop = make(chan struct{})
	bandwidthRunning = true
	stopCh := bandwidthStop
	go bandwidthReporterLoop(stopCh)
}

func StopBandwidthReporter() {
	bandwidthMu.Lock()
	defer bandwidthMu.Unlock()

	if !bandwidthRunning {
		return
	}

	close(bandwidthStop)
	bandwidthRunning = false
}

func bandwidthReporterLoop(stopCh <-chan struct{}) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			logBandwidthReport()
		case <-stopCh:
			return
		}
	}
}

func logBandwidthReport() {
	newIPMap := xsync.NewMapOf[string, *int64]()
	newASNMap := xsync.NewMapOf[string, *asnBandwidth]()

	oldIPMap := bandwidthPerIP.Swap(newIPMap)
	oldASNMap := bandwidthPerASN.Swap(newASNMap)

	if oldIPMap == nil || oldASNMap == nil {
		return
	}

	var ipEntries []ipEntry
	oldIPMap.Range(func(ip string, val *int64) bool {
		ipEntries = append(ipEntries, ipEntry{IP: ip, Bytes: atomic.LoadInt64(val)})
		return true
	})
	sort.Slice(ipEntries, func(i, j int) bool {
		return ipEntries[i].Bytes > ipEntries[j].Bytes
	})
	if len(ipEntries) > 10 {
		ipEntries = ipEntries[:10]
	}

	var asnEntries []asnEntry
	oldASNMap.Range(func(asnKey string, val *asnBandwidth) bool {
		asnEntries = append(asnEntries, asnEntry{ASN: asnKey, Org: val.org, Bytes: atomic.LoadInt64(&val.bytes)})
		return true
	})
	sort.Slice(asnEntries, func(i, j int) bool {
		return asnEntries[i].Bytes > asnEntries[j].Bytes
	})

	updateASNBandwidthThreshold(asnEntries)

	if len(asnEntries) > 5 {
		asnEntries = asnEntries[:5]
	}

	if len(ipEntries) == 0 && len(asnEntries) == 0 {
		return
	}

	slog.Info("bandwidth report",
		"component", "firewall",
		"top_ips", ipEntries,
		"top_asns", asnEntries,
		"asn_throttle_threshold", asnBandwidthThreshold.Load(),
		"asn_throttle_enabled", asnBandwidthMultiplier.Load() > 0 && asnBandwidthThreshold.Load() > 0,
	)
}

func updateASNBandwidthThreshold(entries []asnEntry) {
	if warmup := asnBandwidthWarmup.Load(); warmup > 0 {
		asnBandwidthWarmup.Add(-1)
		asnBandwidthThreshold.Store(0)
		return
	}

	if len(entries) < minASNCount {
		asnBandwidthThreshold.Store(0)
		return
	}

	if len(entries) > topASNsForMedian {
		entries = entries[:topASNsForMedian]
	}

	n := len(entries)
	var median int64
	if n%2 == 1 {
		median = entries[n/2].Bytes
	} else {
		median = (entries[n/2-1].Bytes + entries[n/2].Bytes) / 2
	}

	if median == 0 {
		asnBandwidthThreshold.Store(0)
		return
	}

	multiplier := asnBandwidthMultiplier.Load()
	if multiplier <= 0 {
		asnBandwidthThreshold.Store(0)
		return
	}

	threshold := median * multiplier

	minThreshold := asnBandwidthMinThreshold.Load()
	if threshold < minThreshold {
		asnBandwidthThreshold.Store(0)
		return
	}

	asnBandwidthThreshold.Store(threshold)
}

func CheckASNBandwidthLimit(ip string) (blocked bool, asn int, org string, currentBytes int64, threshold int64) {
	if ip == "" || whitelist[ip] {
		return false, 0, "", 0, 0
	}

	if asnBandwidthMultiplier.Load() <= 0 {
		return false, 0, "", 0, 0
	}

	threshold = asnBandwidthThreshold.Load()
	if threshold <= 0 {
		return false, 0, "", 0, 0
	}

	if asnLookupDisabled.Load() {
		return false, 0, "", 0, 0
	}

	cache := ipToASNCache
	if cache == nil {
		return false, 0, "", 0, 0
	}

	cached, err := cache.Get(ip)
	if err == nil {
		entry := cached.(asnCacheEntry)
		if entry.asn <= 0 {
			return false, 0, "", 0, threshold
		}
		asn = entry.asn
		org = entry.org
	} else {
		org, asn, err = GetProviderForIP(ip)
		if err != nil {
			_ = cache.Set(ip, asnCacheEntry{asn: -1})
			return false, 0, "", 0, threshold
		}
		_ = cache.Set(ip, asnCacheEntry{asn: asn, org: org})
	}

	asnKey := fmt.Sprintf("AS%d", asn)
	asnMap := bandwidthPerASN.Load()
	if asnMap == nil {
		return false, asn, org, 0, threshold
	}

	val, ok := asnMap.Load(asnKey)
	if !ok {
		return false, asn, org, 0, threshold
	}

	currentBytes = atomic.LoadInt64(&val.bytes)
	if currentBytes > threshold {
		return true, asn, org, currentBytes, threshold
	}

	return false, asn, org, currentBytes, threshold
}

func GetWindowSizeSeconds() int {
	return int(WindowSize.Seconds())
}

func SetASNBandwidthMultiplier(m int64) {
	asnBandwidthMultiplier.Store(m)
	if m <= 0 {
		asnBandwidthThreshold.Store(0)
	}
}

func GetASNBandwidthMultiplier() int64 {
	return asnBandwidthMultiplier.Load()
}

func GetASNBandwidthThreshold() int64 {
	return asnBandwidthThreshold.Load()
}

func GetASNBandwidthMinThreshold() int64 {
	return asnBandwidthMinThreshold.Load()
}

func SetASNBandwidthMinThreshold(t int64) {
	asnBandwidthMinThreshold.Store(t)
}

func GetASNBandwidthWarmup() int32 {
	return asnBandwidthWarmup.Load()
}

func ResetASNBandwidthWarmup() {
	asnBandwidthWarmup.Store(defaultWarmupCycles)
}

func GetASNsOverThreshold() []string {
	threshold := asnBandwidthThreshold.Load()
	if threshold <= 0 {
		return nil
	}

	asnMap := bandwidthPerASN.Load()
	if asnMap == nil {
		return nil
	}

	var over []string
	asnMap.Range(func(asnKey string, val *asnBandwidth) bool {
		if atomic.LoadInt64(&val.bytes) > threshold {
			over = append(over, asnKey)
		}
		return true
	})
	return over
}
