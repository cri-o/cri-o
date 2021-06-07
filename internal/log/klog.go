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
	klog.SetLogger(&logger{})
}

type logger struct{}

func (l *logger) Info(msg string, keysAndValues ...interface{}) {
	res := &strings.Builder{}
	res.WriteString(msg)
	writeKeysAndValues(res, keysAndValues...)
	logrus.Debug(res.String())
}

func (l *logger) Error(err error, msg string, keysAndValues ...interface{}) {
	res := &strings.Builder{}
	res.WriteString(msg)
	if err != nil {
		res.WriteString(": ")
		res.WriteString(err.Error())
	}
	writeKeysAndValues(res, keysAndValues...)
	logrus.Error(res.String())
}

func writeKeysAndValues(b *strings.Builder, keysAndValues ...interface{}) {
	const missingValue = "[MISSING]"
	for i := 0; i < len(keysAndValues); i += 2 {
		var v interface{}
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
			b.WriteRune(' ')
		}

		switch v.(type) {
		case string, error:
			b.WriteString(fmt.Sprintf("%s=%q", k, v))
		case []byte:
			b.WriteString(fmt.Sprintf("%s=%+q", k, v))
		default:
			if _, ok := v.(fmt.Stringer); ok {
				b.WriteString(fmt.Sprintf("%s=%q", k, v))
			} else {
				b.WriteString(fmt.Sprintf("%s=%+v", k, v))
			}
		}

		if i == len(keysAndValues) {
			b.WriteRune(')')
		}
	}
}

func (l *logger) Enabled() bool                         { return true }
func (l *logger) V(level int) logr.Logger               { return l }
func (l *logger) WithValues(...interface{}) logr.Logger { return l }
func (l *logger) WithName(string) logr.Logger           { return l }
