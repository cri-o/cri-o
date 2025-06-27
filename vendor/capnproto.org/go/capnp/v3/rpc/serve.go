package rpc

import (
	"context"
	"errors"
	"net"

	"capnproto.org/go/capnp/v3"
)

// serveOpts are options for the Cap'n Proto server.
type serveOpts struct {
	newTransport NewTransportFunc
}

// defaultServeOpts returns the default server opts.
func defaultServeOpts() serveOpts {
	return serveOpts{
		newTransport: NewStreamTransport,
	}
}

type ServeOption func(*serveOpts)

// WithBasicStreamingTransport enables the streaming transport with basic encoding.
func WithBasicStreamingTransport() ServeOption {
	return func(opts *serveOpts) {
		opts.newTransport = NewStreamTransport
	}
}

// WithPackedStreamingTransport enables the streaming transport with packed encoding.
func WithPackedStreamingTransport() ServeOption {
	return func(opts *serveOpts) {
		opts.newTransport = NewPackedStreamTransport
	}
}

// Serve serves a Cap'n Proto RPC to incoming connections.
//
// Serve will take ownership of bootstrapClient and release it after the listener closes.
//
// Serve exits with the listener error if the listener is closed by the owner.
func Serve(lis net.Listener, boot capnp.Client, opts ...ServeOption) error {
	if !boot.IsValid() {
		err := errors.New("bootstrap client is not valid")
		return err
	}
	// Since we took ownership of the bootstrap client, release it after we're done.
	defer boot.Release()

	options := defaultServeOpts()
	for _, o := range opts {
		o(&options)
	}
	for {
		// Accept incoming connections
		conn, err := lis.Accept()
		if err != nil {
			return err
		}

		// the RPC connection takes ownership of the bootstrap interface and will release it when the connection
		// exits, so use AddRef to avoid releasing the provided bootstrap client capability.
		opts := Options{
			BootstrapClient: boot.AddRef(),
		}
		// For each new incoming connection, create a new RPC transport connection that will serve incoming RPC requests
		transport := options.newTransport(conn)
		_ = NewConn(transport, &opts)
	}
}

// ListenAndServe opens a listener on the given address and serves a Cap'n Proto RPC to incoming connections
//
// network and address are passed to net.Listen. Use network "unix" for Unix Domain Sockets
// and "tcp" for regular TCP IP4 or IP6 connections.
//
// ListenAndServe will take ownership of bootstrapClient and release it on exit.
func ListenAndServe(ctx context.Context, network, addr string, bootstrapClient capnp.Client, opts ...ServeOption) error {

	listener, err := net.Listen(network, addr)

	if err == nil {
		// to close this listener, close the context
		go func() {
			<-ctx.Done()
			_ = listener.Close()
		}()
		err = Serve(listener, bootstrapClient)
	}
	return err
}
