package rpc

import (
	"context"
	"errors"

	"capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/exc"
	"capnproto.org/go/capnp/v3/internal/str"
	"capnproto.org/go/capnp/v3/internal/syncutil"
	rpccp "capnproto.org/go/capnp/v3/std/capnp/rpc"
	"capnproto.org/go/capnp/v3/util/deferred"
)

// An exportID is an index into the exports table.
type exportID uint32

// expent is an entry in a Conn's export table.
type expent struct {
	snapshot capnp.ClientSnapshot
	wireRefs uint32

	// Should be called when removing this entry from the exports table:
	cancel context.CancelFunc
}

// A key for use in a client's Metadata, whose value is the export
// id (if any) via which the client is exposed on the given
// connection.
type exportIDKey struct {
	Conn *Conn
}

func (c *lockedConn) findExportID(m *capnp.Metadata) (_ exportID, ok bool) {
	maybeID, ok := m.Get(exportIDKey{(*Conn)(c)})
	if ok {
		return maybeID.(exportID), true
	}
	return 0, false
}

func (c *lockedConn) setExportID(m *capnp.Metadata, id exportID) {
	m.Put(exportIDKey{(*Conn)(c)}, id)
}

func (c *lockedConn) clearExportID(m *capnp.Metadata) {
	m.Delete(exportIDKey{(*Conn)(c)})
}

// findExport returns the export entry with the given ID or nil if
// couldn't be found. The caller must be holding c.mu
func (c *lockedConn) findExport(id exportID) *expent {
	if int64(id) >= int64(len(c.lk.exports)) {
		return nil
	}
	return c.lk.exports[id] // might be nil
}

// releaseExport decreases the number of wire references to an export
// by a given number.  If the export's reference count reaches zero,
// then releaseExport will pop export from the table and schedule further
// cleanup (such as releasing snaphost) via dq.
func (c *lockedConn) releaseExport(dq *deferred.Queue, id exportID, count uint32) error {
	ent := c.findExport(id)
	if ent == nil {
		return rpcerr.Failed(errors.New("unknown export ID " + str.Utod(id)))
	}
	switch {
	case count == ent.wireRefs:
		defer ent.cancel()
		snapshot := ent.snapshot
		c.lk.exports[id] = nil
		c.lk.exportID.remove(id)
		metadata := snapshot.Metadata()
		if metadata != nil {
			syncutil.With(metadata, func() {
				c.clearExportID(metadata)
			})
		}
		dq.Defer(snapshot.Release)
		return nil
	case count > ent.wireRefs:
		return rpcerr.Failed(errors.New("export ID " + str.Utod(id) + " released too many references"))
	default:
		ent.wireRefs -= count
		return nil
	}
}

func (c *lockedConn) releaseExportRefs(dq *deferred.Queue, refs map[exportID]uint32) error {
	var firstErr error
	for id, count := range refs {
		err := c.releaseExport(dq, id, count)
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// sendCap writes a capability descriptor, returning an export ID if
// this vat is hosting the capability. Steals the snapshot.
func (c *lockedConn) sendCap(d rpccp.CapDescriptor, snapshot capnp.ClientSnapshot) (_ exportID, isExport bool, _ error) {
	if !snapshot.IsValid() {
		d.SetNone()
		return 0, false, nil
	}

	defer snapshot.Release()
	bv := snapshot.Brand().Value
	if ic, ok := bv.(*importClient); ok {
		if ic.c == (*Conn)(c) {
			if ent := c.lk.imports[ic.id]; ent != nil && ent.generation == ic.generation {
				d.SetReceiverHosted(uint32(ic.id))
				return 0, false, nil
			}
		}
		if c.network != nil && c.network == ic.c.network {
			panic("TODO: 3PH")
		}
	}

	if pc, ok := bv.(capnp.PipelineClient); ok {
		if q, ok := c.getAnswerQuestion(pc.Answer()); ok {
			if q.c == (*Conn)(c) {
				pcTrans := pc.Transform()
				pa, err := d.NewReceiverAnswer()
				if err != nil {
					return 0, false, err
				}
				trans, err := pa.NewTransform(int32(len(pcTrans)))
				if err != nil {
					return 0, false, err
				}
				for i, op := range pcTrans {
					trans.At(i).SetGetPointerField(op.Field)
				}
				pa.SetQuestionId(uint32(q.id))
				return 0, false, nil
			}
			if c.network != nil && c.network == q.c.network {
				panic("TODO: 3PH")
			}
		}
	}

	// Default to export.
	metadata := snapshot.Metadata()
	metadata.Lock()
	defer metadata.Unlock()
	id, ok := c.findExportID(metadata)
	var ee *expent
	if ok {
		ee = c.lk.exports[id]
		ee.wireRefs++
	} else {
		// Not already present; allocate an export id for it:
		ee = &expent{
			snapshot: snapshot.AddRef(),
			wireRefs: 1,
			cancel:   func() {},
		}
		id = c.lk.exportID.next()
		if int64(id) == int64(len(c.lk.exports)) {
			c.lk.exports = append(c.lk.exports, ee)
		} else {
			c.lk.exports[id] = ee
		}
		c.setExportID(metadata, id)
	}
	if ee.snapshot.IsPromise() {
		c.sendSenderPromise(id, d)
	} else {
		d.SetSenderHosted(uint32(id))
	}
	return id, true, nil
}

// sendSenderPromise is a helper for sendCap that handles the senderPromise case.
func (c *lockedConn) sendSenderPromise(id exportID, d rpccp.CapDescriptor) {
	// Send a promise, wait for the resolution asynchronously, then send
	// a resolve message:
	ee := c.lk.exports[id]
	d.SetSenderPromise(uint32(id))
	ctx, cancel := context.WithCancel(c.bgctx)
	ee.cancel = cancel
	waitRef := ee.snapshot.AddRef()
	go func() {
		defer cancel()
		defer waitRef.Release()
		// Logically we don't hold the lock anymore; it's held by the
		// goroutine that spawned this one. So cast back to an unlocked
		// Conn before trying to use it again:
		unlockedConn := (*Conn)(c)

		waitErr := waitRef.Resolve1(ctx)
		unlockedConn.withLocked(func(c *lockedConn) {
			if len(c.lk.exports) <= int(id) || c.lk.exports[id] != ee {
				// Export was removed from the table at some point;
				// remote peer is uninterested in the resolution, so
				// drop the reference and we're done
				return
			}

			sendRef := waitRef.AddRef()
			var (
				resolvedID exportID
				isExport   bool
			)
			c.sendMessage(c.bgctx, func(m rpccp.Message) error {
				res, err := m.NewResolve()
				if err != nil {
					return err
				}
				res.SetPromiseId(uint32(id))
				if waitErr != nil {
					ex, err := res.NewException()
					if err != nil {
						return err
					}
					return ex.MarshalError(waitErr)
				}
				desc, err := res.NewCap()
				if err != nil {
					return err
				}
				resolvedID, isExport, err = c.sendCap(desc, sendRef)
				return err
			}, func(err error) {
				sendRef.Release()
				if err != nil && isExport {
					dq := &deferred.Queue{}
					defer dq.Run()
					// release 1 ref of the thing it resolved to.
					err := withLockedConn1(
						unlockedConn,
						func(c *lockedConn) error {
							return c.releaseExport(dq, resolvedID, 1)
						},
					)
					if err != nil {
						c.er.ReportError(
							exc.WrapError("releasing export due to failure to send resolve", err),
						)
					}
				}
			})
		})
	}()
}

// fillPayloadCapTable adds descriptors of payload's message's
// capabilities into payload's capability table and returns the
// reference counts that have been added to the exports table.
func (c *lockedConn) fillPayloadCapTable(payload rpccp.Payload) (map[exportID]uint32, error) {
	if !payload.IsValid() {
		return nil, nil
	}
	clients := payload.Message().CapTable()
	if clients.Len() == 0 {
		return nil, nil
	}
	list, err := payload.NewCapTable(int32(clients.Len()))
	if err != nil {
		return nil, rpcerr.WrapFailed("payload capability table", err)
	}
	var refs map[exportID]uint32
	for i := 0; i < clients.Len(); i++ {
		id, isExport, err := c.sendCap(list.At(i), clients.At(i).Snapshot())
		if err != nil {
			return nil, rpcerr.WrapFailed("Serializing capability", err)
		}
		if isExport {
			if refs == nil {
				refs = make(map[exportID]uint32, clients.Len()-i)
			}
			refs[id]++
		}
	}
	return refs, nil
}

type embargoID uint32

type embargo struct {
	result capnp.Ptr
	q      *capnp.AnswerQueue
}

func (e embargo) String() string {
	return "embargo{client: " +
		e.client().String() +
		", q: 0x" + str.PtrToHex(e.q) +
		"}"
}

// embargo creates a new embargoed client, stealing the reference.
//
// The caller must be holding onto c.mu.
func (c *lockedConn) embargo(client capnp.Client) (embargoID, capnp.Client) {
	id := c.lk.embargoID.next()
	e := newEmbargo(client)
	if int64(id) == int64(len(c.lk.embargoes)) {
		c.lk.embargoes = append(c.lk.embargoes, e)
	} else {
		c.lk.embargoes[id] = e
	}
	return id, capnp.NewClient(e)
}

// findEmbargo returns the embargo entry with the given ID or nil if
// couldn't be found.
func (c *lockedConn) findEmbargo(id embargoID) *embargo {
	if int64(id) >= int64(len(c.lk.embargoes)) {
		return nil
	}
	return c.lk.embargoes[id] // might be nil
}

func newEmbargo(client capnp.Client) *embargo {
	msg, seg := capnp.NewSingleSegmentMessage(nil)
	capID := msg.CapTable().Add(client)
	iface := capnp.NewInterface(seg, capID)
	return &embargo{
		result: iface.ToPtr(),
		q:      capnp.NewAnswerQueue(capnp.Method{}),
	}
}

// lift disembargoes the client.  It must be called only once.
func (e *embargo) lift() {
	e.q.Fulfill(e.result)
}

func (e *embargo) Send(ctx context.Context, s capnp.Send) (*capnp.Answer, capnp.ReleaseFunc) {
	return e.q.PipelineSend(ctx, nil, s)
}

func (e *embargo) Recv(ctx context.Context, r capnp.Recv) capnp.PipelineCaller {

	return e.q.PipelineRecv(ctx, nil, r)
}

func (e *embargo) Brand() capnp.Brand {
	return capnp.Brand{}
}

func (e *embargo) client() capnp.Client {
	return e.result.Interface().Client()
}

func (e *embargo) Shutdown() {
	e.client().Release()
}

// senderLoopback holds the salient information for a sender-loopback
// Disembargo message.
type senderLoopback struct {
	id     embargoID
	target parsedMessageTarget
}

func (sl *senderLoopback) buildDisembargo(msg rpccp.Message) error {
	d, err := msg.NewDisembargo()
	if err != nil {
		return rpcerr.WrapFailed("build disembargo", err)
	}
	d.Context().SetSenderLoopback(uint32(sl.id))
	tgt, err := d.NewTarget()
	if err != nil {
		return rpcerr.WrapFailed("build disembargo", err)
	}
	switch sl.target.which {
	case rpccp.MessageTarget_Which_promisedAnswer:
		pa, err := tgt.NewPromisedAnswer()
		if err != nil {
			return rpcerr.WrapFailed("build disembargo", err)
		}
		oplist, err := pa.NewTransform(int32(len(sl.target.transform)))
		if err != nil {
			return rpcerr.WrapFailed("build disembargo", err)
		}

		pa.SetQuestionId(uint32(sl.target.promisedAnswer))
		for i, op := range sl.target.transform {
			oplist.At(i).SetGetPointerField(op.Field)
		}
	case rpccp.MessageTarget_Which_importedCap:
		tgt.SetImportedCap(uint32(sl.target.importedCap))
	default:
		return errors.New("unknown variant for MessageTarget: " + str.Utod(sl.target.which))
	}
	return nil
}
