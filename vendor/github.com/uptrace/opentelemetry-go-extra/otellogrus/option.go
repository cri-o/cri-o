package otellogrus

import "github.com/sirupsen/logrus"

// Option applies a configuration to the given config.
type Option func(h *Hook)

// WithLevels sets the logrus logging levels on which the hook is fired.
//
// The default is all levels between logrus.PanicLevel and logrus.WarnLevel inclusive.
func WithLevels(levels ...logrus.Level) Option {
	return func(h *Hook) {
		h.levels = levels
	}
}

// WithErrorStatusLevel sets the minimal logrus logging level on which
// the span status is set to codes.Error.
//
// The default is <= logrus.ErrorLevel.
func WithErrorStatusLevel(level logrus.Level) Option {
	return func(h *Hook) {
		h.errorStatusLevel = level
	}
}
