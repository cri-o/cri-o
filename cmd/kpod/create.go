package main

import (
	"fmt"

	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
)

type createResourceConfig struct {
	blkioWeight       int64
	blkioDevice       []string
	cpuShares         int64
	cpuCount          int64
	cpuPeriod         int64
	cpusetCpus        string
	cpusetNames       string
	cpuFile           string
	cpuMems           string
	cpuQuota          int64
	cpuRtPeriod       int64
	cpuRtRuntime      int64
	cpus              int64
	deviceReadBps     []string
	deviceReadIops    []string
	deviceWriteBps    []string
	deviceWriteIops   []string
	memory            string
	memoryReservation string
	memorySwap        string
	memorySwapiness   string
	kernelMemory      string
	oomScoreAdj       string
	pidsLimit         string
	shmSize           string
	ulimit            []string
}

type createConfig struct {
	additionalGroups []int64
	args             []string
	capAdd           []string
	capDrop          []string
	cgroupParent     string
	command          string
	detach           bool
	devices          []*pb.Device
	dnsOpt           []string
	dnsSearch        []string
	dnsServers       []string
	entrypoint       string
	env              map[string]string
	expose           []string
	groupAdd         []string
	hostname         string
	image            string
	interactive      bool
	ip6Address       string
	ipAddress        string
	labels           map[string]string
	linkLocalIP      []string
	logDriver        string
	logDriverOpt     []string
	macAddress       string
	mounts           []*pb.Mount
	name             string
	network          string
	networkAlias     []string
	nsIPC            string
	nsNet            string
	nsPID            string
	nsUser           string
	pod              string
	ports            []*pb.PortMapping
	privileged       bool
	publish          []string
	publishAll       bool
	readOnlyRootfs   bool
	resources        createResourceConfig
	rm               bool
	securityOpts     []string
	shmSize          string
	sigProxy         bool
	stdin            bool
	stopSignal       string
	stopTimeout      int64
	storageOpts      []string
	sysctl           string
	tmpfs            []string
	tty              bool
	user             int64
	userns           string
	volumes          []string
	volumesFrom      []string
	workDir          string
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
	return nil, errors.Errorf("NOT IMPLEMENTED")
}

// Parses information needed to create a container into an OCI runtime spec
func createConfigToOCISpec(config *createConfig) (*spec.Spec, error) {
	return nil, errors.Errorf("NOT IMPLEMENTED")
}
