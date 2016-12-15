package server

import (
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/cri-o/oci"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// Exec prepares a streaming endpoint to execute a command in the container.
func (s *Server) Exec(ctx context.Context, req *pb.ExecRequest) (*pb.ExecResponse, error) {
	logrus.Debugf("ExecRequest %+v", req)
	c, err := s.getContainerFromRequest(req.ContainerId)
	if err != nil {
		return nil, err
	}

	if err = s.runtime.UpdateStatus(c); err != nil {
		return nil, err
	}

	cState := s.runtime.ContainerStatus(c)
	if !(cState.Status == oci.ContainerStateRunning || cState.Status == oci.ContainerStateCreated) {
		return nil, fmt.Errorf("container is not created or running")
	}

	if req.Cmd == nil {
		return nil, fmt.Errorf("exec command cannot be empty")
	}

	url := "url"

	return &pb.ExecResponse{
		Url: url,
	}, nil
}
