package server

import (
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// GetContainerEvents sends the stream of container events to the client - kubelet
func (s *Server) GetContainerEvents(req *types.GetEventsRequest, ces types.RuntimeService_GetContainerEventsServer) error {
	if s.Config().EnablePodEvents {
		for containerEvent := range s.ContainerEventsChan {
			if err := ces.Send(&containerEvent); err != nil {
				return err
			}
		}
	}
	return nil
}
