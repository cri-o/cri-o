package capnp

import (
	"context"
	"errors"
)

// A Request is a method call to be sent. Create one with NewReqeust, and send it with
// Request.Send().
type Request struct {
	method          Method
	args            Struct
	client          Client
	releaseResponse ReleaseFunc
	future          *Future
}

// NewRequest creates a new request calling the specified method on the specified client.
// argsSize is the size of the arguments struct.
func NewRequest(client Client, method Method, argsSize ObjectSize) (*Request, error) {
	_, seg := NewMultiSegmentMessage(nil)
	args, err := NewStruct(seg, argsSize)
	if err != nil {
		return nil, err
	}
	return &Request{
		method: method,
		args:   args,
		client: client,
	}, nil
}

// Args returns the arguments struct for this request. The arguments must not
// be accessed after the request is sent.
func (r *Request) Args() Struct {
	return r.args
}

func (r *Request) getSend() Send {
	return Send{
		Method: r.method,
		PlaceArgs: func(args Struct) error {
			err := args.CopyFrom(r.args)
			r.releaseArgs()
			return err
		},
		ArgsSize: r.args.Size(),
	}
}

// Send sends the request, returning a future for its results.
func (r *Request) Send(ctx context.Context) *Future {
	if r.future != nil {
		return ErrorAnswer(r.method, errors.New("sent the same request twice")).Future()
	}

	ans, rel := r.client.SendCall(ctx, r.getSend())
	r.releaseResponse = rel
	r.future = ans.Future()
	return r.future
}

// SendStream is to send as Client.SendStreamCall is to Client.SendCall
func (r *Request) SendStream(ctx context.Context) error {
	if r.future != nil {
		return errors.New("sent the same request twice")
	}

	return r.client.SendStreamCall(ctx, r.getSend())
}

// Future returns a future for the requests results. Returns nil if
// called before the request is sent.
func (r *Request) Future() *Future {
	return r.future
}

// Release resources associated with the request. In particular:
//
// * Release the arguments if they have not yet been released.
// * If the request has been sent, wait for the result and release
//   the results.
func (r *Request) Release() {
	r.releaseArgs()
	rel := r.releaseResponse
	if rel != nil {
		r.releaseResponse = nil
		r.future = nil
		rel()
	}
}

func (r *Request) releaseArgs() {
	if r.args.IsValid() {
		return
	}
	msg := r.args.Message()
	r.args = Struct{}
	arena := msg.Arena
	msg.Reset(nil)
	arena.Release()
}
