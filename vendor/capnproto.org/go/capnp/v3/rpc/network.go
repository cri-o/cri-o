package rpc

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
)

// A PeerID identifies a peer on a Cap'n Proto network. The exact
// format of this is network specific.
type PeerID struct {
	// Network specific value identifying the peer.
	Value any
}

// The information needed to connect to a third party and accept a capability
// from it.
//
// In a network where each vat has a public/private key pair, this could be a
// combination of the third party's public key fingerprint, hints on how to
// connect to the third party (e.g. an IP address), and the nonce used in the
// corresponding `Provide` message's `RecipientId` as sent to that third party
// (used to identify which capability to pick up).
//
// As another example, when communicating between processes on the same machine
// over Unix sockets, ThirdPartyCapId could simply refer to a file descriptor
// attached to the message via SCM_RIGHTS.  This file descriptor would be one
// end of a newly-created socketpair, with the other end having been sent to the
// process hosting the capability in RecipientId.
//
// Some networks, as an optimization, may permit ThirdPartyToContact to be
// forwarded across multiple vats. For example, imagine Alice sends a capability
// to Bob, who passes it along to Carol, who further pass it to Dave. Bob will send
// a `Provide` message to Alice telling her to expect the capability to be picked
// up by Carol, and then will pass Carol a `ThirdPartyToContact` pointing to Alice.
// If `ThirdPartyToContact` is non-forwardable, then Carol must form a connection
// to Alice, send an `Accept` to receive the capability, and then immediately send
// a `Provide` to provide it to Dave, before then being able to give a
// `ThirdPartyToContact` to Dave which points to Alice. This is a bit of a waste.
// If `ThirdPartyToContact` is forwardable, then Carol can simply pass it along to
// Dave without making any connection to Alice. Some VatNetwork implementations may
// require that Carol add a signature to the `ThirdPartyToContact` authenticating
// that she really did forward it to Dave, which Dave will then present back to
// Alice. Other implementations may simply pass along an unguessable token and
// instruct Alice that whoever presents the token should receive the capability.
// A VatNetwork may choose not to allow forwarding if it doesn't want its security
// to be dependent on secret bearer tokens nor cryptographic signatures.
type ThirdPartyToContact capnp.Ptr

// The information that must be sent in a `Provide` message to identify the
// recipient of the capability.
//
// In a network where each vat has a public/private key pair, this could simply
// be the public key fingerprint of the recipient along with a nonce matching
// the one in the `ProvisionId`.
//
// As another example, when communicating between processes on the same machine
// over Unix sockets, RecipientId could simply refer to a file descriptor
// attached to the message via SCM_RIGHTS.  This file descriptor would be one
// end of a newly-created socketpair, with the other end having been sent to the
// capability's recipient in ThirdPartyCapId.
type ThirdPartyToAwait capnp.Ptr

// The information that must be sent in an `Accept` message to identify the
// object being accepted.
//
// In a network where each vat has a public/private key pair, this could simply
// be the public key fingerprint of the provider vat along with a nonce matching
// the one in the `RecipientId` used in the `Provide` message sent from that
// provider.
type ThirdPartyCompletion capnp.Ptr

// Data needed to perform a third-party handoff.
type IntroductionInfo struct {
	SendToProvider  ThirdPartyToAwait
	SendToRecipient ThirdPartyToContact
}

// A Network is a reference to a multi-party (generally >= 3) network
// of Cap'n Proto peers. Use this instead of NewConn when establishing
// connections outside a point-to-point setting.
//
// In addition to satisfying the method set, a correct implementation
// of Network must be comparable.
type Network interface {
	// Return the identifier for caller on this network.
	LocalID() PeerID

	// Connect to another peer by ID. Re-uses any existing connection
	// to the peer.
	Dial(PeerID) (*Conn, error)

	// Accept and handle incoming connections on the network until
	// the context is canceled.
	Serve(context.Context) error
}

// A Network3PH is a Network which supports three-party handoff of capabilities.
// TODO(before merge): could this interface be named better?
type Network3PH interface {
	// Introduces both connections for a three-party handoff. After this,
	// the `ThirdPartyToAwait` will be sent to the `provider` and the
	// `ThirdPartyToContact` will be sent to the `recipient`.
	//
	// An error indicates introduction is not possible between the two `Conn`s.
	Introduce(provider *Conn, recipient *Conn) (IntroductionInfo, error)

	// Attempts forwarding of a `ThirdPartyToContact` received from `from` to
	// `destination`, with both vats being in this Network. This method
	// return a `ThirdPartyToContact` to send to `destination`.
	//
	// An error indicates forwarding is not possible.
	Forward(from *Conn, destination *Conn, info ThirdPartyToContact) (ThirdPartyToContact, error)

	// Completes a three-party handoff.
	//
	// The provided `completion` has been received from `conn` in an `Accept`.
	//
	// This method blocks until there is a matching `AwaitThirdParty`, if there is
	// none currently, and returns the `value` passed to it.
	//
	// An error indicates that this completion can never succeed, for example due
	// to a `completion` that is malformed. The error will be sent in response to the
	// `Accept`.
	CompleteThirdParty(ctx context.Context, conn *Conn, completion ThirdPartyCompletion) (any, error)

	// Awaits for completion of a three-party handoff.
	//
	// The provided `await` has been received from `conn`.
	//
	// While the context is valid, any `CompleteThirdParty` calls that match
	// the provided `await` should return `value`.
	//
	// After the context is canceled, future calls to `CompleteThirdParty` are
	// not required to return the provided `value`.
	//
	// This method SHOULD not block.
	AwaitThirdParty(ctx context.Context, conn *Conn, await ThirdPartyToAwait, value any)
}
