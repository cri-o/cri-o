// +build linux

package libpod

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/containerd/cgroups"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// NewPod makes a new, empty pod
func (r *Runtime) NewPod(ctx context.Context, options ...PodCreateOption) (*Pod, error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if !r.valid {
		return nil, ErrRuntimeStopped
	}

	pod, err := newPod(r)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating pod")
	}

	// Set default namespace to runtime's namespace
	// Do so before options run so they can override it
	if r.config.Namespace != "" {
		pod.config.Namespace = r.config.Namespace
	}

	for _, option := range options {
		if err := option(pod); err != nil {
			return nil, errors.Wrapf(err, "error running pod create option")
		}
	}

	if pod.config.Name == "" {
		name, err := r.generateName()
		if err != nil {
			return nil, err
		}
		pod.config.Name = name
	}

	// Allocate a lock for the pod
	lock, err := r.lockManager.AllocateLock()
	if err != nil {
		return nil, errors.Wrapf(err, "error allocating lock for new pod")
	}
	pod.lock = lock
	pod.config.LockID = pod.lock.ID()

	pod.valid = true

	// Check CGroup parent sanity, and set it if it was not set
	switch r.config.CgroupManager {
	case CgroupfsCgroupsManager:
		if pod.config.CgroupParent == "" {
			pod.config.CgroupParent = CgroupfsDefaultCgroupParent
		} else if strings.HasSuffix(path.Base(pod.config.CgroupParent), ".slice") {
			return nil, errors.Wrapf(ErrInvalidArg, "systemd slice received as cgroup parent when using cgroupfs")
		}
		// If we are set to use pod cgroups, set the cgroup parent that
		// all containers in the pod will share
		// No need to create it with cgroupfs - the first container to
		// launch should do it for us
		if pod.config.UsePodCgroup {
			pod.state.CgroupPath = filepath.Join(pod.config.CgroupParent, pod.ID())
		}
	case SystemdCgroupsManager:
		if pod.config.CgroupParent == "" {
			pod.config.CgroupParent = SystemdDefaultCgroupParent
		} else if len(pod.config.CgroupParent) < 6 || !strings.HasSuffix(path.Base(pod.config.CgroupParent), ".slice") {
			return nil, errors.Wrapf(ErrInvalidArg, "did not receive systemd slice as cgroup parent when using systemd to manage cgroups")
		}
		// If we are set to use pod cgroups, set the cgroup parent that
		// all containers in the pod will share
		if pod.config.UsePodCgroup {
			cgroupPath, err := systemdSliceFromPath(pod.config.CgroupParent, fmt.Sprintf("libpod_pod_%s", pod.ID()))
			if err != nil {
				return nil, errors.Wrapf(err, "unable to create pod cgroup for pod %s", pod.ID())
			}
			pod.state.CgroupPath = cgroupPath
		}
	default:
		return nil, errors.Wrapf(ErrInvalidArg, "unsupported CGroup manager: %s - cannot validate cgroup parent", r.config.CgroupManager)
	}

	if pod.config.UsePodCgroup {
		logrus.Debugf("Got pod cgroup as %s", pod.state.CgroupPath)
	}
	if pod.HasInfraContainer() != pod.SharesNamespaces() {
		return nil, errors.Errorf("Pods must have an infra container to share namespaces")
	}

	if err := r.state.AddPod(pod); err != nil {
		return nil, errors.Wrapf(err, "error adding pod to state")
	}

	if pod.HasInfraContainer() {
		ctr, err := r.createInfraContainer(ctx, pod)
		if err != nil {
			// Tear down pod, as it is assumed a the pod will contain
			// a pause container, and it does not.
			if err2 := r.removePod(ctx, pod, true, true); err2 != nil {
				logrus.Errorf("Error removing pod after pause container creation failure: %v", err2)
			}
			return nil, errors.Wrapf(err, "error adding Infra Container")
		}
		pod.state.InfraContainerID = ctr.ID()
		if err := pod.save(); err != nil {
			return nil, err
		}

		// Once the pod infra container has been created, we start it
		if err := ctr.Start(ctx); err != nil {
			// If the infra container does not start, we need to tear the pod down.
			if err2 := r.removePod(ctx, pod, true, true); err2 != nil {
				logrus.Errorf("Error removing pod after infra container failed to start: %v", err2)
			}
			return nil, errors.Wrapf(err, "error starting Infra Container")
		}
	}

	return pod, nil
}

func (r *Runtime) removePod(ctx context.Context, p *Pod, removeCtrs, force bool) error {
	if err := p.updatePod(); err != nil {
		return err
	}

	ctrs, err := r.state.PodContainers(p)
	if err != nil {
		return err
	}

	numCtrs := len(ctrs)

	// If the only container in the pod is the pause container, remove the pod and container unconditionally.
	pauseCtrID := p.state.InfraContainerID
	if numCtrs == 1 && ctrs[0].ID() == pauseCtrID {
		removeCtrs = true
		force = true
	}
	if !removeCtrs && numCtrs > 0 {
		return errors.Wrapf(ErrCtrExists, "pod %s contains containers and cannot be removed", p.ID())
	}

	// Go through and lock all containers so we can operate on them all at once
	dependencies := make(map[string][]string)
	for _, ctr := range ctrs {
		ctr.lock.Lock()
		defer ctr.lock.Unlock()

		// Sync all containers
		if err := ctr.syncContainer(); err != nil {
			return err
		}

		// Check if the container is in a good state to be removed
		if ctr.state.State == ContainerStatePaused {
			return errors.Wrapf(ErrCtrStateInvalid, "pod %s contains paused container %s, cannot remove", p.ID(), ctr.ID())
		}

		if ctr.state.State == ContainerStateUnknown {
			return errors.Wrapf(ErrCtrStateInvalid, "pod %s contains container %s with invalid state", p.ID(), ctr.ID())
		}

		// If the container is running and force is not set we can't do anything
		if ctr.state.State == ContainerStateRunning && !force {
			return errors.Wrapf(ErrCtrStateInvalid, "pod %s contains container %s which is running", p.ID(), ctr.ID())
		}

		// If the container has active exec sessions and force is not set we can't do anything
		if len(ctr.state.ExecSessions) != 0 && !force {
			return errors.Wrapf(ErrCtrStateInvalid, "pod %s contains container %s which has active exec sessions", p.ID(), ctr.ID())
		}

		deps, err := r.state.ContainerInUse(ctr)
		if err != nil {
			return err
		}
		dependencies[ctr.ID()] = deps
	}

	// Check if containers have dependencies
	// If they do, and the dependencies are not in the pod, error
	for ctr, deps := range dependencies {
		for _, dep := range deps {
			if _, ok := dependencies[dep]; !ok {
				return errors.Wrapf(ErrCtrExists, "container %s depends on container %s not in pod %s", ctr, dep, p.ID())
			}
		}
	}

	// First loop through all containers and stop them
	// Do not remove in this loop to ensure that we don't remove unless all
	// containers are in a good state
	if force {
		for _, ctr := range ctrs {
			// If force is set and the container is running, stop it now
			if ctr.state.State == ContainerStateRunning {
				if err := r.ociRuntime.stopContainer(ctr, ctr.StopTimeout()); err != nil {
					return errors.Wrapf(err, "error stopping container %s to remove pod %s", ctr.ID(), p.ID())
				}

				// Sync again to pick up stopped state
				if err := ctr.syncContainer(); err != nil {
					return err
				}
			}
			// If the container has active exec sessions, stop them now
			if len(ctr.state.ExecSessions) != 0 {
				if err := r.ociRuntime.execStopContainer(ctr, ctr.StopTimeout()); err != nil {
					return err
				}
			}
		}
	}

	// Start removing containers
	// We can remove containers even if they have dependencies now
	// As we have guaranteed their dependencies are in the pod
	for _, ctr := range ctrs {
		// Clean up network namespace, cgroups, mounts
		if err := ctr.cleanup(ctx); err != nil {
			return err
		}

		// Stop container's storage
		if err := ctr.teardownStorage(); err != nil {
			return err
		}

		// Delete the container from runtime (only if we are not
		// ContainerStateConfigured)
		if ctr.state.State != ContainerStateConfigured &&
			ctr.state.State != ContainerStateExited {
			if err := ctr.delete(ctx); err != nil {
				return err
			}
		}

		// Free the container's lock
		if err := ctr.lock.Free(); err != nil {
			return err
		}
	}

	// Remove containers from the state
	if err := r.state.RemovePodContainers(p); err != nil {
		return err
	}

	// Mark containers invalid
	for _, ctr := range ctrs {
		ctr.valid = false
	}

	// Remove pod cgroup, if present
	if p.state.CgroupPath != "" {
		logrus.Debugf("Removing pod cgroup %s", p.state.CgroupPath)

		switch p.runtime.config.CgroupManager {
		case SystemdCgroupsManager:
			if err := deleteSystemdCgroup(p.state.CgroupPath); err != nil {
				// The pod is already almost gone.
				// No point in hard-failing if we fail
				// this bit of cleanup.
				logrus.Errorf("Error deleting pod %s cgroup %s: %v", p.ID(), p.state.CgroupPath, err)
			}
		case CgroupfsCgroupsManager:
			// Delete the cgroupfs cgroup
			// Make sure the conmon cgroup is deleted first
			// Since the pod is almost gone, don't bother failing
			// hard - instead, just log errors.
			v1CGroups := GetV1CGroups(getExcludedCGroups())
			conmonCgroupPath := filepath.Join(p.state.CgroupPath, "conmon")
			conmonCgroup, err := cgroups.Load(v1CGroups, cgroups.StaticPath(conmonCgroupPath))
			if err != nil && err != cgroups.ErrCgroupDeleted {
				return err
			}
			if err == nil {
				if err := conmonCgroup.Delete(); err != nil {
					logrus.Errorf("Error deleting pod %s conmon cgroup %s: %v", p.ID(), conmonCgroupPath, err)
				}
			}
			cgroup, err := cgroups.Load(v1CGroups, cgroups.StaticPath(p.state.CgroupPath))
			if err != nil && err != cgroups.ErrCgroupDeleted {
				return err
			}
			if err == nil {
				if err := cgroup.Delete(); err != nil {
					logrus.Errorf("Error deleting pod %s cgroup %s: %v", p.ID(), p.state.CgroupPath, err)
				}
			}
		default:
			return errors.Wrapf(ErrInvalidArg, "unknown cgroups manager %s specified", p.runtime.config.CgroupManager)
		}
	}

	// Remove pod from state
	if err := r.state.RemovePod(p); err != nil {
		return err
	}

	// Mark pod invalid
	p.valid = false

	return nil
}
