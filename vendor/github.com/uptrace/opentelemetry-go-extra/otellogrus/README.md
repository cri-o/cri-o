[![PkgGoDev](https://pkg.go.dev/badge/github.com/uptrace/opentelemetry-go-extra/otellogrus)](https://pkg.go.dev/github.com/uptrace/opentelemetry-go-extra/otellogrus)

# OpenTelemetry Go instrumentation for logrus logging

This instrumentation records logrus log messages as events on the existing span that is passed via a
`context.Context`. It does not record anything if a context does not contain a span.

## Installation

```shell
go get github.com/uptrace/opentelemetry-go-extra/otellogrus
```

## Usage

You need to install an `otellogrus.Hook` and use `logrus.WithContext` to propagate the active span.

```go
import (
    "github.com/uptrace/opentelemetry-go-extra/otellogrus"
    "github.com/sirupsen/logrus"
)

// Instrument logrus.
logrus.AddHook(otellogrus.NewHook(otellogrus.WithLevels(
	logrus.PanicLevel,
	logrus.FatalLevel,
	logrus.ErrorLevel,
	logrus.WarnLevel,
)))

// Use ctx to pass the active span.
logrus.WithContext(ctx).
	WithError(errors.New("hello world")).
	WithField("foo", "bar").
	Error("something failed")
```

See [example](./example/) for details.

## Options

[otellogrus.NewHook](https://pkg.go.dev/github.com/uptrace/opentelemetry-go-extra/otellogrus#NewHook)
accepts the following
[options](https://pkg.go.dev/github.com/uptrace/opentelemetry-go-extra/otellogrus#Option):

- `otellogrus.WithLevels(logrus.PanicLevel, logrus.FatalLevel, logrus.ErrorLevel, logrus.WarnLevel)`
  sets the logrus logging levels on which the hook is fired.
- `WithErrorStatusLevel(logrus.ErrorLevel)` sets the minimal logrus logging level on which the span
  status is set to codes.Error.
