package server

import (
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

type containerEventConn struct {
	wg  sync.WaitGroup
	err error
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
		wg: sync.WaitGroup{},
	}

	s.containerEventClients.Store(ces, conn)
	conn.wg.Add(1)

	// wait here until we don't want to send events to this client anymore
	conn.wg.Wait()
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
			conn.wg.Done()
		}
	}()

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
				conn.wg.Done()
			}
		}
	}
}
