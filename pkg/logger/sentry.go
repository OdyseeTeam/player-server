package logger

import (
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
)

var IgnoredExceptions = []string{}
var SentryHandler = sentrygin.New(sentrygin.Options{
	Repanic:         true,
	WaitForDelivery: true,
})

func ConfigureSentry(release, env string) {
	l := GetLogger()
	dsn := os.Getenv("SENTRY_DSN")
	opts := sentry.ClientOptions{
		Dsn:              dsn,
		Release:          release,
		AttachStacktrace: true,
		BeforeSend:       filterEvent,
	}

	if env == EnvTest {
		opts.Transport = TestSentryTransport
	} else if dsn == "" {
		l.Info("sentry disabled")
		return
	}

	err := sentry.Init(opts)
	if err != nil {
		l.Fatalf("sentry initialization failed: %v", err)
	} else {
		l.Info("sentry initialized")
	}
}

// SendToSentry sends an error to Sentry with optional extra details.
func SendToSentry(err error, details ...string) *sentry.EventID {
	extra := map[string]string{}
	var eventID *sentry.EventID
	for i := 0; i < len(details); i += 2 {
		if i+1 > len(details)-1 {
			break
		}
		extra[details[i]] = details[i+1]
	}

	sentry.WithScope(func(scope *sentry.Scope) {
		for k, v := range extra {
			scope.SetExtra(k, v)
		}
		sentry.CaptureException(err)
	})
	return eventID
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
