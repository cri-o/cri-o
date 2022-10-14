// Package log provides a global interface to logging functionality
package log

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type (
	ID   struct{}
	Name struct{}
)

func Debugf(ctx context.Context, format string, args ...interface{}) {
	logSpanf(ctx, "DEBUG", format, args...)
	entry(ctx).Debugf(format, args...)
}

func Infof(ctx context.Context, format string, args ...interface{}) {
	logSpanf(ctx, "INFO", format, args...)
	entry(ctx).Infof(format, args...)
}

func Warnf(ctx context.Context, format string, args ...interface{}) {
	logSpanf(ctx, "WARN", format, args...)
	entry(ctx).Warnf(format, args...)
}

func Errorf(ctx context.Context, format string, args ...interface{}) {
	logSpanf(ctx, "ERROR", format, args...)
	entry(ctx).Errorf(format, args...)
}

func Fatalf(ctx context.Context, format string, args ...interface{}) {
	logSpanf(ctx, "FATAL", format, args...)
	entry(ctx).Fatalf(format, args...)
}

func WithFields(ctx context.Context, fields map[string]interface{}) *logrus.Entry {
	return entry(ctx).WithFields(fields)
}

func logSpanf(ctx context.Context, level, format string, args ...interface{}) {
	id := "unknown"
	if ctx != nil {
		ctxID, idOk := ctx.Value(ID{}).(string)
		if idOk {
			id = ctxID
		}
	}
	trace.SpanFromContext(ctx).AddEvent(fmt.Sprintf(format, args...), trace.WithAttributes(
		attribute.String("level", level),
		attribute.String("rpc.id", id),
	))
}

func entry(ctx context.Context) *logrus.Entry {
	logger := logrus.StandardLogger()
	if ctx == nil {
		return logrus.NewEntry(logger)
	}

	id, idOk := ctx.Value(ID{}).(string)
	name, nameOk := ctx.Value(Name{}).(string)
	if idOk && nameOk {
		return logger.WithField("id", id).WithField("name", name)
	}

	return logrus.NewEntry(logger)
}
