package libkpod

import (
	cstorage "github.com/containers/storage"
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
