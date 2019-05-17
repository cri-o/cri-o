// Copyright 2019 Kinvolk GmbH

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os/exec"
	_ "path/filepath"
	"syscall"
	"time"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	fclient "github.com/firecracker-microvm/firecracker-go-sdk/client"
	fcmodels "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	fcops "github.com/firecracker-microvm/firecracker-go-sdk/client/operations"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	// RuntimeTypeFC is the type representing the RuntimeFC implementation.
	RuntimeTypeFC = "fc"

	fcTimeout         = 10
	defaultConfigPath = "/etc/crio/firecracker-crio.json"

	supportedMountFSType = "ext4"
	rootDevice           = "root=/dev/vda1"
)

type FcConfig struct {
	FirecrackerBinaryPath string `json:"firecracker_binary_path"`
	SocketPath            string `json:"socket_path"`
	KernelImagePath       string `json:"kernel_image_path"`
	KernelArgs            string `json:"kernel_args"`
	RootDrive             string `json:"root_drive"`
}

func LoadConfig(path string) (*FcConfig, error) {
	if path == "" {
		path = defaultConfigPath
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg FcConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// runtimeFC is the Runtime interface implementation that is more appropriate
// for FC based container runtimes.
type runtimeFC struct {
	ctx context.Context

	path string

	firecrackerd *exec.Cmd            // firecrackerd process
	fcClient     *fclient.Firecracker // the current active connection

	machine    *firecracker.Machine
	machineCID uint32
	config     *FcConfig
}

// newRuntimeFC creates a new runtimeFC instance
func newRuntimeFC(path string) RuntimeImpl {
	config, err := LoadConfig(defaultConfigPath)
	if err != nil {
		return nil
	}

	return &runtimeFC{
		path:   path,
		ctx:    context.Background(),
		config: config,
	}
}

// Version returns the version of the OCI Runtime
func (r *runtimeFC) Version() (string, error) {
	return "vm", nil
}

func (r *runtimeFC) newFireClient() *fclient.Firecracker {
	httpClient := fclient.NewHTTPClient(strfmt.NewFormats())

	socketTransport := &http.Transport{
		DialContext: func(ctx context.Context, network, path string) (net.Conn, error) {
			addr, err := net.ResolveUnixAddr("unix", r.config.SocketPath)
			if err != nil {
				return nil, err
			}

			return net.DialUnix("unix", nil, addr)
		},
	}

	transport := httptransport.New(fclient.DefaultHost, fclient.DefaultBasePath, fclient.DefaultSchemes)
	transport.Transport = socketTransport
	httpClient.SetTransport(transport)

	return httpClient
}

func (r *runtimeFC) vmRunning() bool {
	logrus.Debug("oci.vmRunning() start")
	defer logrus.Debug("oci.vmRunning() end")

	resp, err := r.client().Operations.DescribeInstance(nil)
	if err != nil {
		return false
	}

	switch resp.Payload.State {
	case fcmodels.InstanceInfoStateStarting:
		logrus.Errorf("unexpected-state %v", fcmodels.InstanceInfoStateStarting)
		return false
	case fcmodels.InstanceInfoStateRunning:
		return true
	case fcmodels.InstanceInfoStateUninitialized, fcmodels.InstanceInfoStateHalting, fcmodels.InstanceInfoStateHalted:
		return false
	default:
		return false
	}
}

// waitVMM waits for the VMM to be up and running.
func (r *runtimeFC) waitVMM(timeout int) error {
	logrus.Debug("oci.waitVMM() start")
	defer logrus.Debug("oci.waitVMM() end")

	if timeout < 0 {
		return fmt.Errorf("Invalid timeout %ds", timeout)
	}

	timeStart := time.Now()
	for {
		_, err := r.client().Operations.DescribeInstance(nil)
		if err == nil {
			return nil
		}

		if int(time.Now().Sub(timeStart).Seconds()) > timeout {
			return fmt.Errorf("Failed to connect to firecrackerinstance (timeout %ds): %v", timeout, err)
		}

		time.Sleep(time.Duration(10) * time.Millisecond)
	}
}

func (r *runtimeFC) client() *fclient.Firecracker {
	logrus.Debug("oci.client() start")
	defer logrus.Debug("oci.client() end")

	if r.fcClient == nil {
		r.fcClient = r.newFireClient()
	}

	return r.fcClient
}

func (r *runtimeFC) fcSetBootSource(path, params string) error {
	logrus.Debug("oci.fcSetBootSource() start")
	defer logrus.Debug("oci.fcSetBootSource() end")

	bootParams := params + " " + rootDevice
	bootSrcParams := fcops.NewPutGuestBootSourceParams()
	src := &fcmodels.BootSource{
		KernelImagePath: &path,
		BootArgs:        bootParams,
	}
	bootSrcParams.SetBody(src)

	_, err := r.client().Operations.PutGuestBootSource(bootSrcParams)
	if err != nil {
		return err
	}

	return nil
}

func (r *runtimeFC) fcSetVMRootfs(path string) error {
	logrus.Debug("oci.fcSetVMRootfs() start")
	defer logrus.Debug("oci.fcSetVMRootfs() end")

	driveID := "rootfs"
	driveParams := fcops.NewPutGuestDriveByIDParams()
	driveParams.SetDriveID(driveID)
	isReadOnly := false

	//Add it as a regular block device
	//This allows us to use a paritioned root block device
	isRootDevice := false
	drive := &fcmodels.Drive{
		DriveID:      &driveID,
		IsReadOnly:   &isReadOnly,
		IsRootDevice: &isRootDevice,
		PathOnHost:   &path,
	}
	driveParams.SetBody(drive)
	_, err := r.client().Operations.PutGuestDriveByID(driveParams)
	if err != nil {
		return err
	}

	return nil
}

// CreateContainer creates a container.
func (r *runtimeFC) CreateContainer(c *Container, cgroupParent string) (err error) {
	logrus.Debug("oci.CreateContainer() start")
	defer logrus.Debug("oci.CreateContainer() end")

	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()

	r.fcClient = r.newFireClient()

	c.state.Status = ContainerStateCreated

	return nil
}

func (r *runtimeFC) startVM(c *Container) error {
	logrus.Debug("oci.startVM() start")
	defer logrus.Debug("oci.startVM() end")

	_ = r.fcCleanup()

	cfg := firecracker.Config{
		SocketPath:      r.config.SocketPath,
		KernelImagePath: r.config.KernelImagePath,
		KernelArgs:      r.config.KernelArgs,
		MachineCfg: fcmodels.MachineConfiguration{
			MemSizeMib: 128,
		},
		Debug:             true,
		DisableValidation: true,
	}

	driveBuilder := firecracker.NewDrivesBuilder(r.config.RootDrive)

	cfg.Drives = driveBuilder.Build()

	cmdBuilder := firecracker.VMCommandBuilder{}.
		WithBin(r.config.FirecrackerBinaryPath).
		WithSocketPath(r.config.SocketPath).
		Build(r.ctx)

	vmmCtx, vmmCancel := context.WithCancel(context.Background())
	defer vmmCancel()

	var errMach error
	r.machine, errMach = firecracker.NewMachine(vmmCtx, cfg, firecracker.WithProcessRunner(cmdBuilder))
	if errMach != nil {
		return errMach
	}

	r.fcSetBootSource(r.config.KernelImagePath, r.config.KernelArgs)
	r.fcSetVMRootfs(r.config.RootDrive)

	if err := r.machine.Start(vmmCtx); err != nil {
		return err
	}

	return r.waitVMM(fcTimeout)
}

func (r *runtimeFC) fcStartVM() error {
	logrus.Debug("oci.fcStartVM() start")
	defer logrus.Debug("oci.fcStartVM() end")

	r.fcClient = r.newFireClient()

	actionParams := fcops.NewCreateSyncActionParams()
	actionInfo := &fcmodels.InstanceActionInfo{
		ActionType: "InstanceStart",
	}
	actionParams.SetInfo(actionInfo)
	_, err := r.client().Operations.CreateSyncAction(actionParams)
	if err != nil {
		return err
	}

	return nil
}

// StartContainer starts a container.
func (r *runtimeFC) StartContainer(c *Container) error {
	logrus.Debug("oci.StartContainer() start")
	defer logrus.Debug("oci.StartContainer() end")

	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()

	if err := r.startVM(c); err != nil {
		return err
	}

	c.state.Status = ContainerStateRunning

	return nil
}

// ExecContainer prepares a streaming endpoint to execute a command in the container.
func (r *runtimeFC) ExecContainer(c *Container, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	return nil
}

// ExecSyncContainer execs a command in a container and returns it's stdout, stderr and return code.
func (r *runtimeFC) ExecSyncContainer(c *Container, command []string, timeout int64) (*ExecSyncResponse, error) {
	return &ExecSyncResponse{}, nil
}

// UpdateContainer updates container resources
func (r *runtimeFC) UpdateContainer(c *Container, res *rspec.LinuxResources) error {
	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()

	return nil
}

// StopContainer stops a container. Timeout is given in seconds.
func (r *runtimeFC) StopContainer(ctx context.Context, c *Container, timeout int64) error {
	logrus.Debug("oci.StopContainer() start")
	defer logrus.Debug("oci.StopContainer() end")

	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()

	c.state.Status = ContainerStateStopped

	if err := waitContainerStop(r.ctx, c, killContainerTimeout, false); err != nil {
		return err
	}

	return nil
}

func (r *runtimeFC) stopVM() error {
	logrus.Debug("oci.stopVM() start")
	defer logrus.Debug("oci.stopVM() end")

	if r.machine == nil {
		return fmt.Errorf("machine is not available")
	}

	return r.machine.StopVMM()
}

// DeleteContainer deletes a container.
func (r *runtimeFC) DeleteContainer(c *Container) error {
	logrus.Debug("oci.DeleteContainer() start")
	defer logrus.Debug("oci.DeleteContainer() end")

	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()

	_ = r.fcCleanup()

	if err := r.remove(r.ctx, c.ID(), ""); err != nil {
		return err
	}

	c.state.Status = ContainerStateStopped

	// NOTE: ignore error, since we should continue removing the container
	// on the cri-o side, even if stopVM could not kill the firecracker process.
	for i := 0; i < 3; i++ {
		if err := r.stopVM(); err != nil {
			logrus.Warnf("stopVM failed, but continue removing the container: %v", err)
			fmt.Printf("stopVM failed, but continue removing the container: %v\n", err)
			time.Sleep(500 * time.Millisecond)
		} else {
			break
		}
	}
	return nil
}

func (r *runtimeFC) fcCleanup() error {
	logrus.Infof("Cleaning up firecracker socket %s", r.config.SocketPath)

	cmd := exec.Command("/bin/rm", "-f", r.config.SocketPath)
	if err := cmd.Start(); err != nil {
		logrus.Errorf("Error cleaning up firecracker", err)
		return err
	}
	return nil
}

// UpdateContainerStatus refreshes the status of the container.
func (r *runtimeFC) UpdateContainerStatus(c *Container) error {
	return nil
}

// PauseContainer pauses a container.
func (r *runtimeFC) PauseContainer(c *Container) error {
	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()

	c.state.Status = ContainerStatePaused

	return nil
}

// UnpauseContainer unpauses a container.
func (r *runtimeFC) UnpauseContainer(c *Container) error {
	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()

	c.state.Status = ContainerStateRunning

	return nil
}

// ContainerStats provides statistics of a container.
func (r *runtimeFC) ContainerStats(c *Container) (*ContainerStats, error) {
	return &ContainerStats{}, nil
}

// SignalContainer sends a signal to a container process.
func (r *runtimeFC) SignalContainer(c *Container, sig syscall.Signal) error {
	logrus.Debug("oci.SignalContainer() start")
	defer logrus.Debug("oci.SignalContainer() end")

	// Lock the container
	c.opLock.Lock()
	defer c.opLock.Unlock()

	return r.kill(r.ctx, c.ID(), "", sig, true)
}

// attachContainer attaches IO to a running container.
func (r *runtimeFC) AttachContainer(c *Container, inputStream io.Reader, outputStream, errorStream io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	return nil
}

// PortForwardContainer forwards the specified port provides statistics of a container.
func (r *runtimeFC) PortForwardContainer(c *Container, port int32, stream io.ReadWriter) error {
	return nil
}

// ReopenContainerLog reopens the log file of a container.
func (r *runtimeFC) ReopenContainerLog(c *Container) error {
	return nil
}

func (r *runtimeFC) WaitContainerStateStopped(ctx context.Context, c *Container) error {
	return nil
}

func (r *runtimeFC) kill(ctx context.Context, ctrID, execID string, signal syscall.Signal, all bool) error {
	return nil
}

func (r *runtimeFC) remove(ctx context.Context, ctrID, execID string) error {
	return nil
}
