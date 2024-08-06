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
	return C(p), localResolver[C]{
		p: f,
	}
}

type localResolver[C ~ClientKind] struct {
	p *Promise
}

func (lf localResolver[C]) Fulfill(c C) {
	msg, seg := NewSingleSegmentMessage(nil)
	capID := msg.CapTable().Add(Client(c))
	iface := NewInterface(seg, capID)
	lf.p.Fulfill(iface.ToPtr())
	lf.p.ReleaseClients()
	msg.Release()
}

func (lf localResolver[C]) Reject(err error) {
	lf.p.Reject(err)
	lf.p.ReleaseClients()
}
