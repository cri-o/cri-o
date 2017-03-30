package server

import (
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/containers/storage"
	"github.com/kubernetes-incubator/cri-o/oci"
	pkgstorage "github.com/kubernetes-incubator/cri-o/pkg/storage"
	"github.com/kubernetes-incubator/cri-o/server/sandbox"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// RemovePodSandbox deletes the sandbox. If there are any running containers in the
// sandbox, they should be force deleted.
func (s *Server) RemovePodSandbox(ctx context.Context, req *pb.RemovePodSandboxRequest) (*pb.RemovePodSandboxResponse, error) {
	logrus.Debugf("RemovePodSandboxRequest %+v", req)
	sb, err := s.getPodSandboxFromRequest(req.PodSandboxId)
	if err != nil {
		if err == sandbox.ErrSandboxIDEmpty {
			return nil, err
		}

		// If the sandbox isn't found we just return an empty response to adhere
		// the the CRI interface which expects to not error out in not found
		// cases.

		resp := &pb.RemovePodSandboxResponse{}
		logrus.Warnf("could not get sandbox %s, it's probably been removed already: %v", req.PodSandboxId, err)
		return resp, nil
	}

	podInfraContainer := sb.InfraContainer()
	containers := sb.Containers()
	containers = append(containers, podInfraContainer)

	// Delete all the containers in the sandbox
	for _, c := range containers {
		if err := s.runtime.UpdateStatus(c); err != nil {
			return nil, fmt.Errorf("failed to update container state: %v", err)
		}

		cState := s.runtime.ContainerStatus(c)
		if cState.Status == oci.ContainerStateCreated || cState.Status == oci.ContainerStateRunning {
			if err := s.runtime.StopContainer(c, -1); err != nil {
				// Assume container is already stopped
				logrus.Warnf("failed to stop container %s: %v", c.Name(), err)
			}
		}

		if err := s.runtime.DeleteContainer(c); err != nil {
			return nil, fmt.Errorf("failed to delete container %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
		}

		if c.ID() == podInfraContainer.ID() {
			continue
		}

		if err := s.storageRuntimeServer.StopContainer(c.ID()); err != nil && err != storage.ErrContainerUnknown {
			// assume container already umounted
			logrus.Warnf("failed to stop container %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
		}
		if err := s.storageRuntimeServer.DeleteContainer(c.ID()); err != nil && err != storage.ErrContainerUnknown {
			return nil, fmt.Errorf("failed to delete container %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
		}

		if err := s.removeContainer(c); err != nil {
			return nil, fmt.Errorf("failed to delete container %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
		}
	}

	if err := s.removeSandbox(sb.ID()); err != nil {
		return nil, fmt.Errorf("error removing sandbox %s: %v", sb.ID(), err)
	}

	// Remove the files related to the sandbox
	if err := s.storageRuntimeServer.StopContainer(sb.ID()); err != nil && err != storage.ErrContainerUnknown {
		logrus.Warnf("failed to stop sandbox container in pod sandbox %s: %v", sb.ID(), err)
	}
	if err := s.storageRuntimeServer.RemovePodSandbox(sb.ID()); err != nil && err != pkgstorage.ErrInvalidSandboxID {
		return nil, fmt.Errorf("failed to remove pod sandbox %s: %v", sb.ID(), err)
	}

	resp := &pb.RemovePodSandboxResponse{}
	logrus.Debugf("RemovePodSandboxResponse %+v", resp)
	return resp, nil
}
