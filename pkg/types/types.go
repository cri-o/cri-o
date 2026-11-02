package types

import (
	"go.podman.io/storage/pkg/idtools"
)

// ContainerInfo stores information about containers.
type ContainerInfo struct {
	Name            string            `json:"name"`
	Pid             int               `json:"pid"`
	Image           string            `json:"image"`     // If set, _some_ name of the image imageID; it may have NO RELATIONSHIP to the users’ requested image name.
	ImageRef        string            `json:"image_ref"` // In the format of StorageImageID.StringForOutOfProcessConsumptionOnly(), or "".
	CreatedTime     int64             `json:"created_time"`
	Labels          map[string]string `json:"labels"`
	Annotations     map[string]string `json:"annotations"`
	CrioAnnotations map[string]string `json:"crio_annotations"`
	LogPath         string            `json:"log_path"`
	Root            string            `json:"root"`
	Sandbox         string            `json:"sandbox"`
	IPs             []string          `json:"ip_addresses"`
	HostNetwork     *bool             `json:"host_network"`
}

// IDMappings specifies the ID mappings used for containers.
type IDMappings struct {
	Uids []idtools.IDMap `json:"uids"`
	Gids []idtools.IDMap `json:"gids"`
}

// CrioInfo stores information about the crio daemon.
type CrioInfo struct {
	StorageDriver     string     `json:"storage_driver"`
	StorageImage      string     `json:"storage_image"`
	StorageRoot       string     `json:"storage_root"`
	CgroupDriver      string     `json:"cgroup_driver"`
	DefaultIDMappings IDMappings `json:"default_id_mappings"`
}

// DedupResult stores the outcome of an on-demand, store-wide layer
// deduplication pass triggered via `crio dedup`.
type DedupResult struct {
	// DurationSeconds is how long the pass took to complete.
	DurationSeconds float64 `json:"duration_seconds"`
	// SavedMB is the total bytes (in MB) currently shared across the store
	// via reflinks, as reported by the storage library. This is a
	// cumulative total as of this pass, not a delta since the last pass.
	SavedMB float64 `json:"saved_mb"`
	// Error is set if the pass failed. Empty on success.
	Error string `json:"error,omitempty"`
}
