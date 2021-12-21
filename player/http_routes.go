package player

import (
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
)

const ProfileRoutePath = "/superdebug/pprof"

func InstallPlayerRoutes(r *gin.Engine, p *Player) {
	playerHandler := NewRequestHandler(p)

	v1Router := r.Group("/content/claims/:claim_name/:claim_id/:filename")
	v1Router.GET("", playerHandler.Handle)
	v1Router.HEAD("", playerHandler.Handle)

	v2Router := r.Group("/api/v2")
	v2Router.GET("/streams/free/:claim_name/:claim_id", playerHandler.Handle)
	v2Router.HEAD("/streams/free/:claim_name/:claim_id", playerHandler.Handle)
	v2Router.GET("/streams/paid/:claim_name/:claim_id/:token", playerHandler.Handle)
	v2Router.HEAD("/streams/paid/:claim_name/:claim_id/:token", playerHandler.Handle)

	v3Router := r.Group("/api/v3")
	v3Router.GET("/streams/free/:claim_name/:claim_id/:sd_hash", playerHandler.Handle)
	v3Router.HEAD("/streams/free/:claim_name/:claim_id/:sd_hash", playerHandler.Handle)
	v3Router.GET("/streams/paid/:claim_name/:claim_id/:sd_hash/:token", playerHandler.Handle)
	v3Router.HEAD("/streams/paid/:claim_name/:claim_id/:sd_hash/:token", playerHandler.Handle)

	v4Router := r.Group("/api/v4")
	v4Router.GET("/streams/free/:claim_name/:claim_id/:sd_hash", playerHandler.Handle)
	v4Router.HEAD("/streams/free/:claim_name/:claim_id/:sd_hash", playerHandler.Handle)

	r.GET(SpeechPrefix, playerHandler.Handle)
	r.HEAD(SpeechPrefix, playerHandler.Handle)

	if p.TCVideoPath != "" {
		v4Router.GET("/streams/tc/:claim_name/:claim_id/:sd_hash/:fragment", playerHandler.HandleTranscodedFragment)
		v4Router.HEAD("/streams/tc/:claim_name/:claim_id/:sd_hash/:fragment", playerHandler.HandleTranscodedFragment)
	}
}

func InstallProfilingRoutes(r *gin.Engine) {
	pprof.Register(r, ProfileRoutePath)
}
