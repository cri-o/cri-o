// Package rpc implements the Cap'n Proto RPC protocol.
package rpc // import "capnproto.org/go/capnp/v3/rpc"

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/exc"
	"capnproto.org/go/capnp/v3/exp/spsc"
	"capnproto.org/go/capnp/v3/internal/str"
	"capnproto.org/go/capnp/v3/internal/syncutil"
	"capnproto.org/go/capnp/v3/rpc/transport"
	rpccp "capnproto.org/go/capnp/v3/std/capnp/rpc"
	"capnproto.org/go/capnp/v3/util"
	"capnproto.org/go/capnp/v3/util/deferred"
)

/*
At a high level, Conn manages three resources:

1) The connection's state: the tables
2) The transport's outbound stream
3) The transport's inbound stream

Each of these resources require mutually exclusive access.  Complexity
ensues because there are two primary actors contending for these
resources: the local vat (sometimes referred to as the application) and
the remote vat.  In this implementation, the remote vat is represented
by a goroutine that is solely responsible for the inbound stream.  This
is referred to as the receive goroutine.  The local vat accesses the
Conn via objects created by the Conn, and may do so from many different
goroutines.  However, the Conn will largely serialize operations coming
from the local vat.  Similarly, outbound messages are enqueued on 'sendTx',
and processed by a single goroutine.

Conn protects the connection state with a simple mutex, embedded in c.lk
to make it clear which fields it protects.  This mutex must not be held
while performing operations that take indeterminate time or are provided
by the application.  This reduces contention, but more importantly,
prevents deadlocks.  An application- provided operation can (and
commonly will) call back into the Conn.

Note that importantly, capnp.ClientHooks should be considered
potentially "provided by the application," since it is an interface,
and since capnp.Clients and capnp.Promises may call into ClientHooks,
methods on those should not be invoked while holding the lock; in the
past this has been the source of *many* bugs.

The receive goroutine, being the only goroutine that receives messages
from the transport, can receive from the transport without additional
synchronization.  One intentional side effect of this arrangement is
that during processing of a message, no other messages will be received.
This provides backpressure to the remote vat as well as simplifying some
message processing.  However, it is essential that the receive goroutine
never block while processing a message.  In other words, the receive
goroutine may only block when waiting for an incoming message.

Some advice for those working on this code:

Many functions are verbose; resist the urge to shorten them.  There's
a lot more going on in this code than in most code, and many steps
require complicated invariants.  Only extract common functionality if
the preconditions are simple.

Avoid complex locking patterns; if you can't express what you need to
do with Conn.withLocked or one of its friends, you should go for a walk
and think, rather than hacking around it by accessing the lock directly
and/or doing something more complex. In the long run, it's never worth
it.

Try to push lock acquisition as high up in the call stack as you can.
This makes it easy to see and avoid extraneous lock transitions, which
is a common source of errors and/or inefficiencies.
*/

// A Conn is a connection to another Cap'n Proto vat.
// It is safe to use from multiple goroutines.
type Conn struct {
	remotePeerID PeerID
	network      Network

	bootstrap    capnp.Client
	er           errReporter
	abortTimeout time.Duration

	// bgctx is a Context that is canceled when shutdown starts. Note
	// that it's parent is context.Background(), so we can rely on this
	// being the *only* time it will be canceled.
	bgctx context.Context

	// tasks block shutdown.
	tasks  sync.WaitGroup
	closed chan struct{} // closed when shutdown() returns

	// The underlying transport. Only the reader & writer goroutines
	// may do IO, but any thread may call NewMessage().
	transport Transport

	// Receive end of the send queue (lk.sendTx). Only the send goroutine may
	// touch this.
	sendRx *spsc.Rx[asyncSend]

	// lk contains all the fields that need to be protected by a mutex.
	// this makes it easy to tell at call sites whether you should or
	// should not be holding the lock. Methods that access fields within
	// should be defined on lockedConn, rather than Conn, so that callers
	// must invoke c.withLocked or one of its variants.
	lk struct {
		sync.Mutex // protects all the fields in lk.

		// The send end of the send queue; outgoing messages are inserted
		// here to be sent by a dedicated goroutine.
		sendTx *spsc.Tx[asyncSend]

		closing  bool               // used to make shutdown() idempotent
		bgcancel context.CancelFunc // bgcancel cancels bgctx.

		// Tables
		questions  []*question
		questionID idgen[questionID]
		answers    map[answerID]*ansent
		exports    []*expent
		exportID   idgen[exportID]
		imports    map[importID]*impent
		embargoes  []*embargo
		embargoID  idgen[embargoID]
	}
}

// A lockedConn is the same as a Conn, but the methods defined on it
// access c.lk without synchronization. Therefore callers must already
// be holding the lock. Conn.withLock can be used to simultaneously
// delmit a critical section and grant access to a lockedConn.
type lockedConn Conn

// withLocked runs f while holding c.lk. It is considered good style
// to shadow the variable name for the receiver with the parameter
// to f, e.g.
//
// c.withLocked(func(c *lockedConn) { ... })
//
// This makes it hard to accidentally use the Conn directly while
// holding the lock, which could result in deadlocks if withLocked
// is called reentrantly.
func (c *Conn) withLocked(f func(*lockedConn)) {
	syncutil.With(&c.lk, func() {
		f((*lockedConn)(c))
	})
}

// withLockedConn1 is like c.withLocked, but allows the function f
// to return a value. It is a stand-alone function so the return type
// can be generic.
func withLockedConn1[T any](c *Conn, f func(*lockedConn) T) T {
	var ret T
	c.withLocked(func(c *lockedConn) {
		ret = f(c)
	})
	return ret
}

// withLockedConn2 is like withLockedConn1, except that the function
// has two return values.
func withLockedConn2[A, B any](c *Conn, f func(*lockedConn) (A, B)) (a A, b B) {
	c.withLocked(func(c *lockedConn) {
		a, b = f(c)
	})
	return
}

// Options specifies optional parameters for creating a Conn.
type Options struct {
	// BootstrapClient is the capability that will be returned to the
	// remote peer when receiving a Bootstrap message.  NewConn "steals"
	// this reference: it will release the client when the connection is
	// closed.
	BootstrapClient capnp.Client

	// Logger is used for logging by the RPC system, including errors that
	// occur while the Conn is receiving messages from the remote vat.
	Logger Logger

	// AbortTimeout specifies how long to block on sending an abort message
	// before closing the transport.  If zero, then a reasonably short
	// timeout is used.
	AbortTimeout time.Duration

	// RemotePeerID is the PeerID of the remote side of the connection. Can
	// be left as the zero value for point to point connections. For >= 3
	// party use, this should be filled in by the Network on Accept or Dial.
	// Application code should not set this.
	RemotePeerID PeerID

	// A reference to the Network that this connection is a part of.  Can be
	// left nil for point to point connections. Otherwise, this must be set
	// by Dial or Accept on the Network itself; application code should not
	// set this.
	Network Network
}

// Logger is used for logging by the RPC system. Each method logs
// messages at a different level, but otherwise has the same semantics:
//
//   - Message is a human-readable description of the log event.
//   - Args is a sequenece of key, value pairs, where the keys must be strings
//     and the values may be any type.
//   - The methods may not block for long periods of time.
//
// This interface is designed such that it is satisfied by *slog.Logger.
type Logger interface {
	Debug(message string, args ...any)
	Info(message string, args ...any)
	Warn(message string, args ...any)
	Error(message string, args ...any)
}

// NewConn creates a new connection that communicates on a given transport.
//
// Closing the connection will close the transport and release the bootstrap
// client provided in opts.
//
// If opts == nil, sensible defaults are used. See Options for more info.
//
// Once a connection is created, it will immediately start receiving
// requests from the transport.
func NewConn(t Transport, opts *Options) *Conn {
	c := &Conn{
		transport: t,
		closed:    make(chan struct{}),
	}

	sender := spsc.New[asyncSend]()
	c.sendRx = &sender.Rx
	c.lk.sendTx = &sender.Tx

	c.lk.answers = make(map[answerID]*ansent)
	c.lk.imports = make(map[importID]*impent)

	if opts != nil {
		c.bootstrap = opts.BootstrapClient
		c.er = errReporter{opts.Logger}
		c.abortTimeout = opts.AbortTimeout
		c.network = opts.Network
		c.remotePeerID = opts.RemotePeerID
	}
	if c.abortTimeout == 0 {
		c.abortTimeout = 100 * time.Millisecond
	}

	c.startBackgroundTasks()

	return c
}

func (c *Conn) startBackgroundTasks() {
	// We use an errgroup to link the lifetime of background tasks
	// to each other.
	ctx, cancel := context.WithCancel(context.Background())
	g, ctx := errgroup.WithContext(ctx)

	c.bgctx = ctx
	c.lk.bgcancel = cancel

	g.Go(c.send(ctx))
	g.Go(c.receive(ctx))

	// Wait for tasks to complete.
	go func() {
		err := g.Wait()

		// Treat context.Canceled as a success indicator.
		// Do not report or send an abort message.
		if errors.Is(err, context.Canceled) {
			err = nil
		}

		c.er.ReportError(err) // ignores nil errors
		c.er.ReportError(c.shutdown(err))
	}()
}

func (c *Conn) backgroundTask(f func() error) func() error {
	c.tasks.Add(1)

	return func() (err error) {
		defer c.tasks.Done()

		// backgroundTask MUST return a non-nil error in order to signal
		// other tasks to stop.  The context.Canceled will be treated as
		// a success indicator by the caller.
		if err = f(); err == nil {
			err = context.Canceled
		}

		return err
	}
}

// Return the peer ID for the remote side of the connection. Returns
// the zero value if this connection was set up with NewConn instead
// of via a Network.
func (c *Conn) RemotePeerID() PeerID {
	return c.remotePeerID
}

// Bootstrap returns the remote vat's bootstrap interface.  This creates
// a new client that the caller is responsible for releasing.
func (c *Conn) Bootstrap(ctx context.Context) (bc capnp.Client) {
	return withLockedConn1(c, func(c *lockedConn) (bc capnp.Client) {
		// Start a background task to prevent the conn from shutting down
		// while sending the bootstrap message.
		if !c.startTask() {
			return capnp.ErrorClient(rpcerr.Disconnected(errors.New("connection closed")))
		}
		defer c.tasks.Done()

		q := c.newQuestion(capnp.Method{})
		bc = q.p.Answer().Client().AddRef()
		go func() {
			q.p.ReleaseClients()
			q.release()
		}()

		c.sendMessage(ctx, func(m rpccp.Message) error {
			boot, err := m.NewBootstrap()
			if err == nil {
				boot.SetQuestionId(uint32(q.id))
			}
			return err

		}, func(err error) {
			if err != nil {
				syncutil.With(&c.lk, func() {
					c.lk.questions[q.id] = nil
				})
				q.p.Reject(exc.Annotate("rpc", "bootstrap", err))
				syncutil.With(&c.lk, func() {
					c.lk.questionID.remove(q.id)
				})
				return
			}

			c.tasks.Add(1)
			go func() {
				defer c.tasks.Done()
				q.handleCancel(ctx)
			}()
		})

		return
	})
}

// Close sends an abort to the remote vat and closes the underlying
// transport.
func (c *Conn) Close() error {
	return c.shutdown(exc.Exception{ // NOTE:  omit "rpc" prefix
		Type:  exc.Failed,
		Cause: ErrConnClosed,
	})
}

// Done returns a channel that is closed after the connection is
// shut down.
func (c *Conn) Done() <-chan struct{} {
	return c.closed
}

// shutdown tears down the connection and transport, optionally sending
// an abort message before closing.  The caller MUST NOT hold c.lk.
// shutdown is idempotent.
func (c *Conn) shutdown(abortErr error) (err error) {
	alreadyClosing := false

	c.withLocked(func(c *lockedConn) {
		alreadyClosing = c.lk.closing
		if !alreadyClosing {
			c.lk.closing = true
			c.lk.bgcancel()
			c.cancelTasks()
		}
	})

	if !alreadyClosing {
		readyForClose := make(chan struct{})
		go func() {
			defer close(c.closed)
			select {
			case <-readyForClose:
			case <-time.After(c.abortTimeout):
			}
			if err = c.transport.Close(); err != nil {
				err = rpcerr.WrapFailed("close transport", err)
			}
		}()

		c.tasks.Wait()
		c.drainQueue()

		dq := &deferred.Queue{}
		c.withLocked(func(c *lockedConn) {
			c.release(dq)
		})
		dq.Run()
		c.abort(abortErr)
		close(readyForClose)
	}
	<-c.closed

	return
}

// Cancel all tasks and prevent new tasks from being started.
// Does not wait for tasks to finish shutting down.
// Called by 'shutdown'.  Callers MUST hold c.lk.
func (c *lockedConn) cancelTasks() {
	for _, a := range c.lk.answers {
		if a != nil && a.cancel != nil {
			a.cancel()
		}
	}
}

// caller MUST NOT hold c.lk
func (c *Conn) drainQueue() {
	for {
		pending, ok := c.sendRx.TryRecv()
		if !ok {
			break
		}

		pending.Abort(ErrConnClosed)
	}
}

// Clear all tables, and arrange for the deferred.Queue to release exported clients
// and unfinished answers. Called by 'shutdown'.  Caller MUST hold c.lk.
func (c *lockedConn) release(dq *deferred.Queue) {
	exports := c.lk.exports
	embargoes := c.lk.embargoes
	answers := c.lk.answers
	questions := c.lk.questions
	c.lk.imports = nil
	c.lk.exports = nil
	c.lk.embargoes = nil
	c.lk.questions = nil
	c.lk.answers = nil

	c.releaseBootstrap(dq)
	c.releaseExports(dq, exports)
	c.liftEmbargoes(dq, embargoes)
	c.releaseAnswers(dq, answers)
	c.releaseQuestions(dq, questions)

}

func (c *lockedConn) releaseBootstrap(dq *deferred.Queue) {
	dq.Defer(c.bootstrap.Release)
	c.bootstrap = capnp.Client{}
}

func (c *lockedConn) releaseExports(dq *deferred.Queue, exports []*expent) {
	for _, e := range exports {
		if e != nil {
			metadata := e.snapshot.Metadata()
			if metadata != nil {
				syncutil.With(metadata, func() {
					c.clearExportID(metadata)
				})
			}
			dq.Defer(e.snapshot.Release)
		}
	}
}

func (c *lockedConn) liftEmbargoes(dq *deferred.Queue, embargoes []*embargo) {
	for _, e := range embargoes {
		if e != nil {
			dq.Defer(e.lift)
		}
	}
}

func (c *lockedConn) releaseAnswers(dq *deferred.Queue, answers map[answerID]*ansent) {
	for _, a := range answers {
		if a != nil {
			for _, s := range a.returner.resultsCapTable {
				dq.Defer(s.Release)
			}
			if a.returner.msgReleaser != nil {
				dq.Defer(a.returner.msgReleaser.Decr)
			}
		}
	}
}

func (c *lockedConn) releaseQuestions(dq *deferred.Queue, questions []*question) {
	for _, q := range questions {
		canceled := q != nil && q.flags.Contains(finished)
		if !canceled {
			// Only reject the question if it isn't already flagged
			// as finished; otherwise it was rejected when the finished
			// flag was set.

			qr := q // Capture a different variable each time through the loop.
			dq.Defer(func() {
				qr.Reject(ExcClosed)
			})
		}
	}
}

// If abortErr != nil, send abort message.  IO and alloc errors are ignored.
// Called by 'shutdown'.  Callers MUST NOT hold c.lk.
func (c *Conn) abort(abortErr error) {
	// send abort message?
	if abortErr != nil {
		outMsg, err := c.transport.NewMessage()
		if err != nil {
			return
		}
		defer outMsg.Release()

		// configure & send abort message
		if abort, err := outMsg.Message().NewAbort(); err == nil {
			abort.SetType(rpccp.Exception_Type(exc.TypeOf(abortErr)))
			if err = abort.SetReason(abortErr.Error()); err == nil {
				outMsg.Send()
			}
		}
	}
}

func (c *Conn) send(ctx context.Context) func() error {
	return c.backgroundTask(func() error {
		for {
			async, err := c.sendRx.Recv(ctx)
			if err != nil {
				return err
			}

			async.Send()
		}
	})
}

// receive receives and dispatches messages coming from c.transport.  receive
// runs in a background goroutine.
//
// After receive returns, the connection is shut down.  If receive
// returns a non-nil error, it is sent to the remove vat as an abort.
func (c *Conn) receive(ctx context.Context) func() error {
	return c.backgroundTask(func() error {
		// We delegate actual IO to a separate goroutine, so that we
		// can always respond to context cancellation.
		incoming := make(chan incomingMessage)
		go c.reader(ctx, incoming)

		var in transport.IncomingMessage
		for {
			select {
			case inMsg := <-incoming:
				// reader error?
				if inMsg.err != nil {
					return fmt.Errorf("reader: %w", inMsg.err)
				}
				in = inMsg.IncomingMessage

			case <-ctx.Done():
				return nil
			}

			switch in.Message().Which() {
			case rpccp.Message_Which_unimplemented:
				if err := c.handleUnimplemented(in); err != nil {
					return fmt.Errorf("handle Unimplemented: %w", err)
				}

			case rpccp.Message_Which_abort:
				c.handleAbort(in)
				return nil

			case rpccp.Message_Which_bootstrap:
				if err := c.handleBootstrap(in); err != nil {
					return fmt.Errorf("handle Bootstrap: %w", err)
				}

			case rpccp.Message_Which_call:
				if err := c.handleCall(ctx, in); err != nil {
					return fmt.Errorf("handle Call: %w", err)
				}

			case rpccp.Message_Which_return:
				if err := c.handleReturn(ctx, in); err != nil {
					return fmt.Errorf("handle Return: %w", err)
				}

			case rpccp.Message_Which_finish:
				if err := c.handleFinish(ctx, in); err != nil {
					return fmt.Errorf("handle Finish: %w", err)
				}

			case rpccp.Message_Which_release:
				if err := c.handleRelease(ctx, in); err != nil {
					return fmt.Errorf("handle Release: %w", err)
				}

			case rpccp.Message_Which_disembargo:
				if err := c.handleDisembargo(ctx, in); err != nil {
					return fmt.Errorf("handle Disembargo: %w", err)
				}

			case rpccp.Message_Which_resolve:
				if err := c.handleResolve(ctx, in); err != nil {
					return fmt.Errorf("handle Resolve: %w", err)
				}

			case rpccp.Message_Which_accept, rpccp.Message_Which_provide:
				if c.network != nil {
					panic("TODO: 3PH")
				}
				fallthrough

			default:
				c.handleUnknownMessageType(ctx, in)
			}
		}
	})
}

func (c *Conn) handleAbort(in transport.IncomingMessage) {
	defer in.Release()

	e, err := in.Message().Abort()
	if err != nil {
		c.er.ReportError(exc.WrapError("read abort", err))
		return
	}

	reason, err := e.Reason()
	if err != nil {
		c.er.ReportError(exc.WrapError("read abort: reason", err))
		return
	}

	c.er.ReportError(exc.New(exc.Type(e.Type()), "rpc", "remote abort: "+reason))
}

func (c *Conn) handleUnimplemented(in transport.IncomingMessage) error {
	defer in.Release()

	msg, err := in.Message().Unimplemented()
	if err != nil {
		return exc.WrapError("read unimplemented", err)
	}
	if msg.Which() == rpccp.Message_Which_resolve {
		// If we get unimplemented for a resolve message, we should
		// release the reference we sent, since it won't be used.
		resolve, err := msg.Resolve()
		if err != nil {
			return exc.WrapError("read unimplemented.resolve", err)
		}
		if resolve.Which() == rpccp.Resolve_Which_cap {
			desc, err := resolve.Cap()
			if err != nil {
				return exc.WrapError("read unimplemented.resolve.cap", err)
			}
			var id exportID
			switch desc.Which() {
			case rpccp.CapDescriptor_Which_senderHosted:
				id = exportID(desc.SenderHosted())
			case rpccp.CapDescriptor_Which_senderPromise:
				id = exportID(desc.SenderPromise())
			default:
				return nil
			}
			dq := &deferred.Queue{}
			defer dq.Run()
			err = withLockedConn1(c, func(c *lockedConn) error {
				return c.releaseExport(dq, id, 1)
			})
			return err
		}
	}
	// For other cases we should just ignore the message.
	return nil
}

func (c *Conn) handleBootstrap(in transport.IncomingMessage) error {
	defer in.Release()

	bootstrap, err := in.Message().Bootstrap()
	if err != nil {
		c.er.ReportError(exc.WrapError("read bootstrap", err))
		return nil
	}

	dq := &deferred.Queue{}
	defer dq.Run()

	ans := ansent{
		returner: ansReturner{
			c:  c,
			id: answerID(bootstrap.QuestionId()),
		},
	}

	ans.returner.ret, ans.sendMsg, ans.returner.msgReleaser, err = c.newReturn()
	if err == nil {
		ans.returner.ret.SetAnswerId(uint32(ans.returner.id))
		ans.returner.ret.SetReleaseParamCaps(false)
	}

	c.withLocked(func(c *lockedConn) {
		if c.lk.answers[ans.returner.id] != nil {
			dq.Defer(ans.returner.msgReleaser.Decr)
			err = rpcerr.Failed(errors.New("incoming bootstrap: answer ID " + str.Utod(ans.returner.id) + " reused"))
			return
		}

		if err != nil {
			err = rpcerr.Annotate(err, "incoming bootstrap")
			c.lk.answers[ans.returner.id] = errorAnswer((*Conn)(c), ans.returner.id, err)
			c.er.ReportError(err)
			return
		}

		c.lk.answers[ans.returner.id] = &ans
		if !c.bootstrap.IsValid() {
			ans.sendException(dq, exc.New(exc.Failed, "", "vat does not expose a public/bootstrap interface"))
			return
		}
		if err := ans.returner.setBootstrap(c.bootstrap.AddRef()); err != nil {
			ans.sendException(dq, err)
			return
		}
		err = ans.sendReturn(dq)
		if err != nil {
			// Answer cannot possibly encounter a Finish, since we still
			// haven't returned to receive().
			panic(err)
		}
	})

	return err
}

func (c *Conn) handleCall(ctx context.Context, in transport.IncomingMessage) error {
	call, err := in.Message().Call()
	if err != nil {
		in.Release()
		c.er.ReportError(exc.WrapError("read call", err))
		return nil
	}

	dq := &deferred.Queue{}
	defer dq.Run()

	id := answerID(call.QuestionId())

	// TODO(3rd-party handshake): support sending results to 3rd party vat
	if call.SendResultsTo().Which() != rpccp.Call_sendResultsTo_Which_caller {
		// TODO(someday): handle SendResultsTo.yourself
		c.er.ReportError(errors.New("incoming call: results destination is not caller"))

		c.withLocked(func(c *lockedConn) {
			c.sendMessage(ctx, func(m rpccp.Message) error {
				defer in.Release()

				mm, err := m.NewUnimplemented()
				if err != nil {
					return rpcerr.Annotate(err, "incoming call: send unimplemented")
				}

				if err = mm.SetCall(call); err != nil {
					return rpcerr.Annotate(err, "incoming call: send unimplemented")
				}

				return nil
			}, func(err error) {
				if err != nil {
					c.er.ReportError(rpcerr.Annotate(err, "incoming call: send unimplemented"))
				}
			})
		})

		return nil
	}

	var (
		p        parsedCall
		parseErr error
	)
	c.withLocked(func(c *lockedConn) {
		if c.lk.answers[id] != nil {
			dq.Defer(in.Release)
			err = rpcerr.Failed(errors.New("incoming call: answer ID " + str.Utod(id) + "reused"))
			return
		}

		parseErr = c.parseCall(dq, &p, call) // parseCall sets CapTable
	})
	if err != nil {
		return err
	}

	// Create return message.
	ret, send, retReleaser, err := c.newReturn()
	if err != nil {
		err = rpcerr.Annotate(err, "incoming call")
		syncutil.With(&c.lk, func() {
			c.lk.answers[id] = errorAnswer(c, id, err)
		})
		c.er.ReportError(err)
		in.Release()
		return nil
	}
	ret.SetAnswerId(uint32(id))
	ret.SetReleaseParamCaps(false)

	// Find target and start call.
	ans := &ansent{
		returner: ansReturner{
			c:           c,
			id:          id,
			ret:         ret,
			msgReleaser: retReleaser,
		},
		sendMsg: send,
	}
	return withLockedConn1(c, func(c *lockedConn) error {
		c.lk.answers[id] = ans
		if parseErr != nil {
			parseErr = rpcerr.Annotate(parseErr, "incoming call")
			ans.sendException(dq, parseErr)
			dq.Defer(func() {
				c.er.ReportError(parseErr)
				in.Release()
			})
			return nil
		}

		recv := capnp.Recv{
			Args:        p.args,
			Method:      p.method,
			ReleaseArgs: util.Idempotent(in.Release),
			Returner:    &ans.returner,
		}

		switch p.target.which {
		case rpccp.MessageTarget_Which_importedCap:
			ent := c.findExport(p.target.importedCap)
			if ent == nil {
				ans.returner.ret = rpccp.Return{}
				ans.sendMsg = nil
				ans.returner.msgReleaser = nil
				dq.Defer(func() {
					retReleaser.Decr()
					in.Release()
				})
				return rpcerr.Failed(errors.New("incoming call: unknown export ID " + str.Utod(id)))
			}
			c.tasks.Add(1) // will be finished by answer.Return
			var callCtx context.Context
			callCtx, ans.cancel = context.WithCancel(c.bgctx)
			pcall := newPromisedPipelineCaller()
			ans.setPipelineCaller(p.method, pcall)
			dq.Defer(func() {
				pcall.resolve(ent.snapshot.Recv(callCtx, recv))
			})
			return nil
		case rpccp.MessageTarget_Which_promisedAnswer:
			tgtAns := c.lk.answers[p.target.promisedAnswer]
			if tgtAns == nil || tgtAns.flags.Contains(finishReceived) {
				ans.returner.ret = rpccp.Return{}
				ans.sendMsg = nil
				ans.returner.msgReleaser = nil
				dq.Defer(func() {
					retReleaser.Decr()
					in.Release()
				})
				return rpcerr.Failed(errors.New(
					"incoming call: use of unknown or finished answer ID " +
						str.Utod(p.target.promisedAnswer) +
						" for promised answer target",
				))
			}
			if tgtAns.flags.Contains(resultsReady) {
				if tgtAns.err != nil {
					ans.sendException(dq, tgtAns.err)
					dq.Defer(in.Release)
					return nil
				}
				// tgtAns.results is guaranteed to stay alive because it hasn't
				// received finish yet (it would have been deleted from the
				// answers table), and it can't receive a finish because this is
				// happening on the receive goroutine.
				content, err := tgtAns.returner.results.Content()
				if err != nil {
					err = rpcerr.WrapFailed("incoming call: read results from target answer", err)
					ans.sendException(dq, err)
					dq.Defer(in.Release)
					c.er.ReportError(err)
					return nil
				}
				sub, err := capnp.Transform(content, p.target.transform)
				if err != nil {
					// Not reporting, as this is the caller's fault.
					ans.sendException(dq, err)
					dq.Defer(in.Release)
					return nil
				}
				iface := sub.Interface()
				var tgt capnp.ClientSnapshot
				if sub.IsValid() && !iface.IsValid() {
					tgt = capnp.ErrorClient(rpcerr.Failed(ErrNotACapability)).Snapshot()
				} else {
					capID := iface.Capability()
					capTable := tgtAns.returner.resultsCapTable
					if int(capID) < len(capTable) {
						tgt = capTable[capID].AddRef()
					}
				}

				c.tasks.Add(1) // will be finished by answer.Return
				var callCtx context.Context
				callCtx, ans.cancel = context.WithCancel(c.bgctx)
				pcall := newPromisedPipelineCaller()
				ans.setPipelineCaller(p.method, pcall)
				dq.Defer(func() {
					defer tgt.Release()
					pcall.resolve(tgt.Recv(callCtx, recv))
				})
			} else {
				// Results not ready, use pipeline caller.
				tgtAns.pcalls.Add(1) // will be finished by answer.Return
				var callCtx context.Context
				callCtx, ans.cancel = context.WithCancel(c.bgctx)
				tgt := tgtAns.pcall
				c.tasks.Add(1) // will be finished by answer.Return
				pcall := newPromisedPipelineCaller()
				ans.setPipelineCaller(p.method, pcall)
				dq.Defer(func() {
					pcall.resolve(tgt.PipelineRecv(callCtx, p.target.transform, recv))
					tgtAns.pcalls.Done()
				})
			}
			return nil
		default:
			panic("unreachable")
		}
	})
}

// A promisedPipelineCaller is a PipelineCaller that stands in for another
// PipelineCaller that may not be available yet. Methods block until
// resolve() is called.
//
// NOTE WELL: This is meant to stand-in for a very short time, to avoid racy
// behavior between releasing locks and calling resolve(), so even the
// context on the recv/send methods is ignored until underlying caller is
// reserved.
type promisedPipelineCaller struct {
	ready      chan struct{}
	underlying capnp.PipelineCaller
}

func newPromisedPipelineCaller() *promisedPipelineCaller {
	return &promisedPipelineCaller{
		ready:      make(chan struct{}),
		underlying: nil,
	}
}

// resolve() resolves p to result.
func (p *promisedPipelineCaller) resolve(result capnp.PipelineCaller) {
	p.underlying = result
	close(p.ready)
}

func (p *promisedPipelineCaller) PipelineSend(
	ctx context.Context,
	transform []capnp.PipelineOp,
	s capnp.Send,
) (*capnp.Answer, capnp.ReleaseFunc) {
	<-p.ready
	return p.underlying.PipelineSend(ctx, transform, s)
}

func (p *promisedPipelineCaller) PipelineRecv(
	ctx context.Context,
	transform []capnp.PipelineOp,
	r capnp.Recv,
) capnp.PipelineCaller {
	<-p.ready
	return p.underlying.PipelineRecv(ctx, transform, r)
}

type parsedCall struct {
	target parsedMessageTarget
	method capnp.Method
	args   capnp.Struct
}

type parsedMessageTarget struct {
	which          rpccp.MessageTarget_Which
	importedCap    exportID
	promisedAnswer answerID
	transform      []capnp.PipelineOp
}

func (c *lockedConn) parseCall(dq *deferred.Queue, p *parsedCall, call rpccp.Call) error {
	p.method = capnp.Method{
		InterfaceID: call.InterfaceId(),
		MethodID:    call.MethodId(),
	}
	payload, err := call.Params()
	if err != nil {
		return rpcerr.WrapFailed("read params", err)
	}
	ptr, _, err := c.recvPayload(dq, payload)
	if err != nil {
		return rpcerr.Annotate(err, "read params")
	}
	p.args = ptr.Struct()
	tgt, err := call.Target()
	if err != nil {
		return rpcerr.WrapFailed("read target", err)
	}
	if err := parseMessageTarget(&p.target, tgt); err != nil {
		return err
	}
	return nil
}

func parseMessageTarget(pt *parsedMessageTarget, tgt rpccp.MessageTarget) error {
	switch pt.which = tgt.Which(); pt.which {
	case rpccp.MessageTarget_Which_importedCap:
		pt.importedCap = exportID(tgt.ImportedCap())
	case rpccp.MessageTarget_Which_promisedAnswer:
		pa, err := tgt.PromisedAnswer()
		if err != nil {
			return rpcerr.WrapFailed("read target answer", err)
		}
		pt.promisedAnswer = answerID(pa.QuestionId())
		opList, err := pa.Transform()
		if err != nil {
			return rpcerr.WrapFailed("read target transform", err)
		}
		pt.transform, err = parseTransform(opList)
		if err != nil {
			return rpcerr.Annotate(err, "read target transform")
		}
	default:
		return rpcerr.Unimplemented(errors.New("unknown message target " + pt.which.String()))
	}

	return nil
}

func parseTransform(list rpccp.PromisedAnswer_Op_List) ([]capnp.PipelineOp, error) {
	ops := make([]capnp.PipelineOp, 0, list.Len())
	for i := 0; i < list.Len(); i++ {
		li := list.At(i)
		switch li.Which() {
		case rpccp.PromisedAnswer_Op_Which_noop:
			// do nothing
		case rpccp.PromisedAnswer_Op_Which_getPointerField:
			ops = append(ops, capnp.PipelineOp{Field: li.GetPointerField()})
		default:
			return nil, rpcerr.Failed(errors.New(
				"transform element " + str.Itod(i) +
					": unknown type " + li.Which().String(),
			))
		}
	}
	return ops, nil
}

func (c *Conn) handleReturn(ctx context.Context, in transport.IncomingMessage) error {
	ret, err := in.Message().Return()
	if err != nil {
		in.Release()
		c.er.ReportError(exc.WrapError("read return", err))
		return nil
	}

	dq := &deferred.Queue{}
	defer dq.Run()

	// Save this, so we can reference it from spawned goroutines that are not holding
	// the lock; we reassign to c at the top of those functions.
	unlockedConn := c

	return withLockedConn1(c, func(c *lockedConn) error {

		qid := questionID(ret.AnswerId())
		if uint32(qid) >= uint32(len(c.lk.questions)) {
			dq.Defer(in.Release)
			return rpcerr.Failed(errors.New(
				"incoming return: question " + str.Utod(qid) + " does not exist",
			))
		}
		// Pop the question from the table.  Receiving the Return message
		// will always remove the question from the table, because it's the
		// only time the remote vat will use it.
		q := c.lk.questions[qid]
		c.lk.questions[qid] = nil
		if q == nil {
			dq.Defer(in.Release)
			return rpcerr.Failed(errors.New(
				"incoming return: question " + str.Utod(qid) + " does not exist",
			))
		}
		canceled := q.flags.Contains(finished)
		q.flags |= finished
		if canceled {
			// Wait for cancelation task to write the Finish message.  If the
			// Finish message could not be sent to the remote vat, we can't
			// reuse the ID.
			select {
			case <-q.finishMsgSend:
				if q.flags.Contains(finishSent) {
					c.lk.questionID.remove(qid)
				}
				dq.Defer(in.Release)
			default:
				dq.Defer(in.Release)

				go func() {
					c := unlockedConn
					<-q.finishMsgSend
					c.withLocked(func(c *lockedConn) {
						if q.flags.Contains(finishSent) {
							c.lk.questionID.remove(qid)
						}
					})
				}()
			}
			return nil
		}
		pr := c.parseReturn(dq, ret, q.called) // fills in CapTable
		if pr.parseFailed {
			c.er.ReportError(rpcerr.Annotate(pr.err, "incoming return"))
		}

		if pr.err == nil {
			// The result of the message contains actual data (not just
			// an error), so we save the ReleaseFunc for later:
			q.release = in.Release
		}
		// We're going to potentially block fulfilling some promises so fork
		// off a goroutine to avoid blocking the receive loop.
		go func() {
			c := unlockedConn
			q.p.Resolve(pr.result, pr.err)
			if pr.err != nil {
				// We can release now; the result is an error, so data from the message
				// won't be accessed:
				in.Release()
			}

			c.withLocked(func(c *lockedConn) {
				// Send disembargoes.  Failing to send one of these just never lifts
				// the embargo on our side, but doesn't cause a leak.
				//
				// TODO(soon): make embargo resolve to error client.
				for _, s := range pr.disembargoes {
					c.sendMessage(ctx, s.buildDisembargo, func(err error) {
						if err != nil {
							err = exc.WrapError("incoming return: send disembargo", err)
							c.er.ReportError(err)
						}
					})
				}

				// Send finish.
				c.sendMessage(ctx, func(m rpccp.Message) error {
					fin, err := m.NewFinish()
					if err == nil {
						fin.SetQuestionId(uint32(qid))
						fin.SetReleaseResultCaps(false)
					}
					return err
				}, func(err error) {
					c.lk.Lock()
					defer c.lk.Unlock()
					defer close(q.finishMsgSend)

					if err != nil {
						err = exc.WrapError("incoming return: send finish: build message", err)
						c.er.ReportError(err)
					} else {
						q.flags |= finishSent
						c.lk.questionID.remove(qid)
					}
				})
			})
		}()

		return nil
	})
}

func (c *lockedConn) parseReturn(dq *deferred.Queue, ret rpccp.Return, called [][]capnp.PipelineOp) parsedReturn {
	switch w := ret.Which(); w {
	case rpccp.Return_Which_results:
		r, err := ret.Results()
		if err != nil {
			return parsedReturn{err: rpcerr.WrapFailed("parse return", err), parseFailed: true}
		}
		content, locals, err := c.recvPayload(dq, r)
		if err != nil {
			return parsedReturn{err: rpcerr.WrapFailed("parse return", err), parseFailed: true}
		}

		var embargoCaps uintSet
		var disembargoes []senderLoopback
		mtab := ret.Message().CapTable()
		for _, xform := range called {
			p2, _ := capnp.Transform(content, xform)
			iface := p2.Interface()
			i := iface.Capability()
			if !mtab.Contains(iface) || !locals.has(uint(i)) || embargoCaps.has(uint(i)) {
				continue
			}

			id, ec := c.embargo(mtab.Get(iface))
			mtab.Set(i, ec)

			embargoCaps.add(uint(i))
			disembargoes = append(disembargoes, senderLoopback{
				id: id,
				target: parsedMessageTarget{
					which:          rpccp.MessageTarget_Which_promisedAnswer,
					promisedAnswer: answerID(ret.AnswerId()),
					transform:      xform,
				},
			})
		}
		return parsedReturn{
			result:       content,
			disembargoes: disembargoes,
		}
	case rpccp.Return_Which_exception:
		e, err := ret.Exception()
		if err != nil {
			return parsedReturn{err: rpcerr.WrapFailed("parse return", err), parseFailed: true}
		}
		reason, err := e.Reason()
		if err != nil {
			return parsedReturn{err: rpcerr.WrapFailed("parse return", err), parseFailed: true}
		}
		return parsedReturn{err: exc.New(exc.Type(e.Type()), "", reason)}
	case rpccp.Return_Which_acceptFromThirdParty:
		// TODO: 3PH. Can wait until after the MVP, because we can keep
		// setting allowThirdPartyTailCall = false
		fallthrough
	default:
		// TODO: go through other variants and make sure we're handling
		// them all correctly.
		return parsedReturn{err: rpcerr.Failed(errors.New(
			"parse return: unhandled type " + w.String(),
		)), parseFailed: true, unimplemented: true}
	}
}

type parsedReturn struct {
	result        capnp.Ptr
	disembargoes  []senderLoopback
	err           error
	parseFailed   bool
	unimplemented bool
}

func (c *Conn) handleFinish(ctx context.Context, in transport.IncomingMessage) error {
	defer in.Release()

	fin, err := in.Message().Finish()
	if err != nil {
		c.er.ReportError(exc.WrapError("read finish", err))
		return nil
	}

	id := answerID(fin.QuestionId())
	releaseResultCaps := fin.ReleaseResultCaps()

	dq := &deferred.Queue{}
	defer dq.Run()
	return withLockedConn1(c, func(c *lockedConn) error {
		ans := c.lk.answers[id]
		if ans == nil {
			return rpcerr.Failed(errors.New(
				"incoming finish: unknown answer ID " + str.Utod(id),
			))
		}
		if ans.flags.Contains(finishReceived) {
			return rpcerr.Failed(errors.New(
				"incoming finish: answer ID " + str.Utod(id) + " already received finish",
			))
		}
		ans.flags |= finishReceived
		if releaseResultCaps {
			ans.flags |= releaseResultCapsFlag
		}
		if ans.cancel != nil {
			ans.cancel()
		}
		if !ans.flags.Contains(returnSent) {
			return nil
		}

		// Return sent and finish received: time to destroy answer.
		err := ans.destroy(dq)
		if err != nil {
			return rpcerr.Annotate(err, "incoming finish: release result caps")
		}

		return nil
	})
}

// recvCap materializes a client for a given descriptor.  The caller is
// responsible for ensuring the client gets released.  Any returned
// error indicates a protocol violation.
//
// The caller must be holding onto c.lk.
func (c *lockedConn) recvCap(d rpccp.CapDescriptor) (capnp.Client, error) {
	switch w := d.Which(); w {
	case rpccp.CapDescriptor_Which_none:
		return capnp.Client{}, nil
	case rpccp.CapDescriptor_Which_senderHosted:
		id := importID(d.SenderHosted())
		return c.addImport(id, false), nil
	case rpccp.CapDescriptor_Which_senderPromise:
		id := importID(d.SenderPromise())
		return c.addImport(id, true), nil
	case rpccp.CapDescriptor_Which_thirdPartyHosted:
		if c.network == nil {
			// We can't do third-party handoff without a network, so instead of
			// trying to pick this up, use the vine and treat it the same as
			// senderHosted:
			thirdPartyDesc, err := d.ThirdPartyHosted()
			if err != nil {
				return capnp.Client{}, exc.WrapError(
					"reading ThridPartyCapDescriptor",
					err,
				)
			}
			id := importID(thirdPartyDesc.VineId())
			return c.addImport(id, false), nil
		}
		panic("TODO: 3PH")
	case rpccp.CapDescriptor_Which_receiverHosted:
		id := exportID(d.ReceiverHosted())
		ent := c.findExport(id)
		if ent == nil {
			return capnp.Client{}, rpcerr.Failed(errors.New(
				"receive capability: invalid export " + str.Utod(id),
			))
		}
		return ent.snapshot.Client(), nil
	case rpccp.CapDescriptor_Which_receiverAnswer:
		promisedAnswer, err := d.ReceiverAnswer()
		if err != nil {
			return capnp.Client{}, rpcerr.WrapFailed("receive capability: reading promised answer", err)
		}
		rawTransform, err := promisedAnswer.Transform()
		if err != nil {
			return capnp.Client{}, rpcerr.WrapFailed("receive capability: reading promised answer transform", err)
		}
		transform, err := parseTransform(rawTransform)
		if err != nil {
			return capnp.Client{}, rpcerr.WrapFailed("read target transform", err)
		}

		id := answerID(promisedAnswer.QuestionId())
		ans, ok := c.lk.answers[id]
		if !ok {
			return capnp.Client{}, rpcerr.Failed(errors.New(
				"receive capability: no such question id: " + str.Utod(id),
			))
		}

		return c.recvCapReceiverAnswer(ans, transform), nil
	default:
		return capnp.ErrorClient(rpcerr.Failed(errors.New(
			"unknown CapDescriptor type " + w.String(),
		))), nil
	}
}

// Helper for lockedConn.recvCap(); handles the receiverAnswer case.
func (c *lockedConn) recvCapReceiverAnswer(ans *ansent, transform []capnp.PipelineOp) capnp.Client {
	if ans.promise != nil {
		// Still unresolved.
		future := ans.promise.Answer().Future()
		for _, op := range transform {
			future = future.Field(op.Field, op.DefaultValue)
		}
		return future.Client().AddRef()
	}

	if ans.err != nil {
		return capnp.ErrorClient(ans.err)
	}

	ptr, err := ans.returner.results.Content()
	if err != nil {
		return capnp.ErrorClient(rpcerr.WrapFailed("except.Failed reading results", err))
	}
	ptr, err = capnp.Transform(ptr, transform)
	if err != nil {
		return capnp.ErrorClient(rpcerr.WrapFailed("Applying transform to results", err))
	}
	iface := ptr.Interface()
	if !iface.IsValid() {
		return capnp.ErrorClient(rpcerr.Failed(errors.New("Result is not a capability")))
	}

	return iface.Client().AddRef()
}

// Returns whether the client should be treated as local, for the purpose of
// embargoes.
func (c *lockedConn) isLocalClient(client capnp.Client) bool {
	if (client == capnp.Client{}) {
		return false
	}

	snapshot := client.Snapshot()
	defer snapshot.Release()
	bv := snapshot.Brand().Value

	if ic, ok := bv.(*importClient); ok {
		if ic.c == (*Conn)(c) {
			return false
		}
		if c.network == nil || c.network != ic.c.network {
			// Different connections on different networks. We must
			// be proxying it, so as far as this connection is
			// concerned, it lives on our side.
			return true
		}
		// Might have to do more refactoring re: what to do in this case;
		// just checking for embargo or not might not be sufficient:
		panic("TODO: 3PH")
	}

	if pc, ok := bv.(capnp.PipelineClient); ok {
		// Same logic re: proxying as with imports:
		if q, ok := c.getAnswerQuestion(pc.Answer()); ok {
			if q.c == (*Conn)(c) {
				return false
			}
			if c.network == nil || c.network != q.c.network {
				return true
			}
			panic("TODO: 3PH")
		}
	}

	if _, ok := bv.(error); ok {
		// Returned by capnp.ErrorClient. No need to treat this as
		// local; all methods will just return the error anyway,
		// so violating E-order will have no effect on the results.
		return false
	}

	return true
}

// recvPayload extracts the content pointer after populating the
// message's capability table.  It also returns the set of indices in
// the capability table that represent capabilities in the local vat.
//
// The caller must be holding onto c.lk.
func (c *lockedConn) recvPayload(dq *deferred.Queue, payload rpccp.Payload) (_ capnp.Ptr, locals uintSet, _ error) {
	if !payload.IsValid() {
		// null pointer; in this case we can treat the cap table as being empty
		// and just return.
		return capnp.Ptr{}, nil, nil
	}
	if payload.Message().CapTable().Len() != 0 {
		// RecvMessage likely violated its invariant.
		return capnp.Ptr{}, nil, rpcerr.WrapFailed("read payload", ErrCapTablePopulated)
	}
	p, err := payload.Content()
	if err != nil {
		return capnp.Ptr{}, nil, rpcerr.WrapFailed("read payload", err)
	}
	ptab, err := payload.CapTable()
	if err != nil {
		// Don't allow unreadable capability table to stop other results,
		// just present an empty capability table.
		c.er.ReportError(exc.WrapError("read payload: capability table", err))
		return p, nil, nil
	}

	mtab := payload.Message().CapTable()
	mtab.Reset()

	var cl capnp.Client
	for i := 0; i < ptab.Len(); i++ {
		if cl, err = c.recvCap(ptab.At(i)); err != nil {
			// It's not safe to release clients while holding the connection lock,
			// as this might trigger a deadlock.  Use the deferred.Queue instead.
			dq.Defer(cl.Release)
			for j := 0; j < i; j++ {
				dq.Defer(mtab.At(j).Release)
			}

			err = rpcerr.Annotate(err, "read payload: capability "+str.Itod(i))
			break
		}

		mtab.Add(cl)
		if c.isLocalClient(cl) {
			locals.add(uint(i))
		}
	}

	return p, locals, err
}

func (c *Conn) handleRelease(ctx context.Context, in transport.IncomingMessage) error {
	defer in.Release()

	rel, err := in.Message().Release()
	if err != nil {
		c.er.ReportError(exc.WrapError("read release", err))
		return nil
	}

	id := exportID(rel.Id())
	count := rel.ReferenceCount()

	dq := &deferred.Queue{}
	defer dq.Run()
	err = withLockedConn1(c, func(c *lockedConn) error {
		return c.releaseExport(dq, id, count)
	})
	if err != nil {
		return rpcerr.Annotate(err, "incoming release")
	}
	return nil
}

func (c *Conn) handleDisembargo(ctx context.Context, in transport.IncomingMessage) error {
	d, err := in.Message().Disembargo()
	if err != nil {
		in.Release()
		c.er.ReportError(exc.WrapError("read disembargo", err))
		return nil
	}

	dtarget, err := d.Target()
	if err != nil {
		in.Release()
		return rpcerr.WrapFailed("incoming disembargo: read target", err)
	}

	var tgt parsedMessageTarget
	if err := parseMessageTarget(&tgt, dtarget); err != nil {
		in.Release()
		return rpcerr.Annotate(err, "incoming disembargo")
	}

	switch d.Context().Which() {
	case rpccp.Disembargo_context_Which_receiverLoopback:
		defer in.Release()

		id := embargoID(d.Context().ReceiverLoopback())
		var e *embargo
		c.withLocked(func(c *lockedConn) {
			e = c.findEmbargo(id)
			if e != nil {
				// TODO(soon): verify target matches the right import.
				c.lk.embargoes[id] = nil
				c.lk.embargoID.remove(id)
			}
		})
		if e == nil {
			return rpcerr.Failed(errors.New(
				"incoming disembargo: received sender loopback for unknown ID " + str.Utod(id),
			))
		}
		e.lift()

	case rpccp.Disembargo_context_Which_senderLoopback:
		snapshot, err := withLockedConn2(c, func(c *lockedConn) (_ capnp.ClientSnapshot, err error) {
			switch tgt.which {
			case rpccp.MessageTarget_Which_promisedAnswer:
				return c.getAnswerSnapshot(
					tgt.promisedAnswer,
					tgt.transform,
				)
			case rpccp.MessageTarget_Which_importedCap:
				ent := c.findExport(tgt.importedCap)
				if ent == nil {
					err = rpcerr.Failed(errors.New("sender loopback: no such export: " +
						str.Utod(tgt.importedCap)))
					return
				}
				if !ent.snapshot.IsPromise() {
					err = rpcerr.Failed(errors.New(
						"sender loopback: target export " +
							str.Utod(tgt.importedCap) +
							" is not a promise"))
					return
				}

				if !ent.snapshot.IsResolved() {
					err = errors.New("target for receiver loopback is an unresolved promise")
					return
				}
				snapshot := ent.snapshot.AddRef()
				err = snapshot.Resolve1(context.Background())
				if err != nil {
					panic("error resolving snapshot: " + err.Error())
				}
				return snapshot, nil
			default:
				err = rpcerr.Failed(errors.New("incoming disembargo: sender loopback: target is not a promised answer"))
				return
			}
		})

		if err != nil {
			in.Release()
			return err
		}

		// FIXME: we're sending the the disembargo right a way, which I(zenhack)
		// *think* is fine, and definitely was before we actually did anything
		// with promises. But this is contingent on making sure that all of the
		// relevant ClientHook implementations queue up their call messages before
		// returning from .Recv(); if this invariant holds then this is fine
		// because anything ahead of it is aready on the wire. But we need to
		// actually check this invariant.
		id := d.Context().SenderLoopback()

		c.withLocked(func(c *lockedConn) {
			c.sendMessage(ctx, func(m rpccp.Message) error {
				d, err := m.NewDisembargo()
				if err != nil {
					return err
				}
				d.Context().SetReceiverLoopback(id)
				tgt, err := d.NewTarget()
				if err != nil {
					return err
				}

				brand := snapshot.Brand()
				if pc, ok := brand.Value.(capnp.PipelineClient); ok {
					if q, ok := c.getAnswerQuestion(pc.Answer()); ok {
						if q.c == (*Conn)(c) {
							pa, err := tgt.NewPromisedAnswer()
							if err != nil {
								return err
							}
							pa.SetQuestionId(uint32(q.id))
							pcTrans := pc.Transform()
							trans, err := pa.NewTransform(int32(len(pcTrans)))
							if err != nil {
								return err
							}
							for i, op := range pcTrans {
								trans.At(i).SetGetPointerField(op.Field)
							}
						}
						return nil
					}
				}

				imp, ok := brand.Value.(*importClient)
				if ok && imp.c == (*Conn)(c) {
					tgt.SetImportedCap(uint32(imp.id))
					return nil
				}
				return errors.New("target for receiver loopback does not point to the right connection")

			}, func(err error) {
				defer in.Release()
				defer snapshot.Release()

				if err != nil {
					c.er.ReportError(rpcerr.Annotate(err, "incoming disembargo: send receiver loopback"))
				}
			})
		})

	case rpccp.Disembargo_context_Which_accept, rpccp.Disembargo_context_Which_provide:
		if c.network != nil {
			panic("TODO: 3PH")
		}
		fallthrough
	default:
		c.er.ReportError(errors.New("incoming disembargo: context " + d.Context().Which().String() + " not implemented"))
		c.withLocked(func(c *lockedConn) {
			c.sendMessage(ctx, func(m rpccp.Message) (err error) {
				if m, err = m.NewUnimplemented(); err == nil {
					err = m.SetDisembargo(d)
				}

				return
			}, func(err error) {
				defer in.Release()
				if err != nil {
					c.er.ReportError(rpcerr.Annotate(err, "incoming disembargo: send unimplemented"))
				}
			})
		})
	}

	return nil
}

func (c *lockedConn) getAnswerSnapshot(
	id answerID,
	transform []capnp.PipelineOp,
) (_ capnp.ClientSnapshot, err error) {
	ans := c.lk.answers[id]
	if ans == nil {
		err = rpcerr.Failed(errors.New(
			"incoming disembargo: unknown answer ID " +
				str.Utod(id)))
		return
	}
	if !ans.flags.Contains(returnSent) {
		err = rpcerr.Failed(errors.New(
			"incoming disembargo: answer ID " +
				str.Utod(id) + " has not sent return",
		))
		return
	}

	if ans.err != nil {
		err = rpcerr.Failed(errors.New(
			"incoming disembargo: answer ID " +
				str.Utod(id) + " returned exception",
		))
		return
	}

	var content capnp.Ptr
	if content, err = ans.returner.results.Content(); err != nil {
		err = rpcerr.Failed(errors.New(
			"incoming disembargo: read answer ID " +
				str.Utod(id) + ": " + err.Error(),
		))
		return
	}

	var ptr capnp.Ptr
	if ptr, err = capnp.Transform(content, transform); err != nil {
		err = rpcerr.Failed(errors.New(
			"incoming disembargo: read answer ID " +
				str.Utod(id) + ": " + err.Error(),
		))
		return
	}

	iface := ptr.Interface()
	if !ans.returner.results.Message().CapTable().Contains(iface) {
		err = rpcerr.Failed(errors.New(
			"incoming disembargo: sender loopback requested on a capability that is not an import",
		))
		return
	}
	caps := ans.returner.resultsCapTable
	capID := iface.Capability()
	if int(capID) >= len(caps) {
		return capnp.ClientSnapshot{}, nil
	}

	return caps[capID].AddRef(), nil
}

func (c *Conn) handleResolve(ctx context.Context, in transport.IncomingMessage) error {
	dq := &deferred.Queue{}
	defer dq.Run()

	resolve, err := in.Message().Resolve()
	if err != nil {
		in.Release()
		c.er.ReportError(exc.WrapError("read resolve", err))
		return nil
	}

	promiseID := importID(resolve.PromiseId())
	err = withLockedConn1(c, func(c *lockedConn) error {
		imp, ok := c.lk.imports[promiseID]
		if !ok {
			return errors.New(
				"incoming resolve: no such import ID: " + str.Utod(promiseID),
			)
		}
		if imp.resolver == nil {
			return errors.New(
				"incoming resolve: import ID " +
					str.Utod(promiseID) +
					"is not a promise",
			)
		}
		switch resolve.Which() {
		case rpccp.Resolve_Which_cap:
			desc, err := resolve.Cap()
			if err != nil {
				return exc.WrapError("reading cap from resolve message", err)
			}
			client, err := c.recvCap(desc)
			if err != nil {
				return err
			}
			if c.isLocalClient(client) {
				var id embargoID
				id, client = c.embargo(client)
				disembargo := senderLoopback{
					id: id,
					target: parsedMessageTarget{
						which:       rpccp.MessageTarget_Which_importedCap,
						importedCap: exportID(promiseID),
					},
				}
				c.sendMessage(ctx, disembargo.buildDisembargo, func(err error) {
					if err != nil {
						c.er.ReportError(
							exc.WrapError(
								"incoming resolve: send disembargo",
								err,
							),
						)
					}
				})
			}
			dq.Defer(func() {
				imp.resolver.Fulfill(client)
				client.Release()
			})
		case rpccp.Resolve_Which_exception:
			ex, err := resolve.Exception()
			if err != nil {
				err = exc.WrapError("reading exception from resolve message", err)
			} else {
				err = ex.ToError()
			}
			dq.Defer(func() {
				imp.resolver.Reject(err)
			})
		}
		return nil
	})
	if err != nil {
		c.er.ReportError(err)
	}
	return err
}

func (c *Conn) handleUnknownMessageType(ctx context.Context, in transport.IncomingMessage) {
	err := errors.New("unknown message type " + in.Message().Which().String() + " from remote")
	c.er.ReportError(err)

	c.withLocked(func(c *lockedConn) {
		c.sendMessage(ctx, func(m rpccp.Message) error {
			defer in.Release()
			if err := m.SetUnimplemented(in.Message()); err != nil {
				return rpcerr.Annotate(err, "send unimplemented")
			}
			return nil
		}, nil)
	})
}

// startTask increments c.tasks if c is not shutting down.
// It returns whether c.tasks was incremented.
func (c *lockedConn) startTask() (ok bool) {
	if c.bgctx.Err() == nil {
		c.tasks.Add(1)
		ok = true
	}

	return
}

// sendMessage creates a new message on the transport, calls build to
// populate its fields, and enqueues it on the outbound queue.
// When build returns, the message MUST have a nil cap table.
//
// If onSent != nil, it will be called by the send gouroutine
// with the error value returned by the send operation.  If this
// error is nil, the message was successfully sent.
//
// onSent will be called without holding c.lk.  Callers of
// sendMessage MAY wish to reacquire the c.lk within the onSent.
func (c *lockedConn) sendMessage(ctx context.Context, build func(rpccp.Message) error, onSent func(error)) {
	outMsg, err := c.transport.NewMessage()
	send := outMsg.Send
	release := outMsg.Release

	// If errors happen when allocating or building the message, set up dummy send/release
	// functions so the error handling logic in onSent() runs as normal:
	if err != nil {
		release = func() {}
		send = func() error {
			return rpcerr.WrapFailed("create message", err)
		}
	} else if err = build(outMsg.Message()); err != nil {
		send = func() error {
			return rpcerr.WrapFailed("build message", err)
		}
	}

	oldSend := send
	send = func() error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return oldSend()
	}

	c.lk.sendTx.Send(asyncSend{
		release: release,
		send:    send,
		onSent:  onSent,
	})
}

// reader reads messages from the transport in a loop, and send them down the
// 'in' channel, until the context is canceled or an error occurs. The first
// time RecvMessage() returns an error, its results will still be sent on the
// channel.
func (c *Conn) reader(ctx context.Context, in chan<- incomingMessage) {
	for {
		inMsg, err := c.transport.RecvMessage()
		select {
		case in <- incomingMessage{IncomingMessage: inMsg, err: err}:
		case <-ctx.Done():
			err = ctx.Err()
		}
		if err != nil {
			return
		}
	}
}

// An incomingMessage bundles the reutrn values of Transport.RecvMessage.
type incomingMessage struct {
	transport.IncomingMessage
	err error
}

type asyncSend struct {
	send    func() error
	onSent  func(error)
	release capnp.ReleaseFunc
}

func (as asyncSend) Abort(err error) {
	defer as.release()

	if as.onSent != nil {
		as.onSent(rpcerr.Disconnected(err))
	}
}

func (as asyncSend) Send() {
	defer as.release()

	if err := as.send(); as.onSent != nil {
		if err != nil {
			err = rpcerr.WrapFailed("send message", err)
		}

		as.onSent(err)
	}
}
