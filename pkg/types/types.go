package types

import (
	"fmt"
	"time"

	"github.com/containers/storage/pkg/idtools"
)

type Output interface {
	MarshalText() string
}

// ContainerInfo stores information about containers
type ContainerInfo struct {
	Name            string            `json:"name" yaml:"name"`
	Pid             int               `json:"pid" yaml:"pid"`
	Image           string            `json:"image" yaml:"image"`
	ImageRef        string            `json:"image_ref" yaml:"imageRef"`
	CreatedTime     int64             `json:"created_time" yaml:"createdTime"`
	Labels          map[string]string `json:"labels" yaml:"labels"`
	Annotations     map[string]string `json:"annotations" yaml:"annotations"`
	CrioAnnotations map[string]string `json:"crio_annotations" yaml:"crioAnnotations"`
	LogPath         string            `json:"log_path" yaml:"logPath"`
	Root            string            `json:"root" yaml:"root"`
	Sandbox         string            `json:"sandbox" yaml:"sandbox"`
	IPs             []string          `json:"ip_addresses" yaml:"ipAddresses"`
}

// Simple "interface" to marshal these objects to human-readable text rather than to a format like json/yaml.
func (info *ContainerInfo) MarshalText() string {
	var str string

	str += fmt.Sprintf("Information for container: %s\n\nPID: %d\nImage: %s\nImageRef: %s\nCreation Timestamp: %v\nLog Path: %s\nRoot: %s\nSandbox: %s\n",
		info.Name,
		info.Pid,
		info.Image,
		info.ImageRef,
		time.Unix(info.CreatedTime, 0), // Give a timestamp that can be more easily parsed by humans.
		info.LogPath,
		info.Root,
		info.Sandbox,
	)

	if info.Labels != nil {
		str += "Labels: \n"

		for key, value := range info.Labels {
			str += fmt.Sprintf("- %s : %s\n", key, value)
		}
	}

	if info.Annotations != nil {
		str += "Annotations: \n"

		for key, value := range info.Annotations {
			str += fmt.Sprintf("- %s : %s\n", key, value)
		}
	}

	if info.CrioAnnotations != nil {
		str += "Crio Annotations: \n"

		for key, value := range info.CrioAnnotations {
			str += fmt.Sprintf("- %s : %s\n", key, value)
		}
	}

	if info.IPs != nil {
		str += "IP Addresses: \n"

		for _, ip := range info.IPs {
			str += fmt.Sprintf("- %s\n", ip)
		}
	}

	return str
}

// IDMappings specifies the ID mappings used for containers.
type IDMappings struct {
	Uids []idtools.IDMap `json:"uids" yaml:"uids"`
	Gids []idtools.IDMap `json:"gids" yaml:"gids"`
}

// Simple "interface" to marshal these objects to human-readable text rather than to a format like json/yaml.
func (mappings *IDMappings) MarshalText() string {
	var str string

	if mappings.Uids != nil {
		str += "Default UID Mappings (format <container>:<host>:<size>): \n"

		for _, m := range mappings.Uids {
			str += fmt.Sprintf("- %d.%d.%d\n", m.ContainerID, m.HostID, m.Size)
		}
	}

	if mappings.Gids != nil {
		str += "Default GID Mappings (format <container>:<host>:<size>): \n"

		for _, m := range mappings.Gids {
			str += fmt.Sprintf("- %d.%d.%d\n", m.ContainerID, m.HostID, m.Size)
		}
	}

	return str
}

// CrioInfo stores information about the crio daemon
type CrioInfo struct {
	StorageDriver     string     `json:"storage_driver" yaml:"storageDriver"`
	StorageRoot       string     `json:"storage_root" yaml:"storageRoot"`
	CgroupDriver      string     `json:"cgroup_driver" yaml:"cgroupDriver"`
	DefaultIDMappings IDMappings `json:"default_id_mappings" yaml:"defaultIdMappings"`
}

// Simple "interface" to marshal these objects to human-readable text rather than to a format like json/yaml.
func (crio *CrioInfo) MarshalText() string {
	var str string

	str += fmt.Sprintf("General Crio Information: \nStorage Driver: %s\nStorage Root: %s\nCGroup Driver: %s\n",
		crio.StorageDriver,
		crio.StorageRoot,
		crio.CgroupDriver,
	)

	str += crio.DefaultIDMappings.MarshalText()

	return str
}
