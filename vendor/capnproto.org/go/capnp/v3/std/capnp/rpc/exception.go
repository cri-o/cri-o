package rpc

import "capnproto.org/go/capnp/v3/exc"

// MarshalError fills in the fields of e according to err. Returns a non-nil
// error if marshalling fails.
func (e Exception) MarshalError(err error) error {
	e.SetType(Exception_Type(exc.TypeOf(err)))
	return e.SetReason(err.Error())
}

// ToError converts the exception to an error. If accessing the reason field
// returns an error, the exception's type field will still be returned by
// exc.TypeOf, but the message will be replaced by something describing the
// read erorr.
func (e Exception) ToError() error {
	// TODO: rework this so that exc.Type and Exception_Type
	// are aliases somehow. For now we rely on the values being
	// identical:
	typ := exc.Type(e.Type())

	reason, err := e.Reason()
	if err != nil {
		return &exc.Exception{
			Type:   typ,
			Prefix: "failed to read reason",
			Cause:  err,
		}
	}
	return exc.New(exc.Type(e.Type()), "", reason)
}
