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
