package log

import (
	"fmt"
	"io"
	"regexp"

	"github.com/sirupsen/logrus"
)

type FilterHook struct {
	custom     *regexp.Regexp
	predefined *regexp.Regexp
}

// NewFilterHook creates a new default FilterHook.
func NewFilterHook(filter string) (*FilterHook, error) {
	var (
		custom *regexp.Regexp
		err    error
	)

	if filter != "" {
		custom, err = regexp.Compile(filter)
		logrus.Debugf("Using log filter: %q", custom)

		if err != nil {
			return nil, fmt.Errorf("custom log level filter does not compile: %w", err)
		}
	}

	predefined := regexp.MustCompile(`\[[\d\s]+\]`)

	return &FilterHook{custom, predefined}, nil
}

// Levels returns the levels for which the hook is activated. This contains
// currently only the DebugLevel.
func (f *FilterHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire executes the hook for every logrus entry.
func (f *FilterHook) Fire(entry *logrus.Entry) error {
	// Custom specified filters get skipped completely
	if f.custom != nil && !f.custom.MatchString(entry.Message) {
		*entry = logrus.Entry{
			Logger: &logrus.Logger{
				Out:       io.Discard,
				Formatter: &logrus.JSONFormatter{},
			},
		}
	}

	// Apply pre-defined filters
	if entry.Level == logrus.DebugLevel {
		entry.Message = f.predefined.ReplaceAllString(entry.Message, "[FILTERED]")
	}

	return nil
}
