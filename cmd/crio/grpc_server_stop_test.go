package main

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const stopAfterGracefulStopMethod = "/repro.Service/Hang"

// TestStopAfterGracefulStopWithRunningHandler verifies that Stop() returns
// promptly while GracefulStop() is blocked on a handler that ignores
// cancellation. This is the pattern CRI-O uses on SIGTERM (GracefulStop with
// a timeout, then Stop). Requires google.golang.org/grpc >= v1.85.0
// (grpc-go#9394).
func TestStopAfterGracefulStopWithRunningHandler(t *testing.T) {
	t.Parallel()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	s := grpc.NewServer()
	started := make(chan struct{})
	unblock := make(chan struct{})

	s.RegisterService(&grpc.ServiceDesc{
		ServiceName: "repro.Service",
		HandlerType: (*any)(nil),
		Streams: []grpc.StreamDesc{{
			StreamName:    "Hang",
			ServerStreams: true,
			ClientStreams: true,
			Handler: func(_ any, _ grpc.ServerStream) error {
				close(started)
				<-unblock

				return nil
			},
		}},
	}, struct{}{})

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- s.Serve(lis)
	}()

	cc, err := grpc.NewClient(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := cc.NewStream(ctx, &grpc.StreamDesc{
		StreamName:    "Hang",
		ServerStreams: true,
		ClientStreams: true,
	}, stopAfterGracefulStopMethod)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}

	_ = stream

	select {
	case <-started:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for handler to start: %v", ctx.Err())
	}

	if err := cc.Close(); err != nil {
		t.Fatalf("Close client: %v", err)
	}

	gracefulDone := make(chan struct{})

	go func() {
		s.GracefulStop()
		close(gracefulDone)
	}()

	// Give GracefulStop time to drain and wait on the stuck handler.
	time.Sleep(200 * time.Millisecond)

	stopDone := make(chan struct{})

	go func() {
		s.Stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
	case <-time.After(5 * time.Second):
		t.Fatal(
			"Stop() did not return within 5s; likely deadlocked behind GracefulStop mutex (need grpc >= v1.85.0)",
		)
	}

	close(unblock)

	select {
	case <-gracefulDone:
	case <-time.After(5 * time.Second):
		t.Fatal("GracefulStop() did not return after handler completed")
	}

	select {
	case err := <-serveErr:
		if err != nil {
			t.Logf("Serve returned: %v", err) // expected after Stop/GracefulStop
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after server stop")
	}
}
