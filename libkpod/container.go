package libkpod

import (
	cstorage "github.com/containers/storage"
	"github.com/pkg/errors"
)

// FindContainer searches for a container with the given name or ID in the given store
func FindContainer(store cstorage.Store, container string) (*cstorage.Container, error) {
	ctrStore, err := store.ContainerStore()
	if err != nil {
		return nil, err
	}
	return ctrStore.Get(container)
}

// GetContainerTopLayerID gets the ID of the top layer of the given container
func GetContainerTopLayerID(store cstorage.Store, containerID string) (string, error) {
	ctr, err := FindContainer(store, containerID)
	if err != nil {
		return "", err
	}
	return ctr.LayerID, nil
}

// GetContainerRwSize Gets the size of the mutable top layer of the container
func GetContainerRwSize(store cstorage.Store, containerID string) (int64, error) {
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

// GetContainerRootFsSize gets the size of the container's root filesystem
// A container FS is split into two parts.  The first is the top layer, a
// mutable layer, and the rest is the RootFS: the set of immutable layers
// that make up the image on which the container is based
func GetContainerRootFsSize(store cstorage.Store, containerID string) (int64, error) {
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
