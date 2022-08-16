package v1

import (
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) GetContainerEvents(req *pb.GetEventsRequest, ces pb.RuntimeService_GetContainerEventsServer) error {
	// Spoofing the GetContainerEvents method to return nil response until actual support code is added.
	return nil
}
