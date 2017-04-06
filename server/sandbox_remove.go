package server

import (
	"fmt"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/kubernetes-incubator/cri-o/server/sandbox"
	"github.com/opencontainers/selinux/go-selinux/label"
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
			if err := s.runtime.StopContainer(c); err != nil {
				return nil, fmt.Errorf("failed to stop container %s: %v", c.Name(), err)
			}
		}

		if err := s.runtime.DeleteContainer(c); err != nil {
			return nil, fmt.Errorf("failed to delete container %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
		}

		if c == podInfraContainer {
			continue
		}

		if err := s.storage.StopContainer(c.ID()); err != nil {
			return nil, fmt.Errorf("failed to delete container %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
		}
		if err := s.storage.DeleteContainer(c.ID()); err != nil {
			return nil, fmt.Errorf("failed to delete container %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
		}

		if err := s.removeContainer(c); err != nil {
			return nil, fmt.Errorf("failed to delete container %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
		}
	}

	if err := label.ReleaseLabel(sb.ProcessLabel()); err != nil {
		return nil, err
	}

	// unmount the shm for the pod
	if sb.ShmPath() != "/dev/shm" {
		if err := syscall.Unmount(sb.ShmPath(), syscall.MNT_DETACH); err != nil {
			return nil, err
		}
	}

	if err := sb.NetNsRemove(); err != nil {
		return nil, fmt.Errorf("failed to remove networking namespace for sandbox %s: %v", sb.ID(), err)
	}

	if err := s.removeSandbox(sb.ID()); err != nil {
		return nil, fmt.Errorf("error removing sandbox %s: %v", sb.ID(), err)
	}

	// Remove the files related to the sandbox
	if err := s.storage.StopContainer(sb.ID()); err != nil {
		return nil, fmt.Errorf("failed to delete sandbox container in pod sandbox %s: %v", sb.ID(), err)
	}
	if err := s.storage.RemovePodSandbox(sb.ID()); err != nil {
		return nil, fmt.Errorf("failed to remove pod sandbox %s: %v", sb.ID(), err)
	}

	resp := &pb.RemovePodSandboxResponse{}
	logrus.Debugf("RemovePodSandboxResponse %+v", resp)
	return resp, nil
}
