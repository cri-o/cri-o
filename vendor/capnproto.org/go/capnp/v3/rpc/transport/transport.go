// Package transport defines an interface for sending and receiving rpc messages.
//
// In addition to the implementations defined here, one of the developers maintains
// a websocket-backed implementation as a separate module:
//
// https://pkg.go.dev/zenhack.net/go/websocket-capnp
package transport

import (
	"errors"
	"io"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/exc"
	rpccp "capnproto.org/go/capnp/v3/std/capnp/rpc"
)

// A Transport sends and receives Cap'n Proto RPC messages to and from
// another vat.
//
// It is safe to call NewMessage and its returned functions concurrently
// with RecvMessage.
type Transport interface {
	// NewMessage allocates a new message to be sent over the transport.
	// The caller must call OutgoingMessage.Release() when it no longer
	// needs to reference the message. Calling Release() more than once
	// has no effect.  Before releasing the message, Send() MAY be called
	// at most once to send the mssage.
	//
	// When Release() is called, the underlying *capnp.Message SHOULD be
	// released.  This will also release any clients in the CapTable and
	// release its Arena.
	//
	// The Arena in the returned message should be fast at allocating new
	// segments.  The returned ReleaseFunc MUST be safe to call concurrently
	// with subsequent calls to NewMessage.
	NewMessage() (OutgoingMessage, error)

	// RecvMessage receives the next message sent from the remote vat.
	// The returned message is only valid until the release method is
	// called.  The IncomingMessage.Release() method may be called
	// concurrently with RecvMessage or with any other release function
	// returned by RecvMessage.
	//
	// When Release() is called, the underlying *capnp.Message SHOULD be
	// released.  This will also release any clients in the CapTable and
	// release its Arena.
	//
	// The Arena in the returned message should not fetch segments lazily;
	// the Arena should be fast to access other segments.
	RecvMessage() (IncomingMessage, error)

	// Close releases any resources associated with the transport. If there
	// are any outstanding calls to NewMessage, a returned send function,
	// or RecvMessage, they will be interrupted and return errors.
	Close() error
}

// OutgoingMessage is a message that can be sent at a later time.
// Release() MUST be called when the OutgoingMessage is no longer in
// use. Before releasing an ougoing message, Send() MAY be called at
// most once to send the message over the transport that produced it.
//
// Implementations SHOULD release the underlying *capnp.Message when
// the Release() method is called.
//
// Release() MUST be idempotent, and calls to Send() after a call to
// Release MUST panic.
type OutgoingMessage interface {
	Send() error
	Message() rpccp.Message
	Release()
}

// IncomingMessage is a message that has arrived over a transport.
// Release() MUST be called when the IncomingMessage is no longer
// in use.
//
// Implementations SHOULD release the underlying *capnp.Message when
// the Release() method is called.  Release() MUST be idempotent.
type IncomingMessage interface {
	Message() rpccp.Message
	Release()
}

// A Codec is responsible for encoding and decoding messages from
// a single logical stream.
type Codec interface {
	Encode(*capnp.Message) error
	Decode() (*capnp.Message, error)
	Close() error
}

// A transport serializes and deserializes Cap'n Proto using a Codec.
// It adds no buffering beyond what is provided by the underlying
// byte transfer mechanism.
type transport struct {
	c      Codec
	closed bool
}

// New creates a new transport that uses the supplied codec
// to read and write messages across the wire.
func New(c Codec) Transport { return &transport{c: c} }

// NewStream creates a new transport that reads and writes to rwc.
// Closing the transport will close rwc.
//
// rwc's Close method must interrupt any outstanding IO, and it must be safe
// to call rwc.Read and rwc.Write concurrently.
func NewStream(rwc io.ReadWriteCloser) Transport {
	return New(newStreamCodec(rwc, basicEncoding{}))
}

// NewPackedStream creates a new transport that uses a packed
// encoding.
//
// See:  NewStream.
func NewPackedStream(rwc io.ReadWriteCloser) Transport {
	return New(newStreamCodec(rwc, packedEncoding{}))
}

// NewMessage allocates a new message to be sent.
//
// It is safe to call NewMessage concurrently with RecvMessage.
func (s *transport) NewMessage() (OutgoingMessage, error) {
	arena := capnp.MultiSegment(nil)
	_, seg, err := capnp.NewMessage(arena)
	if err != nil {
		err = transporterr.Annotate(exc.WrapError("new message", err), "stream transport")
		return nil, err
	}
	m, err := rpccp.NewRootMessage(seg)
	if err != nil {
		err = transporterr.Annotate(exc.WrapError("new message", err), "stream transport")
		return nil, err
	}

	send := func(m *capnp.Message) (err error) {
		if err = s.c.Encode(m); err != nil {
			err = transporterr.Annotate(exc.WrapError("send", err), "stream transport")
		}

		return
	}

	return &outgoingMsg{
		message: m,
		send:    send,
	}, nil
}

// RecvMessage reads the next message from the underlying reader.
//
// It is safe to call RecvMessage concurrently with NewMessage.
func (s *transport) RecvMessage() (IncomingMessage, error) {
	msg, err := s.c.Decode()
	if err != nil {
		err = transporterr.Annotate(exc.WrapError("receive", err), "stream transport")
		return nil, err
	}
	rmsg, err := rpccp.ReadRootMessage(msg)
	if err != nil {
		err = transporterr.Annotate(exc.WrapError("receive", err), "stream transport")
		return nil, err
	}

	return incomingMsg(rmsg), nil
}

// Close closes the underlying ReadWriteCloser.  It is not safe to call
// Close concurrently with any other operations on the transport.
func (s *transport) Close() error {
	if s.closed {
		return transporterr.Disconnected(errors.New("already closed")).Annotate("", "stream transport")
	}
	s.closed = true
	err := s.c.Close()
	if err != nil {
		return transporterr.Annotate(exc.WrapError("close", err), "stream transport")
	}
	return nil
}

type streamCodec struct {
	*capnp.Decoder
	*capnp.Encoder
	io.Closer
}

func newStreamCodec(rwc io.ReadWriteCloser, f streamEncoding) *streamCodec {
	return &streamCodec{
		Decoder: f.NewDecoder(rwc),
		Encoder: f.NewEncoder(rwc),
		Closer:  rwc,
	}
}

type streamEncoding interface {
	NewEncoder(io.Writer) *capnp.Encoder
	NewDecoder(io.Reader) *capnp.Decoder
}

type basicEncoding struct{}

func (basicEncoding) NewEncoder(w io.Writer) *capnp.Encoder { return capnp.NewEncoder(w) }
func (basicEncoding) NewDecoder(r io.Reader) *capnp.Decoder { return capnp.NewDecoder(r) }

type packedEncoding struct{}

func (packedEncoding) NewEncoder(w io.Writer) *capnp.Encoder { return capnp.NewPackedEncoder(w) }
func (packedEncoding) NewDecoder(r io.Reader) *capnp.Decoder { return capnp.NewPackedDecoder(r) }

type outgoingMsg struct {
	message  rpccp.Message
	send     func(*capnp.Message) error
	released bool
}

func (o *outgoingMsg) Release() {
	if m := o.message.Message(); !o.released && m != nil {
		m.Release()
	}
}

func (o *outgoingMsg) Message() rpccp.Message {
	return o.message
}

func (o *outgoingMsg) Send() error {
	if !o.released {
		return o.send(o.message.Message())
	}

	panic("call to Send() after call to Release()")
}

type incomingMsg rpccp.Message

func (i incomingMsg) Message() rpccp.Message {
	return rpccp.Message(i)
}

func (i incomingMsg) Release() {
	if m := i.Message().Message(); m != nil {
		m.Release()
	}
}
