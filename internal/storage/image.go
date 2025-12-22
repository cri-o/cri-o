package storage

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	encconfig "github.com/containers/ocicrypt/config"
	json "github.com/goccy/go-json"
	"github.com/moby/sys/mountinfo"
	digest "github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
	"go.podman.io/common/libimage"
	"go.podman.io/image/v5/copy"
	"go.podman.io/image/v5/docker"
	"go.podman.io/image/v5/docker/reference"
	cimage "go.podman.io/image/v5/image"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/pkg/shortnames"
	"go.podman.io/image/v5/signature"
	istorage "go.podman.io/image/v5/storage"
	"go.podman.io/image/v5/transports"
	"go.podman.io/image/v5/transports/alltransports"
	"go.podman.io/image/v5/types"
	"go.podman.io/storage"
	"go.podman.io/storage/pkg/reexec"
	crierrors "k8s.io/cri-api/pkg/errors"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/ociartifact"
	"github.com/cri-o/cri-o/internal/storage/references"
	"github.com/cri-o/cri-o/pkg/config"
)

const (
	minimumTruncatedIDLength = 3
)

//nolint:gochecknoinits // Init function is intentional here.
func init() {
	reexec.Register("crio-pull-image", pullImageChild)
}

// ImageResult wraps a subset of information about an image: its ID, its names,
// and the size, if known, or nil if it isn't.
type ImageResult struct {
	ID StorageImageID
	// May be nil if the image was referenced by ID and has no names.
	// It also has NO RELATIONSHIP to user input when returned by ImageStatusByName.
	SomeNameOfThisImage *RegistryImageReference
	RepoTags            []string
	RepoDigests         []string
	Size                *uint64
	Digest              digest.Digest
	User                string
	PreviousName        string
	Labels              map[string]string
	OCIConfig           *specs.Image
	Annotations         map[string]string
	Pinned              bool // pinned image to prevent it from garbage collection
	MountPoint          string
}

// A set of information that we prefer to cache about images, so that we can
// avoid having to reread them every time we need to return information about
// images.
// Every field in imageCacheItem are fixed properties of an "image", which in this
// context is the image.ID stored in c/storage, and thus don't need to be recomputed.
type imageCacheItem struct {
	config      *specs.Image
	size        *uint64
	info        *types.ImageInspectInfo
	annotations map[string]string
}

type imageCache map[string]imageCacheItem

// WARNING: All of imageLookupService must be JSON-representable because it is included in pullImageArgs.
type imageLookupService struct {
	DefaultTransport string
}

type imageService struct {
	lookup               *imageLookupService
	store                storage.Store
	storageTransport     StorageTransport
	imageCache           imageCache
	imageCacheLock       sync.Mutex
	ctx                  context.Context
	config               *config.Config
	regexForPinnedImages []*regexp.Regexp
}

// ImageBeingPulled map[string]bool to keep track of the images haven't done pulling.
var ImageBeingPulled sync.Map

// CgroupPullConfiguration
// WARNING: All of imageLookupService must be JSON-representable because it is included in pullImageArgs.
type CgroupPullConfiguration struct {
	UseNewCgroup bool
	ParentCgroup string
}

// subset of copy.Options that is supported by reexec.
// WARNING: All ofImageCopyOptions must be JSON-representable because it is included in pullImageArgs.
type ImageCopyOptions struct {
	SourceCtx        *types.SystemContext
	DestinationCtx   *types.SystemContext
	OciDecryptConfig *encconfig.DecryptConfig
	ProgressInterval time.Duration
	Progress         chan types.ProgressProperties `json:"-"`
	CgroupPull       CgroupPullConfiguration
}

// ImageServer wraps up various CRI-related activities into a reusable
// implementation.
type ImageServer interface {
	// ListImages returns list of all images.
	ListImages(systemContext *types.SystemContext) ([]ImageResult, error)
	// ImageStatusByID returns status of a single image
	ImageStatusByID(systemContext *types.SystemContext, id StorageImageID) (*ImageResult, error)
	// ImageStatusByName returns status of an image tagged with name.
	ImageStatusByName(systemContext *types.SystemContext, name RegistryImageReference) (*ImageResult, error)

	// PullImage imports an image from the specified location.
	//
	// Arguments:
	// - ctx: The context for controlling the function's execution
	// - imageName: A RegistryImageReference representing the image to be pulled
	// - options: Pointer to ImageCopyOptions, which contains various options for the image copy process
	//
	// Returns:
	// - A name@digest value referring to exactly the pulled image (the reference might become dangling if the image
	//   is removed, but it will not ever match a different image). The value is suitable for PullImageResponse.ImageRef
	//   and for ContainerConfig.Image.Image.
	// - error: An error object if pulling the image fails, otherwise nil
	PullImage(ctx context.Context, imageName RegistryImageReference, options *ImageCopyOptions) (RegistryImageReference, error)

	// DeleteImage deletes a storage image (impacting all its tags)
	DeleteImage(systemContext *types.SystemContext, id StorageImageID) error
	// UntagImage removes a name from the specified image, and if it was
	// the only name the image had, removes the image.
	UntagImage(systemContext *types.SystemContext, name RegistryImageReference) error

	// GetStore returns the reference to the storage library Store which
	// the image server uses to hold images, and is the destination used
	// when it's asked to pull an image.
	GetStore() storage.Store

	// HeuristicallyTryResolvingStringAsIDPrefix checks if heuristicInput could be a valid image ID or a prefix, and returns
	// a StorageImageID if so, or nil if the input can be something else.
	// DO NOT CALL THIS from in-process callers who know what their input is and don't NEED to involve heuristics.
	HeuristicallyTryResolvingStringAsIDPrefix(heuristicInput string) *StorageImageID
	// CandidatesForPotentiallyShortImageName resolves an image name into a set of fully-qualified image names (domain/repo/image:tag|@digest).
	// It will only return an empty slice if err != nil.
	CandidatesForPotentiallyShortImageName(systemContext *types.SystemContext, imageName string) ([]RegistryImageReference, error)

	// UpdatePinnedImagesList updates pinned and pause images list in imageService.
	UpdatePinnedImagesList(imageList []string)

	// IsRunningImageAllowed verifies if running of the container image is allowed.
	//
	// Arguments:
	// - ctx: The context for controlling the function's execution
	// - systemContext: server's system context for the given namespace, notably it might have a customized SignaturePolicyPath.
	// - userSpecifiedImage: a RegistryImageReference that expresses users’ _intended_ image.
	// - imageID: A StorageImageID of the image
	IsRunningImageAllowed(ctx context.Context, systemContext *types.SystemContext, userSpecifiedImage RegistryImageReference, imageID StorageImageID) error
}

func parseImageNames(image *storage.Image) (someName *RegistryImageReference, tags []reference.NamedTagged, digests []reference.Canonical, err error) {
	for _, nameString := range image.Names {
		name, err := reference.ParseNormalizedNamed(nameString)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("invalid name %q in image %q: %w", nameString, image.ID, err)
		}

		if reference.IsNameOnly(name) {
			return nil, nil, nil, fmt.Errorf("invalid name %q in image %q, it has neither a tag nor a digest", nameString, image.ID)
		}

		switch name := name.(type) {
		case reference.Canonical:
			digests = append(digests, name)
		case reference.NamedTagged:
			tags = append(tags, name)
		default:
			return nil, nil, nil, fmt.Errorf("internal error, invalid name %q in image %q is !IsNameOnly but neither Canonical nor NamedTagged", nameString, image.ID)
		}
	}

	if len(digests) > 0 {
		best := references.RegistryImageReferenceFromRaw(digests[0])
		someName = &best
	}

	if len(tags) > 0 {
		best := references.RegistryImageReferenceFromRaw(tags[0])
		someName = &best
	}

	return someName, tags, digests, nil
}

func (svc *imageService) makeRepoDigests(knownRepoDigests []reference.Canonical, tags []reference.NamedTagged, img *storage.Image) (imageDigest digest.Digest, repoDigests []reference.Canonical) {
	// Look up the image's digests.
	imageDigest = img.Digest
	if imageDigest == "" {
		imgDigest, err := svc.store.ImageBigDataDigest(img.ID, storage.ImageDigestBigDataKey)
		if err != nil || imgDigest == "" {
			return "", knownRepoDigests
		}

		imageDigest = imgDigest
	}

	imageDigests := []digest.Digest{imageDigest}

	for _, anotherImageDigest := range img.Digests {
		if anotherImageDigest != imageDigest {
			imageDigests = append(imageDigests, anotherImageDigest)
		}
	}
	// We only want to supplement what's already explicitly in the list, so keep track of values
	// that we already know.
	digestMap := make(map[string]struct{})
	repoDigests = knownRepoDigests

	for _, repoDigest := range knownRepoDigests {
		digestMap[repoDigest.String()] = struct{}{}
	}
	// Collect all known repos...
	repos := []reference.Named{}
	for _, tagged := range tags {
		repos = append(repos, reference.TrimNamed(tagged))
	}

	for _, digested := range knownRepoDigests {
		repos = append(repos, reference.TrimNamed(digested))
	}
	// ... and combine each repo with each digest.
	// Note that this may create digested references that never existed on those registries.
	for _, repo := range repos {
		for _, imageDigest := range imageDigests {
			if imageRef, err3 := reference.WithDigest(repo, imageDigest); err3 == nil {
				if _, ok := digestMap[imageRef.String()]; !ok {
					repoDigests = append(repoDigests, imageRef)
					digestMap[imageRef.String()] = struct{}{}
				}
			}
		}
	}

	return imageDigest, repoDigests
}

func (svc *imageService) buildImageCacheItem(systemContext *types.SystemContext, ref types.ImageReference) (imageCacheItem, error) {
	imageFull, err := ref.NewImage(svc.ctx, systemContext)
	if err != nil {
		return imageCacheItem{}, err
	}

	defer imageFull.Close()

	imageConfig, err := imageFull.OCIConfig(svc.ctx)
	if err != nil {
		return imageCacheItem{}, err
	}

	size := imageSize(imageFull)

	info, err := imageFull.Inspect(svc.ctx)
	if err != nil {
		return imageCacheItem{}, fmt.Errorf("inspecting image: %w", err)
	}

	rawSource, err := ref.NewImageSource(svc.ctx, systemContext)
	if err != nil {
		return imageCacheItem{}, err
	}
	defer rawSource.Close()

	topManifestBlob, manifestType, err := rawSource.GetManifest(svc.ctx, nil)
	if err != nil {
		return imageCacheItem{}, err
	}

	var ociManifest specs.Manifest
	if manifestType == specs.MediaTypeImageManifest {
		if err := json.Unmarshal(topManifestBlob, &ociManifest); err != nil {
			return imageCacheItem{}, err
		}
	}

	return imageCacheItem{
		config:      imageConfig,
		size:        size,
		info:        info,
		annotations: ociManifest.Annotations,
	}, nil
}

func (svc *imageService) buildImageResult(image *storage.Image, cacheItem imageCacheItem) (ImageResult, error) {
	someName, tags, digests, err := parseImageNames(image)
	if err != nil {
		return ImageResult{}, err
	}

	imageDigest, repoDigests := svc.makeRepoDigests(digests, tags, image)

	repoTagStrings := make([]string, 0, len(tags))
	for _, t := range tags {
		repoTagStrings = append(repoTagStrings, t.String())
	}

	sort.Strings(repoTagStrings)

	// Build repo digest strings with stable ordering:
	// - Keep knownRepoDigests (from image's stored names) in original order
	// - Sort additional computed digests alphabetically
	// This ensures the PullImage digest appears first, allowing Kubernetes to rely on
	// the first RepoDigest for credential tracking (kubernetes/kubernetes#135369).

	// Convert known digests to strings (preserve order)
	// `digests` contains canonical references from the image's stored names - these are
	// the "known" digests that should appear first in RepoDigests in their original order.
	// `repoDigests` (returned from makeRepoDigests earlier) contains the union of:
	// - known digests from the image's stored names
	// - computed digests from combining the image digest with repository names from tags
	repoDigestStrings := make([]string, 0, len(repoDigests))
	for _, d := range digests {
		repoDigestStrings = append(repoDigestStrings, d.String())
	}

	// Collect additional (computed) digests
	knownSet := make(map[string]bool, len(digests))
	for _, d := range repoDigestStrings {
		knownSet[d] = true
	}

	additionalDigests := make([]string, 0)

	for _, d := range repoDigests {
		digestString := d.String()
		if !knownSet[digestString] {
			additionalDigests = append(additionalDigests, digestString)
		}
	}

	// Sort only the additional digests
	sort.Strings(additionalDigests)

	// Combine: known (in original order) + additional (sorted)
	repoDigestStrings = append(repoDigestStrings, additionalDigests...)

	previousName := ""

	if len(image.NamesHistory) > 0 {
		// Remove the tag because we can only keep the name as indicator
		split := strings.SplitN(image.NamesHistory[0], ":", 2)
		if len(split) > 0 {
			previousName = split[0]
		}
	}

	imagePinned := false

	for _, image := range image.Names {
		if FilterPinnedImage(image, svc.regexForPinnedImages) {
			imagePinned = true

			break
		}
	}

	// Try to retrieve the mountpoint
	mountPoint := ""
	if layer, err := svc.store.Layer(image.TopLayer); err == nil {
		mountPoint = layer.MountPoint
	}
	// Check if the mount actually exists
	if mountPoint != "" {
		infos, err := mountinfo.GetMounts(mountinfo.SingleEntryFilter(mountPoint))
		if err != nil {
			logrus.Warnf("Unable to get mount info for path %s: %v", mountPoint, err)
			mountPoint = ""
		}

		if len(infos) == 0 {
			logrus.Warnf("Unable to find mount path %s for image %s, assuming image is no longer mounted", mountPoint, image.ID)

			if _, err := svc.store.UnmountImage(image.ID, true); err != nil {
				logrus.Warnf("Unable to unmount image %s: %v", image.ID, err)
			}

			mountPoint = ""
		}
	}

	return ImageResult{
		ID:                  storageImageIDFromImage(image),
		SomeNameOfThisImage: someName,
		RepoTags:            repoTagStrings,
		RepoDigests:         repoDigestStrings,
		Size:                cacheItem.size,
		Digest:              imageDigest,
		User:                cacheItem.config.Config.User,
		PreviousName:        previousName,
		Labels:              cacheItem.info.Labels,
		OCIConfig:           cacheItem.config,
		Annotations:         cacheItem.annotations,
		Pinned:              imagePinned,
		MountPoint:          mountPoint,
	}, nil
}

func (svc *imageService) ListImages(systemContext *types.SystemContext) ([]ImageResult, error) {
	images, err := svc.store.Images()
	if err != nil {
		return nil, err
	}

	results := make([]ImageResult, 0, len(images))
	newImageCache := make(imageCache, len(images))

	for i := range images {
		image := &images[i]

		ref, err := istorage.Transport.NewStoreReference(svc.store, nil, image.ID)
		if err != nil {
			return nil, err
		}

		svc.imageCacheLock.Lock()
		cacheItem, ok := svc.imageCache[image.ID]
		svc.imageCacheLock.Unlock()

		if !ok {
			cacheItem, err = svc.buildImageCacheItem(systemContext, ref)
			if err != nil {
				if os.IsNotExist(err) && imageIsBeingPulled(image) { // skip reporting errors if the images haven't finished pulling
					continue
				}

				return nil, err
			}
		}

		newImageCache[image.ID] = cacheItem

		res, err := svc.buildImageResult(image, cacheItem)
		if err != nil {
			return nil, err
		}

		results = append(results, res)
	}
	// replace image cache with cache we just built
	// this invalidates all stale entries in cache
	svc.imageCacheLock.Lock()
	svc.imageCache = newImageCache
	svc.imageCacheLock.Unlock()

	return results, nil
}

func imageIsBeingPulled(image *storage.Image) bool {
	for _, name := range image.Names {
		if _, ok := ImageBeingPulled.Load(name); ok {
			return true
		}
	}

	return false
}

func (svc *imageService) ImageStatusByName(systemContext *types.SystemContext, name RegistryImageReference) (*ImageResult, error) {
	unstableRef, err := istorage.Transport.NewStoreReference(svc.store, name.Raw(), "")
	if err != nil {
		return nil, err
	}

	return svc.imageStatus(systemContext, unstableRef)
}

func (svc *imageService) ImageStatusByID(systemContext *types.SystemContext, id StorageImageID) (*ImageResult, error) {
	ref, err := id.imageRef(svc)
	if err != nil {
		return nil, err
	}

	return svc.imageStatus(systemContext, ref)
}

// imageStatus is the underlying implementation of ImageStatus* for a storage unstableRef.
func (svc *imageService) imageStatus(systemContext *types.SystemContext, unstableRef types.ImageReference) (*ImageResult, error) {
	resolvedRef, image, err := svc.storageTransport.ResolveReference(unstableRef)
	if err != nil {
		return nil, err
	}
	// unstableRef might point to different images over time. Use resolvedRef, which precisely
	// matches image, from now on.

	svc.imageCacheLock.Lock()
	cacheItem, ok := svc.imageCache[image.ID]
	svc.imageCacheLock.Unlock()

	if !ok {
		var err error

		cacheItem, err = svc.buildImageCacheItem(systemContext, resolvedRef) // Single-use-only, not actually cached
		if err != nil {
			return nil, err
		}
	}

	result, err := svc.buildImageResult(image, cacheItem)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (svc *imageService) IsRunningImageAllowed(ctx context.Context, systemContext *types.SystemContext, userSpecifiedImage RegistryImageReference, imageID StorageImageID) error {
	policy, err := signature.DefaultPolicy(systemContext)
	if err != nil {
		return fmt.Errorf("get default policy: %w", err)
	}

	policyContext, err := signature.NewPolicyContext(policy)
	if err != nil {
		return fmt.Errorf("create policy context: %w", err)
	}

	defer func() {
		if err := policyContext.Destroy(); err != nil {
			log.Errorf(ctx, "Error destroying policy: %+v", err)
		}
	}()

	if err := svc.checkSignature(ctx, systemContext, policyContext, userSpecifiedImage, imageID); err != nil {
		return fmt.Errorf("checking signature of %q: %w", userSpecifiedImage, err)
	}

	log.Debugf(ctx, "Is allowed to run config image %s (policy path: %q)", userSpecifiedImage, systemContext.SignaturePolicyPath)

	return nil
}

func (svc *imageService) checkSignature(ctx context.Context, sys *types.SystemContext, policyContext *signature.PolicyContext, userSpecifiedImage RegistryImageReference, imageID StorageImageID) error {
	userSpecifiedImageRef, err := docker.NewReference(userSpecifiedImage.Raw())
	if err != nil {
		return fmt.Errorf("creating docker:// reference for %q: %w", userSpecifiedImage.Raw().String(), err)
	}

	// imageID is authoritative, but it may be a deduplicated image with several manifests,
	// and only one of them might be signed with the signatures required by policy.
	//
	// Here we could, possibly:
	// - if userSpecifiedImage is a repo@digest, resolve up that image, CHECK THAT IT MATCHES storageID, and use that
	//   reference (to use certainly the right digest)
	// - if userSpecifiedImage is a repo:tag, resolve up that image, CHECK THAT IT MATCHES storageID, and use that
	//   reference (assuming some future c/storage that can map repo:tag to the right digest)
	// Failing that (e.g. if a subsequent pull moved the tag, or if the image was untagged), try with the raw imageID.
	storageRef, err := imageID.imageRef(svc)
	if err != nil {
		return fmt.Errorf("creating containers-storage: reference for %v: %w", storageRef, err)
	}

	log.Debugf(ctx, "Created storageRef = %q", transports.ImageName(storageRef))

	storageSource, err := storageRef.NewImageSource(ctx, sys)
	if err != nil {
		return fmt.Errorf("creating image source for local store image: %w", err)
	}
	defer storageSource.Close()

	unparsedToplevel := cimage.UnparsedInstance(storageSource, nil)

	topManifest, topMIMEType, err := unparsedToplevel.Manifest(ctx)
	if err != nil {
		return fmt.Errorf("get top level manifest: %w", err)
	}

	unparsedInstance := unparsedToplevel

	if manifest.MIMETypeIsMultiImage(topMIMEType) {
		manifestList, err := manifest.ListFromBlob(topManifest, topMIMEType)
		if err != nil {
			return fmt.Errorf("parsing list manifest: %w", err)
		}

		instanceDigest, err := manifestList.ChooseInstance(sys)
		if err != nil {
			return fmt.Errorf("choosing instance: %w", err)
		}

		unparsedInstance = cimage.UnparsedInstance(storageSource, &instanceDigest)
	}

	mixedUnparsedInstance := cimage.UnparsedInstanceWithReference(unparsedInstance, userSpecifiedImageRef)

	allowed, err := policyContext.IsRunningImageAllowed(ctx, mixedUnparsedInstance)
	if err != nil {
		return fmt.Errorf("verifying signatures: %w", WrapSignatureCRIErrorIfNeeded(err))
	}

	if !allowed {
		panic("Internal inconsistency: IsRunningImageAllowed returned !allowed and no error when checking image signature")
	}

	return nil
}

// WrapSignatureCRIErrorIfNeeded wraps the CRI ErrSignatureValidationFailed if
// the provided err qualifies for that. If not, then it returns err.
func WrapSignatureCRIErrorIfNeeded(err error) error {
	var (
		policyErr    signature.PolicyRequirementError
		signatureErr signature.InvalidSignatureError
	)
	if errors.As(err, &policyErr) || errors.As(err, &signatureErr) {
		return fmt.Errorf("%w: %w", crierrors.ErrSignatureValidationFailed, err)
	}

	return err
}

func imageSize(img types.Image) *uint64 {
	if sum, err := img.Size(); err == nil {
		usum := uint64(sum)

		return &usum
	}

	return nil
}

// remoteImageReference creates an image reference for a CRI-O image reference.
func (svc *imageLookupService) remoteImageReference(imageName RegistryImageReference) (types.ImageReference, error) {
	if svc.DefaultTransport == "" {
		return nil, errors.New("DefaultTransport is not set")
	}
	// This is not actually out-of-process; the ParseImageName input is defined as cross-process strings, so, close enough.
	// Practically, the only reasonable value of DefaultTransport is docker://, so this should ideally be replaced by
	// a call to c/image/v5/docker.NewReference, and DefaultTransport should be deprecated.
	return alltransports.ParseImageName(svc.DefaultTransport + imageName.StringForOutOfProcessConsumptionOnly())
}

type pullImageArgs struct {
	Lookup       *imageLookupService
	ImageName    string // In the format of RegistryImageReference.StringForOutOfProcessConsumptionOnly()
	ParentCgroup string
	Options      *ImageCopyOptions

	StoreOptions storage.StoreOptions
}

type pullImageOutputItem struct {
	Progress *types.ProgressProperties `json:",omitempty"`
	Result   string                    `json:",omitempty"` // If not "", in the format of RegistryImageReference.StringForOutOfProcessConsumptionOnly(), and always contains a digest.
}

func pullImageChild() {
	var args pullImageArgs

	if err := json.NewDecoder(os.NewFile(0, "stdin")).Decode(&args); err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}

	if err := moveSelfToCgroup(args.ParentCgroup); err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}

	store, err := storage.GetStore(args.StoreOptions)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}

	imageName, err := references.ParseRegistryImageReferenceFromOutOfProcessData(args.ImageName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}

	output := make(chan pullImageOutputItem)
	outputWritten := make(chan struct{})

	go formatPullImageOutputItemGoroutine(os.Stdout, output, outputWritten)

	progress := make(chan types.ProgressProperties)

	go func() {
		for p := range progress {
			output <- pullImageOutputItem{Progress: &p}
		}
	}()

	args.Options.Progress = progress

	canonicalRef, err := pullImageImplementation(context.Background(), args.Lookup, store, imageName, args.Options)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}

	output <- pullImageOutputItem{Result: canonicalRef.StringForOutOfProcessConsumptionOnly()}

	close(output)
	<-outputWritten

	os.Exit(0)
}

func formatPullImageOutputItemGoroutine(dest io.Writer, items <-chan pullImageOutputItem, outputWritten chan<- struct{}) {
	defer func() {
		outputWritten <- struct{}{}
	}()

	encoder := json.NewEncoder(dest)
	for item := range items {
		if err := encoder.Encode(item); err != nil {
			fmt.Fprintf(os.Stderr, "%v", err)
			//nolint:gocritic // "exitAfterDefer: os.Exit will exit, and `defer func(){...}(...)` will not run"
			// If we fail writing output, outputWritten can never really be set, and it is no longer relevant.
			// Just abort.
			os.Exit(1)
		}
	}
}

func (svc *imageService) pullImageParent(ctx context.Context, imageName RegistryImageReference, parentCgroup string, options *ImageCopyOptions) (RegistryImageReference, error) {
	progress := options.Progress
	// the first argument imageName is not used by the re-execed command but it is useful for debugging as it
	// shows in the ps output.
	cmd := reexec.CommandContext(ctx, "crio-pull-image", imageName.StringForOutOfProcessConsumptionOnly())

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return RegistryImageReference{}, fmt.Errorf("error getting stdout pipe for image copy process: %w", err)
	}

	defer stdout.Close()

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return RegistryImageReference{}, fmt.Errorf("error getting stderr pipe for image copy process: %w", err)
	}
	defer stderr.Close()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return RegistryImageReference{}, fmt.Errorf("error getting stdin pipe for image copy process: %w", err)
	}

	stdinArguments := pullImageArgs{
		Lookup:       svc.lookup,
		Options:      options,
		ImageName:    imageName.StringForOutOfProcessConsumptionOnly(),
		ParentCgroup: parentCgroup,
		StoreOptions: storage.StoreOptions{
			RunRoot:            svc.store.RunRoot(),
			GraphRoot:          svc.store.GraphRoot(),
			GraphDriverName:    svc.store.GraphDriverName(),
			GraphDriverOptions: svc.store.GraphOptions(),
			UIDMap:             svc.store.UIDMap(),
			GIDMap:             svc.store.GIDMap(),
		},
	}

	stdinArguments.Options.Progress = nil

	if err := cmd.Start(); err != nil {
		return RegistryImageReference{}, err
	}

	if err := json.NewEncoder(stdin).Encode(&stdinArguments); err != nil {
		stdin.Close()

		if waitErr := cmd.Wait(); waitErr != nil {
			return RegistryImageReference{}, fmt.Errorf("%w: %w", waitErr, err)
		}

		return RegistryImageReference{}, fmt.Errorf("json encode to pipe failed: %w", err)
	}

	stdin.Close()

	resultChan := make(chan string)

	go func() {
		defer func() {
			close(resultChan) // Future reads, if any, will get "".
		}()

		decoder := json.NewDecoder(bufio.NewReader(stdout))

		if progress != nil {
			defer close(progress)
		}

		for decoder.More() {
			var item pullImageOutputItem
			if err := decoder.Decode(&item); err != nil {
				break
			}

			if item.Progress != nil && progress != nil {
				progress <- *item.Progress
			}

			if item.Result != "" {
				resultChan <- item.Result
			}
		}
	}()

	result := <-resultChan // Possibly "" if the process terminates before sending a result

	errOutput, errReadAll := io.ReadAll(stderr)
	if err := cmd.Wait(); err != nil {
		if errReadAll == nil && len(errOutput) > 0 {
			return RegistryImageReference{}, fmt.Errorf("pull image: %s", string(errOutput))
		}

		return RegistryImageReference{}, err
	}

	if result == "" {
		return RegistryImageReference{}, errors.New("pull child finished successfully but didn’t send a result")
	}

	canonicalRef, err := references.ParseRegistryImageReferenceFromOutOfProcessData(result)
	if err != nil {
		return RegistryImageReference{}, err
	}

	return canonicalRef, nil
}

func (svc *imageService) PullImage(ctx context.Context, imageName RegistryImageReference, options *ImageCopyOptions) (RegistryImageReference, error) {
	if options.CgroupPull.UseNewCgroup {
		return svc.pullImageParent(ctx, imageName, options.CgroupPull.ParentCgroup, options)
	} else {
		return pullImageImplementation(ctx, svc.lookup, svc.store, imageName, options)
	}
}

// pullImageImplementation is called in PullImage, both directly and inside pullImageChild.
// NOTE: That means this code can run in a separate process, and it should not access any CRI-O global state.
//
// It returns a name@digest value referring to exactly the pulled image.
func pullImageImplementation(ctx context.Context, lookup *imageLookupService, store storage.Store, imageName RegistryImageReference, options *ImageCopyOptions) (RegistryImageReference, error) {
	srcRef, err := lookup.remoteImageReference(imageName)
	if err != nil {
		return RegistryImageReference{}, err
	}

	srcSystemContext := types.SystemContext{}
	if options.SourceCtx != nil {
		srcSystemContext = *options.SourceCtx // A shallow copy
	}

	destRef, err := istorage.Transport.NewStoreReference(store, imageName.Raw(), "")
	if err != nil {
		return RegistryImageReference{}, err
	}

	policy, err := signature.DefaultPolicy(options.SourceCtx)
	if err != nil {
		return RegistryImageReference{}, err
	}

	policyContext, err := signature.NewPolicyContext(policy)
	if err != nil {
		return RegistryImageReference{}, err
	}

	manifestBytes, err := copy.Image(ctx, policyContext, destRef, srcRef, &copy.Options{
		SourceCtx:        &srcSystemContext,
		DestinationCtx:   options.DestinationCtx,
		OciDecryptConfig: options.OciDecryptConfig,
		ProgressInterval: options.ProgressInterval,
		Progress:         options.Progress,
	})
	isOCIArtifact := false

	var canonicalRef reference.Canonical

	if err != nil {
		artifactStore, artifactErr := ociartifact.NewStore(store.GraphRoot(), &srcSystemContext)
		if artifactErr != nil {
			return RegistryImageReference{}, fmt.Errorf("unable to pull image or OCI artifact: create store err: %w", artifactErr)
		}

		manifestDigest, artifactErr := artifactStore.PullManifest(ctx, srcRef, &libimage.CopyOptions{
			OciDecryptConfig: options.OciDecryptConfig,
			Progress:         options.Progress,
			RemoveSignatures: true, // signature is not supported for OCI layout dest
		})
		if artifactErr != nil {
			return RegistryImageReference{}, fmt.Errorf("unable to pull image or OCI artifact: pull image err: %w; artifact err: %w", err, artifactErr)
		}

		canonicalRef, err = reference.WithDigest(reference.TrimNamed(imageName.Raw()), *manifestDigest)
		if err != nil {
			return RegistryImageReference{}, fmt.Errorf("create canonical reference: %w", err)
		}

		isOCIArtifact = true
	} else {
		manifestDigest, err := manifest.Digest(manifestBytes)
		if err != nil {
			return RegistryImageReference{}, fmt.Errorf("digesting image: %w", err)
		}

		canonicalRef, err = reference.WithDigest(reference.TrimNamed(imageName.Raw()), manifestDigest)
		if err != nil {
			return RegistryImageReference{}, fmt.Errorf("create canonical reference: %w", err)
		}
	}

	// The manifestDigest may differ from the requested reference for multi-arch images
	// (platform-specific vs manifest list digest) or pull-by-tag (tag -> digest form).
	// Ensure the pulled digest appears first in image names so Kubernetes can find it
	// in ImageStatus RepoDigests for credential tracking (kubernetes/kubernetes#135369).
	// Note: This only applies to regular container images, not OCI artifacts.
	canonicalRefString := canonicalRef.String()
	if !isOCIArtifact && canonicalRefString != imageName.Raw().String() {
		// The pulled digest differs from the requested reference
		// Look up the image to reorder its names (reuse destRef from above)
		_, img, err := istorage.ResolveReference(destRef)
		if err != nil {
			return RegistryImageReference{}, fmt.Errorf("looking up pulled image: %w", err)
		}

		// Reorder names to put the platform-specific digest first
		reorderedNames := []string{canonicalRefString}
		for _, name := range img.Names {
			if name != canonicalRefString {
				reorderedNames = append(reorderedNames, name)
			}
		}

		if err := store.SetNames(img.ID, reorderedNames); err != nil {
			return RegistryImageReference{}, fmt.Errorf("reordering image names: %w", err)
		}
	}

	return references.RegistryImageReferenceFromRaw(canonicalRef), nil
}

func (svc *imageService) UntagImage(systemContext *types.SystemContext, name RegistryImageReference) error {
	unstableRef, err := istorage.Transport.NewStoreReference(svc.store, name.Raw(), "")
	if err != nil {
		return err
	}

	_, img, err := svc.storageTransport.ResolveReference(unstableRef)
	if err != nil {
		return err
	}
	// Do not use unstableRef from now on; if the tag moves, ref can refer to a different image.
	// Prefer img.ID or the other return value of ResolveReference.

	nameString := name.Raw().String()
	remainingNames := 0

	for _, imgName := range img.Names {
		if imgName != nameString {
			remainingNames += 1
		}
	}

	if remainingNames > 0 {
		return svc.store.RemoveNames(img.ID, []string{nameString})
	}
	// Note that the remainingNames check is unavoidably racy:
	// the image can be tagged with another name at this point.
	return svc.DeleteImage(systemContext, newExactStorageImageID(img.ID))
}

// DeleteImage deletes a storage image (impacting all its tags).
func (svc *imageService) DeleteImage(systemContext *types.SystemContext, id StorageImageID) error {
	ref, err := id.imageRef(svc)
	if err != nil {
		return err
	}

	return ref.DeleteImage(svc.ctx, systemContext)
}

func (svc *imageService) GetStore() storage.Store {
	return svc.store
}

// HeuristicallyTryResolvingStringAsIDPrefix checks if heuristicInput could be a valid image ID or a prefix, and returns
// a StorageImageID if so, or nil if the input can be something else.
// DO NOT CALL THIS from in-process callers who know what their input is and don't NEED to involve heuristics.
func (svc *imageService) HeuristicallyTryResolvingStringAsIDPrefix(heuristicInput string) *StorageImageID {
	if res, err := parseStorageImageID(heuristicInput); err == nil {
		return &res // If it is already a full image ID, accept it.
	}

	if len(heuristicInput) >= minimumTruncatedIDLength {
		if img, err := svc.store.Image(heuristicInput); err == nil && strings.HasPrefix(img.ID, heuristicInput) {
			// It's a truncated version of the ID of an image that's present in local storage;
			// we need to expand it.
			res := storageImageIDFromImage(img)

			return &res
		}
	}

	return nil
}

// CandidatesForPotentiallyShortImageName resolves an image name into a set of fully-qualified image names (domain/repo/image:tag|@digest).
// It will only return an empty slice if err != nil.
func (svc *imageService) CandidatesForPotentiallyShortImageName(systemContext *types.SystemContext, imageName string) ([]RegistryImageReference, error) {
	// Always resolve unqualified names to all candidates. We should use a more secure mode once we settle on a shortname alias table.
	sc := types.SystemContext{}
	if systemContext != nil {
		sc = *systemContext // A shallow copy
	}

	resolved, err := shortnames.Resolve(&sc, imageName)
	if err != nil {
		// Error is not very clear in this context, and unfortunately is also not a variable.
		if strings.Contains(err.Error(), "short-name resolution enforced but cannot prompt without a TTY") {
			return nil, fmt.Errorf("short name mode is enforcing, but image name %s returns ambiguous list", imageName)
		}

		return nil, err
	}

	if desc := resolved.Description(); desc != "" {
		logrus.Info(desc)
	}

	images := make([]RegistryImageReference, len(resolved.PullCandidates))
	for i := range resolved.PullCandidates {
		// This function will strip the tag if both tag and digest are specified, as it's supported
		// by Docker (and thus CRI-O by example) but not c/image.
		images[i] = references.RegistryImageReferenceFromRaw(resolved.PullCandidates[i].Value)
	}

	return images, nil
}

// GetImageService returns an ImageServer that uses the passed-in store, and
// which will prepend the passed-in DefaultTransport value to an image name if
// a name that's passed to its PullImage() method can't be resolved to an image
// in the store and can't be resolved to a source on its own.
func GetImageService(ctx context.Context, store storage.Store, storageTransport StorageTransport, serverConfig *config.Config) (ImageServer, error) {
	if store == nil {
		var err error

		storeOpts, err := storage.DefaultStoreOptions()
		if err != nil {
			return nil, err
		}

		store, err = storage.GetStore(storeOpts)
		if err != nil {
			return nil, err
		}
	}

	if storageTransport == nil {
		storageTransport = nativeStorageTransport{}
	}

	ils := &imageLookupService{
		DefaultTransport: serverConfig.DefaultTransport,
	}
	// add the sandbox/pause image configured by the user (if any) to the list of pinned_images.
	if serverConfig.PauseImage != "" {
		serverConfig.PinnedImages = append(serverConfig.PinnedImages, serverConfig.PauseImage)
	}

	is := &imageService{
		lookup:               ils,
		store:                store,
		storageTransport:     storageTransport,
		imageCache:           make(map[string]imageCacheItem),
		ctx:                  ctx,
		config:               serverConfig,
		regexForPinnedImages: CompileRegexpsForPinnedImages(serverConfig.PinnedImages),
	}

	//nolint:staticcheck // SA1019: InsecureRegistries is deprecated but still supported for backward compatibility
	if len(serverConfig.InsecureRegistries) > 0 {
		log.Errorf(ctx, "Insecure registries option is deprecated and no longer effective. Please use `insecure` in `registries.conf` instead.")
	}

	return is, nil
}

// StorageTransport is a level of indirection to allow mocking istorage.ResolveReference.
type StorageTransport interface {
	ResolveReference(ref types.ImageReference) (types.ImageReference, *storage.Image, error)
}

type nativeStorageTransport struct{}

func (st nativeStorageTransport) ResolveReference(ref types.ImageReference) (types.ImageReference, *storage.Image, error) {
	return istorage.ResolveReference(ref)
}

// UpdatePinnedImagesList updates pinned images list in imageService.
func (svc *imageService) UpdatePinnedImagesList(pinnedImages []string) {
	svc.regexForPinnedImages = CompileRegexpsForPinnedImages(pinnedImages)
}

// FilterPinnedImage checks if the given image needs to be pinned
// and excluded from kubelet's image GC.
func FilterPinnedImage(image string, pinnedImages []*regexp.Regexp) bool {
	if len(pinnedImages) == 0 {
		return false
	}

	for _, pinnedImage := range pinnedImages {
		if pinnedImage.MatchString(image) {
			return true
		}
	}

	return false
}

// CompileRegexpsForPinnedImages compiles regular expressions for the given
// list of pinned images.
func CompileRegexpsForPinnedImages(patterns []string) []*regexp.Regexp {
	regexps := make([]*regexp.Regexp, 0, len(patterns))

	for _, pattern := range patterns {
		var re *regexp.Regexp

		switch {
		case strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*"):
			// keyword pattern
			keyword := regexp.QuoteMeta(pattern[1 : len(pattern)-1])
			re = regexp.MustCompile("(?i)" + keyword)
		case strings.HasSuffix(pattern, "*"):
			// glob pattern
			pattern = regexp.QuoteMeta(pattern[:len(pattern)-1]) + ".*"
			re = regexp.MustCompile("(?i)" + pattern)
		default:
			// exact pattern
			re = regexp.MustCompile("(?i)^" + regexp.QuoteMeta(pattern) + "$")
		}

		regexps = append(regexps, re)
	}

	return regexps
}
