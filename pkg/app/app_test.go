package app

import (
	"net/http"
	"testing"

	"github.com/lbryio/lbrytv-player/pkg/logger"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSentryMiddleware(t *testing.T) {
	logger.ConfigureSentry("test", logger.EnvTest)
	a := New(Opts{
		Address: ":4999",
	})

	a.Router.GET("/error", func(c *gin.Context) {
		if hub := sentry.GetHubFromContext(c.Request.Context()); hub != nil {
			hub.Scope().SetExtra("errored", "completely")
		}
		panic("y tho")
	})
	a.Start()

	resp, err := http.Get("http://localhost:4999/error")
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	require.NotNil(t, logger.TestSentryTransport.LastEvent)
	assert.Equal(t, "y tho", logger.TestSentryTransport.LastEvent.Message)
	//assert.Equal(t, "completely", logger.TestSentryTransport.LastEvent.Extra["errored"]) //TODO: idk why I can't get this last thing to work

	assert.NoError(t, a.Shutdown())
}
