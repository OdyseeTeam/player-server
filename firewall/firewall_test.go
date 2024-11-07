package firewall

import (
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/lbryio/lbry.go/v2/extras/errors"
	"github.com/stretchr/testify/assert"
)

func TestCheckIPAccess(t *testing.T) {
	ip := "192.168.0.1"
	endpoint := "/api/v1/example"
	WindowSize = 7 * time.Second
	// Test the first five accesses for an IP don't exceed the limit
	for i := 1; i <= 6; i++ {
		result, _ := CheckAndRateLimitIp(ip, endpoint+strconv.Itoa(i))
		assert.False(t, result, "Expected result to be false, got %v for endpoint %s", result, endpoint+strconv.Itoa(i))
	}

	// Test the sixth access for an IP exceeds the limit
	result, _ := CheckAndRateLimitIp(ip, endpoint+"7")
	assert.True(t, result, "Expected result to be true, got %v for endpoint %s", result, endpoint+"7")

	// Wait for the window size to elapse
	time.Sleep(WindowSize)

	// Test the access for an IP after the window size elapses doesn't exceed the limit
	result, _ = CheckAndRateLimitIp(ip, endpoint+"7")
	assert.False(t, result, "Expected result to be false, got %v for endpoint %s", result, endpoint+"7")
}

func Test_initISPGeoIPDB(t *testing.T) {
	if os.Getenv("MAXMIND_KEY") == "" {
		t.Skip("Skipping test because MAXMIND_KEY is not set")
	}
	ip := "1.1.1.1"
	ASN, nr, err := GetProviderForIP(ip)
	if !assert.NoError(t, err) {
		fmt.Println(errors.FullTrace(err))
	}
	assert.Equal(t, "CLOUDFLARENET", ASN)
	assert.Equal(t, 13335, nr)
}
