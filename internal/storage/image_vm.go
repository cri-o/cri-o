package storage

import (
	"context"
	"fmt"
	"regexp"
	"strings"

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

// imageServiceVM is the ImageServer interface implementation that is more appropriate
// for VM based container runtimes.
type imageServiceVM struct {
	ctx context.Context

	// link to the OCI artifact store that will be used by this ImageServer
	store *datastore.Store

	// link to an ImageServer that is used to perform some of the image management
	// operations. This allows imageServiceVM to delegate the core image handling tasks
	// to the storage.ImageServer, while providing a VM-specific interface where
	// needed.
	storageImageServer *imageService

	// FIXME: we're currently storing the StorageImageId and ImageResult in memory.
	// We should find a way to persist this information in the storage, so that
	// it can survive a restart of CRI-O.

	// list of known RegistryImageReference with associated ImageResult and StorageID
	knownImages map[RegistryImageReference]cachedImageRefs
}

// GetImageServiceVM creates a new imageServiceVM instance.
func GetImageServiceVM(ctx context.Context, imageService *imageService) (ImageServer, error) {
	// Create a new OCI artifact store for pulling the artifact.
	// We make the store point to a dedicated location to avoid any risk of
	// mixing the pulled artifacts with regular container images.
	// FIXME: using placeholder location. Proper location TBD
	srcSystemContext := types.SystemContext{}

	artifactStore, artifactErr := datastore.New("/var/lib/containers/storage-for-coco", &srcSystemContext, true)
	if artifactErr != nil {
		return nil, fmt.Errorf("unable to create the ociartifact store err: %w", artifactErr)
	}

	return &imageServiceVM{
		ctx:                ctx,
		store:              artifactStore,
		storageImageServer: imageService,
		knownImages:        make(map[RegistryImageReference]cachedImageRefs),
	}, nil
}

// ListImages returns list of known images.
func (i *imageServiceVM) ListImages(systemContext *types.SystemContext) ([]ImageResult, error) {
	log.Debugf(i.ctx, "ImageServiceVM.ListImages() start")
	defer log.Debugf(i.ctx, "ImageServiceVM.ListImages() end")

	results := make([]ImageResult, 0, len(i.knownImages))
	for _, result := range i.knownImages {
		results = append(results, result.imageResult)
	}

	return results, nil
}

// ImageStatusByID returns status of a single image.
func (i *imageServiceVM) ImageStatusByID(systemContext *types.SystemContext, id StorageImageID) (*ImageResult, error) {
	log.Debugf(i.ctx, "ImageServiceVM.ImageStatusByID() start")
	defer log.Debugf(i.ctx, "ImageServiceVM.ImageStatusByID() end")

	for _, result := range i.knownImages {
		if result.imageResult.ID == id {
			return &result.imageResult, nil
		}
	}

	return nil, fmt.Errorf("image not found: %s", id.IDStringForOutOfProcessConsumptionOnly())
}

// ImageStatusByName returns status of an image tagged with name.
func (i *imageServiceVM) ImageStatusByName(systemContext *types.SystemContext, name RegistryImageReference) (*ImageResult, error) {
	log.Debugf(i.ctx, "ImageServiceVM.ImageStatusByName() start")
	defer log.Debugf(i.ctx, "ImageServiceVM.ImageStatusByName() end")

	// look at our list of known image references, and if we find a match
	// return the associated ImageResult.
	if result, exists := i.knownImages[name]; exists {
		return &result.imageResult, nil
	}

	return nil, fmt.Errorf("image not found: %s", name.StringForOutOfProcessConsumptionOnly())
}

// PullImage: do not pull the data, only get the manifest and return an image reference
//
// For this runtime, the image management is done within the VM that will run
// the container. CRI-O has nothing to do with the image, and must actually avoid
// pulling it, as it may fail if the image is encrypted for instance.
func (i *imageServiceVM) PullImage(ctx context.Context, imageName RegistryImageReference, options *ImageCopyOptions) (RegistryImageReference, error) {
	log.Debugf(i.ctx, "ImageServiceVM.PullImage() start")
	defer log.Debugf(i.ctx, "ImageServiceVM.PullImage() end")

	log.Debugf(ctx, "Skip image pull for runtime %s - image %s", "", imageName)

	srcRef, err := i.storageImageServer.lookup.remoteImageReference(imageName)
	if err != nil {
		return RegistryImageReference{}, err
	}

	copyOptions := &libimage.CopyOptions{
		OciDecryptConfig: options.OciDecryptConfig,
		Progress:         options.Progress,
		RemoveSignatures: true, // signature is not supported for OCI layout dest
	}

	artifactManifestDigest, artifactErr := i.store.Pull(ctx, srcRef, copyOptions)
	if artifactErr != nil {
		return RegistryImageReference{}, fmt.Errorf("unable to pull image or OCI artifact: pull image err: %w; artifact err: %w", err, artifactErr)
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
		repoDigests []string
	)

	if tagged, ok := imageRef.Raw().(reference.NamedTagged); ok {
		repoTags = append(repoTags, tagged.String())
	}

	repoDigests = append(repoDigests, artifactManifestDigest.String())

	imageResult := &ImageResult{
		ID: ID,
		// reuse the imageName here: this is the name that the agent in the VM
		// will use to pull the image on its side. It is useless to give it any
		// already resolved name, as the agent will need to do the resolution on
		// its side.
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
	i.knownImages[imageRef] = cachedImageRefs{
		imageResult: *imageResult,
		imageName:   imageName.StringForOutOfProcessConsumptionOnly(),
	}

	return imageRef, nil
}

// DeleteImage deletes a storage image (impacting all its tags).
func (i *imageServiceVM) DeleteImage(systemContext *types.SystemContext, id StorageImageID) error {
	log.Debugf(i.ctx, "ImageServiceVM.DeleteImage() start")
	defer log.Debugf(i.ctx, "ImageServiceVM.DeleteImage() end")

	err := i.store.Remove(i.ctx, id.IDStringForOutOfProcessConsumptionOnly())
	if err != nil {
		return fmt.Errorf("failed to delete image with ID %s: %w", id.IDStringForOutOfProcessConsumptionOnly(), err)
	}

	// remove the image from our cache
	for index, result := range i.knownImages {
		if result.imageResult.ID == id {
			delete(i.knownImages, index)
			break
		}
	}

	return nil
}

// UntagImage removes a name from the specified image, and if it was
// the only name the image had, removes the image.
func (i *imageServiceVM) UntagImage(systemContext *types.SystemContext, name RegistryImageReference) error {
	log.Debugf(i.ctx, "ImageServiceVM.UntagImage() start")
	defer log.Debugf(i.ctx, "ImageServiceVM.UntagImage() end")

	err := i.store.Remove(i.ctx, name.StringForOutOfProcessConsumptionOnly())
	if err != nil {
		return fmt.Errorf("failed to untag image %s: %w", name.StringForOutOfProcessConsumptionOnly(), err)
	}

	// remove the image from our cache
	delete(i.knownImages, name)

	return nil
}

// GetStore returns the reference to the default image store.
// This instance of the store holds an ociartifact store, which can't be used here.
// We return the default store instead, for compatibility.
func (i *imageServiceVM) GetStore() storage.Store {
	log.Debugf(i.ctx, "ImageServiceVM.GetStore() start")
	defer log.Debugf(i.ctx, "ImageServiceVM.GetStore() end")

	return i.storageImageServer.GetStore()
}

// HeuristicallyTryResolvingStringAsIDPrefix checks if heuristicInput could be a valid image ID or a prefix, and returns
// a StorageImageID if so, or nil if the input can be something else.
// DO NOT CALL THIS from in-process callers who know what their input is and don't NEED to involve heuristics.
func (i *imageServiceVM) HeuristicallyTryResolvingStringAsIDPrefix(heuristicInput string) *StorageImageID {
	log.Debugf(i.ctx, "ImageServiceVM.HeuristicallyTryResolvingStringAsIDPrefix() start")
	defer log.Debugf(i.ctx, "ImageServiceVM.HeuristicallyTryResolvingStringAsIDPrefix() end")

	if len(heuristicInput) >= minimumTruncatedIDLength {
		for index, result := range i.knownImages {
			if strings.HasPrefix(index.Raw().String(), heuristicInput) {
				return &result.imageResult.ID
			}

			if strings.HasPrefix(result.imageName, heuristicInput) {
				return &result.imageResult.ID
			}

			if result.imageResult.ID.IDStringForOutOfProcessConsumptionOnly() == heuristicInput {
				return &result.imageResult.ID
			}
		}
	}

	return nil
}

// CandidatesForPotentiallyShortImageName resolves an image name into a set of fully-qualified image names (domain/repo/image:tag|@digest).
// It will only return an empty slice if err != nil.
// For imageServiceVM, nothing specific is needed here. Name resolution can be
// done with the underlying storage image server.
func (i *imageServiceVM) CandidatesForPotentiallyShortImageName(systemContext *types.SystemContext, imageName string) ([]RegistryImageReference, error) {
	log.Debugf(i.ctx, "ImageServiceVM.CandidatesForPotentiallyShortImageName() start")
	defer log.Debugf(i.ctx, "ImageServiceVM.CandidatesForPotentiallyShortImageName() end")

	return i.storageImageServer.CandidatesForPotentiallyShortImageName(systemContext, imageName)
}

// UpdatePinnedImagesList updates pinned and pause images list in imageService.
func (i *imageServiceVM) UpdatePinnedImagesList(imageList []string) {
	log.Debugf(i.ctx, "ImageServiceVM.UpdatePinnedImagesList() start")
	defer log.Debugf(i.ctx, "ImageServiceVM.UpdatePinnedImagesList() end")

	i.storageImageServer.UpdatePinnedImagesList(imageList)
}

func (i *imageServiceVM) PinnedImageRegexps() []*regexp.Regexp {
	log.Debugf(i.ctx, "ImageServiceVM.PinnedImageRegexps() start")
	defer log.Debugf(i.ctx, "ImageServiceVM.PinnedImageRegexps() end")

	return i.storageImageServer.PinnedImageRegexps()
}

// IsRunningImageAllowed verifies if running of the container image is allowed.
//
// Arguments:
// - ctx: The context for controlling the function's execution
// - systemContext: server's system context for the given namespace, notably it might have a customized SignaturePolicyPath.
// - userSpecifiedImage: a RegistryImageReference that expresses users’ _intended_ image.
// - imageID: A StorageImageID of the image.
func (i *imageServiceVM) IsRunningImageAllowed(ctx context.Context, systemContext *types.SystemContext, userSpecifiedImage RegistryImageReference, imageID StorageImageID) error {
	log.Debugf(i.ctx, "ImageServiceVM.IsRunningImageAllowed() start")
	defer log.Debugf(i.ctx, "ImageServiceVM.IsRunningImageAllowed() end")

	return i.storageImageServer.IsRunningImageAllowed(ctx, systemContext, userSpecifiedImage, imageID)
}

// GetConfigForImage returns the OCI config for the given image reference.
//
// The config is retrieved as part of the PullImage process, and stored in our
// in-memory list of known images, so that it can be returned here without
// pulling anything.
func (i *imageServiceVM) GetConfigForImage(ctx context.Context, imageName string) (*v1.Image, error) {
	log.Debugf(i.ctx, "ImageServiceVM.GetConfigForImage() start")
	defer log.Debugf(i.ctx, "ImageServiceVM.GetConfigForImage() end")

	for index, result := range i.knownImages {
		if result.imageName == imageName {
			return result.imageResult.OCIConfig, nil
		}

		if index.Raw().String() == imageName {
			return result.imageResult.OCIConfig, nil
		}

		if result.imageResult.ID.IDStringForOutOfProcessConsumptionOnly() == imageName {
			return result.imageResult.OCIConfig, nil
		}
	}

	return nil, fmt.Errorf("image not found: %s", imageName)
}
