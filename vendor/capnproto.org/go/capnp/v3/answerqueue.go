package capnp

import (
	"context"
	"errors"
	"sync"

	"capnproto.org/go/capnp/v3/exc"
)

// AnswerQueue is a queue of method calls to make after an earlier
// method call finishes.  The queue is unbounded; it is the caller's
// responsibility to manage/impose backpressure.
//
// An AnswerQueue can be in one of three states:
//
//	1) Queueing.  Incoming method calls will be added to the queue.
//	2) Draining, entered by calling Fulfill or Reject.  Queued method
//	   calls will be delivered in sequence, and new incoming method calls
//	   will block until the AnswerQueue enters the Drained state.
//	3) Drained, entered once all queued methods have been delivered.
//	   Incoming methods are passthrough.
type AnswerQueue struct {
	method   Method
	draining chan struct{} // closed while exiting queueing state

	mu    sync.Mutex
	q     []qent // non-nil while queueing
	bases []base // set when drain starts. len(bases) >= 1
}

// qent is a single entry in an AnswerQueue.
type qent struct {
	ctx   context.Context
	basis int // index in bases
	path  []PipelineOp
	Recv
}

// base is a message target derived from applying a qent.
type base struct {
	ready chan struct{} // closed after recv is assigned
	recv  func(context.Context, []PipelineOp, Recv) PipelineCaller
}

// NewAnswerQueue creates a new answer queue.
func NewAnswerQueue(m Method) *AnswerQueue {
	return &AnswerQueue{
		method: m,
		// N.B. since q == nil denotes the draining state,
		// we do have to allocate something here, even though
		// the queue is an empty slice.
		q:        make([]qent, 0),
		draining: make(chan struct{}),
	}
}

// Fulfill empties the queue, delivering the method calls on the given
// pointer.  After fulfill returns, pipeline calls will be immediately
// delivered instead of being queued.
func (aq *AnswerQueue) Fulfill(ptr Ptr) {
	// Enter draining state.
	aq.mu.Lock()
	q := aq.q
	aq.q = nil
	aq.bases = make([]base, len(q)+1)
	ready := make(chan struct{}) // TODO(soon): use more fine-grained signals
	defer close(ready)
	for i := range aq.bases {
		aq.bases[i].ready = ready
	}
	aq.bases[0].recv = ImmediateAnswer(aq.method, ptr).PipelineRecv
	close(aq.draining)
	aq.mu.Unlock()

	// Drain queue.
	for i := range q {
		ent := &q[i]
		recv := aq.bases[ent.basis].recv
		recv(ent.ctx, ent.path, ent.Recv)
	}
}

// Reject empties the queue, returning errors on all the method calls.
func (aq *AnswerQueue) Reject(e error) {
	if e == nil {
		panic("AnswerQueue.reject(nil)")
	}

	// Enter draining state.
	aq.mu.Lock()
	q := aq.q
	aq.q = nil
	aq.bases = make([]base, len(q)+1)
	ready := make(chan struct{})
	close(ready)
	for i := range aq.bases {
		b := &aq.bases[i]
		b.ready = ready
		b.recv = func(_ context.Context, _ []PipelineOp, r Recv) PipelineCaller {
			r.Reject(e) // TODO(soon): attach pipelined method info
			return nil
		}
	}
	close(aq.draining)
	aq.mu.Unlock()

	// Drain queue by rejecting.
	for i := range q {
		q[i].Reject(e) // TODO(soon): attach pipelined method info
	}
}

func (aq *AnswerQueue) PipelineRecv(ctx context.Context, transform []PipelineOp, r Recv) PipelineCaller {
	return queueCaller{aq, 0}.PipelineRecv(ctx, transform, r)
}

func (aq *AnswerQueue) PipelineSend(ctx context.Context, transform []PipelineOp, r Send) (*Answer, ReleaseFunc) {
	return queueCaller{aq, 0}.PipelineSend(ctx, transform, r)
}

// queueCaller is a client that enqueues calls to an AnswerQueue.
type queueCaller struct {
	aq    *AnswerQueue
	basis int
}

func (qc queueCaller) PipelineRecv(ctx context.Context, transform []PipelineOp, r Recv) PipelineCaller {
	qc.aq.mu.Lock()
	if len(qc.aq.bases) > 0 {
		// Draining/drained.
		qc.aq.mu.Unlock()
		b := &qc.aq.bases[qc.basis]
		select {
		case <-b.ready:
		case <-ctx.Done():
			r.Reject(ctx.Err())
			return nil
		}
		return b.recv(ctx, transform, r)
	}
	// Enqueue.
	qc.aq.q = append(qc.aq.q, qent{
		ctx:   ctx,
		basis: qc.basis,
		path:  transform,
		Recv:  r,
	})
	basis := len(qc.aq.q) - 1
	qc.aq.mu.Unlock()
	return queueCaller{aq: qc.aq, basis: basis}
}

func (qc queueCaller) PipelineSend(ctx context.Context, transform []PipelineOp, s Send) (*Answer, ReleaseFunc) {
	ret := new(StructReturner)
	r := Recv{
		Method:   s.Method,
		Returner: ret,
	}
	if s.PlaceArgs != nil {
		var err error
		_, seg := NewMultiSegmentMessage(nil)
		r.Args, err = NewRootStruct(seg, s.ArgsSize)
		if err != nil {
			return ErrorAnswer(s.Method, err), func() {}
		}
		if err = s.PlaceArgs(r.Args); err != nil {
			return ErrorAnswer(s.Method, err), func() {}
		}
		r.ReleaseArgs = func() {
			r.Args.Message().Reset(nil)
		}
	} else {
		r.ReleaseArgs = func() {}
	}
	pcall := qc.PipelineRecv(ctx, transform, r)
	return ret.Answer(s.Method, pcall)
}

// A StructReturner implements Returner by allocating an in-memory
// message.  It is safe to use from multiple goroutines.  The zero value
// is a Returner in its initial state.
type StructReturner struct {
	mu       sync.Mutex // guards all fields
	p        *Promise   // assigned at most once
	alloced  bool
	released bool
	msg      *Message // assigned at most once

	returned bool   // indicates whether the below fields are filled in
	result   Struct // assigned at most once
	err      error  // assigned at most once

}

func (sr *StructReturner) AllocResults(sz ObjectSize) (Struct, error) {
	defer sr.mu.Unlock()
	sr.mu.Lock()
	if sr.alloced {
		return Struct{}, errors.New("StructReturner: multiple calls to AllocResults")
	}
	sr.alloced = true
	_, seg := NewMultiSegmentMessage(nil)
	s, err := NewRootStruct(seg, sz)
	if err != nil {
		return Struct{}, exc.WrapError("alloc results", err)
	}
	sr.result = s
	sr.msg = s.Message()
	return s, nil
}
func (sr *StructReturner) PrepareReturn(e error) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.err = e
}

func (sr *StructReturner) Return() {
	sr.mu.Lock()
	if sr.returned {
		sr.mu.Unlock()
		panic("StructReturner.Return called twice")
	}
	sr.returned = true
	e := sr.err
	if e == nil {
		sr.mu.Unlock()
		if sr.p != nil {
			sr.p.Fulfill(sr.result.ToPtr())
		}
	} else {
		sr.result = Struct{}
		sr.mu.Unlock()
		if sr.p != nil {
			sr.p.Reject(e)
		}
	}
}

func (sr *StructReturner) ReleaseResults() {
	sr.mu.Lock()
	alloced := sr.alloced
	returned := sr.returned
	released := sr.released
	err := sr.err
	msg := sr.msg
	sr.msg = nil
	sr.released = true
	sr.mu.Unlock()

	if !returned {
		panic("ReleaseResults() called before Return()")
	}
	if released {
		panic("ReleaseResults() called twice")
	}
	if !alloced {
		return
	}
	if err != nil && msg != nil {
		msg.Reset(nil)
	}
}

// Answer returns an Answer that will be resolved when Return is called.
// answer must only be called once per StructReturner.
func (sr *StructReturner) Answer(m Method, pcall PipelineCaller) (*Answer, ReleaseFunc) {
	defer sr.mu.Unlock()
	sr.mu.Lock()
	if sr.p != nil {
		panic("StructReturner.Answer called multiple times")
	}
	if sr.returned {
		if sr.err != nil {
			return ErrorAnswer(m, sr.err), func() {}
		}
		return ImmediateAnswer(m, sr.result.ToPtr()), func() {
			sr.mu.Lock()
			msg := sr.result.Message()
			sr.result = Struct{}
			sr.mu.Unlock()
			if msg != nil {
				msg.Reset(nil)
			}
		}
	}
	sr.p = NewPromise(m, pcall)
	ans := sr.p.Answer()
	return ans, func() {
		<-ans.Done()
		sr.mu.Lock()
		msg := sr.result.Message()
		sr.result = Struct{}
		sr.mu.Unlock()
		sr.p.ReleaseClients()
		if msg != nil {
			msg.Reset(nil)
		}
	}
}
