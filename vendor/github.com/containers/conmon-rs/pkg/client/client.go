package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/rpc"
	"github.com/containers/conmon-rs/internal/proto"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.podman.io/storage/pkg/idtools"
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
	errInvalidValue       = errors.New("invalid value")
	errRunDirNotCreated   = errors.New("could not create RunDir")
	errTimeoutWaitForPid  = errors.New("timed out waiting for server PID to disappear")
	errServerExited       = errors.New("server exited before accepting connections")
	errServerNotUp        = errors.New("server did not accept connections")
)

// ErrServerNotResponding is returned by New if a server accepts connections
// on the socket of the run directory but does not answer in time. New does
// not start another server then, it would take over the sockets of the one
// which runs and orphan its containers.
var ErrServerNotResponding = errors.New("server accepted the connection but did not respond")

// ConmonClient is the main client structure of this package.
type ConmonClient struct {
	serverPID      uint32
	runDir         string
	logger         *logrus.Logger
	attachReaders  map[uint64]*attachReaderValue
	attachReaderMu sync.Mutex
	attachReaderID atomic.Uint64
	tracingEnabled bool
	tracer         trace.Tracer
	cgroupManager  CgroupManager
	featuresMu     sync.Mutex
	features       map[string]bool
	connMu         sync.Mutex
	conn           *rpcConn
}

// ConmonServerConfig is the configuration for the conmon server instance.
type ConmonServerConfig struct {
	// ClientLogger can be set to use a custom logger rather than the
	// logrus.StandardLogger.
	ClientLogger *logrus.Logger

	// ConmonServerPath is the binary path to the conmon server.
	ConmonServerPath string

	// LogLevel of the server to be used.
	// Can be "trace", "debug", "info", "warn", "error" or "off".
	LogLevel LogLevel

	// LogDriver is the possible server logging driver.
	// Can be "stdout" or "systemd".
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

	// CgroupManager can be used to select the cgroup manager.
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
	Env []string
}

// Tracing is the structure for managing server-side OpenTelemetry tracing.
type Tracing struct {
	// Enabled tells the server to run with OpenTelemetry tracing.
	Enabled bool

	// Endpoint is the GRPC tracing endpoint for OTLP.
	// Defaults to "http://127.0.0.1:4317"
	Endpoint string

	// Tracer allows the client to create additional spans if set.
	Tracer trace.Tracer
}

// Heaptrack is the structure for configuring the memory profiling.
type Heaptrack struct {
	// Enabled tells the server to run with heaptrack enabled.
	Enabled bool

	// BinaryPath is the path to the heaptrack binary. Can be empty to lookup
	// the local $PATH variable.
	BinaryPath string

	// OutputPath is the storage path for the memory profile. Can be empty to
	// use the current directory for storing the profile.
	OutputPath string
}

// NewConmonServerConfig creates a new ConmonServerConfig instance for the
// required arguments. Optional arguments are pointing to their corresponding
// default values.
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
// If a server is already started with the same `ServerRunDir` specified
// the client connects to the existing server instead.
// Note: Other options from the `ConmonServerConfig` will be ignored
// and the settings of the existing server will remain unchanged.
//
// A new server only gets started if no server runs on the socket of the run
// directory: the socket does not exist or refuses the connection, the
// process which listens on it exited, or it closed the connection, like a
// server which shuts down, and exits or stops accepting connections within
// a few seconds. If a server which runs accepts the connection but does not
// answer within a few seconds, New returns an error matching
// ErrServerNotResponding which names the PID of the server. The caller may
// kill that process to recover, the next New starts a new server then. If
// dialing the socket fails otherwise, for example because of missing
// permissions, New returns that error. In neither case it starts a second
// server.
func New(config *ConmonServerConfig) (client *ConmonClient, retErr error) {
	cl, err := config.toClient()
	if err != nil {
		return nil, fmt.Errorf("convert config to client: %w", err)
	}
	// Check if the process has already started, and inherit that process instead.
	ctx, cancel := defaultContext()
	defer cancel()

	ctx, span := cl.startSpan(ctx, "New")
	if span != nil {
		defer span.End()
	}

	if connected, err := cl.connectRunningServer(ctx); connected || err != nil {
		if err != nil {
			return nil, err
		}

		return cl, nil
	}

	if err := cl.startServer(ctx, config); err != nil {
		return nil, fmt.Errorf("start server: %w", err)
	}

	pid, err := pidGivenFile(cl.pidFile())
	if err != nil {
		return nil, fmt.Errorf("get pid from env: %w", err)
	}

	cl.serverPID = pid

	if config.Heaptrack != nil && config.Heaptrack.Enabled {
		go cl.attachHeaptrack(config, pid)
	}

	// Cleanup the background server process
	// if we fail any of the next steps
	defer func() {
		if retErr != nil {
			if err := cl.Shutdown(); err != nil {
				cl.logger.Errorf("Unable to shutdown server: %v", err)
			}
		}
	}()

	if err := cl.waitUntilServerUp(ctx); err != nil {
		return nil, fmt.Errorf("wait until server is up: %w", err)
	}

	if err := os.Remove(cl.pidFile()); err != nil {
		return nil, fmt.Errorf("remove pid file: %w", err)
	}

	return cl, nil
}

// connectRunningServer connects to the server which listens on the socket of
// the run directory. It returns false without an error if there is none, so
// that a new one has to be started.
func (c *ConmonClient) connectRunningServer(ctx context.Context) (bool, error) {
	socketConn, err := DialLongSocket("unix", c.socket())
	if err != nil {
		if serverMissing(err) {
			return false, nil
		}

		return false, fmt.Errorf("dial server socket %s, not starting another server: %w", c.socket(), err)
	}

	// Known before the probe, which may not get an answer.
	pid, pidErr := peerPID(socketConn)

	conn := newRPCConn(ctx, socketConn)

	c.connMu.Lock()
	c.closeConnLocked()
	c.conn = conn
	c.connMu.Unlock()

	probeCtx, cancel := context.WithTimeout(ctx, serverProbeTimeout)
	defer cancel()

	resp, err := c.Version(probeCtx, &VersionConfig{})
	if err == nil {
		c.serverPID = resp.ProcessID

		return true, nil
	}

	// A failed read or write means that the server closed the connection.
	closed := conn.broken.Load()

	c.resetConn()

	if c.serverGone(ctx, pid, pidErr == nil, closed) {
		c.logger.WithError(err).Warnf("Server on %s is gone, starting a new one", c.socket())

		return false, nil
	}

	server := "PID " + strconv.FormatInt(int64(pid), 10)
	if pidErr != nil {
		server = "unknown PID: " + pidErr.Error()
	}

	return false, fmt.Errorf("%w: server %s on %s: %w", ErrServerNotResponding, server, c.socket(), err)
}

// serverGoneTimeout bounds the wait for a server which closed the probe
// connection, for example because it shuts down, to exit or to stop
// accepting connections.
const serverGoneTimeout = 3 * time.Second

// serverGone reports whether the server which accepted the probe connection
// but did not answer is gone, so that a new one can be started. That is the
// case if its process exited, also if something else still holds its
// socket, or if it closed the connection and exits or stops accepting
// connections within serverGoneTimeout.
func (c *ConmonClient) serverGone(ctx context.Context, pid int32, pidKnown, closed bool) bool {
	exited := func() bool {
		return pidKnown && processExited(uint32(pid))
	}

	if exited() {
		return true
	}

	if !closed {
		return false
	}

	waitCtx, cancel := context.WithTimeout(ctx, serverGoneTimeout)
	defer cancel()

	const interval = 50 * time.Millisecond

	for {
		if exited() {
			return true
		}

		conn, err := DialLongSocket("unix", c.socket())
		if err != nil {
			if serverMissing(err) {
				return true
			}
		} else {
			conn.Close()
		}

		select {
		case <-waitCtx.Done():
			return false
		case <-time.After(interval):
		}
	}
}

// serverMissing reports whether a dial error means that no server listens on
// the socket: it does not exist, or it is left over by one which exited.
func serverMissing(err error) bool {
	return errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED)
}

// ServerVersion contains the version data of the server instance.
type ServerVersion struct {
	// The current version.
	Version string `json:"version"`

	// The tag of the build, empty if not available.
	Tag string `json:"tag"`

	// The git commit SHA of the build.
	Commit string `json:"commit"`

	// The build date string.
	BuildDate string `json:"build_date"` //nolint:tagliatelle // Rust's serde will use that format.

	// The target triple string.
	Target string `json:"target"`

	// The used Rust version.
	RustVersion string `json:"rust_version"` //nolint:tagliatelle // Rust's serde will use that format.

	// The used Cargo version.
	CargoVersion string `json:"cargo_version"` //nolint:tagliatelle // Rust's serde will use that format.

	// The cargo dependency tree, only available in verbose output.
	CargoTree string `json:"cargo_tree"` //nolint:tagliatelle // Rust's serde will use that format.

	// The optional features the server supports, see the `Feature*`
	// constants.
	Features []string `json:"features"`
}

// versionTimeout bounds running the binary for its version.
const versionTimeout = 10 * time.Second

// Version can be used to retrieve the server version without requiring a
// running Server.
//
// If binaryPath is empty, then the default binary will be used.
// If the server doesn't support it, then an ErrUnsupported error is returned.
func Version(binaryPath string) (res *ServerVersion, err error) {
	if binaryPath == "" {
		binaryPath = binaryName
	}

	const arg = "--version-json"

	// A hanging binary must not block the caller forever.
	ctx, cancel := context.WithTimeout(context.Background(), versionTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, arg)
	// A child holding the pipes open must not keep the wait going after the
	// kill.
	cmd.WaitDelay = time.Second

	data, err := cmd.CombinedOutput()
	if err != nil {
		if bytes.Contains(data, []byte("error: unexpected argument")) {
			return nil, ErrUnsupported
		}

		return nil, fmt.Errorf("run `%s %s`: %s: %w", binaryPath, arg, string(data), err)
	}

	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("unmarshal result: %w", err)
	}

	return res, nil
}

func (c *ConmonServerConfig) toClient() (*ConmonClient, error) {
	const perm = 0o755
	if err := os.MkdirAll(c.ServerRunDir, perm); err != nil {
		return nil, fmt.Errorf("%s: %w", c.ServerRunDir, errRunDirNotCreated)
	}

	if c.ClientLogger == nil {
		c.ClientLogger = logrus.StandardLogger()
	}

	var tracer trace.Tracer
	if c.Tracing != nil && c.Tracing.Tracer != nil {
		tracer = c.Tracing.Tracer
	}

	return &ConmonClient{
		runDir:        c.ServerRunDir,
		logger:        c.ClientLogger,
		attachReaders: map[uint64]*attachReaderValue{},
		// Also set if New connects to a running server, which gets started
		// with the same configuration.
		tracingEnabled: c.Tracing != nil && c.Tracing.Enabled,
		tracer:         tracer,
		cgroupManager:  c.CgroupManager,
	}, nil
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

func (c *ConmonClient) startServer(ctx context.Context, config *ConmonServerConfig) error {
	_, span := c.startSpan(ctx, "startServer")
	if span != nil {
		defer span.End()
	}

	entrypoint, args, err := c.toArgs(config)
	if err != nil {
		return fmt.Errorf("convert config to args: %w", err)
	}

	// The server launcher gets no deadline, a loaded node must not end up
	// with a killed server. The context only carries the span.
	cmd := exec.CommandContext(context.WithoutCancel(ctx), entrypoint, args...)

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// A nil environment inherits the one of the client process.
	cmd.Env = config.Env

	if config.LogDriver == LogDriverStdout {
		release, err := c.setServerOutput(cmd, config)
		if err != nil {
			return fmt.Errorf("set server output: %w", err)
		}

		// The server inherits the files, the copies of the parent are not
		// needed once it got launched.
		defer release()
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run server command: %w", err)
	}

	return nil
}

// setServerOutput sets the stdout and stderr of the server command. The
// returned function releases the files of the parent.
func (c *ConmonClient) setServerOutput(cmd *exec.Cmd, config *ConmonServerConfig) (release func(), err error) {
	var stdout, stderr io.Writer = os.Stdout, os.Stderr

	if config.Stdout != nil {
		stdout = config.Stdout
	}

	if config.Stderr != nil {
		stderr = config.Stderr
	}

	stdoutFile, releaseStdout, err := serverOutputFile(c.logger, stdout)
	if err != nil {
		return nil, fmt.Errorf("stdout: %w", err)
	}

	cmd.Stdout = stdoutFile

	// A shared writer gets a single pipe, two copies could write to it at the
	// same time.
	if sameWriter(stdout, stderr) {
		cmd.Stderr = stdoutFile

		return releaseStdout, nil
	}

	stderrFile, releaseStderr, err := serverOutputFile(c.logger, stderr)
	if err != nil {
		releaseStdout()

		return nil, fmt.Errorf("stderr: %w", err)
	}

	cmd.Stderr = stderrFile

	return func() {
		releaseStdout()
		releaseStderr()
	}, nil
}

// serverOutputFile returns the file the server writes the output for w into.
//
// The command would copy a writer other than an *os.File through a pipe and
// wait for its end. The daemonized server outlives the command and keeps the
// pipe open, so the command would never return. Such a writer gets fed by an
// own pipe instead, whose copy runs as long as any process keeps it open:
// the server, and the processes it starts which inherit its output and may
// outlive it. The returned function closes the write end of the parent, so
// that only those keep the copy going.
func serverOutputFile(logger *logrus.Logger, w io.Writer) (file *os.File, release func(), err error) {
	if file, ok := w.(*os.File); ok {
		return file, func() {}, nil
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, nil, fmt.Errorf("create pipe: %w", err)
	}

	go func() {
		defer reader.Close()

		if _, err := io.Copy(w, reader); err != nil {
			logger.WithError(err).Debug("Unable to copy server output, discarding it")

			// The server must neither block on nor fail to write its output.
			_, _ = io.Copy(io.Discard, reader) //nolint:errcheck // nothing left to report it to
		}
	}()

	return writer, func() {
		if err := writer.Close(); err != nil {
			logger.WithError(err).Debug("Unable to close server output pipe")
		}
	}, nil
}

// sameWriter reports whether both writers are the same, like os/exec does
// it. A writer whose type is not comparable is never the same.
func sameWriter(a, b io.Writer) (same bool) {
	defer func() {
		if recover() != nil {
			same = false
		}
	}()

	return a == b
}

func (c *ConmonClient) toArgs(config *ConmonServerConfig) (entrypoint string, args []string, err error) {
	entrypoint = config.ConmonServerPath
	if entrypoint == "" {
		path, err := exec.LookPath(binaryName)
		if err != nil {
			return "", args, fmt.Errorf("finding path: %w", err)
		}

		entrypoint = path
	}

	if config.Runtime == "" {
		return "", args, errRuntimeUnspecified
	}

	args = append(args, "--runtime", config.Runtime)

	if config.ServerRunDir == "" {
		return "", args, errRunDirUnspecified
	}

	args = append(args, "--runtime-dir", config.ServerRunDir)

	if config.RuntimeRoot != "" {
		args = append(args, "--runtime-root", config.RuntimeRoot)
	}

	if config.LogLevel != "" {
		if err := validateLogLevel(config.LogLevel); err != nil {
			return "", args, fmt.Errorf("validate log level: %w", err)
		}

		args = append(args, "--log-level", string(config.LogLevel))
	}

	if config.LogDriver != "" {
		if err := validateLogDriver(config.LogDriver); err != nil {
			return "", args, fmt.Errorf("validate log driver: %w", err)
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

	return fmt.Errorf("%w: %s %q", errInvalidValue, typ, given)
}

func pidGivenFile(file string) (uint32, error) {
	pidBytes, err := os.ReadFile(file)
	if err != nil {
		return 0, fmt.Errorf("reading pid bytes: %w", err)
	}

	const (
		base    = 10
		bitSize = 32
	)

	pidU64, err := strconv.ParseUint(string(pidBytes), base, bitSize)
	if err != nil {
		return 0, fmt.Errorf("parsing pid: %w", err)
	}

	return uint32(pidU64), nil
}

func (c *ConmonClient) waitUntilServerUp(ctx context.Context) (err error) {
	_, span := c.startSpan(ctx, "waitUntilServerUp")
	if span != nil {
		defer span.End()
	}

	// The polls share their own deadline, the context only carries the span.
	pollCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), defaultTimeout)
	defer cancel()

	const (
		minInterval = time.Millisecond
		maxInterval = 100 * time.Millisecond
		backoff     = 2
	)

	for interval := minInterval; ; interval = min(backoff*interval, maxInterval) {
		_, err = c.Version(pollCtx, &VersionConfig{})
		if err == nil {
			return nil
		}

		// A dial fails right away, a crashed server would otherwise only
		// show up as a timeout with the last dial error.
		if processExited(c.serverPID) {
			return fmt.Errorf("%w: PID %d: %w", errServerExited, c.serverPID, err)
		}

		select {
		case <-pollCtx.Done():
			return fmt.Errorf("%w after %s: %w", errServerNotUp, defaultTimeout, err)
		case <-time.After(interval):
		}
	}
}

// processExited reports whether the process is gone or a zombie, which
// does not run anymore either. A process which cannot be checked counts as
// running.
func processExited(pid uint32) bool {
	if err := syscall.Kill(int(pid), 0); errors.Is(err, syscall.ESRCH) {
		return true
	}

	// Without a readable /proc, for example in a chroot, the process counts
	// as running.
	stat, err := os.ReadFile("/proc/" + strconv.FormatUint(uint64(pid), 10) + "/stat")
	if err != nil {
		return false
	}

	// The state follows the command in parentheses, which can contain any
	// character.
	state := bytes.TrimSpace(stat[bytes.LastIndexByte(stat, ')')+1:])

	return len(state) > 0 && (state[0] == 'Z' || state[0] == 'X')
}

func defaultContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), defaultTimeout)
}

// client returns the RPC client. A connection which got unusable gets
// replaced by a new one.
func (c *ConmonClient) client(ctx context.Context) (proto.Conmon, error) {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn != nil {
		if c.conn.usable() {
			return c.conn.client, nil
		}

		c.logger.Debug("RPC connection unusable, dialing a new one")
		c.closeConnLocked()
	}

	socketConn, err := DialLongSocket("unix", c.socket())
	if err != nil {
		return proto.Conmon{}, fmt.Errorf("dial long socket: %w", err)
	}

	c.conn = newRPCConn(ctx, socketConn)

	return c.conn.client, nil
}

// rpcConn is a connection to the server with its bootstrap capability.
type rpcConn struct {
	conn   *rpc.Conn
	client proto.Conmon

	// broken is set once the socket or the bootstrap capability failed,
	// either makes the connection unusable for good. Other errors of a call,
	// like a rejected request or an ended context, keep the connection for
	// the calls in flight.
	broken atomic.Bool
}

// newRPCConn creates a connection on the socket.
func newRPCConn(ctx context.Context, socket net.Conn) *rpcConn {
	r := &rpcConn{}
	r.conn = rpc.NewConn(rpc.NewStreamTransport(&watchedSocket{Conn: socket, broken: &r.broken}), nil)

	// The bootstrap capability is shared by all later calls, the end of the
	// context of the first one must not break it.
	ctx = context.WithoutCancel(ctx)
	bootstrap := r.conn.Bootstrap(ctx)
	r.client = proto.Conmon(bootstrap)

	go r.watchBootstrap(ctx, bootstrap)

	return r
}

// usable reports whether the connection can still be used. It only reads
// state recorded once, so that the calls do not contend on it.
func (r *rpcConn) usable() bool {
	if r.broken.Load() {
		return false
	}

	select {
	case <-r.conn.Done():
		return false
	default:
		return true
	}
}

// watchBootstrap marks the connection as broken if its bootstrap capability
// fails. It returns once the bootstrap resolved, which a closed connection
// makes it do as well.
func (r *rpcConn) watchBootstrap(ctx context.Context, bootstrap capnp.Client) {
	if err := bootstrap.Resolve(ctx); err != nil {
		r.broken.Store(true)

		return
	}

	// A failed bootstrap resolves to an error client, whose brand is the
	// error.
	snapshot := bootstrap.Snapshot()
	defer snapshot.Release()

	if _, failed := snapshot.Brand().Value.(error); failed {
		r.broken.Store(true)
	}
}

// watchedSocket marks the connection as broken once a read or write of its
// socket fails, for example because the server exited. That happens before
// a call fails because of it, while the connection only reports to be done
// once its shutdown finished, so a retry would get the broken one again.
type watchedSocket struct {
	net.Conn

	broken *atomic.Bool
}

func (s *watchedSocket) Read(b []byte) (int, error) {
	n, err := s.Conn.Read(b)
	if err != nil {
		s.broken.Store(true)
	}

	return n, err //nolint:wrapcheck // the transport checks for io.EOF
}

func (s *watchedSocket) Write(b []byte) (int, error) {
	n, err := s.Conn.Write(b)
	if err != nil {
		s.broken.Store(true)
	}

	return n, err //nolint:wrapcheck // passed on as it is like Read
}

// resetConn closes the connection, the next call dials a new one.
func (c *ConmonClient) resetConn() {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	c.closeConnLocked()
}

// closeConnLocked closes the connection, the caller has to hold connMu.
func (c *ConmonClient) closeConnLocked() {
	if c.conn != nil {
		if err := c.conn.conn.Close(); err != nil {
			c.logger.WithError(err).Debug("Unable to close RPC connection")
		}

		c.conn = nil
	}
}

// DialLongSocket is a wrapper around net.DialUnix.
// Its purpose is to allow for an arbitrarily long socket.
// It does so by opening the parent directory of path, and using the
// `/proc/self/fd` entry of that parent (which is a symlink to the actual parent)
// to construct the path to the socket.
// It assumes a valid path, as well as a file name that doesn't exceed the unix max socket length.
func DialLongSocket(network, path string) (*net.UnixConn, error) {
	parent := filepath.Dir(path)

	f, err := os.Open(parent)
	if err != nil {
		return nil, fmt.Errorf("open socket parent: %w", err)
	}

	defer f.Close()

	socketName := filepath.Base(path)

	const procSelfFDPath = "/proc/self/fd"

	socketPath := filepath.Join(procSelfFDPath, strconv.Itoa(int(f.Fd())), socketName)

	conn, err := net.DialUnix(network, nil, &net.UnixAddr{
		Name: socketPath, Net: network,
	})
	if err != nil {
		return nil, fmt.Errorf("dial unix socket: %w", err)
	}

	return conn, nil
}

// VersionConfig is the configuration for calling the Version method.
type VersionConfig struct {
	// Verbose specifies verbose version output.
	Verbose bool
}

// VersionResponse is the response of the Version method.
type VersionResponse struct {
	// ProcessID is the PID of the server.
	ProcessID uint32

	// Version is the actual version string of the server.
	Version string

	// Tag is the git tag of the server, empty if no tag is available.
	Tag string

	// Commit is git commit SHA of the build.
	Commit string

	// BuildDate is the date of build.
	BuildDate string

	// Target is the build triple.
	Target string

	// RustVersion is the used Rust version.
	RustVersion string

	// CargoVersion is the used Cargo version.
	CargoVersion string

	// CargoTree is the used dependency tree.
	// Only set if request was in verbose mode.
	CargoTree string

	// Features are the optional features the server supports, see the
	// `Feature*` constants.
	Features []string
}

// FeatureSdNotify is reported by servers which relay sd_notify messages.
const FeatureSdNotify = "sdnotify"

// requireServerFeature fails with ErrUnsupported if the server does not
// report the feature. The features are asked once.
func (c *ConmonClient) requireServerFeature(ctx context.Context, feature, usage string) error {
	c.featuresMu.Lock()
	defer c.featuresMu.Unlock()

	if c.features == nil {
		resp, err := c.Version(ctx, &VersionConfig{})
		if err != nil {
			return fmt.Errorf("get server version: %w", err)
		}

		c.features = make(map[string]bool, len(resp.Features))
		for _, feature := range resp.Features {
			c.features[feature] = true
		}
	}

	if !c.features[feature] {
		return fmt.Errorf("%s needs the %q feature of the server: %w", usage, feature, ErrUnsupported)
	}

	return nil
}

// Version can be used to retrieve all available version information.
func (c *ConmonClient) Version(
	ctx context.Context, cfg *VersionConfig,
) (*VersionResponse, error) {
	ctx, span := c.startSpan(ctx, "Version")
	if span != nil {
		defer span.End()
	}

	rpcClient, err := c.client(ctx)
	if err != nil {
		return nil, fmt.Errorf("create RPC client: %w", err)
	}

	future, free := rpcClient.Version(ctx, func(p proto.Conmon_version_Params) error {
		req, err := p.NewRequest()
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		if err := c.setMetadata(ctx, req); err != nil {
			return err
		}

		verbose := false
		if cfg != nil {
			verbose = cfg.Verbose
		}

		req.SetVerbose(verbose)

		return nil
	})
	defer free()

	result, err := future.Struct()
	if err != nil {
		return nil, fmt.Errorf("get result: %w", err)
	}

	response, err := result.Response()
	if err != nil {
		return nil, fmt.Errorf("get response: %w", err)
	}

	version, err := response.Version()
	if err != nil {
		return nil, fmt.Errorf("get version: %w", err)
	}

	tag, err := response.Tag()
	if err != nil {
		return nil, fmt.Errorf("get tag: %w", err)
	}

	commit, err := response.Commit()
	if err != nil {
		return nil, fmt.Errorf("get commit: %w", err)
	}

	buildDate, err := response.BuildDate()
	if err != nil {
		return nil, fmt.Errorf("get build date: %w", err)
	}

	target, err := response.Target()
	if err != nil {
		return nil, fmt.Errorf("get target: %w", err)
	}

	rustVersion, err := response.RustVersion()
	if err != nil {
		return nil, fmt.Errorf("get rust version: %w", err)
	}

	cargoVersion, err := response.CargoVersion()
	if err != nil {
		return nil, fmt.Errorf("get cargo version: %w", err)
	}

	cargoTree, err := response.CargoTree()
	if err != nil {
		return nil, fmt.Errorf("get cargo tree: %w", err)
	}

	featureList, err := response.Features()
	if err != nil {
		return nil, fmt.Errorf("get features: %w", err)
	}

	features := make([]string, 0, featureList.Len())

	for i := range featureList.Len() {
		feature, err := featureList.At(i)
		if err != nil {
			return nil, fmt.Errorf("get feature %d: %w", i, err)
		}

		features = append(features, feature)
	}

	return &VersionResponse{
		ProcessID:    response.ProcessId(),
		Version:      version,
		Tag:          tag,
		Commit:       commit,
		BuildDate:    buildDate,
		Target:       target,
		RustVersion:  rustVersion,
		CargoVersion: cargoVersion,
		CargoTree:    cargoTree,
		Features:     features,
	}, nil
}

// validateCgroupManager checks a per command cgroup manager before a request
// gets built, a validation error must not tear down the RPC connection.
func (c *ConmonClient) validateCgroupManager(cfg CgroupManager) error {
	// The server knows no per command value, it would reject the request.
	if c.cgroupManager == CgroupManagerPerCommand && cfg == CgroupManagerPerCommand {
		return ErrCgroupManagerPerCommand
	}

	return nil
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
		c.logger.Warnf(
			"cgroup manager specified in per command config, but global cgroup manager is not set to CgroupManagerPerCommand",
		)
	}

	req.SetCgroupManager(proto.Conmon_CgroupManager(cgroupManager))
}

// CreateContainerConfig is the configuration for calling the CreateContainer
// method.
type CreateContainerConfig struct {
	// ID is the container identifier.
	ID string

	// BundlePath is the path to the filesystem bundle.
	BundlePath string

	// Terminal indicates if a tty should be used or not.
	Terminal bool

	// Stdin indicates if stdin should be available or not.
	Stdin bool

	// ExitPaths is a slice of paths to write the exit statuses.
	ExitPaths []string

	// OOMExitPaths is a slice of files that should be created if the given container is OOM killed.
	OOMExitPaths []string

	// LogDrivers is a slice of selected log drivers.
	LogDrivers []ContainerLogDriver

	// CleanupCmd is the command that will be executed once the container exits
	CleanupCmd []string

	// GlobalArgs are the additional arguments passed to the create runtime call
	// before the command. e.g: crun --runtime-arg create
	GlobalArgs []string

	// CommandArgs are the additional arguments passed to the create runtime call
	// after the command. e.g: crun create --runtime-opt
	CommandArgs []string

	// EnvVars are the environment variables passed to the create runtime call.
	EnvVars map[string]string

	// CgroupManager can be used to select the cgroup manager.
	//
	// To use this option set `ConmonServerConfig.CgroupManager` to `CgroupManagerPerCommand`.
	CgroupManager CgroupManager

	// AdditionalFDs can be used to pass additional file descriptors to the container.
	AdditionalFDs []RemoteFD

	// LeakFDs can be used to keep file descriptors open as long as the container is running.
	LeakFDs []RemoteFD

	// SdNotifySocket is the path of the host sd_notify socket, usually the
	// NOTIFY_SOCKET of systemd. If set, the server binds
	// `<BundlePath>/notify/notify.sock` before it runs the runtime and relays
	// the messages sent there to the host socket until the container exits.
	// Like conmon, it only passes READY=1, RELOADING=1, STOPPING=1,
	// WATCHDOG=1, WATCHDOG=trigger, STATUS=, ERRNO=, BUSERROR= and
	// MONOTONIC_USEC=, it drops all other assignments. The caller mounts
	// the `notify` directory into the container and sets NOTIFY_SOCKET for
	// the container process accordingly.
	SdNotifySocket string
}

// ContainerLogDriver specifies a selected logging mechanism.
type ContainerLogDriver struct {
	// Type defines the log driver variant.
	Type LogDriverType

	// Path specifies the filesystem path of the log driver.
	Path string

	// MaxSize is the maximum amount of bytes to be written before rotation.
	// 0 translates to an unlimited size.
	MaxSize uint64
}

// LogDriverType specifies available log drivers.
type LogDriverType int

const (
	// LogDriverTypeContainerRuntimeInterface is the Kubernetes CRI logger
	// type.
	LogDriverTypeContainerRuntimeInterface LogDriverType = iota
	LogDriverTypeJSONLogger                LogDriverType = iota
	LogDriverTypeJournald                  LogDriverType = iota
)

// CreateContainerResponse is the response of the CreateContainer method.
type CreateContainerResponse struct {
	// PID is the container process identifier.
	PID uint32

	// NamespacesPath is the base path where the namespaces are mounted.
	//
	// Deprecated: The server never reported it, so it is always empty. Use
	// the paths returned by CreateNamespaces instead.
	NamespacesPath string
}

// CreateContainer can be used to create a new running container instance.
func (c *ConmonClient) CreateContainer(
	ctx context.Context, cfg *CreateContainerConfig,
) (*CreateContainerResponse, error) {
	ctx, span := c.startSpan(ctx, "CreateContainer")
	if span != nil {
		defer span.End()
	}

	if err := c.validateCgroupManager(cfg.CgroupManager); err != nil {
		return nil, err
	}

	// An older server ignores the field, the unit would never get ready.
	if cfg.SdNotifySocket != "" {
		if err := c.requireServerFeature(ctx, FeatureSdNotify, "sd_notify socket"); err != nil {
			return nil, err
		}
	}

	logDrivers, err := logDriversToProto(cfg.LogDrivers)
	if err != nil {
		return nil, err
	}

	rpcClient, err := c.client(ctx)
	if err != nil {
		return nil, fmt.Errorf("create RPC client: %w", err)
	}

	future, free := rpcClient.CreateContainer(ctx, func(p proto.Conmon_createContainer_Params) error {
		req, err := p.NewRequest()
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		if err := c.setMetadata(ctx, req); err != nil {
			return err
		}

		if err := req.SetId(cfg.ID); err != nil {
			return fmt.Errorf("set ID: %w", err)
		}

		if err := req.SetBundlePath(cfg.BundlePath); err != nil {
			return fmt.Errorf("set bundle path: %w", err)
		}

		req.SetTerminal(cfg.Terminal)
		req.SetStdin(cfg.Stdin)

		if err := stringSliceToTextList(cfg.ExitPaths, req.NewExitPaths); err != nil {
			return fmt.Errorf("convert exit paths string slice to text list: %w", err)
		}

		if err := stringSliceToTextList(cfg.OOMExitPaths, req.NewOomExitPaths); err != nil {
			return fmt.Errorf("convert oom exit paths string slice to text list: %w", err)
		}

		if err := initLogDrivers(&req, logDrivers); err != nil {
			return fmt.Errorf("init log drivers: %w", err)
		}

		if err := stringSliceToTextList(cfg.CleanupCmd, req.NewCleanupCmd); err != nil {
			return fmt.Errorf("convert cleanup command string slice to text list: %w", err)
		}

		if err := stringSliceToTextList(cfg.GlobalArgs, req.NewGlobalArgs); err != nil {
			return fmt.Errorf("convert global arguments string slice to text list: %w", err)
		}

		if err := stringSliceToTextList(cfg.CommandArgs, req.NewCommandArgs); err != nil {
			return fmt.Errorf("convert command arguments string slice to text list: %w", err)
		}

		if err := stringStringMapToMapEntryList(cfg.EnvVars, req.NewEnvVars); err != nil {
			return fmt.Errorf("convert environment variables to map entry list: %w", err)
		}

		c.setCgroupManager(cfg.CgroupManager, req)

		if err := remoteFDSliceToUInt64List(cfg.AdditionalFDs, req.NewAdditionalFds); err != nil {
			return fmt.Errorf("convert file descriptor slice to slot list: %w", err)
		}

		if err := remoteFDSliceToUInt64List(cfg.LeakFDs, req.NewLeakFds); err != nil {
			return fmt.Errorf("convert file descriptor slice to slot list: %w", err)
		}

		if err := req.SetSdnotifySocket(cfg.SdNotifySocket); err != nil {
			return fmt.Errorf("set sd_notify socket: %w", err)
		}

		return nil
	})
	defer free()

	result, err := future.Struct()
	if err != nil {
		return nil, fmt.Errorf("get result: %w", err)
	}

	response, err := result.Response()
	if err != nil {
		return nil, fmt.Errorf("get response: %w", err)
	}

	return &CreateContainerResponse{
		PID: response.ContainerPid(),
	}, nil
}

// ExecSyncConfig is the configuration for calling the ExecSyncContainer
// method.
type ExecSyncConfig struct {
	// ID is the container identifier.
	ID string

	// Command is a slice of command line arguments.
	Command []string

	// Timeout is the maximum time the command can run in seconds, 0 for no
	// limit. A deadline of the context limits it further, rounded down to
	// whole seconds, so that the server usually reports the timeout before
	// the context ends the call. The server needs at least one second, with
	// less time left the command may outlive the caller by less than a
	// second. A context which already ended fails the call without running
	// the command.
	Timeout uint64

	// Terminal specifies if a tty should be used.
	Terminal bool

	// EnvVars are the environment variables passed to the exec runtime call.
	EnvVars map[string]string

	// CgroupManager can be used to select the cgroup manager.
	//
	// To use this option set `ConmonServerConfig.CgroupManager` to `CgroupManagerPerCommand`.
	CgroupManager CgroupManager
}

// ExecContainerResult is the result for calling the ExecSyncContainer method.
type ExecContainerResult struct {
	// ExitCode specifies the returned exit status. It is -2 if the server
	// could not start the command at all, for example because the runtime
	// failed.
	ExitCode int32

	// Stdout contains the stdout stream result.
	Stdout []byte

	// Stderr contains the stderr stream result.
	Stderr []byte

	// TimedOut is true if the command timed out.
	TimedOut bool
}

// ExecSyncContainer can be used to execute a command within a running
// container.
func (c *ConmonClient) ExecSyncContainer(ctx context.Context, cfg *ExecSyncConfig) (*ExecContainerResult, error) {
	ctx, span := c.startSpan(ctx, "ExecSyncContainer")
	if span != nil {
		defer span.End()
	}

	if err := c.validateCgroupManager(cfg.CgroupManager); err != nil {
		return nil, err
	}

	// The command would get at least the minimum timeout of the server.
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("exec sync: %w", err)
	}

	rpcClient, err := c.client(ctx)
	if err != nil {
		return nil, fmt.Errorf("create RPC client: %w", err)
	}

	future, free := rpcClient.ExecSyncContainer(ctx, func(p proto.Conmon_execSyncContainer_Params) error {
		req, err := p.NewRequest()
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		if err := c.setMetadata(ctx, req); err != nil {
			return err
		}

		if err := req.SetId(cfg.ID); err != nil {
			return fmt.Errorf("set ID: %w", err)
		}

		req.SetTimeoutSec(execTimeout(ctx, cfg.Timeout))

		if err := stringSliceToTextList(cfg.Command, req.NewCommand); err != nil {
			return fmt.Errorf("convert command string slice to text list: %w", err)
		}

		req.SetTerminal(cfg.Terminal)

		if err := stringStringMapToMapEntryList(cfg.EnvVars, req.NewEnvVars); err != nil {
			return fmt.Errorf("convert environment variables to map entry list: %w", err)
		}

		c.setCgroupManager(cfg.CgroupManager, req)

		return nil
	})
	defer free()

	result, err := future.Struct()
	if err != nil {
		return nil, fmt.Errorf("get result: %w", err)
	}

	resp, err := result.Response()
	if err != nil {
		return nil, fmt.Errorf("get response: %w", err)
	}

	stdout, err := resp.Stdout()
	if err != nil {
		return nil, fmt.Errorf("get stdout: %w", err)
	}

	stderr, err := resp.Stderr()
	if err != nil {
		return nil, fmt.Errorf("get stderr: %w", err)
	}

	execContainerResult := &ExecContainerResult{
		ExitCode: resp.ExitCode(),
		// Copy content of both slices as the `capnp.ReleaseFunc` will
		// zero them when the deferred call to `free()` takes place.
		Stdout:   bytes.Clone(stdout),
		Stderr:   bytes.Clone(stderr),
		TimedOut: resp.TimedOut(),
	}

	return execContainerResult, nil
}

// execTimeout returns the timeout of an exec in seconds, see
// ExecSyncConfig.Timeout.
func execTimeout(ctx context.Context, timeout uint64) uint64 {
	deadline, ok := ctx.Deadline()
	if !ok {
		return timeout
	}

	// Rounded down, so that the server usually reports the timeout before
	// the context of the call ends. A short deadline still needs a timeout,
	// 0 would mean none.
	remaining := max(uint64(max(time.Until(deadline), 0)/time.Second), 1)

	if timeout == 0 {
		return remaining
	}

	return min(timeout, remaining)
}

// logDriverTypes maps the log driver types to their protocol values.
var logDriverTypes = enumTable[LogDriverType, proto.Conmon_LogDriver_Type]{
	{LogDriverTypeContainerRuntimeInterface, proto.Conmon_LogDriver_Type_containerRuntimeInterface},
	{LogDriverTypeJSONLogger, proto.Conmon_LogDriver_Type_json},
	{LogDriverTypeJournald, proto.Conmon_LogDriver_Type_journald},
}

// protoLogDriver is a log driver with its type converted for a request.
type protoLogDriver struct {
	typ     proto.Conmon_LogDriver_Type
	path    string
	maxSize uint64
}

// logDriversToProto converts the log drivers for a request, before it gets
// built.
func logDriversToProto(logDrivers []ContainerLogDriver) ([]protoLogDriver, error) {
	res := make([]protoLogDriver, 0, len(logDrivers))

	for _, logDriver := range logDrivers {
		typ, err := logDriverTypes.toProto(logDriver.Type)
		if err != nil {
			return nil, fmt.Errorf("log driver type: %w", err)
		}

		res = append(res, protoLogDriver{typ: typ, path: logDriver.Path, maxSize: logDriver.MaxSize})
	}

	return res, nil
}

func initLogDrivers(req *proto.Conmon_CreateContainerRequest, logDrivers []protoLogDriver) error {
	newLogDrivers, err := req.NewLogDrivers(int32(len(logDrivers)))
	if err != nil {
		return fmt.Errorf("create log drivers: %w", err)
	}

	for i, logDriver := range logDrivers {
		n := newLogDrivers.At(i)
		n.SetType(logDriver.typ)

		if err := n.SetPath(logDriver.path); err != nil {
			return fmt.Errorf("set log driver path: %w", err)
		}

		n.SetMaxSize(logDriver.maxSize)
	}

	return nil
}

// enumTable maps the values of a client enum to the ones of the protocol.
type enumTable[C, P comparable] []struct {
	client C
	proto  P
}

//nolint:ireturn // the type parameters are enums, not interfaces
func (t enumTable[C, P]) toProto(value C) (P, error) {
	for _, entry := range t {
		if entry.client == value {
			return entry.proto, nil
		}
	}

	var zero P

	return zero, fmt.Errorf("%w: %v", errInvalidValue, value)
}

//nolint:ireturn // the type parameters are enums, not interfaces
func (t enumTable[C, P]) fromProto(value P) (C, error) {
	for _, entry := range t {
		if entry.proto == value {
			return entry.client, nil
		}
	}

	var zero C

	return zero, fmt.Errorf("%w: %v from server", errInvalidValue, value)
}

// PID returns the server process ID.
func (c *ConmonClient) PID() uint32 {
	return c.serverPID
}

// Shutdown kills the server via SIGINT and waits up to 10 seconds for the
// server PID to be removed from the system.
func (c *ConmonClient) Shutdown() error {
	return c.ShutdownContext(context.Background())
}

// ShutdownContext kills the server via SIGINT and waits for the server PID
// to be removed from the system, up to 10 seconds or until the context ends.
func (c *ConmonClient) ShutdownContext(ctx context.Context) error {
	_, span := c.startSpan(ctx, "Shutdown")
	if span != nil {
		defer span.End()
	}

	// Taken out of the map first, so that no attach finds its reader again.
	c.attachReaderMu.Lock()
	readers := slices.Collect(maps.Values(c.attachReaders))
	clear(c.attachReaders)
	c.attachReaderMu.Unlock()

	for _, reader := range readers {
		c.closeAttachReader(reader)
	}

	c.resetConn()

	pid := int(c.serverPID)
	if err := syscall.Kill(pid, syscall.SIGINT); err != nil {
		// Process does not exist any more, it might be manually killed.
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}

		return fmt.Errorf("kill server PID: %w", err)
	}

	const (
		waitInterval = 100 * time.Millisecond
		waitCount    = 100
	)

	for range waitCount {
		// A zombie server, which nobody reaped yet, is gone as well.
		if processExited(c.serverPID) {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for server PID: %w", ctx.Err())
		case <-time.After(waitInterval):
		}
	}

	return errTimeoutWaitForPid
}

func (c *ConmonClient) pidFile() string {
	return filepath.Join(c.runDir, pidFileName)
}

func (c *ConmonClient) socket() string {
	return filepath.Join(c.runDir, socketName)
}

// ReopenLogContainerConfig is the configuration for calling the
// ReopenLogContainer method.
type ReopenLogContainerConfig struct {
	// ID is the container identifier.
	ID string
}

// ReopenLogContainer can be used to rotate all configured container log
// drivers.
func (c *ConmonClient) ReopenLogContainer(ctx context.Context, cfg *ReopenLogContainerConfig) error {
	ctx, span := c.startSpan(ctx, "ReopenLogContainer")
	if span != nil {
		defer span.End()
	}

	rpcClient, err := c.client(ctx)
	if err != nil {
		return fmt.Errorf("create RPC client: %w", err)
	}

	future, free := rpcClient.ReopenLogContainer(ctx, func(p proto.Conmon_reopenLogContainer_Params) error {
		req, err := p.NewRequest()
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		if err := c.setMetadata(ctx, req); err != nil {
			return err
		}

		if err := req.SetId(cfg.ID); err != nil {
			return fmt.Errorf("set ID: %w", err)
		}

		return nil
	})
	defer free()

	result, err := future.Struct()
	if err != nil {
		return fmt.Errorf("get result: %w", err)
	}

	if _, err := result.Response(); err != nil {
		return fmt.Errorf("get response: %w", err)
	}

	return nil
}

// requestWithMetadata is a request which carries the tracing metadata.
type requestWithMetadata interface {
	NewMetadata(n int32) (proto.Conmon_TextTextMapEntry_List, error)
}

// setMetadata sets the tracing metadata properties on the request.
func (c *ConmonClient) setMetadata(ctx context.Context, req requestWithMetadata) error {
	if !c.tracingEnabled {
		return nil
	}

	span := trace.SpanFromContext(ctx)

	if !span.SpanContext().HasSpanID() {
		return stringStringMapToMapEntryList(nil, req.NewMetadata)
	}

	m := make(map[string]string)

	c.logger.Tracef("Injecting tracing span ID %v", span.SpanContext().SpanID())
	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(m))

	if err := stringStringMapToMapEntryList(m, req.NewMetadata); err != nil {
		return fmt.Errorf("set metadata: %w", err)
	}

	return nil
}

// CreateNamespacesConfig is the configuration for calling the
// CreateNamespaces method.
type CreateNamespacesConfig struct {
	// Namespaces are the list of namespaces to unshare. A namespace listed
	// twice is created and reported once.
	Namespaces []Namespace

	// IDMappings are the user and group ID mappings when unsharing the user
	// namespace.
	IDMappings *idtools.IDMappings

	// BasePath is the root path for storing the namespaces.
	// Defaults to "/var/run" if unset.
	BasePath string

	// PodID is the unique identifier of the pod.
	PodID string
}

// CreateNamespacesResponse is the response of the CreateNamespaces method.
type CreateNamespacesResponse struct {
	// Namespaces are the created namespaces, one per requested type. A
	// server holds the namespaces of a single pod and answers later calls
	// for it from the first one, so the order is that of the first request.
	// Match the entries by their Type.
	Namespaces []*NamespacesResponse
}

// NamespacesResponse is the response data for the CreateNamespacesResponse.
type NamespacesResponse struct {
	// Namespace is the type of namespace.
	Type Namespace

	// Path is the bind mount of the namespace, which can be joined via
	// setns(2).
	Path string
}

// CreateNamespaces can be used to create a new set of unshared namespaces by
// bind mounting it to the local filesystem.
//
// Only the namespaces selected by the CreateNamespacesConfig are created and
// returned, in the requested order and without duplicates. They are mounted to
// $BASE_PATH/[ipc,pid,net,user,uts]ns/$POD_ID, where BASE_PATH and POD_ID are
// taken from the CreateNamespacesConfig. A pause process only stays around as
// init of a requested PID namespace, all other namespaces are kept alive by
// their bind mounts.
//
// UID and GID mappings are required if unsharing of the user namespace is
// requested.
func (c *ConmonClient) CreateNamespaces(
	ctx context.Context, cfg *CreateNamespacesConfig,
) (*CreateNamespacesResponse, error) {
	ctx, span := c.startSpan(ctx, "CreateNamespaces")
	if span != nil {
		defer span.End()
	}

	namespaces, err := namespacesToProto(cfg)
	if err != nil {
		return nil, err
	}

	basePath := cfg.BasePath
	if basePath == "" {
		basePath = "/var/run"
	}

	rpcClient, err := c.client(ctx)
	if err != nil {
		return nil, fmt.Errorf("create RPC client: %w", err)
	}

	future, free := rpcClient.CreateNamespaces(ctx, func(p proto.Conmon_createNamespaces_Params) error {
		req, err := p.NewRequest()
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		if err := c.setMetadata(ctx, req); err != nil {
			return err
		}

		namespaceList, err := req.NewNamespaces(int32(len(namespaces)))
		if err != nil {
			return fmt.Errorf("init namespaces: %w", err)
		}

		for i, namespace := range namespaces {
			namespaceList.Set(i, namespace)
		}

		if slices.Contains(namespaces, proto.Conmon_Namespace_user) {
			if err := stringSliceToTextList(
				mappingsToSlice(cfg.IDMappings.UIDs()),
				req.NewUidMappings,
			); err != nil {
				return fmt.Errorf("convert user ID mappings to text list: %w", err)
			}

			if err := stringSliceToTextList(
				mappingsToSlice(cfg.IDMappings.GIDs()),
				req.NewGidMappings,
			); err != nil {
				return fmt.Errorf("convert group ID mappings to text list: %w", err)
			}
		}

		if err := req.SetBasePath(basePath); err != nil {
			return fmt.Errorf("set base path: %w", err)
		}

		if err := req.SetPodId(cfg.PodID); err != nil {
			return fmt.Errorf("set pod ID: %w", err)
		}

		return nil
	})
	defer free()

	result, err := future.Struct()
	if err != nil {
		return nil, fmt.Errorf("get result: %w", err)
	}

	response, err := result.Response()
	if err != nil {
		return nil, fmt.Errorf("get response: %w", err)
	}

	responseNamespaces, err := response.Namespaces()
	if err != nil {
		return nil, fmt.Errorf("get namespaces: %w", err)
	}

	namespacesResponse := make([]*NamespacesResponse, 0, responseNamespaces.Len())

	for i := range responseNamespaces.Len() {
		namespace := responseNamespaces.At(i)

		typ, err := namespaceTypes.fromProto(namespace.Type())
		if err != nil {
			return nil, fmt.Errorf("namespace: %w", err)
		}

		path, err := namespace.Path()
		if err != nil {
			return nil, fmt.Errorf("namespace has no path: %w", err)
		}

		namespacesResponse = append(namespacesResponse,
			&NamespacesResponse{
				Type: typ,
				Path: path,
			},
		)
	}

	return &CreateNamespacesResponse{
		Namespaces: namespacesResponse,
	}, nil
}

// namespacesToProto converts the requested namespaces and checks that a
// user namespace comes with ID mappings.
func namespacesToProto(cfg *CreateNamespacesConfig) ([]proto.Conmon_Namespace, error) {
	namespaces := make([]proto.Conmon_Namespace, 0, len(cfg.Namespaces))

	for _, namespace := range cfg.Namespaces {
		typ, err := namespaceTypes.toProto(namespace)
		if err != nil {
			return nil, fmt.Errorf("namespace: %w", err)
		}

		if typ == proto.Conmon_Namespace_user && (cfg.IDMappings == nil ||
			len(cfg.IDMappings.UIDs()) == 0 ||
			len(cfg.IDMappings.GIDs()) == 0) {
			return nil, ErrMissingIDMappings
		}

		namespaces = append(namespaces, typ)
	}

	return namespaces, nil
}

// namespaceTypes maps the namespaces to their protocol values.
var namespaceTypes = enumTable[Namespace, proto.Conmon_Namespace]{
	{NamespaceIPC, proto.Conmon_Namespace_ipc},
	{NamespacePID, proto.Conmon_Namespace_pid},
	{NamespaceNet, proto.Conmon_Namespace_net},
	{NamespaceUser, proto.Conmon_Namespace_user},
	{NamespaceUTS, proto.Conmon_Namespace_uts},
}

func mappingsToSlice(mappings []idtools.IDMap) []string {
	res := make([]string, 0, len(mappings))
	for _, m := range mappings {
		res = append(res, fmt.Sprintf("%d %d %d", m.ContainerID, m.HostID, m.Size))
	}

	return res
}

// ServeExecContainerConfig is the configuration for calling the ServeExecContainer method.
type ServeExecContainerConfig struct {
	// ID is the container identifier.
	ID string

	// Command is the command to be run.
	Command []string

	// Tty indicates if a tty should be used or not.
	Tty bool

	// Stdin indicates if stdin should be available or not.
	Stdin bool

	// Stdout indicates if stdout should be available or not.
	Stdout bool

	// Stderr indicates if stderr should be available or not.
	Stderr bool

	// CgroupManager can be used to select the cgroup manager.
	//
	// To use this option set `ConmonServerConfig.CgroupManager` to
	// `CgroupManagerPerCommand`.
	CgroupManager CgroupManager
}

// ServeExecContainerResult is the result for calling the ServeExecContainer method.
type ServeExecContainerResult struct {
	// URL specifies the returned URL.
	URL string
}

// ServeExecContainer can be used to execute a command within a running container.
func (c *ConmonClient) ServeExecContainer(
	ctx context.Context,
	cfg *ServeExecContainerConfig,
) (*ServeExecContainerResult, error) {
	ctx, span := c.startSpan(ctx, "ServeExecContainer")
	if span != nil {
		defer span.End()
	}

	if err := c.validateCgroupManager(cfg.CgroupManager); err != nil {
		return nil, err
	}

	rpcClient, err := c.client(ctx)
	if err != nil {
		return nil, fmt.Errorf("create RPC client: %w", err)
	}

	future, free := rpcClient.ServeExecContainer(ctx, func(p proto.Conmon_serveExecContainer_Params) error {
		req, err := p.NewRequest()
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		if err := c.setMetadata(ctx, req); err != nil {
			return err
		}

		if err := req.SetId(cfg.ID); err != nil {
			return fmt.Errorf("set ID: %w", err)
		}

		if err := stringSliceToTextList(cfg.Command, req.NewCommand); err != nil {
			return fmt.Errorf("convert command to text list: %w", err)
		}

		req.SetTty(cfg.Tty)
		req.SetStdin(cfg.Stdin)
		req.SetStdout(cfg.Stdout)
		req.SetStderr(cfg.Stderr)

		c.setCgroupManager(cfg.CgroupManager, req)

		return nil
	})
	defer free()

	result, err := future.Struct()
	if err != nil {
		return nil, fmt.Errorf("get result: %w", err)
	}

	resp, err := result.Response()
	if err != nil {
		return nil, fmt.Errorf("get response: %w", err)
	}

	url, err := resp.Url()
	if err != nil {
		return nil, fmt.Errorf("get url: %w", err)
	}

	return &ServeExecContainerResult{URL: url}, nil
}

// ServeAttachContainerConfig is the configuration for calling the ServeAttachContainer method.
type ServeAttachContainerConfig struct {
	// ID is the container identifier.
	ID string

	// Stdin indicates if stdin should be available or not.
	Stdin bool

	// Stdout indicates if stdout should be available or not.
	Stdout bool

	// Stderr indicates if stderr should be available or not.
	Stderr bool

	// StopAfterStdinEOF indicates if the container stdin should be closed
	// once the client closes its stdin (stdin once). The output is then still
	// forwarded until the container closes it. A terminal is never closed.
	StopAfterStdinEOF bool

	// CgroupManager can be used to select the cgroup manager.
	//
	// Deprecated: Attach does not run the runtime, so the value was never
	// sent and has no effect.
	CgroupManager CgroupManager
}

// ServeAttachContainerResult is the result for calling the ServeAttachContainer method.
type ServeAttachContainerResult struct {
	// URL specifies the returned URL.
	URL string
}

// ServeAttachContainer can be used to attach to a running container.
func (c *ConmonClient) ServeAttachContainer(
	ctx context.Context,
	cfg *ServeAttachContainerConfig,
) (*ServeAttachContainerResult, error) {
	ctx, span := c.startSpan(ctx, "ServeAttachContainer")
	if span != nil {
		defer span.End()
	}

	rpcClient, err := c.client(ctx)
	if err != nil {
		return nil, fmt.Errorf("create RPC client: %w", err)
	}

	future, free := rpcClient.ServeAttachContainer(ctx, func(p proto.Conmon_serveAttachContainer_Params) error {
		req, err := p.NewRequest()
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		if err := c.setMetadata(ctx, req); err != nil {
			return err
		}

		if err := req.SetId(cfg.ID); err != nil {
			return fmt.Errorf("set ID: %w", err)
		}

		req.SetStdin(cfg.Stdin)
		req.SetStdout(cfg.Stdout)
		req.SetStderr(cfg.Stderr)
		req.SetStopAfterStdinEof(cfg.StopAfterStdinEOF)

		return nil
	})
	defer free()

	result, err := future.Struct()
	if err != nil {
		return nil, fmt.Errorf("get result: %w", err)
	}

	resp, err := result.Response()
	if err != nil {
		return nil, fmt.Errorf("get response: %w", err)
	}

	url, err := resp.Url()
	if err != nil {
		return nil, fmt.Errorf("get url: %w", err)
	}

	return &ServeAttachContainerResult{URL: url}, nil
}

// ServePortForwardContainerConfig is the configuration for calling the ServePortForwardContainer method.
type ServePortForwardContainerConfig struct {
	// NetNsPath is the path to the network namespace of the container.
	NetNsPath string
}

// ServePortForwardContainerResult is the result for calling the ServePortForwardContainer method.
type ServePortForwardContainerResult struct {
	// URL specifies the returned URL.
	URL string
}

// ServePortForwardContainer is meant to forward ports to a running container.
// The server has no implementation for it yet and always returns
// ErrUnsupported, without making a request.
func (c *ConmonClient) ServePortForwardContainer(
	ctx context.Context,
	_ *ServePortForwardContainerConfig,
) (*ServePortForwardContainerResult, error) {
	_, span := c.startSpan(ctx, "ServePortForwardContainer")
	if span != nil {
		defer span.End()
	}

	// The server would only reject the request, so it does not get sent.
	return nil, fmt.Errorf("port forwarding: %w", ErrUnsupported)
}

func (c *ConmonClient) attachHeaptrack(config *ConmonServerConfig, pid uint32) {
	c.logger.Infof("Attaching heaptrack to PID %d", pid)

	args := []string{"--record-only"}

	if config.Heaptrack.OutputPath != "" {
		args = append(args, "-o", config.Heaptrack.OutputPath)
	}

	args = append(args, "-p", strconv.FormatUint(uint64(pid), 10))

	entrypoint := config.Heaptrack.BinaryPath
	if entrypoint == "" {
		path, err := exec.LookPath(heaptrackBinaryName)
		if err != nil {
			c.logger.Errorf("Unable to find heaptrack path: %v", err)

			return
		}

		entrypoint = path
	}

	c.logger.Debugf("Running heaptrack via: %s %s", entrypoint, strings.Join(args, " "))

	cmd := exec.CommandContext(context.Background(), entrypoint, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		c.logger.Errorf("Unable to run heaptrack: %v", err)
	}
}
