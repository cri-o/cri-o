package main

import (
	"fmt"
	"io"
	"os"
	"syscall"

	cp "github.com/containers/image/copy"
	"github.com/containers/image/manifest"
	"github.com/containers/image/transports/alltransports"
	"github.com/containers/image/types"
	"github.com/containers/storage"
	"github.com/containers/storage/pkg/archive"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

var (
	pushFlags = []cli.Flag{
		cli.BoolFlag{
			Name:   "disable-compression, D",
			Usage:  "don't compress layers",
			Hidden: true,
		},
		cli.StringFlag{
			Name:   "signature-policy",
			Usage:  "`pathname` of signature policy file (not usually used)",
			Hidden: true,
		},
		cli.StringFlag{
			Name:  "creds",
			Usage: "`credentials` (USERNAME:PASSWORD) to use for authenticating to a registry",
		},
		cli.StringFlag{
			Name:  "cert-dir",
			Usage: "`pathname` of a directory containing TLS certificates and keys",
		},
		cli.BoolTFlag{
			Name:  "tls-verify",
			Usage: "require HTTPS and verify certificates when contacting registries (default: true)",
		},
		cli.BoolFlag{
			Name:  "remove-signatures",
			Usage: "discard any pre-existing signatures in the image",
		},
		cli.StringFlag{
			Name:  "sign-by",
			Usage: "add a signature at the destination using the specified key",
		},
		cli.BoolFlag{
			Name:  "quiet, q",
			Usage: "don't output progress information when pushing images",
		},
	}
	pushDescription = fmt.Sprintf(`
   Pushes an image to a specified location.
   The Image "DESTINATION" uses a "transport":"details" format.
   See kpod-push(1) section "DESTINATION" for the expected format`)

	pushCommand = cli.Command{
		Name:        "push",
		Usage:       "push an image to a specified destination",
		Description: pushDescription,
		Flags:       pushFlags,
		Action:      pushCmd,
		ArgsUsage:   "IMAGE DESTINATION",
	}
)

type pushOptions struct {
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
	dockerRegistryOptions
	// SigningOptions encapsulates settings that control whether or not we
	// strip or add signatures to the image when pushing (uploading) the
	// image to a registry.
	signingOptions
}

func pushCmd(c *cli.Context) error {
	var registryCreds *types.DockerAuthConfig

	args := c.Args()
	if len(args) < 2 {
		return errors.New("kpod push requires exactly 2 arguments")
	}
	srcName := c.Args().Get(0)
	destName := c.Args().Get(1)

	signaturePolicy := c.String("signature-policy")
	compress := archive.Uncompressed
	if !c.Bool("disable-compression") {
		compress = archive.Gzip
	}
	registryCredsString := c.String("creds")
	certPath := c.String("cert-dir")
	skipVerify := !c.BoolT("tls-verify")
	removeSignatures := c.Bool("remove-signatures")
	signBy := c.String("sign-by")

	if registryCredsString != "" {
		creds, err := parseRegistryCreds(registryCredsString)
		if err != nil {
			return err
		}
		registryCreds = creds
	}

	store, err := getStore(c)
	if err != nil {
		return err
	}

	options := pushOptions{
		Compression:         compress,
		SignaturePolicyPath: signaturePolicy,
		Store:               store,
		dockerRegistryOptions: dockerRegistryOptions{
			DockerRegistryCreds:         registryCreds,
			DockerCertPath:              certPath,
			DockerInsecureSkipTLSVerify: skipVerify,
		},
		signingOptions: signingOptions{
			RemoveSignatures: removeSignatures,
			SignBy:           signBy,
		},
	}
	if !c.Bool("quiet") {
		options.ReportWriter = os.Stderr
	}
	return pushImage(srcName, destName, options)
}

func pushImage(srcName, destName string, options pushOptions) error {
	if srcName == "" || destName == "" {
		return errors.Wrapf(syscall.EINVAL, "source and destination image names must be specified")
	}

	// Get the destination Image Reference
	dest, err := alltransports.ParseImageName(destName)
	if err != nil {
		return errors.Wrapf(err, "error getting destination imageReference for %q", destName)
	}

	policyContext, err := getPolicyContext(options.SignaturePolicyPath)
	if err != nil {
		return errors.Wrapf(err, "Could not get default policy context for signature policy path %q", options.SignaturePolicyPath)
	}
	defer policyContext.Destroy()
	// Look up the image name and its layer, then build the imagePushData from
	// the image
	img, err := findImage(options.Store, srcName)
	if err != nil {
		return errors.Wrapf(err, "error locating image %q for importing settings", srcName)
	}
	systemContext := getSystemContext(options.SignaturePolicyPath)
	cid, err := importContainerImageDataFromImage(options.Store, systemContext, img.ID, "", "")
	if err != nil {
		return err
	}
	// Give the image we're producing the same ancestors as its source image
	cid.FromImage = cid.Docker.ContainerConfig.Image
	cid.FromImageID = string(cid.Docker.Parent)

	// Prep the layers and manifest for export
	src, err := cid.makeImageRef(manifest.GuessMIMEType(cid.Manifest), options.Compression, img.Names, img.TopLayer, nil)
	if err != nil {
		return errors.Wrapf(err, "error copying layers and metadata")
	}

	copyOptions := getCopyOptions(options.ReportWriter, options.SignaturePolicyPath, nil, &options.dockerRegistryOptions, options.signingOptions)

	// Copy the image to the remote destination
	err = cp.Image(policyContext, dest, src, copyOptions)
	if err != nil {
		return errors.Wrapf(err, "Error copying image to the remote destination")
	}
	return nil
}
