//go:build seccomp && linux && cgo

package seccomp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	json "github.com/goccy/go-json"
	"github.com/opencontainers/runtime-spec/specs-go"
	libseccomp "github.com/seccomp/libseccomp-golang"
	"go.podman.io/common/pkg/seccomp"
	"golang.org/x/sys/unix"

	"github.com/cri-o/cri-o/internal/log"
	v2 "github.com/cri-o/cri-o/pkg/annotations/v2"
)

// Notifier wraps a seccomp notifier instance for a container.
type Notifier struct {
	listener       net.Listener
	syscalls       sync.Map
	timer          *time.Timer
	timeLock       sync.Mutex
	stopContainers bool
}

// StopContainers returns if the notifier should stop containers or not.
func (n *Notifier) StopContainers() bool {
	return n.stopContainers
}

// Close can be used to close the notifier listener.
func (n *Notifier) Close() error {
	return n.listener.Close()
}

// AddSyscall can be used to add a syscall to the notifier result.
func (n *Notifier) AddSyscall(syscall string) {
	initValue := uint64(1)
	if count, loaded := n.syscalls.LoadOrStore(syscall, &initValue); loaded {
		if c, ok := count.(*uint64); ok {
			atomic.AddUint64(c, 1)
		}
	}
}

// UsedSyscalls returns a string representation of the used syscalls, sorted by
// their name.
func (n *Notifier) UsedSyscalls() string {
	res := []string{}
	for syscall, count := range n.syscalls.Range {
		s, syscallOk := syscall.(string)
		c, countOk := count.(*uint64)
		if syscallOk && countOk {
			res = append(res, fmt.Sprintf("%s (%dx)", s, *c))
		}
	}
	sort.Strings(res)
	return strings.Join(res, ", ")
}

// OnExpired calls the provided callback if the internal timer has been
// expired. It refreshes the timer for each call of this method.
func (n *Notifier) OnExpired(callback func()) {
	n.timeLock.Lock()
	defer n.timeLock.Unlock()
	const duration = 5 * time.Second

	if n.timer == nil || n.timer.Stop() {
		n.timer = time.AfterFunc(duration, callback)
	}
}

// Notification is a seccomp notification which gets sent to the CRI-O server.
type Notification struct {
	ctx                  context.Context
	containerID, syscall string
}

// Ctx returns the context of the notification.
func (n *Notification) Ctx() context.Context {
	return n.ctx
}

// ContainerID returns the container identifier for the notification.
func (n *Notification) ContainerID() string {
	return n.containerID
}

// Syscall returns the syscall name for the notification.
func (n *Notification) Syscall() string {
	return n.syscall
}

func (c *Config) injectNotifier(
	ctx context.Context,
	msgChan chan Notification,
	containerID string,
	sandboxAnnotations map[string]string,
	profile *specs.LinuxSeccomp,
) (*Notifier, error) {
	// Skip on sandboxes (containerID empty) and additionally gate this feature
	// by an allowed annotation.
	if containerID == "" || sandboxAnnotations == nil || msgChan == nil {
		return nil, nil
	}
	if _, ok := v2.GetAnnotationValue(sandboxAnnotations, v2.SeccompNotifierAction); !ok {
		return nil, nil
	}

	log.Infof(ctx, "Injecting seccomp notifier into seccomp profile of container %s", containerID)

	isActionToOverride := func(action specs.LinuxSeccompAction) bool {
		if action == specs.ActErrno ||
			action == specs.ActKill ||
			action == specs.ActKillProcess ||
			action == specs.ActKillThread {
			return true
		}

		return false
	}

	if isActionToOverride(profile.DefaultAction) {
		log.Infof(
			ctx,
			"The seccomp profile default action %s cannot be overridden to %s, "+
				"which means that syscalls using that default action can't be "+
				"traced by the notifier",
			profile.DefaultAction, seccomp.ActNotify,
		)
	}

	for i, syscall := range profile.Syscalls {
		if isActionToOverride(syscall.Action) {
			profile.Syscalls[i].Action = specs.ActNotify
			profile.Syscalls[i].ErrnoRet = nil
		}
	}

	profile.ListenerPath = filepath.Join(c.NotifierPath(), containerID)

	notifier, err := NewNotifier(ctx, msgChan, containerID, profile.ListenerPath, sandboxAnnotations)
	if err != nil {
		return nil, fmt.Errorf("unable to run notifier: %w", err)
	}

	return notifier, nil
}

// NewNotifier starts the notifier for the provided arguments.
func NewNotifier(
	ctx context.Context,
	msgChan chan Notification,
	containerID, listenerPath string,
	annotationMap map[string]string,
) (*Notifier, error) {
	log.Infof(ctx, "Waiting for seccomp file descriptor on container %s", containerID)
	listener, err := net.Listen("unix", listenerPath)
	if err != nil {
		return nil, fmt.Errorf("listen for seccomp socket: %w", err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if errors.Is(err, net.ErrClosed) {
				log.Infof(ctx, "Stopping notifier for container %s", containerID)
				return
			}
			if err != nil {
				log.Errorf(ctx, "Unable to accept connection: %v", err)
				continue
			}
			log.Debugf(ctx, "Got new seccomp notifier connection")

			socket, err := conn.(*net.UnixConn).File()
			if err := conn.Close(); err != nil {
				log.Errorf(ctx, "Unable to close connection: %v", err)
				continue
			}
			if err != nil {
				log.Errorf(ctx, "Unable to get socket: %v", err)
				continue
			}

			newFd, err := handleNewMessage(int(socket.Fd()))
			if err := socket.Close(); err != nil {
				log.Errorf(ctx, "Unable to close socket: %v", err)
				continue
			}
			if err != nil {
				log.Errorf(ctx, "Unable to receive seccomp file descriptor: %v", err)
				continue
			}

			log.Infof(ctx, "Received new seccomp fd: %v", newFd)
			go handler(ctx, containerID, msgChan, libseccomp.ScmpFd(newFd))
		}
	}()

	action, ok := v2.GetAnnotationValue(annotationMap, v2.SeccompNotifierAction)
	if !ok {
		return nil, fmt.Errorf("%s annotation not set on container", v2.SeccompNotifierAction)
	}

	return &Notifier{
		listener:       listener,
		syscalls:       sync.Map{},
		timer:          nil,
		timeLock:       sync.Mutex{},
		stopContainers: action == v2.SeccompNotifierActionStop,
	}, nil
}

func handler(
	ctx context.Context,
	containerID string,
	msgChan chan Notification,
	fd libseccomp.ScmpFd,
) {
	defer unix.Close(int(fd))
	for {
		req, err := libseccomp.NotifReceive(fd)
		if err != nil {
			log.Errorf(ctx, "Unable to receive notification: %v", err)
			continue
		}

		syscall, err := req.Data.Syscall.GetName()
		if err != nil {
			log.Errorf(ctx, "Unable to decode syscall %v: %v", req.Data.Syscall, err)
			continue
		}

		log.Debugf(ctx,
			"Received syscall %s for container %s (pid = %d)",
			syscall, containerID, req.Pid,
		)

		msgChan <- Notification{ctx, containerID, syscall}

		resp := &libseccomp.ScmpNotifResp{
			ID:    req.ID,
			Error: int32(unix.ENOSYS),
			Val:   uint64(0), // -1
			Flags: 0,
		}

		// TOCTOU check
		if err := libseccomp.NotifIDValid(fd, req.ID); err != nil {
			log.Errorf(ctx, "TOCTOU check failed: req.ID is no longer valid: %v", err)
			continue
		}

		if err = libseccomp.NotifRespond(fd, resp); err != nil {
			log.Errorf(ctx, "Unable to send notification response: %v", err)
			continue
		}

		// We only catch the first syscall
		break
	}
}

func handleNewMessage(sockfd int) (uintptr, error) {
	const maxNameLen = 16384
	stateBuf := make([]byte, maxNameLen)
	oobSpace := unix.CmsgSpace(4)
	oob := make([]byte, oobSpace)

	n, oobn, _, _, err := unix.Recvmsg(sockfd, stateBuf, oob, 0)
	if err != nil {
		return 0, err
	}
	if n >= maxNameLen || oobn != oobSpace {
		return 0, fmt.Errorf(
			"recvfd: incorrect number of bytes read (n=%d, maxNameLen=%d, oobn=%d, oobSpace=%d)",
			n, maxNameLen, oobn, oobSpace,
		)
	}

	// Truncate.
	stateBuf = stateBuf[:n]
	oob = oob[:oobn]

	scms, err := unix.ParseSocketControlMessage(oob)
	if err != nil {
		return 0, err
	}
	if len(scms) != 1 {
		return 0, fmt.Errorf("recvfd: number of SCMs is not 1: %d", len(scms))
	}
	scm := scms[0]

	fds, err := unix.ParseUnixRights(&scm)
	if err != nil {
		return 0, err
	}

	containerProcessState := &specs.ContainerProcessState{}
	if err := json.Unmarshal(stateBuf, containerProcessState); err != nil {
		closeStateFds(fds)
		return 0, fmt.Errorf("cannot parse OCI state: %w", err)
	}

	fd, err := parseStateFds(containerProcessState.Fds, fds)
	if err != nil {
		closeStateFds(fds)
		return 0, err
	}

	return fd, nil
}

func closeStateFds(recvFds []int) {
	for i := range recvFds {
		unix.Close(i)
	}
}

// parseStateFds returns the seccomp-fd and closes the rest of the fds in recvFds.
// In case of error, no fd is closed.
// StateFds is assumed to be formatted as specs.ContainerProcessState.Fds and
// recvFds the corresponding list of received fds in the same SCM_RIGHT message.
func parseStateFds(stateFds []string, recvFds []int) (uintptr, error) {
	// Let's find the index in stateFds of the seccomp-fd.
	idx := -1
	err := false

	for i, name := range stateFds {
		if name == specs.SeccompFdName && idx == -1 {
			idx = i
			continue
		}

		// We found the seccompFdName twice. Error out!
		if name == specs.SeccompFdName && idx != -1 {
			err = true
		}
	}

	if idx == -1 || err {
		return 0, errors.New("seccomp fd not found or malformed containerProcessState.Fds")
	}

	if idx >= len(recvFds) || idx < 0 {
		return 0, errors.New("seccomp fd index out of range")
	}

	fd := uintptr(recvFds[idx])

	for i := range recvFds {
		if i == idx {
			continue
		}

		unix.Close(recvFds[i])
	}

	return fd, nil
}
