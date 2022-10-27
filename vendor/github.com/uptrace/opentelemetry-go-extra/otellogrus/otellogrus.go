package otellogrus

import (
	"reflect"
	"strings"

	"github.com/sirupsen/logrus"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/uptrace/opentelemetry-go-extra/otelutil"
)

var (
	logSeverityKey = attribute.Key("log.severity")
	logMessageKey  = attribute.Key("log.message")
)

// Hook is a logrus hook that adds logs to the active span as events.
type Hook struct {
	levels           []logrus.Level
	errorStatusLevel logrus.Level
}

var _ logrus.Hook = (*Hook)(nil)

// NewHook returns a logrus hook.
func NewHook(opts ...Option) *Hook {
	hook := &Hook{
		levels: []logrus.Level{
			logrus.PanicLevel,
			logrus.FatalLevel,
			logrus.ErrorLevel,
			logrus.WarnLevel,
		},
		errorStatusLevel: logrus.ErrorLevel,
	}

	for _, fn := range opts {
		fn(hook)
	}

	return hook
}

// Fire is a logrus hook that is fired on a new log entry.
func (hook *Hook) Fire(entry *logrus.Entry) error {
	ctx := entry.Context
	if ctx == nil {
		return nil
	}

	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return nil
	}

	attrs := make([]attribute.KeyValue, 0, len(entry.Data)+2+3)

	attrs = append(attrs, logSeverityKey.String(levelString(entry.Level)))
	attrs = append(attrs, logMessageKey.String(entry.Message))

	if entry.Caller != nil {
		if entry.Caller.Function != "" {
			attrs = append(attrs, semconv.CodeFunctionKey.String(entry.Caller.Function))
		}
		if entry.Caller.File != "" {
			attrs = append(attrs, semconv.CodeFilepathKey.String(entry.Caller.File))
			attrs = append(attrs, semconv.CodeLineNumberKey.Int(entry.Caller.Line))
		}
	}

	for k, v := range entry.Data {
		if k == "error" {
			if err, ok := v.(error); ok {
				typ := reflect.TypeOf(err).String()
				attrs = append(attrs, semconv.ExceptionTypeKey.String(typ))
				attrs = append(attrs, semconv.ExceptionMessageKey.String(err.Error()))
				continue
			}
		}

		attrs = append(attrs, otelutil.Attribute(k, v))
	}

	span.AddEvent("log", trace.WithAttributes(attrs...))

	if entry.Level <= hook.errorStatusLevel {
		span.SetStatus(codes.Error, entry.Message)
	}

	return nil
}

// Levels returns logrus levels on which this hook is fired.
func (hook *Hook) Levels() []logrus.Level {
	return hook.levels
}

func levelString(lvl logrus.Level) string {
	s := lvl.String()
	if s == "warning" {
		s = "warn"
	}
	return strings.ToUpper(s)
}
