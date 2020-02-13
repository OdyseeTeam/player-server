package app

import (
	"net/http"
	"testing"

	"github.com/lbryio/lbrytv-player/pkg/logger"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSentryMiddleware(t *testing.T) {
	logger.ConfigureSentry("test", logger.EnvTest)
	a := New(Opts{
		Address: ":4999",
	})

	a.Router.HandleFunc("/error", func(w http.ResponseWriter, r *http.Request) {
		if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
			hub.Scope().SetExtra("errored", "completely")
		}
		panic("y tho")
	})
	a.Start()

	_, err := http.Get("http://localhost:4999/error")

	require.Error(t, err)
	require.NotNil(t, logger.TestSentryTransport.LastEvent)
	assert.Equal(t, "y tho", logger.TestSentryTransport.LastEvent.Message)
	assert.Equal(t, "completely", logger.TestSentryTransport.LastEvent.Extra["errored"])

	assert.NoError(t, a.Shutdown())
}
