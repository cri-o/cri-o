package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	cnitypes "github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"go.podman.io/storage"
	"go.podman.io/storage/pkg/idtools"
	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/internal/config/nsmgr"
	"github.com/cri-o/cri-o/internal/lib/constants"
	libsandbox "github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/memorystore"
	oci "github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/resourcestore"
	"github.com/cri-o/cri-o/pkg/annotations"
	libconfig "github.com/cri-o/cri-o/pkg/config"
	"github.com/cri-o/cri-o/utils"
	json "github.com/json-iterator/go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/sirupsen/logrus"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	kubeletTypes "k8s.io/kubelet/pkg/types"
)

func (s *Server) getSandboxIDMappings(ctx context.Context, sb *libsandbox.Sandbox) (*idtools.IDMappings, error) {
	return nil, nil
}

func (s *Server) runPodSandbox(ctx context.Context, req *types.RunPodSandboxRequest) (resp *types.RunPodSandboxResponse, retErr error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	sbox := libsandbox.NewBuilder()

	if err := sbox.SetConfig(req.Config); err != nil {
		return nil, fmt.Errorf("setting sandbox config: %w", err)
	}

	// we need to fill in the container name, as it is not present in the request. Luckily, it is a constant.
	log.Infof(ctx, "Running pod sandbox: %s%s", oci.LabelsToDescription(sbox.Config().Labels), oci.InfraContainerName)

	kubeName := sbox.Config().Metadata.Name
	namespace := sbox.Config().Metadata.Namespace
	attempt := sbox.Config().Metadata.Attempt

	if err := sbox.GenerateNameAndID(); err != nil {
		return nil, fmt.Errorf("setting pod sandbox name and id: %w", err)
	}
	sboxId := sbox.ID()
	sboxName := sbox.Name()
	resourceCleaner := resourcestore.NewResourceCleaner()
	defer func() {
		// no error, no need to cleanup
		if retErr == nil || isContextError(retErr) {
			return
		}
		if err := resourceCleaner.Cleanup(); err != nil {
			log.Errorf(ctx, "Unable to cleanup: %v", err)
		}
	}()

	if _, err := s.ReservePodName(sboxId, sboxName); err != nil {
		reservedID, getErr := s.PodIDForName(sboxName)
		if getErr != nil {
			return nil, fmt.Errorf("failed to get ID of pod with reserved name (%s), after failing to reserve name with %v: %w", sboxName, getErr, getErr)
		}
		// if we're able to find the sandbox, and it's created, this is actually a duplicate request
		// Just return that sandbox
		if reservedsbuilder := s.GetSandbox(reservedID); reservedsbuilder != nil && reservedsbuilder.Created() {
			return &types.RunPodSandboxResponse{PodSandboxId: reservedID}, nil
		}
		cachedID, resourceErr := s.getResourceOrWait(ctx, sboxName, "sandbox")
		if resourceErr == nil {
			return &types.RunPodSandboxResponse{PodSandboxId: cachedID}, nil
		}
		return nil, fmt.Errorf("%v: %w", resourceErr, err)
	}
	resourceCleaner.Add(ctx, "runSandbox: releasing pod sandbox name: "+sboxName, func() error {
		s.ReleasePodName(sboxName)
		return nil
	})

	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox creating")

	var securityContext *types.LinuxSandboxSecurityContext
	if sbox.Config().Linux != nil && sbox.Config().Linux.SecurityContext != nil {
		securityContext = sbox.Config().Linux.SecurityContext
	} else {
		securityContext = &types.LinuxSandboxSecurityContext{}
	}

	if securityContext.NamespaceOptions == nil {
		securityContext.NamespaceOptions = &types.NamespaceOption{}
	}
	hostNetwork := securityContext.NamespaceOptions.Network == types.NamespaceMode_NODE

	if !hostNetwork {
		if err := s.waitForCNIPlugin(ctx, sboxName); err != nil {
			return nil, err
		}
	}

	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox network ready")

	// validate the runtime handler
	runtimeHandler, err := s.runtimeHandler(req)
	if err != nil {
		return nil, err
	}

	if err := s.FilterDisallowedAnnotations(sbox.Config().Annotations, sbox.Config().Annotations, runtimeHandler); err != nil {
		return nil, err
	}

	kubeAnnotations := sbox.Config().Annotations

	usernsMode := kubeAnnotations[annotations.UsernsModeAnnotation]
	sbox.SetUsernsMode(usernsMode)

	containerName, err := s.ReserveSandboxContainerIDAndName(sbox.Config())
	if err != nil {
		return nil, err
	}
	resourceCleaner.Add(ctx, "runSandbox: releasing container name: "+containerName, func() error {
		s.ReleaseContainerName(ctx, containerName)
		return nil
	})

	var labelOptions []string
	privileged := s.privilegedSandbox(req)

	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox storage creation")
	pauseImage, err := s.config.ParsePauseImage()
	if err != nil {
		return nil, err
	}
	podContainer, err := s.ContainerServer.StorageRuntimeServer().CreatePodSandbox(s.config.SystemContext,
		sboxName, sboxId,
		pauseImage,
		s.config.PauseImageAuthFile,
		containerName,
		kubeName,
		sbox.Config().Metadata.Uid,
		namespace,
		attempt,
		nil,
		labelOptions,
		privileged,
	)
	if errors.Is(err, storage.ErrDuplicateName) {
		return nil, fmt.Errorf("pod sandbox with name %q already exists", sboxName)
	}
	if err != nil {
		return nil, fmt.Errorf("creating pod sandbox with name %q: %w", sboxName, err)
	}
	resourceCleaner.Add(ctx, "runSandbox: removing pod sandbox from storage: "+sboxId, func() error {
		return s.ContainerServer.StorageRuntimeServer().DeleteContainer(ctx, sboxId)
	})

	mountLabel := podContainer.MountLabel
	processLabel := podContainer.ProcessLabel

	// set log directory
	logDir := sbox.Config().LogDirectory
	if logDir == "" {
		logDir = filepath.Join(s.config.LogDir, sboxId)
	}
	// This should always be absolute from k8s.
	if !filepath.IsAbs(logDir) {
		return nil, fmt.Errorf("requested logDir for sbuilder ID %s is a relative path: %s", sboxId, logDir)
	}
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return nil, err
	}

	var sandboxIDMappings *idtools.IDMappings

	// TODO: factor generating/updating the spec into something other projects can vendor
	if err := sbox.InitInfraContainer(&s.config, &podContainer, nil); err != nil {
		return nil, err
	}

	// add metadata
	metadata := sbox.Config().Metadata
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}

	// add labels
	labels := sbox.Config().Labels

	if err := validateLabels(labels); err != nil {
		return nil, err
	}

	// Add special container name label for the infra container
	if labels != nil {
		labels[kubeletTypes.KubernetesContainerNameLabel] = oci.InfraContainerName
	}
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return nil, err
	}

	// add annotations
	kubeAnnotationsJSON, err := json.Marshal(kubeAnnotations)
	if err != nil {
		return nil, err
	}

	nsOptsJSON, err := json.Marshal(securityContext.NamespaceOptions)
	if err != nil {
		return nil, err
	}

	// TODO: Add support for hostIPC using sysvmsg=inherit
	hostIPC := false
	// Jail always shares a (redacted) pid namespace with the host
	hostPID := true

	// Don't use SELinux separation with Host Pid or IPC Namespace or privileged.
	if hostPID || hostIPC {
		processLabel, mountLabel = "", ""
	}
	g := sbox.Spec()

	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox spec configuration")

	if err := s.ContainerServer.CtrIDIndex().Add(sboxId); err != nil {
		return nil, err
	}
	resourceCleaner.Add(ctx, "runSandbox: deleting container ID from idIndex for sandbox "+sboxId, func() error {
		if err := s.ContainerServer.CtrIDIndex().Delete(sboxId); err != nil && !strings.Contains(err.Error(), noSuchID) {
			return fmt.Errorf("could not delete ctr id %s from idIndex: %w", sboxId, err)
		}
		return nil
	})

	// set log path inside log directory
	logPath := filepath.Join(logDir, sboxId+".log")

	sbox.SetNamespace(namespace)
	sbox.SetName(sboxName)
	sbox.SetKubeName(kubeName)
	sbox.SetLogDir(logDir)
	sbox.SetContainers(memorystore.New[*oci.Container]())
	sbox.SetProcessLabel(processLabel)
	sbox.SetMountLabel(mountLabel)
	sbox.SetRuntimeHandler(runtimeHandler)

	// Handle https://issues.k8s.io/44043
	if err := utils.EnsureSaneLogPath(logPath); err != nil {
		return nil, err
	}

	hostname, err := getHostname(sboxId, sbox.Config().Hostname, hostNetwork)
	if err != nil {
		return nil, err
	}
	g.SetHostname(hostname)

	g.AddAnnotation(annotations.Metadata, string(metadataJSON))
	g.AddAnnotation(annotations.Labels, string(labelsJSON))
	g.AddAnnotation(annotations.Annotations, string(kubeAnnotationsJSON))
	g.AddAnnotation(annotations.LogPath, logPath)
	g.AddAnnotation(annotations.Name, sboxName)
	g.AddAnnotation(annotations.SandboxName, sboxName)
	g.AddAnnotation(annotations.Namespace, namespace)
	g.AddAnnotation(annotations.ContainerType, annotations.ContainerTypeSandbox)
	g.AddAnnotation(annotations.SandboxID, sboxId)
	g.AddAnnotation(annotations.UserRequestedImage, s.config.PauseImage)
	g.AddAnnotation(annotations.SomeNameOfTheImage, s.config.PauseImage)
	g.AddAnnotation(annotations.ContainerName, containerName)
	g.AddAnnotation(annotations.ContainerID, sboxId)
	g.AddAnnotation(annotations.PrivilegedRuntime, fmt.Sprintf("%v", privileged))
	g.AddAnnotation(annotations.RuntimeHandler, runtimeHandler)
	g.AddAnnotation(annotations.ResolvPath, sbox.ResolvPath())
	g.AddAnnotation(annotations.HostName, hostname)
	g.AddAnnotation(annotations.NamespaceOptions, string(nsOptsJSON))
	g.AddAnnotation(annotations.KubeName, kubeName)
	g.AddAnnotation(annotations.HostNetwork, fmt.Sprintf("%v", hostNetwork))
	g.AddAnnotation(annotations.ContainerManager, constants.ContainerManagerCRIO)
	if podContainer.Config.Config.StopSignal != "" {
		g.AddAnnotation(annotations.StopSignalAnnotation, podContainer.Config.Config.StopSignal)
	}

	if s.config.CgroupManager().IsSystemd() && node.SystemdHasCollectMode() {
		g.AddAnnotation("org.systemd.property.CollectMode", "'inactive-or-failed'")
	}

	created := time.Now()
	sbox.SetCreatedAt(created)
	err = sbox.SetCRISandbox(sboxId, labels, kubeAnnotations, metadata)
	if err != nil {
		return nil, err
	}
	g.AddAnnotation(annotations.Created, created.Format(time.RFC3339Nano))

	portMappings := convertPortMappings(sbox.Config().PortMappings)
	portMappingsJSON, err := json.Marshal(portMappings)
	if err != nil {
		return nil, err
	}
	g.AddAnnotation(annotations.PortMappings, string(portMappingsJSON))

	overhead := sbox.Config().GetLinux().GetOverhead()
	overheadJSON, err := json.Marshal(overhead)
	if err != nil {
		return nil, err
	}
	sbox.SetPodLinuxOverhead(overhead)
	g.AddAnnotation(annotations.PodLinuxOverhead, string(overheadJSON))

	resources := sbox.Config().GetLinux().GetResources()
	resourcesJSON, err := json.Marshal(resources)
	if err != nil {
		return nil, err
	}
	g.AddAnnotation(annotations.PodLinuxResources, string(resourcesJSON))

	sbox.SetResolvPath(sbox.ResolvPath())
	sbox.SetHostname(hostname)
	sbox.SetPortMappings(portMappings)
	sbox.SetHostNetwork(hostNetwork)
	sbox.SetPodLinuxResources(resources)
	sbox.SetDNSConfig(sbox.Config().DnsConfig)

	sb, err := sbox.GetSandbox()
	if err != nil {
		return nil, err
	}

	if err := s.addSandbox(ctx, sb); err != nil {
		return nil, err
	}
	resourceCleaner.Add(ctx, "runSandbox: removing pod sandbox "+sboxId, func() error {
		if err := s.removeSandbox(ctx, sboxId); err != nil {
			return fmt.Errorf("could not remove pod sandbox: %w", err)
		}
		return nil
	})

	if err := s.PodIDIndex().Add(sboxId); err != nil {
		return nil, err
	}
	resourceCleaner.Add(ctx, "runSandbox: deleting pod ID "+sboxId+" from idIndex", func() error {
		if err := s.PodIDIndex().Delete(sboxId); err != nil && !strings.Contains(err.Error(), noSuchID) {
			return fmt.Errorf("could not delete pod id %s from idIndex: %w", sboxId, err)
		}
		return nil
	})

	for k, v := range kubeAnnotations {
		g.AddAnnotation(k, v)
	}
	for k, v := range labels {
		g.AddAnnotation(k, v)
	}

	// Add default sysctls given in crio.conf
	sysctls := s.configureGeneratorForSysctls(ctx, g, hostNetwork, hostIPC, req.Config.Linux.Sysctls)

	// set up namespaces
	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox namespace creation")
	nsCleanupFuncs, err := s.configureGeneratorForSandboxNamespaces(ctx, hostNetwork, hostIPC, hostPID, sandboxIDMappings, sysctls, sb, g)
	// We want to cleanup after ourselves if we are managing any namespaces and fail in this function.
	// However, we don't immediately register this func with resourceCleaner because we need to pair the
	// ns cleanup with networkStop. Otherwise, we could try to cleanup the namespace before the network stop runs,
	// which could put us in a weird state.
	nsCleanupDescription := fmt.Sprintf("runSandbox: cleaning up namespaces after failing to run sandbox %s", sboxId)
	nsCleanupFunc := func() error {
		for idx := range nsCleanupFuncs {
			if err := nsCleanupFuncs[idx](); err != nil {
				return fmt.Errorf("RunSandbox: failed to cleanup namespace %w", err)
			}
		}
		return nil
	}
	if err != nil {
		resourceCleaner.Add(ctx, nsCleanupDescription, nsCleanupFunc)
		return nil, err
	}

	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox storage start")

	mountPoint, err := s.ContainerServer.StorageRuntimeServer().StartContainer(sboxId)
	if err != nil {
		return nil, fmt.Errorf("failed to mount container %s in pod sandbox %s(%s): %w", containerName, sb.Name(), sboxId, err)
	}
	resourceCleaner.Add(ctx, "runSandbox: stopping storage container for sandbox "+sboxId, func() error {
		if err := s.ContainerServer.StorageRuntimeServer().StopContainer(ctx, sboxId); err != nil {
			return fmt.Errorf("could not stop storage container: %s: %w", sboxId, err)
		}
		return nil
	})

	// Set OOM score adjust of the infra container to be very low
	// so it doesn't get killed.
	g.SetProcessOOMScoreAdj(PodInfraOOMAdj)

	g.SetLinuxResourcesCPUShares(PodInfraCPUshares)

	// When infra-ctr-cpuset specified, set the infra container CPU set
	if s.config.InfraCtrCPUSet != "" {
		log.Debugf(ctx, "Set the infra container cpuset to %q", s.config.InfraCtrCPUSet)
		g.SetLinuxResourcesCPUCpus(s.config.InfraCtrCPUSet)
	}

	saveOptions := generate.ExportOptions{}
	g.AddAnnotation(annotations.MountPoint, mountPoint)

	g.SetRootPath(mountPoint)
	sb.SetNamespaceOptions(securityContext.NamespaceOptions)

	// Strip out /dev/fd and /dev mounts - pause doesn't need it
	g.RemoveMount("/dev/fd")
	g.RemoveMount("/dev")

	runtimeType, err := s.ContainerServer.Runtime().RuntimeType(runtimeHandler)
	if err != nil {
		return nil, err
	}

	// A container is kernel separated if we're using shimv2, or we're using a kata v1 binary
	podIsKernelSeparated := runtimeType == libconfig.RuntimeTypeVM ||
		strings.Contains(strings.ToLower(runtimeHandler), "kata") ||
		(runtimeHandler == "" && strings.Contains(strings.ToLower(s.config.DefaultRuntime), "kata"))

	var container *oci.Container
	// In the case of kernel separated containers, we need the infra container to create the VM for the pod
	if sb.NeedsInfra(s.config.DropInfraCtr) || podIsKernelSeparated {
		log.Debugf(ctx, "Keeping infra container for pod %s", sboxId)
		container, err = oci.NewContainer(sboxId, containerName, podContainer.RunDir, logPath, labels, g.Config.Annotations, kubeAnnotations, pauseImage.StringForOutOfProcessConsumptionOnly(), nil, nil, "", nil, sboxId, false, false, false, runtimeHandler, podContainer.Dir, created, podContainer.Config.Config.StopSignal)
		if err != nil {
			return nil, err
		}
		// If using a kernel separated container runtime, the process label should be set to container_kvm_t
		// Keep in mind that kata does *not* apply any process label to containers within the VM
		if podIsKernelSeparated {
			processLabel, err = KVMLabel(processLabel)
			if err != nil {
				return nil, err
			}
			g.SetProcessSelinuxLabel(processLabel)
		}
	} else {
		log.Debugf(ctx, "Dropping infra container for pod %s", sboxId)
		container = oci.NewSpoofedContainer(sboxId, containerName, labels, sboxId, created, podContainer.RunDir)
		g.AddAnnotation(annotations.SpoofedContainer, "true")
	}
	container.SetMountPoint(mountPoint)
	container.SetSpec(g.Config)

	// needed for getSandboxIDMappings()
	container.SetIDMappings(sandboxIDMappings)

	if err := sb.SetInfraContainer(container); err != nil {
		return nil, err
	}

	if err := sb.SetContainerEnvFile(ctx); err != nil {
		return nil, err
	}

	if err = g.SaveToFile(filepath.Join(podContainer.Dir, "config.json"), saveOptions); err != nil {
		return nil, fmt.Errorf("failed to save template configuration for pod sandbox %s(%s): %w", sb.Name(), sboxId, err)
	}
	if err = g.SaveToFile(filepath.Join(podContainer.RunDir, "config.json"), saveOptions); err != nil {
		return nil, fmt.Errorf("failed to write runtime configuration for pod sandbox %s(%s): %w", sb.Name(), sboxId, err)
	}

	s.addInfraContainer(ctx, container)
	resourceCleaner.Add(ctx, "runSandbox: removing infra container "+container.ID(), func() error {
		s.removeInfraContainer(ctx, container)
		return nil
	})

	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox container runtime creation")
	if err := s.createContainerPlatform(ctx, container, sb.CgroupParent(), sandboxIDMappings); err != nil {
		return nil, err
	}
	resourceCleaner.Add(ctx, "runSandbox: stopping container "+container.ID(), func() error {
		// Clean-up steps from RemovePodSandbox
		if err := s.stopContainer(ctx, container, stopTimeoutFromContext(ctx)); err != nil {
			return fmt.Errorf("failed to stop container for removal")
		}

		log.Infof(ctx, "RunSandbox: deleting container %s", container.ID())
		if err := s.ContainerServer.Runtime().DeleteContainer(ctx, container); err != nil {
			return fmt.Errorf("failed to delete container %s in pod sandbox %s: %w", container.Name(), sb.ID(), err)
		}
		log.Infof(ctx, "RunSandbox: writing container %s state to disk", container.ID())
		if err := s.ContainerStateToDisk(ctx, container); err != nil {
			return fmt.Errorf("failed to write container state %s in pod sandbox %s: %w", container.Name(), sb.ID(), err)
		}
		return nil
	})

	// now that we have the namespaces, we should create the network if we're managing namespace Lifecycle
	var ips []string
	var result cnitypes.Result

	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox network creation")
	logrus.Debugf("Calling s.networkStart")
	ips, result, err = s.networkStart(ctx, sb)
	if err != nil {
		resourceCleaner.Add(ctx, nsCleanupDescription, nsCleanupFunc)
		return nil, err
	}
	resourceCleaner.Add(ctx, "runSandbox: stopping network for sandbox"+sb.ID(), func() error {
		// use a new context to prevent an expired context from preventing a stop
		if err := s.networkStop(context.Background(), sb); err != nil {
			return fmt.Errorf("error stopping network on cleanup: %w", err)
		}

		// Now that we've succeeded in stopping the network, cleanup namespaces
		log.Infof(ctx, nsCleanupDescription)
		return nsCleanupFunc()
	})
	if result != nil {
		resultCurrent, err := current.NewResultFromResult(result)
		if err != nil {
			return nil, err
		}
		cniResultJSON, err := json.Marshal(resultCurrent)
		if err != nil {
			return nil, err
		}
		g.AddAnnotation(annotations.CNIResult, string(cniResultJSON))
	}

	s.generateCRIEvent(ctx, sb.InfraContainer(), types.ContainerEventType_CONTAINER_CREATED_EVENT)
	if err := s.ContainerServer.Runtime().StartContainer(ctx, container); err != nil {
		return nil, err
	}

	if err := s.ContainerStateToDisk(ctx, container); err != nil {
		log.Warnf(ctx, "Unable to write containers %s state to disk: %v", container.ID(), err)
	}

	for idx, ip := range ips {
		g.AddAnnotation(fmt.Sprintf("%s.%d", annotations.IP, idx), ip)
	}
	sb.AddIPs(ips)

	if err := s.nri.runPodSandbox(ctx, sb); err != nil {
		return nil, err
	}

	if isContextError(ctx.Err()) {
		if err := s.resourceStore.Put(sboxName, sb, resourceCleaner); err != nil {
			log.Errorf(ctx, "RunSandbox: failed to save progress of sandbox %s: %v", sboxId, err)
		}
		log.Infof(ctx, "RunSandbox: context was either canceled or the deadline was exceeded: %v", ctx.Err())
		return nil, ctx.Err()
	}

	// Since it's not a context error, we can delete the resource from the store, it will be tracked in the server from now on.
	s.resourceStore.Delete(sboxName)

	sb.SetCreated()
	s.generateCRIEvent(ctx, sb.InfraContainer(), types.ContainerEventType_CONTAINER_STARTED_EVENT)

	log.Infof(ctx, "Ran pod sandbox %s with infra container: %s", container.ID(), container.Description())
	resp = &types.RunPodSandboxResponse{PodSandboxId: sboxId}
	return resp, nil
}

func (s *Server) configureGeneratorForSysctls(ctx context.Context, g *generate.Generator, hostNetwork, hostIPC bool, sysctls map[string]string) map[string]string {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	sysctlsToReturn := make(map[string]string)
	defaultSysctls, err := s.config.RuntimeConfig.Sysctls()
	if err != nil {
		log.Warnf(ctx, "Sysctls invalid: %v", err)
	}

	for _, sysctl := range defaultSysctls {
		if err := sysctl.Validate(hostNetwork, hostIPC); err != nil {
			log.Warnf(ctx, "Skipping invalid sysctl specified by config %s: %v", sysctl, err)
			continue
		}
		g.AddLinuxSysctl(sysctl.Key(), sysctl.Value())
		sysctlsToReturn[sysctl.Key()] = sysctl.Value()
	}

	// extract linux sysctls from annotations and pass down to oci runtime
	// Will override any duplicate default systcl from crio.conf
	for key, value := range sysctls {
		sysctl := libconfig.NewSysctl(key, value)
		if err := sysctl.Validate(hostNetwork, hostIPC); err != nil {
			log.Warnf(ctx, "Skipping invalid sysctl specified over CRI %s: %v", sysctl, err)
			continue
		}
		g.AddLinuxSysctl(key, value)
		sysctlsToReturn[key] = value
	}
	return sysctlsToReturn
}

func (s *Server) configureGeneratorForSandboxNamespaces(ctx context.Context, hostNetwork, hostIPC, hostPID bool, idMappings *idtools.IDMappings, sysctls map[string]string, sb *libsandbox.Sandbox, g *generate.Generator) (cleanupFuncs []func() error, retErr error) {
	_, span := log.StartSpan(ctx)
	defer span.End()
	namespaceConfig := &nsmgr.PodNamespacesConfig{
		Namespaces: []*nsmgr.PodNamespaceConfig{
			{
				Type: nsmgr.NETNS,
				Host: hostNetwork,
				Path: sb.ID(),
			},
		},
	}

	// now that we've configured the namespaces we're sharing, create them
	namespaces, err := s.config.NamespaceManager().NewPodNamespaces(namespaceConfig)
	if err != nil {
		return nil, err
	}

	sb.AddManagedNamespaces(namespaces)

	if !hostNetwork {
		g.AddAnnotation("org.freebsd.jail.vnet", "new")
	}

	cleanupFuncs = append(cleanupFuncs, sb.RemoveManagedNamespaces)

	return cleanupFuncs, nil
}
