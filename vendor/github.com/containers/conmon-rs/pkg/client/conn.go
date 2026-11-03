package client

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"sync/atomic"

	"capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/rpc"
	"github.com/containers/conmon-rs/internal/proto"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// client returns a reference of the RPC client, which the caller releases
// once its call is done. A connection which got unusable gets replaced by a
// new one, which records the features of the server again. A connection
// which gets reset during the call, like by Close or Shutdown, fails it as
// disconnected, see ErrServerDisconnected. A dial without a server fails
// with ErrServerUnreachable.
func (c *ConmonClient) client(ctx context.Context) (proto.Conmon, error) {
	_, rpcClient, err := c.clientConn(ctx)

	return rpcClient, err
}

// clientConn works like client and also returns the connection.
func (c *ConmonClient) clientConn(ctx context.Context) (*rpcConn, proto.Conmon, error) {
	conn, rpcClient, dialed, err := c.usableConn(ctx)
	if err != nil {
		return nil, proto.Conmon{}, err
	}

	// Without the lock, a server which does not answer must not block other
	// callers. Features which are not recorded yet get asked for by New and
	// requireServerFeature, which also asks while this refresh runs.
	if dialed && c.serverFeatures() != nil {
		c.refreshServer(ctx, conn, rpcClient)
	}

	return conn, rpcClient, nil
}

// usableConn returns the connection with a reference of its RPC client, and
// whether it got dialed for the call.
func (c *ConmonClient) usableConn(ctx context.Context) (*rpcConn, proto.Conmon, bool, error) {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	// Checked under the lock, so that no connection gets dialed after Close
	// reset the last one.
	if c.closed.Load() {
		return nil, proto.Conmon{}, false, ErrClientClosed
	}

	// The reference gets taken under the lock, before closeConnLocked can
	// release the one of the connection.
	if c.conn != nil {
		if c.conn.usable() {
			return c.conn, c.conn.client.AddRef(), false, nil
		}

		c.logger.Debug("RPC connection unusable, dialing a new one")
		c.closeConnLocked()
	}

	socketConn, err := dialLongSocket("unix", c.socket())
	if err != nil {
		if serverMissing(err) {
			err = fmt.Errorf("%w: %w", ErrServerUnreachable, err)
		}

		return nil, proto.Conmon{}, false, err
	}

	c.conn = newRPCConn(ctx, socketConn, c.logger)

	return c.conn, c.conn.client.AddRef(), true, nil
}

// paramsWithRequest are the parameters of a call, which carry its request.
type paramsWithRequest[Q requestWithMetadata] interface {
	NewRequest() (Q, error)
}

// resultsFuture is the answer of a call.
type resultsFuture[S any] interface {
	Struct() (S, error)
}

// resultsWithResponse are the results of a call, which carry its response.
type resultsWithResponse[T any] interface {
	Response() (T, error)
}

// call runs a method of the server on a reference of the RPC client, see
// client. fill sets the request after its tracing metadata, read converts
// the response before the answer gets released, so it has to copy what it
// keeps.
//
//nolint:ireturn // R is the result of the call, not an interface
func call[
	P paramsWithRequest[Q], Q requestWithMetadata,
	F resultsFuture[S], S resultsWithResponse[T], T, R any,
](
	ctx context.Context,
	c *ConmonClient,
	method func(proto.Conmon, context.Context, func(P) error) (F, capnp.ReleaseFunc),
	fill func(Q) error,
	read func(T) (R, error),
) (R, error) {
	rpcClient, err := c.client(ctx)
	if err != nil {
		var zero R

		return zero, fmt.Errorf("create RPC client: %w", err)
	}
	defer rpcClient.Release()

	return callOn(ctx, c, rpcClient, method, fill, read)
}

// callOn runs a method of the server like call, on the RPC client.
//
//nolint:ireturn // R is the result of the call, not an interface
func callOn[
	P paramsWithRequest[Q], Q requestWithMetadata,
	F resultsFuture[S], S resultsWithResponse[T], T, R any,
](
	ctx context.Context,
	c *ConmonClient,
	rpcClient proto.Conmon,
	method func(proto.Conmon, context.Context, func(P) error) (F, capnp.ReleaseFunc),
	fill func(Q) error,
	read func(T) (R, error),
) (R, error) {
	var zero R

	future, free := method(rpcClient, ctx, func(p P) error {
		req, err := p.NewRequest()
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		if err := c.setMetadata(ctx, req); err != nil {
			return err
		}

		return fill(req)
	})
	defer free()

	results, err := rpcResult(future)
	if err != nil {
		return zero, err
	}

	response, err := results.Response()
	if err != nil {
		return zero, fmt.Errorf("get response: %w", err)
	}

	return read(response)
}

// noRequest fills a request which carries nothing but the metadata.
func noRequest[Q any](Q) error {
	return nil
}

// noResponse reads a response which carries nothing.
func noResponse[T any](T) (struct{}, error) {
	return struct{}{}, nil
}

// refreshServer records the PID and the features of the server on a new
// connection, which may be another server than before, for example one
// which got restarted. Shutdown and KillContext stop that one then. If the
// server does not tell them within serverProbeTimeout, the recorded ones
// stay for SupportsFeature and the call which dialed the connection goes on,
// requireServerFeature asks the server again. The peer credentials of the
// socket are no replacement: they name the process which called listen at
// that time, which may have exited by the time the PID gets used, and the
// PID may belong to another process by then. Only the PID which the server
// reports itself is trusted.
func (c *ConmonClient) refreshServer(ctx context.Context, conn *rpcConn, rpcClient proto.Conmon) {
	ctx, cancel := context.WithTimeout(ctx, c.serverTimeouts().probe)
	defer cancel()

	resp, err := c.version(ctx, rpcClient)
	if err != nil {
		c.logger.WithError(err).
			Debug("Unable to get the features of the server, keeping the recorded ones")

		return
	}

	// A server with the recorded PID is the recorded one, unless that one
	// exited and the replacement got its PID. Checked before the lock,
	// recording the server checks that it is still the one read here.
	server := c.currentServer()
	same := server != nil && resp.ProcessID == server.pid && !c.serverExited(server)

	// Only for the connection in use, a newer one records its own.
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn != conn {
		return
	}

	c.recordFeaturesLocked(conn, resp.Features)

	if resp.ProcessID == 0 || same {
		return
	}

	if c.setServerPIDIf(server, resp.ProcessID) {
		c.logger.Warnf("Connected to server PID %d, which replaced the recorded one",
			resp.ProcessID)
	}
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

// newRPCConn creates a connection on the socket. The RPC system logs its
// errors, like a message of the server it cannot handle, to the logger.
func newRPCConn(ctx context.Context, socket net.Conn, logger logrus.FieldLogger) *rpcConn {
	r := &rpcConn{}
	r.conn = rpc.NewConn(
		rpc.NewStreamTransport(&watchedSocket{Conn: socket, broken: &r.broken}),
		&rpc.Options{Logger: rpcLogger{logger: logger}},
	)

	// The bootstrap capability is shared by all later calls, the end of the
	// context of the first one must not break it.
	ctx = context.WithoutCancel(ctx)
	bootstrap := r.conn.Bootstrap(ctx)
	r.client = proto.Conmon(bootstrap)

	go r.watchBootstrap(ctx, bootstrap)

	return r
}

// rpcLogger passes the logs of the RPC system to the logger of the client,
// at debug level: the errors it logs also fail the calls they belong to, or
// end the connection, which the next call replaces.
type rpcLogger struct {
	logger logrus.FieldLogger
}

func (l rpcLogger) Debug(message string, args ...any) { l.log("debug", message, args) }

func (l rpcLogger) Info(message string, args ...any) { l.log("info", message, args) }

func (l rpcLogger) Warn(message string, args ...any) { l.log("warn", message, args) }

func (l rpcLogger) Error(message string, args ...any) { l.log("error", message, args) }

// log logs the message with the key value pairs of the arguments as fields,
// and the level of the RPC system.
func (l rpcLogger) log(level, message string, args []any) {
	fields := logrus.Fields{"rpcLevel": level}

	for i := 0; i < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			key = fmt.Sprint(args[i])
		}

		var value any
		if i+1 < len(args) {
			value = args[i+1]
		}

		fields[key] = value
	}

	l.logger.WithFields(fields).Debug("RPC: " + message)
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

// close closes the connection and releases its bootstrap capability. Calls
// in progress, which hold references of their own, fail like on any closed
// connection.
func (r *rpcConn) close(logger logrus.FieldLogger) {
	if err := r.conn.Close(); err != nil {
		logger.WithError(err).Debug("Unable to close RPC connection")
	}

	r.client.Release()
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
		c.conn.close(c.logger)

		// The features stay for SupportsFeature, they only belonged to
		// the connection.
		c.featuresMu.Lock()
		c.featuresConn = nil
		c.featuresMu.Unlock()

		c.conn = nil
	}
}

// DialLongSocket is a wrapper around net.DialUnix.
// Its purpose is to allow for an arbitrarily long socket.
// It does so by opening the parent directory of path, and using the
// `/proc/self/fd` entry of that parent (which is a symlink to the actual parent)
// to construct the path to the socket.
// It assumes a valid path, as well as a file name that doesn't exceed the unix max socket length.
//
// Deprecated: It is an implementation detail of the client, which dials all
// sockets of the server itself. It will be removed in a future release.
func DialLongSocket(network, path string) (*net.UnixConn, error) {
	return dialLongSocket(network, path)
}

// dialLongSocket implements DialLongSocket. Its errors name the socket
// path, the address of the dial is only the /proc/self/fd alias of it.
func dialLongSocket(network, path string) (*net.UnixConn, error) {
	conn, err := dialSocketInDir(network, path)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", path, err)
	}

	return conn, nil
}

// dialSocketInDir dials the socket at path through the /proc/self/fd entry
// of its parent directory, see DialLongSocket.
func dialSocketInDir(network, path string) (*net.UnixConn, error) {
	parent := filepath.Dir(path)

	f, err := openDirPath(parent)
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
		return nil, err //nolint:wrapcheck // wrapped by dialLongSocket
	}

	return conn, nil
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
		return nil
	}

	m := make(map[string]string)

	c.logger.Tracef("Injecting tracing span ID %v", span.SpanContext().SpanID())
	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(m))

	if err := stringStringMapToMapEntryList(m, req.NewMetadata); err != nil {
		return fmt.Errorf("set metadata: %w", err)
	}

	return nil
}
