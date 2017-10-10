package libpod

import (
	"os"
	"sync"

	is "github.com/containers/image/storage"
	"github.com/containers/image/types"
	"github.com/containers/storage"
	"github.com/pkg/errors"
	"github.com/ulule/deepcopier"
)

// A RuntimeOption is a functional option which alters the Runtime created by
// NewRuntime
type RuntimeOption func(*Runtime) error

// Runtime is the core libpod runtime
type Runtime struct {
	config         *RuntimeConfig
	state          State
	store          storage.Store
	storageService *storageService
	imageContext   *types.SystemContext
	ociRuntime     *OCIRuntime
	valid          bool
	lock           sync.RWMutex
}

// RuntimeConfig contains configuration options used to set up the runtime
type RuntimeConfig struct {
	StorageConfig         storage.StoreOptions
	ImageDefaultTransport string
	InsecureRegistries    []string
	Registries            []string
	SignaturePolicyPath   string
	RuntimePath           string
	ConmonPath            string
	ConmonEnvVars         []string
	CgroupManager         string
	ExitsDir              string
	SelinuxEnabled        bool
	PidsLimit             int64
	MaxLogSize            int64
	NoPivotRoot           bool
}

var (
	defaultRuntimeConfig = RuntimeConfig{
		// Leave this empty so containers/storage will use its defaults
		StorageConfig:         storage.StoreOptions{},
		ImageDefaultTransport: "docker://",
		RuntimePath:           "/usr/bin/runc",
		ConmonPath:            "/usr/local/libexec/crio/conmon",
		ConmonEnvVars: []string{
			"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		},
		CgroupManager:  "cgroupfs",
		ExitsDir:       "/var/run/libpod/exits",
		SelinuxEnabled: false,
		PidsLimit:      1024,
		MaxLogSize:     -1,
		NoPivotRoot:    false,
	}
)

// NewRuntime creates a new container runtime
// Options can be passed to override the default configuration for the runtime
func NewRuntime(options ...RuntimeOption) (*Runtime, error) {
	runtime := new(Runtime)
	runtime.config = new(RuntimeConfig)

	// Copy the default configuration
	deepcopier.Copy(defaultRuntimeConfig).To(runtime.config)

	// Overwrite it with user-given configuration options
	for _, opt := range options {
		if err := opt(runtime); err != nil {
			return nil, errors.Wrapf(err, "error configuring runtime")
		}
	}

	// Set up containers/storage
	store, err := storage.GetStore(runtime.config.StorageConfig)
	if err != nil {
		return nil, err
	}
	runtime.store = store
	is.Transport.SetStore(store)

	// TODO remove StorageImageServer and make its functions work directly
	// on Runtime (or convert to something that satisfies an image)
	storageService, err := getStorageService(runtime.store)
	if err != nil {
		return nil, err
	}
	runtime.storageService = storageService

	// Set up containers/image
	runtime.imageContext = &types.SystemContext{
		SignaturePolicyPath: runtime.config.SignaturePolicyPath,
	}

	// Set up the state
	state, err := NewInMemoryState()
	if err != nil {
		return nil, err
	}
	runtime.state = state

	// Make an OCI runtime to perform container operations
	ociRuntime, err := newOCIRuntime("runc", runtime.config.RuntimePath,
		runtime.config.ConmonPath, runtime.config.ConmonEnvVars,
		runtime.config.CgroupManager, runtime.config.ExitsDir,
		runtime.config.MaxLogSize, runtime.config.NoPivotRoot)
	if err != nil {
		return nil, err
	}
	runtime.ociRuntime = ociRuntime

	// Make the directory that will hold container exit files
	if err := os.MkdirAll(runtime.config.ExitsDir, 0755); err != nil {
		// The directory is allowed to exist
		if !os.IsExist(err) {
			return nil, errors.Wrapf(err, "error creating container exit files directory %s",
				runtime.config.ExitsDir)
		}
	}

	// Mark the runtime as valid - ready to be used, cannot be modified
	// further
	runtime.valid = true

	return runtime, nil
}

// GetConfig returns a copy of the configuration used by the runtime
func (r *Runtime) GetConfig() *RuntimeConfig {
	r.lock.RLock()
	defer r.lock.RUnlock()

	if !r.valid {
		return nil
	}

	config := new(RuntimeConfig)

	// Copy so the caller won't be able to modify the actual config
	deepcopier.Copy(r.config).To(config)

	return config
}

// Shutdown shuts down the runtime and associated containers and storage
// If force is true, containers and mounted storage will be shut down before
// cleaning up; if force is false, an error will be returned if there are
// still containers running or mounted
func (r *Runtime) Shutdown(force bool) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	if !r.valid {
		return ErrRuntimeStopped
	}

	r.valid = false

	_, err := r.store.Shutdown(force)
	return err
}
