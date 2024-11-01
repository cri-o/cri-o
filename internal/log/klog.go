package log

import (
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
	"k8s.io/klog/v2"
)

// InitKlogShim creates a shim between logrus and klog by forwarding klog
// messages to the logrus logger. To reduce the overall verbosity we log every
// Info klog message in logrus Debug verbosity.
func InitKlogShim() {
	klog.LogToStderr(false)
	klog.SetLogger(logr.New(&logSink{}))
}

type logSink struct{}

func (l *logSink) Info(level int, msg string, keysAndValues ...any) {
	res := &strings.Builder{}
	res.WriteString(msg)
	writeKeysAndValues(res, keysAndValues...)
	logrus.Debug(res.String())
}

func (l *logSink) Error(err error, msg string, keysAndValues ...any) {
	res := &strings.Builder{}
	res.WriteString(msg)
	if err != nil {
		res.WriteString(": ")
		res.WriteString(err.Error())
	}
	writeKeysAndValues(res, keysAndValues...)
	logrus.Error(res.String())
}

func writeKeysAndValues(b *strings.Builder, keysAndValues ...any) {
	const missingValue = "[MISSING]"
	for i := 0; i < len(keysAndValues); i += 2 {
		var v any
		k := keysAndValues[i]
		if i+1 < len(keysAndValues) {
			v = keysAndValues[i+1]
		} else {
			v = missingValue
		}
		if i == 0 {
			b.WriteString(" (")
		}
		if i > 0 {
			b.WriteByte(' ')
		}

		switch v.(type) {
		case string, error:
			fmt.Fprintf(b, "%s=%q", k, v)
		case []byte:
			fmt.Fprintf(b, "%s=%+q", k, v)
		default:
			if _, ok := v.(fmt.Stringer); ok {
				fmt.Fprintf(b, "%s=%q", k, v)
			} else {
				fmt.Fprintf(b, "%s=%+v", k, v)
			}
		}

		if i == len(keysAndValues) {
			b.WriteByte(')')
		}
	}
}

func (l *logSink) Init(logr.RuntimeInfo)          {}
func (l *logSink) Enabled(int) bool               { return true }
func (l *logSink) WithValues(...any) logr.LogSink { return l }
func (l *logSink) WithName(string) logr.LogSink   { return l }
