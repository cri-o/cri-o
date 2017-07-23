package image

import (
	"fmt"
	"io"
	"os"
	"syscall"

	cp "github.com/containers/image/copy"
	"github.com/containers/image/docker/reference"
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
func PullImage(store storage.Store, imgName string, allTags bool, sc *types.SystemContext) error {
	defaultName := DefaultRegistry + imgName
	var fromName string
	var tag string

	srcRef, err := alltransports.ParseImageName(defaultName)
	if err != nil {
		srcRef2, err2 := alltransports.ParseImageName(imgName)
		if err2 != nil {
			return errors.Wrapf(err2, "error parsing image name %q", imgName)
		}
		srcRef = srcRef2
	}

	ref := srcRef.DockerReference()
	if ref != nil {
		imgName = srcRef.DockerReference().Name()
		fromName = imgName
		tagged, ok := srcRef.DockerReference().(reference.NamedTagged)
		if ok {
			imgName = imgName + ":" + tagged.Tag()
			tag = tagged.Tag()
		}
	}

	destRef, err := is.Transport.ParseStoreReference(store, imgName)
	if err != nil {
		return errors.Wrapf(err, "error parsing full image name %q", imgName)
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

	copyOptions := common.GetCopyOptions(os.Stdout, "", nil, nil, common.SigningOptions{})

	fmt.Println(tag + ": pulling from " + fromName)
	return cp.Image(policyContext, destRef, srcRef, copyOptions)
}
