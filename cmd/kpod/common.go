package main

import (
	"io"
	"strings"

	cp "github.com/containers/image/copy"
	"github.com/containers/image/signature"
	is "github.com/containers/image/storage"
	"github.com/containers/image/types"
	"github.com/containers/storage"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

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

func findContainer(store storage.Store, container string) (*storage.Container, error) {
	ctrStore, err := store.ContainerStore()
	if err != nil {
		return nil, err
	}
	return ctrStore.Get(container)
}

func getContainerTopLayerID(store storage.Store, containerID string) (string, error) {
	ctr, err := findContainer(store, containerID)
	if err != nil {
		return "", err
	}
	return ctr.LayerID, nil
}

func getSystemContext(signaturePolicyPath string) *types.SystemContext {
	sc := &types.SystemContext{}
	if signaturePolicyPath != "" {
		sc.SignaturePolicyPath = signaturePolicyPath
	}
	return sc
}

func copyStringStringMap(m map[string]string) map[string]string {
	n := map[string]string{}
	for k, v := range m {
		n[k] = v
	}
	return n
}

// A container FS is split into two parts.  The first is the top layer, a
// mutable layer, and the rest is the RootFS: the set of immutable layers
// that make up the image on which the container is based
func getRootFsSize(store storage.Store, containerID string) (int64, error) {
	ctrStore, err := store.ContainerStore()
	if err != nil {
		return 0, err
	}
	container, err := ctrStore.Get(containerID)
	if err != nil {
		return 0, err
	}
	lstore, err := store.LayerStore()
	if err != nil {
		return 0, err
	}

	// Ignore the size of the top layer.   The top layer is a mutable RW layer
	// and is not considered a part of the rootfs
	rwLayer, err := lstore.Get(container.LayerID)
	if err != nil {
		return 0, err
	}
	layer, err := lstore.Get(rwLayer.Parent)
	if err != nil {
		return 0, err
	}

	size := int64(0)
	for layer.Parent != "" {
		layerSize, err := lstore.DiffSize(layer.Parent, layer.ID)
		if err != nil {
			return size, errors.Wrapf(err, "getting diffsize of layer %q and its parent %q", layer.ID, layer.Parent)
		}
		size += layerSize
		layer, err = lstore.Get(layer.Parent)
		if err != nil {
			return 0, err
		}
	}
	// Get the size of the last layer.  Has to be outside of the loop
	// because the parent of the last layer is "", andlstore.Get("")
	// will return an error
	layerSize, err := lstore.DiffSize(layer.Parent, layer.ID)
	return size + layerSize, err
}

func getContainerRwSize(store storage.Store, containerID string) (int64, error) {
	ctrStore, err := store.ContainerStore()
	if err != nil {
		return 0, err
	}
	container, err := ctrStore.Get(containerID)
	if err != nil {
		return 0, err
	}
	lstore, err := store.LayerStore()
	if err != nil {
		return 0, err
	}

	// Get the size of the top layer by calculating the size of the diff
	// between the layer and its parent.  The top layer of a container is
	// the only RW layer, all others are immutable
	layer, err := lstore.Get(container.LayerID)
	if err != nil {
		return 0, err
	}
	return lstore.DiffSize(layer.Parent, layer.ID)
}

func isTrue(str string) bool {
	return str == "true"
}

func isFalse(str string) bool {
	return str == "false"
}

func isValidBool(str string) bool {
	return isTrue(str) || isFalse(str)
}

func getPolicyContext(path string) (*signature.PolicyContext, error) {
	policy, err := signature.DefaultPolicy(&types.SystemContext{SignaturePolicyPath: path})
	if err != nil {
		return nil, err
	}
	return signature.NewPolicyContext(policy)
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

func (o dockerRegistryOptions) getSystemContext(signaturePolicyPath string) *types.SystemContext {
	sc := &types.SystemContext{
		SignaturePolicyPath:         signaturePolicyPath,
		DockerAuthConfig:            o.DockerRegistryCreds,
		DockerCertPath:              o.DockerCertPath,
		DockerInsecureSkipTLSVerify: o.DockerInsecureSkipTLSVerify,
	}
	return sc
}
