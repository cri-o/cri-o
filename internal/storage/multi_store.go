package storage

import (
	"fmt"

	ctypes "github.com/containers/image/v5/types"
	cstorage "github.com/containers/storage"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// multiStore contains the container store based on the configured storage drivers
type multiStore struct {
	defaultStorage string
	store          map[string]cstorage.Store
	runRoot        string
	graphRoot      string
	iterator       iteratorMultiStore
}

// iteratorMultiStoreServer iterates over the multiStore starting by the default storage driver
type iteratorMultiStore struct {
	keys       []string
	multiStore *multiStore
	counter    int
}

func createMultiStoreIterator(s *multiStore) iteratorMultiStore {
	keys := []string{s.GetDefaultStorageDriver()}
	for k := range s.store {
		if k == s.GetDefaultStorageDriver() {
			continue
		}
		keys = append(keys, k)
	}
	return iteratorMultiStore{
		multiStore: s,
		keys:       keys,
	}
}

func (i *iteratorMultiStore) initialize() {
	i.counter = 0
}

func (i *iteratorMultiStore) next() cstorage.Store {
	if i.counter > len(i.multiStore.store)-1 {
		return nil
	}
	if e, ok := i.multiStore.store[i.keys[i.counter]]; ok {
		i.counter++
		return e
	}
	return nil
}

// NewMultiStore creates a new MultiStore instance.
func NewMultiStore(store map[string]cstorage.Store, defaultStorage, runRoot, graphRoot string) MultiStore {
	s := &multiStore{
		defaultStorage: defaultStorage,
		store:          store,
		runRoot:        runRoot,
		graphRoot:      graphRoot,
	}
	s.iterator = createMultiStoreIterator(s)
	return s
}

// MultiStore wraps up all the methods for accessing and managing containers, layers and images in multiple storage drivers.
type MultiStore interface {
	GetDefaultStorageDriver() string
	GraphDriverName() []string
	GraphOptions() map[string][]string
	Containers() ([]cstorage.Container, error)
	Unmount(id string, force bool) (bool, error)
	Metadata(id string) (string, error)
	DeleteContainer(id string) error
	DeleteImage(id string, commit bool) (layers []string, err error)
	GetStore() map[string]cstorage.Store
	RunRoot() string
	GraphRoot() string
	GetStoreForContainer(idOrName string) (cstorage.Store, error)
	GetDefaultStorage() cstorage.Store
}

// GetDefaultStorageDriver returns the default storage driver.
func (s *multiStore) GetDefaultStorageDriver() string {
	return s.defaultStorage
}

// GetDefaultStorage returns the store for the default storage driver.
func (s *multiStore) GetDefaultStorage() cstorage.Store {
	return s.store[s.defaultStorage]
}

// GraphDriverName return the list of all the storage drivers.
func (s *multiStore) GraphDriverName() (drivers []string) {
	for k := range s.store {
		drivers = append(drivers, k)
	}
	return
}

// GraphOptions returns the graph options per storage driver.
func (s *multiStore) GraphOptions() map[string][]string {
	graphOptions := make(map[string][]string)
	for k, v := range s.store {
		graphOptions[k] = v.GraphOptions()
	}
	return graphOptions
}

// Containers returns a list of the currently known containers.
func (s *multiStore) Containers() (containers []cstorage.Container, lastError error) {
	s.iterator.initialize()
	for store := s.iterator.next(); store != nil; store = s.iterator.next() {
		c, err := store.Containers()
		if err != nil {
			lastError = wrapMultipleErrors(lastError, err)
		}
		containers = append(containers, c...)
	}

	return
}

// Unmount attempts to unmount a container, given an ID.
// Returns whether or not the layer is still mounted.
func (s *multiStore) Unmount(id string, force bool) (bool, error) {
	s.iterator.initialize()
	for store := s.iterator.next(); store != nil; store = s.iterator.next() {
		id, err := store.Lookup(id)
		if err != nil || id == "" {
			continue
		}
		return store.Unmount(id, force)
	}
	return false, fmt.Errorf("ID %s not found", id)
}

// Metadata retrieves the metadata which is associated with a layer,
// image, or container (whichever the passed-in ID refers to).
func (s *multiStore) Metadata(id string) (string, error) {
	s.iterator.initialize()
	for store := s.iterator.next(); store != nil; store = s.iterator.next() {
		id, err := store.Lookup(id)
		if err != nil || id == "" {
			continue
		}
		return store.Metadata(id)
	}
	return "", fmt.Errorf("Metadata for %s not found", id)
}

// DeleteContainer removes the specified container and its layer.
func (s *multiStore) DeleteContainer(id string) error {
	s.iterator.initialize()
	for store := s.iterator.next(); store != nil; store = s.iterator.next() {
		id, err := store.Lookup(id)
		if err != nil || id == "" {
			continue
		}
		return store.DeleteContainer(id)
	}
	return fmt.Errorf("container %s not found", id)
}

// DeleteImage removes the specified image if it is not referred to by any containers.
func (s *multiStore) DeleteImage(id string, commit bool) (layers []string, err error) {
	s.iterator.initialize()
	for store := s.iterator.next(); store != nil; store = s.iterator.next() {
		id, err := store.Lookup(id)
		if err != nil || id == "" {
			continue
		}
		return store.DeleteImage(id, commit)
	}
	return []string{}, fmt.Errorf("image %s not found", id)
}

// GetStore returns all the stores.
func (s *multiStore) GetStore() map[string]cstorage.Store {
	return s.store
}

func (s *multiStore) RunRoot() string {
	return s.runRoot
}

func (s *multiStore) GraphRoot() string {
	return s.graphRoot
}

// GetStoreForContainer returns the store for the given id or name.
func (s *multiStore) GetStoreForContainer(idOrName string) (cstorage.Store, error) {
	logrus.Debugf("GetStore for container %s", idOrName)
	s.iterator.initialize()
	for store := s.iterator.next(); store != nil; store = s.iterator.next() {
		if _, err := store.Container(idOrName); err != nil {
			continue
		}
		return store, nil
	}
	return nil, fmt.Errorf("error locating container %s", idOrName)
}

// multiStoreServer contains the store image servers based on the configured storage drivers.
type multiStoreServer struct {
	store      map[string]ImageServer
	multiStore MultiStore
	iterator   iteratorMultiStoreServer
}

// iteratorMultiStoreServer iterates over the multiStoreServer starting by the default storage driver.
type iteratorMultiStoreServer struct {
	keys             []string
	multiStoreServer *multiStoreServer
	counter          int
}

func createMultiStoreServerIterator(s *multiStoreServer) iteratorMultiStoreServer {
	defaultStorage := s.multiStore.GetDefaultStorageDriver()
	keys := []string{defaultStorage}
	for k := range s.store {
		if k == defaultStorage {
			continue
		}
		keys = append(keys, k)
	}
	return iteratorMultiStoreServer{
		multiStoreServer: s,
		keys:             keys,
	}
}

func (i *iteratorMultiStoreServer) initialize() {
	i.counter = 0
}

func (i *iteratorMultiStoreServer) next() ImageServer {
	if i.counter > len(i.multiStoreServer.store)-1 {
		return nil
	}
	if e, ok := i.multiStoreServer.store[i.keys[i.counter]]; ok {
		i.counter++
		return e
	}
	return nil
}

// MultiStoreServer wraps up all the methods for handling multiple stores and image servers
type MultiStoreServer interface {
	GetDefaultStorage() cstorage.Store
	GetAllStores() []cstorage.Store
	GetImageServer(driver string) (ImageServer, error)
	GetStoreForContainer(idOrName string) (cstorage.Store, error)
	GetStoreForImage(imageID string) (cstorage.Store, error)
	FromContainerDirectory(id, file string) ([]byte, error)
	ContainerRunDirectory(id string) (string, error)
	ContainerDirectory(id string) (string, error)
	Shutdown(force bool) (layers []string, err error)
	GraphRoot() string
	GetImageServerForImage(image string) ([]ImageServer, error)
	GetStore() MultiStore
	ListAllImages(ctx *ctypes.SystemContext, filter string) ([]ImageResult, error)
	ResolveNames(systemContext *ctypes.SystemContext, imageName string) ([]string, error)
	ImageStatus(systemContext *ctypes.SystemContext, filter string) (*ImageResult, error)
}

// NewMultiStoreServer creates a new instance of the MultiStoreServer.
func NewMultiStoreServer(store map[string]ImageServer, istore MultiStore) MultiStoreServer {
	s := &multiStoreServer{
		store:      store,
		multiStore: istore,
	}
	s.iterator = createMultiStoreServerIterator(s)
	return s
}

func (s *multiStoreServer) GetDefaultStorage() cstorage.Store {
	return s.multiStore.GetDefaultStorage()
}

// GetImageServer returns the ImageServer for the selected storage driver. If the driver is empty or not found, the default storage driver is returned
func (s *multiStoreServer) GetImageServer(driver string) (ImageServer, error) {
	iserver, ok := s.store[driver]
	if !ok {
		v, ok := s.store[s.multiStore.GetDefaultStorageDriver()]
		if !ok || v == nil {
			return nil, fmt.Errorf("no image server for default storage driver found")
		}
		return v, nil
	}
	return iserver, nil
}

// GetStore returns the MultiStore used for configuring the MultiStoreServer.
func (s *multiStoreServer) GetStore() MultiStore {
	return s.multiStore
}

// ListAllImages lists all the images known by the MultiStoreServer.
func (s *multiStoreServer) ListAllImages(ctx *ctypes.SystemContext, filter string) (imageResults []ImageResult, lastError error) {
	s.iterator.initialize()
	for is := s.iterator.next(); is != nil; is = s.iterator.next() {
		images, err := is.ListImages(ctx, filter)
		if err != nil {
			lastError = wrapMultipleErrors(lastError, err)
			continue
		}
		imageResults = append(imageResults, images...)
	}
	return imageResults, lastError
}

func (s *multiStoreServer) GetAllStores() (store []cstorage.Store) {
	for _, s := range s.multiStore.GetStore() {
		store = append(store, s)
	}
	return
}

// GetStoreForImage retrives the store by image ID.
func (s *multiStoreServer) GetStoreForImage(imageID string) (cstorage.Store, error) {
	logrus.Debugf("GetStore for image %s", imageID)
	s.iterator.initialize()
	for store := s.iterator.next(); store != nil; store = s.iterator.next() {
		if _, err := store.GetStore().Image(imageID); err != nil {
			continue
		}
		return store.GetStore(), nil
	}
	return nil, fmt.Errorf("error locating image with ID %s", imageID)
}

// GetStoreForContainer retrives the store by container id or name.
func (s *multiStoreServer) GetStoreForContainer(idOrName string) (cstorage.Store, error) {
	logrus.Debugf("GetStore for container %s", idOrName)
	s.iterator.initialize()
	for store := s.iterator.next(); store != nil; store = s.iterator.next() {
		if _, err := store.GetStore().Container(idOrName); err != nil {
			continue
		}
		return store.GetStore(), nil
	}
	return nil, fmt.Errorf("error locating container %s", idOrName)
}

// GetImageServerForImage retrives the ImageServer by image name. The same image could be present in multiple image server at the same time, therefore we return a list of all the image servers that contain the image.
func (s *multiStoreServer) GetImageServerForImage(image string) (iservers []ImageServer, err error) {
	logrus.Debugf("GetImageServerForImage for image %s", image)
	s.iterator.initialize()
	for is := s.iterator.next(); is != nil; is = s.iterator.next() {
		_, e := is.GetStore().Image(image)
		if e != nil {
			continue
		}
		iservers = append(iservers, is)
	}
	if len(iservers) < 1 {
		err = cstorage.ErrImageUnknown
	}
	return
}

// FromContainerDirectory calls the FromContainerDirectory function for the store of the give container id.
func (s *multiStoreServer) FromContainerDirectory(id, file string) ([]byte, error) {
	store, err := s.GetStoreForContainer(id)
	if err != nil {
		return nil, err
	}
	return store.FromContainerDirectory(id, file)
}

// ContainerRunDirectory calls the ContainerRunDirectory function for the store of the give container id.
func (s *multiStoreServer) ContainerRunDirectory(id string) (string, error) {
	store, err := s.GetStoreForContainer(id)
	if err != nil {
		return "", err
	}
	return store.ContainerRunDirectory(id)
}

// ContainerDirectory calls the ContainerDirectory function for the store of the give container id.
func (s *multiStoreServer) ContainerDirectory(id string) (string, error) {
	store, err := s.GetStoreForContainer(id)
	if err != nil {
		return "", err
	}
	return store.ContainerDirectory(id)
}

// Shutdown stops all the image servers.
func (s *multiStoreServer) Shutdown(force bool) (layers []string, err error) {
	for k, v := range s.store {
		l, e := v.GetStore().Shutdown(force)
		if e != nil {
			logrus.Errorf("Shutdown for storage driver %s: %v", k, err)
			err = wrapMultipleErrors(err, e)
		}
		layers = append(layers, l...)
	}
	return
}

func (s *multiStoreServer) GraphRoot() string {
	return s.multiStore.GraphRoot()
}

// ResolveNames resolves the name for the given image.
func (s *multiStoreServer) ResolveNames(systemContext *ctypes.SystemContext, imageName string) ([]string, error) {
	logrus.Debugf("ResolveNames for image %s", imageName)
	s.iterator.initialize()
	for is := s.iterator.next(); is != nil; is = s.iterator.next() {
		names, err := is.ResolveNames(systemContext, imageName)
		if err != nil {
			if err == ErrCannotParseImageID || err == ErrImageMultiplyTagged {
				return []string{}, err
			}
			continue
		}
		return names, nil
	}
	return []string{}, fmt.Errorf("failed resolving image name for %s not found", imageName)
}

// ImageStatus returns the image status for the given image.
func (s *multiStoreServer) ImageStatus(systemContext *ctypes.SystemContext, filter string) (*ImageResult, error) {
	for _, s := range s.store {
		status, err := s.ImageStatus(systemContext, filter)
		if err != nil {
			if err == ErrCannotParseImageID || err == ErrImageMultiplyTagged {
				return nil, err
			}
			continue
		}
		return status, nil
	}
	return nil, cstorage.ErrImageUnknown
}

func wrapMultipleErrors(e1, e2 error) error {
	if e1 == nil {
		return e2
	}
	return errors.Wrap(e1, e2.Error())
}
