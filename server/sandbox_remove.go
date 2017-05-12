package server

import (
	"fmt"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/cri-o/oci"
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
		if err == errSandboxIDEmpty {
			return nil, err
		}

		resp := &pb.RemovePodSandboxResponse{}
		logrus.Warnf("could not get sandbox %s, it's probably been removed already: %v", req.PodSandboxId, err)
		return resp, nil
	}

	podInfraContainer := sb.infraContainer
	containers := sb.containers.List()
	containers = append(containers, podInfraContainer)

	// Delete all the containers in the sandbox
	for _, c := range containers {
		if err := s.runtime.UpdateStatus(c); err != nil {
			logrus.Debugf("failed to update container %s state: %v", c.ID, err)
		}

		cState := s.runtime.ContainerStatus(c)
		if cState != nil && (cState.Status == oci.ContainerStateCreated || cState.Status == oci.ContainerStateRunning) {
			if err := s.runtime.StopContainer(c); err != nil {
				// Assume container is already stopped
				logrus.Warnf("failed to stop container %s: %v", c.Name, err)
			}
		}

		if err := s.runtime.DeleteContainer(c); err != nil {
			logrus.Warnf("failed to delete container %s in pod sandbox %s: %v", c.Name, sb.id, err)
		}

		if c.ID() == podInfraContainer.ID() {
			continue
		}

		if err := s.storageRuntimeServer.StopContainer(c.ID()); err != nil {
			logrus.Warnf("failed to stop container %s in pod sandbox %s: %v", c.Name, sb.id, err)
		}
		if err := s.storageRuntimeServer.DeleteContainer(c.ID()); err != nil {
			return nil, fmt.Errorf("failed to delete container %s in pod sandbox %s: %v", c.Name, sb.id, err)
		}

		s.releaseContainerName(c.Name)
		s.removeContainer(c)
		if err := s.ctrIDIndex.Delete(c.ID()); err != nil {
			return nil, fmt.Errorf("failed to delete container %s in pod sandbox %s from index: %v", c.Name, sb.id, err)
		}
	}

	if err := label.ReleaseLabel(sb.processLabel); err != nil {
		return nil, err
	}

	// unmount the shm for the pod
	if sb.shmPath != "/dev/shm" {
		if err := syscall.Unmount(sb.shmPath, syscall.MNT_DETACH); err != nil {
			return nil, err
		}
	}

	if err := sb.netNsRemove(); err != nil {
		return nil, fmt.Errorf("failed to remove networking namespace for sandbox %s: %v", sb.id, err)
	}

	s.removeContainer(podInfraContainer)

	// Remove the files related to the sandbox
	if err := s.storageRuntimeServer.StopContainer(sb.id); err != nil {
		return nil, fmt.Errorf("failed to delete sandbox container in pod sandbox %s: %v", sb.id, err)
	}
	if err := s.storageRuntimeServer.RemovePodSandbox(sb.id); err != nil {
		return nil, fmt.Errorf("failed to remove pod sandbox %s: %v", sb.id, err)
	}

	s.releaseContainerName(podInfraContainer.Name)
	if err := s.ctrIDIndex.Delete(podInfraContainer.ID()); err != nil {
		return nil, fmt.Errorf("failed to delete infra container %s in pod sandbox %s from index: %v", podInfraContainer.ID(), sb.id, err)
	}

	s.releasePodName(sb.name)
	s.removeSandbox(sb.id)
	if err := s.podIDIndex.Delete(sb.id); err != nil {
		return nil, fmt.Errorf("failed to delete pod sandbox %s from index: %v", sb.id, err)
	}

	resp := &pb.RemovePodSandboxResponse{}
	logrus.Debugf("RemovePodSandboxResponse %+v", resp)
	return resp, nil
}
