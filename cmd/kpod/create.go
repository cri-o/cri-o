package main

import (
	"fmt"

	"strings"

	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
)

type createResourceConfig struct {
	blkioWeight       int64    // blkio-weight
	blkioDevice       []string // blkio-weight
	cpuShares         int64    // cpu-shares
	cpuCount          int64    // cpu-count
	cpuPeriod         int64    // cpu-period
	cpusetCpus        string
	cpusetNames       string
	cpuFile           string
	cpuMems           string   // cpuset-mems
	cpuQuota          int64    // cpu-quota
	cpuRtPeriod       int64    // cpu-rt-period
	cpuRtRuntime      int64    // cpu-rt-runtime
	cpus              int64    // cpus
	deviceReadBps     []string // device-read-bps
	deviceReadIops    []string // device-read-iops
	deviceWriteBps    []string // device-write-bps
	deviceWriteIops   []string // device-write-iops
	memory            string   //memory
	memoryReservation string   // memory-reservation
	memorySwap        string   //memory-swap
	memorySwapiness   string   // memory-swappiness
	kernelMemory      string   // kernel-memory
	oomScoreAdj       string   //oom-score-adj
	pidsLimit         string   // pids-limit
	shmSize           string
	ulimit            []string //ulimit
}

type createConfig struct {
	additionalGroups []int64
	args             []string
	capAdd           []string // cap-add
	capDrop          []string // cap-drop
	cgroupParent     string   // cgroup-parent
	command          []string
	detach           bool         // detach
	devices          []*pb.Device // device
	dnsOpt           []string     //dns-opt
	dnsSearch        []string     //dns-search
	dnsServers       []string     //dns
	entrypoint       string       //entrypoint
	env              []string     //env
	expose           []string     //expose
	groupAdd         []string     // group-add
	hostname         string       //hostname
	image            string
	interactive      bool              //interactive
	ip6Address       string            //ipv6
	ipAddress        string            //ip
	labels           map[string]string //label
	linkLocalIP      []string          // link-local-ip
	logDriver        string            // log-driver
	logDriverOpt     []string          // log-opt
	macAddress       string            //mac-address
	mounts           []*pb.Mount
	name             string   //name
	network          string   //network
	networkAlias     []string //network-alias
	nsIPC            string   // ipc
	nsNet            string   //net
	nsPID            string   //pid
	nsUser           string
	pod              string //pod
	ports            []*pb.PortMapping
	privileged       bool     //privileged
	publish          []string //publish
	publishAll       bool     //publish-all
	readOnlyRootfs   bool     //read-only
	resources        createResourceConfig
	rm               bool     //rm
	securityOpts     []string //security-opt
	shmSize          string   //shm-size
	sigProxy         bool     //sig-proxy
	stdin            bool
	stopSignal       string            // stop-signal
	stopTimeout      int64             // stop-timeout
	storageOpts      []string          //storage-opt
	sysctl           map[string]string //sysctl
	tmpfs            []string          // tmpfs
	tty              bool              //tty
	user             int64             //user
	userns           string            //userns
	volumes          []string          //volume
	volumesFrom      []string          //volumes-from
	workDir          string            //workdir
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

func verifyImage(image string) bool {

	return false
}

func createCmd(c *cli.Context) error {
	// TODO should allow user to create based off a directory on the host not just image
	// Need CLI support for this
	if len(c.Args()) != 1 {
		return errors.Errorf("must specify name of image to create from")
	}
	if err := validateFlags(c, createFlags); err != nil {
		return err
	}
	runtime, err := getRuntime(c)
	if err != nil {
		return errors.Wrapf(err, "error creating libpod runtime")
	}

	createConfig, err := parseCreateOpts(c)
	if err != nil {
		return err
	}

	runtimeSpec, err := createConfigToOCISpec(createConfig)
	if err != nil {
		return err
	}

	ctr, err := runtime.NewContainer(runtimeSpec)
	if err != nil {
		return err
	}

	// Should we also call ctr.Create() to make the container in runc?

	fmt.Printf("%s\n", ctr.ID())

	return nil
}

// Parses CLI options related to container creation into a config which can be
// parsed into an OCI runtime spec
func parseCreateOpts(c *cli.Context) (*createConfig, error) {
	//for _, i := range(c.FlagNames()) {
	//	fmt.Println(i)
	//}
	var command []string
	var env []string
	sysctl := make(map[string]string)
	labels := make(map[string]string)

	if len(c.Args()) < 1 {
		return nil, errors.Errorf("you just provide an image name")
	}
	if len(c.Args()) > 1 {
		command = c.Args()[1:]
	}

	if len(c.StringSlice("env")) > 0 {
		for _, inputEnv := range c.StringSlice("env") {
			env = append(env, inputEnv)
		}
	} else {
		env = append(env, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "TERM=xterm")
	}

	if len(c.StringSlice("sysctl")) > 0 {
		for _, inputSysctl := range c.StringSlice("sysctl") {
			values := strings.Split(inputSysctl, "=")
			sysctl[values[0]] = values[1]
		}
	}

	// TODO HERE
	// insert labels - baude

	image := c.Args()[0]

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
		groupAdd:       c.StringSlice("group-add"),
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
			blkioWeight:       c.Int64("blkio-weight"),
			blkioDevice:       c.StringSlice("blkio-device"),
			cpuShares:         c.Int64("cpu-shares"),
			cpuCount:          c.Int64("cpu-count"),
			cpuPeriod:         c.Int64("cpu-period"),
			cpusetCpus:        c.String("cpu-period"),
			cpuMems:           c.String("cpuset-mems"),
			cpuQuota:          c.Int64("cpu-quota"),
			cpuRtPeriod:       c.Int64("cpu-rt-period"),
			cpuRtRuntime:      c.Int64("cpu-rt-runtime"),
			cpus:              c.Int64("cpus"),
			deviceReadBps:     c.StringSlice("device-read-bps"),
			deviceReadIops:    c.StringSlice("device-read-iops"),
			deviceWriteBps:    c.StringSlice("device-write-bps"),
			deviceWriteIops:   c.StringSlice("device-write-iops"),
			memory:            c.String("memory"),
			memoryReservation: c.String("memory-reservation"),
			memorySwap:        c.String("memory-swap"),
			memorySwapiness:   c.String("memory-swapiness"),
			kernelMemory:      c.String("kernel-memory"),
			oomScoreAdj:       c.String("oom-score-adj"),
			pidsLimit:         c.String("pids-limit"),
			ulimit:            c.StringSlice("ulimit"),
		},
		rm:           c.Bool("rm"),
		securityOpts: c.StringSlice("security-opt"),
		shmSize:      c.String("shm-size"),
		sigProxy:     c.Bool("sig-proxy"),
		stopSignal:   c.String("stop-signal"),
		stopTimeout:  c.Int64("stop-timeout"),
		storageOpts:  c.StringSlice("storage-opt"),
		sysctl:       sysctl,
		tmpfs:        c.StringSlice("tmpfs"),
		tty:          c.Bool("tty"), //
		user:         c.Int64("user"),
		userns:       c.String("userns"),
		volumes:      c.StringSlice("volume"),
		volumesFrom:  c.StringSlice("volumes-from"),
		workDir:      c.String("workdir"),
	}

	return config, nil
}

// Parses information needed to create a container into an OCI runtime spec
func createConfigToOCISpec(config *createConfig) (*spec.Spec, error) {
	spec := &spec.Spec{
		Version: "1.0.0.0", // where do I get this?
		Process: &spec.Process{
			Terminal:     config.tty,
			User:         spec.User{},
			Args:         config.command,
			Env:          config.env,
			Capabilities: &spec.LinuxCapabilities{},
		},
		Root: &spec.Root{
			Readonly: config.readOnlyRootfs,
		},
		// Hostname
		// Mounts
		Hooks: &spec.Hooks{},
		//Annotations
		Linux: &spec.Linux{
			// UIDMappings
			// GIDMappings
			Sysctl:    config.sysctl,
			Resources: &spec.LinuxResources{
			// Devices: spec.LinuxDeviceCgroup,

			},
		},
	}

	return spec, errors.Errorf("NOT IMPLEMENTED")
}
