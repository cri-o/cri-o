package image

import (
	"github.com/containers/storage"
	"github.com/pkg/errors"
)

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
