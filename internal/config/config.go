package config

import (
	"net/http"
	"strconv"

	"github.com/OdyseeTeam/player-server/firewall"
	"github.com/OdyseeTeam/player-server/player"

	"github.com/gin-gonic/gin"
)

var UserName string
var Password string

func InstallConfigRoute(r *gin.Engine) {
	authorized := r.Group("/config", gin.BasicAuth(gin.Accounts{
		UserName: Password,
	}))
	authorized.POST("/throttle", throttle)
	authorized.POST("/blacklist", reloadBlacklist)
	authorized.GET("/asn-bandwidth-limit", getASNBandwidthLimit)
	authorized.POST("/asn-bandwidth-limit", setASNBandwidthLimit)
}

// throttle allows for live configuration of the throttle scalar for the player (MB/s)
// http://localhost:8080/config/throttle?scale=1.2    //postman to add basic auth or temporarily remove basic auth
func throttle(c *gin.Context) {
	enabledStr := c.PostForm("enabled")
	if enabledStr != "" {
		enabled, err := strconv.ParseBool(enabledStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "failed to parse enabled " + enabledStr})
			return
		} else {
			player.ThrottleSwitch = enabled
		}
	}
	scaleStr := c.PostForm("scale")
	if scaleStr != "" {
		scale, err := strconv.ParseFloat(scaleStr, 64)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "failed to parse scale " + scaleStr})
		} else {
			player.ThrottleScale = scale
		}
	}
}

// reloadBlacklist reloads the blacklist from the file
func reloadBlacklist(c *gin.Context) {
	firewall.ReloadBlacklist()
	c.String(http.StatusOK, "blacklist reloaded")
}

func getASNBandwidthLimit(c *gin.Context) {
	multiplier := firewall.GetASNBandwidthMultiplier()
	threshold := firewall.GetASNBandwidthThreshold()
	minThreshold := firewall.GetASNBandwidthMinThreshold()
	warmup := firewall.GetASNBandwidthWarmup()
	over := firewall.GetASNsOverThreshold()

	c.JSON(http.StatusOK, gin.H{
		"enabled":               multiplier > 0 && threshold > 0,
		"multiplier":            multiplier,
		"threshold_bytes":       threshold,
		"min_threshold_bytes":   minThreshold,
		"warmup_cycles_remaining": warmup,
		"over_threshold":        over,
	})
}

func setASNBandwidthLimit(c *gin.Context) {
	enabledStr := c.PostForm("enabled")
	if enabledStr != "" {
		enabled, err := strconv.ParseBool(enabledStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "failed to parse enabled: " + enabledStr})
			return
		}
		if !enabled {
			firewall.SetASNBandwidthMultiplier(0)
			c.JSON(http.StatusOK, gin.H{"enabled": false, "multiplier": 0})
			return
		}
	}

	multiplierStr := c.PostForm("multiplier")
	if multiplierStr != "" {
		multiplier, err := strconv.ParseInt(multiplierStr, 10, 64)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "failed to parse multiplier: " + multiplierStr})
			return
		}
		if multiplier < 0 {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "multiplier must be >= 0"})
			return
		}
		firewall.SetASNBandwidthMultiplier(multiplier)
	}

	minThresholdStr := c.PostForm("min_threshold")
	if minThresholdStr != "" {
		minThreshold, err := strconv.ParseInt(minThresholdStr, 10, 64)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "failed to parse min_threshold: " + minThresholdStr})
			return
		}
		if minThreshold < 0 {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "min_threshold must be >= 0"})
			return
		}
		firewall.SetASNBandwidthMinThreshold(minThreshold)
	}

	if c.PostForm("reset_warmup") == "true" {
		firewall.ResetASNBandwidthWarmup()
	}

	c.JSON(http.StatusOK, gin.H{
		"enabled":             firewall.GetASNBandwidthMultiplier() > 0 && firewall.GetASNBandwidthThreshold() > 0,
		"multiplier":          firewall.GetASNBandwidthMultiplier(),
		"min_threshold_bytes": firewall.GetASNBandwidthMinThreshold(),
		"warmup_cycles_remaining": firewall.GetASNBandwidthWarmup(),
	})
}
