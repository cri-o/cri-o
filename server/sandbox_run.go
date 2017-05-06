package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/containers/storage/storage"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/opencontainers/selinux/go-selinux/label"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

var (
	podConflictRE = regexp.MustCompile(`already reserved for pod "([0-9a-z]+)"`)
)

// privilegedSandbox returns true if the sandbox configuration
// requires additional host privileges for the sandbox.
func (s *Server) privilegedSandbox(req *pb.RunPodSandboxRequest) bool {
	securityContext := req.GetConfig().GetLinux().GetSecurityContext()
	if securityContext == nil {
		return false
	}

	if securityContext.Privileged {
		return true
	}

	namespaceOptions := securityContext.GetNamespaceOptions()
	if namespaceOptions == nil {
		return false
	}

	if namespaceOptions.HostNetwork ||
		namespaceOptions.HostPid ||
		namespaceOptions.HostIpc {
		return true
	}

	return false
}

func (s *Server) runContainer(container *oci.Container, cgroupParent string) error {
	if err := s.runtime.CreateContainer(container, cgroupParent); err != nil {
		return err
	}

	if err := s.runtime.UpdateStatus(container); err != nil {
		return err
	}

	if err := s.runtime.StartContainer(container); err != nil {
		return err
	}

	if err := s.runtime.UpdateStatus(container); err != nil {
		return err
	}

	return nil
}

// RunPodSandbox creates and runs a pod-level sandbox.
func (s *Server) RunPodSandbox(ctx context.Context, req *pb.RunPodSandboxRequest) (resp *pb.RunPodSandboxResponse, err error) {
	s.updateLock.RLock()
	defer s.updateLock.RUnlock()

	logrus.Debugf("RunPodSandboxRequest %+v", req)
	var processLabel, mountLabel, netNsPath, resolvPath string
	// process req.Name
	kubeName := req.GetConfig().GetMetadata().Name
	if kubeName == "" {
		return nil, fmt.Errorf("PodSandboxConfig.Name should not be empty")
	}

	namespace := req.GetConfig().GetMetadata().Namespace
	attempt := req.GetConfig().GetMetadata().Attempt

	id, name, err := s.generatePodIDandName(kubeName, namespace, attempt)
	if err != nil {
		matches := podConflictRE.FindStringSubmatch(err.Error())
		if len(matches) != 2 {
			return nil, err
		}
		podID := matches[1]
		_, err = s.RemovePodSandbox(ctx, &pb.RemovePodSandboxRequest{PodSandboxId: podID})
		if err != nil {
			return nil, err
		}
		id, name, err = s.generatePodIDandName(kubeName, namespace, attempt)
		if err != nil {
			return nil, err
		}
	}

	_, containerName, err := s.generateContainerIDandName(name, "infra", attempt)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			s.releasePodName(name)
		}
	}()

	podContainer, err := s.storageRuntimeServer.CreatePodSandbox(s.imageContext,
		name, id,
		s.config.PauseImage, "",
		containerName,
		req.GetConfig().GetMetadata().Name,
		req.GetConfig().GetMetadata().Uid,
		namespace,
		attempt,
		nil)
	if err == storage.ErrDuplicateName {
		return nil, fmt.Errorf("pod sandbox with name %q already exists", name)
	}
	if err != nil {
		return nil, fmt.Errorf("error creating pod sandbox with name %q: %v", name, err)
	}
	defer func() {
		if err != nil {
			if err2 := s.storageRuntimeServer.RemovePodSandbox(id); err2 != nil {
				logrus.Warnf("couldn't cleanup pod sandbox %q: %v", id, err2)
			}
		}
	}()

	// TODO: factor generating/updating the spec into something other projects can vendor

	// creates a spec Generator with the default spec.
	g := generate.New()

	// setup defaults for the pod sandbox
	g.SetRootReadonly(true)
	if s.config.PauseCommand == "" {
		if podContainer.Config != nil {
			g.SetProcessArgs(podContainer.Config.Config.Cmd)
		} else {
			g.SetProcessArgs([]string{podInfraCommand})
		}
	} else {
		g.SetProcessArgs([]string{s.config.PauseCommand})
	}

	// set hostname
	hostname := req.GetConfig().Hostname
	if hostname != "" {
		g.SetHostname(hostname)
	}

	// set DNS options
	if req.GetConfig().GetDnsConfig() != nil {
		dnsServers := req.GetConfig().GetDnsConfig().Servers
		dnsSearches := req.GetConfig().GetDnsConfig().Searches
		dnsOptions := req.GetConfig().GetDnsConfig().Options
		resolvPath = fmt.Sprintf("%s/resolv.conf", podContainer.RunDir)
		err = parseDNSOptions(dnsServers, dnsSearches, dnsOptions, resolvPath)
		if err != nil {
			err1 := removeFile(resolvPath)
			if err1 != nil {
				err = err1
				return nil, fmt.Errorf("%v; failed to remove %s: %v", err, resolvPath, err1)
			}
			return nil, err
		}
		g.AddBindMount(resolvPath, "/etc/resolv.conf", []string{"ro"})
	}

	// add metadata
	metadata := req.GetConfig().GetMetadata()
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}

	// add labels
	labels := req.GetConfig().GetLabels()
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return nil, err
	}

	// add annotations
	annotations := req.GetConfig().GetAnnotations()
	annotationsJSON, err := json.Marshal(annotations)
	if err != nil {
		return nil, err
	}

	// set log directory
	logDir := req.GetConfig().LogDirectory
	if logDir == "" {
		logDir = filepath.Join(s.config.LogDir, id)
	}
	if err = os.MkdirAll(logDir, 0700); err != nil {
		return nil, err
	}
	// This should always be absolute from k8s.
	if !filepath.IsAbs(logDir) {
		return nil, fmt.Errorf("requested logDir for sbox id %s is a relative path: %s", id, logDir)
	}

	// Don't use SELinux separation with Host Pid or IPC Namespace,
	if !req.GetConfig().GetLinux().GetSecurityContext().GetNamespaceOptions().HostPid && !req.GetConfig().GetLinux().GetSecurityContext().GetNamespaceOptions().HostIpc {
		processLabel, mountLabel, err = getSELinuxLabels(nil)
		if err != nil {
			return nil, err
		}
		g.SetProcessSelinuxLabel(processLabel)
		g.SetLinuxMountLabel(mountLabel)
	}

	// create shm mount for the pod containers.
	var shmPath string
	if req.GetConfig().GetLinux().GetSecurityContext().GetNamespaceOptions().HostIpc {
		shmPath = "/dev/shm"
	} else {
		shmPath, err = setupShm(podContainer.RunDir, mountLabel)
		if err != nil {
			return nil, err
		}
		defer func() {
			if err != nil {
				if err2 := syscall.Unmount(shmPath, syscall.MNT_DETACH); err2 != nil {
					logrus.Warnf("failed to unmount shm for pod: %v", err2)
				}
			}
		}()
	}

	err = s.setPodSandboxMountLabel(id, mountLabel)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			s.releaseContainerName(containerName)
		}
	}()

	if err = s.ctrIDIndex.Add(id); err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			if err2 := s.ctrIDIndex.Delete(id); err2 != nil {
				logrus.Warnf("couldn't delete ctr id %s from idIndex", id)
			}
		}
	}()

	// set log path inside log directory
	logPath := filepath.Join(logDir, id+".log")

	// Handle https://issues.k8s.io/44043
	if err := ensureSaneLogPath(logPath); err != nil {
		return nil, err
	}

	privileged := s.privilegedSandbox(req)
	g.AddAnnotation("ocid/metadata", string(metadataJSON))
	g.AddAnnotation("ocid/labels", string(labelsJSON))
	g.AddAnnotation("ocid/annotations", string(annotationsJSON))
	g.AddAnnotation("ocid/log_path", logPath)
	g.AddAnnotation("ocid/name", name)
	g.AddAnnotation("ocid/container_type", containerTypeSandbox)
	g.AddAnnotation("ocid/sandbox_id", id)
	g.AddAnnotation("ocid/container_name", containerName)
	g.AddAnnotation("ocid/container_id", id)
	g.AddAnnotation("ocid/shm_path", shmPath)
	g.AddAnnotation("ocid/privileged_runtime", fmt.Sprintf("%v", privileged))
	g.AddAnnotation("ocid/resolv_path", resolvPath)
	g.AddAnnotation("ocid/hostname", hostname)

	sb := &sandbox{
		id:           id,
		namespace:    namespace,
		name:         name,
		kubeName:     kubeName,
		logDir:       logDir,
		labels:       labels,
		annotations:  annotations,
		containers:   oci.NewMemoryStore(),
		processLabel: processLabel,
		mountLabel:   mountLabel,
		metadata:     metadata,
		shmPath:      shmPath,
		privileged:   privileged,
		resolvPath:   resolvPath,
		hostname:     hostname,
	}

	defer func() {
		if err != nil {
			s.removeSandbox(id)
			if err2 := s.podIDIndex.Delete(id); err2 != nil {
				logrus.Warnf("couldn't delete pod id %s from idIndex", id)
			}
		}
	}()

	s.addSandbox(sb)
	if err = s.podIDIndex.Add(id); err != nil {
		return nil, err
	}

	for k, v := range annotations {
		g.AddAnnotation(k, v)
	}

	// extract linux sysctls from annotations and pass down to oci runtime
	safe, unsafe, err := SysctlsFromPodAnnotations(annotations)
	if err != nil {
		return nil, err
	}
	for _, sysctl := range safe {
		g.AddLinuxSysctl(sysctl.Name, sysctl.Value)
	}
	for _, sysctl := range unsafe {
		g.AddLinuxSysctl(sysctl.Name, sysctl.Value)
	}

	// setup cgroup settings
	cgroupParent := req.GetConfig().GetLinux().CgroupParent
	if cgroupParent != "" {
		if s.config.CgroupManager == "systemd" {
			cgPath := cgroupParent + ":" + "ocid" + ":" + id
			g.SetLinuxCgroupsPath(cgPath)

		} else {
			g.SetLinuxCgroupsPath(cgroupParent + "/" + id)

		}
		sb.cgroupParent = cgroupParent
	}

	hostNetwork := req.GetConfig().GetLinux().GetSecurityContext().GetNamespaceOptions().HostNetwork

	// set up namespaces
	if hostNetwork {
		err = g.RemoveLinuxNamespace("network")
		if err != nil {
			return nil, err
		}

		netNsPath, err = hostNetNsPath()
		if err != nil {
			return nil, err
		}
	} else {
		// Create the sandbox network namespace
		if err = sb.netNsCreate(); err != nil {
			return nil, err
		}

		defer func() {
			if err == nil {
				return
			}

			if netnsErr := sb.netNsRemove(); netnsErr != nil {
				logrus.Warnf("Failed to remove networking namespace: %v", netnsErr)
			}
		}()

		// Pass the created namespace path to the runtime
		err = g.AddOrReplaceLinuxNamespace("network", sb.netNsPath())
		if err != nil {
			return nil, err
		}

		netNsPath = sb.netNsPath()
	}

	if req.GetConfig().GetLinux().GetSecurityContext().GetNamespaceOptions().HostPid {
		err = g.RemoveLinuxNamespace("pid")
		if err != nil {
			return nil, err
		}
	}

	if req.GetConfig().GetLinux().GetSecurityContext().GetNamespaceOptions().HostIpc {
		err = g.RemoveLinuxNamespace("ipc")
		if err != nil {
			return nil, err
		}
	}

	if !s.seccompEnabled {
		g.Spec().Linux.Seccomp = nil
	}

	saveOptions := generate.ExportOptions{}
	mountPoint, err := s.storageRuntimeServer.StartContainer(id)
	if err != nil {
		return nil, fmt.Errorf("failed to mount container %s in pod sandbox %s(%s): %v", containerName, sb.name, id, err)
	}
	g.SetRootPath(mountPoint)
	err = g.SaveToFile(filepath.Join(podContainer.Dir, "config.json"), saveOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to save template configuration for pod sandbox %s(%s): %v", sb.name, id, err)
	}
	if err = g.SaveToFile(filepath.Join(podContainer.RunDir, "config.json"), saveOptions); err != nil {
		return nil, fmt.Errorf("failed to write runtime configuration for pod sandbox %s(%s): %v", sb.name, id, err)
	}

	container, err := oci.NewContainer(id, containerName, podContainer.RunDir, logPath, sb.netNs(), labels, annotations, nil, nil, id, false, sb.privileged, podContainer.Dir)
	if err != nil {
		return nil, err
	}

	sb.infraContainer = container

	// setup the network
	if !hostNetwork {
		if err = s.netPlugin.SetUpPod(netNsPath, namespace, kubeName, id); err != nil {
			return nil, fmt.Errorf("failed to create network for container %s in sandbox %s: %v", containerName, id, err)
		}
	}

	if err = s.runContainer(container, sb.cgroupParent); err != nil {
		return nil, err
	}

	resp = &pb.RunPodSandboxResponse{PodSandboxId: id}
	logrus.Debugf("RunPodSandboxResponse: %+v", resp)
	return resp, nil
}

func (s *Server) setPodSandboxMountLabel(id, mountLabel string) error {
	storageMetadata, err := s.storageRuntimeServer.GetContainerMetadata(id)
	if err != nil {
		return err
	}
	storageMetadata.SetMountLabel(mountLabel)
	return s.storageRuntimeServer.SetContainerMetadata(id, storageMetadata)
}

func getSELinuxLabels(selinuxOptions *pb.SELinuxOption) (processLabel string, mountLabel string, err error) {
	processLabel = ""
	if selinuxOptions != nil {
		user := selinuxOptions.User
		if user == "" {
			return "", "", fmt.Errorf("SELinuxOption.User is empty")
		}

		role := selinuxOptions.Role
		if role == "" {
			return "", "", fmt.Errorf("SELinuxOption.Role is empty")
		}

		t := selinuxOptions.Type
		if t == "" {
			return "", "", fmt.Errorf("SELinuxOption.Type is empty")
		}

		level := selinuxOptions.Level
		if level == "" {
			return "", "", fmt.Errorf("SELinuxOption.Level is empty")
		}
		processLabel = fmt.Sprintf("%s:%s:%s:%s", user, role, t, level)
	}
	return label.InitLabels(label.DupSecOpt(processLabel))
}

func setupShm(podSandboxRunDir, mountLabel string) (shmPath string, err error) {
	shmPath = filepath.Join(podSandboxRunDir, "shm")
	if err = os.Mkdir(shmPath, 0700); err != nil {
		return "", err
	}
	shmOptions := "mode=1777,size=" + strconv.Itoa(defaultShmSize)
	if err = syscall.Mount("shm", shmPath, "tmpfs", uintptr(syscall.MS_NOEXEC|syscall.MS_NOSUID|syscall.MS_NODEV),
		label.FormatMountLabel(shmOptions, mountLabel)); err != nil {
		return "", fmt.Errorf("failed to mount shm tmpfs for pod: %v", err)
	}
	return shmPath, nil
}
