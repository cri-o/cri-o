package image

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"syscall"
	"time"

	types2 "github.com/containernetworking/cni/pkg/types"
	cp "github.com/containers/image/copy"
	"github.com/containers/image/docker/reference"
	"github.com/containers/image/manifest"
	is "github.com/containers/image/storage"
	"github.com/containers/image/tarball"
	"github.com/containers/image/transports"
	"github.com/containers/image/transports/alltransports"
	"github.com/containers/image/types"
	"github.com/containers/libpod/libpod/common"
	"github.com/containers/libpod/libpod/driver"
	"github.com/containers/libpod/pkg/inspect"
	"github.com/containers/libpod/pkg/registries"
	"github.com/containers/libpod/pkg/util"
	"github.com/containers/storage"
	"github.com/containers/storage/pkg/reexec"
	digest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// imageConversions is used to cache image "cast" types
type imageConversions struct {
	imgRef   types.Image
	storeRef types.ImageReference
}

// Image is the primary struct for dealing with images
// It is still very much a work in progress
type Image struct {
	// Adding these two structs for now but will cull when we near
	// completion of this library.
	imageConversions
	inspect.ImageData
	inspect.ImageResult
	inspectInfo *types.ImageInspectInfo
	InputName   string
	Local       bool
	//runtime   *libpod.Runtime
	image        *storage.Image
	imageruntime *Runtime
	repotagsMap  map[string][]string
}

// Runtime contains the store
type Runtime struct {
	store               storage.Store
	SignaturePolicyPath string
}

// ErrRepoTagNotFound is the error returned when the image id given doesn't match a rep tag in store
var ErrRepoTagNotFound = errors.New("unable to match user input to any specific repotag")

// NewImageRuntimeFromStore creates an ImageRuntime based on a provided store
func NewImageRuntimeFromStore(store storage.Store) *Runtime {
	return &Runtime{
		store: store,
	}
}

// NewImageRuntimeFromOptions creates an Image Runtime including the store given
// store options
func NewImageRuntimeFromOptions(options storage.StoreOptions) (*Runtime, error) {
	if reexec.Init() {
		return nil, errors.Errorf("unable to reexec")
	}
	store, err := setStore(options)
	if err != nil {
		return nil, err
	}

	return &Runtime{
		store: store,
	}, nil
}

func setStore(options storage.StoreOptions) (storage.Store, error) {
	store, err := storage.GetStore(options)
	if err != nil {
		return nil, err
	}
	is.Transport.SetStore(store)
	return store, nil
}

// newFromStorage creates a new image object from a storage.Image
func (ir *Runtime) newFromStorage(img *storage.Image) *Image {
	image := Image{
		InputName:    img.ID,
		Local:        true,
		imageruntime: ir,
		image:        img,
	}
	return &image
}

// NewFromLocal creates a new image object that is intended
// to only deal with local images already in the store (or
// its aliases)
func (ir *Runtime) NewFromLocal(name string) (*Image, error) {
	image := Image{
		InputName:    name,
		Local:        true,
		imageruntime: ir,
	}
	localImage, err := image.getLocalImage()
	if err != nil {
		return nil, err
	}
	image.image = localImage
	return &image, nil
}

// New creates a new image object where the image could be local
// or remote
func (ir *Runtime) New(ctx context.Context, name, signaturePolicyPath, authfile string, writer io.Writer, dockeroptions *DockerRegistryOptions, signingoptions SigningOptions, forcePull bool) (*Image, error) {
	// We don't know if the image is local or not ... check local first
	newImage := Image{
		InputName:    name,
		Local:        false,
		imageruntime: ir,
	}
	if !forcePull {
		localImage, err := newImage.getLocalImage()
		if err == nil {
			newImage.Local = true
			newImage.image = localImage
			return &newImage, nil
		}
	}

	// The image is not local
	if signaturePolicyPath == "" {
		signaturePolicyPath = ir.SignaturePolicyPath
	}
	imageName, err := ir.pullImageFromHeuristicSource(ctx, name, writer, authfile, signaturePolicyPath, signingoptions, dockeroptions)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to pull %s", name)
	}

	newImage.InputName = imageName[0]
	img, err := newImage.getLocalImage()
	if err != nil {
		return nil, errors.Wrapf(err, "error retrieving local image after pulling %s", name)
	}
	newImage.image = img
	return &newImage, nil
}

// LoadFromArchiveReference creates a new image object for images pulled from a tar archive and the like (podman load)
// This function is needed because it is possible for a tar archive to have multiple tags for one image
func (ir *Runtime) LoadFromArchiveReference(ctx context.Context, srcRef types.ImageReference, signaturePolicyPath string, writer io.Writer) ([]*Image, error) {
	var newImages []*Image

	if signaturePolicyPath == "" {
		signaturePolicyPath = ir.SignaturePolicyPath
	}
	imageNames, err := ir.pullImageFromReference(ctx, srcRef, writer, "", signaturePolicyPath, SigningOptions{}, &DockerRegistryOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "unable to pull %s", transports.ImageName(srcRef))
	}

	for _, name := range imageNames {
		newImage := Image{
			InputName:    name,
			Local:        true,
			imageruntime: ir,
		}
		img, err := newImage.getLocalImage()
		if err != nil {
			return nil, errors.Wrapf(err, "error retrieving local image after pulling %s", name)
		}
		newImage.image = img
		newImages = append(newImages, &newImage)
	}

	return newImages, nil
}

// Shutdown closes down the storage and require a bool arg as to
// whether it should do so forcibly.
func (ir *Runtime) Shutdown(force bool) error {
	_, err := ir.store.Shutdown(force)
	return err
}

func (i *Image) reloadImage() error {
	newImage, err := i.imageruntime.getImage(i.ID())
	if err != nil {
		return errors.Wrapf(err, "unable to reload image")
	}
	i.image = newImage.image
	return nil
}

// stringSha256 strips sha256 from user input
func stripSha256(name string) string {
	if strings.HasPrefix(name, "sha256:") && len(name) > 7 {
		return name[7:]
	}
	return name
}

// getLocalImage resolves an unknown input describing an image and
// returns a storage.Image or an error. It is used by NewFromLocal.
func (i *Image) getLocalImage() (*storage.Image, error) {
	imageError := fmt.Sprintf("unable to find '%s' in local storage", i.InputName)
	if i.InputName == "" {
		return nil, errors.Errorf("input name is blank")
	}
	// Check if the input name has a transport and if so strip it
	dest, err := alltransports.ParseImageName(i.InputName)
	if err == nil && dest.DockerReference() != nil {
		i.InputName = dest.DockerReference().String()
	}

	img, err := i.imageruntime.getImage(stripSha256(i.InputName))
	if err == nil {
		return img.image, err
	}

	// container-storage wasn't able to find it in its current form
	// check if the input name has a tag, and if not, run it through
	// again
	decomposedImage, err := decompose(i.InputName)
	if err != nil {
		return nil, err
	}

	// The image has a registry name in it and we made sure we looked for it locally
	// with a tag.  It cannot be local.
	if decomposedImage.hasRegistry {
		return nil, errors.Wrapf(ErrNoSuchImage, imageError)
	}
	// if the image is saved with the repository localhost, searching with localhost prepended is necessary
	// We don't need to strip the sha because we have already determined it is not an ID
	ref, err := decomposedImage.referenceWithRegistry(DefaultLocalRegistry)
	if err != nil {
		return nil, err
	}
	img, err = i.imageruntime.getImage(ref.String())
	if err == nil {
		return img.image, err
	}

	// grab all the local images
	images, err := i.imageruntime.GetImages()
	if err != nil {
		return nil, err
	}

	// check the repotags of all images for a match
	repoImage, err := findImageInRepotags(decomposedImage, images)
	if err == nil {
		return repoImage, nil
	}

	return nil, errors.Wrapf(ErrNoSuchImage, err.Error())
}

// ID returns the image ID as a string
func (i *Image) ID() string {
	return i.image.ID
}

// Digest returns the image's digest
func (i *Image) Digest() digest.Digest {
	return i.image.Digest
}

// Manifest returns the image's manifest as a byte array
// and manifest type as a string.  The manifest type is
// MediaTypeImageManifest from ociv1.
func (i *Image) Manifest(ctx context.Context) ([]byte, string, error) {
	imgRef, err := i.toImageRef(ctx)
	if err != nil {
		return nil, "", err
	}
	return imgRef.Manifest(ctx)
}

// Names returns a string array of names associated with the image
func (i *Image) Names() []string {
	return i.image.Names
}

// RepoDigests returns a string array of repodigests associated with the image
func (i *Image) RepoDigests() ([]string, error) {
	var repoDigests []string
	digest := i.Digest()

	for _, name := range i.Names() {
		named, err := reference.ParseNormalizedNamed(name)
		if err != nil {
			return nil, err
		}

		canonical, err := reference.WithDigest(reference.TrimNamed(named), digest)
		if err != nil {
			return nil, err
		}

		repoDigests = append(repoDigests, canonical.String())
	}
	return repoDigests, nil
}

// Created returns the time the image was created
func (i *Image) Created() time.Time {
	return i.image.Created
}

// TopLayer returns the top layer id as a string
func (i *Image) TopLayer() string {
	return i.image.TopLayer
}

// Remove an image; container removal for the image must be done
// outside the context of images
// TODO: the force param does nothing as of now. Need to move container
// handling logic here eventually.
func (i *Image) Remove(force bool) error {
	parent, err := i.GetParent()
	if err != nil {
		return err
	}
	if _, err := i.imageruntime.store.DeleteImage(i.ID(), true); err != nil {
		return err
	}
	for parent != nil {
		nextParent, err := parent.GetParent()
		if err != nil {
			return err
		}
		children, err := parent.GetChildren()
		if err != nil {
			return err
		}
		// Do not remove if image is a base image and is not untagged, or if
		// the image has more children.
		if len(children) > 0 || len(parent.Names()) > 0 {
			return nil
		}
		id := parent.ID()
		if _, err := i.imageruntime.store.DeleteImage(id, true); err != nil {
			logrus.Debugf("unable to remove intermediate image %q: %v", id, err)
		} else {
			fmt.Println(id)
		}
		parent = nextParent
	}
	return nil
}

// Decompose an Image
func (i *Image) Decompose() error {
	return types2.NotImplementedError
}

// TODO: Rework this method to not require an assembly of the fq name with transport
/*
// GetManifest tries to GET an images manifest, returns nil on success and err on failure
func (i *Image) GetManifest() error {
	pullRef, err := alltransports.ParseImageName(i.assembleFqNameTransport())
	if err != nil {
		return errors.Errorf("unable to parse '%s'", i.Names()[0])
	}
	imageSource, err := pullRef.NewImageSource(nil)
	if err != nil {
		return errors.Wrapf(err, "unable to create new image source")
	}
	_, _, err = imageSource.GetManifest(nil)
	if err == nil {
		return nil
	}
	return err
}
*/

// getImage retrieves an image matching the given name or hash from system
// storage
// If no matching image can be found, an error is returned
func (ir *Runtime) getImage(image string) (*Image, error) {
	var img *storage.Image
	ref, err := is.Transport.ParseStoreReference(ir.store, image)
	if err == nil {
		img, err = is.Transport.GetStoreImage(ir.store, ref)
	}
	if err != nil {
		img2, err2 := ir.store.Image(image)
		if err2 != nil {
			if ref == nil {
				return nil, errors.Wrapf(err, "error parsing reference to image %q", image)
			}
			return nil, errors.Wrapf(err, "unable to locate image %q", image)
		}
		img = img2
	}
	newImage := ir.newFromStorage(img)
	return newImage, nil
}

// GetImages retrieves all images present in storage
func (ir *Runtime) GetImages() ([]*Image, error) {
	var newImages []*Image
	images, err := ir.store.Images()
	if err != nil {
		return nil, err
	}
	for _, i := range images {
		// iterating over these, be careful to not iterate on the literal
		// pointer.
		image := i
		img := ir.newFromStorage(&image)
		newImages = append(newImages, img)
	}
	return newImages, nil
}

// getImageDigest creates an image object and uses the hex value of the digest as the image ID
// for parsing the store reference
func getImageDigest(ctx context.Context, src types.ImageReference, sc *types.SystemContext) (string, error) {
	newImg, err := src.NewImage(ctx, sc)
	if err != nil {
		return "", err
	}
	defer newImg.Close()
	digest := newImg.ConfigInfo().Digest
	if err = digest.Validate(); err != nil {
		return "", errors.Wrapf(err, "error getting config info")
	}
	return "@" + digest.Hex(), nil
}

// normalizedTag returns the canonical version of tag for use in Image.Names()
func normalizedTag(tag string) (reference.Named, error) {
	decomposedTag, err := decompose(tag)
	if err != nil {
		return nil, err
	}
	// If the input doesn't specify a registry, set the registry to localhost
	var ref reference.Named
	if !decomposedTag.hasRegistry {
		ref, err = decomposedTag.referenceWithRegistry(DefaultLocalRegistry)
		if err != nil {
			return nil, err
		}
	} else {
		ref, err = decomposedTag.normalizedReference()
		if err != nil {
			return nil, err
		}
	}
	// If the input does not have a tag, we need to add one (latest)
	ref = reference.TagNameOnly(ref)
	return ref, nil
}

// TagImage adds a tag to the given image
func (i *Image) TagImage(tag string) error {
	i.reloadImage()
	ref, err := normalizedTag(tag)
	if err != nil {
		return err
	}
	tags := i.Names()
	if util.StringInSlice(ref.String(), tags) {
		return nil
	}
	tags = append(tags, ref.String())
	if err := i.imageruntime.store.SetNames(i.ID(), tags); err != nil {
		return err
	}
	i.reloadImage()
	return nil
}

// UntagImage removes a tag from the given image
func (i *Image) UntagImage(tag string) error {
	i.reloadImage()
	var newTags []string
	tags := i.Names()
	if !util.StringInSlice(tag, tags) {
		return nil
	}
	for _, t := range tags {
		if tag != t {
			newTags = append(newTags, t)
		}
	}
	if err := i.imageruntime.store.SetNames(i.ID(), newTags); err != nil {
		return err
	}
	i.reloadImage()
	return nil
}

// PushImageToHeuristicDestination pushes the given image to "destination", which is heuristically parsed.
// Use PushImageToReference if the destination is known precisely.
func (i *Image) PushImageToHeuristicDestination(ctx context.Context, destination, manifestMIMEType, authFile, signaturePolicyPath string, writer io.Writer, forceCompress bool, signingOptions SigningOptions, dockerRegistryOptions *DockerRegistryOptions, additionalDockerArchiveTags []reference.NamedTagged) error {
	if destination == "" {
		return errors.Wrapf(syscall.EINVAL, "destination image name must be specified")
	}

	// Get the destination Image Reference
	dest, err := alltransports.ParseImageName(destination)
	if err != nil {
		if hasTransport(destination) {
			return errors.Wrapf(err, "error getting destination imageReference for %q", destination)
		}
		// Try adding the images default transport
		destination2 := DefaultTransport + destination
		dest, err = alltransports.ParseImageName(destination2)
		if err != nil {
			return err
		}
	}
	return i.PushImageToReference(ctx, dest, manifestMIMEType, authFile, signaturePolicyPath, writer, forceCompress, signingOptions, dockerRegistryOptions, additionalDockerArchiveTags)
}

// PushImageToReference pushes the given image to a location described by the given path
func (i *Image) PushImageToReference(ctx context.Context, dest types.ImageReference, manifestMIMEType, authFile, signaturePolicyPath string, writer io.Writer, forceCompress bool, signingOptions SigningOptions, dockerRegistryOptions *DockerRegistryOptions, additionalDockerArchiveTags []reference.NamedTagged) error {
	sc := GetSystemContext(signaturePolicyPath, authFile, forceCompress)

	policyContext, err := getPolicyContext(sc)
	if err != nil {
		return err
	}
	defer policyContext.Destroy()

	// Look up the source image, expecting it to be in local storage
	src, err := is.Transport.ParseStoreReference(i.imageruntime.store, i.ID())
	if err != nil {
		return errors.Wrapf(err, "error getting source imageReference for %q", i.InputName)
	}
	copyOptions := getCopyOptions(sc, writer, nil, dockerRegistryOptions, signingOptions, manifestMIMEType, additionalDockerArchiveTags)
	copyOptions.DestinationCtx.SystemRegistriesConfPath = registries.SystemRegistriesConfPath() // FIXME: Set this more globally.  Probably no reason not to have it in every types.SystemContext, and to compute the value just once in one place.
	// Copy the image to the remote destination
	_, err = cp.Image(ctx, policyContext, dest, src, copyOptions)
	if err != nil {
		return errors.Wrapf(err, "Error copying image to the remote destination")
	}
	return nil
}

// MatchesID returns a bool based on if the input id
// matches the image's id
func (i *Image) MatchesID(id string) bool {
	return strings.HasPrefix(i.ID(), id)
}

// toStorageReference returns a *storageReference from an Image
func (i *Image) toStorageReference() (types.ImageReference, error) {
	var lookupName string
	if i.storeRef == nil {
		if i.image != nil {
			lookupName = i.ID()
		} else {
			lookupName = i.InputName
		}
		storeRef, err := is.Transport.ParseStoreReference(i.imageruntime.store, lookupName)
		if err != nil {
			return nil, err
		}
		i.storeRef = storeRef
	}
	return i.storeRef, nil
}

// ToImageRef returns an image reference type from an image
// TODO: Hopefully we can remove this exported function for mheon
func (i *Image) ToImageRef(ctx context.Context) (types.Image, error) {
	return i.toImageRef(ctx)
}

// toImageRef returns an Image Reference type from an image
func (i *Image) toImageRef(ctx context.Context) (types.Image, error) {
	if i == nil {
		return nil, errors.Errorf("cannot convert nil image to image reference")
	}
	if i.imgRef == nil {
		ref, err := is.Transport.ParseStoreReference(i.imageruntime.store, "@"+i.ID())
		if err != nil {
			return nil, errors.Wrapf(err, "error parsing reference to image %q", i.ID())
		}
		imgRef, err := ref.NewImage(ctx, nil)
		if err != nil {
			return nil, errors.Wrapf(err, "error reading image %q", i.ID())
		}
		i.imgRef = imgRef
	}
	return i.imgRef, nil
}

// sizer knows its size.
type sizer interface {
	Size() (int64, error)
}

//Size returns the size of the image
func (i *Image) Size(ctx context.Context) (*uint64, error) {
	storeRef, err := is.Transport.ParseStoreReference(i.imageruntime.store, i.ID())
	if err != nil {
		return nil, err
	}
	systemContext := &types.SystemContext{}
	img, err := storeRef.NewImageSource(ctx, systemContext)
	if err != nil {
		return nil, err
	}
	if s, ok := img.(sizer); ok {
		if sum, err := s.Size(); err == nil {
			usum := uint64(sum)
			return &usum, nil
		}
	}
	return nil, errors.Errorf("unable to determine size")
}

// DriverData gets the driver data from the store on a layer
func (i *Image) DriverData() (*inspect.Data, error) {
	topLayer, err := i.Layer()
	if err != nil {
		return nil, err
	}
	return driver.GetDriverData(i.imageruntime.store, topLayer.ID)
}

// Layer returns the image's top layer
func (i *Image) Layer() (*storage.Layer, error) {
	return i.imageruntime.store.Layer(i.image.TopLayer)
}

// History contains the history information of an image
type History struct {
	ID        string     `json:"id"`
	Created   *time.Time `json:"created"`
	CreatedBy string     `json:"createdBy"`
	Size      int64      `json:"size"`
	Comment   string     `json:"comment"`
}

// History gets the history of an image and information about its layers
func (i *Image) History(ctx context.Context) ([]*History, error) {
	img, err := i.toImageRef(ctx)
	if err != nil {
		return nil, err
	}
	oci, err := img.OCIConfig(ctx)
	if err != nil {
		return nil, err
	}

	// Get the IDs of the images making up the history layers
	// if the images exist locally in the store
	images, err := i.imageruntime.GetImages()
	if err != nil {
		return nil, errors.Wrapf(err, "error getting images from store")
	}
	imageIDs := []string{i.ID()}
	if err := i.historyLayerIDs(i.TopLayer(), images, &imageIDs); err != nil {
		return nil, errors.Wrap(err, "error getting image IDs for layers in history")
	}

	var (
		imageID    string
		imgIDCount = 0
		size       int64
		sizeCount  = 1
		allHistory []*History
	)

	for i := len(oci.History) - 1; i >= 0; i-- {
		if imgIDCount < len(imageIDs) {
			imageID = imageIDs[imgIDCount]
			imgIDCount++
		} else {
			imageID = "<missing>"
		}
		if !oci.History[i].EmptyLayer {
			size = img.LayerInfos()[len(img.LayerInfos())-sizeCount].Size
			sizeCount++
		}
		allHistory = append(allHistory, &History{
			ID:        imageID,
			Created:   oci.History[i].Created,
			CreatedBy: oci.History[i].CreatedBy,
			Size:      size,
			Comment:   oci.History[i].Comment,
		})
	}

	return allHistory, nil
}

// historyLayerIDs goes through the images in store and checks if the top layer of an image
// is the same as the parent of topLayerID
func (i *Image) historyLayerIDs(topLayerID string, images []*Image, IDs *[]string) error {
	for _, image := range images {
		// Get the layer info of topLayerID
		layer, err := i.imageruntime.store.Layer(topLayerID)
		if err != nil {
			return errors.Wrapf(err, "error getting layer info %q", topLayerID)
		}
		// Check if the parent of layer is equal to the image's top layer
		// If so add the image ID to the list of IDs and find the parent of
		// the top layer of the image ID added to the list
		// Since we are checking for parent, each top layer can only have one parent
		if layer.Parent == image.TopLayer() {
			*IDs = append(*IDs, image.ID())
			return i.historyLayerIDs(image.TopLayer(), images, IDs)
		}
	}
	return nil
}

// Dangling returns a bool if the image is "dangling"
func (i *Image) Dangling() bool {
	return len(i.Names()) == 0
}

// Labels returns the image's labels
func (i *Image) Labels(ctx context.Context) (map[string]string, error) {
	imgInspect, err := i.imageInspectInfo(ctx)
	if err != nil {
		return nil, nil
	}
	return imgInspect.Labels, nil
}

// GetLabel Returns a case-insensitive match of a given label
func (i *Image) GetLabel(ctx context.Context, label string) (string, error) {
	imageLabels, err := i.Labels(ctx)
	if err != nil {
		return "", err
	}
	for k, v := range imageLabels {
		if strings.ToLower(k) == strings.ToLower(label) {
			return v, nil
		}
	}
	return "", nil
}

// Annotations returns the annotations of an image
func (i *Image) Annotations(ctx context.Context) (map[string]string, error) {
	manifest, manifestType, err := i.Manifest(ctx)
	if err != nil {
		return nil, err
	}
	annotations := make(map[string]string)
	switch manifestType {
	case ociv1.MediaTypeImageManifest:
		var m ociv1.Manifest
		if err := json.Unmarshal(manifest, &m); err == nil {
			for k, v := range m.Annotations {
				annotations[k] = v
			}
		}
	}
	return annotations, nil
}

// ociv1Image converts and image to an imgref and then an
// ociv1 image type
func (i *Image) ociv1Image(ctx context.Context) (*ociv1.Image, error) {
	imgRef, err := i.toImageRef(ctx)
	if err != nil {
		return nil, err
	}

	return imgRef.OCIConfig(ctx)
}

func (i *Image) imageInspectInfo(ctx context.Context) (*types.ImageInspectInfo, error) {
	if i.inspectInfo == nil {
		sr, err := i.toStorageReference()
		if err != nil {
			return nil, err
		}
		ic, err := sr.NewImage(ctx, &types.SystemContext{})
		if err != nil {
			return nil, err
		}
		imgInspect, err := ic.Inspect(ctx)
		if err != nil {
			return nil, err
		}
		i.inspectInfo = imgInspect
	}
	return i.inspectInfo, nil
}

// Inspect returns an image's inspect data
func (i *Image) Inspect(ctx context.Context) (*inspect.ImageData, error) {
	ociv1Img, err := i.ociv1Image(ctx)
	if err != nil {
		return nil, err
	}
	info, err := i.imageInspectInfo(ctx)
	if err != nil {
		return nil, err
	}
	annotations, err := i.Annotations(ctx)
	if err != nil {
		return nil, err
	}

	size, err := i.Size(ctx)
	if err != nil {
		return nil, err
	}

	var repoDigests []string
	for _, name := range i.Names() {
		repoDigests = append(repoDigests, strings.SplitN(name, ":", 2)[0]+"@"+i.Digest().String())
	}

	driver, err := i.DriverData()
	if err != nil {
		return nil, err
	}

	_, manifestType, err := i.Manifest(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to determine manifest type")
	}
	comment, err := i.Comment(ctx, manifestType)
	if err != nil {
		return nil, err
	}

	data := &inspect.ImageData{
		ID:           i.ID(),
		RepoTags:     i.Names(),
		RepoDigests:  repoDigests,
		Comment:      comment,
		Created:      ociv1Img.Created,
		Author:       ociv1Img.Author,
		Architecture: ociv1Img.Architecture,
		Os:           ociv1Img.OS,
		Config:       &ociv1Img.Config,
		Version:      info.DockerVersion,
		Size:         int64(*size),
		VirtualSize:  int64(*size),
		Annotations:  annotations,
		Digest:       i.Digest(),
		Labels:       info.Labels,
		RootFS: &inspect.RootFS{
			Type:   ociv1Img.RootFS.Type,
			Layers: ociv1Img.RootFS.DiffIDs,
		},
		GraphDriver:  driver,
		ManifestType: manifestType,
		User:         ociv1Img.Config.User,
		History:      ociv1Img.History,
	}
	return data, nil
}

// Import imports and image into the store and returns an image
func (ir *Runtime) Import(ctx context.Context, path, reference string, writer io.Writer, signingOptions SigningOptions, imageConfig ociv1.Image) (*Image, error) {
	src, err := tarball.Transport.ParseReference(path)
	if err != nil {
		return nil, errors.Wrapf(err, "error parsing image name %q", path)
	}

	updater, ok := src.(tarball.ConfigUpdater)
	if !ok {
		return nil, errors.Wrapf(err, "unexpected type, a tarball reference should implement tarball.ConfigUpdater")
	}

	annotations := make(map[string]string)

	//	config imgspecv1.Image
	err = updater.ConfigUpdate(imageConfig, annotations)
	if err != nil {
		return nil, errors.Wrapf(err, "error updating image config")
	}

	sc := common.GetSystemContext("", "", false)

	// if reference not given, get the image digest
	if reference == "" {
		reference, err = getImageDigest(ctx, src, sc)
		if err != nil {
			return nil, err
		}
	}
	policyContext, err := getPolicyContext(sc)
	if err != nil {
		return nil, err
	}
	defer policyContext.Destroy()
	copyOptions := getCopyOptions(sc, writer, nil, nil, signingOptions, "", nil)
	dest, err := is.Transport.ParseStoreReference(ir.store, reference)
	if err != nil {
		errors.Wrapf(err, "error getting image reference for %q", reference)
	}
	_, err = cp.Image(ctx, policyContext, dest, src, copyOptions)
	if err != nil {
		return nil, err
	}
	return ir.NewFromLocal(reference)
}

// MatchRepoTag takes a string and tries to match it against an
// image's repotags
func (i *Image) MatchRepoTag(input string) (string, error) {
	results := make(map[int][]string)
	var maxCount int
	// first check if we have an exact match with the input
	if util.StringInSlice(input, i.Names()) {
		return input, nil
	}
	// next check if we are missing the tag
	dcImage, err := decompose(input)
	if err != nil {
		return "", err
	}
	imageRegistry, imageName, imageSuspiciousTagValueForSearch := dcImage.suspiciousRefNameTagValuesForSearch()
	for _, repoName := range i.Names() {
		count := 0
		dcRepoName, err := decompose(repoName)
		if err != nil {
			return "", err
		}
		repoNameRegistry, repoNameName, repoNameSuspiciousTagValueForSearch := dcRepoName.suspiciousRefNameTagValuesForSearch()
		if repoNameRegistry == imageRegistry && imageRegistry != "" {
			count++
		}
		if repoNameName == imageName && imageName != "" {
			count++
		} else if splitString(repoNameName) == splitString(imageName) {
			count++
		}
		if repoNameSuspiciousTagValueForSearch == imageSuspiciousTagValueForSearch {
			count++
		}
		results[count] = append(results[count], repoName)
		if count > maxCount {
			maxCount = count
		}
	}
	if maxCount == 0 {
		return "", ErrRepoTagNotFound
	}
	if len(results[maxCount]) > 1 {
		return "", errors.Errorf("user input matched multiple repotags for the image")
	}
	return results[maxCount][0], nil
}

// splitString splits input string by / and returns the last array item
func splitString(input string) string {
	split := strings.Split(input, "/")
	return split[len(split)-1]
}

// IsParent goes through the layers in the store and checks if i.TopLayer is
// the parent of any other layer in store. Double check that image with that
// layer exists as well.
func (i *Image) IsParent() (bool, error) {
	children, err := i.GetChildren()
	if err != nil {
		return false, err
	}
	return len(children) > 0, nil
}

// GetParent returns the image ID of the parent. Return nil if a parent is not found.
func (i *Image) GetParent() (*Image, error) {
	images, err := i.imageruntime.GetImages()
	if err != nil {
		return nil, err
	}
	layer, err := i.imageruntime.store.Layer(i.TopLayer())
	if err != nil {
		return nil, err
	}
	for _, img := range images {
		if img.TopLayer() == layer.Parent {
			return img, nil
		}
	}
	return nil, nil
}

// GetChildren returns a list of the imageIDs that depend on the image
func (i *Image) GetChildren() ([]string, error) {
	var children []string
	images, err := i.imageruntime.GetImages()
	if err != nil {
		return nil, err
	}
	layers, err := i.imageruntime.store.Layers()
	if err != nil {
		return nil, err
	}

	for _, layer := range layers {
		if layer.Parent == i.TopLayer() {
			if imageID := getImageOfTopLayer(images, layer.ID); len(imageID) > 0 {
				children = append(children, imageID...)
			}
		}
	}
	return children, nil
}

// getImageOfTopLayer returns the image ID where layer is the top layer of the image
func getImageOfTopLayer(images []*Image, layer string) []string {
	var matches []string
	for _, img := range images {
		if img.TopLayer() == layer {
			matches = append(matches, img.ID())
		}
	}
	return matches
}

// InputIsID returns a bool if the user input for an image
// is the image's partial or full id
func (i *Image) InputIsID() bool {
	return strings.HasPrefix(i.ID(), i.InputName)
}

// Containers a list of container IDs associated with the image
func (i *Image) Containers() ([]string, error) {
	containers, err := i.imageruntime.store.Containers()
	if err != nil {
		return nil, err
	}
	var imageContainers []string
	for _, c := range containers {
		if c.ImageID == i.ID() {
			imageContainers = append(imageContainers, c.ID)
		}
	}
	return imageContainers, err
}

// Comment returns the Comment for an image depending on its ManifestType
func (i *Image) Comment(ctx context.Context, manifestType string) (string, error) {
	if manifestType == manifest.DockerV2Schema2MediaType {
		imgRef, err := i.toImageRef(ctx)
		if err != nil {
			return "", errors.Wrapf(err, "unable to create image reference from image")
		}
		blob, err := imgRef.ConfigBlob(ctx)
		if err != nil {
			return "", errors.Wrapf(err, "unable to get config blob from image")
		}
		b := manifest.Schema2Image{}
		if err := json.Unmarshal(blob, &b); err != nil {
			return "", err
		}
		return b.Comment, nil
	}
	ociv1Img, err := i.ociv1Image(ctx)
	if err != nil {
		return "", err
	}
	return ociv1Img.History[0].Comment, nil
}
