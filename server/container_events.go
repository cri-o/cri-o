package server

import (
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

type containerEventConn struct {
	ch   chan struct{}
	once sync.Once
	err  error
}

func (c *containerEventConn) done() {
	// only ever close the channel once, even if multiple messages are sent
	c.once.Do(func() {
		close(c.ch)
	})
}

func (c *containerEventConn) wait() {
	<-c.ch
}

// GetContainerEvents sends the stream of container events to clients.
func (s *Server) GetContainerEvents(_ *types.GetEventsRequest, ces types.RuntimeService_GetContainerEventsServer) error {
	if !s.ContainerServer.Config().EnablePodEvents {
		return nil
	}

	s.containerEventStreamBroadcaster.Do(func() {
		// note that this function will run indefinitely until ContainerEventsChan is closed
		go s.broadcastEvents()
	})

	conn := &containerEventConn{
		ch:   make(chan struct{}),
		once: sync.Once{},
	}
	s.containerEventClients.Store(ces, conn)

	// wait here until we don't want to send events to this client anymore
	conn.wait()
	s.containerEventClients.Delete(ces)

	return conn.err
}

func (s *Server) broadcastEvents() {
	// notify all connections that ContainerEventsChan has been closed
	defer func() {
		for _, value := range s.containerEventClients.Range {
			conn, ok := value.(*containerEventConn)
			if !ok {
				continue
			}

			conn.done()
		}
	}()

	//nolint:govet // copylock is not harmful for this implementation
	for containerEvent := range s.ContainerEventsChan {
		for key, value := range s.containerEventClients.Range {
			stream, ok := key.(types.RuntimeService_GetContainerEventsServer)
			if !ok {
				continue
			}

			conn, ok := value.(*containerEventConn)
			if !ok {
				continue
			}

			if err := stream.Send(&containerEvent); err != nil {
				code, _ := status.FromError(err)
				// when the client closes the connection this error is expected
				// so only log non transport closing errors
				if code.Code() != codes.Unavailable && code.Message() != "transport is closing" {
					conn.err = err
				}
				// notify our waiting client connection that we are done
				conn.done()
			}
		}
	}
}
