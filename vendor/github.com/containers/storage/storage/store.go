package storage

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	// register all of the built-in drivers
	_ "github.com/containers/storage/drivers/register"

	drivers "github.com/containers/storage/drivers"
	"github.com/containers/storage/pkg/archive"
	"github.com/containers/storage/pkg/idtools"
	"github.com/containers/storage/pkg/stringid"
	"github.com/containers/storage/storageversion"
)

var (
	ErrLoadError            = errors.New("error loading storage metadata")
	ErrDuplicateID          = errors.New("that ID is already in use")
	ErrDuplicateName        = errors.New("that name is already in use")
	ErrParentIsContainer    = errors.New("would-be parent layer is a container")
	ErrNotAContainer        = errors.New("identifier is not a container")
	ErrNotAnImage           = errors.New("identifier is not an image")
	ErrNotALayer            = errors.New("identifier is not a layer")
	ErrNotAnID              = errors.New("identifier is not a layer, image, or container")
	ErrLayerHasChildren     = errors.New("layer has children")
	ErrLayerUsedByImage     = errors.New("layer is in use by an image")
	ErrLayerUsedByContainer = errors.New("layer is in use by a container")
	ErrImageUsedByContainer = errors.New("image is in use by a container")
)

// FileBasedStore wraps up the most common methods of the various types of file-based
// data stores that we implement.
//
// Load() reloads the contents of the store from disk.  It should be called
// with the lock held.
//
// Save() saves the contents of the store to disk.  It should be called with
// the lock held, and Touch() should be called afterward before releasing the
// lock.
type FileBasedStore interface {
	Locker
	Load() error
	Save() error
}

// MetadataStore wraps up methods for getting and setting metadata associated with IDs.
//
// GetMetadata() reads metadata associated with an item with the specified ID.
//
// SetMetadata() updates the metadata associated with the item with the specified ID.
type MetadataStore interface {
	GetMetadata(id string) (string, error)
	SetMetadata(id, metadata string) error
}

// A BigDataStore wraps up the most common methods of the various types of
// file-based lookaside stores that we implement.
//
// SetBigData stores a (potentially large) piece of data associated with this
// ID.
//
// GetBigData retrieves a (potentially large) piece of data associated with
// this ID, if it has previously been set.
//
// GetBigDataNames() returns a list of the names of previously-stored pieces of
// data.
type BigDataStore interface {
	SetBigData(id, key string, data []byte) error
	GetBigData(id, key string) ([]byte, error)
	GetBigDataNames(id string) ([]string, error)
}

// A FlaggableStore can have flags set and cleared on items which it manages.
//
// ClearFlag removes a named flag from an item in the store.
//
// SetFlag sets a named flag and its value on an item in the store.
type FlaggableStore interface {
	ClearFlag(id string, flag string) error
	SetFlag(id string, flag string, value interface{}) error
}

// Store wraps up the various types of file-based stores that we use into a
// singleton object that initializes and manages them all together.
//
// GetRunRoot, GetGraphRoot, GetGraphDriverName, and GetGraphOptions retrieve
// settings that were passed to MakeStore() when the object was created.
//
// GetGraphDriver obtains and returns a handle to the graph Driver object used
// by the Store.
//
// GetLayerStore obtains and returns a handle to the layer store object used by
// the Store.
//
// GetImageStore obtains and returns a handle to the image store object used by
// the Store.
//
// GetContainerStore obtains and returns a handle to the container store object
// used by the Store.
//
// CreateLayer creates a new layer in the underlying storage driver, optionally
// having the specified ID (one will be assigned if none is specified), with
// the specified layer (or no layer) as its parent, and with an optional name.
// (The writeable flag is ignored.)
//
// PutLayer combines the functions of CreateLayer and ApplyDiff, marking the
// layer for automatic removal if applying the diff fails for any reason.
//
// CreateImage creates a new image, optionally with the specified ID (one will
// be assigned if none is specified), with an optional name, and referring to a
// specified image and with optional metadata.  An image is a record which
// associates the ID of a layer with a caller-supplied metadata string which
// the library stores for the convenience of the caller.
//
// CreateContainer creates a new container, optionally with the specified ID
// (one will be assigned if none is specified), with an optional name, using
// the specified image's top layer as the basis for the container's layer, and
// assigning the specified ID to that layer (one will be created if none is
// specified).  A container is a layer which is associated with a metadata
// string which the library stores for the convenience of the caller.
//
// GetMetadata retrieves the metadata which is associated with a layer, image,
// or container (whichever the passed-in ID refers to).
//
// SetMetadata updates the metadata which is associated with a layer, image, or
// container (whichever the passed-in ID refers to) to match the specified
// value.  The metadata value can be retrieved at any time using GetMetadata,
// or using GetLayer, GetImage, or GetContainer and reading the object directly.
//
// Exists checks if there is a layer, image, or container which has the
// passed-in ID or name.
//
// Status asks for a status report, in the form of key-value pairs, from the
// underlying storage driver.  The contents vary from driver to driver.
//
// Delete removes the layer, image, or container which has the passed-in ID or
// name.  Note that no safety checks are performed, so this can leave images
// with references to layers which do not exist, and layers with references to
// parents which no longer exist.
//
// DeleteLayer attempts to remove the specified layer.  If the layer is the
// parent of any other layer, or is referred to by any images, it will return
// an error.
//
// DeleteImage removes the specified image if it is not referred to by any
// containers.  If its top layer is then no longer referred to by any other
// images or the parent of any other layers, its top layer will be removed.  If
// that layer's parent is no longer referred to by any other images or the
// parent of any other layers, then it, too, will be removed.  This procedure
// will be repeated until a layer which should not be removed, or the base
// layer, is reached, at which point the list of removed layers is returned.
// If the commit argument is false, the image and layers are not removed, but
// the list of layers which would be removed is still returned.
//
// DeleteContainer removes the specified container and its layer.  If there is
// no matching container, or if the container exists but its layer does not, an
// error will be returned.
//
// Wipe removes all known layers, images, and containers.
//
// Mount attempts to mount a layer, image, or container for access, and returns
// the pathname if it succeeds.
//
// Unmount attempts to unmount a layer, image, or container, given an ID, a
// name, or a mount path.
//
// Changes returns a summary of the changes which would need to be made to one
// layer to make its contents the same as a second layer.  If the first layer
// is not specified, the second layer's parent is assumed.  Each Change
// structure contains a Path relative to the layer's root directory, and a Kind
// which is either ChangeAdd, ChangeModify, or ChangeDelete.
//
// DiffSize returns a count of the size of the tarstream which would specify
// the changes returned by Changes.
//
// Diff returns the tarstream which would specify the changes returned by
// Changes.
//
// ApplyDiff applies a tarstream to a layer.  Information about the tarstream
// is cached with the layer.  Typically, a layer which is populated using a
// tarstream will be expected to not be modified in any other way, either
// before or after the diff is applied.
//
// Layers returns a list of the currently known layers.
//
// Images returns a list of the currently known images.
//
// Containers returns a list of the currently known containers.
//
// GetNames returns the list of names for a layer, image, or container.
//
// SetNames changes the list of names for a layer, image, or container.
//
// ListImageBigData retrieves a list of the (possibly large) chunks of named
// data associated with an image.
//
// GetImageBigData retrieves a (possibly large) chunk of named data associated
// with an image.
//
// SetImageBigData stores a (possibly large) chunk of named data associated
// with an image.
//
// ListContainerBigData retrieves a list of the (possibly large) chunks of
// named data associated with a container.
//
// GetContainerBigData retrieves a (possibly large) chunk of named data
// associated with an image.
//
// SetContainerBigData stores a (possibly large) chunk of named data associated
// with an image.
//
// GetLayer returns a specific layer.
//
// GetImage returns a specific image.
//
// GetImagesByTopLayer returns a list of images which reference the specified
// layer as their top layer.  They will have different names and may have
// different metadata.
//
// GetContainer returns a specific container.
//
// GetContainerByLayer returns a specific container based on its layer ID or
// name.
//
// Lookup returns the ID of a layer, image, or container with the specified
// name.
//
// Crawl enumerates all of the layers, images, and containers which depend on
// or refer to, either directly or indirectly, the specified layer, top layer
// of an image, or container layer.
//
// Version returns version information, in the form of key-value pairs, from
// the storage package.
type Store interface {
	GetRunRoot() string
	GetGraphRoot() string
	GetGraphDriverName() string
	GetGraphOptions() []string
	GetGraphDriver() (drivers.Driver, error)
	GetLayerStore() (LayerStore, error)
	GetImageStore() (ImageStore, error)
	GetContainerStore() (ContainerStore, error)

	CreateLayer(id, parent string, names []string, mountLabel string, writeable bool) (*Layer, error)
	PutLayer(id, parent string, names []string, mountLabel string, writeable bool, diff archive.Reader) (*Layer, error)
	CreateImage(id string, names []string, layer, metadata string) (*Image, error)
	CreateContainer(id string, names []string, image, layer, metadata string) (*Container, error)
	GetMetadata(id string) (string, error)
	SetMetadata(id, metadata string) error
	Exists(id string) bool
	Status() ([][2]string, error)
	Delete(id string) error
	DeleteLayer(id string) error
	DeleteImage(id string, commit bool) (layers []string, err error)
	DeleteContainer(id string) error
	Wipe() error
	Mount(id, mountLabel string) (string, error)
	Unmount(id string) error
	Changes(from, to string) ([]archive.Change, error)
	DiffSize(from, to string) (int64, error)
	Diff(from, to string) (io.ReadCloser, error)
	ApplyDiff(to string, diff archive.Reader) (int64, error)
	Layers() ([]Layer, error)
	Images() ([]Image, error)
	Containers() ([]Container, error)
	GetNames(id string) ([]string, error)
	SetNames(id string, names []string) error
	ListImageBigData(id string) ([]string, error)
	GetImageBigData(id, key string) ([]byte, error)
	SetImageBigData(id, key string, data []byte) error
	ListContainerBigData(id string) ([]string, error)
	GetContainerBigData(id, key string) ([]byte, error)
	SetContainerBigData(id, key string, data []byte) error
	GetLayer(id string) (*Layer, error)
	GetImage(id string) (*Image, error)
	GetImagesByTopLayer(id string) ([]*Image, error)
	GetContainer(id string) (*Container, error)
	GetContainerByLayer(id string) (*Container, error)
	Lookup(name string) (string, error)
	Crawl(layerID string) (*Users, error)
	Version() ([][2]string, error)
}

// Mall is just an old name for Store.  This will be dropped at some point.
type Mall interface {
	Store
}

// Users holds an analysis of which layers, images, and containers depend on a
// given layer, either directly or indirectly.
type Users struct {
	ID                 string   `json:"id"`
	LayerID            string   `json:"layer"`
	LayersDirect       []string `json:"directlayers,omitempty"`
	LayersIndirect     []string `json:"indirectlayers,omitempty"`
	ImagesDirect       []string `json:"directimages,omitempty"`
	ImagesIndirect     []string `json:"indirectimages,omitempty"`
	ContainersDirect   []string `json:"directcontainers,omitempty"`
	ContainersIndirect []string `json:"indirectcontainers,omitempty"`
}

type store struct {
	runRoot         string
	graphLock       sync.Locker
	graphRoot       string
	graphDriverName string
	graphOptions    []string
	uidMap          []idtools.IDMap
	gidMap          []idtools.IDMap
	graphDriver     drivers.Driver
	layerStore      LayerStore
	imageStore      ImageStore
	containerStore  ContainerStore
}

// MakeStore creates and initializes a new Store object, and the underlying
// storage that it controls.
func MakeStore(runRoot, graphRoot, graphDriverName string, graphOptions []string, uidMap, gidMap []idtools.IDMap) (Mall, error) {
	if runRoot == "" && graphRoot == "" && graphDriverName == "" && len(graphOptions) == 0 {
		runRoot = "/var/run/oci-storage"
		graphRoot = "/var/lib/oci-storage"
		graphDriverName = os.Getenv("STORAGE_DRIVER")
		graphOptions = strings.Split(os.Getenv("STORAGE_OPTS"), ",")
		if len(graphOptions) == 1 && graphOptions[0] == "" {
			graphOptions = nil
		}
	}
	if err := os.MkdirAll(runRoot, 0700); err != nil && !os.IsExist(err) {
		return nil, err
	}
	for _, subdir := range []string{} {
		if err := os.MkdirAll(filepath.Join(runRoot, subdir), 0700); err != nil && !os.IsExist(err) {
			return nil, err
		}
	}
	if err := os.MkdirAll(graphRoot, 0700); err != nil && !os.IsExist(err) {
		return nil, err
	}
	for _, subdir := range []string{"mounts", "tmp", graphDriverName} {
		if err := os.MkdirAll(filepath.Join(graphRoot, subdir), 0700); err != nil && !os.IsExist(err) {
			return nil, err
		}
	}
	graphLock, err := GetLockfile(filepath.Join(graphRoot, "storage.lock"))
	if err != nil {
		return nil, err
	}
	s := &store{
		runRoot:         runRoot,
		graphLock:       graphLock,
		graphRoot:       graphRoot,
		graphDriverName: graphDriverName,
		graphOptions:    graphOptions,
		uidMap:          copyIDMap(uidMap),
		gidMap:          copyIDMap(gidMap),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func copyIDMap(idmap []idtools.IDMap) []idtools.IDMap {
	m := []idtools.IDMap{}
	if idmap != nil {
		m = make([]idtools.IDMap, len(idmap))
		copy(m, idmap)
	}
	if len(m) > 0 {
		return m[:]
	}
	return nil
}

func (s *store) GetRunRoot() string {
	return s.runRoot
}

func (s *store) GetGraphDriverName() string {
	return s.graphDriverName
}

func (s *store) GetGraphRoot() string {
	return s.graphRoot
}

func (s *store) GetGraphOptions() []string {
	return s.graphOptions
}

func (s *store) load() error {
	driver, err := drivers.New(s.graphRoot, s.graphDriverName, s.graphOptions, s.uidMap, s.gidMap)
	if err != nil {
		return err
	}
	driverPrefix := driver.String() + "-"

	rrpath := filepath.Join(s.runRoot, driverPrefix+"layers")
	if err := os.MkdirAll(rrpath, 0700); err != nil {
		return err
	}
	rlpath := filepath.Join(s.graphRoot, driverPrefix+"layers")
	if err := os.MkdirAll(rlpath, 0700); err != nil {
		return err
	}
	rls, err := newLayerStore(rrpath, rlpath, driver)
	if err != nil {
		return err
	}
	s.layerStore = rls
	ripath := filepath.Join(s.graphRoot, driverPrefix+"images")
	if err := os.MkdirAll(ripath, 0700); err != nil {
		return err
	}
	ris, err := newImageStore(ripath)
	if err != nil {
		return err
	}
	s.imageStore = ris
	rcpath := filepath.Join(s.graphRoot, driverPrefix+"containers")
	if err := os.MkdirAll(rcpath, 0700); err != nil {
		return err
	}
	rcs, err := newContainerStore(rcpath)
	if err != nil {
		return err
	}
	s.containerStore = rcs
	return nil
}

func (s *store) GetGraphDriver() (drivers.Driver, error) {
	if s.graphDriver != nil {
		return s.graphDriver, nil
	}
	return nil, ErrLoadError
}

func (s *store) GetLayerStore() (LayerStore, error) {
	if s.layerStore != nil {
		return s.layerStore, nil
	}
	return nil, ErrLoadError
}

func (s *store) GetImageStore() (ImageStore, error) {
	if s.imageStore != nil {
		return s.imageStore, nil
	}
	return nil, ErrLoadError
}

func (s *store) GetContainerStore() (ContainerStore, error) {
	if s.containerStore != nil {
		return s.containerStore, nil
	}
	return nil, ErrLoadError
}

func (s *store) PutLayer(id, parent string, names []string, mountLabel string, writeable bool, diff archive.Reader) (*Layer, error) {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return nil, err
	}
	ristore, err := s.GetImageStore()
	if err != nil {
		return nil, err
	}
	rcstore, err := s.GetContainerStore()
	if err != nil {
		return nil, err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	defer rlstore.Touch()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	defer ristore.Touch()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}
	rcstore.Lock()
	defer rcstore.Unlock()
	defer rcstore.Touch()
	if modified, err := rcstore.Modified(); modified || err != nil {
		rcstore.Load()
	}
	if id == "" {
		id = stringid.GenerateRandomID()
	}
	if parent != "" {
		if l, err := rlstore.Get(parent); err == nil && l != nil {
			parent = l.ID
		} else {
			return nil, ErrLayerUnknown
		}
		containers, err := rcstore.Containers()
		if err != nil {
			return nil, err
		}
		for _, container := range containers {
			if container.LayerID == parent {
				return nil, ErrParentIsContainer
			}
		}
	}
	return rlstore.Put(id, parent, names, mountLabel, nil, writeable, nil, diff)
}

func (s *store) CreateLayer(id, parent string, names []string, mountLabel string, writeable bool) (*Layer, error) {
	return s.PutLayer(id, parent, names, mountLabel, writeable, nil)
}

func (s *store) CreateImage(id string, names []string, layer, metadata string) (*Image, error) {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return nil, err
	}
	ristore, err := s.GetImageStore()
	if err != nil {
		return nil, err
	}
	rcstore, err := s.GetContainerStore()
	if err != nil {
		return nil, err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	defer ristore.Touch()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}
	rcstore.Lock()
	defer rcstore.Unlock()
	defer rcstore.Touch()
	if modified, err := rcstore.Modified(); modified || err != nil {
		rcstore.Load()
	}
	if id == "" {
		id = stringid.GenerateRandomID()
	}

	ilayer, err := rlstore.Get(layer)
	if err != nil {
		return nil, err
	}
	if ilayer == nil {
		return nil, ErrLayerUnknown
	}
	layer = ilayer.ID
	return ristore.Create(id, names, layer, metadata)
}

func (s *store) CreateContainer(id string, names []string, image, layer, metadata string) (*Container, error) {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return nil, err
	}
	ristore, err := s.GetImageStore()
	if err != nil {
		return nil, err
	}
	rcstore, err := s.GetContainerStore()
	if err != nil {
		return nil, err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	defer rlstore.Touch()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}
	rcstore.Lock()
	defer rcstore.Unlock()
	defer rcstore.Touch()
	if modified, err := rcstore.Modified(); modified || err != nil {
		rcstore.Load()
	}

	if id == "" {
		id = stringid.GenerateRandomID()
	}

	cimage, err := ristore.Get(image)
	if err != nil {
		return nil, err
	}
	if cimage == nil {
		return nil, ErrImageUnknown
	}
	clayer, err := rlstore.Create(layer, cimage.TopLayer, nil, "", nil, true)
	if err != nil {
		return nil, err
	}
	layer = clayer.ID
	container, err := rcstore.Create(id, names, cimage.ID, layer, metadata)
	if err != nil || container == nil {
		rlstore.Delete(layer)
	}
	return container, err
}

func (s *store) SetMetadata(id, metadata string) error {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return err
	}
	ristore, err := s.GetImageStore()
	if err != nil {
		return err
	}
	rcstore, err := s.GetContainerStore()
	if err != nil {
		return err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}
	rcstore.Lock()
	defer rcstore.Unlock()
	if modified, err := rcstore.Modified(); modified || err != nil {
		rcstore.Load()
	}

	if rcstore.Exists(id) {
		defer rcstore.Touch()
		return rcstore.SetMetadata(id, metadata)
	}
	if ristore.Exists(id) {
		defer ristore.Touch()
		return ristore.SetMetadata(id, metadata)
	}
	if rlstore.Exists(id) {
		defer rlstore.Touch()
		return rlstore.SetMetadata(id, metadata)
	}
	return ErrNotAnID
}

func (s *store) GetMetadata(id string) (string, error) {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return "", err
	}
	ristore, err := s.GetImageStore()
	if err != nil {
		return "", err
	}
	rcstore, err := s.GetContainerStore()
	if err != nil {
		return "", err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}
	rcstore.Lock()
	defer rcstore.Unlock()
	if modified, err := rcstore.Modified(); modified || err != nil {
		rcstore.Load()
	}

	if rcstore.Exists(id) {
		return rcstore.GetMetadata(id)
	}
	if ristore.Exists(id) {
		return ristore.GetMetadata(id)
	}
	if rlstore.Exists(id) {
		return rlstore.GetMetadata(id)
	}
	return "", ErrNotAnID
}

func (s *store) ListImageBigData(id string) ([]string, error) {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return nil, err
	}
	ristore, err := s.GetImageStore()
	if err != nil {
		return nil, err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}

	return ristore.GetBigDataNames(id)
}

func (s *store) GetImageBigData(id, key string) ([]byte, error) {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return nil, err
	}
	ristore, err := s.GetImageStore()
	if err != nil {
		return nil, err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}

	return ristore.GetBigData(id, key)
}

func (s *store) SetImageBigData(id, key string, data []byte) error {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return err
	}
	ristore, err := s.GetImageStore()
	if err != nil {
		return err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}

	return ristore.SetBigData(id, key, data)
}

func (s *store) ListContainerBigData(id string) ([]string, error) {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return nil, err
	}
	ristore, err := s.GetImageStore()
	if err != nil {
		return nil, err
	}
	rcstore, err := s.GetContainerStore()
	if err != nil {
		return nil, err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}
	rcstore.Lock()
	defer rcstore.Unlock()
	if modified, err := rcstore.Modified(); modified || err != nil {
		rcstore.Load()
	}

	return rcstore.GetBigDataNames(id)
}

func (s *store) GetContainerBigData(id, key string) ([]byte, error) {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return nil, err
	}
	ristore, err := s.GetImageStore()
	if err != nil {
		return nil, err
	}
	rcstore, err := s.GetContainerStore()
	if err != nil {
		return nil, err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}
	rcstore.Lock()
	defer rcstore.Unlock()
	if modified, err := rcstore.Modified(); modified || err != nil {
		rcstore.Load()
	}

	return rcstore.GetBigData(id, key)
}

func (s *store) SetContainerBigData(id, key string, data []byte) error {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return err
	}
	ristore, err := s.GetImageStore()
	if err != nil {
		return err
	}
	rcstore, err := s.GetContainerStore()
	if err != nil {
		return err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}
	rcstore.Lock()
	defer rcstore.Unlock()
	if modified, err := rcstore.Modified(); modified || err != nil {
		rcstore.Load()
	}

	return rcstore.SetBigData(id, key, data)
}

func (s *store) Exists(id string) bool {
	rcstore, err := s.GetContainerStore()
	if err != nil {
		return false
	}
	ristore, err := s.GetImageStore()
	if err != nil {
		return false
	}
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return false
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}
	rcstore.Lock()
	defer rcstore.Unlock()
	if modified, err := rcstore.Modified(); modified || err != nil {
		rcstore.Load()
	}

	if rcstore.Exists(id) {
		return true
	}
	if ristore.Exists(id) {
		return true
	}
	return rlstore.Exists(id)
}

func (s *store) SetNames(id string, names []string) error {
	rcstore, err := s.GetContainerStore()
	if err != nil {
		return err
	}
	ristore, err := s.GetImageStore()
	if err != nil {
		return err
	}
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}
	rcstore.Lock()
	defer rcstore.Unlock()
	if modified, err := rcstore.Modified(); modified || err != nil {
		rcstore.Load()
	}

	deduped := []string{}
	seen := make(map[string]bool)
	for _, name := range names {
		if _, wasSeen := seen[name]; !wasSeen {
			seen[name] = true
			deduped = append(deduped, name)
		}
	}

	if rcstore.Exists(id) {
		return rcstore.SetNames(id, deduped)
	}
	if ristore.Exists(id) {
		return ristore.SetNames(id, deduped)
	}
	if rlstore.Exists(id) {
		return rlstore.SetNames(id, deduped)
	}
	return ErrLayerUnknown
}

func (s *store) GetNames(id string) ([]string, error) {
	rcstore, err := s.GetContainerStore()
	if err != nil {
		return nil, err
	}
	ristore, err := s.GetImageStore()
	if err != nil {
		return nil, err
	}
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return nil, err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}
	rcstore.Lock()
	defer rcstore.Unlock()
	if modified, err := rcstore.Modified(); modified || err != nil {
		rcstore.Load()
	}

	if c, err := rcstore.Get(id); c != nil && err == nil {
		return c.Names, nil
	}
	if i, err := ristore.Get(id); i != nil && err == nil {
		return i.Names, nil
	}
	if l, err := rlstore.Get(id); l != nil && err == nil {
		return l.Names, nil
	}
	return nil, ErrLayerUnknown
}

func (s *store) Lookup(name string) (string, error) {
	rcstore, err := s.GetContainerStore()
	if err != nil {
		return "", err
	}
	ristore, err := s.GetImageStore()
	if err != nil {
		return "", err
	}
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return "", err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}
	rcstore.Lock()
	defer rcstore.Unlock()
	if modified, err := rcstore.Modified(); modified || err != nil {
		rcstore.Load()
	}

	if c, err := rcstore.Get(name); c != nil && err == nil {
		return c.ID, nil
	}
	if i, err := ristore.Get(name); i != nil && err == nil {
		return i.ID, nil
	}
	if l, err := rlstore.Get(name); l != nil && err == nil {
		return l.ID, nil
	}
	return "", ErrLayerUnknown
}

func (s *store) DeleteLayer(id string) error {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return err
	}
	ristore, err := s.GetImageStore()
	if err != nil {
		return err
	}
	rcstore, err := s.GetContainerStore()
	if err != nil {
		return err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}
	rcstore.Lock()
	defer rcstore.Unlock()
	if modified, err := rcstore.Modified(); modified || err != nil {
		rcstore.Load()
	}

	if rlstore.Exists(id) {
		defer rlstore.Touch()
		defer rcstore.Touch()
		if l, err := rlstore.Get(id); err != nil {
			id = l.ID
		}
		layers, err := rlstore.Layers()
		if err != nil {
			return err
		}
		for _, layer := range layers {
			if layer.Parent == id {
				return ErrLayerHasChildren
			}
		}
		images, err := ristore.Images()
		if err != nil {
			return err
		}
		for _, image := range images {
			if image.TopLayer == id {
				return ErrLayerUsedByImage
			}
		}
		containers, err := rcstore.Containers()
		if err != nil {
			return err
		}
		for _, container := range containers {
			if container.LayerID == id {
				return ErrLayerUsedByContainer
			}
		}
		return rlstore.Delete(id)
	} else {
		return ErrNotALayer
	}
}

func (s *store) DeleteImage(id string, commit bool) (layers []string, err error) {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return nil, err
	}
	ristore, err := s.GetImageStore()
	if err != nil {
		return nil, err
	}
	rcstore, err := s.GetContainerStore()
	if err != nil {
		return nil, err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}
	rcstore.Lock()
	defer rcstore.Unlock()
	if modified, err := rcstore.Modified(); modified || err != nil {
		rcstore.Load()
	}
	layersToRemove := []string{}
	if ristore.Exists(id) {
		image, err := ristore.Get(id)
		if err != nil {
			return nil, err
		}
		id = image.ID
		defer rlstore.Touch()
		defer ristore.Touch()
		containers, err := rcstore.Containers()
		if err != nil {
			return nil, err
		}
		aContainerByImage := make(map[string]string)
		for _, container := range containers {
			aContainerByImage[container.ImageID] = container.ID
		}
		if _, ok := aContainerByImage[id]; ok {
			return nil, ErrImageUsedByContainer
		}
		images, err := ristore.Images()
		if err != nil {
			return nil, err
		}
		layers, err := rlstore.Layers()
		if err != nil {
			return nil, err
		}
		childrenByParent := make(map[string]*[]string)
		for _, layer := range layers {
			parent := layer.Parent
			if list, ok := childrenByParent[parent]; ok {
				newList := append(*list, layer.ID)
				childrenByParent[parent] = &newList
			} else {
				childrenByParent[parent] = &([]string{layer.ID})
			}
		}
		anyImageByTopLayer := make(map[string]string)
		for _, img := range images {
			if img.ID != id {
				anyImageByTopLayer[img.TopLayer] = img.ID
			}
		}
		if commit {
			if err = ristore.Delete(id); err != nil {
				return nil, err
			}
		}
		layer := image.TopLayer
		lastRemoved := ""
		for layer != "" {
			if rcstore.Exists(layer) {
				break
			}
			if _, ok := anyImageByTopLayer[layer]; ok {
				break
			}
			parent := ""
			if l, err := rlstore.Get(layer); err == nil {
				parent = l.Parent
			}
			otherRefs := 0
			if childList, ok := childrenByParent[layer]; ok && childList != nil {
				children := *childList
				for _, child := range children {
					if child != lastRemoved {
						otherRefs++
					}
				}
			}
			if otherRefs != 0 {
				break
			}
			lastRemoved = layer
			layersToRemove = append(layersToRemove, lastRemoved)
			layer = parent
		}
	} else {
		return nil, ErrNotAnImage
	}
	if commit {
		for _, layer := range layersToRemove {
			if err = rlstore.Delete(layer); err != nil {
				return nil, err
			}
		}
	}
	return layersToRemove, nil
}

func (s *store) DeleteContainer(id string) error {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return err
	}
	ristore, err := s.GetImageStore()
	if err != nil {
		return err
	}
	rcstore, err := s.GetContainerStore()
	if err != nil {
		return err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}
	rcstore.Lock()
	defer rcstore.Unlock()
	if modified, err := rcstore.Modified(); modified || err != nil {
		rcstore.Load()
	}

	if rcstore.Exists(id) {
		defer rlstore.Touch()
		defer rcstore.Touch()
		if container, err := rcstore.Get(id); err == nil {
			if rlstore.Exists(container.LayerID) {
				if err := rlstore.Delete(container.LayerID); err != nil {
					return err
				}
				return rcstore.Delete(id)
			} else {
				return ErrNotALayer
			}
		}
	} else {
		return ErrNotAContainer
	}
	return nil
}

func (s *store) Delete(id string) error {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return err
	}
	ristore, err := s.GetImageStore()
	if err != nil {
		return err
	}
	rcstore, err := s.GetContainerStore()
	if err != nil {
		return err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}
	rcstore.Lock()
	defer rcstore.Unlock()
	if modified, err := rcstore.Modified(); modified || err != nil {
		rcstore.Load()
	}

	if rcstore.Exists(id) {
		defer rlstore.Touch()
		defer rcstore.Touch()
		if container, err := rcstore.Get(id); err == nil {
			if rlstore.Exists(container.LayerID) {
				if err := rlstore.Delete(container.LayerID); err != nil {
					return err
				}
				return rcstore.Delete(id)
			} else {
				return ErrNotALayer
			}
		}
	}
	if ristore.Exists(id) {
		defer ristore.Touch()
		return ristore.Delete(id)
	}
	if rlstore.Exists(id) {
		defer rlstore.Touch()
		return rlstore.Delete(id)
	}
	return ErrLayerUnknown
}

func (s *store) Wipe() error {
	rcstore, err := s.GetContainerStore()
	if err != nil {
		return err
	}
	ristore, err := s.GetImageStore()
	if err != nil {
		return err
	}
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	defer rlstore.Touch()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	defer ristore.Touch()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}
	rcstore.Lock()
	defer rcstore.Unlock()
	defer rcstore.Touch()
	if modified, err := rcstore.Modified(); modified || err != nil {
		rcstore.Load()
	}

	if err = rcstore.Wipe(); err != nil {
		return err
	}
	if err = ristore.Wipe(); err != nil {
		return err
	}
	return rlstore.Wipe()
}

func (s *store) Status() ([][2]string, error) {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return nil, err
	}
	return rlstore.Status()
}

func (s *store) Version() ([][2]string, error) {
	return [][2]string{
		{"GitCommit", storageversion.GitCommit},
		{"Version", storageversion.Version},
		{"BuildTime", storageversion.BuildTime},
	}, nil
}

func (s *store) Mount(id, mountLabel string) (string, error) {
	rcstore, err := s.GetContainerStore()
	if err != nil {
		return "", err
	}
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return "", err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	defer rlstore.Touch()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	rcstore.Lock()
	defer rcstore.Unlock()
	if modified, err := rcstore.Modified(); modified || err != nil {
		rcstore.Load()
	}

	if c, err := rcstore.Get(id); c != nil && err == nil {
		id = c.LayerID
	}
	return rlstore.Mount(id, mountLabel)
}

func (s *store) Unmount(id string) error {
	rcstore, err := s.GetContainerStore()
	if err != nil {
		return err
	}
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	defer rlstore.Touch()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	rcstore.Lock()
	defer rcstore.Unlock()
	if modified, err := rcstore.Modified(); modified || err != nil {
		rcstore.Load()
	}

	if c, err := rcstore.Get(id); c != nil && err == nil {
		id = c.LayerID
	}
	return rlstore.Unmount(id)
}

func (s *store) Changes(from, to string) ([]archive.Change, error) {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return nil, err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}

	return rlstore.Changes(from, to)
}

func (s *store) DiffSize(from, to string) (int64, error) {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return -1, err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}

	return rlstore.DiffSize(from, to)
}

func (s *store) Diff(from, to string) (io.ReadCloser, error) {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return nil, err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}

	return rlstore.Diff(from, to)
}

func (s *store) ApplyDiff(to string, diff archive.Reader) (int64, error) {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return -1, err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}

	return rlstore.ApplyDiff(to, diff)
}

func (s *store) Layers() ([]Layer, error) {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return nil, err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}

	return rlstore.Layers()
}

func (s *store) Images() ([]Image, error) {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return nil, err
	}
	ristore, err := s.GetImageStore()
	if err != nil {
		return nil, err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}

	return ristore.Images()
}

func (s *store) Containers() ([]Container, error) {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return nil, err
	}
	ristore, err := s.GetImageStore()
	if err != nil {
		return nil, err
	}
	rcstore, err := s.GetContainerStore()
	if err != nil {
		return nil, err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}
	rcstore.Lock()
	defer rcstore.Unlock()
	if modified, err := rcstore.Modified(); modified || err != nil {
		rcstore.Load()
	}

	return rcstore.Containers()
}

func (s *store) GetLayer(id string) (*Layer, error) {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return nil, err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}

	return rlstore.Get(id)
}

func (s *store) GetImage(id string) (*Image, error) {
	ristore, err := s.GetImageStore()
	if err != nil {
		return nil, err
	}
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return nil, err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}

	return ristore.Get(id)
}

func (s *store) GetImagesByTopLayer(id string) ([]*Image, error) {
	ristore, err := s.GetImageStore()
	if err != nil {
		return nil, err
	}
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return nil, err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}

	layer, err := rlstore.Get(id)
	if err != nil {
		return nil, err
	}
	images := []*Image{}
	imageList, err := ristore.Images()
	if err != nil {
		return nil, err
	}
	for _, image := range imageList {
		if image.TopLayer == layer.ID {
			images = append(images, &image)
		}
	}

	return images, nil
}

func (s *store) GetContainer(id string) (*Container, error) {
	ristore, err := s.GetImageStore()
	if err != nil {
		return nil, err
	}
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return nil, err
	}
	rcstore, err := s.GetContainerStore()
	if err != nil {
		return nil, err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}
	rcstore.Lock()
	defer rcstore.Unlock()
	if modified, err := rcstore.Modified(); modified || err != nil {
		rcstore.Load()
	}

	return rcstore.Get(id)
}

func (s *store) GetContainerByLayer(id string) (*Container, error) {
	ristore, err := s.GetImageStore()
	if err != nil {
		return nil, err
	}
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return nil, err
	}
	rcstore, err := s.GetContainerStore()
	if err != nil {
		return nil, err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}
	rcstore.Lock()
	defer rcstore.Unlock()
	if modified, err := rcstore.Modified(); modified || err != nil {
		rcstore.Load()
	}

	layer, err := rlstore.Get(id)
	if err != nil {
		return nil, err
	}
	containerList, err := rcstore.Containers()
	if err != nil {
		return nil, err
	}
	for _, container := range containerList {
		if container.LayerID == layer.ID {
			return &container, nil
		}
	}

	return nil, ErrContainerUnknown
}

func (s *store) Crawl(layerID string) (*Users, error) {
	rlstore, err := s.GetLayerStore()
	if err != nil {
		return nil, err
	}
	ristore, err := s.GetImageStore()
	if err != nil {
		return nil, err
	}
	rcstore, err := s.GetContainerStore()
	if err != nil {
		return nil, err
	}

	rlstore.Lock()
	defer rlstore.Unlock()
	if modified, err := rlstore.Modified(); modified || err != nil {
		rlstore.Load()
	}
	ristore.Lock()
	defer ristore.Unlock()
	if modified, err := ristore.Modified(); modified || err != nil {
		ristore.Load()
	}
	rcstore.Lock()
	defer rcstore.Unlock()
	if modified, err := rcstore.Modified(); modified || err != nil {
		rcstore.Load()
	}

	u := &Users{}
	if container, err := rcstore.Get(layerID); err == nil {
		u.ID = container.ID
		layerID = container.LayerID
	}
	if image, err := ristore.Get(layerID); err == nil {
		u.ID = image.ID
		layerID = image.TopLayer
	}
	if layer, err := rlstore.Get(layerID); err == nil {
		u.ID = layer.ID
		layerID = layer.ID
	}
	if u.ID == "" {
		return nil, ErrLayerUnknown
	}
	u.LayerID = layerID
	layers, err := rlstore.Layers()
	if err != nil {
		return nil, err
	}
	images, err := ristore.Images()
	if err != nil {
		return nil, err
	}
	containers, err := rcstore.Containers()
	if err != nil {
		return nil, err
	}
	children := make(map[string][]string)
nextLayer:
	for _, layer := range layers {
		for _, container := range containers {
			if container.LayerID == layer.ID {
				break nextLayer
			}
		}
		if childs, known := children[layer.Parent]; known {
			newChildren := append(childs, layer.ID)
			children[layer.Parent] = newChildren
		} else {
			children[layer.Parent] = []string{layer.ID}
		}
	}
	if childs, known := children[layerID]; known {
		u.LayersDirect = childs
	}
	indirects := []string{}
	examined := make(map[string]bool)
	queue := u.LayersDirect
	for n := 0; n < len(queue); n++ {
		if _, skip := examined[queue[n]]; skip {
			continue
		}
		examined[queue[n]] = true
		for _, child := range children[queue[n]] {
			queue = append(queue, child)
			indirects = append(indirects, child)
		}
	}
	u.LayersIndirect = indirects
	for _, image := range images {
		if image.TopLayer == layerID {
			if u.ImagesDirect == nil {
				u.ImagesDirect = []string{image.ID}
			} else {
				u.ImagesDirect = append(u.ImagesDirect, image.ID)
			}
		} else {
			if _, isDescended := examined[image.TopLayer]; isDescended {
				if u.ImagesIndirect == nil {
					u.ImagesIndirect = []string{image.ID}
				} else {
					u.ImagesIndirect = append(u.ImagesIndirect, image.ID)
				}
			}
		}
	}
	for _, container := range containers {
		parent := ""
		if l, _ := rlstore.Get(container.LayerID); l != nil {
			parent = l.Parent
		}
		if parent == layerID {
			if u.ContainersDirect == nil {
				u.ContainersDirect = []string{container.ID}
			} else {
				u.ContainersDirect = append(u.ContainersDirect, container.ID)
			}
		} else {
			if _, isDescended := examined[parent]; isDescended {
				if u.ContainersIndirect == nil {
					u.ContainersIndirect = []string{container.ID}
				} else {
					u.ContainersIndirect = append(u.ContainersIndirect, container.ID)
				}
			}
		}
	}
	return u, nil
}

// MakeMall was the old name of MakeStore.  It will be dropped at some point.
func MakeMall(runRoot, graphRoot, graphDriverName string, graphOptions []string) (Mall, error) {
	return MakeStore(runRoot, graphRoot, graphDriverName, graphOptions, nil, nil)
}
