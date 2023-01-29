package firewall

import (
	"time"

	"github.com/bluele/gcache"
	"github.com/modern-go/concurrent"
)

const WindowSize = 60 * time.Second
const MaxStringsPerIp = 5

var meCache = gcache.New(1000).Simple().Build()

func IsIpAbusingResources(ip string, endpoint string) bool {
	resources, err := meCache.Get(ip)
	if err == gcache.KeyNotFoundError {
		tokensMap := concurrent.NewMap()
		tokensMap.Store(endpoint, time.Now())
		err := meCache.SetWithExpire(ip, tokensMap, WindowSize)
		if err != nil {
			return false
		}
		return false
	}
	tokensForIP, _ := resources.(*concurrent.Map)
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
		if resourcesCount > MaxStringsPerIp {
			flagged = true
			return false
		}
		return true
	})
	return flagged
}
