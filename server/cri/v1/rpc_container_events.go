package v1

import (
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) GetContainerEvents(req *pb.GetEventsRequest, ces pb.RuntimeService_GetContainerEventsServer) error {

	for containerEvent := range s.server.ContainerEventsChan {
		if err := ces.Send(&containerEvent); err != nil {
			return err
		}
	}
	return nil
}
