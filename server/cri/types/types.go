package server

type VersionRequest struct {
	Version string
}

type VersionResponse struct {
	Version           string
	RuntimeName       string
	RuntimeVersion    string
	RuntimeAPIVersion string
}

type RunPodSandboxRequest struct {
	Config         *PodSandboxConfig
	RuntimeHandler string
}

type RunPodSandboxResponse struct {
	PodSandboxID string
}

type StopPodSandboxRequest struct {
	PodSandboxID string
}

type RemovePodSandboxRequest struct {
	PodSandboxID string
}

type PodSandboxStatusRequest struct {
	PodSandboxID string
	Verbose      bool
}

type PodSandboxStatusResponse struct {
	Status *PodSandboxStatus
	Info   map[string]string
}

type ListPodSandboxRequest struct {
	Filter *PodSandboxFilter
}

type ListPodSandboxResponse struct {
	Items []*PodSandbox
}

type CreateContainerRequest struct {
	PodSandboxID  string
	Config        *ContainerConfig
	SandboxConfig *PodSandboxConfig
}

type CreateContainerResponse struct {
	ContainerID string
}

type StartContainerRequest struct {
	ContainerID string
}

type StartContainerResponse struct {
}

type StopContainerRequest struct {
	ContainerID string
	Timeout     int64
}

type StopContainerResponse struct {
}

type RemoveContainerRequest struct {
	ContainerID string
}

type ListContainersRequest struct {
	Filter *ContainerFilter
}

type ListContainersResponse struct {
	Containers []*Container
}

type ContainerStatusRequest struct {
	ContainerID string
	Verbose     bool
}

type ContainerStatusResponse struct {
	Status *ContainerStatus
	Info   map[string]string
}

type UpdateContainerResourcesRequest struct {
	ContainerID string
	Linux       *LinuxContainerResources
}

type ExecSyncRequest struct {
	ContainerID string
	Cmd         []string
	Timeout     int64
}

type ExecSyncResponse struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int32
}

type ExecRequest struct {
	ContainerID string
	Cmd         []string
	Tty         bool
	Stdin       bool
	Stdout      bool
	Stderr      bool
}

type ExecResponse struct {
	URL string
}

type AttachRequest struct {
	ContainerID string
	Stdin       bool
	Tty         bool
	Stdout      bool
	Stderr      bool
}

type AttachResponse struct {
	URL string
}

type PortForwardRequest struct {
	PodSandboxID string
	Port         []int32
}

type PortForwardResponse struct {
	URL string
}

type ListImagesRequest struct {
	Filter *ImageFilter
}

type ListImagesResponse struct {
	Images []*Image
}

type ImageStatusRequest struct {
	Image   *ImageSpec
	Verbose bool
}

type ImageStatusResponse struct {
	Image *Image
	Info  map[string]string
}

type PullImageRequest struct {
	Image         *ImageSpec
	Auth          *AuthConfig
	SandboxConfig *PodSandboxConfig
}

type PullImageResponse struct {
	ImageRef string
}

type RemoveImageRequest struct {
	Image *ImageSpec
}

type UpdateRuntimeConfigRequest struct {
	RuntimeConfig *RuntimeConfig
}

type StatusRequest struct {
	Verbose bool
}

type StatusResponse struct {
	Status *RuntimeStatus
	Info   map[string]string
}

type ImageFsInfoResponse struct {
	ImageFilesystems []*FilesystemUsage
}

type ContainerStatsRequest struct {
	ContainerID string
}

type ContainerStatsResponse struct {
	Stats *ContainerStats
}

type ListContainerStatsRequest struct {
	Filter *ContainerStatsFilter
}

type ListContainerStatsResponse struct {
	Stats []*ContainerStats
}

type ReopenContainerLogRequest struct {
	ContainerID string
}

type ImageFilter struct {
	Image *ImageSpec
}

type Image struct {
	ID          string
	RepoTags    []string
	RepoDigests []string
	Size        uint64
	UID         *Int64Value
	Username    string
	Spec        *ImageSpec
}

type ImageSpec struct {
	Image       string
	Annotations map[string]string
}

type AuthConfig struct {
	Username      string
	Password      string
	Auth          string
	ServerAddress string
	IDentityToken string
	RegistryToken string
}

type PodSandboxConfig struct {
	Metadata     *PodSandboxMetadata
	Hostname     string
	LogDirectory string
	DNSConfig    *DNSConfig
	PortMappings []*PortMapping
	Labels       map[string]string
	Annotations  map[string]string
	Linux        *LinuxPodSandboxConfig
}

type PodSandboxMetadata struct {
	Name      string
	UID       string
	Namespace string
	Attempt   uint32
}

type DNSConfig struct {
	Servers  []string
	Searches []string
	Options  []string
}

type PortMapping struct {
	Protocol      Protocol
	ContainerPort int32
	HostPort      int32
	HostIP        string
}

type Protocol int32

type LinuxPodSandboxConfig struct {
	CgroupParent    string
	SecurityContext *LinuxSandboxSecurityContext
	Sysctls         map[string]string
}

type LinuxSandboxSecurityContext struct {
	NamespaceOptions   *NamespaceOption
	SelinuxOptions     *SELinuxOption
	RunAsUser          *Int64Value
	RunAsGroup         *Int64Value
	SeccompProfilePath string
	SupplementalGroups []int64
	ReadonlyRootfs     bool
	Privileged         bool
}

type NamespaceOption struct {
	Network  NamespaceMode
	Pid      NamespaceMode
	Ipc      NamespaceMode
	TargetID string
}

type NamespaceMode int32

type SELinuxOption struct {
	User  string
	Role  string
	Type  string
	Level string
}

type Int64Value struct {
	Value int64
}

type FilesystemUsage struct {
	Timestamp  int64
	FsID       *FilesystemIDentifier
	UsedBytes  *UInt64Value
	InodesUsed *UInt64Value
}

type FilesystemIDentifier struct {
	Mountpoint string
}

type UInt64Value struct {
	Value uint64
}

type PodSandboxStatus struct {
	ID             string
	Metadata       *PodSandboxMetadata
	State          PodSandboxState
	CreatedAt      int64
	Network        *PodSandboxNetworkStatus
	Linux          *LinuxPodSandboxStatus
	Labels         map[string]string
	Annotations    map[string]string
	RuntimeHandler string
}

type PodSandboxState int32

type PodSandboxNetworkStatus struct {
	IP            string
	AdditionalIps []*PodIP
}

type PodIP struct {
	IP string
}

type LinuxPodSandboxStatus struct {
	Namespaces *Namespace
}

type Namespace struct {
	Options *NamespaceOption
}

type PodSandboxFilter struct {
	ID            string
	State         *PodSandboxStateValue
	LabelSelector map[string]string
}

type PodSandboxStateValue struct {
	State PodSandboxState
}

type PodSandbox struct {
	ID             string
	Metadata       *PodSandboxMetadata
	State          PodSandboxState
	CreatedAt      int64
	Labels         map[string]string
	Annotations    map[string]string
	RuntimeHandler string
}

type ContainerConfig struct {
	Metadata    *ContainerMetadata
	Image       *ImageSpec
	Command     []string
	Args        []string
	WorkingDir  string
	Envs        []*KeyValue
	Mounts      []*Mount
	Devices     []*Device
	Labels      map[string]string
	Annotations map[string]string
	LogPath     string
	Stdin       bool
	StdinOnce   bool
	Tty         bool
	Linux       *LinuxContainerConfig
}

type ContainerMetadata struct {
	Name    string
	Attempt uint32
}

type KeyValue struct {
	Key   string
	Value string
}

type Mount struct {
	ContainerPath  string
	HostPath       string
	Readonly       bool
	SelinuxRelabel bool
	Propagation    MountPropagation
}

type MountPropagation int32

type Device struct {
	ContainerPath string
	HostPath      string
	Permissions   string
}

type LinuxContainerConfig struct {
	Resources       *LinuxContainerResources
	SecurityContext *LinuxContainerSecurityContext
}

type LinuxContainerResources struct {
	CPUPeriod          int64
	CPUQuota           int64
	CPUShares          int64
	MemoryLimitInBytes int64
	OomScoreAdj        int64
	CPUsetCPUs         string
	CPUsetMems         string
	HugepageLimits     []*HugepageLimit
}

type HugepageLimit struct {
	PageSize string
	Limit    uint64
}

type LinuxContainerSecurityContext struct {
	Capabilities       *Capability
	NamespaceOptions   *NamespaceOption
	SelinuxOptions     *SELinuxOption
	RunAsUser          *Int64Value
	RunAsGroup         *Int64Value
	RunAsUsername      string
	ApparmorProfile    string
	SeccompProfilePath string
	MaskedPaths        []string
	ReadonlyPaths      []string
	SupplementalGroups []int64
	Privileged         bool
	ReadonlyRootfs     bool
	NoNewPrivs         bool
}

type Capability struct {
	AddCapabilities  []string
	DropCapabilities []string
}

type ContainerFilter struct {
	ID            string
	State         *ContainerStateValue
	PodSandboxID  string
	LabelSelector map[string]string
}

type ContainerStateValue struct {
	State ContainerState
}

type ContainerState int32

type Container struct {
	ID           string
	PodSandboxID string
	Metadata     *ContainerMetadata
	Image        *ImageSpec
	ImageRef     string
	State        ContainerState
	CreatedAt    int64
	Labels       map[string]string
	Annotations  map[string]string
}

type ContainerStatus struct {
	ID          string
	ImageRef    string
	Reason      string
	Message     string
	LogPath     string
	Metadata    *ContainerMetadata
	Image       *ImageSpec
	Labels      map[string]string
	Annotations map[string]string
	Mounts      []*Mount
	CreatedAt   int64
	StartedAt   int64
	FinishedAt  int64
	State       ContainerState
	ExitCode    int32
}

type RuntimeConfig struct {
	NetworkConfig *NetworkConfig
}

type NetworkConfig struct {
	PodCidr string
}

type RuntimeStatus struct {
	Conditions []*RuntimeCondition
}

type RuntimeCondition struct {
	Type    string
	Status  bool
	Reason  string
	Message string
}

type ContainerStats struct {
	Attributes    *ContainerAttributes
	CPU           *CPUUsage
	Memory        *MemoryUsage
	WritableLayer *FilesystemUsage
}

type ContainerAttributes struct {
	ID          string
	Metadata    *ContainerMetadata
	Labels      map[string]string
	Annotations map[string]string
}

type CPUUsage struct {
	Timestamp            int64
	UsageCoreNanoSeconds *UInt64Value
}

type MemoryUsage struct {
	Timestamp       int64
	WorkingSetBytes *UInt64Value
}

type ContainerStatsFilter struct {
	ID            string
	PodSandboxID  string
	LabelSelector map[string]string
}
