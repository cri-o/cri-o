package types

import rspec "github.com/opencontainers/runtime-spec/specs-go"

// ContainerInfo stores information about containers
type ContainerInfo struct {
	Name            string            `json:"name"`
	Pid             int               `json:"pid"`
	Image           string            `json:"image"`
	ImageRef        string            `json:"image_ref"`
	CreatedTime     int64             `json:"created_time"`
	Labels          map[string]string `json:"labels"`
	Annotations     map[string]string `json:"annotations"`
	CrioAnnotations map[string]string `json:"crio_annotations"`
	LogPath         string            `json:"log_path"`
	Root            string            `json:"root"`
	Sandbox         string            `json:"sandbox"`
	IP              string            `json:"ip_address"`

	UIDMap []rspec.LinuxIDMapping `json:"uid_map,omitempty"`
	GIDMap []rspec.LinuxIDMapping `json:"gid_map,omitempty"`
}

// CrioInfo stores information about the crio daemon
type CrioInfo struct {
	StorageDriver string `json:"storage_driver"`
	StorageRoot   string `json:"storage_root"`
	CgroupDriver  string `json:"cgroup_driver"`
}
