package logger

import (
	"io"

	"github.com/sirupsen/logrus"
)

const (
	EnvTest = "test"
	EnvProd = "prod"
)

var (
	JsonFormatter = &logrus.JSONFormatter{DisableTimestamp: true}
	TextFormatter = &logrus.TextFormatter{FullTimestamp: true, TimestampFormat: "15:04:05"}
	level         = logrus.InfoLevel
	formatter     = TextFormatter
	loggers       []*logrus.Logger
)

func ConfigureDefaults(logLevel logrus.Level) {
	level = logLevel
	for _, logger := range loggers {
		logger.SetLevel(level)
	}
}

func GetLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetLevel(level)
	logger.SetFormatter(JsonFormatter)
	loggers = append(loggers, logger)
	return logger
}

// DisableLogger turns off logging output for this module logger
func DisableLogger(l *logrus.Logger) {
	l.SetLevel(logrus.PanicLevel)
	l.SetOutput(io.Discard)
}
