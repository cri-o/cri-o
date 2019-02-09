// +build linux

package server

import (
	"fmt"
	"time"

	"github.com/containers/storage"
	"github.com/docker/docker/pkg/mount"
	"github.com/docker/docker/pkg/symlink"
	"github.com/kubernetes-sigs/cri-o/lib/sandbox"
	"github.com/kubernetes-sigs/cri-o/oci"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/unix"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

func (s *Server) stopPodSandbox(ctx context.Context, req *pb.StopPodSandboxRequest) (resp *pb.StopPodSandboxResponse, err error) {
	const operation = "stop_pod_sandbox"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()

	logrus.Debugf("StopPodSandboxRequest %+v", req)
	sb, err := s.getPodSandboxFromRequest(req.PodSandboxId)
	if err != nil {
		if err == sandbox.ErrIDEmpty {
			return nil, err
		}

		// If the sandbox isn't found we just return an empty response to adhere
		// the CRI interface which expects to not error out in not found
		// cases.

		resp = &pb.StopPodSandboxResponse{}
		logrus.Warnf("could not get sandbox %s, it's probably been stopped already: %v", req.PodSandboxId, err)
		logrus.Debugf("StopPodSandboxResponse %s: %+v", req.PodSandboxId, resp)
		return resp, nil
	}
	stopMutex := sb.StopMutex()
	stopMutex.Lock()
	defer stopMutex.Unlock()

	if sb.Stopped() {
		resp = &pb.StopPodSandboxResponse{}
		logrus.Debugf("StopPodSandboxResponse %s: %+v", sb.ID(), resp)
		return resp, nil
	}

	podInfraContainer := sb.InfraContainer()
	containers := sb.Containers().List()
	if podInfraContainer != nil {
		containers = append(containers, podInfraContainer)
	}

	// Clean up sandbox networking and close its network namespace.
	s.networkStop(sb)

	const maxWorkers = 128
	var waitGroup errgroup.Group
	for i := 0; i < len(containers); i += maxWorkers {
		max := i + maxWorkers
		if len(containers) < max {
			max = len(containers)
		}
		for _, ctr := range containers[i:max] {
			cStatus := ctr.State()
			if cStatus.Status != oci.ContainerStateStopped {
				if ctr.ID() == podInfraContainer.ID() {
					continue
				}
				c := ctr
				waitGroup.Go(func() error {
					timeout := int64(10)
					if err := s.Runtime().StopContainer(ctx, c, timeout); err != nil {
						return fmt.Errorf("failed to stop container %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
					}
					if err := s.Runtime().WaitContainerStateStopped(ctx, c); err != nil {
						return fmt.Errorf("failed to get container 'stopped' status %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
					}
					if err := s.StorageRuntimeServer().StopContainer(c.ID()); err != nil && errors.Cause(err) != storage.ErrContainerUnknown {
						// assume container already umounted
						logrus.Warnf("failed to stop container %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
					}
					s.ContainerStateToDisk(c)
					return nil
				})
			}
		}
		if err := waitGroup.Wait(); err != nil {
			return nil, err
		}
	}

	podInfraStatus := podInfraContainer.State()
	if podInfraStatus.Status != oci.ContainerStateStopped {
		timeout := int64(10)
		if err := s.Runtime().StopContainer(ctx, podInfraContainer, timeout); err != nil {
			return nil, fmt.Errorf("failed to stop infra container %s in pod sandbox %s: %v", podInfraContainer.Name(), sb.ID(), err)
		}
		if err := s.Runtime().WaitContainerStateStopped(ctx, podInfraContainer); err != nil {
			return nil, fmt.Errorf("failed to get infra container 'stopped' status %s in pod sandbox %s: %v", podInfraContainer.Name(), sb.ID(), err)
		}
	}
	if s.config.Config.ManageNetworkNSLifecycle {
		if err := sb.NetNsRemove(); err != nil {
			return nil, err
		}
	}

	// unmount the shm for the pod
	if sb.ShmPath() != "/dev/shm" {
		// we got namespaces in the form of
		// /var/run/containers/storage/overlay-containers/CID/userdata/shm
		// but /var/run on most system is symlinked to /run so we first resolve
		// the symlink and then try and see if it's mounted
		fp, err := symlink.FollowSymlinkInScope(sb.ShmPath(), "/")
		if err != nil {
			return nil, err
		}
		if mounted, err := mount.Mounted(fp); err == nil && mounted {
			if err := unix.Unmount(fp, unix.MNT_DETACH); err != nil {
				return nil, err
			}
		}
	}

	if err := s.StorageRuntimeServer().StopContainer(sb.ID()); err != nil && errors.Cause(err) != storage.ErrContainerUnknown {
		logrus.Warnf("failed to stop sandbox container in pod sandbox %s: %v", sb.ID(), err)
	}

	sb.SetStopped()
	resp = &pb.StopPodSandboxResponse{}
	logrus.Debugf("StopPodSandboxResponse %s: %+v", sb.ID(), resp)
	return resp, nil
}
