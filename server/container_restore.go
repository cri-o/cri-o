package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/containers/storage/pkg/archive"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/net/context"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	kubetypes "k8s.io/kubelet/pkg/types"

	"github.com/cri-o/cri-o/internal/factory/container"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/pkg/annotations"
)

// checkIfCheckpointOCIImage returns checks if the input refers to a checkpoint image.
// It returns the StorageImageID of the image the input resolves to, nil otherwise.
func (s *Server) checkIfCheckpointOCIImage(ctx context.Context, input string) (*storage.StorageImageID, error) {
	if input == "" {
		return nil, nil
	}
	if _, err := os.Stat(input); err == nil {
		return nil, nil
	}
	status, err := s.storageImageStatus(ctx, types.ImageSpec{
		Image: input,
	})
	if err != nil {
		return nil, err
	}

	if status == nil || status.Annotations == nil {
		return nil, nil
	}

	ann, ok := status.Annotations[annotations.CheckpointAnnotationName]
	if !ok {
		return nil, nil
	}

	log.Debugf(ctx, "Found checkpoint of container %v in %v", ann, input)

	return &status.ID, nil
}

// taken from Podman.
func (s *Server) CRImportCheckpoint(
	ctx context.Context,
	createConfig *types.ContainerConfig,
	sbID, sandboxUID string,
) (ctrID string, retErr error) {
	var mountPoint string

	// Ensure that the image to restore the checkpoint from has been provided.
	if createConfig.Image == nil || createConfig.Image.Image == "" {
		return "", errors.New(`attribute "image" missing from container definition`)
	}

	inputImage := createConfig.Image.Image
	createMounts := createConfig.Mounts
	createAnnotations := createConfig.Annotations
	createLabels := createConfig.Labels

	restoreStorageImageID, err := s.checkIfCheckpointOCIImage(ctx, inputImage)
	if err != nil {
		return "", err
	}

	var restoreArchivePath string
	if restoreStorageImageID != nil {
		log.Debugf(ctx, "Restoring from oci image %s", inputImage)

		// This is not out-of-process, but it is at least out of the CRI-O codebase; containers/storage uses raw strings.
		mountPoint, err = s.ContainerServer.StorageImageServer().GetStore().MountImage(restoreStorageImageID.IDStringForOutOfProcessConsumptionOnly(), nil, "")
		if err != nil {
			return "", err
		}

		log.Debugf(ctx, "Checkpoint image %s mounted at %v\n", restoreStorageImageID, mountPoint)

		defer func() {
			// This is not out-of-process, but it is at least out of the CRI-O codebase; containers/storage uses raw strings.
			if _, err := s.ContainerServer.StorageImageServer().GetStore().UnmountImage(restoreStorageImageID.IDStringForOutOfProcessConsumptionOnly(), true); err != nil {
				log.Errorf(ctx, "Could not unmount checkpoint image %s: %q", restoreStorageImageID, err)
			}
		}()
	} else {
		// First get the container definition from the
		// tarball to a temporary directory
		archiveFile, err := os.Open(inputImage)
		if err != nil {
			return "", fmt.Errorf("failed to open checkpoint archive %s for import: %w", inputImage, err)
		}
		defer func(f *os.File) {
			if err := f.Close(); err != nil {
				log.Errorf(ctx, "Unable to close file %s: %q", f.Name(), err)
			}
		}(archiveFile)

		restoreArchivePath = inputImage
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
		mountPoint, err = os.MkdirTemp("", "checkpoint")
		if err != nil {
			return "", err
		}
		defer func() {
			if err := os.RemoveAll(mountPoint); err != nil {
				log.Errorf(ctx, "Could not recursively remove %s: %q", mountPoint, err)
			}
		}()
		err = archive.Untar(archiveFile, mountPoint, options)
		if err != nil {
			return "", fmt.Errorf("unpacking of checkpoint archive %s failed: %w", mountPoint, err)
		}
		log.Debugf(ctx, "Unpacked checkpoint in %s", mountPoint)
	}

	// Load spec.dump from temporary directory
	dumpSpec := new(spec.Spec)
	if _, err := metadata.ReadJSONFile(dumpSpec, mountPoint, metadata.SpecDumpFile); err != nil {
		return "", fmt.Errorf("failed to read %q: %w", metadata.SpecDumpFile, err)
	}

	// Load config.dump from temporary directory
	config := new(metadata.ContainerConfig)
	if _, err := metadata.ReadJSONFile(config, mountPoint, metadata.ConfigDumpFile); err != nil {
		return "", fmt.Errorf("failed to read %q: %w", metadata.ConfigDumpFile, err)
	}

	if sbID == "" {
		// restore into previous sandbox
		sbID = dumpSpec.Annotations[annotations.SandboxID]
		ctrID = config.ID
	} else {
		ctrID = ""
	}

	ctrMetadata := types.ContainerMetadata{}
	originalAnnotations := make(map[string]string)
	originalLabels := make(map[string]string)

	if dumpSpec.Annotations[annotations.ContainerManager] == "libpod" {
		// This is an import from Podman
		ctrMetadata.Name = config.Name
		ctrMetadata.Attempt = 0
	} else {
		if err := json.Unmarshal([]byte(dumpSpec.Annotations[annotations.Metadata]), &ctrMetadata); err != nil {
			return "", fmt.Errorf("failed to read %q: %w", annotations.Metadata, err)
		}
		if createConfig.Metadata != nil && createConfig.Metadata.Name != "" {
			ctrMetadata.Name = createConfig.Metadata.Name
		}
		if err := json.Unmarshal([]byte(dumpSpec.Annotations[annotations.Annotations]), &originalAnnotations); err != nil {
			return "", fmt.Errorf("failed to read %q: %w", annotations.Annotations, err)
		}

		if err := json.Unmarshal([]byte(dumpSpec.Annotations[annotations.Labels]), &originalLabels); err != nil {
			return "", fmt.Errorf("failed to read %q: %w", annotations.Labels, err)
		}
		if sandboxUID != "" {
			if _, ok := originalLabels[kubetypes.KubernetesPodUIDLabel]; ok {
				originalLabels[kubetypes.KubernetesPodUIDLabel] = sandboxUID
			}
			if _, ok := originalAnnotations[kubetypes.KubernetesPodUIDLabel]; ok {
				originalAnnotations[kubetypes.KubernetesPodUIDLabel] = sandboxUID
			}
		}

		if createLabels != nil {
			fixupLabels := []string{
				// Update the container name. It has already been update in metadata.Name.
				// It also needs to be updated in the container labels.
				kubetypes.KubernetesContainerNameLabel,
				// Update pod name in the labels.
				kubetypes.KubernetesPodNameLabel,
				// Also update namespace.
				kubetypes.KubernetesPodNamespaceLabel,
			}

			for _, annotation := range fixupLabels {
				_, ok1 := createLabels[annotation]
				_, ok2 := originalLabels[annotation]

				// If the value is not set in the original container or
				// if it is not set in the new container, just skip
				// the step of updating metadata.
				if ok1 && ok2 {
					originalLabels[annotation] = createLabels[annotation]
				}
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
	}

	sb, err := s.getPodSandboxFromRequest(ctx, sbID)
	if err != nil {
		if errors.Is(err, sandbox.ErrIDEmpty) {
			return "", err
		}
		return "", fmt.Errorf("specified sandbox not found: %s: %w", sbID, err)
	}

	systemCtx, err := s.contextForNamespace(sb.Metadata().Namespace)
	if err != nil {
		return "", fmt.Errorf("get context for namespace: %w", err)
	}

	// WARNING: This hard-codes an assumption that SignaturePolicyPath set specifically for the namespace is never less restrictive
	// than the default system-wide policy, i.e. that if an image is successfully pulled, it always conforms to the system-wide policy.
	if systemCtx.SignaturePolicyPath != "" {
		return "", fmt.Errorf("namespaced signature policy %s defined for pods in namespace %s; signature validation is not supported for container restore", systemCtx.SignaturePolicyPath, sb.Metadata().Namespace)
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

	// Newer checkpoints archives have RootfsImageRef set
	// and using it for the restore is more correct.
	// For the Kubernetes use case the output of 'crictl ps'
	// contains for the original container under 'IMAGE' something
	// like 'registry/path/container@sha256:123444444...'.
	// The restored container was, however, only displaying something
	// like 'registry/path/container'.
	// This had two problems, first, the output from the restored
	// container was different, but the bigger problem was, that
	// CRI-O might pull the wrong image from the registry.
	// If the container in the registry was updated (new latest tag)
	// all of a sudden the wrong base image would be downloaded.
	rootFSImage := config.RootfsImageName
	if config.RootfsImageRef != "" {
		id, err := storage.ParseStorageImageIDFromOutOfProcessData(config.RootfsImageRef)
		if err != nil {
			return "", fmt.Errorf("invalid RootfsImageRef %q: %w", config.RootfsImageRef, err)
		}
		// This is not quite out-of-process consumption, but types.ContainerConfig is at least
		// a cross-process API, and this value is correct in that API.
		rootFSImage = id.IDStringForOutOfProcessConsumptionOnly()
	}
	containerConfig := &types.ContainerConfig{
		Metadata: &types.ContainerMetadata{
			Name:    ctrMetadata.Name,
			Attempt: ctrMetadata.Attempt,
		},
		Image: &types.ImageSpec{
			Image: rootFSImage,
		},
		Linux: &types.LinuxContainerConfig{
			Resources:       &types.LinuxContainerResources{},
			SecurityContext: &types.LinuxContainerSecurityContext{},
		},
		Annotations: originalAnnotations,
		Labels:      originalLabels,
	}

	if createConfig.Linux != nil {
		if createConfig.Linux.Resources != nil {
			containerConfig.Linux.Resources = createConfig.Linux.Resources
		}

		if createConfig.Linux.SecurityContext != nil {
			containerConfig.Linux.SecurityContext = createConfig.Linux.SecurityContext
		}
	}

	if dumpSpec.Linux != nil {
		if dumpSpec.Linux.MaskedPaths != nil {
			containerConfig.Linux.SecurityContext.MaskedPaths = dumpSpec.Linux.MaskedPaths
		}

		if dumpSpec.Linux.ReadonlyPaths != nil {
			containerConfig.Linux.SecurityContext.ReadonlyPaths = dumpSpec.Linux.ReadonlyPaths
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

	for _, m := range dumpSpec.Mounts {
		// Following mounts are ignored as they might point to the
		// wrong location and if ignored the mounts will correctly
		// be setup to point to the new location.
		if ignoreMounts[m.Destination] {
			continue
		}
		mount := &types.Mount{
			ContainerPath: m.Destination,
			HostPath:      m.Source,
		}

		for _, createMount := range createMounts {
			if createMount.ContainerPath == m.Destination {
				mount.HostPath = createMount.HostPath
			}
		}

		for _, opt := range m.Options {
			switch opt {
			case "ro":
				mount.Readonly = true
			case "rro":
				mount.RecursiveReadOnly = true
			case "rprivate":
				mount.Propagation = types.MountPropagation_PROPAGATION_PRIVATE
			case "rshared":
				mount.Propagation = types.MountPropagation_PROPAGATION_BIDIRECTIONAL
			case "rslaved":
				mount.Propagation = types.MountPropagation_PROPAGATION_HOST_TO_CONTAINER
			}
		}

		// Recursive Read-only (RRO) support requires the mount to be
		// read-only and the mount propagation set to private.
		if mount.RecursiveReadOnly {
			mount.Readonly = true
			mount.Propagation = types.MountPropagation_PROPAGATION_PRIVATE
		}

		log.Debugf(ctx, "Adding mounts %#v", mount)
		containerConfig.Mounts = append(containerConfig.Mounts, mount)
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
			s.ReleaseContainerName(ctx, ctr.Name())
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
			err2 := s.StorageRuntimeServer().DeleteContainer(ctx, ctr.ID())
			if err2 != nil {
				log.Warnf(ctx, "Failed to cleanup container directory: %v", err2)
			}
		}
	}()

	s.addContainer(ctx, newContainer)

	defer func() {
		if retErr != nil {
			log.Infof(ctx, "RestoreCtr: removing container %s", newContainer.ID())
			s.removeContainer(ctx, newContainer)
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
	newContainer.SetRestoreArchivePath(restoreArchivePath)
	newContainer.SetRestoreStorageImageID(restoreStorageImageID)
	newContainer.SetCheckpointedAt(config.CheckpointedAt)

	if isContextError(ctx.Err()) {
		log.Infof(ctx, "RestoreCtr: context was either canceled or the deadline was exceeded: %v", ctx.Err())
		return "", ctx.Err()
	}
	return ctr.ID(), nil
}
