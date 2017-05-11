package server

import (
	"fmt"
	"os"

	"github.com/Sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// StopPodSandbox stops the sandbox. If there are any running containers in the
// sandbox, they should be force terminated.
func (s *Server) StopPodSandbox(ctx context.Context, req *pb.StopPodSandboxRequest) (*pb.StopPodSandboxResponse, error) {
	logrus.Debugf("StopPodSandboxRequest %+v", req)
	sb, err := s.getPodSandboxFromRequest(req.PodSandboxId)
	if err != nil {
		return nil, err
	}

	podInfraContainer := sb.infraContainer
	netnsPath, err := podInfraContainer.NetNsPath()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(netnsPath); err == nil {
		if err2 := s.netPlugin.TearDownPod(netnsPath, sb.namespace, sb.kubeName, sb.id); err2 != nil {
			return nil, fmt.Errorf("failed to destroy network for container %s in sandbox %s: %v",
				podInfraContainer.Name(), sb.id, err2)
		}
	} else if !os.IsNotExist(err) { // it's ok for netnsPath to *not* exist
		return nil, fmt.Errorf("failed to stat netns path for container %s in sandbox %s before tearing down the network: %v",
			sb.name, sb.id, err)
	}

	// Close the sandbox networking namespace.
	if err := sb.netNsRemove(); err != nil {
		return nil, err
	}

	containers := sb.containers.List()
	containers = append(containers, podInfraContainer)

	for _, c := range containers {
		if err := s.runtime.UpdateStatus(c); err != nil {
			return nil, err
		}
		if err := s.runtime.StopContainer(c); err != nil {
			return nil, fmt.Errorf("failed to stop container %s in pod sandbox %s: %v", c.Name(), sb.id, err)
		}
	}

	resp := &pb.StopPodSandboxResponse{}
	logrus.Debugf("StopPodSandboxResponse: %+v", resp)
	return resp, nil
}
