package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	cp "github.com/containers/image/copy"
	"github.com/containers/image/signature"
	is "github.com/containers/image/storage"
	"github.com/containers/image/types"
	"github.com/containers/storage"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

type imageMetadata struct {
	Tag            string              `json:"tag"`
	CreatedTime    time.Time           `json:"created-time"`
	ID             string              `json:"id"`
	Blobs          []types.BlobInfo    `json:"blob-list"`
	Layers         map[string][]string `json:"layers"`
	SignatureSizes []string            `json:"signature-sizes"`
}

// DockerRegistryOptions encapsulates settings that affect how we connect or
// authenticate to a remote registry.
type dockerRegistryOptions struct {
	// DockerRegistryCreds is the user name and password to supply in case
	// we need to pull an image from a registry, and it requires us to
	// authenticate.
	DockerRegistryCreds *types.DockerAuthConfig
	// DockerCertPath is the location of a directory containing CA
	// certificates which will be used to verify the registry's certificate
	// (all files with names ending in ".crt"), and possibly client
	// certificates and private keys (pairs of files with the same name,
	// except for ".cert" and ".key" suffixes).
	DockerCertPath string
	// DockerInsecureSkipTLSVerify turns off verification of TLS
	// certificates and allows connecting to registries without encryption.
	DockerInsecureSkipTLSVerify bool
}

// SigningOptions encapsulates settings that control whether or not we strip or
// add signatures to images when writing them.
type signingOptions struct {
	// RemoveSignatures directs us to remove any signatures which are already present.
	RemoveSignatures bool
	// SignBy is a key identifier of some kind, indicating that a signature should be generated using the specified private key and stored with the image.
	SignBy string
}

func getStore(c *cli.Context) (storage.Store, error) {
	options := storage.DefaultStoreOptions
	if c.GlobalIsSet("root") {
		options.GraphRoot = c.GlobalString("root")
	}
	if c.GlobalIsSet("runroot") {
		options.RunRoot = c.GlobalString("runroot")
	}

	if c.GlobalIsSet("storage-driver") {
		options.GraphDriverName = c.GlobalString("storage-driver")
	}
	if c.GlobalIsSet("storage-opt") {
		opts := c.GlobalStringSlice("storage-opt")
		if len(opts) > 0 {
			options.GraphDriverOptions = opts
		}
	}
	store, err := storage.GetStore(options)
	if err != nil {
		return nil, err
	}
	is.Transport.SetStore(store)
	return store, nil
}

func getCopyOptions(reportWriter io.Writer, signaturePolicyPath string, srcDockerRegistry, destDockerRegistry *dockerRegistryOptions, signing signingOptions) *cp.Options {
	if srcDockerRegistry == nil {
		srcDockerRegistry = &dockerRegistryOptions{}
	}
	if destDockerRegistry == nil {
		destDockerRegistry = &dockerRegistryOptions{}
	}
	srcContext := srcDockerRegistry.getSystemContext(signaturePolicyPath)
	destContext := destDockerRegistry.getSystemContext(signaturePolicyPath)
	return &cp.Options{
		RemoveSignatures: signing.RemoveSignatures,
		SignBy:           signing.SignBy,
		ReportWriter:     reportWriter,
		SourceCtx:        srcContext,
		DestinationCtx:   destContext,
	}
}

func getPolicyContext(path string) (*signature.PolicyContext, error) {
	policy, err := signature.DefaultPolicy(&types.SystemContext{SignaturePolicyPath: path})
	if err != nil {
		return nil, err
	}
	return signature.NewPolicyContext(policy)
}

func findImage(store storage.Store, image string) (*storage.Image, error) {
	var img *storage.Image
	ref, err := is.Transport.ParseStoreReference(store, image)
	if err == nil {
		img, err := is.Transport.GetStoreImage(store, ref)
		if err != nil {
			return nil, err
		}
		return img, nil
	}
	img2, err2 := store.Image(image)
	if err2 != nil {
		if ref == nil {
			return nil, errors.Wrapf(err, "error parsing reference to image %q", image)
		}
		return nil, errors.Wrapf(err, "unable to locate image %q", image)
	}
	img = img2
	return img, nil
}

func getSystemContext(signaturePolicyPath string) *types.SystemContext {
	sc := &types.SystemContext{}
	if signaturePolicyPath != "" {
		sc.SignaturePolicyPath = signaturePolicyPath
	}
	return sc
}

func parseMetadata(image storage.Image) (imageMetadata, error) {
	var im imageMetadata

	dec := json.NewDecoder(strings.NewReader(image.Metadata))
	if err := dec.Decode(&im); err != nil {
		return imageMetadata{}, err
	}
	return im, nil
}

func getSize(image storage.Image, store storage.Store) (int64, error) {

	is.Transport.SetStore(store)
	storeRef, err := is.Transport.ParseStoreReference(store, "@"+image.ID)
	if err != nil {
		fmt.Println(err)
		return -1, err
	}
	img, err := storeRef.NewImage(nil)
	if err != nil {
		fmt.Println("Error with NewImage")
		return -1, err
	}
	imgSize, err := img.Size()
	if err != nil {
		fmt.Println("Error getting size")
		return -1, err
	}
	return imgSize, nil
}

func copyStringStringMap(m map[string]string) map[string]string {
	n := map[string]string{}
	for k, v := range m {
		n[k] = v
	}
	return n
}

func (o dockerRegistryOptions) getSystemContext(signaturePolicyPath string) *types.SystemContext {
	sc := &types.SystemContext{
		SignaturePolicyPath:         signaturePolicyPath,
		DockerAuthConfig:            o.DockerRegistryCreds,
		DockerCertPath:              o.DockerCertPath,
		DockerInsecureSkipTLSVerify: o.DockerInsecureSkipTLSVerify,
	}
	return sc
}

func parseRegistryCreds(creds string) (*types.DockerAuthConfig, error) {
	if creds == "" {
		return nil, errors.New("no credentials supplied")
	}
	if strings.Index(creds, ":") < 0 {
		return nil, errors.New("user name supplied, but no password supplied")
	}
	v := strings.SplitN(creds, ":", 2)
	cfg := &types.DockerAuthConfig{
		Username: v[0],
		Password: v[1],
	}
	return cfg, nil
}
