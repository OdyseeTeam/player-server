package logger

import (
	"time"

	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
)

var IgnoredExceptions = []string{}
var SentryHandler = sentryhttp.New(sentryhttp.Options{
	Repanic:         true,
	WaitForDelivery: true,
})

func ConfigureSentry(release, env string) {
	opts := sentry.ClientOptions{
		Dsn:              "",
		Release:          release,
		AttachStacktrace: true,
		BeforeSend:       filterEvent,
	}
	if env == EnvTest {
		opts.Transport = TestSentryTransport
	}

	err := sentry.Init(opts)
	if err != nil {
		Logger.Fatalf("sentry initialization failed: %v", err)
	} else {
		Logger.Info("sentry initialized")
	}
}

func filterEvent(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
	for _, exc := range event.Exception {
		for _, ignored := range IgnoredExceptions {
			if exc.Value == ignored {
				return nil
			}
		}
	}
	return event
}

func Flush() {
	sentry.Flush(2 * time.Second)
	sentry.Recover()
}
