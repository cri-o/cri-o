package hcsshim

import (
	"encoding/json"
	"runtime"
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"
)

var (
	defaultTimeout = time.Minute * 4
)

const (
	pendingUpdatesQuery = `{ "PropertyTypes" : ["PendingUpdates"]}`
	statisticsQuery     = `{ "PropertyTypes" : ["Statistics"]}`
	processListQuery    = `{ "PropertyTypes" : ["ProcessList"]}`
)

type container struct {
	handle         hcsSystem
	id             string
	callbackNumber uintptr
}

type containerProperties struct {
	ID                string `json:"Id"`
	Name              string
	SystemType        string
	Owner             string
	SiloGUID          string            `json:"SiloGuid,omitempty"`
	IsDummy           bool              `json:",omitempty"`
	RuntimeID         string            `json:"RuntimeId,omitempty"`
	Stopped           bool              `json:",omitempty"`
	ExitType          string            `json:",omitempty"`
	AreUpdatesPending bool              `json:",omitempty"`
	ObRoot            string            `json:",omitempty"`
	Statistics        Statistics        `json:",omitempty"`
	ProcessList       []ProcessListItem `json:",omitempty"`
}

// MemoryStats holds the memory statistics for a container
type MemoryStats struct {
	UsageCommitBytes            uint64 `json:"MemoryUsageCommitBytes,omitempty"`
	UsageCommitPeakBytes        uint64 `json:"MemoryUsageCommitPeakBytes,omitempty"`
	UsagePrivateWorkingSetBytes uint64 `json:"MemoryUsagePrivateWorkingSetBytes,omitempty"`
}

// ProcessorStats holds the processor statistics for a container
type ProcessorStats struct {
	TotalRuntime100ns  uint64 `json:",omitempty"`
	RuntimeUser100ns   uint64 `json:",omitempty"`
	RuntimeKernel100ns uint64 `json:",omitempty"`
}

// StorageStats holds the storage statistics for a container
type StorageStats struct {
	ReadCountNormalized  uint64 `json:",omitempty"`
	ReadSizeBytes        uint64 `json:",omitempty"`
	WriteCountNormalized uint64 `json:",omitempty"`
	WriteSizeBytes       uint64 `json:",omitempty"`
}

// NetworkStats holds the network statistics for a container
type NetworkStats struct {
	BytesReceived          uint64 `json:",omitempty"`
	BytesSent              uint64 `json:",omitempty"`
	PacketsReceived        uint64 `json:",omitempty"`
	PacketsSent            uint64 `json:",omitempty"`
	DroppedPacketsIncoming uint64 `json:",omitempty"`
	DroppedPacketsOutgoing uint64 `json:",omitempty"`
	EndpointId             string `json:",omitempty"`
	InstanceId             string `json:",omitempty"`
}

// Statistics is the structure returned by a statistics call on a container
type Statistics struct {
	Timestamp          time.Time      `json:",omitempty"`
	ContainerStartTime time.Time      `json:",omitempty"`
	Uptime100ns        uint64         `json:",omitempty"`
	Memory             MemoryStats    `json:",omitempty"`
	Processor          ProcessorStats `json:",omitempty"`
	Storage            StorageStats   `json:",omitempty"`
	Network            []NetworkStats `json:",omitempty"`
}

// ProcessList is the structure of an item returned by a ProcessList call on a container
type ProcessListItem struct {
	CreateTimestamp              time.Time `json:",omitempty"`
	ImageName                    string    `json:",omitempty"`
	KernelTime100ns              uint64    `json:",omitempty"`
	MemoryCommitBytes            uint64    `json:",omitempty"`
	MemoryWorkingSetPrivateBytes uint64    `json:",omitempty"`
	MemoryWorkingSetSharedBytes  uint64    `json:",omitempty"`
	ProcessId                    uint32    `json:",omitempty"`
	UserTime100ns                uint64    `json:",omitempty"`
}

// CreateContainer creates a new container with the given configuration but does not start it.
func CreateContainer(id string, c *ContainerConfig) (Container, error) {
	operation := "CreateContainer"
	title := "HCSShim::" + operation

	container := &container{
		id: id,
	}

	configurationb, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}

	configuration := string(configurationb)
	logrus.Debugf(title+" id=%s config=%s", id, configuration)

	var (
		resultp     *uint16
		createError error
	)
	if hcsCallbacksSupported {
		var identity syscall.Handle
		createError = hcsCreateComputeSystem(id, configuration, identity, &container.handle, &resultp)

		if createError == nil || IsPending(createError) {
			if err := container.registerCallback(); err != nil {
				return nil, makeContainerError(container, operation, "", err)
			}
		}
	} else {
		createError = hcsCreateComputeSystemTP5(id, configuration, &container.handle, &resultp)
	}

	err = processAsyncHcsResult(createError, resultp, container.callbackNumber, hcsNotificationSystemCreateCompleted, &defaultTimeout)
	if err != nil {
		return nil, makeContainerError(container, operation, configuration, err)
	}

	logrus.Debugf(title+" succeeded id=%s handle=%d", id, container.handle)
	runtime.SetFinalizer(container, closeContainer)
	return container, nil
}

// OpenContainer opens an existing container by ID.
func OpenContainer(id string) (Container, error) {
	operation := "OpenContainer"
	title := "HCSShim::" + operation
	logrus.Debugf(title+" id=%s", id)

	container := &container{
		id: id,
	}

	var (
		handle  hcsSystem
		resultp *uint16
	)
	err := hcsOpenComputeSystem(id, &handle, &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		return nil, makeContainerError(container, operation, "", err)
	}

	container.handle = handle

	logrus.Debugf(title+" succeeded id=%s handle=%d", id, handle)
	runtime.SetFinalizer(container, closeContainer)
	return container, nil
}

// Start synchronously starts the container.
func (container *container) Start() error {
	operation := "Start"
	title := "HCSShim::Container::" + operation
	logrus.Debugf(title+" id=%s", container.id)

	var resultp *uint16
	err := hcsStartComputeSystemTP5(container.handle, nil, &resultp)
	err = processAsyncHcsResult(err, resultp, container.callbackNumber, hcsNotificationSystemStartCompleted, &defaultTimeout)
	if err != nil {
		return makeContainerError(container, operation, "", err)
	}

	logrus.Debugf(title+" succeeded id=%s", container.id)
	return nil
}

// Shutdown requests a container shutdown, if IsPending() on the error returned is true,
// it may not actually be shut down until Wait() succeeds.
func (container *container) Shutdown() error {
	operation := "Shutdown"
	title := "HCSShim::Container::" + operation
	logrus.Debugf(title+" id=%s", container.id)

	var resultp *uint16
	err := hcsShutdownComputeSystemTP5(container.handle, nil, &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		return makeContainerError(container, operation, "", err)
	}

	logrus.Debugf(title+" succeeded id=%s", container.id)
	return nil
}

// Terminate requests a container terminate, if IsPending() on the error returned is true,
// it may not actually be shut down until Wait() succeeds.
func (container *container) Terminate() error {
	operation := "Terminate"
	title := "HCSShim::Container::" + operation
	logrus.Debugf(title+" id=%s", container.id)

	var resultp *uint16
	err := hcsTerminateComputeSystemTP5(container.handle, nil, &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		return makeContainerError(container, operation, "", err)
	}

	logrus.Debugf(title+" succeeded id=%s", container.id)
	return nil
}

// Wait synchronously waits for the container to shutdown or terminate.
func (container *container) Wait() error {
	operation := "Wait"
	title := "HCSShim::Container::" + operation
	logrus.Debugf(title+" id=%s", container.id)

	if hcsCallbacksSupported {
		err := waitForNotification(container.callbackNumber, hcsNotificationSystemExited, nil)
		if err != nil {
			return makeContainerError(container, operation, "", err)
		}
	} else {
		_, err := container.waitTimeoutInternal(syscall.INFINITE)
		if err != nil {
			return makeContainerError(container, operation, "", err)
		}
	}

	logrus.Debugf(title+" succeeded id=%s", container.id)
	return nil
}

func (container *container) waitTimeoutInternal(timeout uint32) (bool, error) {
	return waitTimeoutInternalHelper(container, timeout)
}

// WaitTimeout synchronously waits for the container to terminate or the duration to elapse.
// If the timeout expires, IsTimeout(err) == true
func (container *container) WaitTimeout(timeout time.Duration) error {
	operation := "WaitTimeout"
	title := "HCSShim::Container::" + operation
	logrus.Debugf(title+" id=%s", container.id)

	if hcsCallbacksSupported {
		err := waitForNotification(container.callbackNumber, hcsNotificationSystemExited, &timeout)
		if err != nil {
			return makeContainerError(container, operation, "", err)
		}
	} else {
		finished, err := waitTimeoutHelper(container, timeout)
		if !finished {
			err = ErrTimeout
		}
		if err != nil {
			return makeContainerError(container, operation, "", err)
		}
	}

	logrus.Debugf(title+" succeeded id=%s", container.id)
	return nil
}

func (container *container) hcsWait(timeout uint32) (bool, error) {
	var (
		resultp   *uint16
		exitEvent syscall.Handle
	)

	err := hcsCreateComputeSystemWait(container.handle, &exitEvent, &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		return false, err
	}
	defer syscall.CloseHandle(exitEvent)

	return waitForSingleObject(exitEvent, timeout)
}

func (container *container) properties(query string) (*containerProperties, error) {
	var (
		resultp     *uint16
		propertiesp *uint16
	)
	err := hcsGetComputeSystemProperties(container.handle, query, &propertiesp, &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		return nil, err
	}

	if propertiesp == nil {
		return nil, ErrUnexpectedValue
	}
	propertiesRaw := convertAndFreeCoTaskMemBytes(propertiesp)
	properties := &containerProperties{}
	if err := json.Unmarshal(propertiesRaw, properties); err != nil {
		return nil, err
	}
	return properties, nil
}

// HasPendingUpdates returns true if the container has updates pending to install
func (container *container) HasPendingUpdates() (bool, error) {
	operation := "HasPendingUpdates"
	title := "HCSShim::Container::" + operation
	logrus.Debugf(title+" id=%s", container.id)
	properties, err := container.properties(pendingUpdatesQuery)
	if err != nil {
		return false, makeContainerError(container, operation, "", err)
	}

	logrus.Debugf(title+" succeeded id=%s", container.id)
	return properties.AreUpdatesPending, nil
}

// Statistics returns statistics for the container
func (container *container) Statistics() (Statistics, error) {
	operation := "Statistics"
	title := "HCSShim::Container::" + operation
	logrus.Debugf(title+" id=%s", container.id)
	properties, err := container.properties(statisticsQuery)
	if err != nil {
		return Statistics{}, makeContainerError(container, operation, "", err)
	}

	logrus.Debugf(title+" succeeded id=%s", container.id)
	return properties.Statistics, nil
}

// ProcessList returns an array of ProcessListItems for the container
func (container *container) ProcessList() ([]ProcessListItem, error) {
	operation := "ProcessList"
	title := "HCSShim::Container::" + operation
	logrus.Debugf(title+" id=%s", container.id)
	properties, err := container.properties(processListQuery)
	if err != nil {
		return nil, makeContainerError(container, operation, "", err)
	}

	logrus.Debugf(title+" succeeded id=%s", container.id)
	return properties.ProcessList, nil
}

// Pause pauses the execution of the container. This feature is not enabled in TP5.
func (container *container) Pause() error {
	operation := "Pause"
	title := "HCSShim::Container::" + operation
	logrus.Debugf(title+" id=%s", container.id)

	var resultp *uint16
	err := hcsPauseComputeSystemTP5(container.handle, nil, &resultp)
	err = processAsyncHcsResult(err, resultp, container.callbackNumber, hcsNotificationSystemPauseCompleted, &defaultTimeout)
	if err != nil {
		return makeContainerError(container, operation, "", err)
	}

	logrus.Debugf(title+" succeeded id=%s", container.id)
	return nil
}

// Resume resumes the execution of the container. This feature is not enabled in TP5.
func (container *container) Resume() error {
	operation := "Resume"
	title := "HCSShim::Container::" + operation
	logrus.Debugf(title+" id=%s", container.id)
	var (
		resultp *uint16
	)

	err := hcsResumeComputeSystemTP5(container.handle, nil, &resultp)
	err = processAsyncHcsResult(err, resultp, container.callbackNumber, hcsNotificationSystemResumeCompleted, &defaultTimeout)
	if err != nil {
		return makeContainerError(container, operation, "", err)
	}

	logrus.Debugf(title+" succeeded id=%s", container.id)
	return nil
}

// CreateProcess launches a new process within the container.
func (container *container) CreateProcess(c *ProcessConfig) (Process, error) {
	operation := "CreateProcess"
	title := "HCSShim::Container::" + operation
	var (
		processInfo   hcsProcessInformation
		processHandle hcsProcess
		resultp       *uint16
	)

	// If we are not emulating a console, ignore any console size passed to us
	if !c.EmulateConsole {
		c.ConsoleSize[0] = 0
		c.ConsoleSize[1] = 0
	}

	configurationb, err := json.Marshal(c)
	if err != nil {
		return nil, makeContainerError(container, operation, "", err)
	}

	configuration := string(configurationb)
	logrus.Debugf(title+" id=%s config=%s", container.id, configuration)

	err = hcsCreateProcess(container.handle, configuration, &processInfo, &processHandle, &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		return nil, makeContainerError(container, operation, configuration, err)
	}

	process := &process{
		handle:    processHandle,
		processID: int(processInfo.ProcessId),
		container: container,
		cachedPipes: &cachedPipes{
			stdIn:  processInfo.StdInput,
			stdOut: processInfo.StdOutput,
			stdErr: processInfo.StdError,
		},
	}

	if hcsCallbacksSupported {
		if err := process.registerCallback(); err != nil {
			return nil, makeContainerError(container, operation, "", err)
		}
	}

	logrus.Debugf(title+" succeeded id=%s processid=%s", container.id, process.processID)
	runtime.SetFinalizer(process, closeProcess)
	return process, nil
}

// OpenProcess gets an interface to an existing process within the container.
func (container *container) OpenProcess(pid int) (Process, error) {
	operation := "OpenProcess"
	title := "HCSShim::Container::" + operation
	logrus.Debugf(title+" id=%s, processid=%d", container.id, pid)
	var (
		processHandle hcsProcess
		resultp       *uint16
	)

	err := hcsOpenProcess(container.handle, uint32(pid), &processHandle, &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		return nil, makeContainerError(container, operation, "", err)
	}

	process := &process{
		handle:    processHandle,
		processID: pid,
		container: container,
	}

	if hcsCallbacksSupported {
		if err := process.registerCallback(); err != nil {
			return nil, makeContainerError(container, operation, "", err)
		}
	}

	logrus.Debugf(title+" succeeded id=%s processid=%s", container.id, process.processID)
	runtime.SetFinalizer(process, closeProcess)
	return process, nil
}

// Close cleans up any state associated with the container but does not terminate or wait for it.
func (container *container) Close() error {
	operation := "Close"
	title := "HCSShim::Container::" + operation
	logrus.Debugf(title+" id=%s", container.id)

	// Don't double free this
	if container.handle == 0 {
		return nil
	}

	if hcsCallbacksSupported {
		if err := container.unregisterCallback(); err != nil {
			return makeContainerError(container, operation, "", err)
		}
	}

	if err := hcsCloseComputeSystem(container.handle); err != nil {
		return makeContainerError(container, operation, "", err)
	}

	container.handle = 0

	logrus.Debugf(title+" succeeded id=%s", container.id)
	return nil
}

// closeContainer wraps container.Close for use by a finalizer
func closeContainer(container *container) {
	container.Close()
}

func (container *container) registerCallback() error {
	context := &notifcationWatcherContext{
		channels: newChannels(),
	}

	callbackMapLock.Lock()
	callbackNumber := nextCallback
	nextCallback++
	callbackMap[callbackNumber] = context
	callbackMapLock.Unlock()

	var callbackHandle hcsCallback
	err := hcsRegisterComputeSystemCallback(container.handle, notificationWatcherCallback, callbackNumber, &callbackHandle)
	if err != nil {
		return err
	}
	context.handle = callbackHandle
	container.callbackNumber = callbackNumber

	return nil
}

func (container *container) unregisterCallback() error {
	callbackNumber := container.callbackNumber

	callbackMapLock.RLock()
	context := callbackMap[callbackNumber]
	callbackMapLock.RUnlock()

	if context == nil {
		return nil
	}

	handle := context.handle

	if handle == 0 {
		return nil
	}

	// hcsUnregisterComputeSystemCallback has its own syncronization
	// to wait for all callbacks to complete. We must NOT hold the callbackMapLock.
	err := hcsUnregisterComputeSystemCallback(handle)
	if err != nil {
		return err
	}

	closeChannels(context.channels)

	callbackMapLock.Lock()
	callbackMap[callbackNumber] = nil
	callbackMapLock.Unlock()

	handle = 0

	return nil
}
