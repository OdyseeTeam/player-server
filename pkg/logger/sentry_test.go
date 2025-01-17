package logger

import (
	"errors"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSentryMiddleware(t *testing.T) {
	var event *sentry.Event
	ConfigureSentry("test", EnvTest)

	SendToSentry(errors.New("general error"))
	event = TestSentryTransport.LastEvent
	require.NotNil(t, event)
	assert.Equal(t, "general error", event.Exception[0].Value)
	assert.Empty(t, event.Extra)

	SendToSentry(errors.New("general error"), "user", "someUser")
	event = TestSentryTransport.LastEvent
	require.NotNil(t, event)
	assert.Equal(t, "general error", event.Exception[0].Value)
	assert.Equal(t, map[string]any{"user": "someUser"}, event.Extra)

	SendToSentry(errors.New("general error"), "user", "someUser", "dangling param")
	event = TestSentryTransport.LastEvent
	require.NotNil(t, event)
	assert.Equal(t, "general error", event.Exception[0].Value)
	assert.Equal(t, map[string]any{"user": "someUser"}, event.Extra)
}
