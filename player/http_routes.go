package player

import (
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
)

const ProfileRoutePath = "/superdebug/pprof"

func InstallPlayerRoutes(r *gin.Engine, p *Player) {
	playerHandler := NewRequestHandler(p)

	v1Router := r.Group("/content/claims/:claim_name/:claim_id/:filename")
	v1Router.HEAD("", playerHandler.Handle)
	v1Router.GET("", playerHandler.Handle)

	v2Router := r.Group("/api/v2/streams")
	v2Router.HEAD("/free/:claim_name/:claim_id", playerHandler.Handle)
	v2Router.GET("/free/:claim_name/:claim_id", playerHandler.Handle)
	v2Router.HEAD("/paid/:claim_name/:claim_id/:token", playerHandler.Handle)
	v2Router.GET("/paid/:claim_name/:claim_id/:token", playerHandler.Handle)

	v3Router := r.Group("/api/v3/streams")
	v3Router.HEAD("/free/:claim_name/:claim_id/:sd_hash", playerHandler.Handle)
	v3Router.GET("/free/:claim_name/:claim_id/:sd_hash", playerHandler.Handle)
	v3Router.HEAD("/paid/:claim_name/:claim_id/:sd_hash/:token", playerHandler.Handle)
	v3Router.GET("/paid/:claim_name/:claim_id/:sd_hash/:token", playerHandler.Handle)

	v4Router := r.Group("/api/v4/streams")
	v4Router.HEAD("/free/:claim_name/:claim_id/:sd_hash", playerHandler.Handle)
	v4Router.GET("/free/:claim_name/:claim_id/:sd_hash", playerHandler.Handle)

	v5Router := r.Group("/v5/streams")
	// HEAD will redirect to the transcoded version, if available; GET request will send a binary stream regardless
	v5Router.HEAD("/start/:claim_id/:sd_hash", playerHandler.Handle)
	v5Router.GET("/start/:claim_id/:sd_hash", playerHandler.Handle)
	v5Router.GET("/original/:claim_id/:sd_hash", playerHandler.Handle)

	v6Router := r.Group("/v6/streams")
	// HEAD will redirect to the transcoded version, if available; GET request will send a binary stream regardless
	v6Router.HEAD("/:claim_id/:sd_hash", playerHandler.Handle)
	v6Router.GET("/:claim_id/:sd_hash", playerHandler.Handle)

	if p.TCVideoPath != "" {
		v4Router.GET("/tc/:claim_name/:claim_id/:sd_hash/:fragment", playerHandler.HandleTranscodedFragment)
		v4Router.HEAD("/tc/:claim_name/:claim_id/:sd_hash/:fragment", playerHandler.HandleTranscodedFragment)

		v5Router.HEAD("/hls/:claim_id/:sd_hash/:fragment", playerHandler.HandleTranscodedFragment)
		v5Router.GET("/hls/:claim_id/:sd_hash/:fragment", playerHandler.HandleTranscodedFragment)

		v6Router.HEAD("/:claim_id/:sd_hash/:fragment", playerHandler.HandleTranscodedFragment)
		v6Router.GET("/:claim_id/:sd_hash/:fragment", playerHandler.HandleTranscodedFragment)
	}

	r.HEAD(SpeechPrefix+"*whatever", playerHandler.Handle)
	r.GET(SpeechPrefix+"*whatever", playerHandler.Handle)
}

func InstallProfilingRoutes(r *gin.Engine) {
	pprof.Register(r, ProfileRoutePath)
}
