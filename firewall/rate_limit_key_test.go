package firewall

import (
	"testing"
	"time"

	"github.com/bluele/gcache"
	"github.com/stretchr/testify/assert"
)

func TestRateLimitKey(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"192.168.0.1", "192.168.0.1"},
		{"1.2.3.4", "1.2.3.4"},
		{"not-an-ip", "not-an-ip"},
		{"", ""},
		{"2a03:2880:f814:42::1", "2a03:2880:f814:42::"},
		{"2a03:2880:f814:42:aaaa:bbbb:cccc:dddd", "2a03:2880:f814:42::"},
		{"2a03:2880:f814:43::1", "2a03:2880:f814:43::"},
		{"::1", "::"},
		{"::ffff:1.2.3.4", "1.2.3.4"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, rateLimitKey(tc.in))
		})
	}
}

func TestCheckAndRateLimitIp_IPv6FanoutSharesBucket(t *testing.T) {
	resourcesForIPCache = gcache.New(1000).Simple().Build()
	WindowSize = 10 * time.Second
	t.Cleanup(func() { WindowSize = 120 * time.Second })

	prefix := "2a03:2880:f814:42::"
	for i := 1; i <= MaxStringsPerIp; i++ {
		flagged, _ := CheckAndRateLimitIp(prefix+"1", "claim-"+string(rune('A'+i)))
		assert.False(t, flagged, "request %d from same /64 should not flag yet", i)
	}
	flagged, count := CheckAndRateLimitIp("2a03:2880:f814:42::2", "claim-X")
	assert.True(t, flagged, "same-/64 request with a new claim should flag when count exceeds MaxStringsPerIp")
	assert.Greater(t, count, MaxStringsPerIp)
}

func TestCheckAndRateLimitIp_DifferentV6PrefixesIndependent(t *testing.T) {
	resourcesForIPCache = gcache.New(1000).Simple().Build()
	WindowSize = 10 * time.Second
	t.Cleanup(func() { WindowSize = 120 * time.Second })

	prefixes := []string{
		"2a03:2880:f814:10::1",
		"2a03:2880:f814:11::1",
		"2a03:2880:f814:12::1",
		"2a03:2880:f814:13::1",
		"2a03:2880:f814:14::1",
		"2a03:2880:f814:15::1",
	}
	for i, ip := range prefixes {
		flagged, _ := CheckAndRateLimitIp(ip, "claim-"+string(rune('A'+i)))
		assert.False(t, flagged, "ip %s in its own /64 should not share a bucket with other /64s", ip)
	}
}

func TestCheckAndRateLimitIp_WhitelistIsPerAddress(t *testing.T) {
	resourcesForIPCache = gcache.New(1000).Simple().Build()
	WindowSize = 10 * time.Second
	t.Cleanup(func() { WindowSize = 120 * time.Second })

	whitelisted := "2a03:2880:f814:99::1"
	whitelist[whitelisted] = true
	t.Cleanup(func() { delete(whitelist, whitelisted) })

	for i := 0; i < 20; i++ {
		flagged, _ := CheckAndRateLimitIp(whitelisted, "claim-"+string(rune('A'+i)))
		assert.False(t, flagged, "whitelisted /128 should never flag")
	}

	sibling := "2a03:2880:f814:99::2"
	for i := 0; i <= MaxStringsPerIp; i++ {
		CheckAndRateLimitIp(sibling, "sibling-"+string(rune('A'+i)))
	}
	flagged, _ := CheckAndRateLimitIp(sibling, "sibling-final")
	assert.True(t, flagged, "non-whitelisted /128 in the same /64 as a whitelisted address is still rate-limited")
}

func TestCheckAndRateLimitIp_IPv4PassthroughUnchanged(t *testing.T) {
	resourcesForIPCache = gcache.New(1000).Simple().Build()
	WindowSize = 10 * time.Second
	t.Cleanup(func() { WindowSize = 120 * time.Second })

	ip := "198.51.100.7"
	for i := 1; i <= MaxStringsPerIp; i++ {
		flagged, _ := CheckAndRateLimitIp(ip, "v4-claim-"+string(rune('A'+i)))
		assert.False(t, flagged)
	}
	flagged, count := CheckAndRateLimitIp(ip, "v4-claim-final")
	assert.True(t, flagged)
	assert.Greater(t, count, MaxStringsPerIp)

	siblingIP := "198.51.100.8"
	flagged, _ = CheckAndRateLimitIp(siblingIP, "v4-claim-sibling")
	assert.False(t, flagged, "different /32 gets its own bucket")
}
