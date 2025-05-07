package oci

import (
	"context"
	"errors"
	"fmt"
	"github.com/cri-o/cri-o/internal/log"
	"os"
	"sync"
	"syscall"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	"github.com/cri-o/cri-o/utils"
)

const InfraContainerName = "POD"

func (r *runtimeOCI) createContainerPlatform(c *Container, cgroupParent string, pid int) error {
	g := &generate.Generator{
		Config: &rspec.Spec{
			Linux: &rspec.Linux{
				Resources: &rspec.LinuxResources{},
			},
		},
	}

	// First, set the cpuset as the one for the infra container.
	// This should be overridden if specified in a workload.
	// It should not be applied unless the conmon cgroup is "pod".
	// Otherwise, the cpuset will be configured for whatever cgroup the conmons share
	// (which by default is system.slice).
	if r.config.InfraCtrCPUSet != "" && r.handler.MonitorCgroup == utils.PodCgroupName {
		logrus.Debugf("Set the conmon cpuset to %q", r.config.InfraCtrCPUSet)
		g.SetLinuxResourcesCPUCpus(r.config.InfraCtrCPUSet)
	}

	// Mutate our newly created spec to find the customizations that are needed for conmon
	if err := r.config.Workloads.MutateSpecGivenAnnotations(InfraContainerName, g, c.Annotations()); err != nil {
		return err
	}

	// Move conmon to specified cgroup
	conmonCgroupfsPath, err := r.config.CgroupManager().MoveConmonToCgroup(c.ID(), cgroupParent, r.handler.MonitorCgroup, pid, g.Config.Linux.Resources)
	if err != nil {
		return err
	}

	c.conmonCgroupfsPath = conmonCgroupfsPath

	return nil
}

func sysProcAttrPlatform() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// newPipe creates a unix socket pair for communication.
func newPipe() (parent, child *os.File, _ error) {
	fds, err := unix.Socketpair(unix.AF_LOCAL, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}

	return os.NewFile(uintptr(fds[1]), "parent"), os.NewFile(uintptr(fds[0]), "child"), nil
}

// EpollProcessMonitor handles monitoring multiple processes using a single epoll instance
type EpollProcessMonitor struct {
	epfd                   int
	containerIdProcessInfo map[string]*ProcessInfo
	pidfdProcessInfo       map[int]*ProcessInfo
	mu                     sync.Mutex
	stopChan               chan struct{}
}

type monitorCallback func(context.Context, *Container)

type ProcessInfo struct {
	pidfd     int
	container *Container
	callback  monitorCallback
}

// NewEpollProcessMonitor creates a new process monitor
func NewEpollProcessMonitor() (ProcessMonitor, error) {
	epfd, err := syscall.EpollCreate1(syscall.EPOLL_CLOEXEC)
	if err != nil {
		return nil, fmt.Errorf("failed to create epoll instance: %w", err)
	}

	pm := &EpollProcessMonitor{
		epfd:                   epfd,
		containerIdProcessInfo: make(map[string]*ProcessInfo),
		pidfdProcessInfo:       make(map[int]*ProcessInfo),
		mu:                     sync.Mutex{},
		stopChan:               make(chan struct{}),
	}

	go pm.monitor()

	return pm, nil
}

// monitor is the only goroutine that blocks on epoll_wait.
func (pm *EpollProcessMonitor) monitor() {
	// Buffer for epoll events
	events := make([]syscall.EpollEvent, 500)
	ctx := context.Background()

	for {
		// Use a reasonable timeout instead of blocking indefinitely
		n, err := syscall.EpollWait(pm.epfd, events, 1000) // 1 second timeout

		// Check if we should stop
		select {
		case <-pm.stopChan:
			return
		default:
			// Continue processing
		}

		if err != nil {
			if errors.Is(err, syscall.EINTR) {
				continue // Ignore interrupted syscalls
			}
			log.Errorf(context.TODO(), "epoll_wait error: %v", err)
			continue
		}

		for i := 0; i < n; i++ {
			pi, ok := pm.pidfdProcessInfo[int(events[i].Fd)]
			if !ok {
				log.Errorf(context.TODO(), "pidfd not found in pidfds map")
				continue
			}
			go pm.handleExit(ctx, pi)
		}
	}
}

// AddProcess adds a process to be monitored.
func (pm *EpollProcessMonitor) AddProcess(container *Container, pidfd int, callback monitorCallback) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Add the pidfd to the epoll instance
	event := &syscall.EpollEvent{
		Events: syscall.EPOLLIN,
		Fd:     int32(pidfd),
	}

	if err := syscall.EpollCtl(pm.epfd, syscall.EPOLL_CTL_ADD, pidfd, event); err != nil {
		return fmt.Errorf("failed to add pidfd to epoll: %w", err)
	}

	pi := &ProcessInfo{
		pidfd:     pidfd,
		container: container,
		callback:  callback,
	}

	pm.containerIdProcessInfo[container.ID()] = pi
	pm.pidfdProcessInfo[pidfd] = pi

	return nil
}

func (pm *EpollProcessMonitor) DeleteProcess(container *Container) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	log.Debugf(context.Background(), "DeleteMonitoringProcess: container %s", container.ID())

	return pm.deleteProcess(container)
}

func (pm *EpollProcessMonitor) deleteProcess(container *Container) error {
	pi, exists := pm.containerIdProcessInfo[container.ID()]
	if !exists {
		return fmt.Errorf("container %d not found", container.ID())
	}

	if err := syscall.EpollCtl(pm.epfd, syscall.EPOLL_CTL_DEL, pi.pidfd, nil); err != nil {
		return fmt.Errorf("failed to remove pidfd from epoll: %w", err)
	}

	delete(pm.containerIdProcessInfo, container.ID())
	delete(pm.pidfdProcessInfo, pi.pidfd)

	return nil
}

// Close stops the monitor and releases resources.
func (pm *EpollProcessMonitor) Close() error {
	// Signal the monitoring goroutine to stop
	close(pm.stopChan)

	// Close all pidfd's
	pm.mu.Lock()
	for pidfd, pi := range pm.pidfdProcessInfo {
		err := pm.deleteProcess(pi.container)
		if err != nil {
			return fmt.Errorf("failed to delete process from pidfds map: %w", err)
		}
		err = syscall.Close(pidfd)
		if err != nil {
			return fmt.Errorf("failed to close pidfd: %w", err)
		}
		delete(pm.pidfdProcessInfo, pidfd)
		delete(pm.containerIdProcessInfo, pi.container.ID())
	}
	pm.mu.Unlock()

	// Close the epoll instance
	return syscall.Close(pm.epfd)
}

func (pm *EpollProcessMonitor) handleExit(ctx context.Context, pi *ProcessInfo) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	err := pm.deleteProcess(pi.container)
	if err != nil {
		log.Errorf(ctx, "failed to delete process from pidfds map: %v", err)
		return
	}

	pi.callback(ctx, pi.container)
}
