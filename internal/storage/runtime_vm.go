package storage

import (
	"context"
	"time"

	json "github.com/goccy/go-json"
	"github.com/sirupsen/logrus"
	"go.podman.io/image/v5/types"
	"go.podman.io/storage"
)

// runtimeServiceVM is a RuntimeServer implementation dedicated to VM-based
// runtimes. It is designed to do most operation using the regular container
// management workflow, but will delegate to a different ImageService when
// image management must be done by the runtimme.
type runtimeServiceVM struct {
	// Pointer to the runtimeService used for regular containers operations
	// Must be provided by the caller, and will be used to delegate operations
	// that don't need a specific behaviour for the VM runtime.
	runtimeServiceOCI RuntimeServer

	// Pointer to a runtimeService instance that uses an imageVM image service
	// in place of the regular one. This is used for operations that requires
	// specific handling of the image management.
	runtimeServiceVM RuntimeServer

	storageImageServer *imageServiceVM
	ctx                context.Context
}

func (r *runtimeServiceVM) createContainerOrPodSandbox(systemContext *types.SystemContext, containerID string, template *runtimeContainerMetadataTemplate, idMappingsOptions *storage.IDMappingOptions, labelOptions []string) (ci ContainerInfo, retErr error) {
	if template.podName == "" || template.podID == "" {
		return ContainerInfo{}, ErrInvalidPodName
	}

	if template.containerName == "" {
		return ContainerInfo{}, ErrInvalidContainerName
	}

	// Build metadata to store with the container.
	metadata := RuntimeContainerMetadata{
		PodName:       template.podName,
		PodID:         template.podID,
		ImageName:     template.userRequestedImage,
		ImageID:       template.imageID.IDStringForOutOfProcessConsumptionOnly(),
		ContainerName: template.containerName,
		MetadataName:  template.metadataName,
		UID:           template.uid,
		Namespace:     template.namespace,
		MountLabel:    "",
		// CreatedAt is set later
		Attempt: template.attempt,
		// Pod is set later
		Privileged: template.privileged,
	}
	if metadata.MetadataName == "" {
		metadata.MetadataName = metadata.ContainerName
	}

	// Pull out a copy of the image's configuration.
	imageConfig, err := r.storageImageServer.GetConfigForImage(r.ctx, template.userRequestedImage)
	if err != nil {
		return ContainerInfo{}, err
	}

	metadata.Pod = (containerID == metadata.PodID) // Or should this be hard-coded in callers? The caller should know whether it is creating the infra container.
	metadata.CreatedAt = time.Now().Unix()

	mdata, err := json.Marshal(&metadata)
	if err != nil {
		return ContainerInfo{}, err
	}

	// Build the container.
	names := []string{metadata.ContainerName}
	if metadata.Pod {
		names = append(names, metadata.PodName)
	}

	coptions := storage.ContainerOptions{
		LabelOpts: labelOptions,
		Volatile:  true,
	}
	if idMappingsOptions != nil {
		coptions.IDMappingOptions = *idMappingsOptions
	}

	// Call CreateContainer with an empty image name, to avoid image lookup
	// as we don't actually want to create this container with a local image.
	imageID := ""

	container, err := r.storageImageServer.GetStore().CreateContainer(containerID, names, imageID, "", string(mdata), &coptions)
	if err != nil {
		if metadata.Pod {
			logrus.Debugf("Failed to create pod sandbox %s(%s): %v", metadata.PodName, metadata.PodID, err)
		} else {
			logrus.Debugf("Failed to create container %s(%s): %v", metadata.ContainerName, containerID, err)
		}

		return ContainerInfo{}, err
	}

	if idMappingsOptions != nil {
		idMappingsOptions.UIDMap = container.UIDMap
		idMappingsOptions.GIDMap = container.GIDMap
	}

	if metadata.Pod {
		logrus.Debugf("Created pod sandbox %q", container.ID)
	} else {
		logrus.Debugf("Created container %q", container.ID)
	}

	// If anything fails after this point, we need to delete the incomplete
	// container before returning.
	defer func() {
		if retErr != nil {
			if err2 := r.storageImageServer.GetStore().DeleteContainer(container.ID); err2 != nil {
				if metadata.Pod {
					logrus.Debugf("%v deleting partially-created pod sandbox %q", err2, container.ID)
				} else {
					logrus.Debugf("%v deleting partially-created container %q", err2, container.ID)
				}

				return
			}

			logrus.Debugf("Deleted partially-created container %q", container.ID)
		}
	}()

	// Add a name to the container's layer so that it's easier to follow
	// what's going on if we're just looking at the storage-eye view of things.
	layerName := metadata.ContainerName + "-layer"

	err = r.storageImageServer.GetStore().AddNames(container.LayerID, []string{layerName})
	if err != nil {
		return ContainerInfo{}, err
	}

	// Find out where the container work directories are, so that we can return them.
	containerDir, err := r.storageImageServer.GetStore().ContainerDirectory(container.ID)
	if err != nil {
		return ContainerInfo{}, err
	}

	if metadata.Pod {
		logrus.Debugf("Pod sandbox %q has work directory %q", container.ID, containerDir)
	} else {
		logrus.Debugf("Container %q has work directory %q", container.ID, containerDir)
	}

	containerRunDir, err := r.storageImageServer.GetStore().ContainerRunDirectory(container.ID)
	if err != nil {
		return ContainerInfo{}, err
	}

	if metadata.Pod {
		logrus.Debugf("Pod sandbox %q has run directory %q", container.ID, containerRunDir)
	} else {
		logrus.Debugf("Container %q has run directory %q", container.ID, containerRunDir)
	}

	metadata.MountLabel = container.MountLabel()

	return ContainerInfo{
		ID:           container.ID,
		Dir:          containerDir,
		RunDir:       containerRunDir,
		Config:       imageConfig,
		ProcessLabel: container.ProcessLabel(),
		MountLabel:   container.MountLabel(),
	}, nil
}

func (r *runtimeServiceVM) CreatePodSandbox(systemContext *types.SystemContext, podName, podID string, pauseImage RegistryImageReference, imageAuthFile, containerName, metadataName, uid, namespace string, attempt uint32, idMappingsOptions *storage.IDMappingOptions, labelOptions []string, privileged bool) (ContainerInfo, error) {
	// For the VM runtime, we create the pod sandbox using the same underlying
	// service as for regular containers, because at this point the VM doesn't
	// exist and cannot assume the container image pull.
	return r.runtimeServiceOCI.CreatePodSandbox(systemContext, podName, podID, pauseImage, imageAuthFile, containerName, metadataName, uid, namespace, attempt, idMappingsOptions, labelOptions, privileged)
}

func (r *runtimeServiceVM) CreateContainer(systemContext *types.SystemContext, podName, podID, userRequestedImage string, imageID StorageImageID, containerName, containerID, metadataName string, attempt uint32, idMappingsOptions *storage.IDMappingOptions, labelOptions []string, privileged bool) (ContainerInfo, error) {
	return r.createContainerOrPodSandbox(systemContext, containerID, &runtimeContainerMetadataTemplate{
		podName:            podName,
		podID:              podID,
		userRequestedImage: userRequestedImage,
		imageID:            imageID,
		containerName:      containerName,
		metadataName:       metadataName,
		uid:                "",
		namespace:          "",
		attempt:            attempt,
		privileged:         privileged,
	}, idMappingsOptions, labelOptions)
}

func (r *runtimeServiceVM) DeleteContainer(ctx context.Context, idOrName string) error {
	return r.runtimeServiceOCI.DeleteContainer(ctx, idOrName)
}

func (r *runtimeServiceVM) SetContainerMetadata(idOrName string, metadata *RuntimeContainerMetadata) error {
	return r.runtimeServiceOCI.SetContainerMetadata(idOrName, metadata)
}

func (r *runtimeServiceVM) GetContainerMetadata(idOrName string) (RuntimeContainerMetadata, error) {
	return r.runtimeServiceOCI.GetContainerMetadata(idOrName)
}

func (r *runtimeServiceVM) StartContainer(idOrName string) (string, error) {
	return r.runtimeServiceOCI.StartContainer(idOrName)
}

func (r *runtimeServiceVM) StopContainer(ctx context.Context, idOrName string) error {
	return r.runtimeServiceOCI.StopContainer(ctx, idOrName)
}

func (r *runtimeServiceVM) GetWorkDir(id string) (string, error) {
	return r.runtimeServiceOCI.GetWorkDir(id)
}

func (r *runtimeServiceVM) GetRunDir(id string) (string, error) {
	return r.runtimeServiceOCI.GetRunDir(id)
}

// GetRuntimeServiceVM returns a RuntimeServer that uses the passed-in image
// service to pull and manage images, and its store to manage containers based
// on those images.
// The provided ImageServer must be an instance of ImageServiceVM.
func GetRuntimeServiceVM(ctx context.Context, runtimeService RuntimeServer, storageImageServer *imageServiceVM, storageTransport StorageTransport) RuntimeServer {
	if storageTransport == nil {
		storageTransport = nativeStorageTransport{}
	}

	// create an instance of runtimeService that is backed by the provided ImageServer
	rs_vm := GetRuntimeService(ctx, storageImageServer, storageTransport)

	return &runtimeServiceVM{
		runtimeServiceOCI:  runtimeService,
		runtimeServiceVM:   rs_vm,
		storageImageServer: storageImageServer,
		ctx:                ctx,
	}
}
