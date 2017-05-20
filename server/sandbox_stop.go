package server

import (
	"fmt"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/cri-o/oci"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// StopPodSandbox stops the sandbox. If there are any running containers in the
// sandbox, they should be force terminated.
func (s *Server) StopPodSandbox(ctx context.Context, req *pb.StopPodSandboxRequest) (*pb.StopPodSandboxResponse, error) {
	logrus.Debugf("StopPodSandboxRequest %+v", req)
	sb, err := s.getPodSandboxFromRequest(req.PodSandboxId)
	if err != nil {
		if err == errSandboxIDEmpty {
			return nil, err
		}

		// If the sandbox isn't found we just return an empty response to adhere
		// the the CRI interface which expects to not error out in not found
		// cases.

		resp := &pb.StopPodSandboxResponse{}
		logrus.Warnf("could not get sandbox %s, it's probably been stopped already: %v", req.PodSandboxId, err)
		return resp, nil
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
		cStatus := s.runtime.ContainerStatus(c)
		if cStatus.Status != oci.ContainerStateStopped {
			if err := s.runtime.StopContainer(c, -1); err != nil {
				return nil, fmt.Errorf("failed to stop container %s in pod sandbox %s: %v", c.Name(), sb.id, err)
			}
		}
		s.containerStateToDisk(c)
	}

	resp := &pb.StopPodSandboxResponse{}
	logrus.Debugf("StopPodSandboxResponse: %+v", resp)
	return resp, nil
}

// StopAllPodSandboxes removes all pod sandboxes
func (s *Server) StopAllPodSandboxes() {
	logrus.Debugf("StopAllPodSandboxes")
	for _, sb := range s.state.sandboxes {
		pod := &pb.StopPodSandboxRequest{
			PodSandboxId: sb.id,
		}
		if _, err := s.StopPodSandbox(nil, pod); err != nil {
			logrus.Warnf("could not StopPodSandbox %s: %v", sb.id, err)
		}
	}
}
