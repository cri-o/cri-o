package image

import (
	"io"
	"os"
	"strings"
	"syscall"

	cp "github.com/containers/image/copy"
	"github.com/containers/image/docker/tarfile"
	"github.com/containers/image/manifest"
	"github.com/containers/image/signature"
	is "github.com/containers/image/storage"
	"github.com/containers/image/transports/alltransports"
	"github.com/containers/image/types"
	"github.com/containers/storage"
	"github.com/containers/storage/pkg/archive"
	"github.com/kubernetes-incubator/cri-o/libkpod/common"
	"github.com/pkg/errors"
)

const (
	// DefaultRegistry is a prefix that we apply to an image name
	// to check docker hub first for the image
	DefaultRegistry = "docker://"
)

// CopyOptions contains the options given when pushing or pulling images
type CopyOptions struct {
	// Compression specifies the type of compression which is applied to
	// layer blobs.  The default is to not use compression, but
	// archive.Gzip is recommended.
	Compression archive.Compression
	// SignaturePolicyPath specifies an override location for the signature
	// policy which should be used for verifying the new image as it is
	// being written.  Except in specific circumstances, no value should be
	// specified, indicating that the shared, system-wide default policy
	// should be used.
	SignaturePolicyPath string
	// ReportWriter is an io.Writer which will be used to log the writing
	// of the new image.
	ReportWriter io.Writer
	// Store is the local storage store which holds the source image.
	Store storage.Store
	// DockerRegistryOptions encapsulates settings that affect how we
	// connect or authenticate to a remote registry to which we want to
	// push the image.
	common.DockerRegistryOptions
	// SigningOptions encapsulates settings that control whether or not we
	// strip or add signatures to the image when pushing (uploading) the
	// image to a registry.
	common.SigningOptions
}

// PushImage pushes the src image to the destination
func PushImage(srcName, destName string, options CopyOptions) error {
	if srcName == "" || destName == "" {
		return errors.Wrapf(syscall.EINVAL, "source and destination image names must be specified")
	}

	// Get the destination Image Reference
	dest, err := alltransports.ParseImageName(destName)
	if err != nil {
		return errors.Wrapf(err, "error getting destination imageReference for %q", destName)
	}

	policyContext, err := common.GetPolicyContext(options.SignaturePolicyPath)
	if err != nil {
		return errors.Wrapf(err, "Could not get default policy context for signature policy path %q", options.SignaturePolicyPath)
	}
	defer policyContext.Destroy()
	// Look up the image name and its layer, then build the imagePushData from
	// the image
	img, err := FindImage(options.Store, srcName)
	if err != nil {
		return errors.Wrapf(err, "error locating image %q for importing settings", srcName)
	}
	systemContext := common.GetSystemContext(options.SignaturePolicyPath)
	cd, err := ImportCopyDataFromImage(options.Store, systemContext, img.ID, "", "")
	if err != nil {
		return err
	}
	// Give the image we're producing the same ancestors as its source image
	cd.FromImage = cd.Docker.ContainerConfig.Image
	cd.FromImageID = string(cd.Docker.Parent)

	// Prep the layers and manifest for export
	src, err := cd.MakeImageRef(manifest.GuessMIMEType(cd.Manifest), options.Compression, img.Names, img.TopLayer, nil)
	if err != nil {
		return errors.Wrapf(err, "error copying layers and metadata")
	}

	copyOptions := common.GetCopyOptions(options.ReportWriter, options.SignaturePolicyPath, nil, &options.DockerRegistryOptions, options.SigningOptions)

	// Copy the image to the remote destination
	err = cp.Image(policyContext, dest, src, copyOptions)
	if err != nil {
		return errors.Wrapf(err, "Error copying image to the remote destination")
	}
	return nil
}

// PullImage copies the image from the source to the destination
func PullImage(store storage.Store, imgName string, allTags, quiet bool, sc *types.SystemContext) error {
	var (
		images []string
		output io.Writer
	)

	if quiet {
		output = nil
	} else {
		output = os.Stdout
	}

	srcRef, err := alltransports.ParseImageName(imgName)
	if err != nil {
		defaultName := DefaultRegistry + imgName
		srcRef2, err2 := alltransports.ParseImageName(defaultName)
		if err2 != nil {
			return errors.Errorf("error parsing image name %q: %v", defaultName, err2)
		}
		srcRef = srcRef2
	}

	splitArr := strings.Split(imgName, ":")

	// supports pulling from docker-archive, oci, and registries
	if splitArr[0] == "docker-archive" {
		tarSource := tarfile.NewSource(splitArr[len(splitArr)-1])
		manifest, err := tarSource.LoadTarManifest()
		if err != nil {
			return errors.Errorf("error retrieving manifest.json: %v", err)
		}
		// to pull all the images stored in one tar file
		for i := range manifest {
			images = append(images, manifest[i].RepoTags[0])
		}
	} else if splitArr[0] == "oci" {
		// needs to be implemented in future
		return errors.Errorf("oci not supported")
	} else {
		images = append(images, imgName)
	}

	policy, err := signature.DefaultPolicy(sc)
	if err != nil {
		return err
	}

	policyContext, err := signature.NewPolicyContext(policy)
	if err != nil {
		return err
	}
	defer policyContext.Destroy()

	copyOptions := common.GetCopyOptions(output, "", nil, nil, common.SigningOptions{})

	for _, image := range images {
		destRef, err := is.Transport.ParseStoreReference(store, image)
		if err != nil {
			return errors.Errorf("error parsing dest reference name: %v", err)
		}
		if err = cp.Image(policyContext, destRef, srcRef, copyOptions); err != nil {
			return errors.Errorf("error loading image %q: %v", image, err)
		}
	}
	return nil
}
