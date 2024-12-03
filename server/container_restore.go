package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/containers/podman/v4/pkg/annotations"
	"github.com/containers/podman/v4/pkg/errorhandling"
	"github.com/containers/storage/pkg/archive"
	"github.com/cri-o/cri-o/internal/factory/container"
	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	kubetypes "k8s.io/kubernetes/pkg/kubelet/types"
)

// taken from Podman
func (s *Server) CRImportCheckpoint(
	ctx context.Context,
	createConfig *types.ContainerConfig,
	sbID, sandboxUID string,
) (ctrID string, retErr error) {
	// Ensure that the image to restore the checkpoint from has been provided.
	if createConfig.Image == nil || createConfig.Image.Image == "" {
		return "", errors.New(`attribute "image" missing from container definition`)
	}

	if createConfig.Metadata == nil || createConfig.Metadata.Name == "" {
		return "", errors.New(`attribute "metadata" missing from container definition`)
	}

	inputImage := createConfig.Image.Image
	createMounts := createConfig.Mounts
	createAnnotations := createConfig.Annotations
	createLabels := createConfig.Labels

	// First get the container definition from the
	// tarball to a temporary directory
	archiveFile, err := os.Open(inputImage)
	if err != nil {
		return "", fmt.Errorf("failed to open checkpoint archive %s for import: %w", inputImage, err)
	}
	defer errorhandling.CloseQuiet(archiveFile)
	options := &archive.TarOptions{
		// Here we only need the files config.dump and spec.dump
		ExcludePatterns: []string{
			"artifacts",
			"ctr.log",
			metadata.RootFsDiffTar,
			metadata.NetworkStatusFile,
			metadata.DeletedFilesFile,
			metadata.CheckpointDirectory,
		},
	}
	dir, err := os.MkdirTemp("", "checkpoint")
	if err != nil {
		return "", err
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			logrus.Errorf("Could not recursively remove %s: %q", dir, err)
		}
	}()
	err = archive.Untar(archiveFile, dir, options)
	if err != nil {
		return "", fmt.Errorf("unpacking of checkpoint archive %s failed: %w", inputImage, err)
	}
	logrus.Debugf("Unpacked checkpoint in %s", dir)

	// Load spec.dump from temporary directory
	dumpSpec := new(spec.Spec)
	if _, err := metadata.ReadJSONFile(dumpSpec, dir, metadata.SpecDumpFile); err != nil {
		return "", fmt.Errorf("failed to read %q: %w", metadata.SpecDumpFile, err)
	}

	// Load config.dump from temporary directory
	config := new(lib.ContainerConfig)
	if _, err := metadata.ReadJSONFile(config, dir, metadata.ConfigDumpFile); err != nil {
		return "", fmt.Errorf("failed to read %q: %w", metadata.ConfigDumpFile, err)
	}

	if sbID == "" {
		// restore into previous sandbox
		sbID = dumpSpec.Annotations[annotations.SandboxID]
		ctrID = config.ID
	} else {
		ctrID = ""
	}

	originalAnnotations := make(map[string]string)

	if err := json.Unmarshal([]byte(dumpSpec.Annotations[annotations.Annotations]), &originalAnnotations); err != nil {
		return "", fmt.Errorf("failed to read %q: %w", annotations.Annotations, err)
	}

	if sandboxUID != "" {
		if _, ok := originalAnnotations[kubetypes.KubernetesPodUIDLabel]; ok {
			originalAnnotations[kubetypes.KubernetesPodUIDLabel] = sandboxUID
		}
	}

	if createAnnotations != nil {
		// The hash also needs to be update or Kubernetes thinks the container needs to be restarted
		_, ok1 := createAnnotations["io.kubernetes.container.hash"]
		_, ok2 := originalAnnotations["io.kubernetes.container.hash"]

		if ok1 && ok2 {
			originalAnnotations["io.kubernetes.container.hash"] = createAnnotations["io.kubernetes.container.hash"]
		}
	}

	sb, err := s.getPodSandboxFromRequest(sbID)
	if err != nil {
		if err == sandbox.ErrIDEmpty {
			return "", err
		}
		return "", fmt.Errorf("specified sandbox not found: %s: %w", sbID, err)
	}

	stopMutex := sb.StopMutex()
	stopMutex.RLock()
	defer stopMutex.RUnlock()
	if sb.Stopped() {
		return "", fmt.Errorf("CreateContainer failed as the sandbox was stopped: %s", sb.ID())
	}

	ctr, err := container.New()
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	containerConfig := &types.ContainerConfig{
		Metadata: &types.ContainerMetadata{
			Name:    createConfig.Metadata.Name,
			Attempt: createConfig.Metadata.Attempt,
		},
		Image: &types.ImageSpec{Image: config.RootfsImageName},
		Linux: &types.LinuxContainerConfig{
			Resources:       &types.LinuxContainerResources{},
			SecurityContext: &types.LinuxContainerSecurityContext{},
		},
		Annotations: originalAnnotations,
		// The labels are nod changed or adapted. They are just taken from the CRI
		// request without any modification (in contrast to the annotations).
		Labels: createLabels,
	}

	if createConfig.Linux != nil {
		if createConfig.Linux.Resources != nil {
			containerConfig.Linux.Resources = createConfig.Linux.Resources
		}

		if createConfig.Linux.SecurityContext != nil {
			containerConfig.Linux.SecurityContext = createConfig.Linux.SecurityContext
		}
	}

	ignoreMounts := map[string]bool{
		"/proc":              true,
		"/dev":               true,
		"/dev/pts":           true,
		"/dev/mqueue":        true,
		"/sys":               true,
		"/sys/fs/cgroup":     true,
		"/dev/shm":           true,
		"/etc/resolv.conf":   true,
		"/etc/hostname":      true,
		"/run/secrets":       true,
		"/run/.containerenv": true,
	}

	// It is necessary to ensure that all bind mounts in the checkpoint archive are defined
	// in the create container requested coming in via the CRI. If this check would not
	// be here it would be possible to create a checkpoint archive that mounts some random
	// file/directory on the host with the user knowing as it will happen without specifying
	// it in the container definition.
	missingMount := []string{}

	for _, m := range dumpSpec.Mounts {
		// Following mounts are ignored as they might point to the
		// wrong location and if ignored the mounts will correctly
		// be setup to point to the new location.
		if ignoreMounts[m.Destination] {
			continue
		}
		mount := &types.Mount{
			ContainerPath: m.Destination,
		}

		bindMountFound := false
		for _, createMount := range createMounts {
			if createMount.ContainerPath != m.Destination {
				continue
			}

			bindMountFound = true
			mount.HostPath = createMount.HostPath
			mount.Readonly = createMount.Readonly
			mount.Propagation = createMount.Propagation
			break
		}

		if !bindMountFound {
			missingMount = append(missingMount, m.Destination)
			// If one mount is missing we can skip over any further code as we have
			// to abort the restore process anyway. Not using break to get all missing
			// mountpoints in one error message.
			continue
		}

		logrus.Debugf("Adding mounts %#v", mount)
		containerConfig.Mounts = append(containerConfig.Mounts, mount)
	}
	if len(missingMount) > 0 {
		return "", fmt.Errorf(
			"restoring %q expects following bind mounts defined (%s)",
			inputImage,
			strings.Join(missingMount, ","),
		)
	}

	sandboxConfig := &types.PodSandboxConfig{
		Metadata: &types.PodSandboxMetadata{
			Name:      sb.Metadata().Name,
			Uid:       sb.Metadata().Uid,
			Namespace: sb.Metadata().Namespace,
			Attempt:   sb.Metadata().Attempt,
		},
		Linux: &types.LinuxPodSandboxConfig{},
	}

	if err := ctr.SetConfig(containerConfig, sandboxConfig); err != nil {
		return "", fmt.Errorf("setting container config: %w", err)
	}

	if err := ctr.SetNameAndID(ctrID); err != nil {
		return "", fmt.Errorf("setting container name and ID: %w", err)
	}

	if _, err = s.ReserveContainerName(ctr.ID(), ctr.Name()); err != nil {
		return "", fmt.Errorf("kubelet may be retrying requests that are timing out in CRI-O due to system load: %w", err)
	}

	defer func() {
		if retErr != nil {
			log.Infof(ctx, "RestoreCtr: releasing container name %s", ctr.Name())
			s.ReleaseContainerName(ctr.Name())
		}
	}()
	ctr.SetRestore(true)

	newContainer, err := s.createSandboxContainer(ctx, ctr, sb)
	if err != nil {
		return "", err
	}
	defer func() {
		if retErr != nil {
			log.Infof(ctx, "RestoreCtr: deleting container %s from storage", ctr.ID())
			err2 := s.StorageRuntimeServer().DeleteContainer(ctr.ID())
			if err2 != nil {
				log.Warnf(ctx, "Failed to cleanup container directory: %v", err2)
			}
		}
	}()

	s.addContainer(newContainer)

	defer func() {
		if retErr != nil {
			log.Infof(ctx, "RestoreCtr: removing container %s", newContainer.ID())
			s.removeContainer(newContainer)
		}
	}()

	if err := s.CtrIDIndex().Add(ctr.ID()); err != nil {
		return "", err
	}
	defer func() {
		if retErr != nil {
			log.Infof(ctx, "RestoreCtr: deleting container ID %s from idIndex", ctr.ID())
			if err := s.CtrIDIndex().Delete(ctr.ID()); err != nil {
				log.Warnf(ctx, "Couldn't delete ctr id %s from idIndex", ctr.ID())
			}
		}
	}()

	newContainer.SetCreated()
	newContainer.SetRestore(true)
	newContainer.SetRestoreArchive(inputImage)

	if ctx.Err() == context.Canceled || ctx.Err() == context.DeadlineExceeded {
		log.Infof(ctx, "RestoreCtr: context was either canceled or the deadline was exceeded: %v", ctx.Err())
		return "", ctx.Err()
	}
	return ctr.ID(), nil
}
