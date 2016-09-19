package storage

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/containers/storage/pkg/ioutils"
	"github.com/containers/storage/pkg/stringid"
)

var (
	// ErrContainerUnknown indicates that there was no container with the specified name or ID
	ErrContainerUnknown = errors.New("container not known")
)

// A Container is a reference to a read-write layer with a metadata string.
// ID is either one specified at create-time or a randomly-generated value.
// Names is an optional set of user-defined convenience values.
// ImageID is the ID of the image which was used to create the container.
// LayerID is the ID of the read-write layer for the container itself.
// It is assumed that the image's top layer is the parent of the container's
// read-write layer.
type Container struct {
	ID           string                 `json:"id"`
	Names        []string               `json:"names,omitempty"`
	ImageID      string                 `json:"image"`
	LayerID      string                 `json:"layer"`
	Metadata     string                 `json:"metadata,omitempty"`
	BigDataNames []string               `json:"big-data-names,omitempty"`
	Flags        map[string]interface{} `json:"flags,omitempty"`
}

// ContainerStore provides bookkeeping for information about Containers.
//
// Create creates a container that has a specified ID (or a random one) and an
// optional name, based on the specified image, using the specified layer as
// its read-write layer.
//
// GetMetadata retrieves a container's metadata.
//
// SetMetadata replaces the metadata associated with a container with the
// supplied value.
//
// Exists checks if there is a container with the given ID or name.
//
// Get retrieves information about a container given an ID or name.
//
// Delete removes the record of the container.
//
// Wipe removes records of all containers.
//
// Lookup attempts to translate a name to an ID.  Most methods do this
// implicitly.
//
// Containers returns a slice enumerating the known containers.
type ContainerStore interface {
	FileBasedStore
	MetadataStore
	BigDataStore
	FlaggableStore
	Create(id string, names []string, image, layer, metadata string) (*Container, error)
	SetNames(id string, names []string) error
	Get(id string) (*Container, error)
	Exists(id string) bool
	Delete(id string) error
	Wipe() error
	Lookup(name string) (string, error)
	Containers() ([]Container, error)
}

type containerStore struct {
	lockfile   Locker
	dir        string
	containers []Container
	byid       map[string]*Container
	bylayer    map[string]*Container
	byname     map[string]*Container
}

func (r *containerStore) Containers() ([]Container, error) {
	return r.containers, nil
}

func (r *containerStore) containerspath() string {
	return filepath.Join(r.dir, "containers.json")
}

func (r *containerStore) datadir(id string) string {
	return filepath.Join(r.dir, id)
}

func (r *containerStore) datapath(id, key string) string {
	return filepath.Join(r.datadir(id), base64.StdEncoding.EncodeToString([]byte(key)))
}

func (r *containerStore) Load() error {
	rpath := r.containerspath()
	data, err := ioutil.ReadFile(rpath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	containers := []Container{}
	layers := make(map[string]*Container)
	ids := make(map[string]*Container)
	names := make(map[string]*Container)
	if err = json.Unmarshal(data, &containers); len(data) == 0 || err == nil {
		for n, container := range containers {
			ids[container.ID] = &containers[n]
			layers[container.LayerID] = &containers[n]
			for _, name := range container.Names {
				names[name] = &containers[n]
			}
		}
	}
	r.containers = containers
	r.byid = ids
	r.bylayer = layers
	r.byname = names
	return nil
}

func (r *containerStore) Save() error {
	rpath := r.containerspath()
	jdata, err := json.Marshal(&r.containers)
	if err != nil {
		return err
	}
	return ioutils.AtomicWriteFile(rpath, jdata, 0600)
}

func newContainerStore(dir string) (ContainerStore, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	lockfile, err := GetLockfile(filepath.Join(dir, "containers.lock"))
	if err != nil {
		return nil, err
	}
	lockfile.Lock()
	defer lockfile.Unlock()
	cstore := containerStore{
		lockfile:   lockfile,
		dir:        dir,
		containers: []Container{},
		byid:       make(map[string]*Container),
		bylayer:    make(map[string]*Container),
		byname:     make(map[string]*Container),
	}
	if err := cstore.Load(); err != nil {
		return nil, err
	}
	return &cstore, nil
}

func (r *containerStore) ClearFlag(id string, flag string) error {
	if container, ok := r.byname[id]; ok {
		id = container.ID
	} else if container, ok := r.bylayer[id]; ok {
		id = container.ID
	}
	if _, ok := r.byid[id]; !ok {
		return ErrImageUnknown
	}
	container := r.byid[id]
	delete(container.Flags, flag)
	return r.Save()
}

func (r *containerStore) SetFlag(id string, flag string, value interface{}) error {
	if container, ok := r.byname[id]; ok {
		id = container.ID
	} else if container, ok := r.bylayer[id]; ok {
		id = container.ID
	}
	if _, ok := r.byid[id]; !ok {
		return ErrImageUnknown
	}
	container := r.byid[id]
	container.Flags[flag] = value
	return r.Save()
}

func (r *containerStore) Create(id string, names []string, image, layer, metadata string) (container *Container, err error) {
	if id == "" {
		id = stringid.GenerateRandomID()
	}
	if _, idInUse := r.byid[id]; idInUse {
		return nil, ErrDuplicateID
	}
	for _, name := range names {
		if _, nameInUse := r.byname[name]; nameInUse {
			return nil, ErrDuplicateName
		}
	}
	if err == nil {
		newContainer := Container{
			ID:           id,
			Names:        names,
			ImageID:      image,
			LayerID:      layer,
			Metadata:     metadata,
			BigDataNames: []string{},
			Flags:        make(map[string]interface{}),
		}
		r.containers = append(r.containers, newContainer)
		container = &r.containers[len(r.containers)-1]
		r.byid[id] = container
		r.bylayer[layer] = container
		for _, name := range names {
			r.byname[name] = container
		}
		err = r.Save()
	}
	return container, err
}

func (r *containerStore) GetMetadata(id string) (string, error) {
	if container, ok := r.byname[id]; ok {
		id = container.ID
	} else if container, ok := r.bylayer[id]; ok {
		id = container.ID
	}
	if container, ok := r.byid[id]; ok {
		return container.Metadata, nil
	}
	return "", ErrContainerUnknown
}

func (r *containerStore) SetMetadata(id, metadata string) error {
	if container, ok := r.byname[id]; ok {
		id = container.ID
	} else if container, ok := r.bylayer[id]; ok {
		id = container.ID
	}
	if container, ok := r.byid[id]; ok {
		container.Metadata = metadata
		return r.Save()
	}
	return ErrContainerUnknown
}

func (r *containerStore) removeName(container *Container, name string) {
	newNames := []string{}
	for _, oldName := range container.Names {
		if oldName != name {
			newNames = append(newNames, oldName)
		}
	}
	container.Names = newNames
}

func (r *containerStore) SetNames(id string, names []string) error {
	if container, ok := r.byname[id]; ok {
		id = container.ID
	} else if container, ok := r.bylayer[id]; ok {
		id = container.ID
	}
	if container, ok := r.byid[id]; ok {
		for _, name := range container.Names {
			delete(r.byname, name)
		}
		for _, name := range names {
			if otherContainer, ok := r.byname[name]; ok {
				r.removeName(otherContainer, name)
			}
			r.byname[name] = container
		}
		container.Names = names
		return r.Save()
	}
	return ErrContainerUnknown
}

func (r *containerStore) Delete(id string) error {
	if container, ok := r.byname[id]; ok {
		id = container.ID
	} else if container, ok := r.bylayer[id]; ok {
		id = container.ID
	}
	if _, ok := r.byid[id]; !ok {
		return ErrContainerUnknown
	}
	if container, ok := r.byid[id]; ok {
		newContainers := []Container{}
		for _, candidate := range r.containers {
			if candidate.ID != id {
				newContainers = append(newContainers, candidate)
			}
		}
		for _, name := range container.Names {
			delete(r.byname, name)
		}
		r.containers = newContainers
		if err := r.Save(); err != nil {
			return err
		}
		if err := os.RemoveAll(r.datadir(id)); err != nil {
			return err
		}
	}
	return nil
}

func (r *containerStore) Get(id string) (*Container, error) {
	if c, ok := r.byname[id]; ok {
		return c, nil
	} else if c, ok := r.bylayer[id]; ok {
		return c, nil
	}
	if c, ok := r.byid[id]; ok {
		return c, nil
	}
	return nil, ErrContainerUnknown
}

func (r *containerStore) Lookup(name string) (id string, err error) {
	container, ok := r.byname[name]
	if !ok {
		return "", ErrContainerUnknown
	}
	return container.ID, nil
}

func (r *containerStore) Exists(id string) bool {
	if _, ok := r.byname[id]; ok {
		return true
	}
	if _, ok := r.bylayer[id]; ok {
		return true
	}
	if _, ok := r.byid[id]; ok {
		return true
	}
	return false
}

func (r *containerStore) GetBigData(id, key string) ([]byte, error) {
	if img, ok := r.byname[id]; ok {
		id = img.ID
	}
	if _, ok := r.byid[id]; !ok {
		return nil, ErrImageUnknown
	}
	return ioutil.ReadFile(r.datapath(id, key))
}

func (r *containerStore) GetBigDataNames(id string) ([]string, error) {
	if img, ok := r.byname[id]; ok {
		id = img.ID
	}
	if _, ok := r.byid[id]; !ok {
		return nil, ErrImageUnknown
	}
	return r.byid[id].BigDataNames, nil
}

func (r *containerStore) SetBigData(id, key string, data []byte) error {
	if img, ok := r.byname[id]; ok {
		id = img.ID
	}
	if _, ok := r.byid[id]; !ok {
		return ErrImageUnknown
	}
	if err := os.MkdirAll(r.datadir(id), 0700); err != nil {
		return err
	}
	err := ioutils.AtomicWriteFile(r.datapath(id, key), data, 0600)
	if err == nil {
		add := true
		for _, name := range r.byid[id].BigDataNames {
			if name == key {
				add = false
				break
			}
		}
		if add {
			r.byid[id].BigDataNames = append(r.byid[id].BigDataNames, key)
			err = r.Save()
		}
	}
	return err
}

func (r *containerStore) Wipe() error {
	ids := []string{}
	for id := range r.byid {
		ids = append(ids, id)
	}
	for _, id := range ids {
		if err := r.Delete(id); err != nil {
			return err
		}
	}
	return nil
}

func (r *containerStore) Lock() {
	r.lockfile.Lock()
}

func (r *containerStore) Unlock() {
	r.lockfile.Unlock()
}

func (r *containerStore) Touch() error {
	return r.lockfile.Touch()
}

func (r *containerStore) Modified() (bool, error) {
	return r.lockfile.Modified()
}
