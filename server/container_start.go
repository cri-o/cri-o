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
)

// StartContainer starts the container.
func (s *Server) StartContainer(ctx context.Context, req *types.StartContainerRequest) (res *types.StartContainerResponse, retErr error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	log.Infof(ctx, "Starting container: %s", req.GetContainerId())

	c, err := s.GetContainerFromShortID(ctx, req.GetContainerId())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "could not find container %q: %v", req.GetContainerId(), err)
	}

	state := c.State()
	if state.Status != oci.ContainerStateCreated {
		return nil, fmt.Errorf("container %s is not in created state: %s", c.ID(), state.Status)
	}

	sandbox := s.getSandbox(ctx, c.Sandbox())

	hooks := s.hooksRetriever.Get(ctx, sandbox.RuntimeHandler(), sandbox.Annotations())

	if err := s.nri.startContainer(ctx, sandbox, c); err != nil {
		log.Warnf(ctx, "NRI start failed for container %q: %v", c.ID(), err)
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

			if err := s.nri.stopContainer(ctx, sandbox, c, false); err != nil {
				log.Warnf(ctx, "NRI stop failed for container %q: %v", c.ID(), err)
			}

			if err := s.removeContainerInPod(ctx, sandbox, c); err != nil {
				log.Warnf(ctx, "Failed to delete container in runtime %s: %v", c.ID(), err)
			}
		}

		if err := s.ContainerStateToDisk(ctx, c); err != nil {
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
		if _, err := s.ContainerRestore(ctx, &metadata.ContainerConfig{ID: c.ID()}, &lib.ContainerCheckpointOptions{}); err != nil {
			return nil, err
		}

		c.SetRestore(false)
		c.SetRestoreArchivePath("")
		c.SetRestoreStorageImageID(nil)

		if err := s.ContainerStateToDisk(ctx, c); err != nil {
			return nil, fmt.Errorf("persist completed restore for container %s: %w", c.ID(), err)
		}

		if restoreArchive != "" {
			if err := os.Remove(restoreArchive); err != nil && !os.IsNotExist(err) {
				log.Warnf(ctx, "Unable to remove consumed restore archive %q: %v", restoreArchive, err)
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
