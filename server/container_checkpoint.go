package server

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/containers/podman/v4/libpod"
	"github.com/containers/storage/pkg/archive"
	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/log"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// CheckpointContainer checkpoints a container
func (s *Server) CheckpointContainer(ctx context.Context, req *types.CheckpointContainerRequest) error {
	if !s.config.RuntimeConfig.CheckpointRestore() {
		return fmt.Errorf("checkpoint/restore support not available")
	}

	var opts []*lib.ContainerCheckpointRestoreOptions
	var podCheckpointDirectory string
	var checkpointedPodOptions metadata.CheckpointedPodOptions

	_, err := s.GetContainerFromShortID(ctx, req.ContainerId)
	if err != nil {
		// Maybe the user specified a Pod
		sb, err := s.LookupSandbox(req.ContainerId)
		if err != nil {
			return status.Errorf(codes.NotFound, "could not find container or pod %q: %v", req.ContainerId, err)
		}
		if req.Location == "" {
			return status.Errorf(codes.NotFound, "Pod checkpointing requires a destination file")
		}

		log.Infof(ctx, "Checkpointing pod: %s", req.ContainerId)
		// Create a temporary directory
		podCheckpointDirectory, err = os.MkdirTemp("", "checkpoint")
		if err != nil {
			return err
		}
		sandboxConfig := types.PodSandboxConfig{
			Metadata: &types.PodSandboxMetadata{
				Name:      sb.Metadata().Name,
				Uid:       sb.Metadata().Uid,
				Namespace: sb.Metadata().Namespace,
				Attempt:   sb.Metadata().Attempt,
			},
			Hostname:     sb.Hostname(),
			LogDirectory: sb.LogDir(),
		}
		var portMappings []*types.PortMapping
		maps := sb.PortMappings()
		for _, portMap := range maps {
			pm := &types.PortMapping{
				ContainerPort: portMap.ContainerPort,
				HostPort:      portMap.HostPort,
				HostIp:        portMap.HostIP,
			}
			switch portMap.Protocol {
			case "TCP":
				pm.Protocol = 0
			case "UDP":
				pm.Protocol = 1
			case "SCTP":
				pm.Protocol = 2
			}

			portMappings = append(portMappings, pm)
		}
		sandboxConfig.PortMappings = portMappings
		if sb.DNSConfig() != nil {
			dnsConfig := &types.DNSConfig{
				Servers:  sb.DNSConfig().Servers,
				Searches: sb.DNSConfig().Searches,
				Options:  sb.DNSConfig().Options,
			}
			sandboxConfig.DnsConfig = dnsConfig
		}
		if _, err := metadata.WriteJSONFile(sandboxConfig, podCheckpointDirectory, metadata.PodDumpFile); err != nil {
			return err
		}
		defer func() {
			if err := os.RemoveAll(podCheckpointDirectory); err != nil {
				log.Errorf(ctx, "Could not recursively remove %s: %q", podCheckpointDirectory, err)
			}
		}()

		for _, ctr := range sb.Containers().List() {
			localOpts := &lib.ContainerCheckpointRestoreOptions{
				Container: ctr.ID(),
				ContainerCheckpointOptions: libpod.ContainerCheckpointOptions{
					TargetFile:  filepath.Join(podCheckpointDirectory, ctr.Name()+".tar"),
					KeepRunning: true,
				},
			}
			opts = append(opts, localOpts)
			// This should be ID
			checkpointedPodOptions.Containers = append(checkpointedPodOptions.Containers, ctr.Name())
		}
		if len(opts) == 0 {
			return status.Errorf(codes.NotFound, "No containers found in Pod %q", req.ContainerId)
		}
		checkpointedPodOptions.Version = 1
		checkpointedPodOptions.MountLabel = sb.MountLabel()
		checkpointedPodOptions.ProcessLabel = sb.ProcessLabel()
	} else {
		log.Infof(ctx, "Checkpointing container: %s", req.ContainerId)
		localOpts := &lib.ContainerCheckpointRestoreOptions{
			Container: req.ContainerId,
			ContainerCheckpointOptions: libpod.ContainerCheckpointOptions{
				TargetFile: req.Location,
				// For the forensic container checkpointing use case we
				// keep the container running after checkpointing it.
				KeepRunning: true,
			},
		}
		opts = append(opts, localOpts)
	}

	for _, opt := range opts {
		_, err = s.ContainerServer.ContainerCheckpoint(ctx, opt)
		if err != nil {
			return err
		}
	}

	if podCheckpointDirectory != "" {
		if podOptions, err := metadata.WriteJSONFile(checkpointedPodOptions, podCheckpointDirectory, metadata.PodOptionsFile); err != nil {
			return fmt.Errorf("error creating checkpointedContainers list file %q: %w", podOptions, err)
		}
		// It is a Pod checkpoint. Create the archive
		podTar, err := archive.TarWithOptions(podCheckpointDirectory, &archive.TarOptions{
			IncludeSourceDir: true,
		})
		if err != nil {
			return err
		}
		// The resulting tar archive should not readable by everyone as it contains
		// every memory page of the checkpointed processes.
		podTarFile, err := os.OpenFile(req.Location, os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			return fmt.Errorf("error creating pod checkpoint archive %q: %w", req.Location, err)
		}
		defer podTarFile.Close()
		_, err = io.Copy(podTarFile, podTar)
		if err != nil {
			return fmt.Errorf("failed writing to pod tar archive %q: %w", req.Location, err)
		}
		log.Infof(ctx, "Checkpointed pod: %s", req.ContainerId)
	} else {
		log.Infof(ctx, "Checkpointed container: %s", req.ContainerId)
	}

	return nil
}
