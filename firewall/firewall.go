package firewall

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

var geoIpDbLocation = filepath.Join(os.TempDir(), "GeoLite2-ASN.mmdb")
var providerDB *maxminddb.Reader

func GetProviderForIP(ipStr string) (string, int, error) {
	ip := net.ParseIP(ipStr)
	if providerDB == nil {
		p, err := initISPGeoIPDB()
		if err != nil {
			return "", 0, err
		}
		providerDB = p
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
