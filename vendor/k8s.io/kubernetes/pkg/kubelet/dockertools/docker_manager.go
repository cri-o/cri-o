/*
Copyright 2015 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package dockertools

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/armon/circbuf"
	dockertypes "github.com/docker/engine-api/types"
	dockercontainer "github.com/docker/engine-api/types/container"
	dockerstrslice "github.com/docker/engine-api/types/strslice"
	dockerapiversion "github.com/docker/engine-api/types/versions"
	dockernat "github.com/docker/go-connections/nat"
	"github.com/golang/glog"
	cadvisorapi "github.com/google/cadvisor/info/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubetypes "k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/kubelet/cm"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	dockersecurity "k8s.io/kubernetes/pkg/kubelet/dockertools/securitycontext"
	"k8s.io/kubernetes/pkg/kubelet/events"
	"k8s.io/kubernetes/pkg/kubelet/images"
	"k8s.io/kubernetes/pkg/kubelet/lifecycle"
	"k8s.io/kubernetes/pkg/kubelet/metrics"
	knetwork "k8s.io/kubernetes/pkg/kubelet/network"
	"k8s.io/kubernetes/pkg/kubelet/network/hairpin"
	proberesults "k8s.io/kubernetes/pkg/kubelet/prober/results"
	"k8s.io/kubernetes/pkg/kubelet/qos"
	"k8s.io/kubernetes/pkg/kubelet/types"
	"k8s.io/kubernetes/pkg/kubelet/util/cache"
	"k8s.io/kubernetes/pkg/kubelet/util/format"
	"k8s.io/kubernetes/pkg/security/apparmor"
	"k8s.io/kubernetes/pkg/securitycontext"
	"k8s.io/kubernetes/pkg/util/oom"
	"k8s.io/kubernetes/pkg/util/procfs"
	"k8s.io/kubernetes/pkg/util/selinux"
	utilstrings "k8s.io/kubernetes/pkg/util/strings"
	"k8s.io/kubernetes/pkg/util/tail"
	"k8s.io/kubernetes/pkg/util/term"
	utilversion "k8s.io/kubernetes/pkg/util/version"
)

const (
	DockerType                 = "docker"
	dockerDefaultLoggingDriver = "json-file"

	// https://docs.docker.com/engine/reference/api/docker_remote_api/
	// docker version should be at least 1.10.x
	minimumDockerAPIVersion = "1.22"

	// Remote API version for docker daemon versions
	// https://docs.docker.com/engine/reference/api/docker_remote_api/
	dockerV110APIVersion = "1.22"

	// ndots specifies the minimum number of dots that a domain name must contain for the resolver to consider it as FQDN (fully-qualified)
	// we want to able to consider SRV lookup names like _dns._udp.kube-dns.default.svc to be considered relative.
	// hence, setting ndots to be 5.
	ndotsDNSOption = "options ndots:5\n"
	// In order to avoid unnecessary SIGKILLs, give every container a minimum grace
	// period after SIGTERM. Docker will guarantee the termination, but SIGTERM is
	// potentially dangerous.
	// TODO: evaluate whether there are scenarios in which SIGKILL is preferable to
	// SIGTERM for certain process types, which may justify setting this to 0.
	minimumGracePeriodInSeconds = 2

	DockerNetnsFmt = "/proc/%v/ns/net"

	// String used to detect docker host mode for various namespaces (e.g.
	// networking). Must match the value returned by docker inspect -f
	// '{{.HostConfig.NetworkMode}}'.
	namespaceModeHost = "host"

	// The expiration time of version cache.
	versionCacheTTL = 60 * time.Second

	// Docker changed the API for specifying options in v1.11
	SecurityOptSeparatorChangeVersion = "1.23.0" // Corresponds to docker 1.11.x
	SecurityOptSeparatorOld           = ':'
	SecurityOptSeparatorNew           = '='
)

var (
	// DockerManager implements the Runtime and DirectStreamingRuntime interfaces.
	_ kubecontainer.Runtime                = &DockerManager{}
	_ kubecontainer.DirectStreamingRuntime = &DockerManager{}

	// TODO: make this a TTL based pull (if image older than X policy, pull)
	podInfraContainerImagePullPolicy = v1.PullIfNotPresent

	// Default set of seccomp security options.
	defaultSeccompOpt = []dockerOpt{{"seccomp", "unconfined", ""}}
)

type DockerManager struct {
	client              DockerInterface
	recorder            record.EventRecorder
	containerRefManager *kubecontainer.RefManager
	os                  kubecontainer.OSInterface
	machineInfo         *cadvisorapi.MachineInfo

	// The image name of the pod infra container.
	podInfraContainerImage string
	// (Optional) Additional environment variables to be set for the pod infra container.
	podInfraContainerEnv []v1.EnvVar

	// TODO(yifan): Record the pull failure so we can eliminate the image checking?
	// Lower level docker image puller.
	dockerPuller DockerPuller

	// wrapped image puller.
	imagePuller images.ImageManager

	// cgroup driver used by Docker runtime.
	cgroupDriver string

	// Directory of container logs.
	containerLogsDir string

	// Network plugin manager.
	network *knetwork.PluginManager

	// Health check results.
	livenessManager proberesults.Manager

	// RuntimeHelper that wraps kubelet to generate runtime container options.
	runtimeHelper kubecontainer.RuntimeHelper

	// Runner of lifecycle events.
	runner kubecontainer.HandlerRunner

	// Handler used to execute commands in containers.
	execHandler ExecHandler

	// Used to set OOM scores of processes.
	oomAdjuster *oom.OOMAdjuster

	// Get information from /proc mount.
	procFs procfs.ProcFSInterface

	// If true, enforce container cpu limits with CFS quota support
	cpuCFSQuota bool

	// Container GC manager
	containerGC *containerGC

	// Support for gathering custom metrics.
	enableCustomMetrics bool

	// If true, the "hairpin mode" flag is set on container interfaces.
	// A false value means the kubelet just backs off from setting it,
	// it might already be true.
	configureHairpinMode bool

	// Provides image stats
	*imageStatsProvider

	// The version cache of docker daemon.
	versionCache *cache.ObjectCache

	// Directory to host local seccomp profiles.
	seccompProfileRoot string
}

// A subset of the pod.Manager interface extracted for testing purposes.
type podGetter interface {
	GetPodByUID(kubetypes.UID) (*v1.Pod, bool)
}

func PodInfraContainerEnv(env map[string]string) kubecontainer.Option {
	return func(rt kubecontainer.Runtime) {
		dm := rt.(*DockerManager)
		for k, v := range env {
			dm.podInfraContainerEnv = append(dm.podInfraContainerEnv, v1.EnvVar{
				Name:  k,
				Value: v,
			})
		}
	}
}

func NewDockerManager(
	client DockerInterface,
	recorder record.EventRecorder,
	livenessManager proberesults.Manager,
	containerRefManager *kubecontainer.RefManager,
	podGetter podGetter,
	machineInfo *cadvisorapi.MachineInfo,
	podInfraContainerImage string,
	qps float32,
	burst int,
	containerLogsDir string,
	osInterface kubecontainer.OSInterface,
	networkPlugin knetwork.NetworkPlugin,
	runtimeHelper kubecontainer.RuntimeHelper,
	httpClient types.HttpGetter,
	execHandler ExecHandler,
	oomAdjuster *oom.OOMAdjuster,
	procFs procfs.ProcFSInterface,
	cpuCFSQuota bool,
	imageBackOff *flowcontrol.Backoff,
	serializeImagePulls bool,
	enableCustomMetrics bool,
	hairpinMode bool,
	seccompProfileRoot string,
	options ...kubecontainer.Option) *DockerManager {
	// Wrap the docker client with instrumentedDockerInterface
	client = NewInstrumentedDockerInterface(client)

	// cgroup driver is only detectable in docker 1.11+
	// when the execution driver is not detectable, we provide the cgroupfs form.
	// if your docker engine is configured to use the systemd cgroup driver, and you
	// want to use pod level cgroups, you must be on docker 1.12+ to ensure cgroup-parent
	// is converted appropriately.  otherwise, docker will fail to launch the container
	// and complain the cgroup name provided did not conform to systemd conventions.
	var cgroupDriver string
	dockerInfo, err := client.Info()
	if err != nil {
		glog.Errorf("Failed to execute Info() call to the Docker client: %v", err)
	} else {
		cgroupDriver = dockerInfo.CgroupDriver
		glog.Infof("Setting cgroupDriver to %s", cgroupDriver)
	}

	dm := &DockerManager{
		client:                 client,
		recorder:               recorder,
		containerRefManager:    containerRefManager,
		os:                     osInterface,
		machineInfo:            machineInfo,
		podInfraContainerImage: podInfraContainerImage,
		dockerPuller:           newDockerPuller(client),
		cgroupDriver:           cgroupDriver,
		containerLogsDir:       containerLogsDir,
		network:                knetwork.NewPluginManager(networkPlugin),
		livenessManager:        livenessManager,
		runtimeHelper:          runtimeHelper,
		execHandler:            execHandler,
		oomAdjuster:            oomAdjuster,
		procFs:                 procFs,
		cpuCFSQuota:            cpuCFSQuota,
		enableCustomMetrics:    enableCustomMetrics,
		configureHairpinMode:   hairpinMode,
		imageStatsProvider:     newImageStatsProvider(client),
		seccompProfileRoot:     seccompProfileRoot,
	}
	cmdRunner := kubecontainer.DirectStreamingRunner(dm)
	dm.runner = lifecycle.NewHandlerRunner(httpClient, cmdRunner, dm)
	dm.imagePuller = images.NewImageManager(kubecontainer.FilterEventRecorder(recorder), dm, imageBackOff, serializeImagePulls, qps, burst)
	dm.containerGC = NewContainerGC(client, podGetter, dm.network, containerLogsDir)

	dm.versionCache = cache.NewObjectCache(
		func() (interface{}, error) {
			return dm.getVersionInfo()
		},
		versionCacheTTL,
	)

	// apply optional settings..
	for _, optf := range options {
		optf(dm)
	}

	return dm
}

// GetContainerLogs returns logs of a specific container. By
// default, it returns a snapshot of the container log. Set 'follow' to true to
// stream the log. Set 'follow' to false and specify the number of lines (e.g.
// "100" or "all") to tail the log.
// TODO: Make 'RawTerminal' option  flagable.
func (dm *DockerManager) GetContainerLogs(pod *v1.Pod, containerID kubecontainer.ContainerID, logOptions *v1.PodLogOptions, stdout, stderr io.Writer) error {
	container, err := dm.client.InspectContainer(containerID.ID)
	if err != nil {
		return err
	}
	return GetContainerLogs(dm.client, pod, containerID, logOptions, stdout, stderr, container.Config.Tty)
}

// Temporarily export this function to share with dockershim.
// TODO: clean this up.
func GetContainerLogs(client DockerInterface, pod *v1.Pod, containerID kubecontainer.ContainerID, logOptions *v1.PodLogOptions, stdout, stderr io.Writer, rawTerm bool) error {
	var since int64
	if logOptions.SinceSeconds != nil {
		t := metav1.Now().Add(-time.Duration(*logOptions.SinceSeconds) * time.Second)
		since = t.Unix()
	}
	if logOptions.SinceTime != nil {
		since = logOptions.SinceTime.Unix()
	}
	opts := dockertypes.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Since:      strconv.FormatInt(since, 10),
		Timestamps: logOptions.Timestamps,
		Follow:     logOptions.Follow,
	}
	if logOptions.TailLines != nil {
		opts.Tail = strconv.FormatInt(*logOptions.TailLines, 10)
	}

	sopts := StreamOptions{
		OutputStream: stdout,
		ErrorStream:  stderr,
		RawTerminal:  rawTerm,
	}
	return client.Logs(containerID.ID, opts, sopts)
}

var (
	// ErrContainerCannotRun is returned when a container is created, but cannot run properly
	ErrContainerCannotRun = errors.New("ContainerCannotRun")
)

// determineContainerIP determines the IP address of the given container.  It is expected
// that the container passed is the infrastructure container of a pod and the responsibility
// of the caller to ensure that the correct container is passed.
func (dm *DockerManager) determineContainerIP(podNamespace, podName string, container *dockertypes.ContainerJSON) (string, error) {
	result := getContainerIP(container)

	networkMode := getDockerNetworkMode(container)
	isHostNetwork := networkMode == namespaceModeHost

	// For host networking or default network plugin, GetPodNetworkStatus doesn't work
	if !isHostNetwork && dm.network.PluginName() != knetwork.DefaultPluginName {
		netStatus, err := dm.network.GetPodNetworkStatus(podNamespace, podName, kubecontainer.DockerID(container.ID).ContainerID())
		if err != nil {
			glog.Error(err)
			return result, err
		} else if netStatus != nil {
			result = netStatus.IP.String()
		}
	}

	return result, nil
}

func (dm *DockerManager) inspectContainer(id string, podName, podNamespace string) (*kubecontainer.ContainerStatus, string, error) {
	var ip string
	iResult, err := dm.client.InspectContainer(id)
	if err != nil {
		return nil, ip, err
	}
	glog.V(4).Infof("Container inspect result: %+v", *iResult)

	// TODO: Get k8s container name by parsing the docker name. This will be
	// replaced by checking docker labels eventually.
	dockerName, hash, err := ParseDockerName(iResult.Name)
	if err != nil {
		return nil, ip, fmt.Errorf("Unable to parse docker name %q", iResult.Name)
	}
	containerName := dockerName.ContainerName

	var containerInfo *labelledContainerInfo
	containerInfo = getContainerInfoFromLabel(iResult.Config.Labels)

	parseTimestampError := func(label, s string) {
		glog.Errorf("Failed to parse %q timestamp %q for container %q of pod %q", label, s, id, kubecontainer.BuildPodFullName(podName, podNamespace))
	}
	var createdAt, startedAt, finishedAt time.Time
	if createdAt, err = ParseDockerTimestamp(iResult.Created); err != nil {
		parseTimestampError("Created", iResult.Created)
	}
	if startedAt, err = ParseDockerTimestamp(iResult.State.StartedAt); err != nil {
		parseTimestampError("StartedAt", iResult.State.StartedAt)
	}
	if finishedAt, err = ParseDockerTimestamp(iResult.State.FinishedAt); err != nil {
		parseTimestampError("FinishedAt", iResult.State.FinishedAt)
	}

	// default to the image ID, but try and inspect for the RepoDigests
	imageID := DockerPrefix + iResult.Image
	imageName := iResult.Config.Image
	imgInspectResult, err := dm.client.InspectImageByID(iResult.Image)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("unable to inspect docker image %q while inspecting docker container %q: %v", iResult.Image, containerName, err))
	} else {
		if len(imgInspectResult.RepoDigests) > 1 {
			glog.V(4).Infof("Container %q had more than one associated RepoDigest (%v), only using the first", containerName, imgInspectResult.RepoDigests)
		}

		if len(imgInspectResult.RepoDigests) > 0 {
			imageID = DockerPullablePrefix + imgInspectResult.RepoDigests[0]
		}

		if len(imgInspectResult.RepoTags) > 0 {
			imageName = imgInspectResult.RepoTags[0]
		}
	}

	status := kubecontainer.ContainerStatus{
		Name:         containerName,
		RestartCount: containerInfo.RestartCount,
		Image:        imageName,
		ImageID:      imageID,
		ID:           kubecontainer.DockerID(id).ContainerID(),
		ExitCode:     iResult.State.ExitCode,
		CreatedAt:    createdAt,
		Hash:         hash,
	}
	if iResult.State.Running {
		// Container that are running, restarting and paused
		status.State = kubecontainer.ContainerStateRunning
		status.StartedAt = startedAt
		if containerProvidesPodIP(dockerName.ContainerName) {
			ip, err = dm.determineContainerIP(podNamespace, podName, iResult)
			// Kubelet doesn't handle the network error scenario
			if err != nil {
				status.State = kubecontainer.ContainerStateUnknown
				status.Message = fmt.Sprintf("Network error: %#v", err)
			}
		}
		return &status, ip, nil
	}

	// Find containers that have exited or failed to start.
	if !finishedAt.IsZero() || iResult.State.ExitCode != 0 {
		// Containers that are exited, dead or created (docker failed to start container)
		// When a container fails to start State.ExitCode is non-zero, FinishedAt and StartedAt are both zero
		reason := ""
		message := iResult.State.Error

		// Note: An application might handle OOMKilled gracefully.
		// In that case, the container is oom killed, but the exit
		// code could be 0.
		if iResult.State.OOMKilled {
			reason = "OOMKilled"
		} else if iResult.State.ExitCode == 0 {
			reason = "Completed"
		} else if !finishedAt.IsZero() {
			reason = "Error"
		} else {
			// finishedAt is zero and ExitCode is nonZero occurs when docker fails to start the container
			reason = ErrContainerCannotRun.Error()
			// Adjust time to the time docker attempted to run the container, otherwise startedAt and finishedAt will be set to epoch, which is misleading
			finishedAt = createdAt
			startedAt = createdAt
		}

		// retrieve the termination message from logs, file, or file with fallback to logs in case of failure
		fallbackToLogs := containerInfo.TerminationMessagePolicy == v1.TerminationMessageFallbackToLogsOnError && (iResult.State.ExitCode != 0 || iResult.State.OOMKilled)
		if msg := getTerminationMessage(dm.c, iResult, containerInfo.TerminationMessagePath, fallbackToLogs); len(msg) > 0 {
			message = msg
		}

		status.State = kubecontainer.ContainerStateExited
		status.Message = message
		status.Reason = reason
		status.StartedAt = startedAt
		status.FinishedAt = finishedAt
	} else {
		// Non-running containers that are created (not yet started or kubelet failed before calling
		// start container function etc.) Kubelet doesn't handle these scenarios yet.
		status.State = kubecontainer.ContainerStateUnknown
	}
	return &status, "", nil
}

func getTerminationMessage(c DockerInterface, iResult *dockertypes.ContainerJSON, terminationMessagePath string, fallbackToLogs bool) string {
	if len(terminationMessagePath) != 0 {
		for _, mount := range iResult.Mounts {
			if mount.Destination != terminationMessagePath {
				continue
			}
			path := mount.Source
			data, _, err := tail.ReadAtMost(path, kubecontainer.MaxContainerTerminationMessageLength)
			if err != nil {
				return fmt.Sprintf("Error on reading termination log %s: %v", path, err)
			}
			if !fallbackToLogs || len(data) != 0 {
				return string(data)
			}
		}
	}
	if !fallbackToLogs {
		return ""
	}

	return readLastStringFromContainerLogs(c, iResult.Name)
}

// readLastStringFromContainerLogs attempts to a certain amount from the end of the logs for containerName.
// It will attempt to avoid reading excessive logs from the server, which may result in underestimating the amount
// of logs to fetch (such that the length of the response message is < max).
func readLastStringFromContainerLogs(c DockerInterface, containerName string) string {
	logOptions := dockertypes.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	}
	buf, _ := circbuf.NewBuffer(kubecontainer.MaxContainerTerminationMessageLogLength)
	streamOptions := StreamOptions{
		ErrorStream:  buf,
		OutputStream: buf,
	}
	logOptions.Tail = strconv.FormatInt(kubecontainer.MaxContainerTerminationMessageLogLines, 10)
	if err := c.Logs(containerName, logOptions, streamOptions); err != nil {
		return fmt.Sprintf("Error on reading termination message from logs: %v", err)
	}
	return buf.String()
}

// makeEnvList converts EnvVar list to a list of strings, in the form of
// '<key>=<value>', which can be understood by docker.
func makeEnvList(envs []kubecontainer.EnvVar) (result []string) {
	for _, env := range envs {
		result = append(result, fmt.Sprintf("%s=%s", env.Name, env.Value))
	}
	return
}

// makeMountBindings converts the mount list to a list of strings that
// can be understood by docker.
// Each element in the string is in the form of:
// '<HostPath>:<ContainerPath>', or
// '<HostPath>:<ContainerPath>:ro', if the path is read only, or
// '<HostPath>:<ContainerPath>:Z', if the volume requires SELinux
// relabeling
func makeMountBindings(mounts []kubecontainer.Mount) (result []string) {
	for _, m := range mounts {
		bind := fmt.Sprintf("%s:%s", m.HostPath, m.ContainerPath)
		if m.ReadOnly {
			bind += ":ro"
		}
		if m.SELinuxRelabel && selinux.SELinuxEnabled() {
			if m.ReadOnly {
				bind += ",Z"
			} else {
				bind += ":Z"
			}

		}
		result = append(result, bind)
	}
	return
}

func makePortsAndBindings(portMappings []kubecontainer.PortMapping) (map[dockernat.Port]struct{}, map[dockernat.Port][]dockernat.PortBinding) {
	exposedPorts := map[dockernat.Port]struct{}{}
	portBindings := map[dockernat.Port][]dockernat.PortBinding{}
	for _, port := range portMappings {
		exteriorPort := port.HostPort
		if exteriorPort == 0 {
			// No need to do port binding when HostPort is not specified
			continue
		}
		interiorPort := port.ContainerPort
		// Some of this port stuff is under-documented voodoo.
		// See http://stackoverflow.com/questions/20428302/binding-a-port-to-a-host-interface-using-the-rest-api
		var protocol string
		switch strings.ToUpper(string(port.Protocol)) {
		case "UDP":
			protocol = "/udp"
		case "TCP":
			protocol = "/tcp"
		default:
			glog.Warningf("Unknown protocol %q: defaulting to TCP", port.Protocol)
			protocol = "/tcp"
		}

		dockerPort := dockernat.Port(strconv.Itoa(interiorPort) + protocol)
		exposedPorts[dockerPort] = struct{}{}

		hostBinding := dockernat.PortBinding{
			HostPort: strconv.Itoa(exteriorPort),
			HostIP:   port.HostIP,
		}

		// Allow multiple host ports bind to same docker port
		if existedBindings, ok := portBindings[dockerPort]; ok {
			// If a docker port already map to a host port, just append the host ports
			portBindings[dockerPort] = append(existedBindings, hostBinding)
		} else {
			// Otherwise, it's fresh new port binding
			portBindings[dockerPort] = []dockernat.PortBinding{
				hostBinding,
			}
		}
	}
	return exposedPorts, portBindings
}

func (dm *DockerManager) runContainer(
	pod *v1.Pod,
	container *v1.Container,
	opts *kubecontainer.RunContainerOptions,
	ref *v1.ObjectReference,
	imageRef string,
	netMode string,
	ipcMode string,
	utsMode string,
	pidMode string,
	restartCount int,
	oomScoreAdj int) (kubecontainer.ContainerID, error) {

	dockerName := KubeletContainerName{
		PodFullName:   kubecontainer.GetPodFullName(pod),
		PodUID:        pod.UID,
		ContainerName: container.Name,
	}

	securityOpts, err := dm.getSecurityOpts(pod, container.Name)
	if err != nil {
		return kubecontainer.ContainerID{}, err
	}
	optSeparator, err := dm.getDockerOptSeparator()
	if err != nil {
		return kubecontainer.ContainerID{}, err
	}
	fmtSecurityOpts := FmtDockerOpts(securityOpts, optSeparator)

	// Pod information is recorded on the container as labels to preserve it in the event the pod is deleted
	// while the Kubelet is down and there is no information available to recover the pod.
	// TODO: keep these labels up to date if the pod changes
	labels := newLabels(container, pod, restartCount, dm.enableCustomMetrics)

	// TODO(random-liu): Remove this when we start to use new labels for KillContainerInPod
	if container.Lifecycle != nil && container.Lifecycle.PreStop != nil {
		// TODO: This is kind of hacky, we should really just encode the bits we need.
		// TODO: This is hacky because the Kubelet should be parameterized to encode a specific version
		//   and needs to be able to migrate this whenever we deprecate v1. Should be a member of DockerManager.
		if data, err := kruntime.Encode(api.Codecs.LegacyCodec(schema.GroupVersion{Group: v1.GroupName, Version: "v1"}), pod); err == nil {
			labels[kubernetesPodLabel] = string(data)
		} else {
			glog.Errorf("Failed to encode pod: %s for prestop hook", pod.Name)
		}
	}
	memoryLimit := container.Resources.Limits.Memory().Value()
	cpuRequest := container.Resources.Requests.Cpu()
	cpuLimit := container.Resources.Limits.Cpu()
	var cpuShares int64
	// If request is not specified, but limit is, we want request to default to limit.
	// API server does this for new containers, but we repeat this logic in Kubelet
	// for containers running on existing Kubernetes clusters.
	if cpuRequest.IsZero() && !cpuLimit.IsZero() {
		cpuShares = cm.MilliCPUToShares(cpuLimit.MilliValue())
	} else {
		// if cpuRequest.Amount is nil, then milliCPUToShares will return the minimal number
		// of CPU shares.
		cpuShares = cm.MilliCPUToShares(cpuRequest.MilliValue())
	}

	// Set devices for container.
	devices := make([]dockercontainer.DeviceMapping, len(opts.Devices))
	for i, device := range opts.Devices {
		devices[i] = dockercontainer.DeviceMapping{
			PathOnHost:        device.PathOnHost,
			PathInContainer:   device.PathInContainer,
			CgroupPermissions: device.Permissions,
		}
	}
	binds := makeMountBindings(opts.Mounts)

	// The reason we create and mount the log file in here (not in kubelet) is because
	// the file's location depends on the ID of the container, and we need to create and
	// mount the file before actually starting the container.
	// TODO(yifan): Consider to pull this logic out since we might need to reuse it in
	// other container runtime.
	_, containerName, cid := BuildDockerName(dockerName, container)
	if opts.PodContainerDir != "" && len(container.TerminationMessagePath) != 0 {
		// Because the PodContainerDir contains pod uid and container name which is unique enough,
		// here we just add a unique container id to make the path unique for different instances
		// of the same container.
		containerLogPath := path.Join(opts.PodContainerDir, cid)
		fs, err := dm.os.Create(containerLogPath)
		if err != nil {
			// TODO: Clean up the previously created dir? return the error?
			utilruntime.HandleError(fmt.Errorf("error creating termination-log file %q: %v", containerLogPath, err))
		} else {
			fs.Close() // Close immediately; we're just doing a `touch` here

			// Chmod is needed because ioutil.WriteFile() ends up calling
			// open(2) to create the file, so the final mode used is "mode &
			// ~umask". But we want to make sure the specified mode is used
			// in the file no matter what the umask is.
			if err := dm.os.Chmod(containerLogPath, 0666); err != nil {
				utilruntime.HandleError(fmt.Errorf("unable to set termination-log file permissions %q: %v", containerLogPath, err))
			}

			// Have docker relabel the termination log path if SELinux is
			// enabled.
			b := fmt.Sprintf("%s:%s", containerLogPath, container.TerminationMessagePath)
			if selinux.SELinuxEnabled() {
				b += ":Z"
			}
			binds = append(binds, b)
		}
	}

	userNsMode := ""
	if opts.EnableHostUserNamespace {
		userNsMode = "host"
	}

	hc := &dockercontainer.HostConfig{
		Binds:          binds,
		NetworkMode:    dockercontainer.NetworkMode(netMode),
		IpcMode:        dockercontainer.IpcMode(ipcMode),
		UTSMode:        dockercontainer.UTSMode(utsMode),
		PidMode:        dockercontainer.PidMode(pidMode),
		UsernsMode:     dockercontainer.UsernsMode(userNsMode),
		ReadonlyRootfs: readOnlyRootFilesystem(container),
		Resources: dockercontainer.Resources{
			Memory:     memoryLimit,
			MemorySwap: -1,
			CPUShares:  cpuShares,
			Devices:    devices,
		},
		SecurityOpt: fmtSecurityOpts,
	}

	updateHostConfig(hc, opts)

	// Set sysctls if requested
	if container.Name == PodInfraContainerName {
		sysctls, unsafeSysctls, err := v1.SysctlsFromPodAnnotations(pod.Annotations)
		if err != nil {
			dm.recorder.Eventf(ref, v1.EventTypeWarning, events.FailedToCreateContainer, "Failed to create docker container %q of pod %q with error: %v", container.Name, format.Pod(pod), err)
			return kubecontainer.ContainerID{}, err
		}
		if len(sysctls)+len(unsafeSysctls) > 0 {
			hc.Sysctls = make(map[string]string, len(sysctls)+len(unsafeSysctls))
			for _, c := range sysctls {
				hc.Sysctls[c.Name] = c.Value
			}
			for _, c := range unsafeSysctls {
				hc.Sysctls[c.Name] = c.Value
			}
		}
	}

	// If current api version is equal to or newer than docker 1.10 requested, set OomScoreAdj to HostConfig
	result, err := dm.checkDockerAPIVersion(dockerV110APIVersion)
	if err != nil {
		glog.Errorf("Failed to check docker api version: %v", err)
	} else if result >= 0 {
		hc.OomScoreAdj = oomScoreAdj
	}

	if dm.cpuCFSQuota {
		// if cpuLimit.Amount is nil, then the appropriate default value is returned to allow full usage of cpu resource.
		cpuQuota, cpuPeriod := cm.MilliCPUToQuota(cpuLimit.MilliValue())

		hc.CPUQuota = cpuQuota
		hc.CPUPeriod = cpuPeriod
	}

	if len(opts.CgroupParent) > 0 {
		cgroupParent := opts.CgroupParent
		// if docker uses the systemd cgroup driver, it expects *.slice style names for cgroup parent.
		// if we configured kubelet to use --cgroup-driver=cgroupfs, and docker is configured to use systemd driver
		// docker will fail to launch the container because the name we provide will not be a valid slice.
		// this is a very good thing.
		if dm.cgroupDriver == "systemd" {
			cgroupParent, err = cm.ConvertCgroupFsNameToSystemd(opts.CgroupParent)
			if err != nil {
				return kubecontainer.ContainerID{}, err
			}
		}
		hc.CgroupParent = cgroupParent
		glog.V(3).Infof("Container %v/%v/%v: setting cgroup parent: %v", pod.Namespace, pod.Name, container.Name, hc.CgroupParent)
	}

	dockerOpts := dockertypes.ContainerCreateConfig{
		Name: containerName,
		Config: &dockercontainer.Config{
			Env:        makeEnvList(opts.Envs),
			Image:      imageRef,
			WorkingDir: container.WorkingDir,
			Labels:     labels,
			// Interactive containers:
			OpenStdin: container.Stdin,
			StdinOnce: container.StdinOnce,
			Tty:       container.TTY,
		},
		HostConfig: hc,
	}

	// Set network configuration for infra-container
	if container.Name == PodInfraContainerName {
		setInfraContainerNetworkConfig(pod, netMode, opts, &dockerOpts)
	}

	setEntrypointAndCommand(container, opts, dockerOpts)

	glog.V(3).Infof("Container %v/%v/%v: setting entrypoint \"%v\" and command \"%v\"", pod.Namespace, pod.Name, container.Name, dockerOpts.Config.Entrypoint, dockerOpts.Config.Cmd)

	supplementalGids := dm.runtimeHelper.GetExtraSupplementalGroupsForPod(pod)
	securityContextProvider := dockersecurity.NewSimpleSecurityContextProvider(optSeparator)
	securityContextProvider.ModifyContainerConfig(pod, container, dockerOpts.Config)
	securityContextProvider.ModifyHostConfig(pod, container, dockerOpts.HostConfig, supplementalGids)
	createResp, err := dm.client.CreateContainer(dockerOpts)
	if err != nil {
		dm.recorder.Eventf(ref, v1.EventTypeWarning, events.FailedToCreateContainer, "Failed to create docker container %q of pod %q with error: %v", container.Name, format.Pod(pod), err)
		return kubecontainer.ContainerID{}, err
	}
	if len(createResp.Warnings) != 0 {
		glog.V(2).Infof("Container %q of pod %q created with warnings: %v", container.Name, format.Pod(pod), createResp.Warnings)
	}

	createdEventMsg := fmt.Sprintf("Created container with docker id %v", utilstrings.ShortenString(createResp.ID, 12))
	if len(securityOpts) > 0 {
		var msgs []string
		for _, opt := range securityOpts {
			msg := opt.msg
			if msg == "" {
				msg = opt.value
			}
			msgs = append(msgs, fmt.Sprintf("%s=%s", opt.key, truncateMsg(msg, 256)))
		}
		createdEventMsg = fmt.Sprintf("%s; Security:[%s]", createdEventMsg, strings.Join(msgs, " "))
	}
	dm.recorder.Eventf(ref, v1.EventTypeNormal, events.CreatedContainer, createdEventMsg)

	if err = dm.client.StartContainer(createResp.ID); err != nil {
		dm.recorder.Eventf(ref, v1.EventTypeWarning, events.FailedToStartContainer,
			"Failed to start container with docker id %v with error: %v", utilstrings.ShortenString(createResp.ID, 12), err)
		return kubecontainer.ContainerID{}, err
	}
	dm.recorder.Eventf(ref, v1.EventTypeNormal, events.StartedContainer, "Started container with docker id %v", utilstrings.ShortenString(createResp.ID, 12))

	return kubecontainer.DockerID(createResp.ID).ContainerID(), nil
}

// setInfraContainerNetworkConfig sets the network configuration for the infra-container. We only set network configuration for infra-container, all
// the user containers will share the same network namespace with infra-container.
// NOTE: cluster dns settings aren't passed anymore to docker api in all cases, not only for pods with host network:
// the resolver conf will be overwritten after infra-container creation to override docker's behaviour
func setInfraContainerNetworkConfig(pod *v1.Pod, netMode string, opts *kubecontainer.RunContainerOptions, dockerOpts *dockertypes.ContainerCreateConfig) {
	exposedPorts, portBindings := makePortsAndBindings(opts.PortMappings)
	dockerOpts.Config.ExposedPorts = exposedPorts
	dockerOpts.HostConfig.PortBindings = dockernat.PortMap(portBindings)

	if netMode != namespaceModeHost {
		dockerOpts.Config.Hostname = opts.Hostname
	}
}

func setEntrypointAndCommand(container *v1.Container, opts *kubecontainer.RunContainerOptions, dockerOpts dockertypes.ContainerCreateConfig) {
	command, args := kubecontainer.ExpandContainerCommandAndArgs(container, opts.Envs)

	dockerOpts.Config.Entrypoint = dockerstrslice.StrSlice(command)
	dockerOpts.Config.Cmd = dockerstrslice.StrSlice(args)
}

// A helper function to get the KubeletContainerName and hash from a docker
// container.
func getDockerContainerNameInfo(c *dockertypes.Container) (*KubeletContainerName, uint64, error) {
	if len(c.Names) == 0 {
		return nil, 0, fmt.Errorf("cannot parse empty docker container name: %#v", c.Names)
	}
	dockerName, hash, err := ParseDockerName(c.Names[0])
	if err != nil {
		return nil, 0, fmt.Errorf("parse docker container name %q error: %v", c.Names[0], err)
	}
	return dockerName, hash, nil
}

// Get pod UID, name, and namespace by examining the container names.
func getPodInfoFromContainer(c *dockertypes.Container) (kubetypes.UID, string, string, error) {
	dockerName, _, err := getDockerContainerNameInfo(c)
	if err != nil {
		return kubetypes.UID(""), "", "", err
	}
	name, namespace, err := kubecontainer.ParsePodFullName(dockerName.PodFullName)
	if err != nil {
		return kubetypes.UID(""), "", "", fmt.Errorf("parse pod full name %q error: %v", dockerName.PodFullName, err)
	}
	return dockerName.PodUID, name, namespace, nil
}

// GetContainers returns a list of running containers if |all| is false;
// otherwise, it returns all containers.
func (dm *DockerManager) GetContainers(all bool) ([]*kubecontainer.Container, error) {
	containers, err := GetKubeletDockerContainers(dm.client, all)
	if err != nil {
		return nil, err
	}
	// Convert DockerContainers to []*kubecontainer.Container
	result := make([]*kubecontainer.Container, 0, len(containers))
	for _, c := range containers {
		converted, err := toRuntimeContainer(c)
		if err != nil {
			glog.Errorf("Error examining the container %v: %v", c.ID, err)
			continue
		}
		result = append(result, converted)
	}
	return result, nil
}

func (dm *DockerManager) GetPods(all bool) ([]*kubecontainer.Pod, error) {
	start := time.Now()
	defer func() {
		metrics.ContainerManagerLatency.WithLabelValues("GetPods").Observe(metrics.SinceInMicroseconds(start))
	}()
	pods := make(map[kubetypes.UID]*kubecontainer.Pod)
	var result []*kubecontainer.Pod

	containers, err := GetKubeletDockerContainers(dm.client, all)
	if err != nil {
		return nil, err
	}

	// Group containers by pod.
	for _, c := range containers {
		converted, err := toRuntimeContainer(c)
		if err != nil {
			glog.Errorf("Error examining the container %v: %v", c.ID, err)
			continue
		}

		podUID, podName, podNamespace, err := getPodInfoFromContainer(c)
		if err != nil {
			glog.Errorf("Error examining the container %v: %v", c.ID, err)
			continue
		}

		pod, found := pods[podUID]
		if !found {
			pod = &kubecontainer.Pod{
				ID:        podUID,
				Name:      podName,
				Namespace: podNamespace,
			}
			pods[podUID] = pod
		}
		pod.Containers = append(pod.Containers, converted)
	}

	// Convert map to list.
	for _, p := range pods {
		result = append(result, p)
	}
	return result, nil
}

// List all images in the local storage.
func (dm *DockerManager) ListImages() ([]kubecontainer.Image, error) {
	var images []kubecontainer.Image

	dockerImages, err := dm.client.ListImages(dockertypes.ImageListOptions{})
	if err != nil {
		return images, err
	}

	for _, di := range dockerImages {
		image, err := toRuntimeImage(&di)
		if err != nil {
			continue
		}
		images = append(images, *image)
	}
	return images, nil
}

// GetImageRef returns the image digest if exists, or else returns the image ID.
// It is exported for reusing in dockershim.
func GetImageRef(client DockerInterface, image string) (string, error) {
	img, err := client.InspectImageByRef(image)
	if err != nil {
		return "", err
	}
	if img == nil {
		return "", fmt.Errorf("unable to inspect image %s", image)
	}

	// Returns the digest if it exist.
	if len(img.RepoDigests) > 0 {
		return img.RepoDigests[0], nil
	}

	return img.ID, nil
}

// PullImage pulls an image from network to local storage.
func (dm *DockerManager) PullImage(image kubecontainer.ImageSpec, secrets []v1.Secret) (string, error) {
	err := dm.dockerPuller.Pull(image.Image, secrets)
	if err != nil {
		return "", err
	}

	return GetImageRef(dm.client, image.Image)
}

// GetImageRef gets the reference (digest or ID) of the image which has already been in
// the local storage. It returns ("", nil) if the image isn't in the local storage.
func (dm *DockerManager) GetImageRef(image kubecontainer.ImageSpec) (string, error) {
	return dm.dockerPuller.GetImageRef(image.Image)
}

// Removes the specified image.
func (dm *DockerManager) RemoveImage(image kubecontainer.ImageSpec) error {
	// If the image has multiple tags, we need to remove all the tags
	if inspectImage, err := dm.client.InspectImageByID(image.Image); err == nil && len(inspectImage.RepoTags) > 1 {
		for _, tag := range inspectImage.RepoTags {
			if _, err := dm.client.RemoveImage(tag, dockertypes.ImageRemoveOptions{PruneChildren: true}); err != nil {
				return err
			}
		}
		return nil
	}

	_, err := dm.client.RemoveImage(image.Image, dockertypes.ImageRemoveOptions{PruneChildren: true})
	return err
}

// podInfraContainerChanged returns true if the pod infra container has changed.
func (dm *DockerManager) podInfraContainerChanged(pod *v1.Pod, podInfraContainerStatus *kubecontainer.ContainerStatus) (bool, error) {
	var ports []v1.ContainerPort

	// Check network mode.
	if kubecontainer.IsHostNetworkPod(pod) {
		dockerPodInfraContainer, err := dm.client.InspectContainer(podInfraContainerStatus.ID.ID)
		if err != nil {
			return false, err
		}

		networkMode := getDockerNetworkMode(dockerPodInfraContainer)
		if networkMode != namespaceModeHost {
			glog.V(4).Infof("host: %v, %v", pod.Spec.HostNetwork, networkMode)
			return true, nil
		}
	} else if !dm.pluginDisablesDockerNetworking() {
		// Docker only exports ports from the pod infra container. Let's
		// collect all of the relevant ports and export them.
		for _, container := range pod.Spec.InitContainers {
			ports = append(ports, container.Ports...)
		}
		for _, container := range pod.Spec.Containers {
			ports = append(ports, container.Ports...)
		}
	}
	expectedPodInfraContainer := &v1.Container{
		Name:            PodInfraContainerName,
		Image:           dm.podInfraContainerImage,
		Ports:           ports,
		ImagePullPolicy: podInfraContainerImagePullPolicy,
		Env:             dm.podInfraContainerEnv,
	}
	return podInfraContainerStatus.Hash != kubecontainer.HashContainerLegacy(expectedPodInfraContainer), nil
}

// determine if the container root should be a read only filesystem.
func readOnlyRootFilesystem(container *v1.Container) bool {
	return container.SecurityContext != nil && container.SecurityContext.ReadOnlyRootFilesystem != nil && *container.SecurityContext.ReadOnlyRootFilesystem
}

// container must not be nil
func getDockerNetworkMode(container *dockertypes.ContainerJSON) string {
	if container.HostConfig != nil {
		return string(container.HostConfig.NetworkMode)
	}
	return ""
}

func (dm *DockerManager) pluginDisablesDockerNetworking() bool {
	return dm.network.PluginName() == "cni" || dm.network.PluginName() == "kubenet"
}

// newDockerVersion returns a semantically versioned docker version value
func newDockerVersion(version string) (*utilversion.Version, error) {
	return utilversion.ParseSemantic(version)
}

// apiVersion implements kubecontainer.Version interface by implementing
// Compare() and String(). It uses the compare function of engine-api to
// compare docker apiversions.
type apiVersion string

func (v apiVersion) String() string {
	return string(v)
}

func (v apiVersion) Compare(other string) (int, error) {
	if dockerapiversion.LessThan(string(v), other) {
		return -1, nil
	} else if dockerapiversion.GreaterThan(string(v), other) {
		return 1, nil
	}
	return 0, nil
}

func (dm *DockerManager) Type() string {
	return DockerType
}

func (dm *DockerManager) Version() (kubecontainer.Version, error) {
	v, err := dm.client.Version()
	if err != nil {
		return nil, fmt.Errorf("docker: failed to get docker version: %v", err)
	}
	version, err := newDockerVersion(v.Version)
	if err != nil {
		return nil, fmt.Errorf("docker: failed to parse docker version %q: %v", v.Version, err)
	}
	return version, nil
}

func (dm *DockerManager) APIVersion() (kubecontainer.Version, error) {
	v, err := dm.client.Version()
	if err != nil {
		return nil, fmt.Errorf("docker: failed to get docker version: %v", err)
	}

	return apiVersion(v.APIVersion), nil
}

// Status returns error if docker daemon is unhealthy, nil otherwise.
// Now we do this by checking whether:
// 1) `docker version` works
// 2) docker version is compatible with minimum requirement
func (dm *DockerManager) Status() (*kubecontainer.RuntimeStatus, error) {
	return nil, dm.checkVersionCompatibility()
}

func (dm *DockerManager) checkVersionCompatibility() error {
	version, err := dm.APIVersion()
	if err != nil {
		return err
	}
	// Verify the docker version.
	result, err := version.Compare(minimumDockerAPIVersion)
	if err != nil {
		return fmt.Errorf("failed to compare current docker version %v with minimum support Docker version %q - %v", version, minimumDockerAPIVersion, err)
	}
	if result < 0 {
		return fmt.Errorf("container runtime version is older than %s", minimumDockerAPIVersion)
	}
	return nil
}

func (dm *DockerManager) getDockerOptSeparator() (rune, error) {
	sep := SecurityOptSeparatorNew
	if result, err := dm.checkDockerAPIVersion(SecurityOptSeparatorChangeVersion); err != nil {
		return sep, err
	} else if result < 0 {
		sep = SecurityOptSeparatorOld
	}
	return sep, nil
}

// FmtDockerOpts formats the docker security options using the given separator.
func FmtDockerOpts(opts []dockerOpt, sep rune) []string {
	fmtOpts := make([]string, len(opts))
	for i, opt := range opts {
		fmtOpts[i] = fmt.Sprintf("%s%c%s", opt.key, sep, opt.value)
	}
	return fmtOpts
}

type dockerOpt struct {
	// The key-value pair passed to docker.
	key, value string
	// The alternative value to use in log/event messages.
	msg string
}

// Expose key/value from dockertools
func (d dockerOpt) GetKV() (string, string) {
	return d.key, d.value
}

// Get the docker security options for seccomp.
func (dm *DockerManager) getSeccompOpts(pod *v1.Pod, ctrName string) ([]dockerOpt, error) {
	version, err := dm.APIVersion()
	if err != nil {
		return nil, err
	}

	// seccomp is only on docker versions >= v1.10
	if result, err := version.Compare(dockerV110APIVersion); err != nil {
		return nil, err
	} else if result < 0 {
		return nil, nil // return early for Docker < 1.10
	}

	return GetSeccompOpts(pod.ObjectMeta.Annotations, ctrName, dm.seccompProfileRoot)
}

// Temporarily export this function to share with dockershim.
// TODO: clean this up.
func GetSeccompOpts(annotations map[string]string, ctrName, profileRoot string) ([]dockerOpt, error) {
	profile, profileOK := annotations[v1.SeccompContainerAnnotationKeyPrefix+ctrName]
	if !profileOK {
		// try the pod profile
		profile, profileOK = annotations[v1.SeccompPodAnnotationKey]
		if !profileOK {
			// return early the default
			return defaultSeccompOpt, nil
		}
	}

	if profile == "unconfined" {
		// return early the default
		return defaultSeccompOpt, nil
	}

	if profile == "docker/default" {
		// return nil so docker will load the default seccomp profile
		return nil, nil
	}

	if !strings.HasPrefix(profile, "localhost/") {
		return nil, fmt.Errorf("unknown seccomp profile option: %s", profile)
	}

	name := strings.TrimPrefix(profile, "localhost/") // by pod annotation validation, name is a valid subpath
	fname := filepath.Join(profileRoot, filepath.FromSlash(name))
	file, err := ioutil.ReadFile(fname)
	if err != nil {
		return nil, fmt.Errorf("cannot load seccomp profile %q: %v", name, err)
	}

	b := bytes.NewBuffer(nil)
	if err := json.Compact(b, file); err != nil {
		return nil, err
	}
	// Rather than the full profile, just put the filename & md5sum in the event log.
	msg := fmt.Sprintf("%s(md5:%x)", name, md5.Sum(file))

	return []dockerOpt{{"seccomp", b.String(), msg}}, nil
}

// Get the docker security options for AppArmor.
func (dm *DockerManager) getAppArmorOpts(pod *v1.Pod, ctrName string) ([]dockerOpt, error) {
	return GetAppArmorOpts(pod.Annotations, ctrName)
}

// Temporarily export this function to share with dockershim.
// TODO: clean this up.
func GetAppArmorOpts(annotations map[string]string, ctrName string) ([]dockerOpt, error) {
	profile := apparmor.GetProfileNameFromPodAnnotations(annotations, ctrName)
	if profile == "" || profile == apparmor.ProfileRuntimeDefault {
		// The docker applies the default profile by default.
		return nil, nil
	}

	// Assume validation has already happened.
	profileName := strings.TrimPrefix(profile, apparmor.ProfileNamePrefix)
	return []dockerOpt{{"apparmor", profileName, ""}}, nil
}

type dockerExitError struct {
	Inspect *dockertypes.ContainerExecInspect
}

func (d *dockerExitError) String() string {
	return d.Error()
}

func (d *dockerExitError) Error() string {
	return fmt.Sprintf("Error executing in Docker Container: %d", d.Inspect.ExitCode)
}

func (d *dockerExitError) Exited() bool {
	return !d.Inspect.Running
}

func (d *dockerExitError) ExitStatus() int {
	return d.Inspect.ExitCode
}

// ExecInContainer runs the command inside the container identified by containerID.
func (dm *DockerManager) ExecInContainer(containerID kubecontainer.ContainerID, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan term.Size, timeout time.Duration) error {
	if dm.execHandler == nil {
		return errors.New("unable to exec without an exec handler")
	}

	container, err := dm.client.InspectContainer(containerID.ID)
	if err != nil {
		return err
	}
	if !container.State.Running {
		return fmt.Errorf("container not running (%s)", container.ID)
	}

	return dm.execHandler.ExecInContainer(dm.client, container, cmd, stdin, stdout, stderr, tty, resize, timeout)
}

func (dm *DockerManager) AttachContainer(containerID kubecontainer.ContainerID, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan term.Size) error {
	return AttachContainer(dm.client, containerID.ID, stdin, stdout, stderr, tty, resize)
}

// Temporarily export this function to share with dockershim.
// TODO: clean this up.
func AttachContainer(client DockerInterface, containerID string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan term.Size) error {
	// Have to start this before the call to client.AttachToContainer because client.AttachToContainer is a blocking
	// call :-( Otherwise, resize events don't get processed and the terminal never resizes.
	kubecontainer.HandleResizing(resize, func(size term.Size) {
		client.ResizeContainerTTY(containerID, int(size.Height), int(size.Width))
	})

	// TODO(random-liu): Do we really use the *Logs* field here?
	opts := dockertypes.ContainerAttachOptions{
		Stream: true,
		Stdin:  stdin != nil,
		Stdout: stdout != nil,
		Stderr: stderr != nil,
	}
	sopts := StreamOptions{
		InputStream:  stdin,
		OutputStream: stdout,
		ErrorStream:  stderr,
		RawTerminal:  tty,
	}
	return client.AttachToContainer(containerID, opts, sopts)
}

func noPodInfraContainerError(podName, podNamespace string) error {
	return fmt.Errorf("cannot find pod infra container in pod %q", kubecontainer.BuildPodFullName(podName, podNamespace))
}

// PortForward executes socat in the pod's network namespace and copies
// data between stream (representing the user's local connection on their
// computer) and the specified port in the container.
//
// TODO:
//  - match cgroups of container
//  - should we support nsenter + socat on the host? (current impl)
//  - should we support nsenter + socat in a container, running with elevated privs and --pid=host?
func (dm *DockerManager) PortForward(pod *kubecontainer.Pod, port int32, stream io.ReadWriteCloser) error {
	podInfraContainer := pod.FindContainerByName(PodInfraContainerName)
	if podInfraContainer == nil {
		return noPodInfraContainerError(pod.Name, pod.Namespace)
	}

	return PortForward(dm.client, podInfraContainer.ID.ID, port, stream)
}

// UpdatePodCIDR updates the podCIDR for the runtime.
// Currently no-ops, just implemented to satisfy the cri.
func (dm *DockerManager) UpdatePodCIDR(podCIDR string) error {
	return nil
}

// Temporarily export this function to share with dockershim.
func PortForward(client DockerInterface, podInfraContainerID string, port int32, stream io.ReadWriteCloser) error {
	container, err := client.InspectContainer(podInfraContainerID)
	if err != nil {
		return err
	}

	if !container.State.Running {
		return fmt.Errorf("container not running (%s)", container.ID)
	}

	containerPid := container.State.Pid
	socatPath, lookupErr := exec.LookPath("socat")
	if lookupErr != nil {
		return fmt.Errorf("unable to do port forwarding: socat not found.")
	}

	args := []string{"-t", fmt.Sprintf("%d", containerPid), "-n", socatPath, "-", fmt.Sprintf("TCP4:localhost:%d", port)}

	nsenterPath, lookupErr := exec.LookPath("nsenter")
	if lookupErr != nil {
		return fmt.Errorf("unable to do port forwarding: nsenter not found.")
	}

	commandString := fmt.Sprintf("%s %s", nsenterPath, strings.Join(args, " "))
	glog.V(4).Infof("executing port forwarding command: %s", commandString)

	command := exec.Command(nsenterPath, args...)
	command.Stdout = stream

	stderr := new(bytes.Buffer)
	command.Stderr = stderr

	// If we use Stdin, command.Run() won't return until the goroutine that's copying
	// from stream finishes. Unfortunately, if you have a client like telnet connected
	// via port forwarding, as long as the user's telnet client is connected to the user's
	// local listener that port forwarding sets up, the telnet session never exits. This
	// means that even if socat has finished running, command.Run() won't ever return
	// (because the client still has the connection and stream open).
	//
	// The work around is to use StdinPipe(), as Wait() (called by Run()) closes the pipe
	// when the command (socat) exits.
	inPipe, err := command.StdinPipe()
	if err != nil {
		return fmt.Errorf("unable to do port forwarding: error creating stdin pipe: %v", err)
	}
	go func() {
		io.Copy(inPipe, stream)
		inPipe.Close()
	}()

	if err := command.Run(); err != nil {
		return fmt.Errorf("%v: %s", err, stderr.String())
	}

	return nil
}

// TODO(random-liu): Change running pod to pod status in the future. We can't do it now, because kubelet also uses this function without pod status.
// We can only deprecate this after refactoring kubelet.
// TODO(random-liu): After using pod status for KillPod(), we can also remove the kubernetesPodLabel, because all the needed information should have
// been extract from new labels and stored in pod status.
// only hard eviction scenarios should provide a grace period override, all other code paths must pass nil.
func (dm *DockerManager) KillPod(pod *v1.Pod, runningPod kubecontainer.Pod, gracePeriodOverride *int64) error {
	result := dm.killPodWithSyncResult(pod, runningPod, gracePeriodOverride)
	return result.Error()
}

// NOTE(random-liu): The pod passed in could be *nil* when kubelet restarted.
// runtimePod may contain either running or exited containers
func (dm *DockerManager) killPodWithSyncResult(pod *v1.Pod, runtimePod kubecontainer.Pod, gracePeriodOverride *int64) (result kubecontainer.PodSyncResult) {
	// Short circuit if there's nothing to kill.
	if len(runtimePod.Containers) == 0 {
		return
	}
	// Send the kills in parallel since they may take a long time.
	// There may be len(runtimePod.Containers) or len(runtimePod.Containers)-1 of result in the channel
	containerResults := make(chan *kubecontainer.SyncResult, len(runtimePod.Containers))
	wg := sync.WaitGroup{}
	var (
		networkContainers []*kubecontainer.Container
		networkSpecs      []*v1.Container
	)
	wg.Add(len(runtimePod.Containers))
	for _, container := range runtimePod.Containers {
		go func(container *kubecontainer.Container) {
			defer utilruntime.HandleCrash()
			defer wg.Done()

			var containerSpec *v1.Container
			if pod != nil {
				for i, c := range pod.Spec.Containers {
					if c.Name == container.Name {
						containerSpec = &pod.Spec.Containers[i]
						break
					}
				}
				if containerSpec == nil {
					for i, c := range pod.Spec.InitContainers {
						if c.Name == container.Name {
							containerSpec = &pod.Spec.InitContainers[i]
							break
						}
					}
				}
			}

			// TODO: Handle this without signaling the pod infra container to
			// adapt to the generic container runtime.
			if containerIsNetworked(container.Name) {
				// Store the container runtime for later deletion.
				// We do this so that PreStop handlers can run in the network namespace.
				networkContainers = append(networkContainers, container)
				networkSpecs = append(networkSpecs, containerSpec)
			} else {
				killContainerResult := kubecontainer.NewSyncResult(kubecontainer.KillContainer, container.Name)
				err := dm.KillContainerInPod(container.ID, containerSpec, pod, "Need to kill pod.", gracePeriodOverride)
				if err != nil {
					killContainerResult.Fail(kubecontainer.ErrKillContainer, err.Error())
					glog.Errorf("Failed to delete container %v: %v; Skipping pod %q", container.ID.ID, err, runtimePod.ID)
				}
				containerResults <- killContainerResult
			}
		}(container)
	}
	wg.Wait()
	close(containerResults)
	for containerResult := range containerResults {
		result.AddSyncResult(containerResult)
	}

	// Tear down any dead or running network/infra containers, but only kill
	// those that are still running.
	for i := range networkContainers {
		networkContainer := networkContainers[i]
		networkSpec := networkSpecs[i]

		teardownNetworkResult := kubecontainer.NewSyncResult(kubecontainer.TeardownNetwork, kubecontainer.BuildPodFullName(runtimePod.Name, runtimePod.Namespace))
		result.AddSyncResult(teardownNetworkResult)

		ins, err := dm.client.InspectContainer(networkContainer.ID.ID)
		if err != nil {
			err = fmt.Errorf("Error inspecting container %v: %v", networkContainer.ID.ID, err)
			glog.Error(err)
			teardownNetworkResult.Fail(kubecontainer.ErrTeardownNetwork, err.Error())
			continue
		}

		if getDockerNetworkMode(ins) != namespaceModeHost {
			if err := dm.network.TearDownPod(runtimePod.Namespace, runtimePod.Name, networkContainer.ID); err != nil {
				teardownNetworkResult.Fail(kubecontainer.ErrTeardownNetwork, err.Error())
				glog.Error(err)
			}
		}
		if networkContainer.State == kubecontainer.ContainerStateRunning {
			killContainerResult := kubecontainer.NewSyncResult(kubecontainer.KillContainer, networkContainer.Name)
			result.AddSyncResult(killContainerResult)
			if err := dm.KillContainerInPod(networkContainer.ID, networkSpec, pod, "Need to kill pod.", gracePeriodOverride); err != nil {
				killContainerResult.Fail(kubecontainer.ErrKillContainer, err.Error())
				glog.Errorf("Failed to delete container %v: %v; Skipping pod %q", networkContainer.ID.ID, err, runtimePod.ID)
			}
		}
	}
	return
}

// KillContainerInPod kills a container in the pod. It must be passed either a container ID or a container and pod,
// and will attempt to lookup the other information if missing.
func (dm *DockerManager) KillContainerInPod(containerID kubecontainer.ContainerID, container *v1.Container, pod *v1.Pod, message string, gracePeriodOverride *int64) error {
	switch {
	case containerID.IsEmpty():
		// Locate the container.
		pods, err := dm.GetPods(false)
		if err != nil {
			return err
		}
		targetPod := kubecontainer.Pods(pods).FindPod(kubecontainer.GetPodFullName(pod), pod.UID)
		targetContainer := targetPod.FindContainerByName(container.Name)
		if targetContainer == nil {
			return fmt.Errorf("unable to find container %q in pod %q", container.Name, targetPod.Name)
		}
		containerID = targetContainer.ID

	case container == nil || pod == nil:
		// Read information about the container from labels
		inspect, err := dm.client.InspectContainer(containerID.ID)
		if err != nil {
			return err
		}
		storedPod, storedContainer, cerr := containerAndPodFromLabels(inspect)
		if cerr != nil {
			glog.Errorf("unable to access pod data from container: %v", cerr)
		}
		if container == nil {
			container = storedContainer
		}
		if pod == nil {
			pod = storedPod
		}
	}
	return dm.killContainer(containerID, container, pod, message, gracePeriodOverride)
}

// killContainer accepts a containerID and an optional container or pod containing shutdown policies. Invoke
// KillContainerInPod if information must be retrieved first.  It is only valid to provide a grace period override
// during hard eviction scenarios.  All other code paths in kubelet must never provide a grace period override otherwise
// data corruption could occur in the end-user application.
func (dm *DockerManager) killContainer(containerID kubecontainer.ContainerID, container *v1.Container, pod *v1.Pod, reason string, gracePeriodOverride *int64) error {
	ID := containerID.ID
	name := ID
	if container != nil {
		name = fmt.Sprintf("%s %s", name, container.Name)
	}
	if pod != nil {
		name = fmt.Sprintf("%s %s/%s", name, pod.Namespace, pod.Name)
	}

	gracePeriod := int64(minimumGracePeriodInSeconds)
	if pod != nil {
		switch {
		case pod.DeletionGracePeriodSeconds != nil:
			gracePeriod = *pod.DeletionGracePeriodSeconds
		case pod.Spec.TerminationGracePeriodSeconds != nil:
			gracePeriod = *pod.Spec.TerminationGracePeriodSeconds
		}
	}
	glog.V(2).Infof("Killing container %q with %d second grace period", name, gracePeriod)
	start := metav1.Now()

	if pod != nil && container != nil && container.Lifecycle != nil && container.Lifecycle.PreStop != nil {
		glog.V(4).Infof("Running preStop hook for container %q", name)
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer utilruntime.HandleCrash()
			if msg, err := dm.runner.Run(containerID, pod, container, container.Lifecycle.PreStop); err != nil {
				glog.Errorf("preStop hook for container %q failed: %v", name, err)
				dm.generateFailedContainerEvent(containerID, pod.Name, events.FailedPreStopHook, msg)
			}
		}()
		select {
		case <-time.After(time.Duration(gracePeriod) * time.Second):
			glog.Warningf("preStop hook for container %q did not complete in %d seconds", name, gracePeriod)
			message := fmt.Sprintf("preStop hook for container %q did not complete in %d seconds", name, gracePeriod)
			dm.generateFailedContainerEvent(containerID, pod.Name, events.UnfinishedPreStopHook, message)
		case <-done:
			glog.V(4).Infof("preStop hook for container %q completed", name)
		}
		gracePeriod -= int64(metav1.Now().Sub(start.Time).Seconds())
	}

	// if the caller did not specify a grace period override, we ensure that the grace period
	// is not less than the minimal shutdown window to avoid unnecessary SIGKILLs.  if a caller
	// did provide an override, we always set the gracePeriod to that value.  the only valid
	// time to send an override is during eviction scenarios where we want to do a hard kill of
	// a container because of resource exhaustion for incompressible resources (i.e. disk, memory).
	if gracePeriodOverride == nil {
		if gracePeriod < minimumGracePeriodInSeconds {
			gracePeriod = minimumGracePeriodInSeconds
		}
	} else {
		gracePeriod = *gracePeriodOverride
		glog.V(2).Infof("Killing container %q, but using %d second grace period override", name, gracePeriod)
	}

	err := dm.client.StopContainer(ID, int(gracePeriod))
	if err == nil {
		glog.V(2).Infof("Container %q exited after %s", name, metav1.Now().Sub(start.Time))
	} else {
		glog.Warningf("Container %q termination failed after %s: %v", name, metav1.Now().Sub(start.Time), err)
	}
	ref, ok := dm.containerRefManager.GetRef(containerID)
	if !ok {
		glog.Warningf("No ref for pod '%q'", name)
	} else {
		message := fmt.Sprintf("Killing container with docker id %v", utilstrings.ShortenString(ID, 12))
		if reason != "" {
			message = fmt.Sprint(message, ": ", reason)
		}
		dm.recorder.Event(ref, v1.EventTypeNormal, events.KillingContainer, message)
		dm.containerRefManager.ClearRef(containerID)
	}
	return err
}

func (dm *DockerManager) generateFailedContainerEvent(containerID kubecontainer.ContainerID, podName, reason, message string) {
	ref, ok := dm.containerRefManager.GetRef(containerID)
	if !ok {
		glog.Warningf("No ref for pod '%q'", podName)
		return
	}
	dm.recorder.Event(ref, v1.EventTypeWarning, reason, message)
}

var errNoPodOnContainer = fmt.Errorf("no pod information labels on Docker container")

// containerAndPodFromLabels tries to load the appropriate container info off of a Docker container's labels
func containerAndPodFromLabels(inspect *dockertypes.ContainerJSON) (pod *v1.Pod, container *v1.Container, err error) {
	if inspect == nil || inspect.Config == nil || inspect.Config.Labels == nil {
		return nil, nil, errNoPodOnContainer
	}
	labels := inspect.Config.Labels

	// the pod data may not be set
	if body, found := labels[kubernetesPodLabel]; found {
		pod = &v1.Pod{}
		if err = kruntime.DecodeInto(api.Codecs.UniversalDecoder(), []byte(body), pod); err == nil {
			name := labels[types.KubernetesContainerNameLabel]
			for ix := range pod.Spec.Containers {
				if pod.Spec.Containers[ix].Name == name {
					container = &pod.Spec.Containers[ix]
					break
				}
			}
			if container == nil {
				for ix := range pod.Spec.InitContainers {
					if pod.Spec.InitContainers[ix].Name == name {
						container = &pod.Spec.InitContainers[ix]
						break
					}
				}
			}
			if container == nil {
				err = fmt.Errorf("unable to find container %s in pod %v", name, pod)
			}
		} else {
			pod = nil
		}
	}

	// attempt to find the default grace period if we didn't commit a pod, but set the generic metadata
	// field (the one used by kill)
	if pod == nil {
		if period, ok := labels[kubernetesPodTerminationGracePeriodLabel]; ok {
			if seconds, err := strconv.ParseInt(period, 10, 64); err == nil {
				pod = &v1.Pod{}
				pod.DeletionGracePeriodSeconds = &seconds
			}
		}
	}

	return
}

func (dm *DockerManager) applyOOMScoreAdj(pod *v1.Pod, container *v1.Container, containerInfo *dockertypes.ContainerJSON) error {
	if containerInfo.State.Pid == 0 {
		// Container exited. We cannot do anything about it. Ignore this error.
		glog.V(2).Infof("Failed to apply OOM score adj on container %q with ID %q. Init process does not exist.", containerInfo.Name, containerInfo.ID)
		return nil
	}

	cgroupName, err := dm.procFs.GetFullContainerName(containerInfo.State.Pid)
	if err != nil {
		if err == os.ErrNotExist {
			// Container exited. We cannot do anything about it. Ignore this error.
			glog.V(2).Infof("Failed to apply OOM score adj on container %q with ID %q. Init process does not exist.", containerInfo.Name, containerInfo.ID)
			return nil
		}
		return err
	}
	oomScoreAdj := dm.calculateOomScoreAdj(pod, container)
	if err = dm.oomAdjuster.ApplyOOMScoreAdjContainer(cgroupName, oomScoreAdj, 5); err != nil {
		if err == os.ErrNotExist {
			// Container exited. We cannot do anything about it. Ignore this error.
			glog.V(2).Infof("Failed to apply OOM score adj on container %q with ID %q. Init process does not exist.", containerInfo.Name, containerInfo.ID)
			return nil
		}
		return err
	}
	return nil
}

// Run a single container from a pod. Returns the docker container ID
// If do not need to pass labels, just pass nil.
func (dm *DockerManager) runContainerInPod(pod *v1.Pod, container *v1.Container, netMode, ipcMode, pidMode, podIP, imageRef string, restartCount int) (kubecontainer.ContainerID, error) {
	start := time.Now()
	defer func() {
		metrics.ContainerManagerLatency.WithLabelValues("runContainerInPod").Observe(metrics.SinceInMicroseconds(start))
	}()

	ref, err := kubecontainer.GenerateContainerRef(pod, container)
	if err != nil {
		glog.Errorf("Can't make a ref to pod %v, container %v: '%v'", pod.Name, container.Name, err)
	} else {
		glog.V(5).Infof("Generating ref for container %s: %#v", container.Name, ref)
	}

	opts, useClusterFirstPolicy, err := dm.runtimeHelper.GenerateRunContainerOptions(pod, container, podIP)
	if err != nil {
		return kubecontainer.ContainerID{}, fmt.Errorf("GenerateRunContainerOptions: %v", err)
	}

	utsMode := ""
	if kubecontainer.IsHostNetworkPod(pod) {
		utsMode = namespaceModeHost
	}

	oomScoreAdj := dm.calculateOomScoreAdj(pod, container)

	id, err := dm.runContainer(pod, container, opts, ref, imageRef, netMode, ipcMode, utsMode, pidMode, restartCount, oomScoreAdj)
	if err != nil {
		return kubecontainer.ContainerID{}, fmt.Errorf("runContainer: %v", err)
	}

	// Remember this reference so we can report events about this container
	if ref != nil {
		dm.containerRefManager.SetRef(id, ref)
	}

	if container.Lifecycle != nil && container.Lifecycle.PostStart != nil {
		msg, handlerErr := dm.runner.Run(id, pod, container, container.Lifecycle.PostStart)
		if handlerErr != nil {
			err := fmt.Errorf("PostStart handler: %v", handlerErr)
			dm.generateFailedContainerEvent(id, pod.Name, events.FailedPostStartHook, msg)
			dm.KillContainerInPod(id, container, pod, err.Error(), nil)
			return kubecontainer.ContainerID{}, err
		}
	}

	// Container information is used in adjusting OOM scores, adding ndots and getting the logPath.
	containerInfo, err := dm.client.InspectContainer(id.ID)
	if err != nil {
		return kubecontainer.ContainerID{}, fmt.Errorf("InspectContainer: %v", err)
	}

	containerLogFile := containerInfo.LogPath
	if containerLogFile != "" {
		// Create a symbolic link to the Docker container log file using a name which captures the
		// full pod name, the container name and the Docker container ID. Cluster level logging will
		// capture these symbolic filenames which can be used for search terms in Elasticsearch or for
		// labels for Cloud Logging.
		symlinkFile := LogSymlink(dm.containerLogsDir, kubecontainer.GetPodFullName(pod), container.Name, id.ID)
		if err = dm.os.Symlink(containerLogFile, symlinkFile); err != nil {
			glog.Errorf("Failed to create symbolic link to the log file of pod %q container %q: %v", format.Pod(pod), container.Name, err)
		}
	} else {
		dockerLoggingDriver := ""
		dockerInfo, err := dm.client.Info()
		if err != nil {
			glog.Errorf("Failed to execute Info() call to the Docker client: %v", err)
		} else {
			dockerLoggingDriver = dockerInfo.LoggingDriver
			glog.V(10).Infof("Docker logging driver is %s", dockerLoggingDriver)
		}

		if dockerLoggingDriver == dockerDefaultLoggingDriver {
			glog.Errorf("Cannot create symbolic link because container log file doesn't exist!")
		} else {
			glog.V(5).Infof("Unsupported logging driver: %s", dockerLoggingDriver)
		}
	}

	// Check if current docker version is higher than 1.10. Otherwise, we have to apply OOMScoreAdj instead of using docker API.
	// TODO: Remove this logic after we stop supporting docker version < 1.10.
	if err = dm.applyOOMScoreAdjIfNeeded(pod, container, containerInfo); err != nil {
		return kubecontainer.ContainerID{}, err
	}

	// Re-write resolv.conf file generated by docker.
	// NOTE: cluster dns settings aren't passed anymore to docker api in all cases, not only for pods with host network:
	// the resolver conf will be overwritten after infra-container creation to override docker's behaviour
	// This resolv.conf file is shared by all containers of the same pod, and needs to be modified only once per pod.
	// we modify it when the pause container is created since it is the first container created in the pod since it holds
	// the networking namespace.
	if container.Name == PodInfraContainerName {
		if err := RewriteResolvFile(containerInfo.ResolvConfPath, opts.DNS, opts.DNSSearch, useClusterFirstPolicy); err != nil {
			return kubecontainer.ContainerID{}, err
		}
	}

	return id, err
}

func (dm *DockerManager) applyOOMScoreAdjIfNeeded(pod *v1.Pod, container *v1.Container, containerInfo *dockertypes.ContainerJSON) error {
	// Compare current API version with expected api version.
	result, err := dm.checkDockerAPIVersion(dockerV110APIVersion)
	if err != nil {
		return fmt.Errorf("Failed to check docker api version: %v", err)
	}
	// If current api version is older than OOMScoreAdj requested, use the old way.
	if result < 0 {
		if err := dm.applyOOMScoreAdj(pod, container, containerInfo); err != nil {
			return fmt.Errorf("Failed to apply oom-score-adj to container %q- %v", containerInfo.Name, err)
		}
	}

	return nil
}

func (dm *DockerManager) calculateOomScoreAdj(pod *v1.Pod, container *v1.Container) int {
	// Set OOM score of the container based on the priority of the container.
	// Processes in lower-priority pods should be killed first if the system runs out of memory.
	// The main pod infrastructure container is considered high priority, since if it is killed the
	// whole pod will die.
	var oomScoreAdj int
	if container.Name == PodInfraContainerName {
		oomScoreAdj = qos.PodInfraOOMAdj
	} else {
		oomScoreAdj = qos.GetContainerOOMScoreAdjust(pod, container, int64(dm.machineInfo.MemoryCapacity))

	}

	return oomScoreAdj
}

// versionInfo wraps api version and daemon version.
type versionInfo struct {
	apiVersion    kubecontainer.Version
	daemonVersion kubecontainer.Version
}

// checkDockerAPIVersion checks current docker API version against expected version.
// Return:
// 1 : newer than expected version
// -1: older than expected version
// 0 : same version
func (dm *DockerManager) checkDockerAPIVersion(expectedVersion string) (int, error) {

	value, err := dm.versionCache.Get(dm.machineInfo.MachineID)
	if err != nil {
		return 0, err
	}
	apiVersion := value.(versionInfo).apiVersion
	result, err := apiVersion.Compare(expectedVersion)
	if err != nil {
		return 0, fmt.Errorf("Failed to compare current docker api version %v with OOMScoreAdj supported Docker version %q - %v",
			apiVersion, expectedVersion, err)
	}
	return result, nil
}

// RewriteResolvFile rewrites resolv.conf file generated by docker.
// Exported for reusing in dockershim.
func RewriteResolvFile(resolvFilePath string, dns []string, dnsSearch []string, useClusterFirstPolicy bool) error {
	if len(resolvFilePath) == 0 {
		glog.Errorf("ResolvConfPath is empty.")
		return nil
	}

	if _, err := os.Stat(resolvFilePath); os.IsNotExist(err) {
		return fmt.Errorf("ResolvConfPath %q does not exist", resolvFilePath)
	}

	var resolvFileContent []string

	for _, srv := range dns {
		resolvFileContent = append(resolvFileContent, "nameserver "+srv)
	}

	if len(dnsSearch) > 0 {
		resolvFileContent = append(resolvFileContent, "search "+strings.Join(dnsSearch, " "))
	}

	if len(resolvFileContent) > 0 {
		if useClusterFirstPolicy {
			resolvFileContent = append(resolvFileContent, ndotsDNSOption)
		}

		resolvFileContentStr := strings.Join(resolvFileContent, "\n")
		resolvFileContentStr += "\n"

		glog.V(4).Infof("Will attempt to re-write config file %s with: \n%s", resolvFilePath, resolvFileContent)
		if err := rewriteFile(resolvFilePath, resolvFileContentStr); err != nil {
			glog.Errorf("resolv.conf could not be updated: %v", err)
			return err
		}
	}

	return nil
}

func rewriteFile(filePath, stringToWrite string) error {
	f, err := os.OpenFile(filePath, os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(stringToWrite)
	return err
}

// createPodInfraContainer starts the pod infra container for a pod. Returns the docker container ID of the newly created container.
// If any error occurs in this function, it will return a brief error and a detailed error message.
func (dm *DockerManager) createPodInfraContainer(pod *v1.Pod) (kubecontainer.DockerID, error, string) {
	start := time.Now()
	defer func() {
		metrics.ContainerManagerLatency.WithLabelValues("createPodInfraContainer").Observe(metrics.SinceInMicroseconds(start))
	}()
	// Use host networking if specified.
	netNamespace := ""
	var ports []v1.ContainerPort

	if kubecontainer.IsHostNetworkPod(pod) {
		netNamespace = namespaceModeHost
	} else if dm.pluginDisablesDockerNetworking() {
		netNamespace = "none"
	} else {
		// Docker only exports ports from the pod infra container.  Let's
		// collect all of the relevant ports and export them.
		for _, container := range pod.Spec.InitContainers {
			ports = append(ports, container.Ports...)
		}
		for _, container := range pod.Spec.Containers {
			ports = append(ports, container.Ports...)
		}
	}

	container := &v1.Container{
		Name:            PodInfraContainerName,
		Image:           dm.podInfraContainerImage,
		Ports:           ports,
		ImagePullPolicy: podInfraContainerImagePullPolicy,
		Env:             dm.podInfraContainerEnv,
	}

	// No pod secrets for the infra container.
	// The message isn't needed for the Infra container
	imageRef, msg, err := dm.imagePuller.EnsureImageExists(pod, container, nil)
	if err != nil {
		return "", err, msg
	}

	// Currently we don't care about restart count of infra container, just set it to 0.
	id, err := dm.runContainerInPod(pod, container, netNamespace, getIPCMode(pod), getPidMode(pod), "", imageRef, 0)
	if err != nil {
		return "", kubecontainer.ErrRunContainer, err.Error()
	}

	return kubecontainer.DockerID(id.ID), nil, ""
}

// Structure keeping information on changes that need to happen for a pod. The semantics is as follows:
// - startInfraContainer is true if new Infra Containers have to be started and old one (if running) killed.
//   Additionally if it is true then containersToKeep have to be empty
// - infraContainerId have to be set if and only if startInfraContainer is false. It stores dockerID of running Infra Container
// - containersToStart keeps indices of Specs of containers that have to be started and reasons why containers will be started.
// - containersToKeep stores mapping from dockerIDs of running containers to indices of their Specs for containers that
//   should be kept running. If startInfraContainer is false then it contains an entry for infraContainerId (mapped to -1).
//   It shouldn't be the case where containersToStart is empty and containersToKeep contains only infraContainerId. In such case
//   Infra Container should be killed, hence it's removed from this map.
// - all init containers are stored in initContainersToKeep
// - all running containers which are NOT contained in containersToKeep and initContainersToKeep should be killed.
type podContainerChangesSpec struct {
	StartInfraContainer  bool
	InfraChanged         bool
	InfraContainerId     kubecontainer.DockerID
	InitFailed           bool
	InitContainersToKeep map[kubecontainer.DockerID]int
	ContainersToStart    map[int]string
	ContainersToKeep     map[kubecontainer.DockerID]int
}

func (dm *DockerManager) computePodContainerChanges(pod *v1.Pod, podStatus *kubecontainer.PodStatus) (podContainerChangesSpec, error) {
	start := time.Now()
	defer func() {
		metrics.ContainerManagerLatency.WithLabelValues("computePodContainerChanges").Observe(metrics.SinceInMicroseconds(start))
	}()
	glog.V(5).Infof("Syncing Pod %q: %#v", format.Pod(pod), pod)

	containersToStart := make(map[int]string)
	containersToKeep := make(map[kubecontainer.DockerID]int)

	var err error
	var podInfraContainerID kubecontainer.DockerID
	var changed bool
	podInfraContainerStatus := podStatus.FindContainerStatusByName(PodInfraContainerName)
	if podInfraContainerStatus != nil && podInfraContainerStatus.State == kubecontainer.ContainerStateRunning {
		glog.V(4).Infof("Found pod infra container for %q", format.Pod(pod))
		changed, err = dm.podInfraContainerChanged(pod, podInfraContainerStatus)
		if err != nil {
			return podContainerChangesSpec{}, err
		}
	}

	createPodInfraContainer := true
	if podInfraContainerStatus == nil || podInfraContainerStatus.State != kubecontainer.ContainerStateRunning {
		glog.V(2).Infof("Need to restart pod infra container for %q because it is not found", format.Pod(pod))
	} else if changed {
		glog.V(2).Infof("Need to restart pod infra container for %q because it is changed", format.Pod(pod))
	} else {
		glog.V(4).Infof("Pod infra container looks good, keep it %q", format.Pod(pod))
		createPodInfraContainer = false
		podInfraContainerID = kubecontainer.DockerID(podInfraContainerStatus.ID.ID)
		containersToKeep[podInfraContainerID] = -1
	}

	// check the status of the init containers
	initFailed := false
	initContainersToKeep := make(map[kubecontainer.DockerID]int)
	// always reset the init containers if the pod is reset
	if !createPodInfraContainer {
		// keep all successfully completed containers up to and including the first failing container
	Containers:
		for i, container := range pod.Spec.InitContainers {
			containerStatus := podStatus.FindContainerStatusByName(container.Name)
			switch {
			case containerStatus == nil:
				continue
			case containerStatus.State == kubecontainer.ContainerStateRunning:
				initContainersToKeep[kubecontainer.DockerID(containerStatus.ID.ID)] = i
			case containerStatus.State == kubecontainer.ContainerStateExited:
				initContainersToKeep[kubecontainer.DockerID(containerStatus.ID.ID)] = i
				// TODO: should we abstract the "did the init container fail" check?
				if containerStatus.ExitCode != 0 {
					initFailed = true
					break Containers
				}
			}
		}
	}

	// check the status of the containers
	for index, container := range pod.Spec.Containers {

		containerStatus := podStatus.FindContainerStatusByName(container.Name)
		if containerStatus == nil || containerStatus.State != kubecontainer.ContainerStateRunning {
			if kubecontainer.ShouldContainerBeRestarted(&container, pod, podStatus) {
				// If we are here it means that the container is dead and should be restarted, or never existed and should
				// be created. We may be inserting this ID again if the container has changed and it has
				// RestartPolicy::Always, but it's not a big deal.
				message := fmt.Sprintf("Container %+v is dead, but RestartPolicy says that we should restart it.", container)
				glog.V(3).Info(message)
				containersToStart[index] = message
			}
			continue
		}

		containerID := kubecontainer.DockerID(containerStatus.ID.ID)
		glog.V(3).Infof("pod %q container %q exists as %v", format.Pod(pod), container.Name, containerID)

		if createPodInfraContainer {
			// createPodInfraContainer == true and Container exists
			// If we're creating infra container everything will be killed anyway
			// If RestartPolicy is Always or OnFailure we restart containers that were running before we
			// killed them when restarting Infra Container.
			if pod.Spec.RestartPolicy != v1.RestartPolicyNever {
				message := fmt.Sprintf("Infra Container is being recreated. %q will be restarted.", container.Name)
				glog.V(1).Info(message)
				containersToStart[index] = message
			}
			continue
		}

		if initFailed {
			// initialization failed and Container exists
			// If we have an initialization failure everything will be killed anyway
			// If RestartPolicy is Always or OnFailure we restart containers that were running before we
			// killed them when re-running initialization
			if pod.Spec.RestartPolicy != v1.RestartPolicyNever {
				message := fmt.Sprintf("Failed to initialize pod. %q will be restarted.", container.Name)
				glog.V(1).Info(message)
				containersToStart[index] = message
			}
			continue
		}

		// At this point, the container is running and pod infra container is good.
		// We will look for changes and check healthiness for the container.
		expectedHash := kubecontainer.HashContainerLegacy(&container)
		hash := containerStatus.Hash
		containerChanged := hash != 0 && hash != expectedHash
		if containerChanged {
			message := fmt.Sprintf("pod %q container %q hash changed (%d vs %d), it will be killed and re-created.", format.Pod(pod), container.Name, hash, expectedHash)
			glog.Info(message)
			containersToStart[index] = message
			continue
		}

		liveness, found := dm.livenessManager.Get(containerStatus.ID)
		if !found || liveness == proberesults.Success {
			containersToKeep[containerID] = index
			continue
		}
		if pod.Spec.RestartPolicy != v1.RestartPolicyNever {
			message := fmt.Sprintf("pod %q container %q is unhealthy, it will be killed and re-created.", format.Pod(pod), container.Name)
			glog.Info(message)
			containersToStart[index] = message
		}
	}

	// After the loop one of the following should be true:
	// - createPodInfraContainer is true and containersToKeep is empty.
	// (In fact, when createPodInfraContainer is false, containersToKeep will not be touched).
	// - createPodInfraContainer is false and containersToKeep contains at least ID of Infra Container

	// If Infra container is the last running one, we don't want to keep it, and we don't want to
	// keep any init containers.
	if !createPodInfraContainer && len(containersToStart) == 0 && len(containersToKeep) == 1 {
		containersToKeep = make(map[kubecontainer.DockerID]int)
		initContainersToKeep = make(map[kubecontainer.DockerID]int)
	}

	return podContainerChangesSpec{
		StartInfraContainer:  createPodInfraContainer,
		InfraChanged:         changed,
		InfraContainerId:     podInfraContainerID,
		InitFailed:           initFailed,
		InitContainersToKeep: initContainersToKeep,
		ContainersToStart:    containersToStart,
		ContainersToKeep:     containersToKeep,
	}, nil
}

// Sync the running pod to match the specified desired pod.
func (dm *DockerManager) SyncPod(pod *v1.Pod, _ v1.PodStatus, podStatus *kubecontainer.PodStatus, pullSecrets []v1.Secret, backOff *flowcontrol.Backoff) (result kubecontainer.PodSyncResult) {
	start := time.Now()
	defer func() {
		metrics.ContainerManagerLatency.WithLabelValues("SyncPod").Observe(metrics.SinceInMicroseconds(start))
	}()

	containerChanges, err := dm.computePodContainerChanges(pod, podStatus)
	if err != nil {
		result.Fail(err)
		return
	}
	glog.V(3).Infof("Got container changes for pod %q: %+v", format.Pod(pod), containerChanges)

	if containerChanges.InfraChanged {
		dm.recorder.Eventf(pod, v1.EventTypeNormal, "InfraChanged", "Pod infrastructure changed, it will be killed and re-created.")
	}
	if containerChanges.StartInfraContainer || (len(containerChanges.ContainersToKeep) == 0 && len(containerChanges.ContainersToStart) == 0) {
		if len(containerChanges.ContainersToKeep) == 0 && len(containerChanges.ContainersToStart) == 0 {
			glog.V(4).Infof("Killing Infra Container for %q because all other containers are dead.", format.Pod(pod))
		} else {
			glog.V(4).Infof("Killing Infra Container for %q, will start new one", format.Pod(pod))
		}

		// Get list of running container(s) to kill
		podToKill := kubecontainer.ConvertPodStatusToRunningPod(dm.Type(), podStatus)

		// If there are dead network containers, also kill them to ensure
		// their network resources get released and are available to be
		// re-used by new net containers
		for _, containerStatus := range podStatus.ContainerStatuses {
			if containerIsNetworked(containerStatus.Name) && containerStatus.State == kubecontainer.ContainerStateExited {
				container := &kubecontainer.Container{
					ID:      containerStatus.ID,
					Name:    containerStatus.Name,
					Image:   containerStatus.Image,
					ImageID: containerStatus.ImageID,
					Hash:    containerStatus.Hash,
					State:   containerStatus.State,
				}
				podToKill.Containers = append(podToKill.Containers, container)
			}
		}

		// Killing phase: if we want to start new infra container, or nothing is running kill everything (including infra container)
		// TODO(random-liu): We'll use pod status directly in the future
		killResult := dm.killPodWithSyncResult(pod, podToKill, nil)
		result.AddPodSyncResult(killResult)
		if killResult.Error() != nil {
			return
		}
	} else {
		// Otherwise kill any running containers in this pod which are not specified as ones to keep.
		runningContainerStatues := podStatus.GetRunningContainerStatuses()
		for _, containerStatus := range runningContainerStatues {
			_, keep := containerChanges.ContainersToKeep[kubecontainer.DockerID(containerStatus.ID.ID)]
			_, keepInit := containerChanges.InitContainersToKeep[kubecontainer.DockerID(containerStatus.ID.ID)]
			if !keep && !keepInit {
				glog.V(3).Infof("Killing unwanted container %q(id=%q) for pod %q", containerStatus.Name, containerStatus.ID, format.Pod(pod))
				// attempt to find the appropriate container policy
				var podContainer *v1.Container
				var killMessage string
				for i, c := range pod.Spec.Containers {
					if c.Name == containerStatus.Name {
						podContainer = &pod.Spec.Containers[i]
						killMessage = containerChanges.ContainersToStart[i]
						break
					}
				}
				killContainerResult := kubecontainer.NewSyncResult(kubecontainer.KillContainer, containerStatus.Name)
				result.AddSyncResult(killContainerResult)
				if err := dm.KillContainerInPod(containerStatus.ID, podContainer, pod, killMessage, nil); err != nil {
					killContainerResult.Fail(kubecontainer.ErrKillContainer, err.Error())
					glog.Errorf("Error killing container %q(id=%q) for pod %q: %v", containerStatus.Name, containerStatus.ID, format.Pod(pod), err)
					return
				}
			}
		}
	}

	// Keep terminated init containers fairly aggressively controlled
	dm.pruneInitContainersBeforeStart(pod, podStatus, containerChanges.InitContainersToKeep)

	// We pass the value of the podIP down to runContainerInPod, which in turn
	// passes it to various other functions, in order to facilitate
	// functionality that requires this value (hosts file and downward API)
	// and avoid races determining the pod IP in cases where a container
	// requires restart but the podIP isn't in the status manager yet.
	//
	// We default to the IP in the passed-in pod status, and overwrite it if the
	// infra container needs to be (re)started.
	podIP := ""
	if podStatus != nil {
		podIP = podStatus.IP
	}

	// If we should create infra container then we do it first.
	podInfraContainerID := containerChanges.InfraContainerId
	if containerChanges.StartInfraContainer && (len(containerChanges.ContainersToStart) > 0) {
		glog.V(4).Infof("Creating pod infra container for %q", format.Pod(pod))
		startContainerResult := kubecontainer.NewSyncResult(kubecontainer.StartContainer, PodInfraContainerName)
		result.AddSyncResult(startContainerResult)
		var msg string
		podInfraContainerID, err, msg = dm.createPodInfraContainer(pod)
		if err != nil {
			startContainerResult.Fail(err, msg)
			glog.Errorf("Failed to create pod infra container: %v; Skipping pod %q: %s", err, format.Pod(pod), msg)
			return
		}

		setupNetworkResult := kubecontainer.NewSyncResult(kubecontainer.SetupNetwork, kubecontainer.GetPodFullName(pod))
		result.AddSyncResult(setupNetworkResult)
		if !kubecontainer.IsHostNetworkPod(pod) {
			if err := dm.network.SetUpPod(pod.Namespace, pod.Name, podInfraContainerID.ContainerID(), pod.Annotations); err != nil {
				setupNetworkResult.Fail(kubecontainer.ErrSetupNetwork, err.Error())
				glog.Error(err)

				// Delete infra container
				killContainerResult := kubecontainer.NewSyncResult(kubecontainer.KillContainer, PodInfraContainerName)
				result.AddSyncResult(killContainerResult)
				if delErr := dm.KillContainerInPod(podInfraContainerID.ContainerID(), nil, pod, err.Error(), nil); delErr != nil {
					killContainerResult.Fail(kubecontainer.ErrKillContainer, delErr.Error())
					glog.Warningf("Clear infra container failed for pod %q: %v", format.Pod(pod), delErr)
				}
				return
			}

			// Setup the host interface unless the pod is on the host's network (FIXME: move to networkPlugin when ready)
			podInfraContainer, err := dm.client.InspectContainer(string(podInfraContainerID))
			if err != nil {
				glog.Errorf("Failed to inspect pod infra container: %v; Skipping pod %q", err, format.Pod(pod))
				result.Fail(err)
				return
			}

			if dm.configureHairpinMode {
				if err = hairpin.SetUpContainerPid(podInfraContainer.State.Pid, knetwork.DefaultInterfaceName); err != nil {
					glog.Warningf("Hairpin setup failed for pod %q: %v", format.Pod(pod), err)
				}
			}

			// Overwrite the podIP passed in the pod status, since we just started the infra container.
			podIP, err = dm.determineContainerIP(pod.Namespace, pod.Name, podInfraContainer)
			if err != nil {
				glog.Errorf("Network error: %v; Skipping pod %q", err, format.Pod(pod))
				result.Fail(err)
				return
			}
			glog.Infof("Determined pod ip after infra change: %q: %q", format.Pod(pod), podIP)
		}
	}

	next, status, done := findActiveInitContainer(pod, podStatus)
	if status != nil {
		if status.ExitCode != 0 {
			// container initialization has failed, flag the pod as failed
			initContainerResult := kubecontainer.NewSyncResult(kubecontainer.InitContainer, status.Name)
			initContainerResult.Fail(kubecontainer.ErrRunInitContainer, fmt.Sprintf("init container %q exited with %d", status.Name, status.ExitCode))
			result.AddSyncResult(initContainerResult)
			if pod.Spec.RestartPolicy == v1.RestartPolicyNever {
				utilruntime.HandleError(fmt.Errorf("error running pod %q init container %q, restart=Never: %#v", format.Pod(pod), status.Name, status))
				return
			}
			utilruntime.HandleError(fmt.Errorf("Error running pod %q init container %q, restarting: %#v", format.Pod(pod), status.Name, status))
		}
	}

	// Note: when configuring the pod's containers anything that can be configured by pointing
	// to the namespace of the infra container should use namespaceMode.  This includes things like the net namespace
	// and IPC namespace.  PID mode cannot point to another container right now.
	// See createPodInfraContainer for infra container setup.
	namespaceMode := fmt.Sprintf("container:%v", podInfraContainerID)
	pidMode := getPidMode(pod)

	if next != nil {
		if len(containerChanges.ContainersToStart) == 0 {
			glog.V(4).Infof("No containers to start, stopping at init container %+v in pod %v", next.Name, format.Pod(pod))
			return
		}

		// If we need to start the next container, do so now then exit
		container := next
		startContainerResult := kubecontainer.NewSyncResult(kubecontainer.StartContainer, container.Name)
		result.AddSyncResult(startContainerResult)

		// containerChanges.StartInfraContainer causes the containers to be restarted for config reasons
		if !containerChanges.StartInfraContainer {
			isInBackOff, err, msg := dm.doBackOff(pod, container, podStatus, backOff)
			if isInBackOff {
				startContainerResult.Fail(err, msg)
				glog.V(4).Infof("Backing Off restarting init container %+v in pod %v", container, format.Pod(pod))
				return
			}
		}

		glog.V(4).Infof("Creating init container %+v in pod %v", container, format.Pod(pod))
		if err, msg := dm.tryContainerStart(container, pod, podStatus, pullSecrets, namespaceMode, pidMode, podIP); err != nil {
			startContainerResult.Fail(err, msg)
			utilruntime.HandleError(fmt.Errorf("container start failed: %v: %s", err, msg))
			return
		}

		// Successfully started the container; clear the entry in the failure
		glog.V(4).Infof("Completed init container %q for pod %q", container.Name, format.Pod(pod))
		return
	}
	if !done {
		// init container still running
		glog.V(4).Infof("An init container is still running in pod %v", format.Pod(pod))
		return
	}
	if containerChanges.InitFailed {
		// init container still running
		glog.V(4).Infof("Not all init containers have succeeded for pod %v", format.Pod(pod))
		return
	}

	// Start regular containers
	for idx := range containerChanges.ContainersToStart {
		container := &pod.Spec.Containers[idx]
		startContainerResult := kubecontainer.NewSyncResult(kubecontainer.StartContainer, container.Name)
		result.AddSyncResult(startContainerResult)

		// containerChanges.StartInfraContainer causes the containers to be restarted for config reasons
		if !containerChanges.StartInfraContainer {
			isInBackOff, err, msg := dm.doBackOff(pod, container, podStatus, backOff)
			if isInBackOff {
				startContainerResult.Fail(err, msg)
				glog.V(4).Infof("Backing Off restarting container %+v in pod %v", container, format.Pod(pod))
				continue
			}
		}

		glog.V(4).Infof("Creating container %+v in pod %v", container, format.Pod(pod))
		if err, msg := dm.tryContainerStart(container, pod, podStatus, pullSecrets, namespaceMode, pidMode, podIP); err != nil {
			startContainerResult.Fail(err, msg)
			utilruntime.HandleError(fmt.Errorf("container start failed: %v: %s", err, msg))
			continue
		}
	}
	return
}

// tryContainerStart attempts to pull and start the container, returning an error and a reason string if the start
// was not successful.
func (dm *DockerManager) tryContainerStart(container *v1.Container, pod *v1.Pod, podStatus *kubecontainer.PodStatus, pullSecrets []v1.Secret, namespaceMode, pidMode, podIP string) (err error, reason string) {
	imageRef, msg, err := dm.imagePuller.EnsureImageExists(pod, container, pullSecrets)
	if err != nil {
		return err, msg
	}

	if container.SecurityContext != nil && container.SecurityContext.RunAsNonRoot != nil && *container.SecurityContext.RunAsNonRoot {
		err := dm.verifyNonRoot(container)
		if err != nil {
			return kubecontainer.ErrVerifyNonRoot, err.Error()
		}
	}

	// For a new container, the RestartCount should be 0
	restartCount := 0
	containerStatus := podStatus.FindContainerStatusByName(container.Name)
	if containerStatus != nil {
		restartCount = containerStatus.RestartCount + 1
	}

	// Allow override of networking mode for specific platforms (e.g. Windows)
	netMode := getNetworkingMode()
	if netMode == "" {
		// If not overriden, use the namespace mode
		netMode = namespaceMode
	}

	_, err = dm.runContainerInPod(pod, container, netMode, namespaceMode, pidMode, podIP, imageRef, restartCount)
	if err != nil {
		// TODO(bburns) : Perhaps blacklist a container after N failures?
		return kubecontainer.ErrRunContainer, err.Error()
	}
	return nil, ""
}

// pruneInitContainers ensures that before we begin creating init containers, we have reduced the number
// of outstanding init containers still present. This reduces load on the container garbage collector
// by only preserving the most recent terminated init container.
func (dm *DockerManager) pruneInitContainersBeforeStart(pod *v1.Pod, podStatus *kubecontainer.PodStatus, initContainersToKeep map[kubecontainer.DockerID]int) {
	// only the last execution of each init container should be preserved, and only preserve it if it is in the
	// list of init containers to keep.
	initContainerNames := sets.NewString()
	for _, container := range pod.Spec.InitContainers {
		initContainerNames.Insert(container.Name)
	}
	for name := range initContainerNames {
		count := 0
		for _, status := range podStatus.ContainerStatuses {
			if status.Name != name || !initContainerNames.Has(status.Name) || status.State != kubecontainer.ContainerStateExited {
				continue
			}
			count++
			// keep the first init container for this name
			if count == 1 {
				continue
			}
			// if there is a reason to preserve the older container, do so
			if _, ok := initContainersToKeep[kubecontainer.DockerID(status.ID.ID)]; ok {
				continue
			}

			// prune all other init containers that match this container name
			// TODO: we may not need aggressive pruning
			glog.V(4).Infof("Removing init container %q instance %q %d", status.Name, status.ID.ID, count)
			if err := dm.client.RemoveContainer(status.ID.ID, dockertypes.ContainerRemoveOptions{RemoveVolumes: true}); err != nil {
				if IsContainerNotFoundError(err) {
					count--
					continue
				}
				utilruntime.HandleError(fmt.Errorf("failed to remove pod init container %q: %v; Skipping pod %q", status.Name, err, format.Pod(pod)))
				// TODO: report serious errors
				continue
			}

			// remove any references to this container
			if _, ok := dm.containerRefManager.GetRef(status.ID); ok {
				dm.containerRefManager.ClearRef(status.ID)
			} else {
				glog.Warningf("No ref for pod '%q'", pod.Name)
			}
		}
	}
}

// findActiveInitContainer returns the status of the last failed container, the next init container to
// start, or done if there are no further init containers. Status is only returned if an init container
// failed, in which case next will point to the current container.
func findActiveInitContainer(pod *v1.Pod, podStatus *kubecontainer.PodStatus) (next *v1.Container, status *kubecontainer.ContainerStatus, done bool) {
	if len(pod.Spec.InitContainers) == 0 {
		return nil, nil, true
	}

	for i := len(pod.Spec.InitContainers) - 1; i >= 0; i-- {
		container := &pod.Spec.InitContainers[i]
		status := podStatus.FindContainerStatusByName(container.Name)
		switch {
		case status == nil:
			continue
		case status.State == kubecontainer.ContainerStateRunning:
			return nil, nil, false
		case status.State == kubecontainer.ContainerStateExited:
			switch {
			// the container has failed, we'll have to retry
			case status.ExitCode != 0:
				return &pod.Spec.InitContainers[i], status, false
			// all init containers successful
			case i == (len(pod.Spec.InitContainers) - 1):
				return nil, nil, true
			// all containers up to i successful, go to i+1
			default:
				return &pod.Spec.InitContainers[i+1], nil, false
			}
		}
	}

	return &pod.Spec.InitContainers[0], nil, false
}

// verifyNonRoot returns an error if the container or image will run as the root user.
func (dm *DockerManager) verifyNonRoot(container *v1.Container) error {
	if securitycontext.HasRunAsUser(container) {
		if securitycontext.HasRootRunAsUser(container) {
			return fmt.Errorf("container's runAsUser breaks non-root policy")
		}
		return nil
	}

	imgRoot, err := dm.isImageRoot(container.Image)
	if err != nil {
		return fmt.Errorf("can't tell if image runs as root: %v", err)
	}
	if imgRoot {
		return fmt.Errorf("container has no runAsUser and image will run as root")
	}

	return nil
}

// isImageRoot returns true if the user directive is not set on the image, the user is set to 0
// or the user is set to root.  If there is an error inspecting the image this method will return
// false and return the error.
func (dm *DockerManager) isImageRoot(image string) (bool, error) {
	img, err := dm.client.InspectImageByRef(image)
	if err != nil {
		return false, err
	}
	if img == nil || img.Config == nil {
		return false, fmt.Errorf("unable to inspect image %s, nil Config", image)
	}

	user := GetUserFromImageUser(img.Config.User)
	// if no user is defined container will run as root
	if user == "" {
		return true, nil
	}
	// do not allow non-numeric user directives
	uid, err := strconv.Atoi(user)
	if err != nil {
		return false, fmt.Errorf("non-numeric user (%s) is not allowed", user)
	}
	// user is numeric, check for 0
	return uid == 0, nil
}

// GetUserFromImageUser splits the user out of an user:group string.
func GetUserFromImageUser(id string) string {
	if id == "" {
		return id
	}
	// split instances where the id may contain user:group
	if strings.Contains(id, ":") {
		return strings.Split(id, ":")[0]
	}
	// no group, just return the id
	return id
}

// If all instances of a container are garbage collected, doBackOff will also return false, which means the container may be restarted before the
// backoff deadline. However, because that won't cause error and the chance is really slim, we can just ignore it for now.
// If a container is still in backoff, the function will return a brief backoff error and a detailed error message.
func (dm *DockerManager) doBackOff(pod *v1.Pod, container *v1.Container, podStatus *kubecontainer.PodStatus, backOff *flowcontrol.Backoff) (bool, error, string) {
	var cStatus *kubecontainer.ContainerStatus
	// Use the finished time of the latest exited container as the start point to calculate whether to do back-off.
	// TODO(random-liu): Better define backoff start point; add unit and e2e test after we finalize this. (See github issue #22240)
	for _, c := range podStatus.ContainerStatuses {
		if c.Name == container.Name && c.State == kubecontainer.ContainerStateExited {
			cStatus = c
			break
		}
	}
	if cStatus != nil {
		glog.Infof("checking backoff for container %q in pod %q", container.Name, pod.Name)
		ts := cStatus.FinishedAt
		// found a container that requires backoff
		dockerName := KubeletContainerName{
			PodFullName:   kubecontainer.GetPodFullName(pod),
			PodUID:        pod.UID,
			ContainerName: container.Name,
		}
		stableName, _, _ := BuildDockerName(dockerName, container)
		if backOff.IsInBackOffSince(stableName, ts) {
			if ref, err := kubecontainer.GenerateContainerRef(pod, container); err == nil {
				dm.recorder.Eventf(ref, v1.EventTypeWarning, events.BackOffStartContainer, "Back-off restarting failed docker container")
			}
			err := fmt.Errorf("Back-off %s restarting failed container=%s pod=%s", backOff.Get(stableName), container.Name, format.Pod(pod))
			glog.Infof("%s", err.Error())
			return true, kubecontainer.ErrCrashLoopBackOff, err.Error()
		}
		backOff.Next(stableName, ts)
	}
	return false, nil, ""
}

// getPidMode returns the pid mode to use on the docker container based on pod.Spec.HostPID.
func getPidMode(pod *v1.Pod) string {
	pidMode := ""
	if pod.Spec.HostPID {
		pidMode = namespaceModeHost
	}
	return pidMode
}

// getIPCMode returns the ipc mode to use on the docker container based on pod.Spec.HostIPC.
func getIPCMode(pod *v1.Pod) string {
	ipcMode := ""
	if pod.Spec.HostIPC {
		ipcMode = namespaceModeHost
	}
	return ipcMode
}

func (dm *DockerManager) DeleteContainer(containerID kubecontainer.ContainerID) error {
	return dm.containerGC.deleteContainer(containerID.ID)
}

// GetNetNS returns the network namespace path for the given container
func (dm *DockerManager) GetNetNS(containerID kubecontainer.ContainerID) (string, error) {
	inspectResult, err := dm.client.InspectContainer(containerID.ID)
	if err != nil {
		glog.Errorf("Error inspecting container: '%v'", err)
		return "", err
	}
	if inspectResult.State.Pid == 0 {
		// Docker reports pid 0 for an exited container. We can't use it to
		// check the network namespace, so return an empty string instead.
		glog.V(4).Infof("Cannot find network namespace for the terminated container %q", containerID.ID)
		return "", nil
	}

	netnsPath := fmt.Sprintf(DockerNetnsFmt, inspectResult.State.Pid)
	return netnsPath, nil
}

func (dm *DockerManager) GetPodContainerID(pod *kubecontainer.Pod) (kubecontainer.ContainerID, error) {
	for _, c := range pod.Containers {
		if c.Name == PodInfraContainerName {
			return c.ID, nil
		}
	}

	return kubecontainer.ContainerID{}, fmt.Errorf("Pod %s unknown to docker.", kubecontainer.BuildPodFullName(pod.Name, pod.Namespace))
}

// Garbage collection of dead containers
func (dm *DockerManager) GarbageCollect(gcPolicy kubecontainer.ContainerGCPolicy, allSourcesReady bool) error {
	return dm.containerGC.GarbageCollect(gcPolicy, allSourcesReady)
}

func (dm *DockerManager) GetPodStatus(uid kubetypes.UID, name, namespace string) (*kubecontainer.PodStatus, error) {
	podStatus := &kubecontainer.PodStatus{ID: uid, Name: name, Namespace: namespace}
	// Now we retain restart count of container as a docker label. Each time a container
	// restarts, pod will read the restart count from the registered dead container, increment
	// it to get the new restart count, and then add a label with the new restart count on
	// the newly started container.
	// However, there are some limitations of this method:
	//	1. When all dead containers were garbage collected, the container status could
	//	not get the historical value and would be *inaccurate*. Fortunately, the chance
	//	is really slim.
	//	2. When working with old version containers which have no restart count label,
	//	we can only assume their restart count is 0.
	// Anyhow, we only promised "best-effort" restart count reporting, we can just ignore
	// these limitations now.
	var containerStatuses []*kubecontainer.ContainerStatus
	// We have added labels like pod name and pod namespace, it seems that we can do filtered list here.
	// However, there may be some old containers without these labels, so at least now we can't do that.
	// TODO(random-liu): Do only one list and pass in the list result in the future
	// TODO(random-liu): Add filter when we are sure that all the containers have the labels
	containers, err := dm.client.ListContainers(dockertypes.ContainerListOptions{All: true})
	if err != nil {
		return podStatus, err
	}
	// Loop through list of running and exited docker containers to construct
	// the statuses. We assume docker returns a list of containers sorted in
	// reverse by time.
	// TODO: optimization: set maximum number of containers per container name to examine.
	for _, c := range containers {
		if len(c.Names) == 0 {
			continue
		}
		dockerName, _, err := ParseDockerName(c.Names[0])
		if err != nil {
			continue
		}
		if dockerName.PodUID != uid {
			continue
		}
		result, ip, err := dm.inspectContainer(c.ID, name, namespace)
		if err != nil {
			if IsContainerNotFoundError(err) {
				// https://github.com/kubernetes/kubernetes/issues/22541
				// Sometimes when docker's state is corrupt, a container can be listed
				// but couldn't be inspected. We fake a status for this container so
				// that we can still return a status for the pod to sync.
				result = &kubecontainer.ContainerStatus{
					ID:    kubecontainer.DockerID(c.ID).ContainerID(),
					Name:  dockerName.ContainerName,
					State: kubecontainer.ContainerStateUnknown,
				}
				glog.Errorf("Unable to inspect container %q: %v", c.ID, err)
			} else {
				return podStatus, err
			}
		}
		containerStatuses = append(containerStatuses, result)
		if containerProvidesPodIP(dockerName.ContainerName) && ip != "" {
			podStatus.IP = ip
		}
	}

	podStatus.ContainerStatuses = containerStatuses
	return podStatus, nil
}

// getVersionInfo returns apiVersion & daemonVersion of docker runtime
func (dm *DockerManager) getVersionInfo() (versionInfo, error) {
	apiVersion, err := dm.APIVersion()
	if err != nil {
		return versionInfo{}, err
	}
	daemonVersion, err := dm.Version()
	if err != nil {
		return versionInfo{}, err
	}
	return versionInfo{
		apiVersion:    apiVersion,
		daemonVersion: daemonVersion,
	}, nil
}

// Truncate the message if it exceeds max length.
func truncateMsg(msg string, max int) string {
	if len(msg) <= max {
		return msg
	}
	glog.V(2).Infof("Truncated %s", msg)
	const truncatedMsg = "..TRUNCATED.."
	begin := (max - len(truncatedMsg)) / 2
	end := len(msg) - (max - (len(truncatedMsg) + begin))
	return msg[:begin] + truncatedMsg + msg[end:]
}
