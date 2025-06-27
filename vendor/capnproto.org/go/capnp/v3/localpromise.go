package capnp

// ClientHook for a promise that will be resolved to some other capability
// at some point. Buffers calls in a queue until the promsie is fulfilled,
// then forwards them.
type localPromise struct {
	aq *AnswerQueue
}

// NewLocalPromise returns a client that will eventually resolve to a capability,
// supplied via the resolver.
func NewLocalPromise[C ~ClientKind]() (C, Resolver[C]) {
	aq := NewAnswerQueue(Method{})
	f := NewPromise(Method{}, aq, aq)
	p := f.Answer().Client().AddRef()

	c := C(p)
	r := localResolver[C]{
		p: f,
		c: c,
	}
	return c, r
}

type localResolver[C ~ClientKind] struct {
	p *Promise
	c C
}

func (lf localResolver[C]) Fulfill(c C) {
	msg, seg := NewSingleSegmentMessage(nil)
	Client(lf.c).AttachReleaser(lf.p.ReleaseClients)
	Client(lf.c).AttachReleaser(msg.Release)
	capID := msg.CapTable().Add(Client(c))
	iface := NewInterface(seg, capID)
	lf.p.Fulfill(iface.ToPtr())
}

func (lf localResolver[C]) Reject(err error) {
	Client(lf.c).AttachReleaser(lf.p.ReleaseClients)
	lf.p.Reject(err)
}
