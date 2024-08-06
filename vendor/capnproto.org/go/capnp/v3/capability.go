package capnp

import (
	"context"
	"errors"
	"runtime"
	"strconv"
	"sync"

	"capnproto.org/go/capnp/v3/exc"
	"capnproto.org/go/capnp/v3/exp/bufferpool"
	"capnproto.org/go/capnp/v3/flowcontrol"
	"capnproto.org/go/capnp/v3/internal/str"
	"capnproto.org/go/capnp/v3/util/deferred"
	"capnproto.org/go/capnp/v3/util/maybe"
	"capnproto.org/go/capnp/v3/util/rc"
	"capnproto.org/go/capnp/v3/util/sync/mutex"
)

func init() {
	close(closedSignal)
}

// An Interface is a reference to a client in a message's capability table.
type Interface struct {
	seg *Segment
	cap CapabilityID
}

// i.EncodeAsPtr is equivalent to i.ToPtr(); for implementing TypeParam.
// The segment argument is ignored.
func (i Interface) EncodeAsPtr(*Segment) Ptr { return i.ToPtr() }

// DecodeFromPtr(p) is equivalent to p.Interface(); for implementing TypeParam.
func (Interface) DecodeFromPtr(p Ptr) Interface { return p.Interface() }

var _ TypeParam[Interface] = Interface{}

// NewInterface creates a new interface pointer.
//
// No allocation is performed in the given segment: it is used purely
// to associate the interface pointer with a message.
func NewInterface(s *Segment, cap CapabilityID) Interface {
	return Interface{s, cap}
}

// ToPtr converts the interface to a generic pointer.
func (i Interface) ToPtr() Ptr {
	return Ptr{
		seg:      i.seg,
		lenOrCap: uint32(i.cap),
		flags:    interfacePtrFlag,
	}
}

// Message returns the message whose capability table the interface
// references or nil if the pointer is invalid.
func (i Interface) Message() *Message {
	if i.seg == nil {
		return nil
	}
	return i.seg.msg
}

// IsValid returns whether the interface is valid.
func (i Interface) IsValid() bool {
	return i.seg != nil
}

// Capability returns the capability ID of the interface.
func (i Interface) Capability() CapabilityID {
	return i.cap
}

// value returns a raw interface pointer with the capability ID.
func (i Interface) value(paddr address) rawPointer {
	if i.seg == nil {
		return 0
	}
	return rawInterfacePointer(i.cap)
}

// Client returns the client stored in the message's capability table
// or nil if the pointer is invalid.
func (i Interface) Client() (c Client) {
	if msg := i.Message(); msg != nil {
		c = msg.CapTable().Get(i)
	}

	return
}

// A CapabilityID is an index into a message's capability table.
type CapabilityID uint32

// String returns the ID in the format "capability X".
func (id CapabilityID) String() string {
	return "capability " + str.Utod(id)
}

// GoString returns the ID as a Go expression.
func (id CapabilityID) GoString() string {
	return "capnp.CapabilityID(" + str.Utod(id) + ")"
}

// A Client is a reference to a Cap'n Proto capability.
// The zero value is a null capability reference.
// It is safe to use from multiple goroutines.
type Client ClientKind

// The underlying type of Client. We expose this so that
// we can use ~ClientKind as a constraint in generics to
// capture any capability type.
type ClientKind = struct {
	*client
}

type client struct {
	state mutex.Mutex[clientState]
}

type clientState struct {
	limiter  flowcontrol.FlowLimiter
	cursor   *rc.Ref[clientCursor] // never nil
	released bool

	stream struct {
		err error          // Last error from streaming calls.
		wg  sync.WaitGroup // Outstanding calls.
	}
}

// clientCursor is an indirection pointing to a link in the resolution
// chain of clientHooks. Places that need to do path shortening should
// store one of these, rather than storing clientHook directly.
type clientCursor struct {
	hook mutex.Mutex[*rc.Ref[clientHook]] // nil if resolved to nil or released
}

func newClientCursor(hook clientHook) *rc.Ref[clientCursor] {
	hookRef := rc.NewRefInPlace(func(h *clientHook) func() {
		*h = hook
		return h.Release
	})
	return rc.NewRefInPlace(func(c *clientCursor) func() {
		*c = clientCursor{hook: mutex.New(hookRef)}
		return c.Release
	})
}

// compress advances the hook referred to by this cursor as far
// as possible without blocking on a resolution.
func (c *clientCursor) compress() {
	c.hook.With(func(hook **rc.Ref[clientHook]) {
		for {
			h := *hook
			if h == nil {
				return
			}
			res, ok := h.Value().resolution.Get()
			if !ok {
				return
			}
			l := res.Lock()
			if !l.Value().isResolved() {
				l.Unlock()
				return
			}
			r := l.Value().resolvedHook
			if r != nil {
				r = r.AddRef()
			}
			l.Unlock()
			h.Release()
			*hook = r
		}
	})
}

func (c *clientCursor) Release() {
	c.hook.With(func(hook **rc.Ref[clientHook]) {
		(*hook).Release()
	})
}

// clientHook is a reference-counted wrapper for a ClientHook.
// It is assumed that a clientHook's address uniquely identifies a hook,
// since they are only created in NewClient and NewPromisedClient.
type clientHook struct {
	// ClientHook will never be nil and will not change for the lifetime of
	// a clientHook.
	ClientHook

	// Place for callers to attach arbitrary metadata to the client.
	metadata Metadata

	// State of the promise's resolution. If this is absent, then
	// this clientHook is not a promise.
	resolution maybe.Maybe[*mutex.Mutex[resolveState]]
}

func (h *clientHook) Release() {
	h.Shutdown()
	r, ok := h.resolution.Get()
	if ok {
		r.With(func(s *resolveState) {
			if s.isResolved() {
				s.resolvedHook.Release()
			}
		})
	}
}

type resolveState struct {
	// resolved is closed after resolvedHook is set
	resolved chan struct{}

	// Valid only if resolved is closed.
	resolvedHook *rc.Ref[clientHook]
}

// NewClient creates the first reference to a capability.
// If hook is nil, then NewClient returns nil.
//
// Typically the RPC system will create a client for the application.
// Most applications will not need to use this directly.
func NewClient(hook ClientHook) Client {
	if hook == nil {
		return Client{}
	}
	h := clientHook{
		ClientHook: hook,
		metadata:   *NewMetadata(),
	}
	cs := mutex.New(clientState{cursor: newClientCursor(h)})
	c := Client{client: &client{state: cs}}
	setupLeakReporting(c)
	return c
}

// NewPromisedClient creates the first reference to a capability that
// can resolve to a different capability.  The hook will be shut down
// when the promise is resolved or the client has no more references,
// whichever comes first.
//
// Typically the RPC system will create a client for the application.
// Most applications will not need to use this directly.
func NewPromisedClient(hook ClientHook) (Client, Resolver[Client]) {
	return newPromisedClient(hook)
}

// newPromisedClient is the same as NewPromisedClient, but the return
// value exposes the concrete type of the resolver.
func newPromisedClient(hook ClientHook) (Client, *clientPromise) {
	if hook == nil {
		panic("NewPromisedClient(nil)")
	}
	rs := mutex.New(resolveState{
		resolved: make(chan struct{}),
	})
	cursor := newClientCursor(clientHook{
		ClientHook: hook,
		metadata:   *NewMetadata(),
		resolution: maybe.New(&rs),
	})
	cs := mutex.New(clientState{cursor: cursor})
	c := Client{client: &client{state: cs}}
	setupLeakReporting(c)
	return c, &clientPromise{cursor: cursor.Weak()}
}

// startCall holds onto a hook to prevent it from shutting down until
// finish is called.  It resolves the client's hook as much as possible
// first.  The caller must not be holding onto c.mu.
func (c Client) startCall() (hook *rc.Ref[clientHook], resolved, released bool) {
	if c.client == nil {
		return nil, true, false
	}
	return mutex.With3(&c.state, func(c *clientState) (hook *rc.Ref[clientHook], resolved, released bool) {
		if c.released || !c.cursor.IsValid() {
			return nil, true, c.released
		}
		c.cursor.Value().compress()
		hook, ok := mutex.With2(&c.cursor.Value().hook, func(h **rc.Ref[clientHook]) (*rc.Ref[clientHook], bool) {
			ret := *h
			if ret.IsValid() {
				return ret.AddRef(), true
			}
			return nil, false
		})
		if !ok {
			return nil, true, false
		}
		r, ok := hook.Value().resolution.Get()
		if !ok {
			return hook, true, false
		}
		resolved = mutex.With1(r, func(s *resolveState) bool {
			return s.isResolved()
		})
		return hook, resolved, false
	})
}

// Get the current flowcontrol.FlowLimiter used to manage flow control
// for this client.
func (c Client) GetFlowLimiter() flowcontrol.FlowLimiter {
	return mutex.With1(&c.state, func(c *clientState) flowcontrol.FlowLimiter {
		ret := c.limiter
		if ret == nil {
			ret = flowcontrol.NopLimiter
		}
		return ret
	})
}

// Update the flowcontrol.FlowLimiter used to manage flow control for
// this client. This affects all future calls, but not calls already
// waiting to send. Passing nil sets the value to flowcontrol.NopLimiter,
// which is also the default.
//
// When .Release() is called on the client, it will call .Release() on
// the FlowLimiter in turn.
func (c Client) SetFlowLimiter(lim flowcontrol.FlowLimiter) {
	c.state.With(func(c *clientState) {
		c.limiter = lim
	})
}

// SendCall allocates space for parameters, calls args.Place to fill out
// the parameters, then starts executing a method, returning an answer
// that will hold the result.  The caller must call the returned release
// function when it no longer needs the answer's data.
//
// This method respects the flow control policy configured with SetFlowLimiter;
// it may block if the sender is sending too fast.
func (c Client) SendCall(ctx context.Context, s Send) (*Answer, ReleaseFunc) {
	h, _, released := c.startCall()
	defer h.Release()
	if released {
		return ErrorAnswer(s.Method, errors.New("call on released client")), func() {}
	}
	if h == nil {
		return ErrorAnswer(s.Method, errors.New("call on null client")), func() {}
	}

	err := mutex.With1(&c.state, func(c *clientState) error {
		return c.stream.err
	})

	if err != nil {
		return ErrorAnswer(s.Method, exc.WrapError("stream error", err)), func() {}
	}

	limiter := c.GetFlowLimiter()

	// We need to call PlaceArgs before we will know the size of message for
	// flow control purposes, so wrap it in a function that measures after the
	// arguments have been placed:
	placeArgs := s.PlaceArgs
	var size uint64
	s.PlaceArgs = func(args Struct) error {
		var err error
		if placeArgs != nil {
			err = placeArgs(args)
			if err != nil {
				return err
			}
		}

		size, err = args.Segment().Message().TotalSize()
		return err
	}

	ans, rel := h.Value().Send(ctx, s)
	// FIXME: an earlier version of this code called StartMessage() from
	// within PlaceArgs -- but that can result in a deadlock, since it means
	// the client hook is holding a lock while we're waiting on the limiter.
	//
	// As a temporary workaround, we instead do StartMessage *after* the send.
	// This still has a bug, but a much less serious one: we may slightly
	// over-use our limit, but only by the size of a single message. This is
	// mostly a problem in that it contradicts the documentation and is
	// conceptually odd.
	//
	// Longer term, we should fix a more serious design problem: Send() is
	// holding a lock while calling into user code (PlaceArgs), so this
	// deadlock could also arise if the user code blocks. Once that is solved,
	// we can back out this hack.
	gotResponse, err := limiter.StartMessage(ctx, size)
	if err != nil {
		// HACK: An error should only happen if the context was cancelled,
		// in which case the caller will notice it soon probably. The call
		// still went off ok, so we can just return the result we already
		// got, and trying to report the error is awkward because we can't
		// return one... so we don't. Set gotResponse to something that won't
		// break things, and call it a day. See comments above about a
		// longer term solution to this mess.
		gotResponse = func() {}
	}
	p := ans.f.promise
	l := p.state.Lock()
	if l.Value().isResolved() {
		// Wow, that was fast.
		l.Unlock()
		gotResponse()
	} else {
		l.Value().signals = append(l.Value().signals, gotResponse)
		l.Unlock()
	}

	return ans, rel
}

// SendStreamCall is like SendCall except that:
//
//  1. It does not return an answer for the eventual result.
//  2. If the call returns an error, all future calls on this
//     client will return the same error (without starting
//     the method or calling PlaceArgs).
func (c Client) SendStreamCall(ctx context.Context, s Send) error {
	streamError := mutex.With1(&c.state, func(c *clientState) error {
		err := c.stream.err
		if err == nil {
			c.stream.wg.Add(1)
		}
		return err
	})
	if streamError != nil {
		return streamError
	}
	ans, release := c.SendCall(ctx, s)
	go func() {
		defer c.state.With(func(c *clientState) {
			c.stream.wg.Done()
		})
		_, err := ans.Future().Ptr()
		release()
		if err != nil {
			c.state.With(func(c *clientState) {
				c.stream.err = err
			})
		}
	}()
	return nil
}

// WaitStreaming waits for all outstanding streaming calls (i.e. calls
// started with SendStreamCall) to complete, and then returns an error
// if any streaming call has failed.
func (c Client) WaitStreaming() error {
	wg := mutex.With1(&c.state, func(c *clientState) *sync.WaitGroup {
		return &c.stream.wg
	})
	wg.Wait()
	return mutex.With1(&c.state, func(c *clientState) error {
		return c.stream.err
	})
}

// RecvCall starts executing a method with the referenced arguments
// and returns an answer that will hold the result.  The hook will call
// a.Release when it no longer needs to reference the parameters.  The
// caller must call the returned release function when it no longer
// needs the answer's data.
//
// Note that unlike SendCall, this method does *not* respect the flow
// control policy configured with SetFlowLimiter.
func (c Client) RecvCall(ctx context.Context, r Recv) PipelineCaller {
	h, _, released := c.startCall()
	defer h.Release()
	if released {
		r.Reject(errors.New("call on released client"))
		return nil
	}
	if h == nil {
		r.Reject(errors.New("call on null client"))
		return nil
	}
	return h.Value().Recv(ctx, r)
}

// IsValid reports whether c is a valid reference to a capability.
// A reference is invalid if it is nil, has resolved to null, or has
// been released.
func (c Client) IsValid() bool {
	h, _, released := c.startCall()
	defer h.Release()
	return !released && h != nil
}

// IsSame reports whether c and c2 refer to a capability created by the
// same call to NewClient.  This can return false negatives if c or c2
// are not fully resolved: use Resolve if this is an issue.  If either
// c or c2 are released, then IsSame panics.
func (c Client) IsSame(c2 Client) bool {
	h1, _, released := c.startCall()
	defer h1.Release()
	if released {
		panic("IsSame on released client")
	}
	h2, _, released := c2.startCall()
	defer h2.Release()
	if released {
		panic("IsSame on released client")
	}
	valid1 := h1.IsValid()
	valid2 := h2.IsValid()
	if !valid1 && !valid2 {
		return true
	}
	if !valid1 || !valid2 {
		return false
	}
	return h1.Value() == h2.Value()
}

// Resolve blocks until the capability is fully resolved or the Context is Done.
// Resolve only returns an error if the context is canceled; it returns nil even
// if the capability resolves to an error.
func (c Client) Resolve(ctx context.Context) error {
	h, resolved, released := c.startCall()
	defer h.Release()
	if released {
		return errors.New("cannot resolve released client")
	}
	if resolved {
		return nil
	}
	h, err := resolveClientHook(ctx, h)
	h.Release()
	return err
}

// AddRef creates a new Client that refers to the same capability as c.
// If c is nil or has resolved to null, then AddRef returns nil.
func (c Client) AddRef() Client {
	if c.client == nil {
		return Client{}
	}
	h, _, released := c.startCall()
	defer h.Release()
	if released {
		panic("AddRef on released client")
	}
	return mutex.With1(&c.state, func(c *clientState) Client {
		d := Client{client: &client{state: mutex.New(clientState{cursor: c.cursor.AddRef()})}}
		setupLeakReporting(d)
		return d
	})
}

// WeakRef creates a new WeakClient that refers to the same capability
// as c.  If c is nil or has resolved to null, then WeakRef returns nil.
func (c Client) WeakRef() WeakClient {
	cursor := mutex.With1(&c.state, func(s *clientState) *rc.WeakRef[clientCursor] {
		if s.released {
			panic("WeakRef on released client")
		}
		return s.cursor.Weak()
	})
	return WeakClient{r: cursor}
}

// Snapshot reads the current state of the client.  It returns the zero
// ClientSnapshot if c is nil, has resolved to null, or has been released.
func (c Client) Snapshot() ClientSnapshot {
	h, _, _ := c.startCall()
	s := ClientSnapshot{hook: h}
	setupLeakReporting(s)
	return s
}

// A Brand is an opaque value used to identify a capability.
type Brand struct {
	Value any
}

// ClientSnapshot is a snapshot of a client's identity. If the Client
// is a promise, then the corresponding ClientSnapshot will *not*
// redirect to point at the resolution.
type ClientSnapshot struct {
	hook *rc.Ref[clientHook]
}

func (cs ClientSnapshot) String() string {
	if !cs.IsValid() {
		return "ClientSnapshot{}"
	}
	return "ClientSnapshot{" + cs.hook.Value().String() + "}"
}

func (cs ClientSnapshot) IsValid() bool {
	return cs.hook.IsValid()
}

// IsPromise returns true if the snapshot is a promise.
func (cs ClientSnapshot) IsPromise() bool {
	if cs.hook == nil {
		return false
	}
	_, ret := cs.hook.Value().resolution.Get()
	return ret
}

// IsResolved returns true if the snapshot has resolved to its final value.
// If IsPromise() returns false, then this will also return false. Otherwise,
// it returns false before resolution and true afterwards.
func (cs ClientSnapshot) IsResolved() bool {
	if cs.hook == nil {
		return false
	}
	res, ok := cs.hook.Value().resolution.Get()
	if !ok {
		return false
	}
	return mutex.With1(res, func(s *resolveState) bool {
		return s.isResolved()
	})
}

// Send implements ClientHook.Send
func (cs ClientSnapshot) Send(ctx context.Context, s Send) (*Answer, ReleaseFunc) {
	if cs.hook == nil {
		return ErrorAnswer(s.Method, errors.New("call on null client")), func() {}
	}
	return cs.hook.Value().Send(ctx, s)
}

// Recv implements ClientHook.Recv
func (cs ClientSnapshot) Recv(ctx context.Context, r Recv) PipelineCaller {
	if cs.hook == nil {
		r.Reject(errors.New("call on null client"))
		return nil
	}
	return cs.hook.Value().Recv(ctx, r)
}

// Client returns a client pointing at the most-resolved version of the snapshot.
func (cs ClientSnapshot) Client() Client {
	cursor := rc.NewRefInPlace(func(c *clientCursor) func() {
		*c = clientCursor{hook: mutex.New(cs.hook.AddRef())}
		c.compress()
		return c.Release
	})
	c := Client{client: &client{
		state: mutex.New(clientState{cursor: cursor}),
	}}
	setupLeakReporting(c)
	return c
}

// Brand is the value returned from the ClientHook's Brand method.
// Returns the zero Brand if the receiver is the zero ClientSnapshot.
func (cs ClientSnapshot) Brand() Brand {
	if cs.hook == nil {
		return Brand{}
	}
	return cs.hook.Value().Brand()
}

// Return a the reference to the Metadata associated with this client hook.
// Callers may store whatever they need here.
func (cs ClientSnapshot) Metadata() *Metadata {
	return &cs.hook.Value().metadata
}

// Create a copy of the snapshot, with its own underlying reference.
func (cs ClientSnapshot) AddRef() ClientSnapshot {
	cs.hook = cs.hook.AddRef()
	setupLeakReporting(cs)
	return cs
}

// Release the reference to the hook.
func (cs *ClientSnapshot) Release() {
	cs.hook.Release()
}

// Shutdown is an alias for Release, to implement ClientHook.
func (cs *ClientSnapshot) Shutdown() {
	cs.Release()
}

func (cs *ClientSnapshot) Resolve1(ctx context.Context) error {
	var err error
	cs.hook, _, err = resolve1ClientHook(ctx, cs.hook)
	setupLeakReporting(*cs)
	return err
}

func (cs *ClientSnapshot) resolve1(ctx context.Context) (more bool, err error) {
	cs.hook, more, err = resolve1ClientHook(ctx, cs.hook)
	return
}

func (cs *ClientSnapshot) Resolve(ctx context.Context) error {
	var err error
	cs.hook, err = resolveClientHook(ctx, cs.hook)
	setupLeakReporting(*cs)
	return err
}

func resolveClientHook(ctx context.Context, h *rc.Ref[clientHook]) (_ *rc.Ref[clientHook], err error) {
	for {
		var more bool
		h, more, err = resolve1ClientHook(ctx, h)
		if !more || err != nil {
			return h, err
		}
	}
}

func resolve1ClientHook(ctx context.Context, h *rc.Ref[clientHook]) (_ *rc.Ref[clientHook], more bool, err error) {
	if !h.IsValid() {
		return h, false, nil
	}
	defer h.Release()

	r, ok := h.Value().resolution.Get()
	if !ok {
		return h.AddRef(), false, nil
	}

	resolvedCh := mutex.With1(r, func(s *resolveState) <-chan struct{} {
		return s.resolved
	})

	select {
	case <-resolvedCh:
		rh := mutex.With1(r, func(r *resolveState) *rc.Ref[clientHook] {
			return r.resolvedHook
		})
		if rh == nil {
			return nil, false, nil
		}
		return rh.AddRef(), true, nil
	case <-ctx.Done():
		return h.AddRef(), true, ctx.Err()
	}
}

// String returns a string that identifies this capability for debugging
// purposes.  Its format should not be depended on: in particular, it
// should not be used to compare clients.  Use IsSame to compare clients
// for equality.
func (c Client) String() string {
	if c.client == nil {
		return "<nil>"
	}
	h, resolved, released := c.startCall()
	defer h.Release()
	if released {
		return "<released client>"
	}
	if h == nil {
		return "<nil>"
	}
	var s string
	if resolved {
		s = "<client " + h.Value().ClientHook.String() + ">"
	} else {
		s = "<unresolved client " + h.Value().ClientHook.String() + ">"
	}
	return s
}

// Release releases a capability reference.  If this is the last
// reference to the capability, then the underlying resources associated
// with the capability will be released.
//
// Release has no effect if c has already been released, or if c is
// nil or resolved to null.
func (c Client) Release() {
	if c.client == nil {
		return
	}
	limiter := c.GetFlowLimiter()
	c.state.With(func(s *clientState) {
		if !s.released {
			s.released = true
			s.cursor.Release()
			limiter.Release()
		}
	})
}

func (c Client) EncodeAsPtr(seg *Segment) Ptr {
	capId := seg.Message().CapTable().Add(c)
	return NewInterface(seg, capId).ToPtr()
}

func (Client) DecodeFromPtr(p Ptr) Client {
	return p.Interface().Client()
}

var _ TypeParam[Client] = Client{}

// isResolve reports whether the clientHook s belongs to is resolved.
func (s *resolveState) isResolved() bool {
	select {
	case <-s.resolved:
		return true
	default:
		return false
	}
}

var setupLeakReporting func(any) = func(any) {}

// SetClientLeakFunc sets a callback for reporting Clients that went
// out of scope without being released.  The callback is not guaranteed
// to be called and must be safe to call concurrently from multiple
// goroutines.  The exact format of the message is unspecified.
//
// SetClientLeakFunc must not be called after any calls to NewClient or
// NewPromisedClient.
func SetClientLeakFunc(clientLeakFunc func(msg string)) {
	setupLeakReporting = func(v any) {
		buf := bufferpool.Default.Get(1e6)
		n := runtime.Stack(buf, false)
		stack := string(buf[:n])
		bufferpool.Default.Put(buf)
		switch c := v.(type) {
		case Client:
			runtime.SetFinalizer(c.client, func(c *client) {
				released := mutex.With1(&c.state, func(c *clientState) bool {
					return c.released
				})
				if released {
					return
				}
				clientLeakFunc("leaked client created at:\n\n" + stack)
			})
		case ClientSnapshot:
			if !c.IsValid() {
				return
			}
			runtime.SetFinalizer(c.hook, func(c *rc.Ref[clientHook]) {
				if !c.IsValid() {
					return
				}
				clientLeakFunc("leaked client snapshot created at:\n\n" + stack)
			})
		default:
			panic("setupLeakReporting called on unrecognized type!")
		}
	}
}

// A ClientPromise resolves the identity of a client created by NewPromisedClient.
type clientPromise struct {
	cursor *rc.WeakRef[clientCursor]
}

func (cp *clientPromise) Reject(err error) {
	cp.Fulfill(ErrorClient(err))
}

// Fulfill resolves the client promise to c.  After Fulfill returns,
// then all future calls to the client created by NewPromisedClient will
// be sent to c.  It is guaranteed that the hook passed to
// NewPromisedClient will be shut down after Fulfill returns, but the
// hook may have been shut down earlier if the client ran out of
// references.
func (cp *clientPromise) Fulfill(c Client) {
	dq := &deferred.Queue{}
	defer dq.Run()
	cp.fulfill(dq, c)
}

// fulfill is like Fulfill, except that it does not wait for outsanding calls
// to return answers or shut down the underlying hook; instead, it adds functions
// to do this to dq.
func (cp *clientPromise) fulfill(dq *deferred.Queue, c Client) {
	cursor, ok := cp.cursor.AddRef()
	if !ok {
		return
	}
	dq.Defer(cursor.Release)

	// Obtain next client hook.
	var rh *rc.Ref[clientHook]
	if (c != Client{}) {
		h, _, released := c.startCall()
		if released {
			panic("ClientPromise.Fulfill with a released client")
		}
		rh = h
	}

	// Mark hook as resolved.
	cursor.Value().hook.With(func(h **rc.Ref[clientHook]) {
		r, ok := (*h).Value().resolution.Get()
		if !ok {
			panic("BUG: clientPromise referred to a clientHook that was not a promise")
		}
		r.With(func(s *resolveState) {
			if s.isResolved() {
				panic("ClientPromise.Fulfill called more than once")
			}
			s.resolvedHook = rh
			close(s.resolved)
		})
	})
	cursor.Value().compress()
}

// A WeakClient is a weak reference to a capability: it refers to a
// capability without preventing it from being shut down.  The zero
// value is a null reference.
type WeakClient struct {
	r *rc.WeakRef[clientCursor]
}

// AddRef creates a new Client that refers to the same capability as c
// as long as the capability hasn't already been shut down.
func (wc WeakClient) AddRef() (c Client, ok bool) {
	if wc.r == nil {
		return Client{}, true
	}
	cursor, ok := wc.r.AddRef()
	if !ok {
		return Client{}, false
	}
	c = Client{client: &client{state: mutex.New(clientState{cursor: cursor})}}
	setupLeakReporting(c)
	return c, true
}

// A ClientHook represents a Cap'n Proto capability.  Application code
// should not pass around ClientHooks; applications should pass around
// Clients.  A ClientHook must be safe to use from multiple goroutines.
//
// Calls must be delivered to the capability in the order they are made.
// This guarantee is based on the concept of a capability acknowledging
// delivery of a call: this is specific to an implementation of ClientHook.
// A type that implements ClientHook must guarantee that if foo() then bar()
// is called on a client, then the capability acknowledging foo() happens
// before the capability observing bar().
//
// ClientHook is an internal interface.  Users generally SHOULD NOT supply
// their own implementations.
type ClientHook interface {
	// Send allocates space for parameters, calls s.PlaceArgs to fill out
	// the arguments, then starts executing a method, returning an answer
	// that will hold the result.  The hook must call s.PlaceArgs at most
	// once, and if it does call s.PlaceArgs, it must return before Send
	// returns.  The caller must call the returned release function when
	// it no longer needs the answer's data.
	//
	// Send is typically used when application code is making a call.
	Send(ctx context.Context, s Send) (*Answer, ReleaseFunc)

	// Recv starts executing a method with the referenced arguments
	// and places the result in a message controlled by the caller.
	// The hook will call r.ReleaseArgs when it no longer needs to
	// reference the parameters and use r.Returner to complete the method
	// call.  If Recv does not call r.Returner.Return before it returns,
	// then it must return a non-nil PipelineCaller.
	//
	// Recv is typically used when the RPC system has received a call.
	Recv(ctx context.Context, r Recv) PipelineCaller

	// Brand returns an implementation-specific value.  This can be used
	// to introspect and identify kinds of clients.
	Brand() Brand

	// Shutdown releases any resources associated with this capability.
	// The behavior of calling any methods on the receiver after calling
	// Shutdown is undefined. Shutdown must not interrupt any already
	// outstanding calls.
	Shutdown()

	// String formats the hook as a string (same as fmt.Stringer)
	String() string
}

// Send is the input to ClientHook.Send.
type Send struct {
	// Method must have InterfaceID and MethodID filled in.
	Method Method

	// PlaceArgs is a function that will be called at most once before Send
	// returns to populate the arguments for the RPC.  PlaceArgs may be nil.
	PlaceArgs func(Struct) error

	// ArgsSize specifies the size of the struct to pass to PlaceArgs.
	ArgsSize ObjectSize
}

// Recv is the input to ClientHook.Recv.
type Recv struct {
	// Method must have InterfaceID and MethodID filled in.
	Method Method

	// Args is the set of arguments for the RPC.
	Args Struct

	// ReleaseArgs is called after Args is no longer referenced.
	// Must not be nil. If called more than once, subsequent calls
	// must silently no-op.
	ReleaseArgs ReleaseFunc

	// Returner manages the results.
	Returner Returner
}

// AllocResults allocates a result struct.  It is the same as calling
// r.Returner.AllocResults(sz).
func (r Recv) AllocResults(sz ObjectSize) (Struct, error) {
	return r.Returner.AllocResults(sz)
}

// Return ends the method call successfully, releasing the arguments.
func (r Recv) Return() {
	r.ReleaseArgs()
	r.Returner.PrepareReturn(nil)
	r.Returner.Return()
}

// Reject ends the method call with an error, releasing the arguments.
func (r Recv) Reject(e error) {
	if e == nil {
		panic("Reject(nil)")
	}
	r.ReleaseArgs()
	r.Returner.PrepareReturn(e)
	r.Returner.Return()
}

// A Returner allocates and sends the results from a received
// capability method call.
type Returner interface {
	// AllocResults allocates the results struct that will be sent using
	// Return.  It can be called at most once, and only before calling
	// Return.  The struct returned by AllocResults must not be mutated
	// after Return is called, and may not be accessed after
	// ReleaseResults is called.
	AllocResults(sz ObjectSize) (Struct, error)

	// PrepareReturn finalizes the return message. The method call will
	// resolve successfully if e is nil, or otherwise it will return an
	// exception to the caller.
	//
	// PrepareReturn must be called once.
	//
	// After PrepareReturn is invoked, no goroutine may modify the message
	// containing the results.
	PrepareReturn(e error)

	// Return resolves the method call, using the results finalized in
	// PrepareReturn. Return must be called once.
	//
	// Return must wait for all ongoing pipelined calls to be delivered,
	// and after it returns, no new calls can be sent to the PipelineCaller
	// returned from Recv.
	Return()

	// ReleaseResults relinquishes the caller's access to the message
	// containing the results; once this is called the message may be
	// released or reused, and it is not safe to access.
	//
	// If AllocResults has not been called, then this is a no-op.
	ReleaseResults()
}

// A ReleaseFunc tells the RPC system that a parameter or result struct
// is no longer in use and may be reclaimed.  After the first call,
// subsequent calls to a ReleaseFunc do nothing.  A ReleaseFunc should
// not be called concurrently.
type ReleaseFunc func()

// A Method identifies a method along with an optional human-readable
// description of the method.
type Method struct {
	InterfaceID uint64
	MethodID    uint16

	// Canonical name of the interface.  May be empty.
	InterfaceName string
	// Method name as it appears in the schema.  May be empty.
	MethodName string
}

// String returns a formatted string containing the interface name or
// the method name if present, otherwise it uses the raw IDs.
// This is suitable for use in error messages and logs.
func (m *Method) String() string {
	buf := make([]byte, 0, 128)
	if m.InterfaceName == "" {
		buf = append(buf, '@', '0', 'x')
		buf = strconv.AppendUint(buf, m.InterfaceID, 16)
	} else {
		buf = append(buf, m.InterfaceName...)
	}
	buf = append(buf, '.')
	if m.MethodName == "" {
		buf = append(buf, '@')
		buf = strconv.AppendUint(buf, uint64(m.MethodID), 10)
	} else {
		buf = append(buf, m.MethodName...)
	}
	return string(buf)
}

type errorClient struct {
	e error
}

// ErrorClient returns a Client that always returns error e.
// An ErrorClient does not need to be released: it is a sentinel like a
// nil Client.
//
// The returned client's State() method returns a State with its
// Brand.Value set to e.
func ErrorClient(e error) Client {
	if e == nil {
		panic("ErrorClient(nil)")
	}

	// Avoid NewClient because it can set a finalizer.
	h := clientHook{
		ClientHook: errorClient{e},
		metadata:   *NewMetadata(),
	}
	cs := mutex.New(clientState{cursor: newClientCursor(h)})
	return Client{client: &client{state: cs}}
}

func (ec errorClient) Send(_ context.Context, s Send) (*Answer, ReleaseFunc) {
	return ErrorAnswer(s.Method, ec.e), func() {}
}

func (ec errorClient) Recv(_ context.Context, r Recv) PipelineCaller {
	r.Reject(ec.e)
	return nil
}

func (ec errorClient) Brand() Brand {
	return Brand{Value: ec.e}
}

func (ec errorClient) Shutdown() {
}

func (ec errorClient) String() string {
	return "errorClient{" + ec.e.Error() + "}"
}

var closedSignal = make(chan struct{})
