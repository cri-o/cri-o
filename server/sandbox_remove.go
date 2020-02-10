package server

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/containers/storage"
	"github.com/cri-o/cri-o/lib/sandbox"
	"github.com/cri-o/cri-o/oci"
	pkgstorage "github.com/cri-o/cri-o/pkg/storage"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

// RemovePodSandbox deletes the sandbox. If there are any running containers in the
// sandbox, they should be force deleted.
func (s *Server) RemovePodSandbox(ctx context.Context, req *pb.RemovePodSandboxRequest) (resp *pb.RemovePodSandboxResponse, err error) {
	const operation = "remove_pod_sandbox"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()

	logrus.Debugf("RemovePodSandboxRequest %+v", req)
	sb, err := s.getPodSandboxFromRequest(req.PodSandboxId)
	if err != nil {
		if err == sandbox.ErrIDEmpty {
			return nil, err
		}

		// If the sandbox isn't found we just return an empty response to adhere
		// the CRI interface which expects to not error out in not found
		// cases.

		resp = &pb.RemovePodSandboxResponse{}
		logrus.Warnf("could not get sandbox %s, it's probably been removed already: %v", req.PodSandboxId, err)
		return resp, nil
	}

	podInfraContainer := sb.InfraContainer()
	containers := sb.Containers().List()
	containers = append(containers, podInfraContainer)

	// Delete all the containers in the sandbox
	for _, c := range containers {
		if !sb.Stopped() {
			cState := c.State()
			if cState.Status == oci.ContainerStateCreated || cState.Status == oci.ContainerStateRunning {
				timeout := int64(10)
				if err := s.Runtime().StopContainer(ctx, c, timeout); err != nil {
					// Assume container is already stopped
					logrus.Warnf("failed to stop container %s: %v", c.Name(), err)
				}
				if err := s.Runtime().WaitContainerStateStopped(ctx, c); err != nil {
					return nil, fmt.Errorf("failed to get container 'stopped' status %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
				}
			}
		}

		if err := s.Runtime().DeleteContainer(c); err != nil {
			return nil, fmt.Errorf("failed to delete container %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
		}

		if c.ID() == podInfraContainer.ID() {
			continue
		}

		c.CleanupConmonCgroup()

		if err := s.StorageRuntimeServer().StopContainer(c.ID()); err != nil && err != storage.ErrContainerUnknown {
			// assume container already umounted
			logrus.Warnf("failed to stop container %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
		}
		if err := s.StorageRuntimeServer().DeleteContainer(c.ID()); err != nil && err != storage.ErrContainerUnknown {
			return nil, fmt.Errorf("failed to delete container %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
		}

		s.ReleaseContainerName(c.Name())
		s.removeContainer(c)
		if err := s.CtrIDIndex().Delete(c.ID()); err != nil {
			return nil, fmt.Errorf("failed to delete container %s in pod sandbox %s from index: %v", c.Name(), sb.ID(), err)
		}
		s.StopMonitoringConmon(c)
	}

	s.removeInfraContainer(podInfraContainer)
	podInfraContainer.CleanupConmonCgroup()

	// Remove the files related to the sandbox
	if err := s.StorageRuntimeServer().StopContainer(sb.ID()); err != nil && errors.Cause(err) != storage.ErrContainerUnknown {
		logrus.Warnf("failed to stop sandbox container in pod sandbox %s: %v", sb.ID(), err)
	}
	if err := s.StorageRuntimeServer().RemovePodSandbox(sb.ID()); err != nil && err != pkgstorage.ErrInvalidSandboxID {
		return nil, fmt.Errorf("failed to remove pod sandbox %s: %v", sb.ID(), err)
	}

	s.ReleaseContainerName(podInfraContainer.Name())
	if err := s.CtrIDIndex().Delete(podInfraContainer.ID()); err != nil {
		return nil, fmt.Errorf("failed to delete infra container %s in pod sandbox %s from index: %v", podInfraContainer.ID(), sb.ID(), err)
	}
	if err := os.Remove(filepath.Join(s.Config().RuntimeConfig.ContainerExitsDir, podInfraContainer.ID())); err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrapf(err, "failed to remove pod infra container exit file %s", podInfraContainer.ID())
	}

	s.ReleasePodName(sb.Name())
	s.removeSandbox(sb.ID())
	if err := s.PodIDIndex().Delete(sb.ID()); err != nil {
		return nil, fmt.Errorf("failed to delete pod sandbox %s from index: %v", sb.ID(), err)
	}

	logrus.Infof("Removed pod sandbox with infra container: %s", podInfraContainer.Description())
	resp = &pb.RemovePodSandboxResponse{}
	logrus.Debugf("RemovePodSandboxResponse %+v", resp)
	return resp, nil
}
