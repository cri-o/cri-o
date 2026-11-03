package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/sirupsen/logrus"
)

// serverProcess is a server process with a known PID.
type serverProcess struct {
	pid uint32
	// pidfd refers to the process, nil if pidfds are not supported. Unlike
	// the PID, it never refers to another process which got the PID once
	// the server exited. It gets closed once the server is known to be
	// gone, got replaced by another one or the client got closed, and no
	// Shutdown or KillContext uses it anymore, see usePidfd.
	pidfd *os.File
	// done is set once the process is known to be gone.
	done atomic.Bool
	// users counts the Shutdown and KillContext calls in progress, guarded
	// by the pidfdMu of the client.
	users int
	// startTime is when the process started, see processStartTime, 0 if
	// it is unknown. Without a pidfd, it tells the server apart from a
	// process which got its PID later.
	startTime uint64
}

// setServerPID records the PID of the server and opens a pidfd for it. The
// pidfd of the server recorded before gets closed once nothing uses it
// anymore.
func (c *ConmonClient) setServerPID(pid uint32) {
	server := c.newServerProcess(pid)

	c.pidfdMu.Lock()
	defer c.pidfdMu.Unlock()

	c.replaceServerLocked(server)
}

// setServerPIDIf records the PID of the server like setServerPID, but only
// if the recorded server is still old, so that a server which got recorded
// in the meantime stays. It reports whether it recorded the PID.
func (c *ConmonClient) setServerPIDIf(old *serverProcess, pid uint32) bool {
	server := c.newServerProcess(pid)

	c.pidfdMu.Lock()
	defer c.pidfdMu.Unlock()

	if c.server != old {
		if server.pidfd != nil {
			server.pidfd.Close()
		}

		return false
	}

	c.replaceServerLocked(server)

	return true
}

// newServerProcess returns the server process of the PID with a pidfd.
func (c *ConmonClient) newServerProcess(pid uint32) *serverProcess {
	server := &serverProcess{pid: pid, startTime: processStartTime(pid)}

	var gone bool

	server.pidfd, gone = openPidfd(c.logger, pid)
	// A server which exited already must not get signaled through its PID,
	// which may belong to another process by now.
	server.done.Store(gone)

	return server
}

// replaceServerLocked records the server process, the caller has to hold
// pidfdMu.
func (c *ConmonClient) replaceServerLocked(server *serverProcess) {
	old := c.server
	c.server = server

	// A closed client releases the new one right away.
	for _, server := range []*serverProcess{old, server} {
		if err := c.closeUnusedPidfdLocked(server); err != nil {
			c.logger.WithError(err).Warn("Unable to close server pidfd")
		}
	}
}

// currentServer returns the server process, nil while its PID is unknown.
func (c *ConmonClient) currentServer() *serverProcess {
	c.pidfdMu.Lock()
	defer c.pidfdMu.Unlock()

	return c.server
}

// connectRunningServer connects to the server which listens on the socket of
// the run directory. It returns false without an error if there is none, so
// that a new one has to be started.
func (c *ConmonClient) connectRunningServer(ctx context.Context) (bool, error) {
	socketConn, err := dialLongSocket("unix", c.socket())
	if err != nil {
		if serverMissing(err) {
			return false, nil
		}

		// The backlog of the socket is full, the server does not accept
		// the connections it got.
		if errors.Is(err, syscall.EAGAIN) {
			return false, fmt.Errorf("%w: %w", ErrServerNotResponding, err)
		}

		return false, fmt.Errorf("not starting another server: %w", err)
	}

	// Known before the probe, which may not get an answer.
	pid, pidErr := peerPID(socketConn)

	conn := newRPCConn(ctx, socketConn, c.logger)

	c.connMu.Lock()
	c.closeConnLocked()
	c.conn = conn
	c.connMu.Unlock()

	probeCtx, cancel := context.WithTimeout(ctx, c.serverTimeouts().probe)
	defer cancel()

	resp, err := c.Version(probeCtx, &VersionConfig{})
	if err == nil {
		c.setServerPID(resp.ProcessID)
		c.setFeatures(resp.Features)

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

	return false, fmt.Errorf(
		"%w: server %s on %s: %w",
		ErrServerNotResponding,
		server,
		c.socket(),
		err,
	)
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

	waitCtx, cancel := context.WithTimeout(ctx, c.serverTimeouts().gone)
	defer cancel()

	const interval = 50 * time.Millisecond

	for {
		if exited() {
			return true
		}

		conn, err := dialLongSocket("unix", c.socket())
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
// the socket: it does not exist, it is left over by one which exited or it
// is no socket at all, which all refuse the connection, or it is no stream
// socket. The server replaces such a socket on start, see socket_in_use in
// conmon-rs/server/src/config.rs.
func serverMissing(err error) bool {
	return errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.EPROTOTYPE)
}

func (c *ConmonClient) startServer(
	ctx context.Context,
	config *ConmonServerConfig,
) (retErr error) {
	ctx, span := c.startSpan(ctx, "startServer")
	defer endSpan(span, &retErr)

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

	cmd.Env = serverEnv(config.Env)

	// The errors of the launch, like an invalid configuration, which the
	// server reports on stderr before it forks.
	var errFile *os.File

	if serverLogDriver(config.LogDriver, cmd.Env) == LogDriverStdout {
		release, err := c.setServerOutput(cmd, config)
		if err != nil {
			return fmt.Errorf("set server output: %w", err)
		}

		// The server inherits the files, the copies of the parent are not
		// needed once it got launched.
		defer release()
	} else if file, fileErr := serverErrorFile(); fileErr != nil {
		c.logger.WithError(fileErr).
			Debug("Unable to create the server error file, discarding its errors")
	} else {
		// Only the launch writes to it, the forked server replaces its
		// stdio with /dev/null right away, see Server::start in
		// conmon-rs/server/src/server.rs. Older servers keep it as stderr
		// until they exit, so it then also holds what they write there
		// after the fork. The copy of the client is not needed after the
		// launch.
		defer file.Close()

		errFile = file
		cmd.Stderr = file
	}

	if err := cmd.Run(); err != nil {
		if output := readServerErrors(errFile); output != "" {
			return fmt.Errorf("run server command: %w: %s", err, output)
		}

		return fmt.Errorf("run server command: %w", err)
	}

	return nil
}

// serverLogDriver returns the log driver of the server launch: the one of
// the configuration, or else the one CONMON_LOG_DRIVER of the environment
// names, whose last entry is the one the command passes, or else the default
// of the server.
func serverLogDriver(driver LogDriver, env []string) LogDriver {
	if driver != "" {
		return driver
	}

	for _, entry := range slices.Backward(env) {
		if value, ok := strings.CutPrefix(entry, "CONMON_LOG_DRIVER="); ok {
			return LogDriver(value)
		}
	}

	return LogDriverSystemd
}

// serverLaunchEnvDenied are the variables which make the server launch exit
// without a server or never return: the server reads its flags from CONMON_
// variables as well, see conmon-rs/server/src/config.rs.
var serverLaunchEnvDenied = []string{"CONMON_VERSION", "CONMON_VERSION_JSON", "CONMON_SKIP_FORK"}

// serverEnv returns the environment of the server launch: the one of the
// configuration, or of the client process if nil, without the variables of
// serverLaunchEnvDenied.
func serverEnv(env []string) []string {
	if env == nil {
		env = os.Environ()
	}

	return slices.DeleteFunc(slices.Clone(env), func(entry string) bool {
		name, _, _ := strings.Cut(entry, "=")

		return slices.Contains(serverLaunchEnvDenied, name)
	})
}

// tempErrorFile returns an unlinked temporary file for the errors of the
// server launch. Unlike a pipe, the launch does not wait for the forked
// server to close it.
func tempErrorFile() (*os.File, error) {
	file, err := os.CreateTemp("", "conmonrs-errors-")
	if err != nil {
		return nil, fmt.Errorf("create temporary file: %w", err)
	}

	if err := os.Remove(file.Name()); err != nil {
		file.Close()

		return nil, fmt.Errorf("remove temporary file: %w", err)
	}

	return file, nil
}

// maxServerErrors bounds how much of the server error file ends up in the
// error of New.
const maxServerErrors = 4096

// readServerErrors returns the end of what the server wrote into the error
// file, at most maxServerErrors bytes, empty if there is no file or nothing
// could be read.
func readServerErrors(file *os.File) string {
	if file == nil {
		return ""
	}

	info, err := file.Stat()
	if err != nil {
		return ""
	}

	offset := max(info.Size()-maxServerErrors, 0)
	buf := make([]byte, info.Size()-offset)

	// The server shares the file offset, which ReadAt does not use.
	n, err := file.ReadAt(buf, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return ""
	}

	return errorTail(string(buf[:n]), offset > 0)
}

// errorTail returns the trimmed end of an error output, at most
// maxServerErrors bytes of it. A "..." in front tells that it got cut, here
// or by the caller, which sets cut then. A cut drops what it left of a split
// character.
func errorTail(output string, cut bool) string {
	output = strings.TrimSpace(output)
	if len(output) > maxServerErrors {
		output = output[len(output)-maxServerErrors:]
		cut = true
	}

	if cut {
		for i := 0; i < utf8.UTFMax-1 && output != "" && !utf8.RuneStart(output[0]); i++ {
			output = output[1:]
		}

		output = strings.TrimSpace(output)
	}

	if cut && output != "" {
		output = "..." + output
	}

	return output
}

// setServerOutput sets the stdout and stderr of the server command. The
// returned function releases the files of the parent.
func (c *ConmonClient) setServerOutput(
	cmd *exec.Cmd,
	config *ConmonServerConfig,
) (release func(), err error) {
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
func serverOutputFile(
	logger logrus.FieldLogger,
	w io.Writer,
) (file *os.File, release func(), err error) {
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

// waitUntilServerUp polls the server until it answers, up to 10 seconds or
// until the context ends, and returns its answer.
func (c *ConmonClient) waitUntilServerUp(
	ctx context.Context,
) (_ *VersionResponse, retErr error) {
	ctx, span := c.startSpan(ctx, "waitUntilServerUp")
	defer endSpan(span, &retErr)

	pollCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	for interval := minWaitInterval; ; interval = nextWaitInterval(interval) {
		resp, err := c.Version(pollCtx, &VersionConfig{})
		if err == nil {
			return resp, nil
		}

		// A dial fails right away, a crashed server would otherwise only
		// show up as a timeout with the last dial error.
		if server := c.currentServer(); server != nil && server.pid != 0 &&
			c.serverExited(server) {
			return nil, fmt.Errorf("%w: PID %d: %w", ErrServerExited, server.pid, err)
		}

		select {
		case <-pollCtx.Done():
			if ctx.Err() != nil {
				return nil, fmt.Errorf("%w: %w: %w", ErrServerNotUp, ctx.Err(), err)
			}

			return nil, fmt.Errorf("%w after %s: %w", ErrServerNotUp, defaultTimeout, err)
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
	stat := processStat(pid)

	return len(stat) > 0 && (stat[0][0] == 'Z' || stat[0][0] == 'X')
}

// processStartTime returns when the process started after the boot, in
// clock ticks, 0 if it is unknown. A process which got the PID of another
// one which exited started later.
func processStartTime(pid uint32) uint64 {
	// The start time is the 22nd field, the 20th after the command.
	const startTimeField = 22 - 3

	stat := processStat(pid)
	if len(stat) <= startTimeField {
		return 0
	}

	start, err := strconv.ParseUint(string(stat[startTimeField]), 10, 64)
	if err != nil {
		return 0
	}

	return start
}

// processStat returns the fields of /proc/<pid>/stat after the command,
// starting with the state, none if it cannot be read.
func processStat(pid uint32) [][]byte {
	stat, err := os.ReadFile("/proc/" + strconv.FormatUint(uint64(pid), 10) + "/stat")
	if err != nil {
		return nil
	}

	// The fields follow the command in parentheses, which can contain any
	// character.
	return bytes.Fields(stat[bytes.LastIndexByte(stat, ')')+1:])
}

// Shutdown kills the server via SIGINT and waits up to 10 seconds for the
// server PID to be removed from the system, see ShutdownContext.
func (c *ConmonClient) Shutdown() error {
	return c.ShutdownContext(context.Background())
}

// ShutdownContext kills the server via SIGINT and waits for the server PID
// to be removed from the system, up to 10 seconds or until the context ends.
// The server kills its containers before it exits. If it is still running
// after the 10 seconds, ShutdownContext returns ErrShutdownTimeout,
// KillContext ends it then.
//
// It fails with ErrServerPIDUnknown if the server PID is unknown, without
// signaling anything or touching the attach sessions and the connection.
// Where the kernel supports pidfds, the signal cannot reach another process
// which got the PID after the server exited. It closes the stdin of the
// attach sessions of the client and its connection first, the attach
// sessions end once the server exited. If the recorded server exited and
// another one, for example a restarted one, answers on the socket of the run
// directory within a few seconds, that one gets stopped.
func (c *ConmonClient) ShutdownContext(ctx context.Context) (retErr error) {
	ctx, span := c.startSpan(ctx, "Shutdown")
	defer endSpan(span, &retErr)

	return c.endServer(ctx, "stop server", func(server *serverProcess) error {
		return c.stopServer(ctx, server, serverStopTimeout, false)
	})
}

// Kill kills the server via SIGKILL and waits up to 10 seconds for the
// server PID to be removed from the system, see KillContext.
func (c *ConmonClient) Kill() error {
	return c.KillContext(context.Background())
}

// KillContext kills the server via SIGKILL and waits for the server PID to
// be removed from the system, up to 10 seconds or until the context ends.
// Unlike with ShutdownContext, the server cannot kill its containers, which
// keep running without a monitor. Use it if ShutdownContext returned
// ErrShutdownTimeout. It returns ErrShutdownTimeout itself if the server
// still runs after the 10 seconds, for example because it is stuck in the
// kernel. Like ShutdownContext, it closes the stdin of the attach sessions
// and the connection of the client first, stops a server which replaced the
// recorded one, and fails with ErrServerPIDUnknown without touching anything
// if the server PID is unknown.
func (c *ConmonClient) KillContext(ctx context.Context) (retErr error) {
	ctx, span := c.startSpan(ctx, "Kill")
	defer endSpan(span, &retErr)

	return c.endServer(ctx, "kill server", func(server *serverProcess) error {
		return c.killServer(ctx, server, serverStopTimeout)
	})
}

// endServer runs end, which signals the server and waits for it to exit, for
// ShutdownContext and KillContext. It picks up a server which replaced the
// recorded one first, and fails with ErrServerPIDUnknown behind the action
// without touching anything if the server PID is unknown. Otherwise it closes
// the stdin of the attach sessions and the connection before end.
func (c *ConmonClient) endServer(
	ctx context.Context, action string, end func(*serverProcess) error,
) error {
	c.pickUpReplacement(ctx)

	server, release, err := c.usePidfd()
	if err != nil {
		return err
	}
	defer release()

	// A zero PID would signal the process group of the caller. Nothing ends
	// while nothing can be stopped.
	if server == nil || server.pid == 0 {
		return fmt.Errorf("%s: %w", action, ErrServerPIDUnknown)
	}

	c.closeAttachStdins()
	c.resetConn()

	return end(server)
}

// Close releases the resources of the client without stopping the server:
// it ends the attach sessions of the client, and closes its connection to
// the server and its pidfd of the server. The server and its containers
// keep running, a later New for the same ServerRunDir connects to the
// server again. A client which is not needed anymore has to be closed or
// shut down, otherwise its resources leak.
//
// The methods of the client fail with ErrClientClosed afterwards, calls in
// progress fail as well, attach sessions with ErrClientClosed. A Shutdown or
// KillContext (or Kill) in progress still finishes. Close is safe to call at
// any time: on a closed client it does nothing, after Shutdown or KillContext
// (or Kill) it closes the client for good. Those do not mark the client as
// closed, its methods connect again to a server which runs on the
// ServerRunDir later. Close does not close a RemoteFDs which the RemoteFDs
// method returned, the caller closes it.
func (c *ConmonClient) Close() error {
	// Set before taking pidfdMu, so that usePidfd fails once Close got the
	// lock.
	if c.closed.Swap(true) {
		return nil
	}

	c.attachSessionMu.Lock()
	sessions := slices.Collect(maps.Values(c.attachSessions))
	clear(c.attachSessions)
	c.attachSessionMu.Unlock()

	for _, session := range sessions {
		session.cancel(ErrClientClosed)
	}

	c.resetConn()

	c.pidfdMu.Lock()
	defer c.pidfdMu.Unlock()

	return c.closeUnusedPidfdLocked(c.server)
}

// pickUpReplacement records the server which replaced the recorded one,
// if that one exited and another server answers Version on the socket of
// the run directory within serverProbeTimeout. Shutdown and KillContext
// stop the server which runs then, also if no call connected to it yet.
func (c *ConmonClient) pickUpReplacement(ctx context.Context) {
	server := c.currentServer()
	if server == nil || c.closed.Load() || !c.serverExited(server) {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, c.serverTimeouts().probe)
	defer cancel()

	socketConn, err := dialLongSocket("unix", c.socket())
	if err != nil {
		return
	}

	conn := newRPCConn(ctx, socketConn, c.logger)

	defer conn.close(c.logger)

	// The recorded server exited, an answering one replaced it, also one
	// which got the same PID, which the new pidfd tells apart.
	resp, err := c.version(ctx, conn.client)
	if err != nil || resp.ProcessID == 0 {
		return
	}

	// Another Shutdown, KillContext or connection may have recorded a
	// server in the meantime, which stays.
	if c.setServerPIDIf(server, resp.ProcessID) {
		c.logger.Warnf("Server PID %d exited, server PID %d replaced it",
			server.pid, resp.ProcessID)
	}
}

// closeAttachStdins closes the stdin of the attach sessions of the client.
func (c *ConmonClient) closeAttachStdins() {
	c.attachSessionMu.Lock()
	sessions := slices.Collect(maps.Values(c.attachSessions))
	c.attachSessionMu.Unlock()

	for _, session := range sessions {
		session.closeStdin()
	}
}

// usePidfd returns the server process, nil if its PID is unknown, and keeps
// its pidfd open until release gets called, so that neither Close nor a
// connection to another server can close it while the server gets signaled
// or waited for. It fails with ErrClientClosed after Close.
func (c *ConmonClient) usePidfd() (server *serverProcess, release func(), err error) {
	c.pidfdMu.Lock()
	defer c.pidfdMu.Unlock()

	if c.closed.Load() {
		return nil, nil, ErrClientClosed
	}

	server = c.server
	if server == nil {
		return nil, func() {}, nil
	}

	server.users++

	return server, func() {
		c.pidfdMu.Lock()
		defer c.pidfdMu.Unlock()

		server.users--

		if err := c.closeUnusedPidfdLocked(server); err != nil {
			c.logger.WithError(err).Warn("Unable to close server pidfd")
		}
	}, nil
}

// closeUnusedPidfdLocked closes the pidfd of the server process once it is
// not needed anymore, because the server is gone, got replaced by another
// one or the client got closed, and nothing uses it. The caller has to hold
// pidfdMu.
func (c *ConmonClient) closeUnusedPidfdLocked(server *serverProcess) error {
	if server == nil || server.pidfd == nil || server.users > 0 ||
		(!server.done.Load() && !c.closed.Load() && server == c.server) {
		return nil
	}

	if err := server.pidfd.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		return fmt.Errorf("close server pidfd: %w", err)
	}

	return nil
}

// serverStopTimeout bounds the wait for the server to exit on Shutdown. The
// server finishes its shutdown before, it checks its budget against a copy
// of this value at compile time, see CLIENT_STOP_TIMEOUT in
// conmon-rs/server/src/server.rs.
const serverStopTimeout = 10 * time.Second

// serverCleanupTimeout bounds each wait for a server which New started but
// failed to connect to, first after SIGINT and then after SIGKILL.
const serverCleanupTimeout = 3 * time.Second

// stopServer sends SIGINT to the server and waits for it to exit, up to the
// timeout or until the context ends. With kill, a server which is still
// running then gets SIGKILL. The server is only signaled through its PID if
// pidfds are not supported. The caller keeps the pidfd open, see usePidfd.
func (c *ConmonClient) stopServer(
	ctx context.Context,
	server *serverProcess,
	timeout time.Duration,
	kill bool,
) error {
	// A zero PID would signal the process group of the caller.
	if server == nil || server.pid == 0 {
		return fmt.Errorf("stop server: %w", ErrServerPIDUnknown)
	}

	if err := server.signal(syscall.SIGINT); err != nil {
		// Process does not exist any more, it might be manually killed.
		if errors.Is(err, syscall.ESRCH) {
			c.serverGoneForGood(server)

			return nil
		}

		return fmt.Errorf("signal server PID %d: %w", server.pid, err)
	}

	err := c.waitServerExited(ctx, server, timeout)
	if err == nil || !kill || !errors.Is(err, ErrShutdownTimeout) {
		return err
	}

	c.logger.Warnf("Server PID %d did not exit on SIGINT, sending SIGKILL", server.pid)

	return c.killServer(ctx, server, timeout)
}

// killServer sends SIGKILL to the server and waits for it to exit, up to the
// timeout or until the context ends.
func (c *ConmonClient) killServer(
	ctx context.Context,
	server *serverProcess,
	timeout time.Duration,
) error {
	if err := server.signal(syscall.SIGKILL); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			c.serverGoneForGood(server)

			return nil
		}

		return fmt.Errorf("kill server PID %d: %w", server.pid, err)
	}

	return c.waitServerExited(ctx, server, timeout)
}

// The first and the longest interval between the checks of waitServerExited
// and waitUntilServerUp, and how much the interval grows after each check.
const (
	minWaitInterval = time.Millisecond
	maxWaitInterval = 100 * time.Millisecond
	waitBackoff     = 2
)

// nextWaitInterval grows the interval between the checks of
// waitServerExited and waitUntilServerUp, up to maxWaitInterval.
func nextWaitInterval(interval time.Duration) time.Duration {
	return min(waitBackoff*interval, maxWaitInterval)
}

// waitServerExited waits for the server to exit, up to the timeout or until
// the context ends. A server mostly exits right after the signal, the checks
// back off from there, so that stopping it does not take a whole interval.
func (c *ConmonClient) waitServerExited(
	ctx context.Context,
	server *serverProcess,
	timeout time.Duration,
) error {
	deadline := time.Now().Add(timeout)

	for interval := minWaitInterval; ; interval = nextWaitInterval(interval) {
		// A zombie server, which nobody reaped yet, is gone as well.
		if c.serverExited(server) {
			c.serverGoneForGood(server)

			return nil
		}

		if time.Now().After(deadline) {
			return ErrShutdownTimeout
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for server PID: %w", ctx.Err())
		case <-time.After(interval):
		}
	}
}

// signal sends the signal to the server. It fails with ESRCH if the server
// exited. The caller keeps the pidfd open, see usePidfd.
func (s *serverProcess) signal(sig syscall.Signal) error {
	// A server known to be gone never gets signaled through its PID, which
	// may belong to another process by now.
	if s.done.Load() {
		return syscall.ESRCH
	}

	if s.pidfd == nil {
		return syscall.Kill(int(s.pid), sig) //nolint:wrapcheck // wrapped by the caller
	}

	return pidfdSendSignal(s.pidfd, sig)
}

// serverExited reports whether the server exited or is a zombie. Without a
// pidfd, or without usePidfd, which keeps it from getting closed
// concurrently, the check falls back to the PID. A process which got the
// PID later counts as an exited server if its start time differs, which
// cannot be read everywhere, so that an exited server can still look
// running. Only Shutdown and KillContext, which keep the pidfd open, signal
// a server based on it.
func (c *ConmonClient) serverExited(server *serverProcess) bool {
	if server.done.Load() {
		return true
	}

	if server.pidfd != nil {
		exited, err := pidfdExited(server.pidfd)
		if err == nil {
			return exited
		}

		c.logger.WithError(err).Debug("Unable to poll server pidfd, checking its PID")
	}

	if processExited(server.pid) {
		return true
	}

	// Another process which got the PID started later.
	start := processStartTime(server.pid)

	return server.startTime != 0 && start != 0 && start != server.startTime
}

// serverGoneForGood marks a server which is known to be gone, so that it
// never gets signaled again, and releases its pidfd once nothing uses it.
func (c *ConmonClient) serverGoneForGood(server *serverProcess) {
	server.done.Store(true)

	c.pidfdMu.Lock()
	defer c.pidfdMu.Unlock()

	if err := c.closeUnusedPidfdLocked(server); err != nil {
		c.logger.WithError(err).Warn("Unable to close server pidfd")
	}
}

func (c *ConmonClient) pidFile() string {
	return filepath.Join(c.runDir, pidFileName)
}

func (c *ConmonClient) socket() string {
	return filepath.Join(c.runDir, socketName)
}

func (c *ConmonClient) attachHeaptrack(ctx context.Context, heaptrack Heaptrack, pid uint32) {
	c.logger.Infof("Attaching heaptrack to PID %d", pid)

	args := []string{"--record-only"}

	if heaptrack.OutputPath != "" {
		args = append(args, "-o", heaptrack.OutputPath)
	}

	args = append(args, "-p", strconv.FormatUint(uint64(pid), 10))

	entrypoint := heaptrack.BinaryPath
	if entrypoint == "" {
		path, err := exec.LookPath(heaptrackBinaryName)
		if err != nil {
			c.logger.Errorf("Unable to find heaptrack path: %v", err)

			return
		}

		entrypoint = path
	}

	c.logger.Debugf("Running heaptrack via: %s %s", entrypoint, strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, entrypoint, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		c.logger.Errorf("Unable to run heaptrack: %v", err)
	}
}
