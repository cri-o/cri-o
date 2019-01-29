package image

import (
	"context"
	"fmt"
	"io"

	cp "github.com/containers/image/copy"
	"github.com/containers/image/directory"
	"github.com/containers/image/docker"
	dockerarchive "github.com/containers/image/docker/archive"
	"github.com/containers/image/docker/tarfile"
	ociarchive "github.com/containers/image/oci/archive"
	"github.com/containers/image/pkg/sysregistries"
	is "github.com/containers/image/storage"
	"github.com/containers/image/transports"
	"github.com/containers/image/transports/alltransports"
	"github.com/containers/image/types"
	"github.com/containers/libpod/pkg/registries"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	// DockerArchive is the transport we prepend to an image name
	// when saving to docker-archive
	DockerArchive = dockerarchive.Transport.Name()
	// OCIArchive is the transport we prepend to an image name
	// when saving to oci-archive
	OCIArchive = ociarchive.Transport.Name()
	// DirTransport is the transport for pushing and pulling
	// images to and from a directory
	DirTransport = directory.Transport.Name()
	// DockerTransport is the transport for docker registries
	DockerTransport = docker.Transport.Name()
	// AtomicTransport is the transport for atomic registries
	AtomicTransport = "atomic"
	// DefaultTransport is a prefix that we apply to an image name
	// NOTE: This is a string prefix, not actually a transport name usable for transports.Get();
	// and because syntaxes of image names are transport-dependent, the prefix is not really interchangeable;
	// each user implicitly assumes the appended string is a Docker-like reference.
	DefaultTransport = DockerTransport + "://"
	// DefaultLocalRegistry is the default local registry for local image operations
	// Remote pulls will still use defined registries
	DefaultLocalRegistry = "localhost"
)

// pullRefPair records a pair of prepared image references to pull.
type pullRefPair struct {
	image  string
	srcRef types.ImageReference
	dstRef types.ImageReference
}

// pullGoal represents the prepared image references and decided behavior to be executed by imagePull
type pullGoal struct {
	refPairs             []pullRefPair
	pullAllPairs         bool     // Pull all refPairs instead of stopping on first success.
	usedSearchRegistries bool     // refPairs construction has depended on registries.GetRegistries()
	searchedRegistries   []string // The list of search registries used; set only if usedSearchRegistries
}

// singlePullRefPairGoal returns a no-frills pull goal for the specified reference pair.
func singlePullRefPairGoal(rp pullRefPair) *pullGoal {
	return &pullGoal{
		refPairs:             []pullRefPair{rp},
		pullAllPairs:         false, // Does not really make a difference.
		usedSearchRegistries: false,
		searchedRegistries:   nil,
	}
}

func (ir *Runtime) getPullRefPair(srcRef types.ImageReference, destName string) (pullRefPair, error) {
	decomposedDest, err := decompose(destName)
	if err == nil && !decomposedDest.hasRegistry {
		// If the image doesn't have a registry, set it as the default repo
		ref, err := decomposedDest.referenceWithRegistry(DefaultLocalRegistry)
		if err != nil {
			return pullRefPair{}, err
		}
		destName = ref.String()
	}

	reference := destName
	if srcRef.DockerReference() != nil {
		reference = srcRef.DockerReference().String()
	}
	destRef, err := is.Transport.ParseStoreReference(ir.store, reference)
	if err != nil {
		return pullRefPair{}, errors.Wrapf(err, "error parsing dest reference name %#v", destName)
	}
	return pullRefPair{
		image:  destName,
		srcRef: srcRef,
		dstRef: destRef,
	}, nil
}

// getSinglePullRefPairGoal calls getPullRefPair with the specified parameters, and returns a single-pair goal for the return value.
func (ir *Runtime) getSinglePullRefPairGoal(srcRef types.ImageReference, destName string) (*pullGoal, error) {
	rp, err := ir.getPullRefPair(srcRef, destName)
	if err != nil {
		return nil, err
	}
	return singlePullRefPairGoal(rp), nil
}

// pullGoalFromImageReference returns a pull goal for a single ImageReference, depending on the used transport.
func (ir *Runtime) pullGoalFromImageReference(ctx context.Context, srcRef types.ImageReference, imgName string, sc *types.SystemContext) (*pullGoal, error) {
	// supports pulling from docker-archive, oci, and registries
	switch srcRef.Transport().Name() {
	case DockerArchive:
		archivePath := srcRef.StringWithinTransport()
		tarSource, err := tarfile.NewSourceFromFile(archivePath)
		if err != nil {
			return nil, err
		}
		manifest, err := tarSource.LoadTarManifest()

		if err != nil {
			return nil, errors.Wrapf(err, "error retrieving manifest.json")
		}
		// to pull the first image stored in the tar file
		if len(manifest) == 0 {
			// use the hex of the digest if no manifest is found
			reference, err := getImageDigest(ctx, srcRef, sc)
			if err != nil {
				return nil, err
			}
			return ir.getSinglePullRefPairGoal(srcRef, reference)
		}

		if len(manifest[0].RepoTags) == 0 {
			// If the input image has no repotags, we need to feed it a dest anyways
			digest, err := getImageDigest(ctx, srcRef, sc)
			if err != nil {
				return nil, err
			}
			return ir.getSinglePullRefPairGoal(srcRef, digest)
		}

		// Need to load in all the repo tags from the manifest
		res := []pullRefPair{}
		for _, dst := range manifest[0].RepoTags {
			pullInfo, err := ir.getPullRefPair(srcRef, dst)
			if err != nil {
				return nil, err
			}
			res = append(res, pullInfo)
		}
		return &pullGoal{
			refPairs:             res,
			pullAllPairs:         true,
			usedSearchRegistries: false,
			searchedRegistries:   nil,
		}, nil

	case OCIArchive:
		// retrieve the manifest from index.json to access the image name
		manifest, err := ociarchive.LoadManifestDescriptor(srcRef)
		if err != nil {
			return nil, errors.Wrapf(err, "error loading manifest for %q", srcRef)
		}

		var dest string
		if manifest.Annotations == nil || manifest.Annotations["org.opencontainers.image.ref.name"] == "" {
			// If the input image has no image.ref.name, we need to feed it a dest anyways
			// use the hex of the digest
			dest, err = getImageDigest(ctx, srcRef, sc)
			if err != nil {
				return nil, errors.Wrapf(err, "error getting image digest; image reference not found")
			}
		} else {
			dest = manifest.Annotations["org.opencontainers.image.ref.name"]
		}
		return ir.getSinglePullRefPairGoal(srcRef, dest)

	case DirTransport:
		path := srcRef.StringWithinTransport()
		image := path
		if image[:1] == "/" {
			// Set localhost as the registry so docker.io isn't prepended, and the path becomes the repository
			image = DefaultLocalRegistry + image
		}
		return ir.getSinglePullRefPairGoal(srcRef, image)

	default:
		return ir.getSinglePullRefPairGoal(srcRef, imgName)
	}
}

// pullImageFromHeuristicSource pulls an image based on inputName, which is heuristically parsed and may involve configured registries.
// Use pullImageFromReference if the source is known precisely.
func (ir *Runtime) pullImageFromHeuristicSource(ctx context.Context, inputName string, writer io.Writer, authfile, signaturePolicyPath string, signingOptions SigningOptions, dockerOptions *DockerRegistryOptions) ([]string, error) {
	var goal *pullGoal
	sc := GetSystemContext(signaturePolicyPath, authfile, false)
	srcRef, err := alltransports.ParseImageName(inputName)
	if err != nil {
		// could be trying to pull from registry with short name
		goal, err = ir.pullGoalFromPossiblyUnqualifiedName(inputName)
		if err != nil {
			return nil, errors.Wrap(err, "error getting default registries to try")
		}
	} else {
		goal, err = ir.pullGoalFromImageReference(ctx, srcRef, inputName, sc)
		if err != nil {
			return nil, errors.Wrapf(err, "error determining pull goal for image %q", inputName)
		}
	}
	return ir.doPullImage(ctx, sc, *goal, writer, signingOptions, dockerOptions)
}

// pullImageFromReference pulls an image from a types.imageReference.
func (ir *Runtime) pullImageFromReference(ctx context.Context, srcRef types.ImageReference, writer io.Writer, authfile, signaturePolicyPath string, signingOptions SigningOptions, dockerOptions *DockerRegistryOptions) ([]string, error) {
	sc := GetSystemContext(signaturePolicyPath, authfile, false)
	goal, err := ir.pullGoalFromImageReference(ctx, srcRef, transports.ImageName(srcRef), sc)
	if err != nil {
		return nil, errors.Wrapf(err, "error determining pull goal for image %q", transports.ImageName(srcRef))
	}
	return ir.doPullImage(ctx, sc, *goal, writer, signingOptions, dockerOptions)
}

// doPullImage is an internal helper interpreting pullGoal. Almost everyone should call one of the callers of doPullImage instead.
func (ir *Runtime) doPullImage(ctx context.Context, sc *types.SystemContext, goal pullGoal, writer io.Writer, signingOptions SigningOptions, dockerOptions *DockerRegistryOptions) ([]string, error) {
	policyContext, err := getPolicyContext(sc)
	if err != nil {
		return nil, err
	}
	defer policyContext.Destroy()

	systemRegistriesConfPath := registries.SystemRegistriesConfPath()
	var images []string
	var pullErrors *multierror.Error
	for _, imageInfo := range goal.refPairs {
		copyOptions := getCopyOptions(sc, writer, dockerOptions, nil, signingOptions, "", nil)
		copyOptions.SourceCtx.SystemRegistriesConfPath = systemRegistriesConfPath // FIXME: Set this more globally.  Probably no reason not to have it in every types.SystemContext, and to compute the value just once in one place.
		// Print the following statement only when pulling from a docker or atomic registry
		if writer != nil && (imageInfo.srcRef.Transport().Name() == DockerTransport || imageInfo.srcRef.Transport().Name() == AtomicTransport) {
			io.WriteString(writer, fmt.Sprintf("Trying to pull %s...", imageInfo.image))
		}
		_, err = cp.Image(ctx, policyContext, imageInfo.dstRef, imageInfo.srcRef, copyOptions)
		if err != nil {
			pullErrors = multierror.Append(pullErrors, err)
			logrus.Debugf("Error pulling image ref %s: %v", imageInfo.srcRef.StringWithinTransport(), err)
			if writer != nil {
				io.WriteString(writer, "Failed\n")
			}
		} else {
			if !goal.pullAllPairs {
				return []string{imageInfo.image}, nil
			}
			images = append(images, imageInfo.image)
		}
	}
	// If no image was found, we should handle.  Lets be nicer to the user and see if we can figure out why.
	if len(images) == 0 {
		registryPath := sysregistries.RegistriesConfPath(&types.SystemContext{SystemRegistriesConfPath: systemRegistriesConfPath})
		if goal.usedSearchRegistries && len(goal.searchedRegistries) == 0 {
			return nil, errors.Errorf("image name provided is a short name and no search registries are defined in %s.", registryPath)
		}
		// If the image passed in was fully-qualified, we will have 1 refpair.  Bc the image is fq'd, we dont need to yap about registries.
		if !goal.usedSearchRegistries {
			if pullErrors != nil && len(pullErrors.Errors) > 0 { // this should always be true
				return nil, errors.Wrap(pullErrors.Errors[0], "unable to pull image")
			}
			return nil, errors.Errorf("unable to pull image, or you do not have pull access")
		}
		return nil, pullErrors
	}
	return images, nil
}

// pullGoalFromPossiblyUnqualifiedName looks at inputName and determines the possible
// image references to try pulling in combination with the registries.conf file as well
func (ir *Runtime) pullGoalFromPossiblyUnqualifiedName(inputName string) (*pullGoal, error) {
	decomposedImage, err := decompose(inputName)
	if err != nil {
		return nil, err
	}
	if decomposedImage.hasRegistry {
		srcRef, err := docker.ParseReference("//" + inputName)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to parse '%s'", inputName)
		}
		return ir.getSinglePullRefPairGoal(srcRef, inputName)
	}

	searchRegistries, err := registries.GetRegistries()
	if err != nil {
		return nil, err
	}
	var refPairs []pullRefPair
	for _, registry := range searchRegistries {
		ref, err := decomposedImage.referenceWithRegistry(registry)
		if err != nil {
			return nil, err
		}
		imageName := ref.String()
		srcRef, err := docker.ParseReference("//" + imageName)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to parse '%s'", imageName)
		}
		ps, err := ir.getPullRefPair(srcRef, imageName)
		if err != nil {
			return nil, err
		}
		refPairs = append(refPairs, ps)
	}
	return &pullGoal{
		refPairs:             refPairs,
		pullAllPairs:         false,
		usedSearchRegistries: true,
		searchedRegistries:   searchRegistries,
	}, nil
}
