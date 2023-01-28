package firewall

import (
	"time"

	"github.com/bluele/gcache"
	"github.com/modern-go/concurrent"
)

const WINDOW_SIZE = 60 * time.Second
const MAX_STRINGS_PER_IP = 5

var meCache = gcache.New(1000).Simple().Build()

func IsIpAbusingResources(ip string, endpoint string) bool {
	resources, err := meCache.Get(ip)
	if err == gcache.KeyNotFoundError {
		tokensMap := concurrent.NewMap()
		tokensMap.Store(endpoint, true)
		err := meCache.SetWithExpire(ip, tokensMap, WINDOW_SIZE)
		if err != nil {
			return false
		}
		return false
	}
	tokensForIP, _ := resources.(*concurrent.Map)
	tokensForIP.Store(endpoint, true)
	resourcesCount := 0
	flagged := false
	tokensForIP.Range(func(k, v interface{}) bool {
		resourcesCount++
		if resourcesCount > MAX_STRINGS_PER_IP {
			flagged = true
			return false
		}
		return true
	})
	return flagged
}
