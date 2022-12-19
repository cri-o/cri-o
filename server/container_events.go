package server

import (
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *Server) GetContainerEvents(req *types.GetEventsRequest, res types.RuntimeService_GetContainerEventsServer) error {
	// Spoofing the GetContainerEvents method to return nil response until actual support code is added.
	return nil
}
