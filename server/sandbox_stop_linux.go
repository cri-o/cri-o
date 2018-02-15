// +build linux

package server

import (
	"time"

	"github.com/docker/docker/pkg/mount"
	"github.com/docker/docker/pkg/symlink"
	"github.com/kubernetes-sigs/cri-o/lib/sandbox"
	"github.com/kubernetes-sigs/cri-o/oci"
	"github.com/opencontainers/selinux/go-selinux/label"
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

	containers := sb.Containers().List()

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
			id := ctr.ID()
			waitGroup.Go(func() error {
				_, err := s.ContainerServer.ContainerStop(ctx, id, 10)
				return err
			})
		}
		if err := waitGroup.Wait(); err != nil {
			return nil, err
		}
	}

	podInfraContainer := sb.InfraContainer()
	if podInfraContainer != nil {
		podInfraStatus := podInfraContainer.State()

		switch podInfraStatus.Status {
		case oci.ContainerStateStopped: // no-op
		case oci.ContainerStatePaused:
			return nil, errors.Errorf("cannot stop paused infra container %s in pod sandbox %s", podInfraContainer.Name(), sb.ID())
		default:
			if err := s.Runtime().StopContainer(ctx, podInfraContainer, 10); err != nil {
				return nil, errors.Wrapf(err, "failed to stop infra container %s in pod sandbox %s", podInfraContainer.Name(), sb.ID())
			}
			if err := s.Runtime().WaitContainerStateStopped(ctx, podInfraContainer); err != nil {
				return nil, errors.Wrapf(err, "failed to get infra container 'stopped' status %s in pod sandbox %s", podInfraContainer.Name(), sb.ID())
			}
		}
	}
	if s.config.Config.ManageNetworkNSLifecycle {
		if err := sb.NetNsRemove(); err != nil {
			return nil, err
		}
	}

	if err := label.ReleaseLabel(sb.ProcessLabel()); err != nil {
		return nil, err
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

	if err := s.StorageRuntimeServer().StopContainer(sb.ID()); err != nil {
		return nil, errors.Wrapf(err, "failed to unmount infra container %s in pod sandbox %s", podInfraContainer.Name(), sb.ID())
	}

	logrus.Infof("Stopped pod sandbox %s", sb.ID())
	sb.SetStopped()
	resp = &pb.StopPodSandboxResponse{}
	logrus.Debugf("StopPodSandboxResponse %s: %+v", sb.ID(), resp)
	return resp, nil
}
