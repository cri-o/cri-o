package main

import (
	"fmt"

	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
)

// TODO: Missing flags from docker-create - particularly --security-opt or equiv and resource limit flags
// TODO stop using Kubernetes API structs here - replace with our own versions (remove pb import entirely)
// TODO Add missing flags from docker-create
// TODO parse flags into a createConfig, and parse createConfig into an OCI runtime spec

// TODO These temporary values should be replaced with sane defaults
const (
	defaultHostname     = "kpod-launch"
	defaultCgroupParent = "/kpod-launch"
)

type createConfig struct {
	image            string
	command          string
	args             []string
	pod              string
	privileged       bool
	rm               bool
	hostNet          bool
	hostPID          bool
	hostIPC          bool
	name             string
	labels           map[string]string
	workDir          string
	env              map[string]string
	detach           bool
	stdin            bool
	tty              bool
	devices          []*pb.Device
	mounts           []*pb.Mount
	capAdd           []string
	capDrop          []string
	dnsServers       []string
	dnsSearch        []string
	dnsOpt           []string
	ports            []*pb.PortMapping
	hostname         string
	cgroupParent     string
	sysctl           string
	user             int64
	additionalGroups []int64
	readOnlyRootfs   bool
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
