// Package log provides a global interface to logging functionality
package log

import (
	"context"
	"runtime"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/trace"
)

type (
	ID   struct{}
	Name struct{}
)

func Tracef(ctx context.Context, format string, args ...any) {
	entry(ctx).Tracef(format, args...)
}

func Debugf(ctx context.Context, format string, args ...any) {
	entry(ctx).Debugf(format, args...)
}

func Infof(ctx context.Context, format string, args ...any) {
	entry(ctx).Infof(format, args...)
}

func Warnf(ctx context.Context, format string, args ...any) {
	entry(ctx).Warnf(format, args...)
}

func Errorf(ctx context.Context, format string, args ...any) {
	entry(ctx).Errorf(format, args...)
}

func Fatalf(ctx context.Context, format string, args ...any) {
	entry(ctx).Fatalf(format, args...)
}

func WithFields(ctx context.Context, fields map[string]any) *logrus.Entry {
	return entry(ctx).WithFields(fields)
}

func entry(ctx context.Context) *logrus.Entry {
	logger := logrus.StandardLogger()
	if ctx == nil {
		return logrus.NewEntry(logger)
	}

	id, idOk := ctx.Value(ID{}).(string)
	name, nameOk := ctx.Value(Name{}).(string)

	if idOk && nameOk {
		return logger.WithField("id", id).WithField("name", name).WithContext(ctx)
	}

	return logrus.NewEntry(logger).WithContext(ctx)
}

func StartSpan(ctx context.Context) (context.Context, trace.Span) {
	spanName := "unknown"
	// Use function signature as a span name if available
	if pc, _, _, ok := runtime.Caller(1); ok {
		spanName = runtime.FuncForPC(pc).Name()
	} else {
		Debugf(ctx, "Unable to retrieve a caller when starting span")
	}
	//nolint:spancheck // see https://github.com/jjti/go-spancheck/issues/7
	return trace.SpanFromContext(ctx).TracerProvider().Tracer("").Start(ctx, spanName)
}
