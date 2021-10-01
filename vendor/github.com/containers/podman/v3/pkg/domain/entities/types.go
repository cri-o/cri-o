package entities

import (
	"net"

	buildahDefine "github.com/containers/buildah/define"
	"github.com/containers/podman/v3/libpod/define"
	"github.com/containers/podman/v3/libpod/events"
	"github.com/containers/podman/v3/libpod/network/types"
	"github.com/containers/podman/v3/pkg/specgen"
	"github.com/containers/storage/pkg/archive"
)

type Container struct {
	IDOrNamed
}

type Volume struct {
	Identifier
}

type Report struct {
	Id  []string //nolint
	Err map[string]error
}

type PodDeleteReport struct{ Report }

type VolumeDeleteOptions struct{}
type VolumeDeleteReport struct{ Report }

// NetOptions reflect the shared network options between
// pods and containers
type NetFlags struct {
	AddHosts     []string `json:"add-host,omitempty"`
	DNS          []string `json:"dns,omitempty"`
	DNSOpt       []string `json:"dns-opt,omitempty"`
	DNDSearch    []string `json:"dns-search,omitempty"`
	MacAddr      string   `json:"mac-address,omitempty"`
	Publish      []string `json:"publish,omitempty"`
	IP           string   `json:"ip,omitempty"`
	NoHosts      bool     `json:"no-hosts,omitempty"`
	Network      string   `json:"network,omitempty"`
	NetworkAlias []string `json:"network-alias,omitempty"`
}
type NetOptions struct {
	AddHosts           []string            `json:"hostadd,omitempty"`
	Aliases            []string            `json:"network_alias,omitempty"`
	CNINetworks        []string            `json:"cni_networks,omitempty"`
	UseImageResolvConf bool                `json:"no_manage_resolv_conf,omitempty"`
	DNSOptions         []string            `json:"dns_option,omitempty"`
	DNSSearch          []string            `json:"dns_search,omitempty"`
	DNSServers         []net.IP            `json:"dns_server,omitempty"`
	Network            specgen.Namespace   `json:"netns,omitempty"`
	NoHosts            bool                `json:"no_manage_hosts,omitempty"`
	PublishPorts       []types.PortMapping `json:"portmappings,omitempty"`
	StaticIP           *net.IP             `json:"static_ip,omitempty"`
	StaticMAC          *net.HardwareAddr   `json:"static_mac,omitempty"`
	// NetworkOptions are additional options for each network
	NetworkOptions map[string][]string `json:"network_options,omitempty"`
}

// All CLI inspect commands and inspect sub-commands use the same options
type InspectOptions struct {
	// Format - change the output to JSON or a Go template.
	Format string `json:",omitempty"`
	// Latest - inspect the latest container Podman is aware of.
	Latest bool `json:",omitempty"`
	// Size (containers only) - display total file size.
	Size bool `json:",omitempty"`
	// Type -- return JSON for specified type.
	Type string `json:",omitempty"`
	// All -- inspect all
	All bool `json:",omitempty"`
}

// All API and CLI diff commands and diff sub-commands use the same options
type DiffOptions struct {
	Format  string          `json:",omitempty"` // CLI only
	Latest  bool            `json:",omitempty"` // API and CLI, only supported by containers
	Archive bool            `json:",omitempty"` // CLI only
	Type    define.DiffType // Type which should be compared
}

// DiffReport provides changes for object
type DiffReport struct {
	Changes []archive.Change
}

type EventsOptions struct {
	FromStart bool
	EventChan chan *events.Event
	Filter    []string
	Stream    bool
	Since     string
	Until     string
}

// ContainerCreateResponse is the response struct for creating a container
type ContainerCreateResponse struct {
	// ID of the container created
	ID string `json:"Id"`
	// Warnings during container creation
	Warnings []string `json:"Warnings"`
}

// BuildOptions describe the options for building container images.
type BuildOptions struct {
	buildahDefine.BuildOptions
}

// BuildReport is the image-build report.
type BuildReport struct {
	// ID of the image.
	ID string
}
