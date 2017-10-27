package main

import (
	"fmt"

	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
	"strings"

	"github.com/docker/go-units"
	"github.com/kubernetes-incubator/cri-o/libpod"
	ann "github.com/kubernetes-incubator/cri-o/pkg/annotations"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"strconv"
)

type mountType string

// Type constants
const (
	// TypeBind is the type for mounting host dir
	TypeBind mountType = "bind"
	// TypeVolume is the type for remote storage volumes
	TypeVolume mountType = "volume"
	// TypeTmpfs is the type for mounting tmpfs
	TypeTmpfs mountType = "tmpfs"
	// TypeNamedPipe is the type for mounting Windows named pipes
	TypeNamedPipe mountType = "npipe"
)

var (
	defaultEnvVariables = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "TERM=xterm"}
)

type createResourceConfig struct {
	blkioDevice       []string // blkio-weight-device
	blkioWeight       uint16   // blkio-weight
	cpuPeriod         uint64   // cpu-period
	cpuQuota          int64    // cpu-quota
	cpuRtPeriod       uint64   // cpu-rt-period
	cpuRtRuntime      int64    // cpu-rt-runtime
	cpuShares         uint64   // cpu-shares
	cpus              string   // cpus
	cpusetCpus        string
	cpusetMems        string   // cpuset-mems
	deviceReadBps     []string // device-read-bps
	deviceReadIops    []string // device-read-iops
	deviceWriteBps    []string // device-write-bps
	deviceWriteIops   []string // device-write-iops
	disableOomKiller  bool     // oom-kill-disable
	kernelMemory      int64    // kernel-memory
	memory            int64    //memory
	memoryReservation int64    // memory-reservation
	memorySwap        int64    //memory-swap
	memorySwapiness   uint64   // memory-swappiness
	oomScoreAdj       int      //oom-score-adj
	pidsLimit         int64    // pids-limit
	shmSize           string
	ulimit            []string //ulimit
}

type createConfig struct {
	args           []string
	capAdd         []string // cap-add
	capDrop        []string // cap-drop
	cidFile        string
	cgroupParent   string // cgroup-parent
	command        []string
	detach         bool         // detach
	devices        []*pb.Device // device
	dnsOpt         []string     //dns-opt
	dnsSearch      []string     //dns-search
	dnsServers     []string     //dns
	entrypoint     string       //entrypoint
	env            []string     //env
	expose         []string     //expose
	groupAdd       []uint32     // group-add
	hostname       string       //hostname
	image          string
	interactive    bool              //interactive
	ip6Address     string            //ipv6
	ipAddress      string            //ip
	labels         map[string]string //label
	linkLocalIP    []string          // link-local-ip
	logDriver      string            // log-driver
	logDriverOpt   []string          // log-opt
	macAddress     string            //mac-address
	name           string            //name
	network        string            //network
	networkAlias   []string          //network-alias
	nsIPC          string            // ipc
	nsNet          string            //net
	nsPID          string            //pid
	nsUser         string
	pod            string   //pod
	privileged     bool     //privileged
	publish        []string //publish
	publishAll     bool     //publish-all
	readOnlyRootfs bool     //read-only
	resources      createResourceConfig
	rm             bool              //rm
	securityOpts   []string          //security-opt
	sigProxy       bool              //sig-proxy
	stopSignal     string            // stop-signal
	stopTimeout    int64             // stop-timeout
	storageOpts    []string          //storage-opt
	sysctl         map[string]string //sysctl
	tmpfs          []string          // tmpfs
	tty            bool              //tty
	user           uint32            //user
	group          uint32            // group
	volumes        []string          //volume
	volumesFrom    []string          //volumes-from
	workDir        string            //workdir
}

var createDescription = "Creates a new container from the given image or" +
	" storage and prepares it for running the specified command. The" +
	" container ID is then printed to stdout. You can then start it at" +
	" any time with the kpod start <container_id> command. The container" +
	" will be created with the initial state 'created'."

var createCommand = cli.Command{
	Name:        "create",
	Usage:       "create but do not start a container",
	Description: createDescription,
	Flags:       createFlags,
	Action:      createCmd,
	ArgsUsage:   "IMAGE [COMMAND [ARG...]]",
}

func createCmd(c *cli.Context) error {
	// TODO should allow user to create based off a directory on the host not just image
	// Need CLI support for this
	if err := validateFlags(c, createFlags); err != nil {
		return err
	}
	//runtime, err := getRuntime(c)
	runtime, err := libpod.NewRuntime()
	if err != nil {
		return errors.Wrapf(err, "error creating libpod runtime")
	}

	createConfig, err := parseCreateOpts(c, runtime)
	if err != nil {
		return err
	}

	// Deal with the image after all the args have been checked
	createImage := runtime.NewImage(createConfig.image)
	if !createImage.HasImageLocal() {
		// The image wasnt found by the user input'd name or its fqname
		// Pull the image
		fmt.Printf("Trying to pull %s...", createImage.PullName)
		createImage.Pull()
	}

	runtimeSpec, err := createConfigToOCISpec(createConfig)
	if err != nil {
		return err
	}

	imageName, err := createImage.GetFQName()
	if err != nil {
		return err
	}
	fmt.Println(imageName)
	imageID, err := createImage.GetImageID()
	if err != nil {
		return err
	}
	ctr, err := runtime.NewContainer(runtimeSpec, libpod.WithRootFSFromImage(imageID, imageName, false))
	if err != nil {
		return err
	}

	if err := ctr.Create(); err != nil {
		return err
	}

	if c.String("cid-file") != "" {
		libpod.WriteFile(ctr.ID(), c.String("cid-file"))
		return nil
	}
	fmt.Printf("%s\n", ctr.ID())

	return nil
}

/* The following funcs should land in parse.go */
//
//
func stringSlicetoUint32Slice(inputSlice []string) ([]uint32, error) {
	var outputSlice []uint32
	for _, v := range inputSlice {
		u, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return outputSlice, err
		}
		outputSlice = append(outputSlice, uint32(u))
	}
	return outputSlice, nil
}

// Parses CLI options related to container creation into a config which can be
// parsed into an OCI runtime spec
func parseCreateOpts(c *cli.Context, runtime *libpod.Runtime) (*createConfig, error) {
	var command []string
	var memoryLimit, memoryReservation, memorySwap, memoryKernel int64
	var blkioWeight uint16
	var env []string
	var labelValues []string
	var uid, gid uint32
	sysctl := make(map[string]string)
	labels := make(map[string]string)

	image := c.Args()[0]

	if len(c.Args()) < 1 {
		return nil, errors.Errorf("you just provide an image name")
	}
	if len(c.Args()) > 1 {
		command = c.Args()[1:]
	}

	// LABEL VARIABLES
	// TODO where should labels be verified to be x=y format
	labelValues, labelErr := readKVStrings(c.StringSlice("label-file"), c.StringSlice("label"))
	if labelErr != nil {
		return &createConfig{}, errors.Wrapf(labelErr, "unable to process labels from --label and label-file")
	}
	// Process KEY=VALUE stringslice in string map for WithLabels func
	if len(labelValues) > 0 {
		for _, i := range labelValues {
			spliti := strings.Split(i, "=")
			labels[spliti[0]] = spliti[1]
		}
	}

	// ENVIRONMENT VARIABLES
	// TODO where should env variables be verified to be x=y format
	env, err := readKVStrings(c.StringSlice("env-file"), c.StringSlice("env"))
	if err != nil {
		return &createConfig{}, errors.Wrapf(err, "unable to process variables from --env and --env-file")
	}
	// Add default environment variables if nothing defined
	if len(env) == 0 {
		env = append(env, defaultEnvVariables...)
	}

	if len(c.StringSlice("sysctl")) > 0 {
		for _, inputSysctl := range c.StringSlice("sysctl") {
			values := strings.Split(inputSysctl, "=")
			sysctl[values[0]] = values[1]
		}
	}

	groupAdd, err := stringSlicetoUint32Slice(c.StringSlice("group-add"))
	if err != nil {
		return &createConfig{}, errors.Wrapf(err, "invalid value for groups provided")
	}

	if c.String("user") != "" {
		// TODO
		// We need to mount the imagefs and get the uid/gid
		// For now, user zeros
		uid = 0
		gid = 0
	}

	if c.String("memory") != "" {
		memoryLimit, err = units.RAMInBytes(c.String("memory"))
		if err != nil {
			return nil, errors.Wrapf(err, "invalid value for memory")
		}
	}
	if c.String("memory-reservation") != "" {
		memoryReservation, err = units.RAMInBytes(c.String("memory-reservation"))
		if err != nil {
			return nil, errors.Wrapf(err, "invalid value for memory-reservation")
		}
	}
	if c.String("memory-swap") != "" {
		memorySwap, err = units.RAMInBytes(c.String("memory-swap"))
		if err != nil {
			return nil, errors.Wrapf(err, "invalid value for memory-swap")
		}
	}
	if c.String("kernel-memory") != "" {
		memoryKernel, err = units.RAMInBytes(c.String("kernel-memory"))
		if err != nil {
			return nil, errors.Wrapf(err, "invalid value for kernel-memory")
		}
	}
	if c.String("blkio-weight") != "" {
		u, err := strconv.ParseUint(c.String("blkio-weight"), 10, 16)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid value for blkio-weight")
		}
		blkioWeight = uint16(u)
	}

	config := &createConfig{
		capAdd:         c.StringSlice("cap-add"),
		capDrop:        c.StringSlice("cap-drop"),
		cgroupParent:   c.String("cgroup-parent"),
		command:        command,
		detach:         c.Bool("detach"),
		dnsOpt:         c.StringSlice("dns-opt"),
		dnsSearch:      c.StringSlice("dns-search"),
		dnsServers:     c.StringSlice("dns"),
		entrypoint:     c.String("entrypoint"),
		env:            env,
		expose:         c.StringSlice("env"),
		groupAdd:       groupAdd,
		hostname:       c.String("hostname"),
		image:          image,
		interactive:    c.Bool("interactive"),
		ip6Address:     c.String("ipv6"),
		ipAddress:      c.String("ip"),
		labels:         labels,
		linkLocalIP:    c.StringSlice("link-local-ip"),
		logDriver:      c.String("log-driver"),
		logDriverOpt:   c.StringSlice("log-opt"),
		macAddress:     c.String("mac-address"),
		name:           c.String("name"),
		network:        c.String("network"),
		networkAlias:   c.StringSlice("network-alias"),
		nsIPC:          c.String("ipc"),
		nsNet:          c.String("net"),
		nsPID:          c.String("pid"),
		pod:            c.String("pod"),
		privileged:     c.Bool("privileged"),
		publish:        c.StringSlice("publish"),
		publishAll:     c.Bool("publish-all"),
		readOnlyRootfs: c.Bool("read-only"),
		resources: createResourceConfig{
			blkioWeight: blkioWeight,
			blkioDevice: c.StringSlice("blkio-weight-device"),
			cpuShares:   c.Uint64("cpu-shares"),
			//cpuCount:          c.Int64("cpu-count"),
			cpuPeriod:         c.Uint64("cpu-period"),
			cpusetCpus:        c.String("cpu-period"),
			cpusetMems:        c.String("cpuset-mems"),
			cpuQuota:          c.Int64("cpu-quota"),
			cpuRtPeriod:       c.Uint64("cpu-rt-period"),
			cpuRtRuntime:      c.Int64("cpu-rt-runtime"),
			cpus:              c.String("cpus"),
			deviceReadBps:     c.StringSlice("device-read-bps"),
			deviceReadIops:    c.StringSlice("device-read-iops"),
			deviceWriteBps:    c.StringSlice("device-write-bps"),
			deviceWriteIops:   c.StringSlice("device-write-iops"),
			disableOomKiller:  c.Bool("oom-kill-disable"),
			memory:            memoryLimit,
			memoryReservation: memoryReservation,
			memorySwap:        memorySwap,
			memorySwapiness:   c.Uint64("memory-swapiness"),
			kernelMemory:      memoryKernel,
			oomScoreAdj:       c.Int("oom-score-adj"),

			pidsLimit: c.Int64("pids-limit"),
			ulimit:    c.StringSlice("ulimit"),
		},
		rm:           c.Bool("rm"),
		securityOpts: c.StringSlice("security-opt"),
		//shmSize: c.String("shm-size"),
		sigProxy:    c.Bool("sig-proxy"),
		stopSignal:  c.String("stop-signal"),
		stopTimeout: c.Int64("stop-timeout"),
		storageOpts: c.StringSlice("storage-opt"),
		sysctl:      sysctl,
		tmpfs:       c.StringSlice("tmpfs"),
		tty:         c.Bool("tty"), //
		user:        uid,
		group:       gid,
		//userns: c.String("userns"),
		volumes:     c.StringSlice("volume"),
		volumesFrom: c.StringSlice("volumes-from"),
		workDir:     c.String("workdir"),
	}

	return config, nil
}

// Parses information needed to create a container into an OCI runtime spec
func createConfigToOCISpec(config *createConfig) (*spec.Spec, error) {

	//blkio, err := config.CreateBlockIO()
	//if err != nil {
	//	return &spec.Spec{}, err
	//}

	spec := config.GetDefaultLinuxSpec()
	spec.Process.Cwd = config.workDir
	spec.Process.Args = config.command

	if config.tty {
		spec.Process.Terminal = config.tty
	}

	if config.user != 0 {
		// User and Group must go together
		spec.Process.User.UID = config.user
		spec.Process.User.GID = config.group
	}
	if len(config.groupAdd) > 0 {
		spec.Process.User.AdditionalGids = config.groupAdd
	}
	if len(config.env) > 0 {
		spec.Process.Env = config.env
	}
	//TODO
	// Need examples of capacity additions so I can load that properly

	if config.readOnlyRootfs {
		spec.Root.Readonly = config.readOnlyRootfs
	}

	if config.hostname != "" {
		spec.Hostname = config.hostname
	}

	// BIND MOUNTS
	if len(config.volumes) > 0 {
		spec.Mounts = append(spec.Mounts, config.GetVolumeMounts()...)
	}
	// TMPFS MOUNTS
	if len(config.tmpfs) > 0 {
		spec.Mounts = append(spec.Mounts, config.GetTmpfsMounts()...)
	}

	// RESOURCES - MEMORY
	if len(config.sysctl) > 0 {
		spec.Linux.Sysctl = config.sysctl
	}
	if config.resources.memory != 0 {
		spec.Linux.Resources.Memory.Limit = &config.resources.memory
	}
	if config.resources.memoryReservation != 0 {
		spec.Linux.Resources.Memory.Reservation = &config.resources.memoryReservation
	}
	if config.resources.memorySwap != 0 {
		spec.Linux.Resources.Memory.Swap = &config.resources.memorySwap
	}
	if config.resources.kernelMemory != 0 {
		spec.Linux.Resources.Memory.Kernel = &config.resources.kernelMemory
	}
	if config.resources.memorySwapiness != 0 {
		spec.Linux.Resources.Memory.Swappiness = &config.resources.memorySwapiness
	}
	if config.resources.disableOomKiller {
		spec.Linux.Resources.Memory.DisableOOMKiller = &config.resources.disableOomKiller
	}

	// RESOURCES - CPU

	if config.resources.cpuShares != 0 {
		spec.Linux.Resources.CPU.Shares = &config.resources.cpuShares
	}
	if config.resources.cpuQuota != 0 {
		spec.Linux.Resources.CPU.Quota = &config.resources.cpuQuota
	}
	if config.resources.cpuPeriod != 0 {
		spec.Linux.Resources.CPU.Period = &config.resources.cpuPeriod
	}
	if config.resources.cpuRtRuntime != 0 {
		spec.Linux.Resources.CPU.RealtimeRuntime = &config.resources.cpuRtRuntime
	}
	if config.resources.cpuRtPeriod != 0 {
		spec.Linux.Resources.CPU.RealtimePeriod = &config.resources.cpuRtPeriod
	}
	if config.resources.cpus != "" {
		spec.Linux.Resources.CPU.Cpus = config.resources.cpus
	}
	if config.resources.cpusetMems != "" {
		spec.Linux.Resources.CPU.Mems = config.resources.cpusetMems
	}

	// RESOURCES - PIDS
	if config.resources.pidsLimit != 0 {
		spec.Linux.Resources.Pids.Limit = config.resources.pidsLimit
	}

	/*
				Capabilities: &spec.LinuxCapabilities{
				// Rlimits []PosixRlimit // Where does this come from
				// Type string
				// Hard uint64
				// Limit uint64
				// NoNewPrivileges bool // No user input for this
				// ApparmorProfile string // No user input for this
				OOMScoreAdj: &config.resources.oomScoreAdj,
				// Selinuxlabel
			},
			Hooks: &spec.Hooks{},
			//Annotations
				Resources: &spec.LinuxResources{
					Devices: config.GetDefaultDevices(),
					BlockIO: &blkio,
					//HugepageLimits:
					Network: &spec.LinuxNetwork{
					// ClassID *uint32
					// Priorites []LinuxInterfacePriority
					},
				},
				//CgroupsPath:
				//Namespaces: []LinuxNamespace
				//Devices
				Seccomp: &spec.LinuxSeccomp{
				// DefaultAction:
				// Architectures
				// Syscalls:
				},
				// RootfsPropagation
				// MaskedPaths
				// ReadonlyPaths:
				// MountLabel
				// IntelRdt
			},
		}
	*/
	return &spec, nil
}

func getStatFromPath(path string) unix.Stat_t {
	s := unix.Stat_t{}
	_ = unix.Stat(path, &s)
	return s
}

func makeThrottleArray(throttleInput []string) ([]spec.LinuxThrottleDevice, error) {
	var ltds []spec.LinuxThrottleDevice
	for _, i := range throttleInput {
		t, err := validateBpsDevice(i)
		if err != nil {
			return []spec.LinuxThrottleDevice{}, err
		}
		ltd := spec.LinuxThrottleDevice{}
		ltd.Rate = t.rate
		ltdStat := getStatFromPath(t.path)
		ltd.Major = int64(unix.Major(ltdStat.Rdev))
		ltd.Minor = int64(unix.Major(ltdStat.Rdev))
		ltds = append(ltds, ltd)
	}
	return ltds, nil

}

func (c *createConfig) CreateBlockIO() (spec.LinuxBlockIO, error) {
	bio := spec.LinuxBlockIO{}
	bio.Weight = &c.resources.blkioWeight
	if len(c.resources.blkioDevice) > 0 {
		var lwds []spec.LinuxWeightDevice
		for _, i := range c.resources.blkioDevice {
			wd, err := validateweightDevice(i)
			if err != nil {
				return bio, errors.Wrapf(err, "invalid values for blkio-weight-device")
			}
			wdStat := getStatFromPath(wd.path)
			lwd := spec.LinuxWeightDevice{
				Weight: &wd.weight,
			}
			lwd.Major = int64(unix.Major(wdStat.Rdev))
			lwd.Minor = int64(unix.Minor(wdStat.Rdev))
			lwds = append(lwds, lwd)
		}
	}
	if len(c.resources.deviceReadBps) > 0 {
		readBps, err := makeThrottleArray(c.resources.deviceReadBps)
		if err != nil {
			return bio, err
		}
		bio.ThrottleReadBpsDevice = readBps
	}
	if len(c.resources.deviceWriteBps) > 0 {
		writeBpds, err := makeThrottleArray(c.resources.deviceWriteBps)
		if err != nil {
			return bio, err
		}
		bio.ThrottleWriteBpsDevice = writeBpds
	}
	if len(c.resources.deviceReadIops) > 0 {
		readIops, err := makeThrottleArray(c.resources.deviceReadIops)
		if err != nil {
			return bio, err
		}
		bio.ThrottleReadIOPSDevice = readIops
	}
	if len(c.resources.deviceWriteIops) > 0 {
		writeIops, err := makeThrottleArray(c.resources.deviceWriteIops)
		if err != nil {
			return bio, err
		}
		bio.ThrottleWriteIOPSDevice = writeIops
	}

	return bio, nil
}

func (c *createConfig) GetDefaultMounts() []spec.Mount {
	return []spec.Mount{
		{
			Destination: "/proc",
			Type:        "proc",
			Source:      "proc",
			Options:     []string{"nosuid", "noexec", "nodev"},
		},
		{
			Destination: "/dev",
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options:     []string{"nosuid", "strictatime", "mode=755", "size=65536k"},
		},
		{
			Destination: "/dev/pts",
			Type:        "devpts",
			Source:      "devpts",
			Options:     []string{"nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620", "gid=5"},
		},
		{
			Destination: "/sys",
			Type:        "sysfs",
			Source:      "sysfs",
			Options:     []string{"nosuid", "noexec", "nodev", "ro"},
		},
		{
			Destination: "/sys/fs/cgroup",
			Type:        "cgroup",
			Source:      "cgroup",
			Options:     []string{"ro", "nosuid", "noexec", "nodev"},
		},
		{
			Destination: "/dev/mqueue",
			Type:        "mqueue",
			Source:      "mqueue",
			Options:     []string{"nosuid", "noexec", "nodev"},
		},
		{
			Destination: "/dev/shm",
			Type:        "tmpfs",
			Source:      "shm",
			Options:     []string{"nosuid", "noexec", "nodev", "mode=1777"},
		},
	}
}
func iPtr(i int64) *int64 { return &i }

func (c *createConfig) GetDefaultDevices() []spec.LinuxDeviceCgroup {
	return []spec.LinuxDeviceCgroup{
		{
			Allow:  false,
			Access: "rwm",
		},
		{
			Allow:  true,
			Type:   "c",
			Major:  iPtr(1),
			Minor:  iPtr(5),
			Access: "rwm",
		},
		{
			Allow:  true,
			Type:   "c",
			Major:  iPtr(1),
			Minor:  iPtr(3),
			Access: "rwm",
		},
		{
			Allow:  true,
			Type:   "c",
			Major:  iPtr(1),
			Minor:  iPtr(9),
			Access: "rwm",
		},
		{
			Allow:  true,
			Type:   "c",
			Major:  iPtr(1),
			Minor:  iPtr(8),
			Access: "rwm",
		},
		{
			Allow:  true,
			Type:   "c",
			Major:  iPtr(5),
			Minor:  iPtr(0),
			Access: "rwm",
		},
		{
			Allow:  true,
			Type:   "c",
			Major:  iPtr(5),
			Minor:  iPtr(1),
			Access: "rwm",
		},
		{
			Allow:  false,
			Type:   "c",
			Major:  iPtr(10),
			Minor:  iPtr(229),
			Access: "rwm",
		},
	}
}

func defaultCapabilities() []string {
	return []string{
		"CAP_CHOWN",
		"CAP_DAC_OVERRIDE",
		"CAP_FSETID",
		"CAP_FOWNER",
		"CAP_MKNOD",
		"CAP_NET_RAW",
		"CAP_SETGID",
		"CAP_SETUID",
		"CAP_SETFCAP",
		"CAP_SETPCAP",
		"CAP_NET_BIND_SERVICE",
		"CAP_SYS_CHROOT",
		"CAP_KILL",
		"CAP_AUDIT_WRITE",
	}
}

func (c *createConfig) GetDefaultLinuxSpec() spec.Spec {
	s := spec.Spec{
		Version: spec.Version,
		Root:    &spec.Root{},
	}
	s.Annotations = c.GetAnnotations()
	s.Mounts = c.GetDefaultMounts()
	s.Process = &spec.Process{
		Capabilities: &spec.LinuxCapabilities{
			Bounding:    defaultCapabilities(),
			Permitted:   defaultCapabilities(),
			Inheritable: defaultCapabilities(),
			Effective:   defaultCapabilities(),
		},
	}
	s.Linux = &spec.Linux{
		MaskedPaths: []string{
			"/proc/kcore",
			"/proc/latency_stats",
			"/proc/timer_list",
			"/proc/timer_stats",
			"/proc/sched_debug",
		},
		ReadonlyPaths: []string{
			"/proc/asound",
			"/proc/bus",
			"/proc/fs",
			"/proc/irq",
			"/proc/sys",
			"/proc/sysrq-trigger",
		},
		Namespaces: []spec.LinuxNamespace{
			{Type: "mount"},
			{Type: "network"},
			{Type: "uts"},
			{Type: "pid"},
			{Type: "ipc"},
		},
		Devices: []spec.LinuxDevice{},
		Resources: &spec.LinuxResources{
			Devices: c.GetDefaultDevices(),
		},
	}

	return s
}

func (c *createConfig) GetAnnotations() map[string]string {
	a := getDefaultAnnotations()
	// TODO
	// Which annotations do we want added by default
	if c.tty {
		a["io.kubernetes.cri-o.TTY"] = "true"
	}
	return a
}

func getDefaultAnnotations() map[string]string {
	var a map[string]string
	a = make(map[string]string)
	a[ann.Annotations] = ""
	a[ann.ContainerID] = ""
	a[ann.ContainerName] = ""
	a[ann.ContainerType] = ""
	a[ann.Created] = ""
	a[ann.HostName] = ""
	a[ann.IP] = ""
	a[ann.Image] = ""
	a[ann.ImageName] = ""
	a[ann.ImageRef] = ""
	a[ann.KubeName] = ""
	a[ann.Labels] = ""
	a[ann.LogPath] = ""
	a[ann.Metadata] = ""
	a[ann.Name] = ""
	a[ann.PrivilegedRuntime] = ""
	a[ann.ResolvPath] = ""
	a[ann.HostnamePath] = ""
	a[ann.SandboxID] = ""
	a[ann.SandboxName] = ""
	a[ann.ShmPath] = ""
	a[ann.MountPoint] = ""
	a[ann.TrustedSandbox] = ""
	a[ann.TTY] = "false"
	a[ann.Stdin] = ""
	a[ann.StdinOnce] = ""
	a[ann.Volumes] = ""

	return a
}

//GetTmpfsMounts takes user provided input for bind mounts and creates Mount structs
func (c *createConfig) GetVolumeMounts() []spec.Mount {
	var m []spec.Mount
	var options []string
	for _, i := range c.volumes {
		spliti := strings.Split(i, ":")
		if len(spliti) > 2 {
			options = strings.Split(spliti[2], ",")
		}
		// always add rbind bc mount ignores the bind filesystem when mounting
		options = append(options, "rbind")
		m = append(m, spec.Mount{
			Destination: spliti[1],
			Type:        string(TypeBind),
			Source:      spliti[0],
			Options:     options,
		})
	}
	return m
}

//GetTmpfsMounts takes user provided input for tmpfs mounts and creates Mount structs
func (c *createConfig) GetTmpfsMounts() []spec.Mount {
	var m []spec.Mount
	for _, i := range c.tmpfs {
		// Default options if nothing passed
		options := []string{"rw", "noexec", "nosuid", "nodev", "size=65536k"}
		spliti := strings.Split(i, ":")
		destPath := spliti[0]
		if len(spliti) > 1 {
			options = strings.Split(spliti[1], ",")
		}
		m = append(m, spec.Mount{
			Destination: destPath,
			Type:        string(TypeTmpfs),
			Options:     options,
		})
	}
	return m
}

func (c *createConfig) GetContainerCreateOptions(cli *cli.Context) ([]libpod.CtrCreateOption, error) {
	/*
		WithStorageConfig
		WithImageConfig
		WithSignaturePolicy
		WithOCIRuntime
		WithConmonPath
		WithConmonEnv
		WithCgroupManager
		WithStaticDir
		WithTmpDir
		WithSELinux
		WithPidsLimit // dont need
		WithMaxLogSize
		WithNoPivotRoot
		WithRootFSFromPath
		WithRootFSFromImage
		WithStdin // done
		WithSharedNamespaces
		WithLabels //done
		WithAnnotations // dont need
		WithName // done
		WithStopSignal
		WithPodName
	*/
	var options []libpod.CtrCreateOption

	// Uncomment after talking to mheon about unimplemented funcs
	// options = append(options, libpod.WithLabels(c.labels))

	if c.interactive {
		options = append(options, libpod.WithStdin())
	}
	if c.name != "" {
		logrus.Info("appending name %s", c.name)
		options = append(options, libpod.WithName(c.name))
	}

	return options, nil
}
