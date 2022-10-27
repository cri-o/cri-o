package server

import (
	"fmt"

	"github.com/containers/podman/v4/libpod"
	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/runtimehandlerhooks"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// StartContainer starts the container.
func (s *Server) StartContainer(ctx context.Context, req *types.StartContainerRequest) (retErr error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	log.Infof(ctx, "Starting container: %s", req.ContainerId)
	c, err := s.GetContainerFromShortID(ctx, req.ContainerId)
	if err != nil {
		return status.Errorf(codes.NotFound, "could not find container %q: %v", req.ContainerId, err)
	}

	if c.Restore() {
		// If the create command found a checkpoint image, the container
		// has the restore flag set to true. At this point we need to jump
		// into the restore code.
		log.Debugf(ctx, "Restoring container %q", req.ContainerId)

		ctr, err := s.ContainerServer.ContainerRestore(
			ctx,
			&lib.ContainerCheckpointRestoreOptions{
				Container: c.ID(),
				Pod:       s.getSandbox(ctx, c.Sandbox()).ID(),
				ContainerCheckpointOptions: libpod.ContainerCheckpointOptions{
					TargetFile: c.ImageName(),
				},
			},
		)
		if err != nil {
			ociContainer, err1 := s.GetContainerFromShortID(ctx, c.ID())
			if err1 != nil {
				return fmt.Errorf("failed to find container %s: %v", c.ID(), err1)
			}
			s.ReleaseContainerName(ctx, ociContainer.Name())
			err2 := s.StorageRuntimeServer().DeleteContainer(ctx, c.ID())
			if err2 != nil {
				log.Warnf(ctx, "Failed to cleanup container directory: %v", err2)
			}
			s.removeContainer(ctx, ociContainer)
			return err
		}

		log.Infof(ctx, "Restored container: %s", ctr)
		return nil
	}

	state := c.State()
	if state.Status != oci.ContainerStateCreated {
		return fmt.Errorf("container %s is not in created state: %s", c.ID(), state.Status)
	}

	sandbox := s.getSandbox(ctx, c.Sandbox())
	hooks, err := runtimehandlerhooks.GetRuntimeHandlerHooks(ctx, &s.config, sandbox.RuntimeHandler(), sandbox.Annotations())
	if err != nil {
		return fmt.Errorf("failed to get runtime handler %q hooks", sandbox.RuntimeHandler())
	}

	defer func() {
		// if the call to StartContainer fails below we still want to fill
		// some fields of a container status. In particular, we're going to
		// adjust container started/finished time and set an error to be
		// returned in the Reason field for container status call.
		if retErr != nil {
			c.SetStartFailed(retErr)
			if hooks != nil {
				if err := hooks.PreStop(ctx, c, sandbox); err != nil {
					log.Warnf(ctx, "Failed to run pre-stop hook for container %q: %v", c.ID(), err)
				}
			}
		}
		if err := s.ContainerStateToDisk(ctx, c); err != nil {
			log.Warnf(ctx, "Unable to write containers %s state to disk: %v", c.ID(), err)
		}
	}()

	if hooks != nil {
		if err := hooks.PreStart(ctx, c, sandbox); err != nil {
			return fmt.Errorf("failed to run pre-start hook for container %q: %w", c.ID(), err)
		}
	}

	if err := s.Runtime().StartContainer(ctx, c); err != nil {
		return fmt.Errorf("failed to start container %s: %w", c.ID(), err)
	}

	log.WithFields(ctx, map[string]interface{}{
		"description": c.Description(),
		"containerID": c.ID(),
		"sandboxID":   sandbox.ID(),
		"PID":         state.Pid,
	}).Infof("Started container")
	return nil
}
