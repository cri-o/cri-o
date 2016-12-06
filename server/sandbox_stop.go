package server

import (
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/cri-o/oci"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// StopPodSandbox stops the sandbox. If there are any running containers in the
// sandbox, they should be force terminated.
func (s *Server) StopPodSandbox(ctx context.Context, req *pb.StopPodSandboxRequest) (*pb.StopPodSandboxResponse, error) {
	logrus.Debugf("StopPodSandboxRequest %+v", req)
	sb, err := s.getPodSandboxFromRequest(req)
	if err != nil {
		return nil, err
	}

	podNamespace := ""
	podInfraContainer := sb.infraContainer
	netnsPath, err := podInfraContainer.NetNsPath()
	if err != nil {
		return nil, err
	}

	if err := s.netPlugin.TearDownPod(netnsPath, podNamespace, sb.id, podInfraContainer.Name()); err != nil {
		return nil, fmt.Errorf("failed to destroy network for container %s in sandbox %s: %v",
			podInfraContainer.Name(), sb.id, err)
	}

	containers := sb.containers.List()
	containers = append(containers, podInfraContainer)

	for _, c := range containers {
		if err := s.runtime.UpdateStatus(c); err != nil {
			return nil, err
		}
		cStatus := s.runtime.ContainerStatus(c)
		if cStatus.Status != oci.ContainerStateStopped {
			if err := s.runtime.StopContainer(c); err != nil {
				return nil, fmt.Errorf("failed to stop container %s in sandbox %s: %v", c.Name(), sb.id, err)
			}
		}
	}

	resp := &pb.StopPodSandboxResponse{}
	logrus.Debugf("StopPodSandboxResponse: %+v", resp)
	return resp, nil
}
