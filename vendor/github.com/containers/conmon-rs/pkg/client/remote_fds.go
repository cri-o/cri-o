package client

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/containers/conmon-rs/internal/proto"
)

// ErrRemoteFDsBroken gets matched by the error of a request of RemoteFDs
// which got no valid response, which got interrupted by its context after
// it was sent, or which ran into Close. Every request afterwards fails with
// it as well, a new RemoteFDs is needed then. A request which the server
// rejected, or which could not be sent, does not match it and keeps the
// connection usable.
var ErrRemoteFDsBroken = errors.New("fd socket connection broken")

var (
	errTooManyFileDescriptors  = errors.New("too many file descriptors")
	errResponseTooShort        = errors.New("response too short")
	errResponseIDDoesNotMatch  = errors.New("response id does not match")
	errNumberOfFDsDoesNotMatch = errors.New("number of fds does not match")
	errInvalidResponseLength   = errors.New("invalid response length")
)

type serverError string

func (s serverError) Error() string {
	return "server error: " + string(s)
}

const (
	uint64Bytes = 8

	maxFDs        = 253
	msgBufferSize = uint64Bytes + maxFDs*uint64Bytes + 1 // one additional byte used to detect packet truncation

	numFDsBits = 32
)

// RemoteFD represents a file descriptor on the server, identified by a slot number.
type RemoteFD uint64

// String returns the slot number of the file descriptor, like RemoteFD(3).
func (r RemoteFD) String() string {
	return fmt.Sprintf("RemoteFD(%d)", r)
}

// RemoteFDs can be used to send file descriptors to the server. It is safe
// for concurrent use, the requests are serialized. A request carries at most
// 253 file descriptors. The server holds the file descriptors of a
// connection until a request like CreateContainer takes their slots, servers
// since v1.2.1 at most 1024 which no request took yet, a request beyond that
// fails. The slots belong to the connection: closing it, or a broken
// connection, drops the file descriptors the server still holds, so keep it
// open until the requests which use the slots returned.
type RemoteFDs struct {
	conn  *net.UnixConn
	mu    sync.Mutex
	reqID uint32

	// A request which got cancelled or failed after it was sent may leave
	// its response in the connection, every later request would read the
	// wrong one.
	broken bool

	// closed is set by Close, which does not wait for a request in
	// progress.
	closed atomic.Bool
}

// NewRemoteFDs connects to the fd socket at `path`.
func NewRemoteFDs(path string) (*RemoteFDs, error) {
	conn, err := dialLongSocket("unixpacket", path)
	if err != nil {
		return nil, fmt.Errorf("dial long socket: %w", err)
	}

	return &RemoteFDs{
		conn: conn,
	}, nil
}

// Send file descriptors to the server.
func (r *RemoteFDs) Send(fds ...int) ([]RemoteFD, error) {
	return r.SendContext(context.Background(), fds...)
}

// SendContext sends file descriptors to the server. The context bounds the
// wait for the response, not the wait for a concurrent request. A request
// interrupted by the context, or one which got no valid response, breaks
// the connection, which also drops the slots of earlier requests the server
// still holds, every later request fails with an error matching
// ErrRemoteFDsBroken.
func (r *RemoteFDs) SendContext(ctx context.Context, fds ...int) ([]RemoteFD, error) {
	if len(fds) == 0 {
		return nil, nil
	}

	if len(fds) > maxFDs {
		return nil, errTooManyFileDescriptors
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed.Load() {
		return nil, fmt.Errorf("%w: closed", ErrRemoteFDsBroken)
	}

	if r.broken {
		return nil, fmt.Errorf("%w by an earlier request", ErrRemoteFDsBroken)
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	// A stuck server must not block the caller forever, the ended context
	// interrupts the response read.
	stop := context.AfterFunc(ctx, func() {
		// Fails only on a closed connection, which blocks nobody anymore.
		_ = r.conn.SetDeadline(time.Now()) //nolint:errcheck // see above
	})

	remote, err := r.send(fds)

	// The interrupt may have fired after the response arrived, either way
	// the pairing of requests and responses is gone.
	if !stop() {
		r.breakConn()

		return nil, fmt.Errorf("%w: request interrupted: %w", ErrRemoteFDsBroken, ctx.Err())
	}

	return remote, err
}

// breakConn closes the connection, whose requests and responses may not
// pair up anymore.
func (r *RemoteFDs) breakConn() {
	if !r.broken {
		r.broken = true
		r.conn.Close()
	}
}

func (r *RemoteFDs) send(fds []int) ([]RemoteFD, error) {
	r.reqID++
	reqID := r.reqID

	b := binary.LittleEndian.AppendUint64(nil, uint64(reqID)<<numFDsBits|uint64(len(fds)))
	oob := syscall.UnixRights(fds...)

	// A packet is sent as a whole or not at all, a failed one leaves no
	// response behind. Close may have closed the connection after the
	// request checked for it.
	_, _, err := r.conn.WriteMsgUnix(b, oob, nil)
	if err != nil {
		if r.closed.Load() {
			return nil, fmt.Errorf("%w: closed: send request: %w", ErrRemoteFDsBroken, err)
		}

		return nil, fmt.Errorf("send request: %w", err)
	}

	slots, err := r.receive(reqID, len(fds))

	// A server error is a complete response, anything else leaves the
	// pairing in doubt.
	if err != nil && !errors.As(err, new(serverError)) {
		r.breakConn()

		err = fmt.Errorf("%w: %w", ErrRemoteFDsBroken, err)
	}

	return slots, err
}

// receive reads the response of the request.
func (r *RemoteFDs) receive(reqID uint32, numRequested int) ([]RemoteFD, error) {
	buf := make([]byte, msgBufferSize)

	n, err := r.conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("receive response: %w", err)
	}

	buf = buf[:n]

	if len(buf) < uint64Bytes {
		return nil, errResponseTooShort
	}

	resIDAndNumFDs := binary.LittleEndian.Uint64(buf[:8])
	buf = buf[8:]

	if resID := uint32(resIDAndNumFDs >> numFDsBits); resID != reqID {
		return nil, fmt.Errorf("%w: %d (expected %d)", errResponseIDDoesNotMatch, resID, reqID)
	}

	numFDs := int(resIDAndNumFDs & (1<<numFDsBits - 1))
	if int64(numFDs) == 1<<numFDsBits-1 {
		return nil, serverError(buf)
	}

	if numFDs != numRequested {
		return nil, fmt.Errorf(
			"%w: %d (expected %d)",
			errNumberOfFDsDoesNotMatch,
			numFDs,
			numRequested,
		)
	}

	if len(buf) != numFDs*uint64Bytes {
		return nil, errInvalidResponseLength
	}

	slots := make([]RemoteFD, 0, numFDs)
	for i := range numFDs {
		slots = append(slots, RemoteFD(binary.LittleEndian.Uint64(buf[i*uint64Bytes:])))
	}

	return slots, nil
}

// Close the connection and unused remote file descriptors. A request in
// progress fails then. Closing a closed RemoteFDs does nothing.
func (r *RemoteFDs) Close() error {
	if r.closed.Swap(true) {
		return nil
	}

	// Without the lock, which a request holds while it waits for its
	// response. A broken connection got closed already.
	if err := r.conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("close fd socket: %w", err)
	}

	return nil
}

// RemoteFDs can be used to start and connect to the remote fd socket.
func (c *ConmonClient) RemoteFDs(ctx context.Context) (_ *RemoteFDs, retErr error) {
	ctx, span := c.startSpan(ctx, "RemoteFDs")
	defer endSpan(span, &retErr)

	rpcClient, err := c.client(ctx)
	if err != nil {
		return nil, fmt.Errorf("create RPC client: %w", err)
	}

	future, free := rpcClient.StartFdSocket(
		ctx,
		func(p proto.Conmon_startFdSocket_Params) (err error) {
			req, err := p.NewRequest()
			if err != nil {
				return fmt.Errorf("create request: %w", err)
			}

			if err := c.setMetadata(ctx, req); err != nil {
				return fmt.Errorf("set metadata: %w", err)
			}

			return nil
		},
	)
	defer free()

	result, err := rpcResult(future)
	if err != nil {
		return nil, err
	}

	res, err := result.Response()
	if err != nil {
		return nil, fmt.Errorf("get response: %w", err)
	}

	path, err := res.Path()
	if err != nil {
		return nil, fmt.Errorf("get path: %w", err)
	}

	r, err := NewRemoteFDs(path)
	if err != nil {
		return nil, fmt.Errorf("connect to remote fd socket: %w", err)
	}

	return r, nil
}
