package define

import "github.com/containers/storage/pkg/idtools"

// Info is the overall struct that describes the host system
// running libpod/podman
type Info struct {
	Host       *HostInfo              `json:"host"`
	Store      *StoreInfo             `json:"store"`
	Registries map[string]interface{} `json:"registries"`
}

//HostInfo describes the libpod host
type HostInfo struct {
	Arch           string                 `json:"arch"`
	BuildahVersion string                 `json:"buildahVersion"`
	CGroupsVersion string                 `json:"cgroupVersion"`
	Conmon         *ConmonInfo            `json:"conmon"`
	CPUs           int                    `json:"cpus"`
	Distribution   DistributionInfo       `json:"distribution"`
	EventLogger    string                 `json:"eventLogger"`
	Hostname       string                 `json:"hostname"`
	IDMappings     IDMappings             `json:"idMappings,omitempty"`
	Kernel         string                 `json:"kernel"`
	MemFree        int64                  `json:"memFree"`
	MemTotal       int64                  `json:"memTotal"`
	OCIRuntime     *OCIRuntimeInfo        `json:"ociRuntime"`
	OS             string                 `json:"os"`
	Rootless       bool                   `json:"rootless"`
	RuntimeInfo    map[string]interface{} `json:"runtimeInfo,omitempty"`
	Slirp4NetNS    SlirpInfo              `json:"slirp4netns,omitempty"`
	SwapFree       int64                  `json:"swapFree"`
	SwapTotal      int64                  `json:"swapTotal"`
	Uptime         string                 `json:"uptime"`
}

// SlirpInfo describes the slirp exectuable that
// is being being used.
type SlirpInfo struct {
	Executable string `json:"executable"`
	Package    string `json:"package"`
	Version    string `json:"version"`
}

// IDMappings describe the GID and UID mappings
type IDMappings struct {
	GIDMap []idtools.IDMap `json:"gidmap"`
	UIDMap []idtools.IDMap `json:"uidmap"`
}

// DistributionInfo describes the host distribution
// for libpod
type DistributionInfo struct {
	Distribution string `json:"distribution"`
	Version      string `json:"version"`
}

// ConmonInfo describes the conmon executable being used
type ConmonInfo struct {
	Package string `json:"package"`
	Path    string `json:"path"`
	Version string `json:"version"`
}

// OCIRuntimeInfo describes the runtime (crun or runc) being
// used with podman
type OCIRuntimeInfo struct {
	Name    string `json:"name"`
	Package string `json:"package"`
	Path    string `json:"path"`
	Version string `json:"version"`
}

// StoreInfo describes the container storage and its
// attributes
type StoreInfo struct {
	ConfigFile      string                 `json:"configFile"`
	ContainerStore  ContainerStore         `json:"containerStore"`
	GraphDriverName string                 `json:"graphDriverName"`
	GraphOptions    map[string]interface{} `json:"graphOptions"`
	GraphRoot       string                 `json:"graphRoot"`
	GraphStatus     map[string]string      `json:"graphStatus"`
	ImageStore      ImageStore             `json:"imageStore"`
	RunRoot         string                 `json:"runRoot"`
	VolumePath      string                 `json:"volumePath"`
}

// ImageStore describes the image store.  Right now only the number
// of images present
type ImageStore struct {
	Number int `json:"number"`
}

// ContainerStore describes the quantity of containers in the
// store by status
type ContainerStore struct {
	Number  int `json:"number"`
	Paused  int `json:"paused"`
	Running int `json:"running"`
	Stopped int `json:"stopped"`
}
