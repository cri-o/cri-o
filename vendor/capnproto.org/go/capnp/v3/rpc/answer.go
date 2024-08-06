package rpc

import (
	"context"
	"errors"
	"sync"

	"capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/exc"
	"capnproto.org/go/capnp/v3/internal/rc"
	rpccp "capnproto.org/go/capnp/v3/std/capnp/rpc"
	"capnproto.org/go/capnp/v3/util/deferred"
)

// An answerID is an index into the answers table.
type answerID uint32

// ansent is an entry in a Conn's answer table.
type ansent struct {
	// flags is a bitmask of events that have occurred in an answer's
	// lifetime.
	flags answerFlags

	// exportRefs is the number of references to exports placed in the
	// results.
	exportRefs map[exportID]uint32

	// pcall is the PipelineCaller returned by RecvCall.  It will be set
	// to nil once results are ready.
	pcall capnp.PipelineCaller
	// promise is a promise wrapping pcall. It will be resolved, and
	// subsequently set to nil, once the results are ready.
	promise *capnp.Promise

	// pcalls is added to for every pending RecvCall and subtracted from
	// for every RecvCall return (delivery acknowledgement).  This is used
	// to satisfy the Returner.Return contract.
	pcalls sync.WaitGroup

	// err is the error passed to (*answer).sendException or from creating
	// the Return message.  Can only be read after resultsReady is set in
	// flags.
	err error

	// sendMsg sends the return message.
	sendMsg func()

	// cancel cancels the Context used in the received method call.
	// May be nil.
	cancel context.CancelFunc

	// Unlike other fields in this struct, it is ok to hand out pointers
	// to this that can be used while not holding the connection lock.
	returner ansReturner
}

// Returns the already-locked connection to which this entry belongs.
// Since ansents are only supposed to be accessed through c.lk, it is
// assumed that the caller already holds the lock.
func (ans *ansent) lockedConn() *lockedConn {
	return (*lockedConn)(ans.returner.c)
}

// ansReturner is the implementation of capnp.Returner that is used when
// handling an incoming call on a local capability.
type ansReturner struct {
	// c and id must be set before any answer methods are called.
	c  *Conn
	id answerID

	// ret is the outgoing Return struct.  ret is valid iff there was no
	// error creating the message.  If ret is invalid, then this answer
	// entry is a placeholder until the remote vat cancels the call.
	ret rpccp.Return

	// msgReleaser releases the return message when its refcount hits zero.
	// The caller MUST NOT hold ans.c.lk.
	msgReleaser *rc.Releaser

	// results is the memoized answer to ret.Results().
	// Set by AllocResults and setBootstrap, but contents can only be read
	// if flags has resultsReady but not finishReceived set.
	results rpccp.Payload

	// Snapshots of the clients results's capTable, at the time of PrepareReturn().
	// Pipelined calls are invoked on the snapshots, rather than the clients,
	// to respect the invariant regarding the 4-way race condition that table
	// entries must not path-shorten.
	resultsCapTable []capnp.ClientSnapshot
}

type answerFlags uint8

const (
	returnSent answerFlags = 1 << iota
	finishReceived
	resultsReady
	releaseResultCapsFlag
)

// flags.Contains(flag) Returns true iff flags contains flag, which must
// be a single flag.
func (flags answerFlags) Contains(flag answerFlags) bool {
	return flags&flag != 0
}

// errorAnswer returns a placeholder answer entry with an error result already set.
func errorAnswer(c *Conn, id answerID, err error) *ansent {
	return &ansent{
		returner: ansReturner{
			c:  c,
			id: id,
		},
		err:   err,
		flags: resultsReady | returnSent,
	}
}

// newReturn creates a new Return message. The returned Releaser will release the message when
// all references to it are dropped; the caller is responsible for one reference. This will not
// happen before the message is sent, as the returned send function retains a reference.
func (c *Conn) newReturn() (_ rpccp.Return, sendMsg func(), _ *rc.Releaser, _ error) {
	outMsg, err := c.transport.NewMessage()
	if err != nil {
		return rpccp.Return{}, nil, nil, rpcerr.WrapFailed("create return", err)
	}
	ret, err := outMsg.Message().NewReturn()
	if err != nil {
		outMsg.Release()
		return rpccp.Return{}, nil, nil, rpcerr.WrapFailed("create return", err)
	}

	// Before releasing the message, we need to wait both until it is sent and
	// until the local vat is done with it.  We therefore implement a simple
	// ref-counting mechanism such that 'release' must be called twice before
	// 'releaseMsg' is called.
	releaser := rc.NewReleaser(2, outMsg.Release)

	return ret, func() {
		c.lk.sendTx.Send(asyncSend{
			send:    outMsg.Send,
			release: releaser.Decr,
			onSent: func(err error) {
				if err != nil {
					c.er.ReportError(exc.WrapError("send return", err))
				}
			},
		})
	}, releaser, nil
}

// setPipelineCaller sets ans.pcall to pcall if the answer has not
// already returned.  The caller MUST hold ans.c.lk.
//
// This also sets ans.promise to a new promise, wrapping pcall.
func (ans *ansent) setPipelineCaller(m capnp.Method, pcall capnp.PipelineCaller) {
	if !ans.flags.Contains(resultsReady) {
		ans.pcall = pcall
		ans.promise = capnp.NewPromise(m, pcall, nil)
	}
}

// AllocResults allocates the results struct.
func (ans *ansReturner) AllocResults(sz capnp.ObjectSize) (capnp.Struct, error) {
	var err error
	ans.results, err = ans.ret.NewResults()
	if err != nil {
		return capnp.Struct{}, rpcerr.WrapFailed("alloc results", err)
	}
	s, err := capnp.NewStruct(ans.results.Segment(), sz)
	if err != nil {
		return capnp.Struct{}, rpcerr.WrapFailed("alloc results", err)
	}
	if err := ans.results.SetContent(s.ToPtr()); err != nil {
		return capnp.Struct{}, rpcerr.WrapFailed("alloc results", err)
	}
	ans.msgReleaser.Incr()
	return s, nil
}

// setBootstrap sets the results to an interface pointer, stealing the
// reference.
func (ans *ansReturner) setBootstrap(c capnp.Client) error {
	if ans.ret.HasResults() || ans.ret.Message().CapTable().Len() > 0 {
		panic("setBootstrap called after creating results")
	}
	// Add the capability to the table early to avoid leaks if setBootstrap fails.
	ans.ret.Message().CapTable().Reset(c)

	var err error
	ans.results, err = ans.ret.NewResults()
	if err != nil {
		return rpcerr.WrapFailed("alloc bootstrap results", err)
	}
	iface := capnp.NewInterface(ans.results.Segment(), 0)
	if err := ans.results.SetContent(iface.ToPtr()); err != nil {
		return rpcerr.WrapFailed("alloc bootstrap results", err)
	}
	return nil
}

// PrepareReturn implements capnp.Returner.PrepareReturn
func (ans *ansReturner) PrepareReturn(e error) {
	dq := &deferred.Queue{}
	defer dq.Run()

	ans.c.withLocked(func(c *lockedConn) {
		ent := c.lk.answers[ans.id]
		if e == nil {
			ent.prepareSendReturn(dq)
		} else {
			ent.prepareSendException(dq, e)
		}
	})
}

// Return implements capnp.Returner.Return
func (ans *ansReturner) Return() {
	dq := &deferred.Queue{}
	defer dq.Run()
	pcallsWait := func() {}

	var err error
	ans.c.withLocked(func(c *lockedConn) {
		ent := c.lk.answers[ans.id]
		pcallsWait = ent.pcalls.Wait

		if ent.err == nil {
			err = ent.completeSendReturn(dq)
		} else {
			ent.completeSendException(dq)
		}
	})
	defer pcallsWait()
	ans.c.tasks.Done() // added by handleCall

	if err == nil {
		return
	}

	if err = ans.c.shutdown(err); err != nil {
		ans.c.er.ReportError(err)
	}
}

func (ans *ansReturner) ReleaseResults() {
	if ans.results.IsValid() {
		ans.msgReleaser.Decr()
	}
}

// sendReturn sends the return message with results allocated by a
// previous call to AllocResults.  If the answer already received a
// Finish with releaseResultCaps set to true, then sendReturn returns
// the number of references to be subtracted from each export.
//
// The caller MUST be holding onto ans.c.lk, and the lockedConn parameter
// must be equal to ans.c (it exists to make it hard to forget to acquire
// the lock, per usual).
//
// sendReturn MUST NOT be called if sendException was previously called.
func (ans *ansent) sendReturn(dq *deferred.Queue) error {
	ans.prepareSendReturn(dq)
	return ans.completeSendReturn(dq)
}

func (ans *ansent) prepareSendReturn(dq *deferred.Queue) {
	var err error
	c := ans.lockedConn()
	ans.exportRefs, err = c.fillPayloadCapTable(ans.returner.results)
	if err != nil {
		c.er.ReportError(rpcerr.Annotate(err, "send return"))
	}
	// Continue.  Don't fail to send return if cap table isn't fully filled.
	results := ans.returner.results
	if results.IsValid() {
		capTable := results.Message().CapTable()
		snapshots := make([]capnp.ClientSnapshot, capTable.Len())
		for i := range snapshots {
			snapshots[i] = capTable.At(i).Snapshot()
		}
		ans.returner.resultsCapTable = snapshots
	}

	select {
	case <-c.bgctx.Done():
		// We're not going to send the message after all, so don't forget to release it.
		dq.Defer(ans.returner.msgReleaser.Decr)
		ans.sendMsg = nil
	default:
	}
}

func (ans *ansent) completeSendReturn(dq *deferred.Queue) error {
	ans.pcall = nil
	ans.flags |= resultsReady

	fin := ans.flags.Contains(finishReceived)
	if ans.sendMsg != nil {
		if ans.promise != nil {
			if fin {
				// Can't use ans.result after a finish, but it's
				// ok to return an error if the finish comes in
				// before the return. Possible enhancement: use
				// the cancel variant of return.
				ans.promise.Reject(rpcerr.Failed(errors.New("received finish before return")))
			} else {
				ans.promise.Resolve(ans.returner.results.Content())
			}
			dq.Defer(ans.promise.ReleaseClients)
			ans.promise = nil
		}
		ans.sendMsg()
	}

	ans.flags |= returnSent
	if fin {
		return ans.destroy(dq)
	}
	return nil
}

// sendException sends an exception on the answer's return message.
//
// The caller MUST be holding onto ans.c.lk. sendException MUST NOT
// be called if sendReturn was previously called.
func (ans *ansent) sendException(dq *deferred.Queue, ex error) {
	ans.prepareSendException(dq, ex)
	ans.completeSendException(dq)
}

func (ans *ansent) prepareSendException(dq *deferred.Queue, ex error) {
	ans.err = ex

	c := ans.lockedConn()
	select {
	case <-c.bgctx.Done():
	default:
		// Send exception.
		if e, err := ans.returner.ret.NewException(); err != nil {
			c.er.ReportError(exc.WrapError("send exception", err))
			ans.sendMsg = nil
		} else if err := e.MarshalError(ex); err != nil {
			c.er.ReportError(exc.WrapError("send exception", err))
			ans.sendMsg = nil
		}
	}
}

func (ans *ansent) completeSendException(dq *deferred.Queue) {
	ex := ans.err
	ans.pcall = nil
	ans.flags |= resultsReady

	if ans.promise != nil {
		ans.promise.Reject(ex)
		dq.Defer(ans.promise.ReleaseClients)
		ans.promise = nil
	}
	if ans.sendMsg != nil {
		ans.sendMsg()
	}
	ans.flags |= returnSent
	if ans.flags.Contains(finishReceived) {
		// destroy will never return an error because sendException does
		// create any exports.
		_ = ans.destroy(dq)
	}
}

// destroy removes the answer from the table and schedule ReleaseFuncs to
// run using dq. The answer must have sent a return and received a finish.
//
// shutdown has its own strategy for cleaning up an answer.
func (ans *ansent) destroy(dq *deferred.Queue) error {
	dq.Defer(ans.returner.msgReleaser.Decr)
	c := ans.lockedConn()
	delete(c.lk.answers, ans.returner.id)
	for _, s := range ans.returner.resultsCapTable {
		dq.Defer(s.Release)
	}
	if !ans.flags.Contains(releaseResultCapsFlag) || len(ans.exportRefs) == 0 {
		return nil

	}
	return c.releaseExportRefs(dq, ans.exportRefs)
}
