package main

import (
	"fmt"
	"os"

	"github.com/Sirupsen/logrus"
	cp "github.com/containers/image/copy"
	"github.com/containers/image/docker/reference"
	"github.com/containers/image/signature"
	is "github.com/containers/image/storage"
	"github.com/containers/image/transports/alltransports"
	"github.com/containers/image/types"
	"github.com/containers/storage"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	// DefaultRegistry is a prefix that we apply to an image name
	// to check docker hub first for the image
	DefaultRegistry = "docker://"
)

var (
	pullFlags = []cli.Flag{
		cli.BoolFlag{
			// all-tags is hidden since it has not been implemented yet
			Name:   "all-tags, a",
			Hidden: true,
			Usage:  "Download all tagged images in the repository",
		},
	}

	pullDescription = "Pulls an image from a registry and stores it locally.\n" +
		"An image can be pulled using its tag or digest. If a tag is not\n" +
		"specified, the image with the 'latest' tag (if it exists) is pulled."
	pullCommand = cli.Command{
		Name:        "pull",
		Usage:       "pull an image from a registry",
		Description: pullDescription,
		Flags:       pullFlags,
		Action:      pullCmd,
		ArgsUsage:   "",
	}
)

// pullCmd gets the data from the command line and calls pullImage
// to copy an image from a registry to a local machine
func pullCmd(c *cli.Context) error {
	args := c.Args()
	if len(args) == 0 {
		logrus.Errorf("an image name must be specified")
		return nil
	}
	if len(args) > 1 {
		logrus.Errorf("too many arguments. Requires exactly 1")
		return nil
	}
	image := args[0]

	store, err := getStore(c)
	if err != nil {
		return err
	}

	allTags := false
	if c.IsSet("all-tags") {
		allTags = c.Bool("all-tags")
	}

	systemContext := getSystemContext("")

	err = pullImage(store, image, allTags, systemContext)
	if err != nil {
		return errors.Errorf("error pulling image from %q: %v", image, err)
	}
	return nil
}

// pullImage copies the image from the source to the destination
func pullImage(store storage.Store, imgName string, allTags bool, sc *types.SystemContext) error {
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

	copyOptions := getCopyOptions(os.Stdout, "", nil, nil, signingOptions{})

	fmt.Println(tag + ": pulling from " + fromName)
	return cp.Image(policyContext, destRef, srcRef, copyOptions)
}
