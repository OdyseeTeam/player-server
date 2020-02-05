package logger

import (
	"io/ioutil"

	"github.com/sirupsen/logrus"
)

var jsonFormatter = logrus.JSONFormatter{DisableTimestamp: true}
var textFormatter = logrus.TextFormatter{FullTimestamp: true, TimestampFormat: "15:04:05"}

func GetLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	logger.SetFormatter(&textFormatter)
	return logger
}

// Disable turns off logging output for this module logger
func DisableLogger(l *logrus.Logger) {
	l.SetLevel(logrus.PanicLevel)
	l.SetOutput(ioutil.Discard)
}
