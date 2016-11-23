package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/kubernetes-incubator/cri-o/utils"
	"github.com/opencontainers/runc/libcontainer/label"
	"github.com/opencontainers/runtime-tools/generate"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// RunPodSandbox creates and runs a pod-level sandbox.
func (s *Server) RunPodSandbox(ctx context.Context, req *pb.RunPodSandboxRequest) (resp *pb.RunPodSandboxResponse, err error) {
	logrus.Debugf("RunPodSandboxRequest %+v", req)
	var processLabel, mountLabel, netNsPath string
	// process req.Name
	name := req.GetConfig().GetMetadata().GetName()
	if name == "" {
		return nil, fmt.Errorf("PodSandboxConfig.Name should not be empty")
	}

	namespace := req.GetConfig().GetMetadata().GetNamespace()
	attempt := req.GetConfig().GetMetadata().GetAttempt()

	id, name, err := s.generatePodIDandName(name, namespace, attempt)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			s.releasePodName(name)
		}
	}()

	if err = s.podIDIndex.Add(id); err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			if err = s.podIDIndex.Delete(id); err != nil {
				logrus.Warnf("couldn't delete pod id %s from idIndex", id)
			}
		}
	}()

	podSandboxDir := filepath.Join(s.config.SandboxDir, id)
	if _, err = os.Stat(podSandboxDir); err == nil {
		return nil, fmt.Errorf("pod sandbox (%s) already exists", podSandboxDir)
	}

	defer func() {
		if err != nil {
			if err2 := os.RemoveAll(podSandboxDir); err2 != nil {
				logrus.Warnf("couldn't cleanup podSandboxDir %s: %v", podSandboxDir, err2)
			}
		}
	}()

	if err = os.MkdirAll(podSandboxDir, 0755); err != nil {
		return nil, err
	}

	// creates a spec Generator with the default spec.
	g := generate.New()

	// TODO: Make the `graph/vfs` part of this configurable once the storage
	//       integration has been merged.
	podInfraRootfs := filepath.Join(s.config.Root, "graph/vfs/pause")
	// setup defaults for the pod sandbox
	g.SetRootPath(filepath.Join(podInfraRootfs, "rootfs"))
	g.SetRootReadonly(true)
	g.SetProcessArgs([]string{"/pause"})

	// set hostname
	hostname := req.GetConfig().GetHostname()
	if hostname != "" {
		g.SetHostname(hostname)
	}

	// set log directory
	logDir := req.GetConfig().GetLogDirectory()
	if logDir == "" {
		logDir = filepath.Join(s.config.LogDir, id)
	}

	// set DNS options
	dnsServers := req.GetConfig().GetDnsConfig().GetServers()
	dnsSearches := req.GetConfig().GetDnsConfig().GetSearches()
	dnsOptions := req.GetConfig().GetDnsConfig().GetOptions()
	resolvPath := fmt.Sprintf("%s/resolv.conf", podSandboxDir)
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

	// Don't use SELinux separation with Host Pid or IPC Namespace,
	if !req.GetConfig().GetLinux().GetSecurityContext().GetNamespaceOptions().GetHostPid() && !req.GetConfig().GetLinux().GetSecurityContext().GetNamespaceOptions().GetHostIpc() {
		processLabel, mountLabel, err = getSELinuxLabels(nil)
		if err != nil {
			return nil, err
		}
		g.SetProcessSelinuxLabel(processLabel)
	}

	// create shm mount for the pod containers.
	var shmPath string
	if req.GetConfig().GetLinux().GetSecurityContext().GetNamespaceOptions().GetHostIpc() {
		shmPath = "/dev/shm"
	} else {
		shmPath, err = setupShm(podSandboxDir, mountLabel)
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

	containerID, containerName, err := s.generateContainerIDandName(name, "infra", 0)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			s.releaseContainerName(containerName)
		}
	}()

	if err = s.ctrIDIndex.Add(containerID); err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			if err = s.ctrIDIndex.Delete(containerID); err != nil {
				logrus.Warnf("couldn't delete ctr id %s from idIndex", containerID)
			}
		}
	}()

	g.AddAnnotation("ocid/metadata", string(metadataJSON))
	g.AddAnnotation("ocid/labels", string(labelsJSON))
	g.AddAnnotation("ocid/annotations", string(annotationsJSON))
	g.AddAnnotation("ocid/log_path", logDir)
	g.AddAnnotation("ocid/name", name)
	g.AddAnnotation("ocid/container_type", containerTypeSandbox)
	g.AddAnnotation("ocid/container_name", containerName)
	g.AddAnnotation("ocid/container_id", containerID)
	g.AddAnnotation("ocid/shm_path", shmPath)

	sb := &sandbox{
		id:           id,
		name:         name,
		logDir:       logDir,
		labels:       labels,
		annotations:  annotations,
		containers:   oci.NewMemoryStore(),
		processLabel: processLabel,
		mountLabel:   mountLabel,
		metadata:     metadata,
		shmPath:      shmPath,
	}

	s.addSandbox(sb)

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
	cgroupParent := req.GetConfig().GetLinux().GetCgroupParent()
	if cgroupParent != "" {
		g.SetLinuxCgroupsPath(cgroupParent)
	}

	// set up namespaces
	if req.GetConfig().GetLinux().GetSecurityContext().GetNamespaceOptions().GetHostNetwork() {
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
		} ()

		// Pass the created namespace path to the runtime
		err = g.AddOrReplaceLinuxNamespace("network", sb.netNsPath())
		if err != nil {
			return nil, err
		}

		netNsPath = sb.netNsPath()
	}

	if req.GetConfig().GetLinux().GetSecurityContext().GetNamespaceOptions().GetHostPid() {
		err = g.RemoveLinuxNamespace("pid")
		if err != nil {
			return nil, err
		}
	}

	if req.GetConfig().GetLinux().GetSecurityContext().GetNamespaceOptions().GetHostIpc() {
		err = g.RemoveLinuxNamespace("ipc")
		if err != nil {
			return nil, err
		}
	}

	err = g.SaveToFile(filepath.Join(podSandboxDir, "config.json"), generate.ExportOptions{})
	if err != nil {
		return nil, err
	}

	if _, err = os.Stat(podInfraRootfs); err != nil {
		if os.IsNotExist(err) {
			// TODO: Replace by rootfs creation API when it is ready
			if err = utils.CreateInfraRootfs(podInfraRootfs, s.config.Pause); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	container, err := oci.NewContainer(containerID, containerName, podSandboxDir, podSandboxDir, sb.netNs(), labels, annotations, nil, nil, id, false)
	if err != nil {
		return nil, err
	}

	sb.infraContainer = container

	if err = s.runtime.CreateContainer(container); err != nil {
		return nil, err
	}

	if err = s.runtime.UpdateStatus(container); err != nil {
		return nil, err
	}

	// setup the network
	podNamespace := ""
	if err = s.netPlugin.SetUpPod(netNsPath, podNamespace, id, containerName); err != nil {
		return nil, fmt.Errorf("failed to create network for container %s in sandbox %s: %v", containerName, id, err)
	}

	if err = s.runtime.StartContainer(container); err != nil {
		return nil, err
	}

	if err = s.runtime.UpdateStatus(container); err != nil {
		return nil, err
	}

	resp = &pb.RunPodSandboxResponse{PodSandboxId: &id}
	logrus.Debugf("RunPodSandboxResponse: %+v", resp)
	return resp, nil
}

func getSELinuxLabels(selinuxOptions *pb.SELinuxOption) (processLabel string, mountLabel string, err error) {
	processLabel = ""
	if selinuxOptions != nil {
		user := selinuxOptions.GetUser()
		if user == "" {
			return "", "", fmt.Errorf("SELinuxOption.User is empty")
		}

		role := selinuxOptions.GetRole()
		if role == "" {
			return "", "", fmt.Errorf("SELinuxOption.Role is empty")
		}

		t := selinuxOptions.GetType()
		if t == "" {
			return "", "", fmt.Errorf("SELinuxOption.Type is empty")
		}

		level := selinuxOptions.GetLevel()
		if level == "" {
			return "", "", fmt.Errorf("SELinuxOption.Level is empty")
		}
		processLabel = fmt.Sprintf("%s:%s:%s:%s", user, role, t, level)
	}
	return label.InitLabels(label.DupSecOpt(processLabel))
}

func setupShm(podSandboxDir, mountLabel string) (shmPath string, err error) {
	shmPath = filepath.Join(podSandboxDir, "shm")
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
