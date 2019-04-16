package createconfig

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/containers/image/manifest"
	"github.com/containers/libpod/libpod"
	"github.com/containers/libpod/pkg/namespaces"
	"github.com/containers/storage"
	"github.com/containers/storage/pkg/stringid"
	"github.com/cri-o/ocicni/pkg/ocicni"
	"github.com/docker/go-connections/nat"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// Type constants
const (
	bps = iota
	iops
	// TypeBind is the type for mounting host dir
	TypeBind = "bind"
	// TypeVolume is the type for remote storage volumes
	// TypeVolume = "volume"  // re-enable upon use
	// TypeTmpfs is the type for mounting tmpfs
	TypeTmpfs = "tmpfs"
)

// CreateResourceConfig represents resource elements in CreateConfig
// structures
type CreateResourceConfig struct {
	BlkioWeight       uint16   // blkio-weight
	BlkioWeightDevice []string // blkio-weight-device
	CPUPeriod         uint64   // cpu-period
	CPUQuota          int64    // cpu-quota
	CPURtPeriod       uint64   // cpu-rt-period
	CPURtRuntime      int64    // cpu-rt-runtime
	CPUShares         uint64   // cpu-shares
	CPUs              float64  // cpus
	CPUsetCPUs        string
	CPUsetMems        string   // cpuset-mems
	DeviceReadBps     []string // device-read-bps
	DeviceReadIOps    []string // device-read-iops
	DeviceWriteBps    []string // device-write-bps
	DeviceWriteIOps   []string // device-write-iops
	DisableOomKiller  bool     // oom-kill-disable
	KernelMemory      int64    // kernel-memory
	Memory            int64    //memory
	MemoryReservation int64    // memory-reservation
	MemorySwap        int64    //memory-swap
	MemorySwappiness  int      // memory-swappiness
	OomScoreAdj       int      //oom-score-adj
	PidsLimit         int64    // pids-limit
	ShmSize           int64
	Ulimit            []string //ulimit
}

// CreateConfig is a pre OCI spec structure.  It represents user input from varlink or the CLI
type CreateConfig struct {
	Runtime            *libpod.Runtime
	Annotations        map[string]string
	Args               []string
	CapAdd             []string // cap-add
	CapDrop            []string // cap-drop
	CidFile            string
	ConmonPidFile      string
	CgroupParent       string // cgroup-parent
	Command            []string
	Detach             bool              // detach
	Devices            []string          // device
	DNSOpt             []string          //dns-opt
	DNSSearch          []string          //dns-search
	DNSServers         []string          //dns
	Entrypoint         []string          //entrypoint
	Env                map[string]string //env
	ExposedPorts       map[nat.Port]struct{}
	GroupAdd           []string // group-add
	HealthCheck        *manifest.Schema2HealthConfig
	NoHosts            bool
	HostAdd            []string //add-host
	Hostname           string   //hostname
	Image              string
	ImageID            string
	BuiltinImgVolumes  map[string]struct{} // volumes defined in the image config
	IDMappings         *storage.IDMappingOptions
	ImageVolumeType    string                 // how to handle the image volume, either bind, tmpfs, or ignore
	Interactive        bool                   //interactive
	IpcMode            namespaces.IpcMode     //ipc
	IP6Address         string                 //ipv6
	IPAddress          string                 //ip
	Labels             map[string]string      //label
	LinkLocalIP        []string               // link-local-ip
	LogDriver          string                 // log-driver
	LogDriverOpt       []string               // log-opt
	MacAddress         string                 //mac-address
	Name               string                 //name
	NetMode            namespaces.NetworkMode //net
	Network            string                 //network
	NetworkAlias       []string               //network-alias
	PidMode            namespaces.PidMode     //pid
	Pod                string                 //pod
	PortBindings       nat.PortMap
	Privileged         bool     //privileged
	Publish            []string //publish
	PublishAll         bool     //publish-all
	Quiet              bool     //quiet
	ReadOnlyRootfs     bool     //read-only
	Resources          CreateResourceConfig
	Rm                 bool              //rm
	StopSignal         syscall.Signal    // stop-signal
	StopTimeout        uint              // stop-timeout
	Sysctl             map[string]string //sysctl
	Systemd            bool
	Tmpfs              []string              // tmpfs
	Tty                bool                  //tty
	UsernsMode         namespaces.UsernsMode //userns
	User               string                //user
	UtsMode            namespaces.UTSMode    //uts
	Mounts             []spec.Mount          //mounts
	Volumes            []string              //volume
	VolumesFrom        []string
	NamedVolumes       []*libpod.ContainerNamedVolume // Filled in by CreateConfigToOCISpec
	WorkDir            string                         //workdir
	LabelOpts          []string                       //SecurityOpts
	NoNewPrivs         bool                           //SecurityOpts
	ApparmorProfile    string                         //SecurityOpts
	SeccompProfilePath string                         //SecurityOpts
	SecurityOpts       []string
	Rootfs             string
	Syslog             bool // Whether to enable syslog on exit commands
}

func u32Ptr(i int64) *uint32     { u := uint32(i); return &u }
func fmPtr(i int64) *os.FileMode { fm := os.FileMode(i); return &fm }

// CreateBlockIO returns a LinuxBlockIO struct from a CreateConfig
func (c *CreateConfig) CreateBlockIO() (*spec.LinuxBlockIO, error) {
	return c.createBlockIO()
}

// AddContainerInitBinary adds the init binary specified by path iff the
// container will run in a private PID namespace that is not shared with the
// host or another pre-existing container, where an init-like process is
// already running.
//
// Note that AddContainerInitBinary prepends "/dev/init" "--" to the command
// to execute the bind-mounted binary as PID 1.
func (c *CreateConfig) AddContainerInitBinary(path string) error {
	if path == "" {
		return fmt.Errorf("please specify a path to the container-init binary")
	}
	if !c.PidMode.IsPrivate() {
		return fmt.Errorf("cannot add init binary as PID 1 (PID namespace isn't private)")
	}
	if c.Systemd {
		return fmt.Errorf("cannot use container-init binary with systemd")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return errors.Wrap(err, "container-init binary not found on the host")
	}
	c.Command = append([]string{"/dev/init", "--"}, c.Command...)
	c.Mounts = append(c.Mounts, spec.Mount{
		Destination: "/dev/init",
		Type:        TypeBind,
		Source:      path,
		Options:     []string{TypeBind, "ro"},
	})
	return nil
}

func processOptions(options []string) []string {
	var (
		foundrw, foundro bool
		rootProp         string
	)
	options = append(options, "rbind")
	for _, opt := range options {
		switch opt {
		case "rw":
			foundrw = true
		case "ro":
			foundro = true
		case "private", "rprivate", "slave", "rslave", "shared", "rshared":
			rootProp = opt
		}
	}
	if !foundrw && !foundro {
		options = append(options, "rw")
	}
	if rootProp == "" {
		options = append(options, "rprivate")
	}
	return options
}

func (c *CreateConfig) initFSMounts() []spec.Mount {
	var mounts []spec.Mount
	for _, m := range c.Mounts {
		m.Options = processOptions(m.Options)
		if m.Type == "tmpfs" {
			m.Options = append(m.Options, "tmpcopyup")
		} else {
			mounts = append(mounts, m)
		}
	}
	return mounts
}

// GetVolumeMounts takes user provided input for bind mounts and creates Mount structs
func (c *CreateConfig) GetVolumeMounts(specMounts []spec.Mount) ([]spec.Mount, error) {
	m := []spec.Mount{}
	for _, i := range c.Volumes {
		var options []string
		spliti := strings.Split(i, ":")
		if len(spliti) > 2 {
			options = strings.Split(spliti[2], ",")
		}

		m = append(m, spec.Mount{
			Destination: spliti[1],
			Type:        string(TypeBind),
			Source:      spliti[0],
			Options:     processOptions(options),
		})

		logrus.Debugf("User mount %s:%s options %v", spliti[0], spliti[1], options)
	}

	if c.ImageVolumeType == "ignore" {
		return m, nil
	}

	for vol := range c.BuiltinImgVolumes {
		if libpod.MountExists(specMounts, vol) || libpod.MountExists(m, vol) {
			continue
		}

		mount := spec.Mount{
			Destination: vol,
			Type:        c.ImageVolumeType,
			Options:     []string{"rprivate", "rw", "nodev"},
		}
		if c.ImageVolumeType == "tmpfs" {
			mount.Source = "tmpfs"
			mount.Options = append(mount.Options, "tmpcopyup")
		} else {
			// TODO: Move support for this and tmpfs into libpod
			// Should tmpfs also be handled as named volumes? Wouldn't be hard
			// This will cause a new local Volume to be created on your system
			mount.Source = stringid.GenerateNonCryptoID()
			mount.Options = append(mount.Options, TypeBind)
		}
		m = append(m, mount)
	}

	return m, nil
}

// GetVolumesFrom reads the create-config artifact of the container to get volumes from
// and adds it to c.Volumes of the current container.
func (c *CreateConfig) GetVolumesFrom() error {
	if os.Geteuid() != 0 {
		return nil
	}

	for _, vol := range c.VolumesFrom {
		options := ""
		splitVol := strings.SplitN(vol, ":", 2)
		if len(splitVol) == 2 {
			options = splitVol[1]
		}
		ctr, err := c.Runtime.LookupContainer(splitVol[0])
		if err != nil {
			return errors.Wrapf(err, "error looking up container %q", splitVol[0])
		}

		logrus.Debugf("Adding volumes from container %s", ctr.ID())

		// Look up the container's user volumes. This gets us the
		// destinations of all mounts the user added to the container.
		userVolumesArr := ctr.UserVolumes()

		// We're going to need to access them a lot, so convert to a map
		// to reduce looping.
		// We'll also use the map to indicate if we missed any volumes along the way.
		userVolumes := make(map[string]bool)
		for _, dest := range userVolumesArr {
			userVolumes[dest] = false
		}

		// Now we get the container's spec and loop through its volumes
		// and append them in if we can find them.
		spec := ctr.Spec()
		if spec == nil {
			return errors.Errorf("error retrieving container %s spec", ctr.ID())
		}
		for _, mnt := range spec.Mounts {
			if mnt.Type != TypeBind {
				continue
			}
			if _, exists := userVolumes[mnt.Destination]; exists {
				userVolumes[mnt.Destination] = true
				localOptions := options
				if localOptions == "" {
					localOptions = strings.Join(mnt.Options, ",")
				}
				c.Volumes = append(c.Volumes, fmt.Sprintf("%s:%s:%s", mnt.Source, mnt.Destination, localOptions))
			}
		}

		// We're done with the spec mounts. Add named volumes.
		// Add these unconditionally - none of them are automatically
		// part of the container, as some spec mounts are.
		namedVolumes := ctr.NamedVolumes()
		for _, namedVol := range namedVolumes {
			if _, exists := userVolumes[namedVol.Dest]; exists {
				userVolumes[namedVol.Dest] = true
			}
			localOptions := options
			if localOptions == "" {
				localOptions = strings.Join(namedVol.Options, ",")
			}
			c.Volumes = append(c.Volumes, fmt.Sprintf("%s:%s:%s", namedVol.Name, namedVol.Dest, localOptions))
		}

		// Check if we missed any volumes
		for volDest, found := range userVolumes {
			if !found {
				logrus.Warnf("Unable to match volume %s from container %s for volumes-from", volDest, ctr.ID())
			}
		}
	}
	return nil
}

//GetTmpfsMounts takes user provided input for Tmpfs mounts and creates Mount structs
func (c *CreateConfig) GetTmpfsMounts() []spec.Mount {
	var m []spec.Mount
	for _, i := range c.Tmpfs {
		// Default options if nothing passed
		options := []string{"rprivate", "rw", "noexec", "nosuid", "nodev", "size=65536k"}
		spliti := strings.Split(i, ":")
		destPath := spliti[0]
		if len(spliti) > 1 {
			options = strings.Split(spliti[1], ",")
		}
		m = append(m, spec.Mount{
			Destination: destPath,
			Type:        string(TypeTmpfs),
			Options:     options,
			Source:      string(TypeTmpfs),
		})
	}
	return m
}

func (c *CreateConfig) createExitCommand() ([]string, error) {
	config, err := c.Runtime.GetConfig()
	if err != nil {
		return nil, err
	}

	cmd, _ := os.Executable()
	command := []string{cmd,
		"--root", config.StorageConfig.GraphRoot,
		"--runroot", config.StorageConfig.RunRoot,
		"--log-level", logrus.GetLevel().String(),
		"--cgroup-manager", config.CgroupManager,
		"--tmpdir", config.TmpDir,
	}
	if config.OCIRuntime != "" {
		command = append(command, []string{"--runtime", config.OCIRuntime}...)
	}
	if config.StorageConfig.GraphDriverName != "" {
		command = append(command, []string{"--storage-driver", config.StorageConfig.GraphDriverName}...)
	}
	if c.Syslog {
		command = append(command, "--syslog", "true")
	}
	command = append(command, []string{"container", "cleanup"}...)

	if c.Rm {
		command = append(command, "--rm")
	}

	return command, nil
}

// GetContainerCreateOptions takes a CreateConfig and returns a slice of CtrCreateOptions
func (c *CreateConfig) GetContainerCreateOptions(runtime *libpod.Runtime, pod *libpod.Pod) ([]libpod.CtrCreateOption, error) {
	var options []libpod.CtrCreateOption
	var portBindings []ocicni.PortMapping
	var err error

	if c.Interactive {
		options = append(options, libpod.WithStdin())
	}
	if c.Systemd && (strings.HasSuffix(c.Command[0], "init") ||
		strings.HasSuffix(c.Command[0], "systemd")) {
		options = append(options, libpod.WithSystemd())
	}
	if c.Name != "" {
		logrus.Debugf("appending name %s", c.Name)
		options = append(options, libpod.WithName(c.Name))
	}
	if c.Pod != "" || pod != nil {
		if pod == nil {
			pod, err = runtime.LookupPod(c.Pod)
			if err != nil {
				return nil, errors.Wrapf(err, "unable to add container to pod %s", c.Pod)
			}
		}
		logrus.Debugf("adding container to pod %s", c.Pod)
		options = append(options, runtime.WithPod(pod))
	}
	if len(c.PortBindings) > 0 {
		portBindings, err = c.CreatePortBindings()
		if err != nil {
			return nil, errors.Wrapf(err, "unable to create port bindings")
		}
	}

	if len(c.Volumes) != 0 {
		// Volumes consist of multiple, comma-delineated fields
		// The image spec only includes one part of that, so drop the
		// others, if they are included
		volumes := make([]string, 0, len(c.Volumes))
		for _, vol := range c.Volumes {
			// We always want the volume destination
			splitVol := strings.SplitN(vol, ":", 3)
			if len(splitVol) > 1 {
				volumes = append(volumes, splitVol[1])
			} else {
				volumes = append(volumes, splitVol[0])
			}
		}

		options = append(options, libpod.WithUserVolumes(volumes))
	}

	if len(c.NamedVolumes) != 0 {
		options = append(options, libpod.WithNamedVolumes(c.NamedVolumes))
	}

	if len(c.Command) != 0 {
		options = append(options, libpod.WithCommand(c.Command))
	}

	// Add entrypoint unconditionally
	// If it's empty it's because it was explicitly set to "" or the image
	// does not have one
	options = append(options, libpod.WithEntrypoint(c.Entrypoint))

	networks := make([]string, 0)
	userNetworks := c.NetMode.UserDefined()
	if IsPod(userNetworks) {
		userNetworks = ""
	}
	if userNetworks != "" {
		for _, netName := range strings.Split(userNetworks, ",") {
			if netName == "" {
				return nil, errors.Wrapf(err, "container networks %q invalid", networks)
			}
			networks = append(networks, netName)
		}
	}

	if c.NetMode.IsNS() {
		ns := c.NetMode.NS()
		if ns == "" {
			return nil, errors.Errorf("invalid empty user-defined network namespace")
		}
		_, err := os.Stat(ns)
		if err != nil {
			return nil, err
		}
	} else if c.NetMode.IsContainer() {
		connectedCtr, err := c.Runtime.LookupContainer(c.NetMode.Container())
		if err != nil {
			return nil, errors.Wrapf(err, "container %q not found", c.NetMode.Container())
		}
		options = append(options, libpod.WithNetNSFrom(connectedCtr))
	} else if !c.NetMode.IsHost() && !c.NetMode.IsNone() {
		postConfigureNetNS := c.NetMode.IsSlirp4netns() || (len(c.IDMappings.UIDMap) > 0 || len(c.IDMappings.GIDMap) > 0) && !c.UsernsMode.IsHost()
		options = append(options, libpod.WithNetNS(portBindings, postConfigureNetNS, string(c.NetMode), networks))
	}

	if c.PidMode.IsContainer() {
		connectedCtr, err := c.Runtime.LookupContainer(c.PidMode.Container())
		if err != nil {
			return nil, errors.Wrapf(err, "container %q not found", c.PidMode.Container())
		}

		options = append(options, libpod.WithPIDNSFrom(connectedCtr))
	}

	if c.IpcMode.IsContainer() {
		connectedCtr, err := c.Runtime.LookupContainer(c.IpcMode.Container())
		if err != nil {
			return nil, errors.Wrapf(err, "container %q not found", c.IpcMode.Container())
		}

		options = append(options, libpod.WithIPCNSFrom(connectedCtr))
	}

	if IsPod(string(c.UtsMode)) {
		options = append(options, libpod.WithUTSNSFromPod(pod))
	}
	if c.UtsMode.IsContainer() {
		connectedCtr, err := c.Runtime.LookupContainer(c.UtsMode.Container())
		if err != nil {
			return nil, errors.Wrapf(err, "container %q not found", c.UtsMode.Container())
		}

		options = append(options, libpod.WithUTSNSFrom(connectedCtr))
	}

	// TODO: MNT, USER, CGROUP
	options = append(options, libpod.WithStopSignal(c.StopSignal))
	options = append(options, libpod.WithStopTimeout(c.StopTimeout))
	if len(c.DNSSearch) > 0 {
		options = append(options, libpod.WithDNSSearch(c.DNSSearch))
	}
	if len(c.DNSServers) > 0 {
		if len(c.DNSServers) == 1 && strings.ToLower(c.DNSServers[0]) == "none" {
			options = append(options, libpod.WithUseImageResolvConf())
		} else {
			options = append(options, libpod.WithDNS(c.DNSServers))
		}
	}
	if len(c.DNSOpt) > 0 {
		options = append(options, libpod.WithDNSOption(c.DNSOpt))
	}
	if c.NoHosts {
		options = append(options, libpod.WithUseImageHosts())
	}
	if len(c.HostAdd) > 0 && !c.NoHosts {
		options = append(options, libpod.WithHosts(c.HostAdd))
	}
	logPath := getLoggingPath(c.LogDriverOpt)
	if logPath != "" {
		options = append(options, libpod.WithLogPath(logPath))
	}
	if c.IPAddress != "" {
		ip := net.ParseIP(c.IPAddress)
		if ip == nil {
			return nil, errors.Wrapf(libpod.ErrInvalidArg, "cannot parse %s as IP address", c.IPAddress)
		} else if ip.To4() == nil {
			return nil, errors.Wrapf(libpod.ErrInvalidArg, "%s is not an IPv4 address", c.IPAddress)
		}
		options = append(options, libpod.WithStaticIP(ip))
	}

	options = append(options, libpod.WithPrivileged(c.Privileged))

	useImageVolumes := c.ImageVolumeType == TypeBind
	// Gather up the options for NewContainer which consist of With... funcs
	options = append(options, libpod.WithRootFSFromImage(c.ImageID, c.Image, useImageVolumes))
	options = append(options, libpod.WithSecLabels(c.LabelOpts))
	options = append(options, libpod.WithConmonPidFile(c.ConmonPidFile))
	options = append(options, libpod.WithLabels(c.Labels))
	options = append(options, libpod.WithUser(c.User))
	if c.IpcMode.IsHost() {
		options = append(options, libpod.WithShmDir("/dev/shm"))

	} else if c.IpcMode.IsContainer() {
		ctr, err := runtime.LookupContainer(c.IpcMode.Container())
		if err != nil {
			return nil, errors.Wrapf(err, "container %q not found", c.IpcMode.Container())
		}
		options = append(options, libpod.WithShmDir(ctr.ShmDir()))
	}
	options = append(options, libpod.WithShmSize(c.Resources.ShmSize))
	options = append(options, libpod.WithGroups(c.GroupAdd))
	options = append(options, libpod.WithIDMappings(*c.IDMappings))
	if c.Rootfs != "" {
		options = append(options, libpod.WithRootFS(c.Rootfs))
	}
	// Default used if not overridden on command line

	if c.CgroupParent != "" {
		options = append(options, libpod.WithCgroupParent(c.CgroupParent))
	}

	// Always use a cleanup process to clean up Podman after termination
	exitCmd, err := c.createExitCommand()
	if err != nil {
		return nil, err
	}
	options = append(options, libpod.WithExitCommand(exitCmd))

	if c.HealthCheck != nil {
		options = append(options, libpod.WithHealthCheck(c.HealthCheck))
		logrus.Debugf("New container has a health check")
	}
	return options, nil
}

// CreatePortBindings iterates ports mappings and exposed ports into a format CNI understands
func (c *CreateConfig) CreatePortBindings() ([]ocicni.PortMapping, error) {
	return NatToOCIPortBindings(c.PortBindings)
}

// NatToOCIPortBindings iterates a nat.portmap slice and creates []ocicni portmapping slice
func NatToOCIPortBindings(ports nat.PortMap) ([]ocicni.PortMapping, error) {
	var portBindings []ocicni.PortMapping
	for containerPb, hostPb := range ports {
		var pm ocicni.PortMapping
		pm.ContainerPort = int32(containerPb.Int())
		for _, i := range hostPb {
			var hostPort int
			var err error
			pm.HostIP = i.HostIP
			if i.HostPort == "" {
				hostPort = containerPb.Int()
			} else {
				hostPort, err = strconv.Atoi(i.HostPort)
				if err != nil {
					return nil, errors.Wrapf(err, "unable to convert host port to integer")
				}
			}

			pm.HostPort = int32(hostPort)
			pm.Protocol = containerPb.Proto()
			portBindings = append(portBindings, pm)
		}
	}
	return portBindings, nil
}

// AddPrivilegedDevices iterates through host devices and adds all
// host devices to the spec
func (c *CreateConfig) AddPrivilegedDevices(g *generate.Generator) error {
	return c.addPrivilegedDevices(g)
}

func getStatFromPath(path string) (unix.Stat_t, error) {
	s := unix.Stat_t{}
	err := unix.Stat(path, &s)
	return s, err
}
