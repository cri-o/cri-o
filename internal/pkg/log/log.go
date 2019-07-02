// Package log provides a global interface to logging functionality
package log

import (
	"context"

	"github.com/sirupsen/logrus"
)

type id struct{}

func Debugf(ctx context.Context, format string, args ...interface{}) {
	entry(ctx).Debugf(format, args...)
}

func Infof(ctx context.Context, format string, args ...interface{}) {
	entry(ctx).Infof(format, args...)
}

func Warnf(ctx context.Context, format string, args ...interface{}) {
	entry(ctx).Warnf(format, args...)
}

func Errorf(ctx context.Context, format string, args ...interface{}) {
	entry(ctx).Errorf(format, args...)
}

func entry(ctx context.Context) *logrus.Entry {
	logger := logrus.StandardLogger()
	if ctx == nil {
		return logrus.NewEntry(logger)
	}

	idValue := ctx.Value(id{})
	if ret, ok := idValue.(string); ok {
		return logger.WithField("id", ret)
	}

	return logrus.NewEntry(logger)
}
