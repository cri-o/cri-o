package lib

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/checkpoint-restore/go-criu/v7/stats"
	"github.com/containers/common/pkg/crutils"
	"github.com/containers/storage/pkg/archive"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/pkg/annotations"
)

// ContainerCheckpointOptions is the relevant subset of libpod.ContainerCheckpointOptions.
type ContainerCheckpointOptions struct {
	// Keep tells the API to not delete checkpoint artifacts
	Keep bool
	// KeepRunning tells the API to keep the container running
	// after writing the checkpoint to disk
	KeepRunning bool
	// TargetFile tells the API to read (or write) the checkpoint image
	// from (or to) the filename set in TargetFile
	TargetFile string
}

// ContainerCheckpoint checkpoints a running container.
func (c *ContainerServer) ContainerCheckpoint(
	ctx context.Context,
	config *metadata.ContainerConfig,
	opts *ContainerCheckpointOptions,
) (string, error) {
	ctr, err := c.LookupContainer(ctx, config.ID)
	if err != nil {
		return "", fmt.Errorf("failed to find container %s: %w", config.ID, err)
	}

	configFile := filepath.Join(ctr.BundlePath(), "config.json")

	specgen, err := generate.NewFromFile(configFile)
	if err != nil {
		return "", fmt.Errorf("not able to read config for container %q: %w", ctr.ID(), err)
	}

	cStatus := ctr.State()
	if cStatus.Status != oci.ContainerStateRunning {
		return "", fmt.Errorf("container %s is not running", ctr.ID())
	}

	// At this point the container needs to be paused. As we first checkpoint
	// the processes in the container and the container will continue to run
	// after checkpointing, there is a chance that the changed files we include
	// in the checkpoint archive might change by the now again running processes
	// in the container.
	// Assuming this uses runc/crun (the OCI runtime is currently the only one
	// supporting checkpointing), PauseContainer() will use the cgroup freezer
	// to freeze the processes. CRIU will also use the cgroup freezer to freeze
	// the processes if possible. If the cgroup is already frozen by runc/crun
	// CRIU will not change the freezer status.
	if err = c.runtime.PauseContainer(ctx, ctr); err != nil {
		return "", fmt.Errorf("failed to pause container %q before checkpointing: %w", ctr.ID(), err)
	}

	defer func() {
		if err := c.runtime.UpdateContainerStatus(ctx, ctr); err != nil {
			log.Errorf(ctx, "Failed to update container status: %q: %v", ctr.ID(), err)
		}

		if ctr.State().Status == oci.ContainerStatePaused {
			err := c.runtime.UnpauseContainer(ctx, ctr)
			if err != nil {
				log.Errorf(ctx, "Failed to unpause container: %q: %v", ctr.ID(), err)
			}
		}
		// container state needs to be written _after_ unpausing
		if err = c.ContainerStateToDisk(ctx, ctr); err != nil {
			log.Warnf(ctx, "Unable to write containers %s state to disk: %v", ctr.ID(), err)
		}
	}()

	if opts.TargetFile != "" {
		if err := c.prepareCheckpointExport(ctr); err != nil {
			return "", fmt.Errorf("failed to write config dumps for container %s: %w", ctr.ID(), err)
		}
	}

	if err := c.runtime.CheckpointContainer(ctx, ctr, specgen.Config, opts.KeepRunning); err != nil {
		return "", fmt.Errorf("failed to checkpoint container %s: %w", ctr.ID(), err)
	}

	if opts.TargetFile != "" {
		if err := c.exportCheckpoint(ctx, ctr, specgen.Config, opts.TargetFile); err != nil {
			return "", fmt.Errorf("failed to write file system changes of container %s: %w", ctr.ID(), err)
		}

		defer func() {
			// clean up checkpoint directory
			if err := os.RemoveAll(ctr.CheckpointPath()); err != nil {
				log.Warnf(ctx, "Unable to remove checkpoint directory %s: %v", ctr.CheckpointPath(), err)
			}
		}()
	}

	if !opts.KeepRunning {
		if err := c.storageRuntimeServer.StopContainer(ctx, ctr.ID()); err != nil {
			return "", fmt.Errorf("failed to unmount container %s: %w", ctr.ID(), err)
		}
	}

	if !opts.Keep {
		cleanup := []string{
			metadata.DumpLogFile,
			stats.StatsDump,
			metadata.ConfigDumpFile,
			metadata.SpecDumpFile,
		}
		for _, del := range cleanup {
			file := filepath.Join(ctr.Dir(), del)
			if err := os.Remove(file); err != nil {
				log.Debugf(ctx, "Unable to remove file %s", file)
			}
		}
	}

	return ctr.ID(), nil
}

// Copied from libpod/diff.go.
var containerMounts = map[string]bool{
	"/dev":               true,
	"/dev/shm":           true,
	"/proc":              true,
	"/run":               true,
	"/run/.containerenv": true,
	"/run/secrets":       true,
	"/sys":               true,
}

const bindMount = "bind"

func skipBindMount(mountPath string, specgen *rspec.Spec) bool {
	for _, m := range specgen.Mounts {
		if m.Type != bindMount {
			continue
		}

		if m.Destination == mountPath {
			return true
		}
	}

	return false
}

// getDiff returns the file system differences
// Copied from libpod/diff.go and simplified for the checkpoint use case.
func (c *ContainerServer) getDiff(ctx context.Context, id string, specgen *rspec.Spec) (rchanges []archive.Change, err error) {
	layerID, err := c.GetContainerTopLayerID(ctx, id)
	if err != nil {
		return nil, err
	}

	changes, err := c.store.Changes("", layerID)
	if err == nil {
		for _, c := range changes {
			if skipBindMount(c.Path, specgen) {
				continue
			}

			if containerMounts[c.Path] {
				continue
			}

			rchanges = append(rchanges, c)
		}
	}

	return rchanges, err
}

type ExternalBindMount struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	FileType    string `json:"file_type"`
	Permissions uint32 `json:"permissions"`
}

// prepareCheckpointExport writes the config and spec to
// JSON files for later export
// Podman: libpod/container_internal.go.
func (c *ContainerServer) prepareCheckpointExport(ctr *oci.Container) error {
	// save spec
	jsonPath := filepath.Join(ctr.BundlePath(), "config.json")

	g, err := generate.NewFromFile(jsonPath)
	if err != nil {
		return fmt.Errorf("generating spec for container %q failed: %w", ctr.ID(), err)
	}

	if _, err := metadata.WriteJSONFile(g.Config, ctr.Dir(), metadata.SpecDumpFile); err != nil {
		return fmt.Errorf("generating spec for container %q failed: %w", ctr.ID(), err)
	}

	rootFSImageRef := ""
	if id := ctr.ImageID(); id != nil {
		rootFSImageRef = id.IDStringForOutOfProcessConsumptionOnly()
	}

	rootFSImageName := ""
	if someNameOfTheImage := ctr.SomeNameOfTheImage(); someNameOfTheImage != nil {
		rootFSImageName = someNameOfTheImage.StringForOutOfProcessConsumptionOnly()
	}

	config := &metadata.ContainerConfig{
		ID:              ctr.ID(),
		Name:            ctr.Name(),
		RootfsImage:     ctr.UserRequestedImage(),
		RootfsImageRef:  rootFSImageRef,
		RootfsImageName: rootFSImageName,
		CreatedTime:     ctr.CreatedAt(),
		OCIRuntime: func() string {
			runtimeHandler := c.GetSandbox(ctr.Sandbox()).RuntimeHandler()
			if runtimeHandler != "" {
				return runtimeHandler
			}

			return c.config.DefaultRuntime
		}(),
		CheckpointedAt: time.Now(),
		Restored:       ctr.Restore(),
	}

	if _, err := metadata.WriteJSONFile(config, ctr.Dir(), metadata.ConfigDumpFile); err != nil {
		return err
	}

	// During container creation CRI-O creates all missing bind mount sources as
	// directories. This is disabled during restore as CRIU requires the bind mount
	// source to be of the same type. Directories need to be directories and regular
	// files need to be regular files. CRIU will fail to bind mount a directory on
	// a file. Especially when restoring a Kubernetes container outside of Kubernetes
	// a couple of bind mounts are files (e.g. /etc/resolv.conf). To solve this
	// CRI-O is now tracking all bind mount types in the checkpoint archive. This
	// way it is possible to know if a missing bind mount needs to be a file or a
	// directory.
	var externalBindMounts []ExternalBindMount //nolint:prealloc

	for _, m := range g.Config.Mounts {
		if containerMounts[m.Destination] {
			continue
		}

		if m.Type != bindMount {
			continue
		}

		fileInfo, err := os.Stat(m.Source)
		if err != nil {
			return fmt.Errorf("unable to stat() %q: %w", m.Source, err)
		}

		externalBindMounts = append(
			externalBindMounts,
			ExternalBindMount{
				Source:      m.Source,
				Destination: m.Destination,
				FileType: func() string {
					if fileInfo.Mode().IsDir() {
						return "directory"
					}

					return "file"
				}(),
				Permissions: uint32(fileInfo.Mode().Perm()),
			},
		)
	}

	if len(externalBindMounts) > 0 {
		if _, err := metadata.WriteJSONFile(externalBindMounts, ctr.Dir(), "bind.mounts"); err != nil {
			return fmt.Errorf("error writing 'bind.mounts' for %q: %w", ctr.ID(), err)
		}
	}

	return nil
}

func (c *ContainerServer) exportCheckpoint(ctx context.Context, ctr *oci.Container, specgen *rspec.Spec, export string) error {
	id := ctr.ID()
	dest := ctr.Dir()
	log.Debugf(ctx, "Exporting checkpoint image of container %q to %q", id, dest)

	includeFiles := []string{
		stats.StatsDump,
		metadata.DumpLogFile,
		metadata.CheckpointDirectory,
		metadata.ConfigDumpFile,
		metadata.SpecDumpFile,
		"bind.mounts",
	}

	// To correctly track deleted files, let's go through the output of 'podman diff'
	rootFsChanges, err := c.getDiff(ctx, id, specgen)
	if err != nil {
		return fmt.Errorf("error exporting root file-system diff for %q: %w", id, err)
	}

	mountPoint, err := c.StorageImageServer().GetStore().Mount(id, specgen.Linux.MountLabel)
	if err != nil {
		return fmt.Errorf("not able to get mountpoint for container %q: %w", id, err)
	}

	addToTarFiles, err := crutils.CRCreateRootFsDiffTar(&rootFsChanges, mountPoint, dest)
	if err != nil {
		return err
	}

	// Put log file into checkpoint archive
	_, err = os.Stat(specgen.Annotations[annotations.LogPath])
	if err == nil {
		src, err := os.Open(specgen.Annotations[annotations.LogPath])
		if err != nil {
			return fmt.Errorf("error opening log file %q: %w", specgen.Annotations[annotations.LogPath], err)
		}

		defer src.Close()

		destLogPath := filepath.Join(dest, annotations.LogPath)

		destLog, err := os.Create(destLogPath)
		if err != nil {
			return fmt.Errorf("error opening log file %q: %w", destLogPath, err)
		}

		defer destLog.Close()

		_, err = io.Copy(destLog, src)
		if err != nil {
			return fmt.Errorf("copying log file to %q failed: %w", destLogPath, err)
		}

		addToTarFiles = append(addToTarFiles, annotations.LogPath)
	}

	includeFiles = append(includeFiles, addToTarFiles...)

	input, err := archive.TarWithOptions(ctr.Dir(), &archive.TarOptions{
		// This should be configurable via api.proti
		Compression:      archive.Uncompressed,
		IncludeSourceDir: true,
		IncludeFiles:     includeFiles,
	})
	if err != nil {
		return fmt.Errorf("error reading checkpoint directory %q: %w", id, err)
	}

	// The resulting tar archive should not be readable by everyone as it contains
	// every memory page of the checkpointed processes.
	outFile, err := os.OpenFile(export, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("error creating checkpoint export file %q: %w", export, err)
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, input)
	if err != nil {
		return err
	}

	for _, file := range addToTarFiles {
		os.Remove(filepath.Join(dest, file))
	}

	return nil
}
