package storage

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"go.podman.io/common/libimage"
	"go.podman.io/image/v5/docker/reference"
	"go.podman.io/image/v5/types"
	"go.podman.io/storage"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/ociartifact/datastore"
	"github.com/cri-o/cri-o/internal/storage/references"
)

type cachedImageRefs struct {
	// store the imageResult for later use
	imageResult ImageResult

	// keep track of the original image name that was used to pull the image,
	// so that it can be used for heuristics when asked with a short name.
	imageName string
}

// runtimePulledImageService is an ImageServer implementation that used with
// runtimes that perform the PullImage phase themselves.
type runtimePulledImageService struct {
	ctx context.Context

	// link to the OCI artifact store that will be used by this ImageServer
	store *datastore.Store

	// link to an ImageServer that is used to perform some of the image management
	// operations. This allows runtimePulledImageService to delegate the core image
	// handling tasks to the storage.ImageServer, while providing a specific
	// interface for runtimes that need it.
	storageImageServer *imageService

	// FIXME: we're currently storing the StorageImageId and ImageResult in memory.
	// We should find a way to persist this information in the storage, so that
	// it can survive a restart of CRI-O.

	// list of known RegistryImageReference with associated ImageResult and StorageID
	knownImages map[RegistryImageReference]cachedImageRefs

	// knownImagesLock protects concurrent access to knownImages
	knownImagesLock sync.RWMutex
}

// GetRuntimePulledImageService creates a new runtimePulledImageService instance.
func GetRuntimePulledImageService(ctx context.Context, imageService *imageService) (ImageServer, error) {
	// Create a new OCI artifact store for pulling the artifact.
	// We make the store point to a dedicated location to avoid any risk of
	// mixing the pulled artifacts with regular container images.
	srcSystemContext := *imageService.config.SystemContext // shallow copy to inherit auth, policy, etc.

	artifactStore, artifactErr := datastore.New("/var/lib/containers/storage-for-coco", &srcSystemContext, true)
	if artifactErr != nil {
		return nil, fmt.Errorf("unable to create the ociartifact store err: %w", artifactErr)
	}

	return &runtimePulledImageService{
		ctx:                ctx,
		store:              artifactStore,
		storageImageServer: imageService,
		knownImages:        make(map[RegistryImageReference]cachedImageRefs),
	}, nil
}

// ListImages returns list of known images.
func (i *runtimePulledImageService) ListImages(systemContext *types.SystemContext) ([]ImageResult, error) {
	log.Debugf(i.ctx, "runtimePulledImageService.ListImages() start")
	defer log.Debugf(i.ctx, "runtimePulledImageService.ListImages() end")

	i.knownImagesLock.RLock()
	defer i.knownImagesLock.RUnlock()

	results := make([]ImageResult, 0, len(i.knownImages))
	for id := range i.knownImages {
		results = append(results, i.knownImages[id].imageResult)
	}

	return results, nil
}

// ImageStatusByID returns status of a single image.
func (i *runtimePulledImageService) ImageStatusByID(systemContext *types.SystemContext, id StorageImageID) (*ImageResult, error) {
	log.Debugf(i.ctx, "runtimePulledImageService.ImageStatusByID() start")
	defer log.Debugf(i.ctx, "runtimePulledImageService.ImageStatusByID() end")

	i.knownImagesLock.RLock()
	defer i.knownImagesLock.RUnlock()

	for idx := range i.knownImages {
		if i.knownImages[idx].imageResult.ID == id {
			result := i.knownImages[idx].imageResult

			return &result, nil
		}
	}

	return nil, fmt.Errorf("image not found: %s", id.IDStringForOutOfProcessConsumptionOnly())
}

// ImageStatusByName returns status of an image tagged with name.
func (i *runtimePulledImageService) ImageStatusByName(systemContext *types.SystemContext, name RegistryImageReference) (*ImageResult, error) {
	log.Debugf(i.ctx, "runtimePulledImageService.ImageStatusByName() start")
	defer log.Debugf(i.ctx, "runtimePulledImageService.ImageStatusByName() end")

	i.knownImagesLock.RLock()
	defer i.knownImagesLock.RUnlock()

	// look at our list of known image references, and if we find a match
	// return the associated ImageResult.
	if result, exists := i.knownImages[name]; exists {
		return &result.imageResult, nil
	}

	// if not found by canonical digest key, check against the original pull reference
	nameStr := name.StringForOutOfProcessConsumptionOnly()
	for idx := range i.knownImages {
		if i.knownImages[idx].imageName == nameStr {
			result := i.knownImages[idx].imageResult

			return &result, nil
		}
	}

	return nil, fmt.Errorf("image not found: %s", nameStr)
}

// PullImage: do not pull the data, only get the manifest and return an image reference
//
// For this runtime, the image management is done by the runtime itself.
// CRI-O has nothing to do with the image, and must actually avoid
// pulling it, as it may fail if the image is encrypted for instance.
func (i *runtimePulledImageService) PullImage(ctx context.Context, imageName RegistryImageReference, options *ImageCopyOptions) (RegistryImageReference, error) {
	log.Debugf(i.ctx, "runtimePulledImageService.PullImage() start")
	defer log.Debugf(i.ctx, "runtimePulledImageService.PullImage() end")

	log.Debugf(ctx, "Skip image pull - image %s", imageName)

	srcRef, err := i.storageImageServer.lookup.remoteImageReference(imageName)
	if err != nil {
		return RegistryImageReference{}, err
	}

	copyOptions := &libimage.CopyOptions{
		OciDecryptConfig: options.OciDecryptConfig,
		Progress:         options.Progress,
		RemoveSignatures: true, // signature is not supported for OCI layout dest
	}

	// copy the SourceCtx options to keep per-request authentication credentials
	if options.SourceCtx != nil {
		if options.SourceCtx.AuthFilePath != "" {
			copyOptions.AuthFilePath = options.SourceCtx.AuthFilePath
		}

		if options.SourceCtx.DockerAuthConfig != nil {
			copyOptions.Username = options.SourceCtx.DockerAuthConfig.Username
			copyOptions.Password = options.SourceCtx.DockerAuthConfig.Password
		}
	}

	artifactManifestDigest, err := i.store.PullManifestOnly(ctx, srcRef, copyOptions)
	if err != nil {
		return RegistryImageReference{}, fmt.Errorf("unable to pull OCI artifact: %w", err)
	}

	canonicalRef, err := reference.WithDigest(reference.TrimNamed(imageName.Raw()), *artifactManifestDigest)
	if err != nil {
		return RegistryImageReference{}, fmt.Errorf("create canonical reference: %w", err)
	}

	imageRef := references.RegistryImageReferenceFromRaw(canonicalRef)

	// create the StorageImageID from the manifest digest
	ID := newExactStorageImageID(artifactManifestDigest.Encoded())

	// Get the OCIConfig
	ociConfig, err := i.store.PullConfig(ctx, artifactManifestDigest.Encoded(), &datastore.PullOptions{})
	if err != nil {
		return RegistryImageReference{}, fmt.Errorf("unable to pull image or OCI artifact: pull config err: %w", err)
	}

	// Generate an ImageResult with the available information, so that it can be
	// returned by ImageStatus when asked with the same reference.
	// Note that this structure is incomplete, since we're not actually pulling
	// the image.
	var (
		repoTags    []string
		repoDigests = make([]string, 0, 1)
	)

	if tagged, ok := imageRef.Raw().(reference.NamedTagged); ok {
		repoTags = append(repoTags, tagged.String())
	}

	repoDigests = append(repoDigests, artifactManifestDigest.String())

	imageResult := &ImageResult{
		ID: ID,
		// reuse the imageName here: this is the name that the runtime
		// will use to pull the image on its side. It is useless to give it any
		// already resolved name, as it will need to do the resolution on its side.
		SomeNameOfThisImage: &imageName,
		RepoTags:            repoTags,
		RepoDigests:         repoDigests,
		Digest:              *artifactManifestDigest,
		OCIConfig:           ociConfig,
		// Following fields are not available at this stage, and will be left
		// empty, or with default value
		Size:         nil,
		User:         "",
		PreviousName: "",
		Labels:       nil,
		Annotations:  nil,
		Pinned:       false,
		MountPoint:   "",
	}

	// Store the generated ImageResult and StorageImageID in a cache of known images
	i.knownImagesLock.Lock()
	i.knownImages[imageRef] = cachedImageRefs{
		imageResult: *imageResult,
		imageName:   imageName.StringForOutOfProcessConsumptionOnly(),
	}
	i.knownImagesLock.Unlock()

	return imageRef, nil
}

// DeleteImage deletes a storage image (impacting all its tags).
func (i *runtimePulledImageService) DeleteImage(systemContext *types.SystemContext, id StorageImageID) error {
	log.Debugf(i.ctx, "runtimePulledImageService.DeleteImage() start")
	defer log.Debugf(i.ctx, "runtimePulledImageService.DeleteImage() end")

	err := i.store.Remove(i.ctx, id.IDStringForOutOfProcessConsumptionOnly())
	if err != nil {
		return fmt.Errorf("failed to delete image with ID %s: %w", id.IDStringForOutOfProcessConsumptionOnly(), err)
	}

	// remove the image from our cache
	i.knownImagesLock.Lock()
	defer i.knownImagesLock.Unlock()

	for index := range i.knownImages {
		if i.knownImages[index].imageResult.ID == id {
			delete(i.knownImages, index)

			break
		}
	}

	return nil
}

// UntagImage removes a name from the specified image, and if it was
// the only name the image had, removes the image.
func (i *runtimePulledImageService) UntagImage(systemContext *types.SystemContext, name RegistryImageReference) error {
	log.Debugf(i.ctx, "runtimePulledImageService.UntagImage() start")
	defer log.Debugf(i.ctx, "runtimePulledImageService.UntagImage() end")

	err := i.store.Remove(i.ctx, name.StringForOutOfProcessConsumptionOnly())
	if err != nil {
		return fmt.Errorf("failed to untag image %s: %w", name.StringForOutOfProcessConsumptionOnly(), err)
	}

	// remove the image from our cache
	i.knownImagesLock.Lock()
	delete(i.knownImages, name)
	i.knownImagesLock.Unlock()

	return nil
}

// GetStore returns the reference to the default image store.
// This instance of the store holds an ociartifact store, which can't be used here.
// We return the default store instead, for compatibility.
func (i *runtimePulledImageService) GetStore() storage.Store {
	log.Debugf(i.ctx, "runtimePulledImageService.GetStore() start")
	defer log.Debugf(i.ctx, "runtimePulledImageService.GetStore() end")

	return i.storageImageServer.GetStore()
}

// HeuristicallyTryResolvingStringAsIDPrefix checks if heuristicInput could be a valid image ID or a prefix, and returns
// a StorageImageID if so, or nil if the input can be something else.
// DO NOT CALL THIS from in-process callers who know what their input is and don't NEED to involve heuristics.
func (i *runtimePulledImageService) HeuristicallyTryResolvingStringAsIDPrefix(heuristicInput string) *StorageImageID {
	log.Debugf(i.ctx, "runtimePulledImageService.HeuristicallyTryResolvingStringAsIDPrefix() start")
	defer log.Debugf(i.ctx, "runtimePulledImageService.HeuristicallyTryResolvingStringAsIDPrefix() end")

	i.knownImagesLock.RLock()
	defer i.knownImagesLock.RUnlock()

	if len(heuristicInput) >= minimumTruncatedIDLength {
		for index := range i.knownImages {
			if strings.HasPrefix(i.knownImages[index].imageResult.ID.IDStringForOutOfProcessConsumptionOnly(), heuristicInput) {
				id := i.knownImages[index].imageResult.ID

				return &id
			}
		}
	}

	return nil
}

// CandidatesForPotentiallyShortImageName resolves an image name into a set of fully-qualified image names (domain/repo/image:tag|@digest).
// It will only return an empty slice if err != nil.
// For this implementation of ImageServer, nothing specific is needed here.
// Name resolution can be done with the underlying storage image server.
func (i *runtimePulledImageService) CandidatesForPotentiallyShortImageName(systemContext *types.SystemContext, imageName string) ([]RegistryImageReference, error) {
	log.Debugf(i.ctx, "runtimePulledImageService.CandidatesForPotentiallyShortImageName() start")
	defer log.Debugf(i.ctx, "runtimePulledImageService.CandidatesForPotentiallyShortImageName() end")

	return i.storageImageServer.CandidatesForPotentiallyShortImageName(systemContext, imageName)
}

// UpdatePinnedImagesList updates pinned and pause images list in imageService.
func (i *runtimePulledImageService) UpdatePinnedImagesList(imageList []string) {
	log.Debugf(i.ctx, "runtimePulledImageService.UpdatePinnedImagesList() start")
	defer log.Debugf(i.ctx, "runtimePulledImageService.UpdatePinnedImagesList() end")

	i.storageImageServer.UpdatePinnedImagesList(imageList)
}

func (i *runtimePulledImageService) PinnedImageRegexps() []*regexp.Regexp {
	log.Debugf(i.ctx, "runtimePulledImageService.PinnedImageRegexps() start")
	defer log.Debugf(i.ctx, "runtimePulledImageService.PinnedImageRegexps() end")

	return i.storageImageServer.PinnedImageRegexps()
}

// IsRunningImageAllowed verifies if running of the container image is allowed.
//
// Arguments:
// - ctx: The context for controlling the function's execution
// - systemContext: server's system context for the given namespace, notably it might have a customized SignaturePolicyPath.
// - userSpecifiedImage: a RegistryImageReference that expresses users’ _intended_ image.
// - imageID: A StorageImageID of the image.
func (i *runtimePulledImageService) IsRunningImageAllowed(ctx context.Context, systemContext *types.SystemContext, userSpecifiedImage RegistryImageReference, imageID StorageImageID) error {
	log.Debugf(i.ctx, "runtimePulledImageService.IsRunningImageAllowed() start")
	defer log.Debugf(i.ctx, "runtimePulledImageService.IsRunningImageAllowed() end")

	return i.storageImageServer.IsRunningImageAllowed(ctx, systemContext, userSpecifiedImage, imageID)
}

// GetConfigForImage returns the OCI config for the given image reference.
//
// The config is retrieved as part of the PullImage process, and stored in our
// in-memory list of known images, so that it can be returned here without
// pulling anything.
func (i *runtimePulledImageService) GetConfigForImage(ctx context.Context, imageName string) (*v1.Image, error) {
	log.Debugf(i.ctx, "runtimePulledImageService.GetConfigForImage() start")
	defer log.Debugf(i.ctx, "runtimePulledImageService.GetConfigForImage() end")

	i.knownImagesLock.RLock()
	defer i.knownImagesLock.RUnlock()

	for index := range i.knownImages {
		if i.knownImages[index].imageName == imageName {
			return i.knownImages[index].imageResult.OCIConfig, nil
		}

		if index.Raw().String() == imageName {
			return i.knownImages[index].imageResult.OCIConfig, nil
		}

		if i.knownImages[index].imageResult.ID.IDStringForOutOfProcessConsumptionOnly() == imageName {
			return i.knownImages[index].imageResult.OCIConfig, nil
		}
	}

	return nil, fmt.Errorf("image not found: %s", imageName)
}
