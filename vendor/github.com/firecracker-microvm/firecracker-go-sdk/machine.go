// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package firecracker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/containerd/fifo"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/gofrs/uuid"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
)

const (
	userAgent = "firecracker-go-sdk"

	// as specified in http://man7.org/linux/man-pages/man8/ip-netns.8.html
	defaultNetNSDir = "/var/run/netns"

	// env name to make firecracker init timeout configurable
	firecrackerInitTimeoutEnv = "FIRECRACKER_GO_SDK_INIT_TIMEOUT_SECONDS"

	defaultFirecrackerInitTimeoutSeconds = 3
)

// SeccompLevelValue represents a secure computing level type.
type SeccompLevelValue int

// secure computing levels
const (
	// SeccompLevelDisable is the default value.
	SeccompLevelDisable SeccompLevelValue = iota
	// SeccompLevelBasic prohibits syscalls not whitelisted by Firecracker.
	SeccompLevelBasic
	// SeccompLevelAdvanced adds further checks on some of the parameters of the
	// allowed syscalls.
	SeccompLevelAdvanced
)

func (level SeccompLevelValue) String() string {
	return strconv.Itoa(int(level))
}

// ErrAlreadyStarted signifies that the Machine has already started and cannot
// be started again.
var ErrAlreadyStarted = errors.New("firecracker: machine already started")

// Config is a collection of user-configurable VMM settings
type Config struct {
	// SocketPath defines the file path where the Firecracker control socket
	// should be created.
	SocketPath string

	// LogFifo defines the file path where the Firecracker log named-pipe should
	// be located.
	LogFifo string

	// LogLevel defines the verbosity of Firecracker logging.  Valid values are
	// "Error", "Warning", "Info", and "Debug", and are case-sensitive.
	LogLevel string

	// MetricsFifo defines the file path where the Firecracker metrics
	// named-pipe should be located.
	MetricsFifo string

	// KernelImagePath defines the file path where the kernel image is located.
	// The kernel image must be an uncompressed ELF image.
	KernelImagePath string

	// InitrdPath defines the file path where initrd image is located.
	//
	// This parameter is optional.
	InitrdPath string

	// KernelArgs defines the command-line arguments that should be passed to
	// the kernel.
	KernelArgs string

	// Drives specifies BlockDevices that should be made available to the
	// microVM.
	Drives []models.Drive

	// NetworkInterfaces specifies the tap devices that should be made available
	// to the microVM.
	NetworkInterfaces NetworkInterfaces

	// FifoLogWriter is an io.Writer that is used to redirect the contents of the
	// fifo log to the writer.
	FifoLogWriter io.Writer

	// VsockDevices specifies the vsock devices that should be made available to
	// the microVM.
	VsockDevices []VsockDevice

	// Debug enables debug-level logging for the SDK.
	Debug bool

	// MachineCfg represents the firecracker microVM process configuration
	MachineCfg models.MachineConfiguration

	// DisableValidation allows for easier mock testing by disabling the
	// validation of configuration performed by the SDK.
	DisableValidation bool

	// JailerCfg is configuration specific for the jailer process.
	JailerCfg *JailerConfig

	// (Optional) VMID is a unique identifier for this VM. It's set to a
	// random uuid if not provided by the user. It's currently used to
	// set the CNI ContainerID and create a network namespace path if
	// CNI configuration is provided as part of NetworkInterfaces
	VMID string

	// NetNS represents the path to a network namespace handle. If present, the
	// application will use this to join the associated network namespace
	NetNS string

	// ForwardSignals is an optional list of signals to catch and forward to
	// firecracker. If not provided, the default signals will be used.
	ForwardSignals []os.Signal

	// SeccompLevel specifies whether seccomp filters should be installed and how
	// restrictive they should be. Possible values are:
	//
	//	0 : (default): disabled.
	//	1 : basic filtering. This prohibits syscalls not whitelisted by Firecracker.
	//	2 : advanced filtering. This adds further checks on some of the
	//			parameters of the allowed syscalls.
	SeccompLevel SeccompLevelValue
}

// Validate will ensure that the required fields are set and that
// the fields are valid values.
func (cfg *Config) Validate() error {
	if cfg.DisableValidation {
		return nil
	}

	if _, err := os.Stat(cfg.KernelImagePath); err != nil {
		return fmt.Errorf("failed to stat kernel image path, %q: %v", cfg.KernelImagePath, err)
	}

	if cfg.InitrdPath != "" {
		if _, err := os.Stat(cfg.InitrdPath); err != nil {
			return fmt.Errorf("failed to stat initrd image path, %q: %v", cfg.InitrdPath, err)
		}
	}

	for _, drive := range cfg.Drives {
		if BoolValue(drive.IsRootDevice) {
			rootPath := StringValue(drive.PathOnHost)
			if _, err := os.Stat(rootPath); err != nil {
				return fmt.Errorf("failed to stat host path, %q: %v", rootPath, err)
			}

			break
		}
	}

	// Check the non-existence of some files:
	if _, err := os.Stat(cfg.SocketPath); err == nil {
		return fmt.Errorf("socket %s already exists", cfg.SocketPath)
	}

	if cfg.MachineCfg.VcpuCount == nil ||
		Int64Value(cfg.MachineCfg.VcpuCount) < 1 {
		return fmt.Errorf("machine needs a nonzero VcpuCount")
	}
	if cfg.MachineCfg.MemSizeMib == nil ||
		Int64Value(cfg.MachineCfg.MemSizeMib) < 1 {
		return fmt.Errorf("machine needs a nonzero amount of memory")
	}
	if cfg.MachineCfg.HtEnabled == nil {
		return fmt.Errorf("machine needs a setting for ht_enabled")
	}
	return nil
}

func (cfg *Config) ValidateNetwork() error {
	if cfg.DisableValidation {
		return nil
	}

	return cfg.NetworkInterfaces.validate(parseKernelArgs(cfg.KernelArgs))
}

// Machine is the main object for manipulating Firecracker microVMs
type Machine struct {
	// Handlers holds the set of handlers that are run for validation and start
	Handlers Handlers

	Cfg           Config
	client        *Client
	cmd           *exec.Cmd
	logger        *log.Entry
	machineConfig models.MachineConfiguration // The actual machine config as reported by Firecracker
	// startOnce ensures that the machine can only be started once
	startOnce sync.Once
	// exitCh is a channel which gets closed when the VMM exits
	exitCh chan struct{}
	// fatalErr records an error that either stops or prevent starting the VMM
	fatalErr error

	// callbacks that should be run when the machine is being torn down
	cleanupOnce  sync.Once
	cleanupFuncs []func() error
}

// Logger returns a logrus logger appropriate for logging hypervisor messages
func (m *Machine) Logger() *log.Entry {
	return m.logger.WithField("subsystem", userAgent)
}

// PID returns the machine's running process PID or an error if not running
func (m *Machine) PID() (int, error) {
	if m.cmd == nil || m.cmd.Process == nil {
		return 0, fmt.Errorf("machine is not running")
	}
	select {
	case <-m.exitCh:
		return 0, fmt.Errorf("machine process has exited")
	default:
	}
	return m.cmd.Process.Pid, nil
}

func (m *Machine) doCleanup() error {
	var err *multierror.Error
	m.cleanupOnce.Do(func() {
		// run them in reverse order so changes are "unwound" (similar to defer statements)
		for i := range m.cleanupFuncs {
			cleanupFunc := m.cleanupFuncs[len(m.cleanupFuncs)-1-i]
			err = multierror.Append(err, cleanupFunc())
		}
	})
	return err.ErrorOrNil()
}

// RateLimiterSet represents a pair of RateLimiters (inbound and outbound)
type RateLimiterSet struct {
	// InRateLimiter limits the incoming bytes.
	InRateLimiter *models.RateLimiter
	// OutRateLimiter limits the outgoing bytes.
	OutRateLimiter *models.RateLimiter
}

// VsockDevice represents a vsock connection between the host and the guest
// microVM.
type VsockDevice struct {
	// ID defines the vsock's device ID for firecracker.
	ID string
	// Path defines the filesystem path of the vsock device on the host.
	Path string
	// CID defines the 32-bit Context Identifier for the vsock device.  See
	// the vsock(7) manual page for more information.
	CID uint32
}

// SocketPath returns the filesystem path to the socket used for VMM
// communication
func (m *Machine) socketPath() string {
	return m.Cfg.SocketPath
}

// LogFile returns the filesystem path of the VMM log
func (m *Machine) LogFile() string {
	return m.Cfg.LogFifo
}

// LogLevel returns the VMM log level.
func (m *Machine) LogLevel() string {
	return m.Cfg.LogLevel
}

// NewMachine initializes a new Machine instance and performs validation of the
// provided Config.
func NewMachine(ctx context.Context, cfg Config, opts ...Opt) (*Machine, error) {
	m := &Machine{
		exitCh: make(chan struct{}),
	}

	m.Handlers = defaultHandlers

	if cfg.JailerCfg != nil {
		m.Handlers.Validation = m.Handlers.Validation.Append(JailerConfigValidationHandler)
		if err := jail(ctx, m, &cfg); err != nil {
			return nil, err
		}
	} else {
		m.Handlers.Validation = m.Handlers.Validation.Append(ConfigValidationHandler)
		m.cmd = defaultFirecrackerVMMCommandBuilder.
			WithSocketPath(cfg.SocketPath).
			AddArgs("--seccomp-level", cfg.SeccompLevel.String()).
			Build(ctx)
	}

	for _, opt := range opts {
		opt(m)
	}

	if m.logger == nil {
		logger := log.New()
		if cfg.Debug {
			logger.SetLevel(log.DebugLevel)
		}

		m.logger = log.NewEntry(logger)
	}

	if m.client == nil {
		m.client = NewClient(cfg.SocketPath, m.logger, cfg.Debug)
	}

	if cfg.VMID == "" {
		randomID, err := uuid.NewV4()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create random ID for VMID")
		}
		cfg.VMID = randomID.String()
	}

	if cfg.ForwardSignals == nil {
		cfg.ForwardSignals = []os.Signal{
			os.Interrupt,
			syscall.SIGQUIT,
			syscall.SIGTERM,
			syscall.SIGHUP,
			syscall.SIGABRT,
		}
	}

	m.machineConfig = cfg.MachineCfg
	m.Cfg = cfg

	if cfg.NetNS == "" && cfg.NetworkInterfaces.cniInterface() != nil {
		m.Cfg.NetNS = m.defaultNetNSPath()
	}

	m.logger.Debug("Called NewMachine()")
	return m, nil
}

// Start actually start a Firecracker microVM.
// The context must not be cancelled while the microVM is running.
//
// It will iterate through the handler list and call each handler. If an
// error occurred during handler execution, that error will be returned. If the
// handlers succeed, then this will start the VMM instance.
// Start may only be called once per Machine.  Subsequent calls will return
// ErrAlreadyStarted.
func (m *Machine) Start(ctx context.Context) error {
	m.logger.Debug("Called Machine.Start()")
	alreadyStarted := true
	m.startOnce.Do(func() {
		m.logger.Debug("Marking Machine as Started")
		alreadyStarted = false
	})
	if alreadyStarted {
		return ErrAlreadyStarted
	}

	var err error
	defer func() {
		if err != nil {
			if cleanupErr := m.doCleanup(); cleanupErr != nil {
				m.Logger().Errorf(
					"failed to cleanup VM after previous start failure: %v", cleanupErr)
			}
		}
	}()

	err = m.Handlers.Run(ctx, m)
	if err != nil {
		return err
	}

	err = m.startInstance(ctx)
	return err
}

// Shutdown requests a clean shutdown of the VM by sending CtrlAltDelete on the virtual keyboard
func (m *Machine) Shutdown(ctx context.Context) error {
	m.logger.Debug("Called machine.Shutdown()")
	return m.sendCtrlAltDel(ctx)
}

// Wait will wait until the firecracker process has finished.  Wait is safe to
// call concurrently, and will deliver the same error to all callers, subject to
// each caller's context cancellation.
func (m *Machine) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-m.exitCh:
		return m.fatalErr
	}
}

func (m *Machine) setupNetwork(ctx context.Context) error {
	err, cleanupFuncs := m.Cfg.NetworkInterfaces.setupNetwork(ctx, m.Cfg.VMID, m.Cfg.NetNS, m.logger)
	m.cleanupFuncs = append(m.cleanupFuncs, cleanupFuncs...)
	return err
}

func (m *Machine) setupKernelArgs(ctx context.Context) error {
	kernelArgs := parseKernelArgs(m.Cfg.KernelArgs)

	// If any network interfaces have a static IP configured, we need to set the "ip=" boot param.
	// Validation that we are not overriding an existing "ip=" setting happens in the network validation
	if staticIPInterface := m.Cfg.NetworkInterfaces.staticIPInterface(); staticIPInterface != nil {
		ipBootParam := staticIPInterface.StaticConfiguration.IPConfiguration.ipBootParam()
		kernelArgs["ip"] = &ipBootParam
	}

	m.Cfg.KernelArgs = kernelArgs.String()
	return nil
}

func (m *Machine) createNetworkInterfaces(ctx context.Context, ifaces ...NetworkInterface) error {
	for id, iface := range ifaces {
		if err := m.createNetworkInterface(ctx, iface, id+1); err != nil {
			return err
		}
	}

	return nil
}

func (m *Machine) addVsocks(ctx context.Context, vsocks ...VsockDevice) error {
	for _, dev := range m.Cfg.VsockDevices {
		if err := m.addVsock(ctx, dev); err != nil {
			return err
		}
	}
	return nil
}

func (m *Machine) attachDrives(ctx context.Context, drives ...models.Drive) error {
	for _, dev := range drives {
		if err := m.attachDrive(ctx, dev); err != nil {
			m.logger.Errorf("While attaching drive %s, got error %s", StringValue(dev.PathOnHost), err)
			return err
		}
		m.logger.Debugf("attachDrive returned for %s", StringValue(dev.PathOnHost))
	}

	return nil
}

func (m *Machine) defaultNetNSPath() string {
	return filepath.Join(defaultNetNSDir, m.Cfg.VMID)
}

// startVMM starts the firecracker vmm process and configures logging.
func (m *Machine) startVMM(ctx context.Context) error {
	m.logger.Printf("Called startVMM(), setting up a VMM on %s", m.Cfg.SocketPath)
	startCmd := m.cmd.Start

	var err error
	if m.Cfg.NetNS != "" && m.Cfg.JailerCfg == nil {
		// If the VM needs to be started in a netns but no jailer netns was configured,
		// start the vmm child process in the netns directly here.
		err = ns.WithNetNSPath(m.Cfg.NetNS, func(_ ns.NetNS) error {
			return startCmd()
		})
	} else {
		// Else, just start the process normally as it's either not in a netns or will
		// be placed in one by the jailer process instead.
		err = startCmd()
	}

	if err != nil {
		m.logger.Errorf("Failed to start VMM: %s", err)

		m.fatalErr = err
		close(m.exitCh)

		return err
	}
	m.logger.Debugf("VMM started socket path is %s", m.Cfg.SocketPath)

	m.cleanupFuncs = append(m.cleanupFuncs,
		func() error {
			if err := os.Remove(m.Cfg.SocketPath); !os.IsNotExist(err) {
				return err
			}
			return nil
		},
	)

	errCh := make(chan error)
	go func() {
		waitErr := m.cmd.Wait()
		if waitErr != nil {
			m.logger.Warnf("firecracker exited: %s", waitErr.Error())
		} else {
			m.logger.Printf("firecracker exited: status=0")
		}

		cleanupErr := m.doCleanup()
		if cleanupErr != nil {
			m.logger.Errorf("failed to cleanup after VM exit: %v", cleanupErr)
		}

		errCh <- multierror.Append(waitErr, cleanupErr).ErrorOrNil()

		// Notify subscribers that there will be no more values.
		// When err is nil, two reads are performed (waitForSocket and close exitCh goroutine),
		// second one never ends as it tries to read from empty channel.
		close(errCh)
	}()

	m.setupSignals()

	// Wait for firecracker to initialize:
	err = m.waitForSocket(time.Duration(m.client.firecrackerInitTimeout)*time.Second, errCh)
	if err != nil {
		err = errors.Wrapf(err, "Firecracker did not create API socket %s", m.Cfg.SocketPath)
		m.fatalErr = err
		close(m.exitCh)

		return err
	}

	// This goroutine is used to kill the process by context cancelletion,
	// but doesn't tell anyone about that.
	go func() {
		<-ctx.Done()
		err := m.stopVMM()
		if err != nil {
			m.logger.WithError(err).Errorf("failed to stop vm %q", m.Cfg.VMID)
		}
	}()

	// This goroutine is used to tell clients that the process is stopped
	// (gracefully or not).
	go func() {
		m.fatalErr = <-errCh
		m.logger.Debugf("closing the exitCh %v", m.fatalErr)
		close(m.exitCh)
	}()

	m.logger.Debugf("returning from startVMM()")
	return nil
}

//StopVMM stops the current VMM.
func (m *Machine) StopVMM() error {
	return m.stopVMM()
}

func (m *Machine) stopVMM() error {
	if m.cmd != nil && m.cmd.Process != nil {
		m.logger.Debug("stopVMM(): sending sigterm to firecracker")
		err := m.cmd.Process.Signal(syscall.SIGTERM)
		if err != nil && !strings.Contains(err.Error(), "os: process already finished") {
			return err
		}
		return nil
	}
	m.logger.Debug("stopVMM(): no firecracker process running, not sending a signal")

	// don't return an error if the process isn't even running
	return nil
}

// createFifos sets up the firecracker logging and metrics FIFOs
func createFifos(logFifo, metricsFifo string) error {
	log.Debugf("Creating FIFO %s", logFifo)
	if err := syscall.Mkfifo(logFifo, 0700); err != nil {
		return fmt.Errorf("Failed to create log fifo: %v", err)
	}

	log.Debugf("Creating metric FIFO %s", metricsFifo)
	if err := syscall.Mkfifo(metricsFifo, 0700); err != nil {
		return fmt.Errorf("Failed to create metric fifo: %v", err)
	}
	return nil
}

func (m *Machine) setupLogging(ctx context.Context) error {
	if len(m.Cfg.LogFifo) == 0 || len(m.Cfg.MetricsFifo) == 0 {
		// No logging configured
		m.logger.Printf("VMM logging and metrics disabled.")
		return nil
	}

	l := models.Logger{
		LogFifo:       String(m.Cfg.LogFifo),
		Level:         String(m.Cfg.LogLevel),
		MetricsFifo:   String(m.Cfg.MetricsFifo),
		ShowLevel:     Bool(true),
		ShowLogOrigin: Bool(false),
	}

	_, err := m.client.PutLogger(ctx, &l)
	if err != nil {
		return err
	}

	m.logger.Debugf("Configured VMM logging to %s, metrics to %s",
		m.Cfg.LogFifo,
		m.Cfg.MetricsFifo,
	)

	return nil
}

func (m *Machine) captureFifoToFile(ctx context.Context, logger *log.Entry, fifoPath string, w io.Writer) error {
	return m.captureFifoToFileWithChannel(ctx, logger, fifoPath, w, make(chan error, 1))
}

func (m *Machine) captureFifoToFileWithChannel(ctx context.Context, logger *log.Entry, fifoPath string, w io.Writer, done chan error) error {
	// open the fifo pipe which will be used
	// to write its contents to a file.
	fifoPipe, err := fifo.OpenFifo(ctx, fifoPath, syscall.O_RDONLY|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return fmt.Errorf("Failed to open fifo path at %q: %v", fifoPath, err)
	}

	logger.Debugf("Capturing %q to writer", fifoPath)

	// this goroutine is to track the life of the application along with whether
	// or not the context has been cancelled which is signified by the exitCh. In
	// the event that the exitCh has been closed, we will close the fifo file.
	go func() {
		<-m.exitCh
		if err := fifoPipe.Close(); err != nil {
			logger.WithError(err).Debug("failed to close fifo")
		}
	}()

	// Uses a goroutine to copy the contents of the fifo pipe to the io.Writer.
	// In the event that the goroutine finishes, which is caused by either the
	// context being closed or the application being closed, we will close the
	// pipe and unlink the fifo path.
	go func() {
		defer func() {
			if err := fifoPipe.Close(); err != nil {
				logger.Warnf("Failed to close fifo pipe: %v", err)
			}

			if err := syscall.Unlink(fifoPath); err != nil {
				logger.Warnf("Failed to unlink %s: %v", fifoPath, err)
			}
		}()

		if _, err := io.Copy(w, fifoPipe); err != nil {
			logger.WithError(err).Warn("io.Copy failed to copy contents of fifo pipe")
			done <- err
		}

		close(done)
	}()

	return nil
}

func (m *Machine) createMachine(ctx context.Context) error {
	resp, err := m.client.PutMachineConfiguration(ctx, &m.Cfg.MachineCfg)
	if err != nil {
		m.logger.Errorf("PutMachineConfiguration returned %s", resp.Error())
		return err
	}

	m.logger.Debug("PutMachineConfiguration returned")
	err = m.refreshMachineConfiguration()
	if err != nil {
		m.logger.Errorf("Unable to inspect Firecracker MachineConfiguration. Continuing anyway. %s", err)
	}
	m.logger.Debug("createMachine returning")
	return err
}

func (m *Machine) createBootSource(ctx context.Context, imagePath, initrdPath, kernelArgs string) error {
	bsrc := models.BootSource{
		KernelImagePath: &imagePath,
		InitrdPath:      initrdPath,
		BootArgs:        kernelArgs,
	}

	resp, err := m.client.PutGuestBootSource(ctx, &bsrc)
	if err == nil {
		m.logger.Printf("PutGuestBootSource: %s", resp.Error())
	}

	return err
}

func (m *Machine) createNetworkInterface(ctx context.Context, iface NetworkInterface, iid int) error {
	ifaceID := strconv.Itoa(iid)

	if iface.StaticConfiguration == nil {
		// this should not be possible, but check nil anyways to prevent a panic
		// if there is a bug
		return errors.New("invalid nil state for network interface")
	}

	m.logger.Printf("Attaching NIC %s (hwaddr %s) at index %s",
		iface.StaticConfiguration.HostDevName, iface.StaticConfiguration.MacAddress, ifaceID)

	ifaceCfg := models.NetworkInterface{
		IfaceID:           &ifaceID,
		GuestMac:          iface.StaticConfiguration.MacAddress,
		HostDevName:       String(iface.StaticConfiguration.HostDevName),
		AllowMmdsRequests: iface.AllowMMDS,
	}

	if iface.InRateLimiter != nil {
		ifaceCfg.RxRateLimiter = iface.InRateLimiter
	}

	if iface.OutRateLimiter != nil {
		ifaceCfg.TxRateLimiter = iface.OutRateLimiter
	}

	if iface.InRateLimiter != nil {
		ifaceCfg.RxRateLimiter = iface.InRateLimiter
	}

	if iface.OutRateLimiter != nil {
		ifaceCfg.TxRateLimiter = iface.OutRateLimiter
	}

	resp, err := m.client.PutGuestNetworkInterfaceByID(ctx, ifaceID, &ifaceCfg)
	if err == nil {
		m.logger.Debugf("PutGuestNetworkInterfaceByID: %s", resp.Error())
	}

	m.logger.Debugf("createNetworkInterface returned for %s", iface.StaticConfiguration.HostDevName)
	return err
}

// UpdateGuestNetworkInterfaceRateLimit modifies the specified network interface's rate limits
func (m *Machine) UpdateGuestNetworkInterfaceRateLimit(ctx context.Context, ifaceID string, rateLimiters RateLimiterSet, opts ...PatchGuestNetworkInterfaceByIDOpt) error {
	iface := models.PartialNetworkInterface{
		IfaceID: &ifaceID,
	}
	if rateLimiters.InRateLimiter != nil {
		iface.RxRateLimiter = rateLimiters.InRateLimiter
	}
	if rateLimiters.OutRateLimiter != nil {
		iface.TxRateLimiter = rateLimiters.InRateLimiter
	}
	if _, err := m.client.PatchGuestNetworkInterfaceByID(ctx, ifaceID, &iface, opts...); err != nil {
		m.logger.Errorf("Update network interface failed: %s: %v", ifaceID, err)
		return err
	}

	m.logger.Infof("Updated network interface: %s", ifaceID)
	return nil
}

// attachDrive attaches a secondary block device
func (m *Machine) attachDrive(ctx context.Context, dev models.Drive) error {
	hostPath := StringValue(dev.PathOnHost)
	m.logger.Infof("Attaching drive %s, slot %s, root %t.", hostPath, StringValue(dev.DriveID), BoolValue(dev.IsRootDevice))
	respNoContent, err := m.client.PutGuestDriveByID(ctx, StringValue(dev.DriveID), &dev)
	if err == nil {
		m.logger.Printf("Attached drive %s: %s", hostPath, respNoContent.Error())
	} else {
		m.logger.Errorf("Attach drive failed: %s: %s", hostPath, err)
	}
	return err
}

// addVsock adds a vsock to the instance
func (m *Machine) addVsock(ctx context.Context, dev VsockDevice) error {
	vsockCfg := models.Vsock{
		GuestCid: Int64(int64(dev.CID)),
		UdsPath:  &dev.Path,
		VsockID:  &dev.ID,
	}

	resp, err := m.client.PutGuestVsock(ctx, &vsockCfg)
	if err != nil {
		return err
	}
	m.logger.Debugf("Attach vsock %s successful: %s", dev.Path, resp.Error())
	return nil
}

func (m *Machine) startInstance(ctx context.Context) error {
	action := models.InstanceActionInfoActionTypeInstanceStart
	info := models.InstanceActionInfo{
		ActionType: &action,
	}

	resp, err := m.client.CreateSyncAction(ctx, &info)
	if err == nil {
		m.logger.Printf("startInstance successful: %s", resp.Error())
	} else {
		m.logger.Errorf("Starting instance: %s", err)
	}
	return err
}

func (m *Machine) sendCtrlAltDel(ctx context.Context) error {
	action := models.InstanceActionInfoActionTypeSendCtrlAltDel
	info := models.InstanceActionInfo{
		ActionType: &action,
	}

	resp, err := m.client.CreateSyncAction(ctx, &info)
	if err == nil {
		m.logger.Printf("Sent instance shutdown request: %s", resp.Error())
	} else {
		m.logger.Errorf("Unable to send CtrlAltDel: %s", err)
	}
	return err
}

// SetMetadata sets the machine's metadata for MDDS
func (m *Machine) SetMetadata(ctx context.Context, metadata interface{}) error {
	if _, err := m.client.PutMmds(ctx, metadata); err != nil {
		m.logger.Errorf("Setting metadata: %s", err)
		return err
	}

	m.logger.Printf("SetMetadata successful")
	return nil
}

// UpdateMetadata patches the machine's metadata for MDDS
func (m *Machine) UpdateMetadata(ctx context.Context, metadata interface{}) error {
	if _, err := m.client.PatchMmds(ctx, metadata); err != nil {
		m.logger.Errorf("Updating metadata: %s", err)
		return err
	}

	m.logger.Printf("UpdateMetadata successful")
	return nil
}

// GetMetadata gets the machine's metadata from MDDS and unmarshals it into v
func (m *Machine) GetMetadata(ctx context.Context, v interface{}) error {
	resp, err := m.client.GetMmds(ctx)
	if err != nil {
		m.logger.Errorf("Getting metadata: %s", err)
		return err
	}

	payloadData, err := json.Marshal(resp.Payload)
	if err != nil {
		m.logger.Errorf("Getting metadata failed parsing payload: %s", err)
		return err
	}

	if err := json.Unmarshal(payloadData, v); err != nil {
		m.logger.Errorf("Getting metadata failed parsing payload: %s", err)
		return err
	}

	m.logger.Printf("GetMetadata successful")
	return nil
}

// UpdateGuestDrive will modify the current guest drive of ID index with the new
// parameters of the partialDrive.
func (m *Machine) UpdateGuestDrive(ctx context.Context, driveID, pathOnHost string, opts ...PatchGuestDriveByIDOpt) error {
	if _, err := m.client.PatchGuestDriveByID(ctx, driveID, pathOnHost, opts...); err != nil {
		m.logger.Errorf("PatchGuestDrive failed: %v", err)
		return err
	}

	m.logger.Printf("PatchGuestDrive successful")
	return nil
}

// refreshMachineConfiguration synchronizes our cached representation of the machine configuration
// with that reported by the Firecracker API
func (m *Machine) refreshMachineConfiguration() error {
	resp, err := m.client.GetMachineConfiguration()
	if err != nil {
		return err
	}

	m.logger.Infof("refreshMachineConfiguration: %s", resp.Error())
	m.machineConfig = *resp.Payload
	return nil
}

// waitForSocket waits for the given file to exist
func (m *Machine) waitForSocket(timeout time.Duration, exitchan chan error) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	ticker := time.NewTicker(10 * time.Millisecond)

	defer func() {
		cancel()
		ticker.Stop()
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-exitchan:
			return err
		case <-ticker.C:
			if _, err := os.Stat(m.Cfg.SocketPath); err != nil {
				continue
			}

			// Send test HTTP request to make sure socket is available
			if _, err := m.client.GetMachineConfiguration(); err != nil {
				continue
			}

			return nil
		}
	}
}

// Set up a signal handler to pass through to firecracker
func (m *Machine) setupSignals() {
	signals := m.Cfg.ForwardSignals

	if len(signals) == 0 {
		return
	}

	m.logger.Debugf("Setting up signal handler: %v", signals)
	sigchan := make(chan os.Signal, len(signals))
	signal.Notify(sigchan, signals...)

	go func() {
	ForLoop:
		for {
			select {
			case sig := <-sigchan:
				m.logger.Debugf("Caught signal %s", sig)
				// Some signals kill the process, some of them are not.
				m.cmd.Process.Signal(sig)
			case <-m.exitCh:
				// And if a signal kills the process, we can stop this for loop and remove sigchan.
				break ForLoop
			}
		}

		signal.Stop(sigchan)
		close(sigchan)
	}()
}
