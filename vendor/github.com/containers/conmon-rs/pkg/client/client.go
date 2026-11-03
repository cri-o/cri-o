package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/containers/conmon-rs/internal/proto"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	binaryName          = "conmonrs"
	socketName          = "conmon.sock"
	pidFileName         = "pidfile"
	defaultTimeout      = 10 * time.Second
	heaptrackBinaryName = "heaptrack"
)

var (
	errRuntimeUnspecified = errors.New("runtime must be specified")
	errRunDirUnspecified  = errors.New("RunDir must be specified")
	errRunDirNotCreated   = errors.New("could not create RunDir")
)

// ConmonClient is the main client structure of this package, see the package
// documentation.
type ConmonClient struct {
	// server is the server process, nil while its PID is unknown. It gets
	// replaced once the client connects to another server, for example one
	// which got restarted, guarded by pidfdMu.
	server          *serverProcess
	pidfdMu         sync.Mutex
	closed          atomic.Bool
	runDir          string
	logger          logrus.Ext1FieldLogger
	attachSessions  map[uint64]*attachSession
	attachSessionMu sync.Mutex
	attachSessionID atomic.Uint64
	tracingEnabled  bool
	tracer          trace.Tracer
	cgroupManager   CgroupManager
	// cgroupManagerWarning warns once about per command cgroup managers
	// which get ignored.
	cgroupManagerWarning sync.Once
	// features are the recorded features of the server and featuresConn
	// the connection they got recorded on, guarded by featuresMu, which
	// gets taken after connMu.
	featuresMu   sync.Mutex
	features     map[string]bool
	featuresConn *rpcConn
	connMu       sync.Mutex
	conn         *rpcConn
	// timeouts are the waits for servers which do not answer or go away,
	// nil for the default ones, see serverTimeouts.
	timeouts *serverTimeouts
}

// ConmonServerConfig is the configuration for the conmon server instance.
type ConmonServerConfig struct {
	// ClientLogger can be set to use a custom logger rather than the
	// logrus.StandardLogger. ClientFieldLogger takes precedence.
	ClientLogger *logrus.Logger

	// ClientFieldLogger can be set to log through an entry with fields, like
	// a *logrus.Entry, rather than a logger. It takes precedence over
	// ClientLogger.
	ClientFieldLogger logrus.Ext1FieldLogger

	// ConmonServerPath is the binary path to the conmon server. If empty,
	// conmonrs gets looked up in $PATH.
	ConmonServerPath string

	// LogLevel of the server to be used, see the LogLevel constants.
	// Can be "trace", "debug", "info", "warn", "error" or "off".
	LogLevel LogLevel

	// LogDriver is the possible server logging driver, see the LogDriver
	// constants. Can be "none", "stdout" or "systemd".
	LogDriver LogDriver

	// Runtime is the binary path of the OCI runtime to use to operate on the
	// containers.
	Runtime string

	// RuntimeRoot is the root directory used by the OCI runtime to operate on
	// containers.
	RuntimeRoot string

	// ServerRunDir is the path of the directory for the server to hold files
	// at runtime.
	ServerRunDir string

	// Stdout is the standard output stream of the server when the log driver
	// "stdout" is being used, os.Stdout if nil.
	//
	// An *os.File is passed to the server as it is. Any other writer gets fed
	// through a pipe by a goroutine of the client process, as long as any
	// process holds the pipe open: the server and the processes it starts
	// which inherit its output, like the pause process or cleanup commands,
	// which can outlive the server. Such a writer has to stay usable for that
	// time and must be safe to write to from another goroutine. It does not
	// get closed. The output is then tied to the client process: once it
	// exits, for example on a restart, the output is lost silently.
	Stdout io.Writer

	// Stderr is the standard error stream of the server when the log driver
	// "stdout" is being used, os.Stderr if nil. It works like Stdout, the same
	// writer can be used for both.
	Stderr io.Writer

	// CgroupManager can be used to select the cgroup manager. The zero value
	// is CgroupManagerSystemd.
	CgroupManager CgroupManager

	// Tracing can be used to enable OpenTelemetry tracing.
	Tracing *Tracing

	// Heaptrack can be used to memory profile the server.
	Heaptrack *Heaptrack

	// Env is the environment of the server, in the form of "key=value". The
	// server passes it on to the OCI runtime, its hooks and the cleanup
	// commands, so callers should pass a curated environment rather than
	// their own. If nil, the server inherits the environment of the client
	// process, an empty slice starts it without any.
	//
	// Variables named after a flag of the server with the CONMON_ prefix,
	// like CONMON_LOG_LEVEL for --log-level, configure the server like the
	// flag, unless the client passes that flag itself. CONMON_LOG_DRIVER
	// set to "stdout" uses Stdout and Stderr like LogDriverStdout. The
	// client drops CONMON_VERSION, CONMON_VERSION_JSON and CONMON_SKIP_FORK,
	// with which the server would print its version or stay in the
	// foreground instead of starting in the background.
	Env []string

	// timeouts replaces the default waits of the client for servers which do
	// not answer or go away, which the tests shorten.
	timeouts *serverTimeouts
}

// serverTimeouts are the waits of a client for servers which do not answer
// or go away.
type serverTimeouts struct {
	// probe bounds a Version call to a server which accepted the connection,
	// see serverProbeTimeout.
	probe time.Duration

	// gone bounds the wait for a server which closed the probe connection,
	// see serverGoneTimeout.
	gone time.Duration

	// cleanup bounds each wait for a server which New started but failed to
	// connect to, see serverCleanupTimeout.
	cleanup time.Duration
}

// serverTimeouts returns the waits of the client for servers which do not
// answer or go away.
func (c *ConmonClient) serverTimeouts() serverTimeouts {
	if c.timeouts != nil {
		return *c.timeouts
	}

	return serverTimeouts{
		probe:   serverProbeTimeout,
		gone:    serverGoneTimeout,
		cleanup: serverCleanupTimeout,
	}
}

// Tracing is the structure for managing server-side OpenTelemetry tracing.
type Tracing struct {
	// Enabled tells the server to run with OpenTelemetry tracing. The client
	// then passes the span of the context of each call to the server, through
	// the global propagator of OpenTelemetry. The server extracts the W3C
	// Trace Context, so the caller has to register that propagator, for
	// example with otel.SetTextMapPropagator(propagation.TraceContext{}).
	// The default global propagator passes nothing, the spans of the server
	// do not join the trace of the call then. Server builds without the
	// tracing feature ignore it, the static release binaries and the RPMs
	// are such builds.
	Enabled bool

	// Endpoint is the gRPC tracing endpoint for OTLP.
	// Defaults to "http://127.0.0.1:4317"
	Endpoint string

	// Tracer allows the client to create additional spans if set.
	Tracer trace.Tracer
}

// Heaptrack is the structure for configuring the memory profiling.
type Heaptrack struct {
	// Enabled attaches heaptrack to a server which New starts, as a child of
	// the client process.
	Enabled bool

	// BinaryPath is the path to the heaptrack binary. Can be empty to lookup
	// the local $PATH variable.
	BinaryPath string

	// OutputPath is the storage path for the memory profile. Can be empty to
	// use the current directory for storing the profile.
	OutputPath string
}

// NewConmonServerConfig creates a new ConmonServerConfig instance for the
// required arguments. It sets the LogLevel to LogLevelDebug, which is more
// verbose than the LogLevelInfo the server defaults to, the LogDriver to
// LogDriverSystemd and Stdout and Stderr to os.Stdout and os.Stderr. The
// other fields keep their zero values.
func NewConmonServerConfig(
	runtime, runtimeRoot, serverRunDir string,
) *ConmonServerConfig {
	return &ConmonServerConfig{
		LogLevel:     LogLevelDebug,
		LogDriver:    LogDriverSystemd,
		Runtime:      runtime,
		RuntimeRoot:  runtimeRoot,
		ServerRunDir: serverRunDir,
		Stdout:       os.Stdout,
		Stderr:       os.Stderr,
	}
}

// FromLogrusLevel converts the logrus.Level to a conmon-rs server log level.
func FromLogrusLevel(level logrus.Level) LogLevel {
	switch level {
	case logrus.PanicLevel, logrus.FatalLevel:
		return LogLevelOff

	case logrus.ErrorLevel:
		return LogLevelError

	case logrus.WarnLevel:
		return LogLevelWarn

	case logrus.InfoLevel:
		return LogLevelInfo

	case logrus.DebugLevel:
		return LogLevelDebug

	case logrus.TraceLevel:
		return LogLevelTrace
	}

	return LogLevelDebug
}

// serverProbeTimeout bounds the Version call New uses to check a server which
// accepted the connection. A running server answers within milliseconds, the
// call does not wait for any container. A few seconds cover a loaded host,
// while a caller which holds a lock during New, like CRI-O, does not stall
// for the whole default timeout. A server which is slower than that is not
// replaced either, New returns ErrServerNotResponding.
const serverProbeTimeout = 3 * time.Second

// New creates a new conmon server, starts it and connects a new client to it.
//
// If a server is already started with the same
// [ConmonServerConfig.ServerRunDir] the client connects to the existing server
// instead. The other options of the [ConmonServerConfig] are ignored then and
// the settings of the existing server remain unchanged.
//
// A new server only gets started if no server runs on the socket of the run
// directory: the socket does not exist or refuses the connection, the
// process which listens on it exited, or it closed the connection, like a
// server which shuts down, and exits or stops accepting connections within
// a few seconds. If a server which runs accepts the connection but does not
// answer within a few seconds, New returns an error matching
// ErrServerNotResponding which names the PID of the server. The caller may
// kill that process to recover, the next New starts a new server then. A
// socket whose backlog is full, of a server which does not accept the
// connections, makes New return ErrServerNotResponding as well. If
// dialing the socket fails otherwise, for example because of missing
// permissions, New returns that error. In neither case it starts a second
// server.
//
// If New started a server but fails afterwards, it stops the server again,
// with SIGKILL if SIGINT does not end it within a few seconds. New removes
// the pidfile in the ServerRunDir, which the server launch writes, before
// the launch and once it returns, but keeps it for a server which it could
// not stop.
func New(config *ConmonServerConfig) (*ConmonClient, error) {
	return NewContext(context.Background(), config)
}

// NewContext works like New. The context bounds the check of a running
// server and the wait for a started one to accept connections, which
// otherwise take up to 10 seconds each. A server which NewContext started
// keeps running once it returned, but if the context ends before the server
// answers, NewContext stops it again like on any other failure.
//
// Concurrent calls for the same ServerRunDir are not supported: each one
// may start a server, and the PID file of one may name the server of the
// other, which the cleanup of a failing call then stops, or get removed by
// the other before it got read.
func NewContext(
	ctx context.Context,
	config *ConmonServerConfig,
) (client *ConmonClient, retErr error) {
	if config == nil {
		return nil, ErrMissingConfig
	}

	cl, err := config.toClient()
	if err != nil {
		return nil, fmt.Errorf("convert config to client: %w", err)
	}

	ctx, span := cl.startSpan(ctx, "New")
	defer endSpan(span, &retErr)

	// Check if the process has already started, and inherit that process instead.
	probeCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	connected, err := cl.connectRunningServer(probeCtx)

	cancel()

	// A probe which ran out of time tells about the server, one which got
	// cancelled with the caller does not, nor may a server get started then.
	if !connected {
		if ctxErr := ctx.Err(); ctxErr != nil {
			if err != nil {
				return nil, fmt.Errorf("connect to server: %w: %w", ctxErr, err)
			}

			return nil, fmt.Errorf("connect to server: %w", ctxErr)
		}
	}

	if connected || err != nil {
		if err != nil {
			return nil, err
		}

		return cl, nil
	}

	// A PID file left behind, for example by a New which failed, would name
	// a process which may not exist anymore, or another one which got its
	// PID, if the launch does not write a new one.
	cl.removePIDFile()

	if err = cl.startServer(ctx, config); err != nil {
		return nil, fmt.Errorf("start server: %w", err)
	}

	// Cleanup the background server process
	// if we fail any of the next steps
	defer func() {
		if retErr != nil {
			cl.stopFailedServer(context.WithoutCancel(ctx))
		}
	}()

	// The PID file names the server before it answers, so that a crash shows
	// up right away. The server itself tells its PID once it is up, which is
	// the one to use: a concurrent New on the same run directory may have
	// replaced the file.
	if pid, pidErr := pidGivenFile(cl.pidFile()); pidErr != nil {
		cl.logger.WithError(pidErr).
			Warn("Unable to get server PID from file, waiting for the server")
	} else {
		cl.setServerPID(pid)
	}

	// Heaptrack follows the server, also if it reports another PID than the
	// file named. Its goroutines get a copy of the configuration, which the
	// caller may change once New returned.
	heaptrackEnabled := config.Heaptrack != nil && config.Heaptrack.Enabled

	var heaptrack Heaptrack
	if heaptrackEnabled {
		heaptrack = *config.Heaptrack
	}

	heaptrackPID := cl.PID()

	if heaptrackEnabled && heaptrackPID != 0 {
		go cl.attachHeaptrack(context.WithoutCancel(ctx), heaptrack, heaptrackPID)
	}

	resp, err := cl.waitUntilServerUp(ctx)
	if err != nil {
		return nil, fmt.Errorf("wait until server is up: %w", err)
	}

	if filePID := cl.PID(); resp.ProcessID != filePID {
		if filePID != 0 {
			cl.logger.Warnf(
				"Server reports PID %d, the PID file named %d",
				resp.ProcessID,
				filePID,
			)
		}

		cl.setServerPID(resp.ProcessID)
	}

	cl.setFeatures(resp.Features)

	if heaptrackEnabled && heaptrackPID != resp.ProcessID {
		go cl.attachHeaptrack(context.WithoutCancel(ctx), heaptrack, resp.ProcessID)
	}

	cl.removePIDFile()

	return cl, nil
}

// stopFailedServer stops the server which New started but failed to
// connect to, and removes its PID file once it is gone.
func (c *ConmonClient) stopFailedServer(ctx context.Context) {
	c.resetConn()

	err := c.stopServer(ctx, c.currentServer(), c.serverTimeouts().cleanup, true)
	// The PID file of a server which may still run names it, until the next
	// launch removes the file.
	if err != nil {
		c.logger.Errorf("Unable to shutdown server, keeping its PID file: %v", err)

		return
	}

	c.removePIDFile()
}

// removePIDFile removes the PID file which the launch of the server writes,
// once it is not needed anymore or before the next launch.
func (c *ConmonClient) removePIDFile() {
	if err := os.Remove(c.pidFile()); err != nil && !errors.Is(err, fs.ErrNotExist) {
		c.logger.WithError(err).Warn("Unable to remove server PID file")
	}
}

func (c *ConmonServerConfig) toClient() (*ConmonClient, error) {
	if c.ServerRunDir == "" {
		return nil, errRunDirUnspecified
	}

	if !validCgroupManager(c.CgroupManager, true) {
		return nil, fmt.Errorf("%w: cgroup manager %v", ErrInvalidValue, c.CgroupManager)
	}

	// The server resolves a relative path when it starts, the client has to
	// find the socket in the same place later, also after a change of its
	// working directory.
	runDir, err := filepath.Abs(c.ServerRunDir)
	if err != nil {
		return nil, fmt.Errorf("make RunDir absolute: %w", err)
	}

	const perm = 0o755
	if err := os.MkdirAll(runDir, perm); err != nil {
		return nil, fmt.Errorf("%w: %w", errRunDirNotCreated, err)
	}

	var logger logrus.Ext1FieldLogger = logrus.StandardLogger()

	// A typed nil, like a nil *logrus.Entry, counts as not set.
	switch {
	case c.ClientFieldLogger != nil && !isNilPointer(c.ClientFieldLogger):
		logger = c.ClientFieldLogger
	case c.ClientLogger != nil:
		logger = c.ClientLogger
	}

	var tracer trace.Tracer
	if c.Tracing != nil && c.Tracing.Tracer != nil {
		tracer = c.Tracing.Tracer
	}

	// A copy, which the configuration cannot change afterwards.
	var timeouts *serverTimeouts
	if c.timeouts != nil {
		copied := *c.timeouts
		timeouts = &copied
	}

	return &ConmonClient{
		runDir:         runDir,
		timeouts:       timeouts,
		logger:         logger,
		attachSessions: map[uint64]*attachSession{},
		// Also set if New connects to a running server, which gets started
		// with the same configuration.
		tracingEnabled: c.Tracing != nil && c.Tracing.Enabled,
		tracer:         tracer,
		cgroupManager:  c.CgroupManager,
	}, nil
}

// isNilPointer reports whether the value is a nil pointer.
func isNilPointer(value any) bool {
	v := reflect.ValueOf(value)

	return v.Kind() == reflect.Pointer && v.IsNil()
}

// traceEnabled reports whether the logger logs trace messages, so that the
// paths which run for every packet do not build entries with fields which
// get dropped. A logger which cannot tell counts as enabled.
func (c *ConmonClient) traceEnabled() bool {
	switch logger := c.logger.(type) {
	case *logrus.Logger:
		return logger.IsLevelEnabled(logrus.TraceLevel)
	case *logrus.Entry:
		return logger.Logger == nil || logger.Logger.IsLevelEnabled(logrus.TraceLevel)
	default:
		return true
	}
}

//nolint:ireturn,nolintlint // Returning the interface is intentional
func (c *ConmonClient) startSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	if c.tracer == nil {
		return ctx, nil
	}

	const prefix = "conmonrs-client: "

	//nolint:spancheck // https://github.com/jjti/go-spancheck/issues/7
	return c.tracer.Start(ctx, prefix+name, trace.WithSpanKind(trace.SpanKindClient))
}

// endSpan records the error the call returned, if any, and ends the span of
// startSpan, which is nil without a tracer.
//
//nolint:gocritic // the pointer gets read when the deferred call runs
func endSpan(span trace.Span, err *error) {
	if span == nil {
		return
	}

	if *err != nil {
		span.RecordError(*err)
		span.SetStatus(codes.Error, (*err).Error())
	}

	span.End()
}

func (c *ConmonClient) toArgs(
	config *ConmonServerConfig,
) (entrypoint string, args []string, err error) {
	entrypoint = config.ConmonServerPath
	if entrypoint == "" {
		path, err := exec.LookPath(binaryName)
		if err != nil {
			return "", nil, fmt.Errorf("finding path: %w", err)
		}

		entrypoint = path
	}

	if config.Runtime == "" {
		return "", nil, errRuntimeUnspecified
	}

	// The absolute path of the client, which a later change of its working
	// directory does not move. toClient rejected an empty ServerRunDir.
	args = append(args, "--runtime", config.Runtime, "--runtime-dir", c.runDir)

	if config.RuntimeRoot != "" {
		args = append(args, "--runtime-root", config.RuntimeRoot)
	}

	if config.LogLevel != "" {
		if err := validateLogLevel(config.LogLevel); err != nil {
			return "", nil, fmt.Errorf("validate log level: %w", err)
		}

		args = append(args, "--log-level", string(config.LogLevel))
	}

	if config.LogDriver != "" {
		if err := validateLogDriver(config.LogDriver); err != nil {
			return "", nil, fmt.Errorf("validate log driver: %w", err)
		}

		args = append(args, "--log-driver", string(config.LogDriver))
	}

	if config.Tracing != nil && config.Tracing.Enabled {
		args = append(args, "--enable-tracing")

		if config.Tracing.Endpoint != "" {
			args = append(args, "--tracing-endpoint", config.Tracing.Endpoint)
		}
	}

	return entrypoint, args, nil
}

func validateLogLevel(level LogLevel) error {
	return validateStringSlice(
		"log level",
		string(level),
		string(LogLevelTrace),
		string(LogLevelDebug),
		string(LogLevelInfo),
		string(LogLevelWarn),
		string(LogLevelError),
		string(LogLevelOff),
	)
}

func validateLogDriver(driver LogDriver) error {
	return validateStringSlice(
		"log driver",
		string(driver),
		string(LogDriverNone),
		string(LogDriverStdout),
		string(LogDriverSystemd),
	)
}

func validateStringSlice(typ, given string, possibleValues ...string) error {
	if slices.Contains(possibleValues, given) {
		return nil
	}

	return fmt.Errorf("%w: %s %q", ErrInvalidValue, typ, given)
}

// validateCgroupManager checks a per command cgroup manager before a request
// gets built, a validation error must not tear down the RPC connection. Only
// a value which gets sent is checked, others get ignored, see
// setCgroupManager.
func (c *ConmonClient) validateCgroupManager(cfg CgroupManager) error {
	if c.cgroupManager != CgroupManagerPerCommand {
		return nil
	}

	// The server knows no per command value, it would reject the request.
	if cfg == CgroupManagerPerCommand {
		return ErrCgroupManagerPerCommand
	}

	if !validCgroupManager(cfg, false) {
		return fmt.Errorf("%w: cgroup manager %v", ErrInvalidValue, cfg)
	}

	return nil
}

// validCgroupManager reports whether the cgroup manager is one the client
// knows, CgroupManagerPerCommand only if allowed, which only the global one
// may be.
func validCgroupManager(manager CgroupManager, allowPerCommand bool) bool {
	switch manager {
	case CgroupManagerSystemd, CgroupManagerCgroupfs:
		return true
	case CgroupManagerPerCommand:
		return allowPerCommand
	default:
		return false
	}
}

func (c *ConmonClient) setCgroupManager(
	cfg CgroupManager,
	req interface {
		SetCgroupManager(manager proto.Conmon_CgroupManager)
	},
) {
	cgroupManager := c.cgroupManager
	if cgroupManager == CgroupManagerPerCommand {
		cgroupManager = cfg
	} else if cfg != CgroupManager(0) {
		c.cgroupManagerWarning.Do(func() {
			c.logger.Warn(
				"cgroup manager specified in per command config, but global cgroup manager " +
					"is not set to CgroupManagerPerCommand, ignoring it (logged once)",
			)
		})
	}

	req.SetCgroupManager(proto.Conmon_CgroupManager(cgroupManager))
}

// PID returns the server process ID. It changes once the client connects to
// another server, for example one which got restarted on the same
// ServerRunDir, which Shutdown and KillContext then stop.
func (c *ConmonClient) PID() uint32 {
	if server := c.currentServer(); server != nil {
		return server.pid
	}

	return 0
}
