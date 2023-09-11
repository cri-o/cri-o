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
type ThirdPartyCapID capnp.Ptr

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
type RecipientID capnp.Ptr

// The information that must be sent in an `Accept` message to identify the
// object being accepted.
//
// In a network where each vat has a public/private key pair, this could simply
// be the public key fingerprint of the provider vat along with a nonce matching
// the one in the `RecipientId` used in the `Provide` message sent from that
// provider.
type ProvisionID capnp.Ptr

// Data needed to perform a third-party handoff, returned by
// Newtork.Introduce.
type IntroductionInfo struct {
	SendToRecipient ThirdPartyCapID
	SendToProvider  RecipientID
}

// A Network is a reference to a multi-party (generally >= 3) network
// of Cap'n Proto peers. Use this instead of NewConn when establishing
// connections outside a point-to-point setting.
type Network interface {
	// Return the identifier for caller on this network.
	LocalID() PeerID

	// Connect to another peer by ID. The supplied Options are used
	// for the connection, with the values for RemotePeerID and Network
	// overridden by the Network.
	Dial(PeerID, *Options) (*Conn, error)

	// Accept the next incoming connection on the network, using the
	// supplied Options for the connection. Generally, callers will
	// want to invoke this in a loop when launching a server.
	Accept(context.Context, *Options) (*Conn, error)

	// Introduce the two connections, in preparation for a third party
	// handoff. Afterwards, a Provide messsage should be sent to
	// provider, and a ThirdPartyCapId should be sent to recipient.
	Introduce(provider, recipient *Conn) (IntroductionInfo, error)

	// Given a ThirdPartyCapID, received from introducedBy, connect
	// to the third party. The caller should then send an Accept
	// message over the returned Connection.
	DialIntroduced(capID ThirdPartyCapID, introducedBy *Conn) (*Conn, ProvisionID, error)

	// Given a RecipientID received in a Provide message via
	// introducedBy, wait for the recipient to connect, and
	// return the connection formed. If there is already an
	// established connection to the relevant Peer, this
	// SHOULD return the existing connection immediately.
	AcceptIntroduced(recipientID RecipientID, introducedBy *Conn) (*Conn, error)
}
