package server

import (
	"context"
	"fmt"
	"os"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/podoperation"
)

// StartContainer starts a container under any existing Pod restore owner.
func (s *Server) StartContainer(
	ctx context.Context,
	request *types.StartContainerRequest,
) (*types.StartContainerResponse, error) {
	return s.startContainerWithPodOperation(ctx, request)
}

// StartContainer starts the container.
func (s *Server) startContainer(
	ctx context.Context,
	req *types.StartContainerRequest,
) (res *types.StartContainerResponse, retErr error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	log.Infof(ctx, "Starting container: %s", req.GetContainerId())

	c, err := s.GetContainerFromShortID(ctx, req.GetContainerId())
	if err != nil {
		return nil, status.Errorf(
			codes.NotFound,
			"could not find container %q: %v",
			req.GetContainerId(),
			err,
		)
	}

	state := c.State()

	// A container created from a checkpoint archive has no OCI task, and
	// therefore no runtime status, until the restore below creates it.
	pendingRestore := c.Restore() && state.Status == ""
	if state.Status != oci.ContainerStateCreated && !pendingRestore {
		return nil, fmt.Errorf("container %s is not in created state: %s", c.ID(), state.Status)
	}

	if err := s.requirePodRestoreCommitted(c); err != nil {
		return nil, err
	}

	sandbox := s.getSandbox(ctx, c.Sandbox())

	hooks := s.hooksRetriever.Get(ctx, sandbox.RuntimeHandler(), sandbox.Annotations())

	if err := s.nri.startContainer(ctx, sandbox, c); err != nil {
		log.Warnf(ctx, "NRI start failed for container %q: %v", c.ID(), err)
	}

	defer func() {
		// if the call to StartContainer fails below we still want to fill
		// some fields of a container status. Identified restores defer cleanup
		// to their settlement owner so storage cannot disappear under late work.
		// In particular, we're going to
		// adjust container started/finished time and set an error to be
		// returned in the Reason field for container status call.
		if retErr != nil && podoperation.ID(ctx) == "" {
			c.SetStartFailed(retErr)

			if hooks != nil {
				if err := hooks.PreStop(ctx, c, sandbox); err != nil {
					log.Warnf(ctx, "Failed to run pre-stop hook for container %q: %v", c.ID(), err)
				}
			}

			if err := s.nri.stopContainer(ctx, sandbox, c); err != nil {
				log.Warnf(ctx, "NRI stop failed for container %q: %v", c.ID(), err)
			}

			if err := s.removeContainerInPod(ctx, sandbox, c); err != nil {
				log.Warnf(ctx, "Failed to delete container in runtime %s: %v", c.ID(), err)
			}
		}

		if err := s.ContainerStateToDiskWithUpdate(ctx, c); err != nil {
			log.Warnf(ctx, "Unable to write containers %s state to disk: %v", c.ID(), err)
		}
	}()

	if hooks != nil {
		if err := hooks.PreStart(ctx, c, sandbox); err != nil {
			return nil, fmt.Errorf("failed to run pre-start hook for container %q: %w", c.ID(), err)
		}
	}

	if c.Restore() {
		log.Debugf(ctx, "Restoring container %q", req.GetContainerId())

		restoreArchive := c.RestoreArchivePath()
		if _, err := s.ContainerRestore(
			ctx,
			&metadata.ContainerConfig{ID: c.ID()},
			&lib.ContainerCheckpointOptions{},
		); err != nil {
			return nil, err
		}

		c.SetRestore(false)
		c.SetRestoreArchivePath("")
		c.SetRestoreStorageImageID(nil)
		c.SetRestored(true)

		// The process is running now, so a state write failure must not fail
		// the start. Keep the staged archive until the completed state is saved.
		stateErr := s.ContainerStateToDiskWithUpdate(ctx, c)
		if stateErr != nil {
			log.Warnf(ctx, "Unable to write containers %s state to disk: %v", c.ID(), stateErr)
		}

		// A single-container restore reads the caller's archive in place. Only
		// a Pod restore owns its staged copy.
		if stateErr == nil && c.PodRestoreKey() != "" && restoreArchive != "" {
			if err := os.Remove(restoreArchive); err != nil && !os.IsNotExist(err) {
				log.Warnf(
					ctx,
					"Unable to remove consumed restore archive %q: %v",
					restoreArchive,
					err,
				)
			}
		}

		log.Infof(ctx, "Restored container: %s", c.ID())
	} else if err := s.ContainerServer.Runtime().StartContainer(ctx, c); err != nil {
		return nil, fmt.Errorf("failed to start container %s: %w", c.ID(), err)
	}

	s.generateCRIEvent(ctx, c, types.ContainerEventType_CONTAINER_STARTED_EVENT)

	if err := s.nri.postStartContainer(ctx, sandbox, c); err != nil {
		log.Warnf(ctx, "NRI post-start failed for container %q: %v", c.ID(), err)
	}

	log.WithFields(ctx, map[string]any{
		"description": c.Description(),
		"containerID": c.ID(),
		"sandboxID":   sandbox.ID(),
		"PID":         state.Pid,
	}).Infof("Started container")

	return &types.StartContainerResponse{}, nil
}
