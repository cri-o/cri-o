package server

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/opencontainers/runc/libcontainer/label"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// RemovePodSandbox deletes the sandbox. If there are any running containers in the
// sandbox, they should be force deleted.
func (s *Server) RemovePodSandbox(ctx context.Context, req *pb.RemovePodSandboxRequest) (*pb.RemovePodSandboxResponse, error) {
	logrus.Debugf("RemovePodSandboxRequest %+v", req)
	sb, err := s.getPodSandboxFromRequest(req)
	if err != nil {
		if err == errSandboxIDEmpty {
			return nil, err
		}

		resp := &pb.RemovePodSandboxResponse{}
		logrus.Warnf("could not get sandbox %s, it's probably been removed already: %v", req.GetPodSandboxId(), err)
		return resp, nil
	}

	podInfraContainer := sb.infraContainer
	containers := sb.containers.List()
	containers = append(containers, podInfraContainer)

	// Delete all the containers in the sandbox
	for _, c := range containers {
		if err := s.runtime.UpdateStatus(c); err != nil {
			return nil, fmt.Errorf("failed to update container state: %v", err)
		}

		cState := s.runtime.ContainerStatus(c)
		if cState.Status == oci.ContainerStateCreated || cState.Status == oci.ContainerStateRunning {
			if err := s.runtime.StopContainer(c); err != nil {
				return nil, fmt.Errorf("failed to stop container %s: %v", c.Name(), err)
			}
		}

		if err := s.runtime.DeleteContainer(c); err != nil {
			return nil, fmt.Errorf("failed to delete container %s in sandbox %s: %v", c.Name(), sb.id, err)
		}

		if c == podInfraContainer {
			continue
		}

		containerDir := filepath.Join(s.runtime.ContainerDir(), c.ID())
		if err := os.RemoveAll(containerDir); err != nil {
			return nil, fmt.Errorf("failed to remove container %s directory: %v", c.Name(), err)
		}

		s.releaseContainerName(c.Name())
		s.removeContainer(c)
	}

	if err := label.UnreserveLabel(sb.processLabel); err != nil {
		return nil, err
	}

	// Remove the files related to the sandbox
	podSandboxDir := filepath.Join(s.config.SandboxDir, sb.id)
	if err := os.RemoveAll(podSandboxDir); err != nil {
		return nil, fmt.Errorf("failed to remove sandbox %s directory: %v", sb.id, err)
	}
	s.releaseContainerName(podInfraContainer.Name())
	s.removeContainer(podInfraContainer)
	sb.infraContainer = nil

	s.releasePodName(sb.name)
	s.removeSandbox(sb.id)

	resp := &pb.RemovePodSandboxResponse{}
	logrus.Debugf("RemovePodSandboxResponse %+v", resp)
	return resp, nil
}
