// Package server provides runtime support for implementing Cap'n Proto
// interfaces locally.
package server // import "capnproto.org/go/capnp/v3/server"

import (
	"context"
	"sort"
	"sync"

	"capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/exc"
	"capnproto.org/go/capnp/v3/exp/mpsc"
	"capnproto.org/go/capnp/v3/internal/str"
)

// A Method describes a single capability method on a server object.
type Method struct {
	capnp.Method
	Impl func(context.Context, *Call) error
}

// Call holds the state of an ongoing capability method call.
// A Call cannot be used after the server method returns.
type Call struct {
	ctx    context.Context
	method *Method
	recv   capnp.Recv
	aq     *capnp.AnswerQueue
	srv    *Server

	alloced bool
	results capnp.Struct

	acked bool
}

// Args returns the call's arguments.  Args is not safe to
// reference after a method implementation returns.  Args is safe to
// call and read from multiple goroutines.
func (c *Call) Args() capnp.Struct {
	return c.recv.Args
}

// AllocResults allocates the results struct.  It is an error to call
// AllocResults more than once.
func (c *Call) AllocResults(sz capnp.ObjectSize) (capnp.Struct, error) {
	if c.alloced {
		return capnp.Struct{}, newError("multiple calls to AllocResults")
	}
	var err error
	c.alloced = true
	c.results, err = c.recv.Returner.AllocResults(sz)
	return c.results, err
}

// Go is a function that is called to unblock future calls; by default
// a server only accepts one method call at a time, waiting until
// the method returns before servicing the next method in the queue.
// calling Go spawns another goroutine to service additional Calls
// in the queue, allowing the current goroutine to block, do expensive
// computation, etc. without holding up other calls. If Go is called,
// the calling Goroutine exits when the method returns, so that there
// is never more than one goroutine pulling things from the queue.
//
// Go need not be the first call in a function nor is it required.
// short functions can return without calling Go.
func (c *Call) Go() {
	if c.acked {
		return
	}
	c.acked = true
	go c.srv.handleCalls()
}

// Shutdowner is the interface that wraps the Shutdown method.
type Shutdowner interface {
	Shutdown()
}

// A Server is a locally implemented interface.  It implements the
// capnp.ClientHook interface.
type Server struct {
	methods  sortedMethods
	brand    any
	shutdown Shutdowner

	// wg is incremented each time a method is queued, and
	// decremented after it is handled.
	wg sync.WaitGroup

	// Calls are inserted into this queue, to be handled
	// by a goroutine running handleCalls()
	callQueue *mpsc.Queue[*Call]

	// Handler for custom behavior of unknown methods
	HandleUnknownMethod func(m capnp.Method) *Method

	// Arena implementation
	NewArena func() capnp.Arena
}

func (s *Server) String() string {
	return "*Server@0x" + str.PtrToHex(s)
}

// New returns a client hook that makes calls to a set of methods.
// If shutdown is nil then the server's shutdown is a no-op.  The server
// guarantees message delivery order by blocking each call on the
// return of the previous call or a call to Call.Go.
func New(methods []Method, brand any, shutdown Shutdowner) *Server {
	srv := &Server{
		methods:   make(sortedMethods, len(methods)),
		brand:     brand,
		shutdown:  shutdown,
		callQueue: mpsc.New[*Call](),
	}
	copy(srv.methods, methods)
	sort.Sort(srv.methods)
	go srv.handleCalls()
	return srv
}

// Send starts a method call.
func (srv *Server) Send(ctx context.Context, s capnp.Send) (*capnp.Answer, capnp.ReleaseFunc) {
	mm := srv.methods.find(s.Method)
	if mm == nil && srv.HandleUnknownMethod != nil {
		mm = srv.HandleUnknownMethod(s.Method)
	}
	if mm == nil {
		return capnp.ErrorAnswer(s.Method, capnp.Unimplemented("unimplemented")), func() {}
	}
	args, err := srv.sendArgsToStruct(s)
	if err != nil {
		return capnp.ErrorAnswer(mm.Method, err), func() {}
	}
	ret := new(capnp.StructReturner)
	pcaller := srv.start(ctx, mm, capnp.Recv{
		Method: mm.Method, // pick up names from server method
		Args:   args,
		ReleaseArgs: func() {
			if msg := args.Message(); msg != nil {
				msg.Release()
				args = capnp.Struct{}
			}
		},
		Returner: ret,
	})
	return ret.Answer(mm.Method, pcaller)
}

// Recv starts a method call.
func (srv *Server) Recv(ctx context.Context, r capnp.Recv) capnp.PipelineCaller {
	mm := srv.methods.find(r.Method)
	if mm == nil && srv.HandleUnknownMethod != nil {
		mm = srv.HandleUnknownMethod(r.Method)
	}
	if mm == nil {
		r.Reject(capnp.Unimplemented("unimplemented"))
		return nil
	}
	return srv.start(ctx, mm, r)
}

func (srv *Server) handleCalls() {
	ctx := context.Background()
	for {
		call, err := srv.callQueue.Recv(ctx)
		if err != nil {
			// Queue closed; wait for outstanding calls and shut down.
			if srv.shutdown != nil {
				srv.wg.Wait()
				srv.shutdown.Shutdown()
			}
			return
		}

		srv.handleCall(call)
		if call.acked {
			// Another goroutine has taken over; time
			// to retire.
			return
		}
	}
}

func (srv *Server) handleCall(c *Call) {
	defer srv.wg.Done()

	err := c.method.Impl(c.ctx, c)

	c.recv.ReleaseArgs()
	c.recv.Returner.PrepareReturn(err)
	if err == nil {
		c.aq.Fulfill(c.results.ToPtr())
	} else {
		c.aq.Reject(err)
	}
	c.recv.Returner.Return()
	c.recv.Returner.ReleaseResults()
}

func (srv *Server) start(ctx context.Context, m *Method, r capnp.Recv) capnp.PipelineCaller {
	srv.wg.Add(1)

	aq := capnp.NewAnswerQueue(r.Method)
	srv.callQueue.Send(&Call{
		ctx:    ctx,
		method: m,
		recv:   r,
		aq:     aq,
		srv:    srv,
	})
	return aq
}

// Brand returns a value that will match IsServer.
func (srv *Server) Brand() capnp.Brand {
	return capnp.Brand{Value: serverBrand{srv.brand}}
}

// Shutdown arranges for Shutdown to be called on the Shutdowner passed
// into NewServer after outstanding all calls have been serviced.
// Shutdown must not be called more than once.
func (srv *Server) Shutdown() {
	srv.callQueue.Close()
}

// IsServer reports whether a brand returned by capnp.Client.Brand
// originated from Server.Brand, and returns the brand argument passed
// to New.
func IsServer(brand capnp.Brand) (_ any, ok bool) {
	sb, ok := brand.Value.(serverBrand)
	return sb.x, ok
}

type serverBrand struct {
	x any
}

func (srv *Server) sendArgsToStruct(s capnp.Send) (capnp.Struct, error) {
	if s.PlaceArgs == nil {
		return capnp.Struct{}, nil
	}

	if srv.NewArena == nil {
		srv.NewArena = func() capnp.Arena {
			// TODO:  change to single segment?
			return capnp.MultiSegment(nil)
		}
	}

	_, seg, err := capnp.NewMessage(srv.NewArena())
	if err != nil {
		return capnp.Struct{}, err
	}
	st, err := capnp.NewRootStruct(seg, s.ArgsSize)
	if err != nil {
		return capnp.Struct{}, err
	}
	if err := s.PlaceArgs(st); err != nil {
		st.Message().Release()
		return capnp.Struct{}, exc.WrapError("place args", err)
	}
	return st, nil
}

type sortedMethods []Method

// find returns the method with the given ID or nil.
func (sm sortedMethods) find(id capnp.Method) *Method {
	i := sort.Search(len(sm), func(i int) bool {
		m := &sm[i]
		if m.InterfaceID != id.InterfaceID {
			return m.InterfaceID >= id.InterfaceID
		}
		return m.MethodID >= id.MethodID
	})
	if i == len(sm) {
		return nil
	}
	m := &sm[i]
	if m.InterfaceID != id.InterfaceID || m.MethodID != id.MethodID {
		return nil
	}
	return m
}

func (sm sortedMethods) Len() int {
	return len(sm)
}

func (sm sortedMethods) Less(i, j int) bool {
	if id1, id2 := sm[i].InterfaceID, sm[j].InterfaceID; id1 != id2 {
		return id1 < id2
	}
	return sm[i].MethodID < sm[j].MethodID
}

func (sm sortedMethods) Swap(i, j int) {
	sm[i], sm[j] = sm[j], sm[i]
}

func newError(msg string) error {
	return exc.New(exc.Failed, "capnp server", msg)
}
