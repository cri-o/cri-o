package client

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/containers/conmon-rs/internal/proto"
)

var (
	errConnectionBroken        = errors.New("fd socket connection broken by an interrupted request")
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

func (r RemoteFD) String() string {
	return fmt.Sprintf("RemoteFD(%d)", r)
}

// RemoteFDs can be used to send file descriptors to the server. It is safe
// for concurrent use, the requests are serialized.
type RemoteFDs struct {
	conn  *net.UnixConn
	mu    sync.Mutex
	reqID uint32

	// A request which got cancelled midway leaves its response in the
	// connection, every later request would read the wrong one.
	broken bool
}

// NewRemoteFDs connects to the fd socket at `path`.
func NewRemoteFDs(path string) (*RemoteFDs, error) {
	conn, err := DialLongSocket("unixpacket", path)
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
// interrupted by the context breaks the connection, which also drops the
// slots of earlier requests the server still holds, every later request
// fails with an error.
func (r *RemoteFDs) SendContext(ctx context.Context, fds ...int) ([]RemoteFD, error) {
	if len(fds) == 0 {
		return nil, nil
	}

	if len(fds) > maxFDs {
		return nil, errTooManyFileDescriptors
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.broken {
		return nil, errConnectionBroken
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
		r.broken = true
		r.conn.Close()

		return nil, fmt.Errorf("request interrupted: %w", ctx.Err())
	}

	return remote, err
}

func (r *RemoteFDs) send(fds []int) ([]RemoteFD, error) {
	r.reqID++
	reqID := r.reqID

	b := binary.LittleEndian.AppendUint64(nil, uint64(reqID)<<numFDsBits|uint64(len(fds)))
	oob := syscall.UnixRights(fds...)

	_, _, err := r.conn.WriteMsgUnix(b, oob, nil)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

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

	if numFDs != len(fds) {
		return nil, fmt.Errorf("%w: %d (expected %d)", errNumberOfFDsDoesNotMatch, numFDs, len(fds))
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

// Close the connection and unused remote file descriptors.
func (r *RemoteFDs) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// An interrupted request closed the connection already.
	if r.broken {
		return nil
	}

	if err := r.conn.Close(); err != nil {
		return fmt.Errorf("close fd socket: %w", err)
	}

	return nil
}

// RemoteFDs can be used start and connect to the remote fd socket.
func (c *ConmonClient) RemoteFDs(ctx context.Context) (*RemoteFDs, error) {
	ctx, span := c.startSpan(ctx, "RemoteFDs")
	if span != nil {
		defer span.End()
	}

	rpcClient, err := c.client(ctx)
	if err != nil {
		return nil, fmt.Errorf("create RPC client: %w", err)
	}

	future, free := rpcClient.StartFdSocket(ctx, func(p proto.Conmon_startFdSocket_Params) error {
		req, err := p.NewRequest()
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		if err := c.setMetadata(ctx, req); err != nil {
			return fmt.Errorf("set metadata: %w", err)
		}

		return nil
	})
	defer free()

	result, err := future.Struct()
	if err != nil {
		return nil, fmt.Errorf("get result: %w", err)
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
