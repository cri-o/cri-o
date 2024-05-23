package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	istorage "github.com/containers/image/v5/storage"
	"github.com/containers/image/v5/types"
	"github.com/containers/storage"
	json "github.com/json-iterator/go"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"

	"github.com/cri-o/cri-o/internal/log"
)

var (
	// ErrInvalidPodName is returned when a pod name specified to a
	// function call is found to be invalid (most often, because it's
	// empty).
	ErrInvalidPodName = errors.New("invalid pod name")
	// ErrInvalidContainerName is returned when a container name specified
	// to a function call is found to be invalid (most often, because it's
	// empty).
	ErrInvalidContainerName = errors.New("invalid container name")
	// ErrInvalidSandboxID is returned when a sandbox ID specified to a
	// function call is found to be invalid (because it's either
	// empty or doesn't match a valid sandbox).
	ErrInvalidSandboxID = errors.New("invalid sandbox ID")
	// ErrInvalidContainerID is returned when a container ID specified to a
	// function call is found to be invalid (because it's either
	// empty or doesn't match a valid container).
	ErrInvalidContainerID = errors.New("invalid container ID")
)

type runtimeService struct {
	storageImageServer ImageServer
	storageTransport   StorageTransport
	ctx                context.Context
}

// ContainerInfo wraps a subset of information about a container: its ID and
// the locations of its nonvolatile and volatile per-container directories,
// along with a copy of the configuration blob from the image that was used to
// create the container, if the image had a configuration.
type ContainerInfo struct {
	ID           string
	Dir          string
	RunDir       string
	Config       *v1.Image
	ProcessLabel string
	MountLabel   string
}

// RuntimeServer wraps up various CRI-related activities into a reusable
// implementation.
type RuntimeServer interface {
	// CreatePodSandbox creates a pod infrastructure container, using the
	// specified PodID for the infrastructure container's ID.  In the CRI
	// view of things, a sandbox is distinct from its containers, including
	// its infrastructure container, but at this level the sandbox is
	// essentially the same as its infrastructure container, with a
	// container's membership in a pod being signified by it listing the
	// same pod ID in its metadata that the pod's other members do, and
	// with the pod's infrastructure container having the same value for
	// both its pod's ID and its container ID.
	// Pointer arguments can be nil.  All other arguments are required.
	CreatePodSandbox(systemContext *types.SystemContext, podName, podID string, pauseImage RegistryImageReference, imageAuthFile, containerName, metadataName, uid, namespace string, attempt uint32, idMappingsOptions *storage.IDMappingOptions, labelOptions []string, privileged bool) (ContainerInfo, error)

	// GetContainerMetadata returns the metadata we've stored for a container.
	GetContainerMetadata(idOrName string) (RuntimeContainerMetadata, error)
	// SetContainerMetadata updates the metadata we've stored for a container.
	SetContainerMetadata(idOrName string, metadata *RuntimeContainerMetadata) error

	// CreateContainer creates a container with the specified ID.
	// Pointer arguments can be nil.
	// All other arguments are required.
	CreateContainer(systemContext *types.SystemContext, podName, podID, userRequestedImage string, imageID StorageImageID, containerName, containerID, metadataName string, attempt uint32, idMappingsOptions *storage.IDMappingOptions, labelOptions []string, privileged bool) (ContainerInfo, error)
	// DeleteContainer deletes a container, unmounting it first if need be.
	DeleteContainer(ctx context.Context, idOrName string) error

	// StartContainer makes sure a container's filesystem is mounted, and
	// returns the location of its root filesystem, which is not guaranteed
	// by lower-level drivers to never change.
	StartContainer(idOrName string) (string, error)
	// StopContainer attempts to unmount a container's root filesystem,
	// freeing up any kernel resources which may be limited.
	StopContainer(ctx context.Context, idOrName string) error

	// GetWorkDir returns the path of a nonvolatile directory on the
	// filesystem (somewhere under the Store's Root directory) which can be
	// used to store arbitrary data that is specific to the container.  It
	// will be removed automatically when the container is deleted.
	GetWorkDir(id string) (string, error)
	// GetRunDir returns the path of a volatile directory (does not survive
	// the host rebooting, somewhere under the Store's RunRoot directory)
	// on the filesystem which can be used to store arbitrary data that is
	// specific to the container.  It will be removed automatically when
	// the container is deleted.
	GetRunDir(id string) (string, error)
}

// RuntimeContainerMetadata is the structure that we encode as JSON and store
// in the metadata field of storage.Container objects.  It is used for
// specifying attributes of pod sandboxes and containers when they are being
// created, and allows a container's MountLabel, and possibly other values, to
// be modified in one read/write cycle via calls to
// RuntimeServer.ContainerMetadata, RuntimeContainerMetadata.SetMountLabel,
// and RuntimeServer.SetContainerMetadata.
type RuntimeContainerMetadata struct {
	// The pod's name and ID, kept for use by upper layers in determining
	// which containers belong to which pods.
	PodName string `json:"pod-name"` // Applicable to both PodSandboxes and Containers, mandatory
	PodID   string `json:"pod-id"`   // Applicable to both PodSandboxes and Containers, mandatory
	// The users' input originally used to find imageID; it might evaluate to a different image (or to a different kind of reference!) at any future time.
	ImageName string `json:"image-name"` // Applicable to both PodSandboxes and Containers
	// The ID of the image that was used to instantiate the container.
	ImageID string `json:"image-id"` // Applicable to both PodSandboxes and Containers
	// The container's name, which for an infrastructure container is usually PodName + "-infra".
	ContainerName string `json:"name"` // Applicable to both PodSandboxes and Containers, mandatory
	// The name as originally specified in PodSandbox or Container CRI metadata.
	MetadataName string `json:"metadata-name"`        // Applicable to both PodSandboxes and Containers, mandatory
	UID          string `json:"uid,omitempty"`        // Only applicable to pods
	Namespace    string `json:"namespace,omitempty"`  // Only applicable to pods
	MountLabel   string `json:"mountlabel,omitempty"` // Applicable to both PodSandboxes and Containers
	CreatedAt    int64  `json:"created-at"`           // Applicable to both PodSandboxes and Containers
	Attempt      uint32 `json:"attempt,omitempty"`    // Applicable to both PodSandboxes and Containers
	// Pod is true if this is the pod's infrastructure container.
	Pod        bool `json:"pod,omitempty"`        // Applicable to both PodSandboxes and Containers
	Privileged bool `json:"privileged,omitempty"` // Applicable to both PodSandboxes and Containers
}

// SetMountLabel updates the mount label held by a RuntimeContainerMetadata
// object.
func (metadata *RuntimeContainerMetadata) SetMountLabel(mountLabel string) {
	metadata.MountLabel = mountLabel
}

// runtimeContainerMetadataTemplate is an in-memory subset of RuntimeContainerMetadata.
type runtimeContainerMetadataTemplate struct {
	// The pod's name and ID, kept for use by upper layers in determining
	// which containers belong to which pods.
	podName string // Applicable to both PodSandboxes and Containers, mandatory
	podID   string // Applicable to both PodSandboxes and Containers, mandatory
	// The users' input originally used to find imageID; it might evaluate to a different image (or to a different kind of reference!) at any future time.
	userRequestedImage string // Applicable to both PodSandboxes and Containers
	// The ID of the image that was used to instantiate the container.
	imageID StorageImageID // Applicable to both PodSandboxes and Containers. Should refer to an image which existed just now (but that can change at any time).
	// The container's name, which for an infrastructure container is usually PodName + "-infra".
	containerName string // Applicable to both PodSandboxes and Containers, mandatory
	// The name as originally specified in PodSandbox or Container CRI metadata.
	metadataName string // Applicable to both PodSandboxes and Containers. May be "", defaults to ContainerName in that case
	uid          string // Only applicable to pods
	namespace    string // Only applicable to pods
	attempt      uint32 // Applicable to both PodSandboxes and Containers
	privileged   bool   // Applicable to both PodSandboxes and Containers
}

func (r *runtimeService) createContainerOrPodSandbox(systemContext *types.SystemContext, containerID string, template *runtimeContainerMetadataTemplate, idMappingsOptions *storage.IDMappingOptions, labelOptions []string) (ci ContainerInfo, retErr error) {
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
	// Ideally we would call imageID.imageRef(r.storageImageServer), but storageImageServer does not have access to private data.
	ref, err := istorage.Transport.NewStoreReference(r.storageImageServer.GetStore(), nil, template.imageID.privateID)
	if err != nil {
		return ContainerInfo{}, err
	}
	image, err := ref.NewImage(r.ctx, systemContext)
	if err != nil {
		return ContainerInfo{}, err
	}
	defer image.Close()

	imageConfig, err := image.OCIConfig(r.ctx)
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
	container, err := r.storageImageServer.GetStore().CreateContainer(containerID, names, template.imageID.privateID, "", string(mdata), &coptions)
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

func (r *runtimeService) CreatePodSandbox(systemContext *types.SystemContext, podName, podID string, pauseImage RegistryImageReference, imageAuthFile, containerName, metadataName, uid, namespace string, attempt uint32, idMappingsOptions *storage.IDMappingOptions, labelOptions []string, privileged bool) (ContainerInfo, error) {
	// Check if we have the specified image.
	var ref types.ImageReference
	ref, err := istorage.Transport.NewStoreReference(r.storageImageServer.GetStore(), pauseImage.Raw(), "")
	if err != nil {
		return ContainerInfo{}, err
	}
	_, img, err := r.storageTransport.ResolveReference(ref)
	if err != nil && errors.Is(err, istorage.ErrNoSuchImage) {
		logrus.Debugf("Couldn't find image %q, retrieving it", pauseImage)
		sourceCtx := types.SystemContext{}
		if systemContext != nil {
			sourceCtx = *systemContext // A shallow copy
		}
		if imageAuthFile != "" {
			sourceCtx.AuthFilePath = imageAuthFile
		}
		ref, _, err = r.storageImageServer.PullImage(context.Background(), pauseImage, &ImageCopyOptions{
			SourceCtx:      &sourceCtx,
			DestinationCtx: systemContext,
		})
		if err != nil {
			return ContainerInfo{}, err
		}
		_, img, err = r.storageTransport.ResolveReference(ref)
		if err != nil {
			return ContainerInfo{}, err
		}
		logrus.Debugf("Successfully pulled image %q", pauseImage)
	}
	if err != nil {
		if errors.Is(err, istorage.ErrNoSuchImage) {
			return ContainerInfo{}, fmt.Errorf("image %q not present in image store", pauseImage)
		}
		return ContainerInfo{}, err
	}

	// Resolve the image ID.
	imageID := storageImageIDFromImage(img)

	return r.createContainerOrPodSandbox(systemContext, podID, &runtimeContainerMetadataTemplate{
		podName:            podName,
		podID:              podID,
		userRequestedImage: pauseImage.StringForOutOfProcessConsumptionOnly(), // userRequestedImage is only used to write to container metadata on disk
		imageID:            imageID,
		containerName:      containerName,
		metadataName:       metadataName,
		uid:                uid,
		namespace:          namespace,
		attempt:            attempt,
		privileged:         privileged,
	}, idMappingsOptions, labelOptions)
}

func (r *runtimeService) CreateContainer(systemContext *types.SystemContext, podName, podID, userRequestedImage string, imageID StorageImageID, containerName, containerID, metadataName string, attempt uint32, idMappingsOptions *storage.IDMappingOptions, labelOptions []string, privileged bool) (ContainerInfo, error) {
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

func (r *runtimeService) deleteLayerIfMapped(imageID, layerID string) {
	if layerID == "" {
		return
	}
	store := r.storageImageServer.GetStore()

	image, err := store.Image(imageID)
	if err != nil {
		logrus.Debugf("Failed to retrieve image %q: %v", imageID, err)
		return
	}

	// ignore if it is the top layer.  It was pulled already with the specified
	// mapping.  In this case we don't delete it.
	if image.TopLayer == layerID {
		return
	}
	for _, ml := range image.MappedTopLayers {
		if ml == layerID {
			// if the layer is used by other containers, DeleteLayer
			// will fail.
			store.DeleteLayer(layerID) //nolint: errcheck
			return
		}
	}
}

func (r *runtimeService) DeleteContainer(ctx context.Context, idOrName string) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	if idOrName == "" {
		return ErrInvalidContainerID
	}
	container, err := r.storageImageServer.GetStore().Container(idOrName)
	// Already deleted
	if errors.Is(err, storage.ErrContainerUnknown) {
		return nil
	}
	if err != nil {
		return err
	}
	layer, err := r.storageImageServer.GetStore().Layer(container.LayerID)
	if err != nil {
		log.Debugf(ctx, "Failed to retrieve layer %q: %v", container.LayerID, err)
	}
	err = r.storageImageServer.GetStore().DeleteContainer(container.ID)
	if err != nil {
		log.Debugf(ctx, "Failed to delete container %q: %v", container.ID, err)
		return err
	}
	if layer != nil {
		r.deleteLayerIfMapped(container.ImageID, layer.Parent)
	}
	return nil
}

func (r *runtimeService) SetContainerMetadata(idOrName string, metadata *RuntimeContainerMetadata) error {
	mdata, err := json.Marshal(&metadata)
	if err != nil {
		logrus.Debugf("Failed to encode metadata for %q: %v", idOrName, err)
		return err
	}
	return r.storageImageServer.GetStore().SetMetadata(idOrName, string(mdata))
}

func (r *runtimeService) GetContainerMetadata(idOrName string) (RuntimeContainerMetadata, error) {
	metadata := RuntimeContainerMetadata{}
	mdata, err := r.storageImageServer.GetStore().Metadata(idOrName)
	if err != nil {
		return metadata, err
	}
	if err := json.Unmarshal([]byte(mdata), &metadata); err != nil {
		return metadata, err
	}
	return metadata, nil
}

func (r *runtimeService) StartContainer(idOrName string) (string, error) {
	container, err := r.storageImageServer.GetStore().Container(idOrName)
	if err != nil {
		if errors.Is(err, storage.ErrContainerUnknown) {
			return "", ErrInvalidContainerID
		}
		return "", err
	}
	metadata := RuntimeContainerMetadata{}
	if err := json.Unmarshal([]byte(container.Metadata), &metadata); err != nil {
		return "", err
	}
	mountPoint, err := r.storageImageServer.GetStore().Mount(container.ID, metadata.MountLabel)
	if err != nil {
		logrus.Debugf("Failed to mount container %q: %v", container.ID, err)
		return "", err
	}
	logrus.Debugf("Mounted container %q at %q", container.ID, mountPoint)
	return mountPoint, nil
}

func (r *runtimeService) StopContainer(ctx context.Context, idOrName string) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	if idOrName == "" {
		return ErrInvalidContainerID
	}
	container, err := r.storageImageServer.GetStore().Container(idOrName)
	if err != nil {
		if errors.Is(err, storage.ErrContainerUnknown) {
			log.Infof(ctx, "Container %s not known, assuming it got already removed", idOrName)
			return nil
		}

		log.Warnf(ctx, "Failed to get container %s: %v", idOrName, err)
		return err
	}

	if _, err := r.storageImageServer.GetStore().Unmount(container.ID, true); err != nil {
		if errors.Is(err, storage.ErrLayerUnknown) {
			log.Infof(ctx, "Layer for container %s not known", container.ID)
			return nil
		}

		log.Warnf(ctx, "Failed to unmount container %s: %v", container.ID, err)
		return err
	}

	log.Debugf(ctx, "Unmounted container %s", container.ID)
	return nil
}

func (r *runtimeService) GetWorkDir(id string) (string, error) {
	container, err := r.storageImageServer.GetStore().Container(id)
	if err != nil {
		if errors.Is(err, storage.ErrContainerUnknown) {
			return "", ErrInvalidContainerID
		}
		return "", err
	}
	return r.storageImageServer.GetStore().ContainerDirectory(container.ID)
}

func (r *runtimeService) GetRunDir(id string) (string, error) {
	container, err := r.storageImageServer.GetStore().Container(id)
	if err != nil {
		if errors.Is(err, storage.ErrContainerUnknown) {
			return "", ErrInvalidContainerID
		}
		return "", err
	}
	return r.storageImageServer.GetStore().ContainerRunDirectory(container.ID)
}

// GetRuntimeService returns a RuntimeServer that uses the passed-in image
// service to pull and manage images, and its store to manage containers based
// on those images.
func GetRuntimeService(ctx context.Context, storageImageServer ImageServer, storageTransport StorageTransport) RuntimeServer {
	if storageTransport == nil {
		storageTransport = nativeStorageTransport{}
	}
	return &runtimeService{
		storageImageServer: storageImageServer,
		storageTransport:   storageTransport,
		ctx:                ctx,
	}
}
