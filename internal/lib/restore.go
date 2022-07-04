package lib

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/checkpoint-restore/go-criu/v5/stats"
	"github.com/containers/podman/v4/pkg/checkpoint/crutils"
	"github.com/containers/storage/pkg/archive"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/sirupsen/logrus"
)

// ContainerRestore restores a checkpointed container.
func (c *ContainerServer) ContainerRestore(ctx context.Context, opts *ContainerCheckpointRestoreOptions) (string, error) {
	var ctr *oci.Container
	var err error
	ctr, err = c.LookupContainer(opts.Container)
	if err != nil {
		return "", fmt.Errorf("failed to find container %s: %w", opts.Container, err)
	}

	cStatus := ctr.State()
	if cStatus.Status == oci.ContainerStateRunning {
		return "", fmt.Errorf("cannot restore running container %s", ctr.ID())
	}

	// Get config.json
	configFile := filepath.Join(ctr.Dir(), "config.json")
	ctrSpec, err := generate.NewFromFile(configFile)
	if err != nil {
		return "", err
	}
	// During checkpointing the container is unmounted. This mounts the container again.
	mountPoint, err := c.StorageImageServer().GetStore().Mount(ctr.ID(), ctrSpec.Config.Linux.MountLabel)
	if err != nil {
		logrus.Debugf("Failed to mount container %q: %v", ctr.ID(), err)
		return "", err
	}
	logrus.Debugf("Container mountpoint %v", mountPoint)
	logrus.Debugf("Sandbox %v", ctr.Sandbox())
	logrus.Debugf("Specgen.Config.Annotations[io.kubernetes.cri-o.SandboxID] %v", ctrSpec.Config.Annotations["io.kubernetes.cri-o.SandboxID"])
	// If there was no podID specified this will restore the container
	// in its original sandbox
	if opts.Pod == "" {
		opts.Pod = ctr.Sandbox()
	}
	sb, err := c.LookupSandbox(opts.Pod)
	if err != nil {
		return "", err
	}
	ic := sb.InfraContainer()
	if ic == nil {
		return "", fmt.Errorf("infra container of sandbox %v not found", sb.Name())
	}
	infraConfigFile := filepath.Join(ic.BundlePath(), "config.json")
	specgen, err := generate.NewFromFile(infraConfigFile)
	if err != nil {
		return "", err
	}

	if ctr.RestoreArchive() != "" {
		if ctr.RestoreIsOCIImage() {
			logrus.Debugf("Restoring from %v", ctr.RestoreArchive())
			imageMountPoint, err := c.StorageImageServer().GetStore().MountImage(ctr.RestoreArchive(), nil, "")
			if err != nil {
				return "", err
			}
			logrus.Debugf("Checkpoint image mounted at %v", imageMountPoint)
			defer func() {
				_, err := c.StorageImageServer().GetStore().UnmountImage(ctr.RestoreArchive(), true)
				if err != nil {
					logrus.Errorf("Failed to unmount checkpoint image: %q", err)
				}
			}()

			// Import all checkpoint files except ConfigDumpFile and SpecDumpFile. We
			// generate new container config files to enable to specifying a new
			// container name.
			checkpoint := []string{
				"artifacts",
				metadata.CheckpointDirectory,
				metadata.DevShmCheckpointTar,
				metadata.RootFsDiffTar,
				metadata.DeletedFilesFile,
				metadata.PodOptionsFile,
				metadata.PodDumpFile,
			}
			for _, name := range checkpoint {
				src := filepath.Join(imageMountPoint, name)
				dst := filepath.Join(ctr.Dir(), name)
				if err := archive.NewDefaultArchiver().CopyWithTar(src, dst); err != nil {
					logrus.Debugf("Can't import '%s' from checkpoint image", name)
				}
			}
		} else {
			if err := crutils.CRImportCheckpointWithoutConfig(ctr.Dir(), ctr.RestoreArchive()); err != nil {
				return "", err
			}
		}
		if err := c.restoreFileSystemChanges(ctr, mountPoint); err != nil {
			return "", err
		}
		for _, m := range ctrSpec.Config.Mounts {
			// This checks if all bind mount sources exist.
			// We cannot create missing bind mount sources automatically
			// as the source and destination need to be of the same type.
			// CRIU will fail restoring if the external bind mount source
			// is a directory but the internal destination is a file.
			// As destinations can be in nested bind mounts, which are only
			// correctly setup by runc/crun during container restore, we
			// cannot figure out the file type of the destination.
			// At this point we will fail and tell the user to create
			// the missing bind mount source file/directory.
			if m.Type != "bind" {
				continue
			}
			_, err := os.Lstat(m.Source)
			if err != nil {
				return "", fmt.Errorf(
					"the bind mount source %s is missing. %s",
					m.Source,
					"Please create the corresponding file or directory",
				)
			}
		}
	}

	if err := c.runtime.RestoreContainer(ctx, ctr, specgen.Config, ic.State().Pid, sb.CgroupParent()); err != nil {
		return "", fmt.Errorf("failed to restore container %s: %w", ctr.ID(), err)
	}
	if err := c.ContainerStateToDisk(ctx, ctr); err != nil {
		logrus.Warnf("Unable to write containers %s state to disk: %v", ctr.ID(), err)
	}

	if !opts.Keep {
		// Delete all checkpoint related files. At this point, in theory, all files
		// should exist. Still ignoring errors for now as the container should be
		// restored and running. Not erroring out just because some cleanup operation
		// failed. Starting with the checkpoint directory
		err = os.RemoveAll(ctr.CheckpointPath())
		if err != nil {
			logrus.Debugf("Non-fatal: removal of checkpoint directory (%s) failed: %v", ctr.CheckpointPath(), err)
		}
		cleanup := [...]string{
			metadata.RestoreLogFile,
			metadata.DumpLogFile,
			stats.StatsDump,
			stats.StatsRestore,
			metadata.NetworkStatusFile,
			metadata.RootFsDiffTar,
			metadata.DeletedFilesFile,
		}
		for _, del := range cleanup {
			var file string
			if del == metadata.RestoreLogFile || del == stats.StatsRestore {
				// Checkpointing uses runc and it is possible to tell runc
				// the location of the log file using '--work-path'.
				// Restore goes through conmon and conmon does (not yet?)
				// expose runc's '--work-path' which means that temporary
				// restore files are put into BundlePath().
				file = filepath.Join(ctr.BundlePath(), del)
			} else {
				file = filepath.Join(ctr.Dir(), del)
			}
			err = os.Remove(file)
			if err != nil {
				logrus.Debugf("Non-fatal: removal of checkpoint file (%s) failed: %v", file, err)
			}
		}
	}

	return ctr.ID(), nil
}

func (c *ContainerServer) restoreFileSystemChanges(ctr *oci.Container, mountPoint string) error {
	if err := crutils.CRApplyRootFsDiffTar(ctr.Dir(), mountPoint); err != nil {
		return err
	}

	if err := crutils.CRRemoveDeletedFiles(ctr.ID(), ctr.Dir(), mountPoint); err != nil {
		return err
	}
	return nil
}
