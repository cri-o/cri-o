package libpod

import (
	"context"
	"time"

	istorage "github.com/containers/image/v4/storage"
	"github.com/containers/image/v4/types"
	"github.com/containers/libpod/libpod/define"
	"github.com/containers/storage"
	"github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type storageService struct {
	store storage.Store
}

// getStorageService returns a storageService which can create container root
// filesystems from images
func getStorageService(store storage.Store) (*storageService, error) {
	return &storageService{store: store}, nil
}

// ContainerInfo wraps a subset of information about a container: the locations
// of its nonvolatile and volatile per-container directories, along with a copy
// of the configuration blob from the image that was used to create the
// container, if the image had a configuration.
// It also returns the ProcessLabel and MountLabel selected for the container
type ContainerInfo struct {
	Dir          string
	RunDir       string
	Config       *v1.Image
	ProcessLabel string
	MountLabel   string
}

// RuntimeContainerMetadata is the structure that we encode as JSON and store
// in the metadata field of storage.Container objects.  It is used for
// specifying attributes containers when they are being created, and allows a
// container's MountLabel, and possibly other values, to be modified in one
// read/write cycle via calls to storageService.ContainerMetadata,
// RuntimeContainerMetadata.SetMountLabel, and
// storageService.SetContainerMetadata.
type RuntimeContainerMetadata struct {
	// The provided name and the ID of the image that was used to
	// instantiate the container.
	ImageName string `json:"image-name"` // Applicable to both PodSandboxes and Containers
	ImageID   string `json:"image-id"`   // Applicable to both PodSandboxes and Containers
	// The container's name, which for an infrastructure container is usually PodName + "-infra".
	ContainerName string `json:"name"`                 // Applicable to both PodSandboxes and Containers, mandatory
	CreatedAt     int64  `json:"created-at"`           // Applicable to both PodSandboxes and Containers
	MountLabel    string `json:"mountlabel,omitempty"` // Applicable to both PodSandboxes and Containers
}

// SetMountLabel updates the mount label held by a RuntimeContainerMetadata
// object.
func (metadata *RuntimeContainerMetadata) SetMountLabel(mountLabel string) {
	metadata.MountLabel = mountLabel
}

// CreateContainerStorage creates the storage end of things.  We already have the container spec created
// TO-DO We should be passing in an Image object in the future.
func (r *storageService) CreateContainerStorage(ctx context.Context, systemContext *types.SystemContext, imageName, imageID, containerName, containerID string, options storage.ContainerOptions) (cinfo ContainerInfo, err error) {
	span, _ := opentracing.StartSpanFromContext(ctx, "createContainerStorage")
	span.SetTag("type", "storageService")
	defer span.Finish()

	var imageConfig *v1.Image
	if imageName != "" {
		var ref types.ImageReference
		if containerName == "" {
			return ContainerInfo{}, define.ErrEmptyID
		}
		// Check if we have the specified image.
		ref, err := istorage.Transport.ParseStoreReference(r.store, imageID)
		if err != nil {
			return ContainerInfo{}, err
		}
		img, err := istorage.Transport.GetStoreImage(r.store, ref)
		if err != nil {
			return ContainerInfo{}, err
		}
		// Pull out a copy of the image's configuration.
		image, err := ref.NewImage(ctx, systemContext)
		if err != nil {
			return ContainerInfo{}, err
		}
		defer image.Close()

		// Get OCI configuration of image
		imageConfig, err = image.OCIConfig(ctx)
		if err != nil {
			return ContainerInfo{}, err
		}

		// Update the image name and ID.
		if imageName == "" && len(img.Names) > 0 {
			imageName = img.Names[0]
		}
		imageID = img.ID
	}

	// Build metadata to store with the container.
	metadata := RuntimeContainerMetadata{
		ImageName:     imageName,
		ImageID:       imageID,
		ContainerName: containerName,
		CreatedAt:     time.Now().Unix(),
	}
	mdata, err := json.Marshal(&metadata)
	if err != nil {
		return ContainerInfo{}, err
	}

	// Build the container.
	names := []string{containerName}

	container, err := r.store.CreateContainer(containerID, names, imageID, "", string(mdata), &options)
	if err != nil {
		logrus.Debugf("failed to create container %s(%s): %v", metadata.ContainerName, containerID, err)

		return ContainerInfo{}, err
	}
	logrus.Debugf("created container %q", container.ID)

	// If anything fails after this point, we need to delete the incomplete
	// container before returning.
	defer func() {
		if err != nil {
			if err2 := r.store.DeleteContainer(container.ID); err2 != nil {
				logrus.Infof("%v deleting partially-created container %q", err2, container.ID)

				return
			}
			logrus.Infof("deleted partially-created container %q", container.ID)
		}
	}()

	// Add a name to the container's layer so that it's easier to follow
	// what's going on if we're just looking at the storage-eye view of things.
	layerName := metadata.ContainerName + "-layer"
	names, err = r.store.Names(container.LayerID)
	if err != nil {
		return ContainerInfo{}, err
	}
	names = append(names, layerName)
	err = r.store.SetNames(container.LayerID, names)
	if err != nil {
		return ContainerInfo{}, err
	}

	// Find out where the container work directories are, so that we can return them.
	containerDir, err := r.store.ContainerDirectory(container.ID)
	if err != nil {
		return ContainerInfo{}, err
	}
	logrus.Debugf("container %q has work directory %q", container.ID, containerDir)

	containerRunDir, err := r.store.ContainerRunDirectory(container.ID)
	if err != nil {
		return ContainerInfo{}, err
	}
	logrus.Debugf("container %q has run directory %q", container.ID, containerRunDir)

	return ContainerInfo{
		Dir:          containerDir,
		RunDir:       containerRunDir,
		Config:       imageConfig,
		ProcessLabel: container.ProcessLabel(),
		MountLabel:   container.MountLabel(),
	}, nil
}

func (r *storageService) DeleteContainer(idOrName string) error {
	if idOrName == "" {
		return define.ErrEmptyID
	}
	container, err := r.store.Container(idOrName)
	if err != nil {
		return err
	}
	err = r.store.DeleteContainer(container.ID)
	if err != nil {
		logrus.Debugf("failed to delete container %q: %v", container.ID, err)
		return err
	}
	return nil
}

func (r *storageService) SetContainerMetadata(idOrName string, metadata RuntimeContainerMetadata) error {
	mdata, err := json.Marshal(&metadata)
	if err != nil {
		logrus.Debugf("failed to encode metadata for %q: %v", idOrName, err)
		return err
	}
	return r.store.SetMetadata(idOrName, string(mdata))
}

func (r *storageService) GetContainerMetadata(idOrName string) (RuntimeContainerMetadata, error) {
	metadata := RuntimeContainerMetadata{}
	mdata, err := r.store.Metadata(idOrName)
	if err != nil {
		return metadata, err
	}
	if err = json.Unmarshal([]byte(mdata), &metadata); err != nil {
		return metadata, err
	}
	return metadata, nil
}

func (r *storageService) MountContainerImage(idOrName string) (string, error) {
	container, err := r.store.Container(idOrName)
	if err != nil {
		if errors.Cause(err) == storage.ErrContainerUnknown {
			return "", define.ErrNoSuchCtr
		}
		return "", err
	}
	metadata := RuntimeContainerMetadata{}
	if err = json.Unmarshal([]byte(container.Metadata), &metadata); err != nil {
		return "", err
	}
	mountPoint, err := r.store.Mount(container.ID, metadata.MountLabel)
	if err != nil {
		logrus.Debugf("failed to mount container %q: %v", container.ID, err)
		return "", err
	}
	logrus.Debugf("mounted container %q at %q", container.ID, mountPoint)
	return mountPoint, nil
}

func (r *storageService) UnmountContainerImage(idOrName string, force bool) (bool, error) {
	if idOrName == "" {
		return false, define.ErrEmptyID
	}
	container, err := r.store.Container(idOrName)
	if err != nil {
		return false, err
	}

	if !force {
		mounted, err := r.store.Mounted(container.ID)
		if err != nil {
			return false, err
		}
		if mounted == 0 {
			return false, storage.ErrLayerNotMounted
		}
	}
	mounted, err := r.store.Unmount(container.ID, force)
	if err != nil {
		logrus.Debugf("failed to unmount container %q: %v", container.ID, err)
		return false, err
	}
	logrus.Debugf("unmounted container %q", container.ID)
	return mounted, nil
}

func (r *storageService) MountedContainerImage(idOrName string) (int, error) {
	if idOrName == "" {
		return 0, define.ErrEmptyID
	}
	container, err := r.store.Container(idOrName)
	if err != nil {
		return 0, err
	}
	mounted, err := r.store.Mounted(container.ID)
	if err != nil {
		return 0, err
	}
	return mounted, nil
}

func (r *storageService) GetMountpoint(id string) (string, error) {
	container, err := r.store.Container(id)
	if err != nil {
		if errors.Cause(err) == storage.ErrContainerUnknown {
			return "", define.ErrNoSuchCtr
		}
		return "", err
	}
	layer, err := r.store.Layer(container.LayerID)
	if err != nil {
		return "", err
	}

	return layer.MountPoint, nil
}

func (r *storageService) GetWorkDir(id string) (string, error) {
	container, err := r.store.Container(id)
	if err != nil {
		if errors.Cause(err) == storage.ErrContainerUnknown {
			return "", define.ErrNoSuchCtr
		}
		return "", err
	}
	return r.store.ContainerDirectory(container.ID)
}

func (r *storageService) GetRunDir(id string) (string, error) {
	container, err := r.store.Container(id)
	if err != nil {
		if errors.Cause(err) == storage.ErrContainerUnknown {
			return "", define.ErrNoSuchCtr
		}
		return "", err
	}
	return r.store.ContainerRunDirectory(container.ID)
}
