package logger

import (
	"fmt"
	"log/slog"
	"net/http"
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
	dsn := os.Getenv("SENTRY_DSN")
	playerName := os.Getenv("PLAYER_NAME")
	opts := sentry.ClientOptions{
		Dsn:              dsn,
		Release:          release,
		AttachStacktrace: true,
		BeforeSend:       filterEvent,
		ServerName:       playerName,
	}

	if env == EnvTest {
		opts.Transport = TestSentryTransport
	} else if dsn == "" {
		slog.Info("sentry disabled", "component", "logger")
		return
	}

	err := sentry.Init(opts)
	if err != nil {
		Fatal("sentry initialization failed", "component", "logger", "error", err)
	} else {
		slog.Info("sentry initialized", "component", "logger")
	}
}

func SendToSentry(err error, request *http.Request, details ...any) *sentry.EventID {
	extra := map[string]any{}
	var eventID *sentry.EventID
	for i := 0; i < len(details); i += 2 {
		if i+1 > len(details)-1 {
			break
		}
		extra[fmt.Sprint(details[i])] = details[i+1]
	}

	sentry.WithScope(func(scope *sentry.Scope) {
		for k, v := range extra {
			scope.SetExtra(k, v)
		}
		scope.SetRequest(request)
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
