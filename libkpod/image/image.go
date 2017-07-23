package image

import (
	is "github.com/containers/image/storage"
	"github.com/containers/storage"
	"github.com/pkg/errors"
)

// FindImage searches for an image with a matching the given name or ID in the given store
func FindImage(store storage.Store, image string) (*storage.Image, error) {
	var img *storage.Image
	ref, err := is.Transport.ParseStoreReference(store, image)
	if err == nil {
		img, err = is.Transport.GetStoreImage(store, ref)
	}
	if err != nil {
		img2, err2 := store.Image(image)
		if err2 != nil {
			if ref == nil {
				return nil, errors.Wrapf(err, "error parsing reference to image %q", image)
			}
			return nil, errors.Wrapf(err, "unable to locate image %q", image)
		}
		img = img2
	}
	return img, nil
}

// Size returns the size of the image in the given store
func Size(store storage.Store, img storage.Image) (int64, error) {
	is.Transport.SetStore(store)
	storeRef, err := is.Transport.ParseStoreReference(store, "@"+img.ID)
	if err != nil {
		return -1, err
	}
	imgRef, err := storeRef.NewImage(nil)
	if err != nil {
		return -1, err
	}
	imgSize, err := imgRef.Size()
	if err != nil {
		return -1, err
	}
	return imgSize, nil
}

// GetTopLayerID returns the ID of the top layer of the image
func GetTopLayerID(img storage.Image) (string, error) {
	metadata, err := ParseMetadata(img)
	if err != nil {
		return "", err
	}
	// Get the digest of the first blob
	digest := string(metadata.Blobs[0].Digest)
	// Return the first layer associated with the given digest
	return metadata.Layers[digest][0], nil
}
