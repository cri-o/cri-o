package client

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"syscall"

	"github.com/containers/conmon-rs/internal/proto"
)

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

func (r RemoteFD) String() string {
	return fmt.Sprintf("RemoteFD(%d)", r)
}

// RemoteFDs can be used to send file descriptors to the server.
type RemoteFDs struct {
	conn  *net.UnixConn
	reqID uint32
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
	if len(fds) == 0 {
		return nil, nil
	}

	if len(fds) > maxFDs {
		return nil, errTooManyFileDescriptors
	}

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
		return nil, fmt.Errorf("receviree reaponse: %w", err)
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
	for i := 0; i < numFDs; i++ {
		slots = append(slots, RemoteFD(binary.LittleEndian.Uint64(buf[i*uint64Bytes:])))
	}

	return slots, nil
}

// Close the connection and unused remote file descriptors.
func (r *RemoteFDs) Close() error {
	if err := r.conn.Close(); err != nil {
		return fmt.Errorf("close fd socket: %w", err)
	}

	return nil
}

// RemoteFDs can be used start and connect to the remote fd socket.
func (c *ConmonClient) RemoteFDs(ctx context.Context) (*RemoteFDs, error) {
	ctx, span := c.startSpan(ctx, "AttachContainer")
	if span != nil {
		defer span.End()
	}

	conn, err := c.newRPCConn()
	if err != nil {
		return nil, fmt.Errorf("create RPC connection: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			c.logger.Errorf("Unable to close connection: %v", err)
		}
	}()

	client := proto.Conmon(conn.Bootstrap(ctx))

	future, free := client.StartFdSocket(ctx, func(p proto.Conmon_startFdSocket_Params) error {
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
		return nil, fmt.Errorf("create result: %w", err)
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
