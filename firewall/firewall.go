package firewall

import (
	"errors"
	"sync"
	"time"

	"github.com/bluele/gcache"
)

const WindowSize = 120 * time.Second
const MaxStringsPerIp = 6

var resourcesForIPCache = gcache.New(1000).Simple().Build()
var whitelist = map[string]bool{
	"51.210.0.171": true,
}

func IsIpAbusingResources(ip string, endpoint string) (bool, int) {
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
	return flagged, resourcesCount
}
