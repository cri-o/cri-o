package image

import (
	"fmt"

	is "github.com/containers/image/storage"
	"github.com/containers/image/transports"
	"github.com/containers/image/transports/alltransports"
	"github.com/containers/image/types"
	"github.com/containers/storage"
	"github.com/pkg/errors"
)

// If it's looks like a proper image reference, parse it and check if it
// corresponds to an image that actually exists.
func properImageRef(id string) (types.ImageReference, error) {
	var ref types.ImageReference
	var err error
	if ref, err = alltransports.ParseImageName(id); err == nil {
		if img, err2 := ref.NewImage(nil); err2 == nil {
			img.Close()
			return ref, nil
		}
		return nil, fmt.Errorf("error confirming presence of image reference %q: %v", transports.ImageName(ref), err)
	}
	return nil, fmt.Errorf("error parsing %q as an image reference: %v", id, err)
}

// If it's looks like an image reference that's relative to our storage, parse
// it and check if it corresponds to an image that actually exists.
func storageImageRef(store storage.Store, id string) (types.ImageReference, error) {
	var ref types.ImageReference
	var err error
	if ref, err = is.Transport.ParseStoreReference(store, id); err == nil {
		if img, err2 := ref.NewImage(nil); err2 == nil {
			img.Close()
			return ref, nil
		}
		return nil, fmt.Errorf("error confirming presence of storage image reference %q: %v", transports.ImageName(ref), err)
	}
	return nil, fmt.Errorf("error parsing %q as a storage image reference: %v", id, err)
}

// If it might be an ID that's relative to our storage, parse it and check if it
// corresponds to an image that actually exists.  This _should_ be redundant,
// since we already tried deleting the image using the ID directly above, but it
// can't hurt either.
func storageImageID(store storage.Store, id string) (types.ImageReference, error) {
	var ref types.ImageReference
	var err error
	if ref, err = is.Transport.ParseStoreReference(store, "@"+id); err == nil {
		if img, err2 := ref.NewImage(nil); err2 == nil {
			img.Close()
			return ref, nil
		}
		return nil, fmt.Errorf("error confirming presence of storage image reference %q: %v", transports.ImageName(ref), err)
	}
	return nil, fmt.Errorf("error parsing %q as a storage image reference: %v", "@"+id, err)
}

// GetImage TODO: Ask Nalin if this or FindImage() is better
func GetImage(store storage.Store, id string) (*storage.Image, error) {
	var ref types.ImageReference
	ref, err := properImageRef(id)
	if err != nil {
		//logrus.Debug(err)
	}
	if ref == nil {
		if ref, err = storageImageRef(store, id); err != nil {
			//logrus.Debug(err)
		}
	}
	if ref == nil {
		if ref, err = storageImageID(store, id); err != nil {
			//logrus.Debug(err)
		}
	}
	if ref != nil {
		image, err2 := is.Transport.GetStoreImage(store, ref)
		if err2 != nil {
			return nil, err2
		}
		return image, nil
	}
	return nil, err
}

// UntagImage removes the tag from the given image
func UntagImage(store storage.Store, image *storage.Image, imgArg string) (string, error) {
	// Remove name from image.Names and set the new name in the ImageStore
	imgStore, err := store.ImageStore()
	if err != nil {
		return "", errors.Wrap(err, "could not untag image")
	}
	newNames := []string{}
	removedName := ""
	for _, name := range image.Names {
		if MatchesReference(name, imgArg) {
			removedName = name
			continue
		}
		newNames = append(newNames, name)
	}
	imgStore.SetNames(image.ID, newNames)
	err = imgStore.Save()
	return removedName, err
}

// RemoveImage removes the given image from storage
func RemoveImage(image *storage.Image, store storage.Store) (string, error) {
	imgStore, err := store.ImageStore()
	if err != nil {
		return "", errors.Wrapf(err, "could not open image store")
	}
	err = imgStore.Delete(image.ID)
	if err != nil {
		return "", errors.Wrapf(err, "could not remove image")
	}
	err = imgStore.Save()
	if err != nil {
		return "", errors.Wrapf(err, "could not save image store")
	}
	return image.ID, nil
}
