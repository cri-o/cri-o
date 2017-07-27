package image

import (
	"time"

	"github.com/containers/image/docker/reference"
	"github.com/containers/storage"
	"github.com/kubernetes-incubator/cri-o/cmd/kpod/docker"
	"github.com/kubernetes-incubator/cri-o/libkpod/driver"
	digest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

// ImageData handles the data used when inspecting a container
// nolint
type ImageData struct {
	ID              string
	Tags            []string
	Digests         []string
	Digest          digest.Digest
	Parent          string
	Comment         string
	Created         *time.Time
	Container       string
	ContainerConfig docker.Config
	Author          string
	Config          ociv1.ImageConfig
	Architecture    string
	OS              string
	Size            uint
	VirtualSize     uint
	GraphDriver     driver.Data
	RootFS          ociv1.RootFS
}

// ParseImageNames parses the names we've stored with an image into a list of
// tagged references and a list of references which contain digests.
func ParseImageNames(names []string) (tags, digests []string, err error) {
	for _, name := range names {
		if named, err := reference.ParseNamed(name); err == nil {
			if digested, ok := named.(reference.Digested); ok {
				canonical, err := reference.WithDigest(named, digested.Digest())
				if err == nil {
					digests = append(digests, canonical.String())
				}
			} else {
				if reference.IsNameOnly(named) {
					named = reference.TagNameOnly(named)
				}
				if tagged, ok := named.(reference.Tagged); ok {
					namedTagged, err := reference.WithTag(named, tagged.Tag())
					if err == nil {
						tags = append(tags, namedTagged.String())
					}
				}
			}
		}
	}
	return tags, digests, nil
}

// GetImageData gets the ImageData for a container with the given name in the given store.
func GetImageData(store storage.Store, name string) (*ImageData, error) {
	img, err := FindImage(store, name)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading image %q", name)
	}

	cid, err := GetImageCopyData(store, name)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading image %q", name)
	}

	tags, digests, err := ParseImageNames(img.Names)
	if err != nil {
		return nil, errors.Wrapf(err, "error parsing image names for %q", name)
	}

	driverName, err := driver.GetDriverName(store)
	if err != nil {
		return nil, err
	}

	topLayerID := img.TopLayer

	driverMetadata, err := driver.GetDriverMetadata(store, topLayerID)
	if err != nil {
		return nil, err
	}

	layer, err := store.Layer(topLayerID)
	if err != nil {
		return nil, err
	}
	size, err := store.DiffSize(layer.Parent, layer.ID)
	if err != nil {
		return nil, err
	}

	_, digest, virtualSize, err := InfoAndDigestAndSize(store, *img)
	if err != nil {
		return nil, err
	}

	return &ImageData{
		ID:              img.ID,
		Tags:            tags,
		Digests:         digests,
		Digest:          digest,
		Parent:          string(cid.Docker.Parent),
		Comment:         cid.OCIv1.History[0].Comment,
		Created:         cid.OCIv1.Created,
		Container:       cid.Docker.Container,
		ContainerConfig: cid.Docker.ContainerConfig,
		Author:          cid.OCIv1.Author,
		Config:          cid.OCIv1.Config,
		Architecture:    cid.OCIv1.Architecture,
		OS:              cid.OCIv1.OS,
		Size:            uint(size),
		VirtualSize:     uint(virtualSize),
		GraphDriver: driver.Data{
			Name: driverName,
			Data: driverMetadata,
		},
		RootFS: cid.OCIv1.RootFS,
	}, nil
}
