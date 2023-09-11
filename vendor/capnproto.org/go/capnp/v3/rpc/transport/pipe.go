package transport

import (
	"io"
	"sync"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/internal/syncutil"
)

// NewPipe returns a pair of codecs which communicate over
// channels, copying messages at the channel boundary.
// bufSz is the size of the channel buffers.
func NewPipe(bufSz int) (c1, c2 Codec) {
	ch1 := make(chan *capnp.Message, bufSz)
	ch2 := make(chan *capnp.Message, bufSz)

	c1 = &pipe{
		send:   ch1,
		recv:   ch2,
		closed: make(chan struct{}),
	}

	c2 = &pipe{
		send:   ch2,
		recv:   ch1,
		closed: make(chan struct{}),
	}

	return
}

type pipe struct {
	// Must hold while sending or closing `send`:
	sendMu sync.Mutex

	send   chan<- *capnp.Message
	recv   <-chan *capnp.Message
	closed chan struct{}
}

func (p *pipe) Encode(m *capnp.Message) (err error) {
	b, err := m.Marshal()
	if err != nil {
		return err
	}

	if m, err = capnp.Unmarshal(b); err != nil {
		return err
	}

	p.sendMu.Lock()
	defer p.sendMu.Unlock()
	select {
	case p.send <- m:
		return nil
	case <-p.closed:
		return io.ErrClosedPipe
	}
}

func (p *pipe) Decode() (*capnp.Message, error) {
	select {
	case <-p.closed:
		return nil, io.ErrClosedPipe
	case m, ok := <-p.recv:
		if !ok {
			return nil, io.ErrClosedPipe
		}
		return m, nil
	}

}

func (*pipe) ReleaseMessage(*capnp.Message) {}

func (p *pipe) Close() error {
	close(p.closed)
	syncutil.With(&p.sendMu, func() {
		close(p.send)
		p.send = nil
	})
	return nil
}
