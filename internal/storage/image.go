package storage

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/pkg/shortnames"
	"github.com/containers/image/v5/signature"
	istorage "github.com/containers/image/v5/storage"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	encconfig "github.com/containers/ocicrypt/config"
	"github.com/containers/podman/v4/pkg/rootless"
	"github.com/containers/storage"
	"github.com/containers/storage/pkg/reexec"
	systemdDbus "github.com/coreos/go-systemd/v22/dbus"
	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/internal/dbusmgr"
	"github.com/cri-o/cri-o/utils"
	"github.com/godbus/dbus/v5"
	json "github.com/json-iterator/go"
	digest "github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	minimumTruncatedIDLength = 3
)

var (
	// ErrCannotParseImageID is returned when we try to ResolveNames for an image ID
	ErrCannotParseImageID = errors.New("cannot parse an image ID")
	// ErrImageMultiplyTagged is returned when we try to remove an image that still has multiple names
	ErrImageMultiplyTagged = errors.New("image still has multiple names applied")
)

// ImageResult wraps a subset of information about an image: its ID, its names,
// and the size, if known, or nil if it isn't.
type ImageResult struct {
	ID           string
	Name         string
	RepoTags     []string
	RepoDigests  []string
	Size         *uint64
	Digest       digest.Digest
	ConfigDigest digest.Digest
	User         string
	PreviousName string
	Labels       map[string]string
	OCIConfig    *specs.Image
	Annotations  map[string]string
}

type indexInfo struct {
	name   string
	secure bool
}

// A set of information that we prefer to cache about images, so that we can
// avoid having to reread them every time we need to return information about
// images.
// Every field in imageCacheItem are fixed properties of an "image", which in this
// context is the image.ID stored in c/storage, and thus don't need to be recomputed.
type imageCacheItem struct {
	config       *specs.Image
	size         *uint64
	configDigest digest.Digest
	info         *types.ImageInspectInfo
	annotations  map[string]string
}

type imageCache map[string]imageCacheItem

type imageLookupService struct {
	DefaultTransport      string
	InsecureRegistryCIDRs []*net.IPNet
	IndexConfigs          map[string]*indexInfo
}

type imageService struct {
	lookup         *imageLookupService
	store          storage.Store
	imageCache     imageCache
	imageCacheLock sync.Mutex
	ctx            context.Context
}

type imageServiceList struct {
	defaultImageServer ImageServer
	imageServers       map[string]ImageServer
}

// ImageBeingPulled map[string]bool to keep track of the images haven't done pulling.
var ImageBeingPulled sync.Map

// CgroupPullConfiguration
type CgroupPullConfiguration struct {
	UseNewCgroup bool
	ParentCgroup string
}

// subset of copy.Options that is supported by reexec.
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
	// ListImages returns list of all images which match the filter.
	ListImages(systemContext *types.SystemContext, filter string) ([]ImageResult, error)
	// ImageStatus returns status of an image which matches the filter.
	ImageStatus(systemContext *types.SystemContext, filter string) (*ImageResult, error)
	// PrepareImage returns an Image where the config digest can be grabbed
	// for further analysis. Call Close() on the resulting image.
	PrepareImage(systemContext *types.SystemContext, imageName string) (types.ImageCloser, error)
	// PullImage imports an image from the specified location.
	PullImage(systemContext *types.SystemContext, imageName string, options *ImageCopyOptions) (types.ImageReference, error)
	// UntagImage removes a name from the specified image, and if it was
	// the only name the image had, removes the image.
	UntagImage(systemContext *types.SystemContext, imageName string) error
	// GetStore returns the reference to the storage library Store which
	// the image server uses to hold images, and is the destination used
	// when it's asked to pull an image.
	GetStore() storage.Store
	// ResolveNames takes an image reference and if it's unqualified (w/o hostname),
	// it uses crio's default registries to qualify it.
	ResolveNames(systemContext *types.SystemContext, imageName string) ([]string, error)
}

// ImageServerList provides a way to access ImageServer instances.
// A default ImageServer is used for most containers, and a specific ImageServer
// can be registered for some containers.
type ImageServerList interface {
	// GetDefaultImageServer returns the default ImageServer
	GetDefaultImageServer() ImageServer
	// SetDefaultImageServer sets the default ImageServer for the ImageServerList
	SetDefaultImageServer(is ImageServer)
	// GetImageServer returns the ImageServer associated to the given container ID.
	// If there is none, it returns the default ImageServer.
	GetImageServer(containerID string) ImageServer
	// SetImageServer associates an ImageServer to the given container ID.
	SetImageServer(containerID string, is ImageServer)
	// DeleteImageServer removes the ImageServer associated to the given
	// containerID (if any)
	DeleteImageServer(containerID string)
	// ResolveNames makes a call to ResolveName() on each ImageServer and returns
	// the data returned by the first ImageServer that can resolve the name.
	ResolveNames(systemContext *types.SystemContext, imageName string) ([]string, error)
	// ListImages makes a call to ListImages() on each ImageServer, and returns
	// the combined list of images.
	ListImages(systemContext *types.SystemContext, filter string) ([]ImageResult, error)
	// UntagImage makes a call to UntagImage on each ImageServer that contains
	// the target image.
	UntagImage(systemContext *types.SystemContext, nameOrID string) (lastError error)
	// ImageStatus makes a call to ImageStatus on each ImageServer, starting
	// with the default, and returns the status from the first ImageServer that
	// can find the image.
	// It returns storage.ErrImageUnknown if no ImageServer can find the image.
	ImageStatus(systemContext *types.SystemContext, filter string) (*ImageResult, error)
}

func (svc *imageService) getRef(name string) (types.ImageReference, error) {
	ref, err := alltransports.ParseImageName(name)
	if err != nil {
		ref2, err2 := istorage.Transport.ParseStoreReference(svc.store, "@"+name)
		if err2 != nil {
			ref3, err3 := istorage.Transport.ParseStoreReference(svc.store, name)
			if err3 != nil {
				return nil, err
			}
			ref2 = ref3
		}
		ref = ref2
	}
	return ref, nil
}

func sortNamesByType(names []string) (bestName string, tags, digests []string) {
	for _, name := range names {
		if len(name) > 72 && name[len(name)-72:len(name)-64] == "@sha256:" {
			digests = append(digests, name)
		} else {
			tags = append(tags, name)
		}
	}
	if len(digests) > 0 {
		bestName = digests[0]
	}
	if len(tags) > 0 {
		bestName = tags[0]
	}
	return bestName, tags, digests
}

func (svc *imageService) makeRepoDigests(knownRepoDigests, tags []string, img *storage.Image) (imageDigest digest.Digest, repoDigests []string) {
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
		digestMap[repoDigest] = struct{}{}
	}
	// For each tagged name, parse the name, and if we can extract a named reference, convert
	// it into a canonical reference using the digest and add it to the list.
	for _, name := range append(tags, knownRepoDigests...) {
		if ref, err2 := reference.ParseNormalizedNamed(name); err2 == nil {
			trimmed := reference.TrimNamed(ref)
			for _, imageDigest := range imageDigests {
				if imageRef, err3 := reference.WithDigest(trimmed, imageDigest); err3 == nil {
					if _, ok := digestMap[imageRef.String()]; !ok {
						repoDigests = append(repoDigests, imageRef.String())
						digestMap[imageRef.String()] = struct{}{}
					}
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
	configDigest := imageFull.ConfigInfo().Digest
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
		config:       imageConfig,
		size:         size,
		configDigest: configDigest,
		info:         info,
		annotations:  ociManifest.Annotations,
	}, nil
}

func (svc *imageService) buildImageResult(image *storage.Image, cacheItem imageCacheItem) ImageResult {
	name, tags, digests := sortNamesByType(image.Names)
	imageDigest, repoDigests := svc.makeRepoDigests(digests, tags, image)
	sort.Strings(tags)
	sort.Strings(repoDigests)
	previousName := ""
	if len(image.NamesHistory) > 0 {
		// Remove the tag because we can only keep the name as indicator
		split := strings.SplitN(image.NamesHistory[0], ":", 2)
		if len(split) > 0 {
			previousName = split[0]
		}
	}

	return ImageResult{
		ID:           image.ID,
		Name:         name,
		RepoTags:     tags,
		RepoDigests:  repoDigests,
		Size:         cacheItem.size,
		Digest:       imageDigest,
		ConfigDigest: cacheItem.configDigest,
		User:         cacheItem.config.Config.User,
		PreviousName: previousName,
		Labels:       cacheItem.info.Labels,
		OCIConfig:    cacheItem.config,
		Annotations:  cacheItem.annotations,
	}
}

func (svc *imageService) appendCachedResult(systemContext *types.SystemContext, ref types.ImageReference, image *storage.Image, results []ImageResult, newImageCache imageCache) ([]ImageResult, error) {
	var err error
	svc.imageCacheLock.Lock()
	cacheItem, ok := svc.imageCache[image.ID]
	svc.imageCacheLock.Unlock()
	if !ok {
		cacheItem, err = svc.buildImageCacheItem(systemContext, ref)
		if err != nil {
			return results, err
		}
		if newImageCache == nil {
			svc.imageCacheLock.Lock()
			svc.imageCache[image.ID] = cacheItem
			svc.imageCacheLock.Unlock()
		} else {
			newImageCache[image.ID] = cacheItem
		}
	} else if newImageCache != nil {
		newImageCache[image.ID] = cacheItem
	}

	return append(results, svc.buildImageResult(image, cacheItem)), nil
}

func (svc *imageService) ListImages(systemContext *types.SystemContext, filter string) ([]ImageResult, error) {
	var results []ImageResult
	if filter != "" {
		// we never remove entries from cache unless unfiltered ListImages call is made. Is it safe?
		ref, err := svc.getRef(filter)
		if err != nil {
			return nil, err
		}
		if image, err := istorage.Transport.GetStoreImage(svc.store, ref); err == nil {
			results, err = svc.appendCachedResult(systemContext, ref, image, []ImageResult{}, nil)
			if err != nil {
				return nil, err
			}
		}
	} else {
		images, err := svc.store.Images()
		if err != nil {
			return nil, err
		}
		newImageCache := make(imageCache, len(images))
		for i := range images {
			image := &images[i]
			ref, err := istorage.Transport.ParseStoreReference(svc.store, "@"+image.ID)
			if err != nil {
				return nil, err
			}
			results, err = svc.appendCachedResult(systemContext, ref, image, results, newImageCache)
			if err != nil {
				// skip reporting errors if the images haven't finished pulling
				if os.IsNotExist(err) {
					donePulling := true
					for _, name := range image.Names {
						if _, ok := ImageBeingPulled.Load(name); ok {
							donePulling = false
							break
						}
					}
					if !donePulling {
						continue
					}
				}
				return nil, err
			}
		}
		// replace image cache with cache we just built
		// this invalidates all stale entries in cache
		svc.imageCacheLock.Lock()
		svc.imageCache = newImageCache
		svc.imageCacheLock.Unlock()
	}
	return results, nil
}

func (svc *imageService) ImageStatus(systemContext *types.SystemContext, nameOrID string) (*ImageResult, error) {
	ref, err := svc.getRef(nameOrID)
	if err != nil {
		return nil, err
	}
	image, err := istorage.Transport.GetStoreImage(svc.store, ref)
	if err != nil {
		return nil, err
	}
	svc.imageCacheLock.Lock()
	cacheItem, ok := svc.imageCache[image.ID]
	svc.imageCacheLock.Unlock()

	if !ok {
		cacheItem, err = svc.buildImageCacheItem(systemContext, ref) // Single-use-only, not actually cached
		if err != nil {
			return nil, err
		}
	}

	result := svc.buildImageResult(image, cacheItem)
	return &result, nil
}

func imageSize(img types.Image) *uint64 {
	if sum, err := img.Size(); err == nil {
		usum := uint64(sum)
		return &usum
	}
	return nil
}

// remoteImageReference creates an image reference from an image string
func (svc *imageLookupService) remoteImageReference(imageName string) (types.ImageReference, error) {
	if imageName == "" {
		return nil, storage.ErrNotAnImage
	}

	srcRef, err := alltransports.ParseImageName(imageName)
	if err != nil {
		if svc.DefaultTransport == "" {
			return nil, err
		}
		srcRef2, err2 := alltransports.ParseImageName(svc.DefaultTransport + imageName)
		if err2 != nil {
			return nil, err
		}
		srcRef = srcRef2
	}
	return srcRef, nil
}

// prepareReference creates an image reference from an image string and returns an updated types.SystemContext (never nil) for the image
func (svc *imageLookupService) prepareReference(inputSystemContext *types.SystemContext, imageName string) (*types.SystemContext, types.ImageReference, error) {
	srcRef, err := svc.remoteImageReference(imageName)
	if err != nil {
		return nil, nil, err
	}

	sc := types.SystemContext{}
	if inputSystemContext != nil {
		sc = *inputSystemContext // A shallow copy
	}
	if srcRef.DockerReference() != nil {
		hostname := reference.Domain(srcRef.DockerReference())
		if secure := svc.isSecureIndex(hostname); !secure {
			sc.DockerInsecureSkipTLSVerify = types.OptionalBoolTrue
		}
	}
	return &sc, srcRef, nil
}

func (svc *imageService) PrepareImage(inputSystemContext *types.SystemContext, imageName string) (types.ImageCloser, error) {
	systemContext, srcRef, err := svc.lookup.prepareReference(inputSystemContext, imageName)
	if err != nil {
		return nil, err
	}

	return srcRef.NewImage(svc.ctx, systemContext)
}

// nolint: gochecknoinits
func init() {
	reexec.Register("crio-copy-image", copyImageChild)
}

type copyImageArgs struct {
	Lookup         *imageLookupService
	ImageName      string
	ParentCgroup   string
	SystemContext  *types.SystemContext
	Options        *ImageCopyOptions
	HasCollectMode bool

	StoreOptions storage.StoreOptions
}

// moveSelfToCgroup moves the current process to a new transient cgroup.
func moveSelfToCgroup(cgroup string, hasCollectMode bool) error {
	slice := "system.slice"
	if rootless.IsRootless() {
		slice = "user.slice"
	}

	if cgroup != "" {
		if !strings.Contains(cgroup, ".slice") {
			return fmt.Errorf("invalid systemd cgroup %q", cgroup)
		}
		slice = filepath.Base(cgroup)
	}

	unitName := fmt.Sprintf("crio-pull-image-%d.scope", os.Getpid())

	systemdProperties := []systemdDbus.Property{}
	if hasCollectMode {
		systemdProperties = append(systemdProperties,
			systemdDbus.Property{
				Name:  "CollectMode",
				Value: dbus.MakeVariant("inactive-or-failed"),
			})
	}

	return utils.RunUnderSystemdScope(dbusmgr.NewDbusConnManager(rootless.IsRootless()), os.Getpid(), slice, unitName, systemdProperties...)
}

func copyImageChild() {
	var args copyImageArgs

	if err := json.NewDecoder(os.NewFile(0, "stdin")).Decode(&args); err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}

	if err := moveSelfToCgroup(args.ParentCgroup, args.HasCollectMode); err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}

	store, err := storage.GetStore(args.StoreOptions)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}

	policy, err := signature.DefaultPolicy(args.SystemContext)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}
	policyContext, err := signature.NewPolicyContext(policy)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}

	srcSystemContext, srcRef, destRef, err := args.Lookup.getReferences(args.Options.SourceCtx, store, args.ImageName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}

	progress := make(chan types.ProgressProperties)
	go func() {
		stream := json.NewStream(json.ConfigDefault, os.Stdout, 4096)
		for p := range progress {
			stream.WriteVal(p)
			stream.WriteRaw("\n")
			if err := stream.Flush(); err != nil {
				fmt.Fprintf(os.Stderr, "%v", err)
				os.Exit(1)
			}
		}
	}()

	options := toCopyOptions(args.Options, progress)
	options.SourceCtx = srcSystemContext
	if _, err := copy.Image(context.Background(), policyContext, destRef, srcRef, options); err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}

	os.Exit(0)
}

func toCopyOptions(options *ImageCopyOptions, progress chan types.ProgressProperties) *copy.Options {
	return &copy.Options{
		SourceCtx:        options.SourceCtx,
		DestinationCtx:   options.DestinationCtx,
		OciDecryptConfig: options.OciDecryptConfig,
		ProgressInterval: options.ProgressInterval,
		Progress:         progress,
	}
}

func (svc *imageService) copyImage(systemContext *types.SystemContext, imageName, parentCgroup string, options *ImageCopyOptions) error {
	progress := options.Progress
	dest := imageName
	// the first argument DEST is not used by the re-execed command but it is useful for debugging as it
	// shows in the ps output.
	cmd := reexec.CommandContext(svc.ctx, "crio-copy-image", dest)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error getting stdout pipe for image copy process: %w", err)
	}
	defer stdout.Close()

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("error getting stderr pipe for image copy process: %w", err)
	}
	defer stderr.Close()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("error getting stdin pipe for image copy process: %w", err)
	}

	if _, err := alltransports.ParseImageName(imageName); err != nil {
		if svc.lookup.DefaultTransport == "" {
			return err
		}
		imageName = svc.lookup.DefaultTransport + imageName
	}

	stdinArguments := copyImageArgs{
		Lookup:        svc.lookup,
		SystemContext: systemContext,
		Options:       options,
		ImageName:     imageName,
		ParentCgroup:  parentCgroup,
		StoreOptions: storage.StoreOptions{
			RunRoot:            svc.store.RunRoot(),
			GraphRoot:          svc.store.GraphRoot(),
			GraphDriverName:    svc.store.GraphDriverName(),
			GraphDriverOptions: svc.store.GraphOptions(),
			UIDMap:             svc.store.UIDMap(),
			GIDMap:             svc.store.GIDMap(),
		},
		HasCollectMode: node.SystemdHasCollectMode(),
	}

	stdinArguments.Options.Progress = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := json.NewEncoder(stdin).Encode(&stdinArguments); err != nil {
		stdin.Close()
		if waitErr := cmd.Wait(); waitErr != nil {
			return fmt.Errorf("%v: %w", waitErr, err)
		}
		return fmt.Errorf("json encode to pipe failed: %w", err)
	}
	stdin.Close()

	go func() {
		decoder := json.NewDecoder(bufio.NewReader(stdout))
		if progress != nil {
			defer close(progress)
		}
		for decoder.More() {
			var p types.ProgressProperties
			if err := decoder.Decode(&p); err != nil {
				break
			}

			if progress != nil {
				progress <- p
			}
		}
	}()
	errOutput, errReadAll := io.ReadAll(stderr)
	if err := cmd.Wait(); err != nil {
		if errReadAll == nil && len(errOutput) > 0 {
			return fmt.Errorf("pull image: %s", string(errOutput))
		}
		return err
	}
	return nil
}

func (svc *imageService) PullImage(systemContext *types.SystemContext, imageName string, inputOptions *ImageCopyOptions) (types.ImageReference, error) {
	options := *inputOptions // A shallow copy

	srcSystemContext, srcRef, destRef, err := svc.lookup.getReferences(options.SourceCtx, svc.store, imageName)
	if err != nil {
		return nil, err
	}
	options.SourceCtx = srcSystemContext

	if inputOptions.CgroupPull.UseNewCgroup {
		if err := svc.copyImage(systemContext, imageName, inputOptions.CgroupPull.ParentCgroup, &options); err != nil {
			return nil, err
		}
	} else {
		policy, err := signature.DefaultPolicy(systemContext)
		if err != nil {
			return nil, err
		}
		policyContext, err := signature.NewPolicyContext(policy)
		if err != nil {
			return nil, err
		}

		copyOptions := toCopyOptions(&options, inputOptions.Progress)

		if _, err = copy.Image(svc.ctx, policyContext, destRef, srcRef, copyOptions); err != nil {
			return nil, err
		}
	}
	return destRef, nil
}

func (svc *imageLookupService) getReferences(inputSystemContext *types.SystemContext, store storage.Store, imageName string) (_ *types.SystemContext, srcRef, destRef types.ImageReference, _ error) {
	srcSystemContext, srcRef, err := svc.prepareReference(inputSystemContext, imageName)
	if err != nil {
		return nil, nil, nil, err
	}

	dest := imageName
	if srcRef.DockerReference() != nil {
		dest = srcRef.DockerReference().String()
	}

	destRef, err = istorage.Transport.ParseStoreReference(store, dest)
	if err != nil {
		return nil, nil, nil, err
	}
	return srcSystemContext, srcRef, destRef, nil
}

func (svc *imageService) UntagImage(systemContext *types.SystemContext, nameOrID string) error {
	ref, err := svc.getRef(nameOrID)
	if err != nil {
		return err
	}
	img, err := istorage.Transport.GetStoreImage(svc.store, ref)
	if err != nil {
		return err
	}

	if !strings.HasPrefix(img.ID, nameOrID) {
		namedRef, err := svc.lookup.remoteImageReference(nameOrID)
		if err != nil {
			return err
		}

		name := nameOrID
		if namedRef.DockerReference() != nil {
			name = namedRef.DockerReference().String()
		}

		prunedNames := 0
		for _, imgName := range img.Names {
			if imgName != name && imgName != nameOrID {
				prunedNames += 1
			}
		}

		if prunedNames > 0 {
			return svc.store.RemoveNames(img.ID, []string{name, nameOrID})
		}
	}

	return ref.DeleteImage(svc.ctx, systemContext)
}

func (svc *imageService) GetStore() storage.Store {
	return svc.store
}

func (svc *imageLookupService) isSecureIndex(indexName string) bool {
	if index, ok := svc.IndexConfigs[indexName]; ok {
		return index.secure
	}

	host, _, err := net.SplitHostPort(indexName)
	if err != nil {
		// assume indexName is of the form `host` without the port and go on.
		host = indexName
	}

	addrs, err := net.LookupIP(host)
	if err != nil {
		ip := net.ParseIP(host)
		if ip != nil {
			addrs = []net.IP{ip}
		}

		// if ip == nil, then `host` is neither an IP nor it could be looked up,
		// either because the index is unreachable, or because the index is behind an HTTP proxy.
		// So, len(addrs) == 0 and we're not aborting.
	}

	// Try CIDR notation only if addrs has any elements, i.e. if `host`'s IP could be determined.
	for _, addr := range addrs {
		for _, ipnet := range svc.InsecureRegistryCIDRs {
			// check if the addr falls in the subnet
			if ipnet.Contains(addr) {
				return false
			}
		}
	}

	return true
}

// ResolveNames resolves an image name into a storage image ID or a fully-qualified image name (domain/repo/image:tag).
// Will only return an empty slice if err != nil.
func (svc *imageService) ResolveNames(systemContext *types.SystemContext, imageName string) ([]string, error) {
	// _Maybe_ it's a truncated image ID.  Don't prepend a registry name, then.
	if len(imageName) >= minimumTruncatedIDLength && svc.store != nil {
		if img, err := svc.store.Image(imageName); err == nil && img != nil && strings.HasPrefix(img.ID, imageName) {
			// It's a truncated version of the ID of an image that's present in local storage;
			// we need to expand it.
			return []string{img.ID}, nil
		}
	}
	// This to prevent any image ID to go through this routine
	_, err := reference.ParseNormalizedNamed(imageName)
	if err != nil {
		if strings.Contains(err.Error(), "cannot specify 64-byte hexadecimal strings") {
			return nil, ErrCannotParseImageID
		}
		return nil, err
	}

	// Disable short name alias mode. Will enable it once we settle on a shortname alias table.
	disabled := types.ShortNameModeDisabled
	systemContext.ShortNameMode = &disabled
	resolved, err := shortnames.Resolve(systemContext, imageName)
	if err != nil {
		return nil, err
	}

	if desc := resolved.Description(); len(desc) > 0 {
		logrus.Info(desc)
	}

	images := make([]string, len(resolved.PullCandidates))
	for i := range resolved.PullCandidates {
		// Strip the tag from ambiguous image references that have a
		// digest as well (e.g.  `image:tag@sha256:123...`).  Such
		// image references are supported by docker but, due to their
		// ambiguity, explicitly not by containers/image.
		ref := resolved.PullCandidates[i].Value
		_, isTagged := ref.(reference.NamedTagged)
		canonical, isDigested := ref.(reference.Canonical)
		if isTagged && isDigested {
			canonical, err = reference.WithDigest(reference.TrimNamed(ref), canonical.Digest())
			if err != nil {
				return nil, err
			}
			ref = canonical
		}
		images[i] = ref.String()
	}

	return images, nil
}

func (isl *imageServiceList) GetDefaultImageServer() ImageServer {
	return isl.defaultImageServer
}

func (isl *imageServiceList) SetDefaultImageServer(is ImageServer) {
	isl.defaultImageServer = is
}

func (isl *imageServiceList) GetImageServer(containerID string) ImageServer {
	is, ok := isl.imageServers[containerID]
	if ok {
		return is
	}
	return isl.defaultImageServer
}

func (isl *imageServiceList) SetImageServer(containerID string, is ImageServer) {
	isl.imageServers[containerID] = is
}

func (isl *imageServiceList) DeleteImageServer(containerID string) {
	delete(isl.imageServers, containerID)
}

// ResolveNames will call ResolveNames on each registered ImageServer, starting
// with the default.
// It returns the data provided by the first ImageServer that provides valid
// data.
// It returns an error if no ImageServer could resolve the name.
func (isl *imageServiceList) ResolveNames(systemContext *types.SystemContext, imageName string) ([]string, error) {
	// check the default ImageServer first
	names, err := isl.defaultImageServer.ResolveNames(systemContext, imageName)
	if err == nil {
		return names, nil
	}
	if err != nil && (err == ErrCannotParseImageID || err == ErrImageMultiplyTagged) {
		return []string{}, err
	}

	// if no answer came from it, try the other registered servers
	for _, is := range isl.imageServers {
		names, err := is.ResolveNames(systemContext, imageName)
		if err != nil {
			if err == ErrCannotParseImageID || err == ErrImageMultiplyTagged {
				return []string{}, err
			}
			continue
		}
		return names, nil
	}
	return []string{}, fmt.Errorf("failed resolving image name for %s - not found", imageName)
}

func (isl *imageServiceList) ListImages(systemContext *types.SystemContext, filter string) (imageResults []ImageResult, lastError error) {
	// first call ListImages on the default ImageServer
	imageResults, lastError = isl.defaultImageServer.ListImages(systemContext, filter)

	// then call it on each container-specific ImageServer
	for _, is := range isl.imageServers {
		images, err := is.ListImages(systemContext, filter)
		if err != nil {
			if lastError == nil {
				lastError = err
			} else {
				lastError = errors.Wrap(lastError, err.Error())
			}
			continue
		}
		imageResults = append(imageResults, images...)
	}
	return imageResults, lastError
}

func (isl *imageServiceList) UntagImage(systemContext *types.SystemContext, nameOrID string) (lastError error) {
	// start with the default ImageServer
	_, e := isl.defaultImageServer.GetStore().Image(nameOrID)
	if e == nil {
		lastError = isl.defaultImageServer.UntagImage(systemContext, nameOrID)
	}

	for _, is := range isl.imageServers {
		_, e := is.GetStore().Image(nameOrID)
		if e != nil {
			continue
		}
		err := is.UntagImage(systemContext, nameOrID)
		if err != nil {
			if lastError == nil {
				lastError = err
			} else {
				lastError = errors.Wrap(lastError, err.Error())
			}
		}
	}
	return lastError
}

// ImageStatus returns the image status for the given image.
func (isl *imageServiceList) ImageStatus(systemContext *types.SystemContext, filter string) (*ImageResult, error) {
	// start with the default ImageServer
	status, lastError := isl.defaultImageServer.ImageStatus(systemContext, filter)
	if lastError == nil {
		return status, nil
	}

	for _, is := range isl.imageServers {
		status, err := is.ImageStatus(systemContext, filter)
		if err == nil {
			return status, nil
		}
		if lastError == nil {
			lastError = err
		} else {
			lastError = errors.Wrap(lastError, err.Error())
		}
	}

	return nil, lastError
}

// GetImageService returns an ImageServer that uses the passed-in store, and
// which will prepend the passed-in DefaultTransport value to an image name if
// a name that's passed to its PullImage() method can't be resolved to an image
// in the store and can't be resolved to a source on its own.
func GetImageService(ctx context.Context, sc *types.SystemContext, store storage.Store, defaultTransport string, insecureRegistries []string) (ImageServer, error) {
	if store == nil {
		var err error
		storeOpts, err := storage.DefaultStoreOptions(rootless.IsRootless(), rootless.GetRootlessUID())
		if err != nil {
			return nil, err
		}
		store, err = storage.GetStore(storeOpts)
		if err != nil {
			return nil, err
		}
	}
	ils := &imageLookupService{
		DefaultTransport:      defaultTransport,
		IndexConfigs:          make(map[string]*indexInfo),
		InsecureRegistryCIDRs: make([]*net.IPNet, 0),
	}
	is := &imageService{
		lookup:     ils,
		store:      store,
		imageCache: make(map[string]imageCacheItem),
		ctx:        ctx,
	}

	insecureRegistries = append(insecureRegistries, "127.0.0.0/8")
	// Split --insecure-registry into CIDR and registry-specific settings.
	for _, r := range insecureRegistries {
		// Check if CIDR was passed to --insecure-registry
		_, ipnet, err := net.ParseCIDR(r)
		if err == nil {
			// Valid CIDR.
			is.lookup.InsecureRegistryCIDRs = append(is.lookup.InsecureRegistryCIDRs, ipnet)
		} else {
			// Assume `host:port` if not CIDR.
			is.lookup.IndexConfigs[r] = &indexInfo{
				name:   r,
				secure: false,
			}
		}
	}

	return is, nil
}

func GetImageServiceList(is ImageServer) ImageServerList {
	return &imageServiceList{
		defaultImageServer: is,
		imageServers:       make(map[string]ImageServer),
	}
}
