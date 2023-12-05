package rpc

import (
	"context"
	"errors"

	"capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/internal/str"
	"capnproto.org/go/capnp/v3/internal/syncutil"
	rpccp "capnproto.org/go/capnp/v3/std/capnp/rpc"
)

// An importID is an index into the imports table.
type importID uint32

// impent is an entry in the import table.  All fields are protected by
// Conn.mu.
type impent struct {
	wc *capnp.WeakClient

	// wireRefs is the number of times that the importID has appeared in
	// messages received from the remote vat.  Used to populate the
	// Release.referenceCount field.
	wireRefs int

	// generation is a counter used to disambiguate the following
	// condition:
	//
	// 1) An import given to application code.
	// 2) A new reference to the import is received over the wire, while
	//    the application concurrently closes the import.
	//    importClient.Close is called, but the receive has the lock
	//    first.
	// 3) Conn.addImport attempts to return a weak client reference, but
	//    can't because it has been closed.  It creates a new client
	//    with a new importClient.
	// 4) importClient.Close attempts to remove the import from the import
	//    table.  This is the critical step: it needs to be informed that
	//    it should not do this because another client has been created.
	//    No release message should be sent.
	//
	// The generation counter solves this by amending steps 3 and 4.  When
	// a new importClient is created, generation is incremented.
	// When importClient.Close is called, then it must check that the
	// importClient's generation matches the entry's generation before
	// removing the entry from the table and sending a release message.
	generation uint64
}

// addImport returns a client that represents the given import,
// incrementing the number of references to this import from this vat.
// This is separate from the reference counting that capnp.Client does.
//
// The caller must be holding onto c.mu.
func (c *lockedConn) addImport(id importID) capnp.Client {
	if ent := c.lk.imports[id]; ent != nil {
		ent.wireRefs++
		client, ok := ent.wc.AddRef()
		if !ok {
			ent.generation++
			client = capnp.NewClient(&importClient{
				c:          (*Conn)(c),
				id:         id,
				generation: ent.generation,
			})
			ent.wc = client.WeakRef()
		}
		return client
	}
	client := capnp.NewClient(&importClient{
		c:  (*Conn)(c),
		id: id,
	})
	c.lk.imports[id] = &impent{
		wc:       client.WeakRef(),
		wireRefs: 1,
	}
	return client
}

// An importClient implements capnp.Client for a remote capability.
type importClient struct {
	c          *Conn
	id         importID
	generation uint64
}

func (ic *importClient) String() string {
	return "importClient{c: 0x" + str.PtrToHex(ic.c) + ", id: " + str.Utod(ic.id) + "}"
}

func (ic *importClient) Send(ctx context.Context, s capnp.Send) (*capnp.Answer, capnp.ReleaseFunc) {
	return withLockedConn2(ic.c, func(c *lockedConn) (*capnp.Answer, capnp.ReleaseFunc) {
		if !c.startTask() {
			return capnp.ErrorAnswer(s.Method, ExcClosed), func() {}
		}
		defer c.tasks.Done()
		ent := c.lk.imports[ic.id]
		if ent == nil || ic.generation != ent.generation {
			return capnp.ErrorAnswer(s.Method, rpcerr.Disconnected(errors.New("send on closed import"))), func() {}
		}
		q := c.newQuestion(s.Method)

		// Send call message.
		c.sendMessage(ctx, func(m rpccp.Message) error {
			return c.newImportCallMessage(m, ic.id, q.id, s)
		}, func(err error) {
			if err != nil {
				syncutil.With(&ic.c.lk, func() {
					ic.c.lk.questions[q.id] = nil
				})
				q.p.Reject(rpcerr.WrapFailed("send message", err))
				syncutil.With(&ic.c.lk, func() {
					ic.c.lk.questionID.remove(uint32(q.id))
				})
				return
			}

			q.c.tasks.Add(1)
			go func() {
				defer q.c.tasks.Done()
				q.handleCancel(ctx)
			}()
		})

		ans := q.p.Answer()
		return ans, func() {
			<-ans.Done()
			q.p.ReleaseClients()
			q.release()
		}
	})
}

// newImportCallMessage builds a Call message targeted to an import.
func (c *lockedConn) newImportCallMessage(msg rpccp.Message, imp importID, qid questionID, s capnp.Send) error {
	call, err := msg.NewCall()
	if err != nil {
		return rpcerr.WrapFailed("build call message", err)
	}
	call.SetQuestionId(uint32(qid))
	call.SetInterfaceId(s.Method.InterfaceID)
	call.SetMethodId(s.Method.MethodID)
	target, err := call.NewTarget()
	if err != nil {
		return rpcerr.WrapFailed("build call message", err)
	}
	target.SetImportedCap(uint32(imp))
	payload, err := call.NewParams()
	if err != nil {
		return rpcerr.WrapFailed("build call message", err)
	}
	args, err := capnp.NewStruct(payload.Segment(), s.ArgsSize)
	if err != nil {
		return rpcerr.WrapFailed("build call message", err)
	}
	if err := payload.SetContent(args.ToPtr()); err != nil {
		return rpcerr.WrapFailed("build call message", err)
	}

	if s.PlaceArgs == nil {
		return nil
	}
	if err := s.PlaceArgs(args); err != nil {
		return rpcerr.WrapFailed("place arguments", err)
	}
	// TODO(soon): save param refs
	_, err = c.fillPayloadCapTable(payload)
	if err != nil {
		return rpcerr.Annotate(err, "build call message")
	}
	return nil
}

func (ic *importClient) Recv(ctx context.Context, r capnp.Recv) capnp.PipelineCaller {
	ans, finish := ic.Send(ctx, capnp.Send{
		Method:   r.Method,
		ArgsSize: r.Args.Size(),
		PlaceArgs: func(s capnp.Struct) error {
			err := s.CopyFrom(r.Args)
			r.ReleaseArgs()
			return err
		},
	})
	r.ReleaseArgs()
	select {
	case <-ans.Done():
		returnAnswer(r.Returner, ans, finish)
		return nil
	default:
		go returnAnswer(r.Returner, ans, finish)
		return ans
	}
}

func returnAnswer(ret capnp.Returner, ans *capnp.Answer, finish func()) {
	defer finish()
	defer ret.ReleaseResults()
	result, err := ans.Struct()
	if err != nil {
		ret.PrepareReturn(err)
		ret.Return()
		return
	}
	recvResult, err := ret.AllocResults(result.Size())
	if err != nil {
		ret.PrepareReturn(err)
		ret.Return()
		return
	}
	if err := recvResult.CopyFrom(result); err != nil {
		ret.PrepareReturn(err)
		ret.Return()
		return
	}
	ret.PrepareReturn(nil)
	ret.Return()
}

func (ic *importClient) Brand() capnp.Brand {
	return capnp.Brand{Value: ic}
}

func (ic *importClient) Shutdown() {
	ic.c.withLocked(func(c *lockedConn) {
		if !c.startTask() {
			return
		}
		defer c.tasks.Done()

		ent := c.lk.imports[ic.id]
		if ic.generation != ent.generation {
			// A new reference was added concurrently with the Shutdown.  See
			// impent.generation documentation for an explanation.
			return
		}
		delete(ic.c.lk.imports, ic.id)
		c.sendMessage(c.bgctx, func(msg rpccp.Message) error {
			rel, err := msg.NewRelease()
			if err == nil {
				rel.SetId(uint32(ic.id))
				rel.SetReferenceCount(uint32(ent.wireRefs))
			}
			return err
		}, func(err error) {
			if err != nil {
				ic.c.er.ReportError(rpcerr.Annotate(err, "send release"))
			}
		})
	})
}
