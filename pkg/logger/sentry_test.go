package logger

import (
	"errors"
	"net/http"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSentryMiddleware(t *testing.T) {
	var event *sentry.Event
	ConfigureSentry("test", EnvTest)
	request, err := http.NewRequest(http.MethodGet, "http://player.tv/v6/stream", nil)
	require.NoError(t, err)

	SendToSentry(errors.New("general error"), request)
	event = TestSentryTransport.LastEvent
	require.NotNil(t, event)
	assert.Equal(t, "general error", event.Exception[0].Value)
	assert.Equal(t, "http://player.tv/v6/stream", event.Request.URL)
	assert.Empty(t, event.Extra)

	SendToSentry(errors.New("general error"), request, "user", "someUser")
	event = TestSentryTransport.LastEvent
	require.NotNil(t, event)
	assert.Equal(t, "general error", event.Exception[0].Value)
	assert.Equal(t, "http://player.tv/v6/stream", event.Request.URL)
	assert.Equal(t, map[string]any{"user": "someUser"}, event.Extra)

	SendToSentry(errors.New("general error"), request, "user", "someUser", "dangling param")
	event = TestSentryTransport.LastEvent
	require.NotNil(t, event)
	assert.Equal(t, "general error", event.Exception[0].Value)
	assert.Equal(t, "http://player.tv/v6/stream", event.Request.URL)
	assert.Equal(t, map[string]any{"user": "someUser"}, event.Extra)
}
