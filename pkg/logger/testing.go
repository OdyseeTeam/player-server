package logger

import (
	"context"
	"time"

	"github.com/getsentry/sentry-go"
)

var TestSentryTransport = &TestTransport{}

type TestTransport struct {
	LastEvent *sentry.Event
}

func (t *TestTransport) Flush(timeout time.Duration) bool {
	return true
}

func (t *TestTransport) FlushWithContext(ctx context.Context) bool {
	return true
}

func (t *TestTransport) Configure(options sentry.ClientOptions) {
}

func (t *TestTransport) SendEvent(event *sentry.Event) {
	t.LastEvent = event
}

func (t *TestTransport) Close() {
}
