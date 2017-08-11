package image

import (
	"encoding/json"
	"time"

	"github.com/containers/image/docker/reference"
	"github.com/containers/image/transports"
	"github.com/containers/storage"
	"github.com/kubernetes-incubator/cri-o/libpod/driver"
	digest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

// ImageData handles the data used when inspecting a container
// nolint
type ImageData struct {
	ID           string
	Tags         []string
	Digests      []string
	Digest       digest.Digest
	Comment      string
	Created      *time.Time
	Container    string
	Author       string
	Config       ociv1.ImageConfig
	Architecture string
	OS           string
	Annotations  map[string]string
	CreatedBy    string
	Size         uint
	VirtualSize  uint
	GraphDriver  driver.Data
	RootFS       ociv1.RootFS
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

func annotations(manifest []byte, manifestType string) map[string]string {
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
	return annotations
}

// GetImageData gets the ImageData for a container with the given name in the given store.
func GetImageData(store storage.Store, name string) (*ImageData, error) {
	img, err := FindImage(store, name)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading image %q", name)
	}

	imgRef, err := FindImageRef(store, "@"+img.ID)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading image %q", img.ID)
	}
	defer imgRef.Close()

	tags, digests, err := ParseImageNames(img.Names)
	if err != nil {
		return nil, errors.Wrapf(err, "error parsing image names for %q", name)
	}

	driverName, err := driver.GetDriverName(store)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading name of storage driver")
	}

	topLayerID := img.TopLayer

	driverMetadata, err := driver.GetDriverMetadata(store, topLayerID)
	if err != nil {
		return nil, errors.Wrapf(err, "error asking storage driver %q for metadata", driverName)
	}

	layer, err := store.Layer(topLayerID)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading information about layer %q", topLayerID)
	}
	size, err := store.DiffSize(layer.Parent, layer.ID)
	if err != nil {
		return nil, errors.Wrapf(err, "error determining size of layer %q", layer.ID)
	}

	imgSize, err := imgRef.Size()
	if err != nil {
		return nil, errors.Wrapf(err, "error determining size of image %q", transports.ImageName(imgRef.Reference()))
	}

	manifest, manifestType, err := imgRef.Manifest()
	if err != nil {
		return nil, errors.Wrapf(err, "error reading manifest for image %q", img.ID)
	}
	manifestDigest := digest.Digest("")
	if len(manifest) > 0 {
		manifestDigest = digest.Canonical.FromBytes(manifest)
	}
	annotations := annotations(manifest, manifestType)

	config, err := imgRef.OCIConfig()
	if err != nil {
		return nil, errors.Wrapf(err, "error reading image configuration for %q", img.ID)
	}
	historyComment := ""
	historyCreatedBy := ""
	if len(config.History) > 0 {
		historyComment = config.History[len(config.History)-1].Comment
		historyCreatedBy = config.History[len(config.History)-1].CreatedBy
	}

	return &ImageData{
		ID:           img.ID,
		Tags:         tags,
		Digests:      digests,
		Digest:       manifestDigest,
		Comment:      historyComment,
		Created:      config.Created,
		Author:       config.Author,
		Config:       config.Config,
		Architecture: config.Architecture,
		OS:           config.OS,
		Annotations:  annotations,
		CreatedBy:    historyCreatedBy,
		Size:         uint(size),
		VirtualSize:  uint(size + imgSize),
		GraphDriver: driver.Data{
			Name: driverName,
			Data: driverMetadata,
		},
		RootFS: config.RootFS,
	}, nil
}
