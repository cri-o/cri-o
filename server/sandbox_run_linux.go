package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	cnitypes "github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containers/storage"
	"github.com/containers/storage/pkg/idtools"
	"github.com/containers/storage/pkg/unshare"
	json "github.com/json-iterator/go"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/opencontainers/selinux/go-selinux/label"
	"golang.org/x/sys/unix"
	"k8s.io/apimachinery/pkg/api/resource"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	kubeletTypes "k8s.io/kubelet/pkg/types"

	"github.com/cri-o/cri-o/internal/config/nsmgr"
	ctrfactory "github.com/cri-o/cri-o/internal/factory/container"
	"github.com/cri-o/cri-o/internal/lib/constants"
	libsandbox "github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/linklogs"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/memorystore"
	oci "github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/resourcestore"
	"github.com/cri-o/cri-o/pkg/annotations"
	libconfig "github.com/cri-o/cri-o/pkg/config"
	"github.com/cri-o/cri-o/utils"
)

// DefaultUserNSSize is the default size for the user namespace created.
const DefaultUserNSSize = 65536

// addToMappingsIfMissing ensures the specified id is mapped from the host.
func addToMappingsIfMissing(ids []idtools.IDMap, id int64) []idtools.IDMap {
	firstAvailable := int(0)

	for _, r := range ids {
		if int(id) >= r.HostID && int(id) < r.HostID+r.Size {
			// Already present, nothing to do
			return ids
		}

		if r.ContainerID+r.Size > firstAvailable {
			firstAvailable = r.ContainerID + r.Size
		}
	}

	newMapping := idtools.IDMap{
		ContainerID: firstAvailable,
		HostID:      int(id),
		Size:        1,
	}

	return append(ids, newMapping)
}

func (s *Server) configureSandboxIDMappings(mode string, sc *types.LinuxSandboxSecurityContext) (*storage.IDMappingOptions, error) {
	if sc.GetNamespaceOptions().GetUsernsOptions() != nil {
		switch sc.GetNamespaceOptions().GetUsernsOptions().GetMode() {
		case types.NamespaceMode_NODE:
			return nil, nil
		case types.NamespaceMode_POD:
			return &storage.IDMappingOptions{
				UIDMap: convertToStorageIDMap(sc.GetNamespaceOptions().GetUsernsOptions().GetUids()),
				GIDMap: convertToStorageIDMap(sc.GetNamespaceOptions().GetUsernsOptions().GetGids()),
			}, nil
		default:
			return nil, fmt.Errorf("unsupported pod mode: %q", sc.GetNamespaceOptions().GetUsernsOptions().GetMode())
		}
	}

	if mode == "" {
		// No mode specified but mappings set in the config file, let's use them.
		if s.defaultIDMappings != nil {
			uids := s.defaultIDMappings.UIDs()
			gids := s.defaultIDMappings.GIDs()

			return &storage.IDMappingOptions{UIDMap: uids, GIDMap: gids}, nil
		}

		return nil, nil
	}

	// expect a configuration like: private:uidmapping=0:1000:2000,2000:1000:2000;gidmapping=0:1000:4000,4000:1000:2000
	parts := strings.SplitN(mode, ":", 2)

	values := map[string]string{}

	if len(parts) > 1 {
		for _, r := range strings.Split(parts[1], ";") {
			kv := strings.SplitN(r, "=", 2)
			if len(kv) != 2 {
				return nil, fmt.Errorf("invalid argument: %q", r)
			}

			values[kv[0]] = kv[1]
		}
	}

	_, uidMappingsPresent := values["uidmapping"]
	_, gidMappingsPresent := values["gidmapping"]
	// limit mappings for pods that aren't going to be running as root
	minimumMappableUID, minimumMappableGID := s.minimumMappableUID, s.minimumMappableGID

	if uidMappingsPresent || gidMappingsPresent {
		user := sc.GetRunAsUser()
		if user == nil {
			return nil, errors.New("cannot use uidmapping or gidmapping if RunAsUser is not set")
		}

		if user.GetValue() != 0 {
			if minimumMappableUID < 0 {
				return nil, errors.New("cannot use uidmapping or gidmapping if not running as root and minimum mappable ID is not set")
			}

			if user.GetValue() < minimumMappableUID {
				return nil, fmt.Errorf("cannot use uidmapping or gidmapping if running as a UID below minimum mappable ID %d", minimumMappableUID)
			}
		}
	}

	switch parts[0] {
	case "auto":
		const t = "true"

		ret := &storage.IDMappingOptions{
			AutoUserNs: true,
		}
		// If keep-id=true then the UID:GID won't be changed inside of the user namespace and it
		// will map to the same value on the host.
		keepID := values["keep-id"] == t
		// If map-to-root=true then the UID:GID will be mapped to root inside of the user namespace.
		mapToRoot := values["map-to-root"] == t
		if keepID && mapToRoot {
			return nil, fmt.Errorf("cannot use both keep-id and map-to-root: %q", mode)
		}

		if v, ok := values["size"]; ok {
			s, err := strconv.ParseUint(v, 10, 32)
			if err != nil {
				return nil, err
			}

			ret.AutoUserNsOpts.Size = uint32(s)
		} else {
			ret.AutoUserNsOpts.Size = DefaultUserNSSize
		}

		if v, ok := values["uidmapping"]; ok {
			uids, err := idtools.ParseIDMap(strings.Split(v, ","), "UID")
			if err != nil {
				return nil, err
			}

			ret.AutoUserNsOpts.AdditionalUIDMappings = append(ret.AutoUserNsOpts.AdditionalUIDMappings, uids...)
		}

		if v, ok := values["gidmapping"]; ok {
			gids, err := idtools.ParseIDMap(strings.Split(v, ","), "GID")
			if err != nil {
				return nil, err
			}

			ret.AutoUserNsOpts.AdditionalGIDMappings = append(ret.AutoUserNsOpts.AdditionalGIDMappings, gids...)
		}

		if sc.GetRunAsUser() != nil {
			if keepID || mapToRoot {
				id := 0
				if keepID {
					id = int(sc.GetRunAsUser().GetValue())
				}

				ret.AutoUserNsOpts.AdditionalUIDMappings = append(
					ret.AutoUserNsOpts.AdditionalUIDMappings,
					idtools.IDMap{
						ContainerID: id,
						HostID:      int(sc.GetRunAsUser().GetValue()),
						Size:        1,
					})
			} else {
				m := addToMappingsIfMissing(ret.AutoUserNsOpts.AdditionalUIDMappings, sc.GetRunAsUser().GetValue())
				ret.AutoUserNsOpts.AdditionalUIDMappings = m
			}
		}

		if sc.GetRunAsGroup() != nil {
			if keepID || mapToRoot {
				id := 0
				if keepID {
					id = int(sc.GetRunAsGroup().GetValue())
				}

				ret.AutoUserNsOpts.AdditionalGIDMappings = append(
					ret.AutoUserNsOpts.AdditionalGIDMappings,
					idtools.IDMap{
						ContainerID: id,
						HostID:      int(sc.GetRunAsGroup().GetValue()),
						Size:        1,
					})
			} else {
				m := addToMappingsIfMissing(ret.AutoUserNsOpts.AdditionalGIDMappings, sc.GetRunAsGroup().GetValue())
				ret.AutoUserNsOpts.AdditionalGIDMappings = m
			}
		}

		for _, g := range sc.GetSupplementalGroups() {
			if keepID {
				ret.AutoUserNsOpts.AdditionalGIDMappings = append(
					ret.AutoUserNsOpts.AdditionalGIDMappings,
					idtools.IDMap{
						ContainerID: int(g),
						HostID:      int(g),
						Size:        1,
					})
			} else {
				m := addToMappingsIfMissing(ret.AutoUserNsOpts.AdditionalGIDMappings, g)
				ret.AutoUserNsOpts.AdditionalGIDMappings = m
			}
		}
		// make sure we haven't asked to map any sensitive and/or privileged IDs
		if minimumMappableUID >= 0 {
			for _, uidSlice := range ret.AutoUserNsOpts.AdditionalUIDMappings {
				if int64(uidSlice.HostID) < minimumMappableUID {
					return nil, fmt.Errorf("not allowed to map UID range (%d-%d), below minimum mappable UID %d", uidSlice.HostID, uidSlice.HostID+uidSlice.Size-1, minimumMappableUID)
				}
			}
		}

		if minimumMappableGID >= 0 {
			for _, gidSlice := range ret.AutoUserNsOpts.AdditionalGIDMappings {
				if int64(gidSlice.HostID) < minimumMappableGID {
					return nil, fmt.Errorf("not allowed to map GID range (%d-%d), below minimum mappable GID %d", gidSlice.HostID, gidSlice.HostID+gidSlice.Size-1, minimumMappableGID)
				}
			}
		}

		return ret, nil
	case "private":
		var err error

		var uids, gids []idtools.IDMap

		if v, ok := values["uidmapping"]; ok {
			uids, err = idtools.ParseIDMap(strings.Split(v, ","), "UID")
			if err != nil {
				return nil, err
			}
		}

		if v, ok := values["gidmapping"]; ok {
			// both gidmapping and uidmapping are specified
			gids, err = idtools.ParseIDMap(strings.Split(v, ","), "GID")
			if err != nil {
				return nil, err
			}
		}

		if uids == nil && gids == nil {
			if s.defaultIDMappings == nil {
				// no configuration and no global mappings
				return nil, errors.New("userns requested but no userns mappings configured")
			}

			// no configuration specified, so use the global mappings
			uids = s.defaultIDMappings.UIDs()
			gids = s.defaultIDMappings.GIDs()
		} else {
			// one between uids and gids is set, use the same range
			if uids == nil && gids != nil {
				uids = gids
			} else if gids == nil && uids != nil {
				gids = uids
			}
		}
		// make sure the specified users are part of the namespace
		if sc.GetRunAsUser() != nil {
			uids = addToMappingsIfMissing(uids, sc.GetRunAsUser().GetValue())
		}

		if sc.GetRunAsGroup() != nil {
			gids = addToMappingsIfMissing(gids, sc.GetRunAsGroup().GetValue())
		}

		for _, g := range sc.GetSupplementalGroups() {
			gids = addToMappingsIfMissing(gids, g)
		}

		// make sure we haven't mapped any sensitive and/or privileged IDs
		if minimumMappableUID >= 0 {
			for _, uidSlice := range uids {
				if int64(uidSlice.HostID) < minimumMappableUID {
					return nil, fmt.Errorf("not allowed to map UID range (%d-%d), below minimum mappable UID %d", uidSlice.HostID, uidSlice.HostID+uidSlice.Size-1, minimumMappableUID)
				}
			}
		}

		if minimumMappableGID >= 0 {
			for _, gidSlice := range gids {
				if int64(gidSlice.HostID) < minimumMappableGID {
					return nil, fmt.Errorf("not allowed to map GID range (%d-%d), below minimum mappable GID %d", gidSlice.HostID, gidSlice.HostID+gidSlice.Size-1, minimumMappableGID)
				}
			}
		}

		return &storage.IDMappingOptions{UIDMap: uids, GIDMap: gids}, nil
	}

	return nil, fmt.Errorf("invalid userns mode: %q", mode)
}

func convertToStorageIDMap(mappings []*types.IDMapping) []idtools.IDMap {
	ret := make([]idtools.IDMap, len(mappings))
	for i, m := range mappings {
		ret[i] = idtools.IDMap{
			ContainerID: int(m.GetContainerId()),
			HostID:      int(m.GetHostId()),
			Size:        int(m.GetLength()),
		}
	}

	return ret
}

func (s *Server) getSandboxIDMappings(ctx context.Context, sb *libsandbox.Sandbox) (*idtools.IDMappings, error) {
	_, span := log.StartSpan(ctx)
	defer span.End()

	ic := sb.InfraContainer()
	if ic != nil {
		mappings := ic.IDMappings()
		if mappings != nil {
			return mappings, nil
		}
	}

	if sb.UsernsMode() == "" && s.defaultIDMappings == nil {
		return nil, nil
	}

	if ic == nil {
		return nil, errors.New("infra container not found")
	}

	uids, gids, err := unshare.GetHostIDMappings(strconv.Itoa(ic.State().Pid))
	if err != nil {
		return nil, err
	}

	mappings := convertToStorageIDMappings(uids, gids)
	ic.SetIDMappings(mappings)

	return mappings, nil
}

func convertToStorageIDMappings(uidMappings, gidMappings []spec.LinuxIDMapping) *idtools.IDMappings {
	uids := make([]idtools.IDMap, len(uidMappings))
	gids := make([]idtools.IDMap, len(gidMappings))

	for i, v := range uidMappings {
		uids[i] = idtools.IDMap{ContainerID: int(v.ContainerID), HostID: int(v.HostID), Size: int(v.Size)}
	}

	for i, v := range gidMappings {
		gids[i] = idtools.IDMap{ContainerID: int(v.ContainerID), HostID: int(v.HostID), Size: int(v.Size)}
	}

	return idtools.NewIDMappingsFromMaps(uids, gids)
}

func (s *Server) runPodSandbox(ctx context.Context, req *types.RunPodSandboxRequest) (resp *types.RunPodSandboxResponse, retErr error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	sbox := libsandbox.NewBuilder()
	if err := sbox.SetConfig(req.GetConfig()); err != nil {
		return nil, fmt.Errorf("setting sandbox config: %w", err)
	}

	kubeName := sbox.Config().GetMetadata().GetName()
	kubePodUID := sbox.Config().GetMetadata().GetUid()
	namespace := sbox.Config().GetMetadata().GetNamespace()

	sbox.SetNamespace(namespace)
	sbox.SetKubeName(kubeName)
	sbox.SetContainers(memorystore.New[*oci.Container]())

	attempt := sbox.Config().GetMetadata().GetAttempt()

	// These fields are populated by the Kubelet, but not crictl. Populate if needed.
	sbox.Config().Labels = populateSandboxLabels(sbox.Config().GetLabels(), kubeName, kubePodUID, namespace)
	// we need to fill in the container name, as it is not present in the request. Luckily, it is a constant.
	log.Infof(ctx, "Running pod sandbox: %s%s", oci.LabelsToDescription(sbox.Config().GetLabels()), oci.InfraContainerName)

	if err := sbox.GenerateNameAndID(); err != nil {
		return nil, fmt.Errorf("setting pod sandbox name and id: %w", err)
	}

	sboxID := sbox.ID()
	sboxName := sbox.Name()

	sbox.SetName(sboxName)

	resourceCleaner := resourcestore.NewResourceCleaner()
	// in some cases, it is still necessary to reserve container resources when an error occurs (such as just a request context timeout error)
	storeResource := false

	defer func() {
		// no error or resource need to be stored, no need to cleanup
		if retErr == nil || storeResource {
			return
		}

		if err := resourceCleaner.Cleanup(); err != nil {
			log.Errorf(ctx, "Unable to cleanup: %v", err)
		}
	}()

	if _, err := s.ReservePodName(sboxID, sboxName); err != nil {
		reservedID, getErr := s.PodIDForName(sboxName)
		if getErr != nil {
			return nil, fmt.Errorf("failed to get ID of pod with reserved name (%s), after failing to reserve name with %w: %w", sboxName, getErr, getErr)
		}
		// if we're able to find the sandbox, and it's created, this is actually a duplicate request
		// Just return that sandbox
		if reservedSbox := s.GetSandbox(reservedID); reservedSbox != nil && reservedSbox.Created() {
			return &types.RunPodSandboxResponse{PodSandboxId: reservedID}, nil
		}

		cachedID, resourceErr := s.getResourceOrWait(ctx, sboxName, "sandbox")
		if resourceErr == nil {
			return &types.RunPodSandboxResponse{PodSandboxId: cachedID}, nil
		}

		return nil, fmt.Errorf("%w: %w", resourceErr, err)
	}

	resourceCleaner.Add(ctx, "runSandbox: releasing pod sandbox name: "+sboxName, func() error {
		s.ReleasePodName(sboxName)

		return nil
	})

	// TODO: Pass interface instead of individual field.
	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox creating")

	securityContext := sbox.Config().GetLinux().GetSecurityContext()

	if securityContext.GetNamespaceOptions() == nil {
		securityContext.NamespaceOptions = &types.NamespaceOption{}
	}

	hostNetwork := securityContext.GetNamespaceOptions().GetNetwork() == types.NamespaceMode_NODE
	sbox.SetHostNetwork(hostNetwork)

	if !hostNetwork {
		if err := s.waitForCNIPlugin(ctx, sboxName); err != nil {
			return nil, err
		}
	}

	// TODO: Pass interface instead of individual field.
	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox network ready")

	// validate the runtime handler
	runtimeHandler, err := s.runtimeHandler(req)
	if err != nil {
		return nil, err
	}

	sbox.SetRuntimeHandler(runtimeHandler)

	defaultAnnotations, err := s.ContainerServer.Runtime().RuntimeDefaultAnnotations(runtimeHandler)
	if err != nil {
		return nil, err
	}

	kubeAnnotations := map[string]string{}
	// Deep copy to prevent writing to the same map in the config
	for k, v := range defaultAnnotations {
		kubeAnnotations[k] = v
	}

	if err := s.FilterDisallowedAnnotations(sbox.Config().GetAnnotations(), sbox.Config().GetAnnotations(), runtimeHandler); err != nil {
		return nil, err
	}

	// override default annotations with pod spec specified ones
	for k, v := range sbox.Config().GetAnnotations() {
		if _, ok := kubeAnnotations[k]; ok {
			log.Debugf(ctx, "Overriding default pod annotation %s for pod %s", k, sbox.ID())
		}

		kubeAnnotations[k] = v
	}

	usernsMode := kubeAnnotations[annotations.UsernsModeAnnotation]
	if usernsMode != "" {
		log.Warnf(ctx, "Annotation 'io.kubernetes.cri-o.userns-mode' is deprecated, and will be replaced with native Kubernetes support for user namespaces in the future")
	}

	sbox.SetUsernsMode(usernsMode)

	idMappingsOptions, err := s.configureSandboxIDMappings(usernsMode, sbox.Config().GetLinux().GetSecurityContext())
	if err != nil {
		return nil, err
	}

	containerName, err := s.ReserveSandboxContainerIDAndName(sbox.Config())
	if err != nil {
		return nil, err
	}

	resourceCleaner.Add(ctx, "runSandbox: releasing container name: "+containerName, func() error {
		s.ReleaseContainerName(ctx, containerName)

		return nil
	})

	var labelOptions []string

	selinuxConfig := securityContext.GetSelinuxOptions()
	if selinuxConfig != nil {
		labelOptions = utils.GetLabelOptions(selinuxConfig)
	}

	privileged := s.privilegedSandbox(req)
	sbox.SetPrivileged(privileged)

	// TODO: Pass interface instead of individual field.
	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox storage creation")

	pauseImage, err := s.config.ParsePauseImage()
	if err != nil {
		return nil, err
	}

	podContainer, err := s.ContainerServer.StorageRuntimeServer().CreatePodSandbox(s.config.SystemContext,
		sboxName, sboxID,
		pauseImage,
		s.config.PauseImageAuthFile,
		containerName,
		kubeName,
		sbox.Config().GetMetadata().GetUid(),
		namespace,
		attempt,
		idMappingsOptions,
		labelOptions,
		privileged,
	)
	if errors.Is(err, storage.ErrDuplicateName) {
		return nil, fmt.Errorf("pod sandbox with name %q already exists", sboxName)
	}

	if err != nil {
		return nil, fmt.Errorf("creating pod sandbox with name %q: %w", sboxName, err)
	}

	resourceCleaner.Add(ctx, "runSandbox: removing pod sandbox from storage: "+sboxID, func() error {
		return s.ContainerServer.StorageRuntimeServer().DeleteContainer(ctx, sboxID)
	})

	mountLabel := podContainer.MountLabel
	processLabel := podContainer.ProcessLabel
	sbox.SetProcessLabel(processLabel)
	sbox.SetMountLabel(mountLabel)

	// set log directory
	logDir := sbox.Config().GetLogDirectory()
	if logDir == "" {
		logDir = filepath.Join(s.config.LogDir, sboxID)
	}
	// This should always be absolute from k8s.
	if !filepath.IsAbs(logDir) {
		return nil, fmt.Errorf("requested logDir for sbuilder ID %s is a relative path: %s", sboxID, logDir)
	}

	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return nil, err
	}

	sbox.SetLogDir(logDir)

	var sandboxIDMappings *idtools.IDMappings
	if idMappingsOptions != nil {
		sandboxIDMappings = idtools.NewIDMappingsFromMaps(idMappingsOptions.UIDMap, idMappingsOptions.GIDMap)
	}

	// TODO: factor generating/updating the spec into something other projects can vendor.
	if err := sbox.InitInfraContainer(&s.config, &podContainer, sandboxIDMappings); err != nil {
		return nil, err
	}

	// add metadata
	metadata := sbox.Config().GetMetadata()

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}

	// add labels
	labels := sbox.Config().GetLabels()

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

	nsOptsJSON, err := json.Marshal(securityContext.GetNamespaceOptions())
	if err != nil {
		return nil, err
	}

	hostIPC := securityContext.GetNamespaceOptions().GetIpc() == types.NamespaceMode_NODE
	hostPID := securityContext.GetNamespaceOptions().GetPid() == types.NamespaceMode_NODE

	// Don't use SELinux separation with Host Pid or IPC Namespace or privileged.
	if hostPID || hostIPC {
		processLabel, mountLabel = "", ""
	}

	g := sbox.Spec()
	g.SetProcessSelinuxLabel(processLabel)
	g.SetLinuxMountLabel(mountLabel)

	// Remove the default /dev/shm mount to ensure we overwrite it
	g.RemoveMount(libsandbox.DevShmPath)

	// create shm mount for the pod containers.
	// TODO: Pass interface instead of individual field.
	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox shm creation")

	var shmPath string
	if hostIPC {
		shmPath = libsandbox.DevShmPath
	} else {
		shmSize := int64(libsandbox.DefaultShmSize)

		if shmSizeStr, ok := kubeAnnotations[annotations.ShmSizeAnnotation]; ok {
			quantity, err := resource.ParseQuantity(shmSizeStr)
			if err != nil {
				return nil, fmt.Errorf("failed to parse shm size '%s': %w", shmSizeStr, err)
			}

			shmSize = quantity.Value()
		}

		shmPath, err = libsandbox.SetupShm(podContainer.RunDir, mountLabel, shmSize)
		if err != nil {
			return nil, err
		}

		if sandboxIDMappings != nil {
			rootPair := sandboxIDMappings.RootPair()
			if err := os.Chown(shmPath, rootPair.UID, rootPair.GID); err != nil {
				return nil, fmt.Errorf("cannot chown %s to %d:%d: %w", shmPath, rootPair.UID, rootPair.GID, err)
			}
		}

		resourceCleaner.Add(ctx, "runSandbox: unmounting shmPath for sandbox "+sboxID, func() error {
			if err := unix.Unmount(shmPath, unix.MNT_DETACH); err != nil {
				return fmt.Errorf("failed to unmount shm for sandbox: %w", err)
			}

			return nil
		})
	}

	sbox.SetShmPath(shmPath)

	// Link logs if requested
	if emptyDirVolName, ok := kubeAnnotations[annotations.LinkLogsAnnotation]; ok {
		if err = linklogs.MountPodLogs(ctx, kubePodUID, emptyDirVolName, namespace, kubeName, mountLabel); err != nil {
			log.Warnf(ctx, "Failed to link logs: %v", err)
		}
	}

	// TODO: Pass interface instead of individual field.
	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox spec configuration")

	mnt := spec.Mount{
		Type:        "bind",
		Source:      shmPath,
		Destination: libsandbox.DevShmPath,
		Options:     []string{"rw", "bind"},
	}
	// bind mount the pod shm
	g.AddMount(mnt)

	err = s.setPodSandboxMountLabel(ctx, sboxID, mountLabel)
	if err != nil {
		return nil, err
	}

	if err := s.ContainerServer.CtrIDIndex().Add(sboxID); err != nil {
		return nil, err
	}

	resourceCleaner.Add(ctx, "runSandbox: deleting container ID from idIndex for sandbox "+sboxID, func() error {
		if err := s.ContainerServer.CtrIDIndex().Delete(sboxID); err != nil && !strings.Contains(err.Error(), noSuchID) {
			return fmt.Errorf("could not delete ctr id %s from idIndex: %w", sboxID, err)
		}

		return nil
	})

	// set log path inside log directory
	logPath := filepath.Join(logDir, sboxID+".log")

	// Handle https://issues.k8s.io/44043
	if err := utils.EnsureSaneLogPath(logPath); err != nil {
		return nil, err
	}

	hostname, err := getHostname(sboxID, sbox.Config().GetHostname(), hostNetwork)
	if err != nil {
		return nil, err
	}

	sbox.SetHostname(hostname)
	g.SetHostname(hostname)

	g.AddAnnotation(annotations.Metadata, string(metadataJSON))
	g.AddAnnotation(annotations.Labels, string(labelsJSON))
	g.AddAnnotation(annotations.Annotations, string(kubeAnnotationsJSON))
	g.AddAnnotation(annotations.LogPath, logPath)
	g.AddAnnotation(annotations.Name, sboxName)
	g.AddAnnotation(annotations.SandboxName, sboxName)
	g.AddAnnotation(annotations.Namespace, namespace)
	g.AddAnnotation(annotations.ContainerType, annotations.ContainerTypeSandbox)
	g.AddAnnotation(annotations.SandboxID, sboxID)
	g.AddAnnotation(annotations.UserRequestedImage, pauseImage.StringForOutOfProcessConsumptionOnly())
	g.AddAnnotation(annotations.SomeNameOfTheImage, pauseImage.StringForOutOfProcessConsumptionOnly())
	g.AddAnnotation(annotations.ContainerName, containerName)
	g.AddAnnotation(annotations.ContainerID, sboxID)
	g.AddAnnotation(annotations.ShmPath, shmPath)
	g.AddAnnotation(annotations.PrivilegedRuntime, strconv.FormatBool(privileged))
	g.AddAnnotation(annotations.RuntimeHandler, runtimeHandler)
	g.AddAnnotation(annotations.ResolvPath, sbox.ResolvPath())
	g.AddAnnotation(annotations.HostName, hostname)
	g.AddAnnotation(annotations.NamespaceOptions, string(nsOptsJSON))
	g.AddAnnotation(annotations.KubeName, kubeName)
	g.AddAnnotation(annotations.HostNetwork, strconv.FormatBool(hostNetwork))
	g.AddAnnotation(annotations.ContainerManager, constants.ContainerManagerCRIO)

	if podContainer.Config.Config.StopSignal != "" {
		g.AddAnnotation(annotations.StopSignalAnnotation, podContainer.Config.Config.StopSignal)
	}

	created := time.Now()
	sbox.SetCreatedAt(created)

	err = sbox.SetCRISandbox(sboxID, labels, kubeAnnotations, metadata)
	if err != nil {
		return nil, err
	}

	g.AddAnnotation(annotations.Created, created.Format(time.RFC3339Nano))

	portMappings := convertPortMappings(sbox.Config().GetPortMappings())

	portMappingsJSON, err := json.Marshal(portMappings)
	if err != nil {
		return nil, err
	}

	sbox.SetPortMappings(portMappings)

	g.AddAnnotation(annotations.PortMappings, string(portMappingsJSON))

	containerMinMemory, err := s.ContainerServer.Runtime().GetContainerMinMemory(runtimeHandler)
	if err != nil {
		return nil, err
	}

	cgroupParent, cgroupPath, err := s.config.CgroupManager().SandboxCgroupPath(sbox.Config().GetLinux().GetCgroupParent(), sboxID, containerMinMemory)
	if err != nil {
		return nil, err
	}

	if cgroupPath != "" {
		g.SetLinuxCgroupsPath(cgroupPath)
	}

	sbox.SetCgroupParent(cgroupParent)
	g.AddAnnotation(annotations.CgroupParent, cgroupParent)

	if sandboxIDMappings != nil {
		if err := g.AddOrReplaceLinuxNamespace(string(spec.UserNamespace), ""); err != nil {
			return nil, fmt.Errorf("add or replace linux namespace: %w", err)
		}

		for _, uidmap := range sandboxIDMappings.UIDs() {
			g.AddLinuxUIDMapping(uint32(uidmap.HostID), uint32(uidmap.ContainerID), uint32(uidmap.Size))
		}

		for _, gidmap := range sandboxIDMappings.GIDs() {
			g.AddLinuxGIDMapping(uint32(gidmap.HostID), uint32(gidmap.ContainerID), uint32(gidmap.Size))
		}
	}

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

	sbox.SetPodLinuxResources(resources)
	g.AddAnnotation(annotations.PodLinuxResources, string(resourcesJSON))

	seccompRef := types.SecurityProfile_Unconfined.String()
	setupSeccompForPrivCtr := (privileged && s.config.PrivilegedSeccompProfile != "")

	if !privileged || setupSeccompForPrivCtr {
		if setupSeccompForPrivCtr {
			// Inject a custom seccomp profile for a privileged container
			securityContext.Seccomp = &types.SecurityProfile{
				ProfileType:  types.SecurityProfile_Localhost,
				LocalhostRef: s.config.PrivilegedSeccompProfile,
			}
		}

		seccompConfig, err := s.ContainerServer.Runtime().Seccomp(runtimeHandler)
		if err != nil {
			return nil, err
		}

		_, ref, err := seccompConfig.Setup(
			ctx,
			s.config.SystemContext,
			nil,
			"",
			"",
			nil,
			nil,
			g,
			securityContext.GetSeccomp(),
			s.Store().GraphRoot(),
		)
		if err != nil {
			return nil, fmt.Errorf("setup seccomp: %w", err)
		}

		seccompRef = ref
	}

	hostnamePath := podContainer.RunDir + "/hostname"

	sbox.SetResolvPath(sbox.ResolvPath())
	sbox.SetDNSConfig(sbox.Config().GetDnsConfig())
	sbox.SetHostnamePath(hostnamePath)
	sbox.SetNamespaceOptions(securityContext.GetNamespaceOptions())
	sbox.SetSeccompProfilePath(seccompRef)

	sb, err := sbox.GetSandbox()
	if err != nil {
		return nil, err
	}

	if err := s.addSandbox(ctx, sb); err != nil {
		return nil, err
	}

	resourceCleaner.Add(ctx, "runSandbox: removing pod sandbox "+sboxID, func() error {
		if err := s.removeSandbox(ctx, sboxID); err != nil {
			return fmt.Errorf("could not remove pod sandbox: %w", err)
		}

		return nil
	})

	if err := s.ContainerServer.PodIDIndex().Add(sboxID); err != nil {
		return nil, err
	}

	resourceCleaner.Add(ctx, "runSandbox: deleting pod ID "+sboxID+" from idIndex", func() error {
		if err := s.ContainerServer.PodIDIndex().Delete(sboxID); err != nil && !strings.Contains(err.Error(), noSuchID) {
			return fmt.Errorf("could not delete pod id %s from idIndex: %w", sboxID, err)
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
	sysctls := s.configureGeneratorForSysctls(ctx, g, hostNetwork, hostIPC, sandboxIDMappings, req.GetConfig().GetLinux().GetSysctls())

	// set up namespaces
	// TODO: Pass interface instead of individual field.
	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox namespace creation")
	nsCleanupFuncs, err := s.configureGeneratorForSandboxNamespaces(ctx, hostNetwork, hostIPC, hostPID, sandboxIDMappings, sysctls, sb, g)
	// We want to cleanup after ourselves if we are managing any namespaces and fail in this function.
	// However, we don't immediately register this func with resourceCleaner because we need to pair the
	// ns cleanup with networkStop. Otherwise, we could try to cleanup the namespace before the network stop runs,
	// which could put us in a weird state.
	nsCleanupDescription := "runSandbox: cleaning up namespaces after failing to run sandbox " + sboxID
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

	// now that we have the namespaces, we should create the network if we're managing namespace Lifecycle
	var ips []string

	var result cnitypes.Result

	// TODO: Pass interface instead of individual field.
	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox network creation")

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
		log.Infof(ctx, "%s", nsCleanupDescription)

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
	// TODO: Pass interface instead of individual field.
	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox storage start")

	mountPoint, err := s.ContainerServer.StorageRuntimeServer().StartContainer(sboxID)
	if err != nil {
		return nil, fmt.Errorf("failed to mount container %s in pod sandbox %s(%s): %w", containerName, sb.Name(), sboxID, err)
	}

	resourceCleaner.Add(ctx, "runSandbox: stopping storage container for sandbox "+sboxID, func() error {
		if err := s.ContainerServer.StorageRuntimeServer().StopContainer(ctx, sboxID); err != nil {
			return fmt.Errorf("could not stop storage container: %s: %w", sboxID, err)
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

	if err := os.WriteFile(hostnamePath, []byte(hostname+"\n"), 0o644); err != nil {
		return nil, err
	}

	if err := label.Relabel(hostnamePath, mountLabel, false); err != nil && !errors.Is(err, unix.ENOTSUP) {
		return nil, err
	}

	if sandboxIDMappings != nil {
		rootPair := sandboxIDMappings.RootPair()
		if err := os.Chown(hostnamePath, rootPair.UID, rootPair.GID); err != nil {
			return nil, fmt.Errorf("cannot chown %s to %d:%d: %w", hostnamePath, rootPair.UID, rootPair.GID, err)
		}
	}

	mnt = spec.Mount{
		Type:        "bind",
		Source:      hostnamePath,
		Destination: "/etc/hostname",
		Options:     []string{"ro", "bind", "nodev", "nosuid", "noexec"},
	}
	g.AddMount(mnt)
	g.AddAnnotation(annotations.HostnamePath, hostnamePath)

	if sandboxIDMappings != nil {
		if securityContext.GetNamespaceOptions().GetIpc() == types.NamespaceMode_NODE {
			g.RemoveMount("/dev/mqueue")

			mqueue := spec.Mount{
				Type:        "bind",
				Source:      "/dev/mqueue",
				Destination: "/dev/mqueue",
				Options:     []string{"rw", "rbind", "nodev", "nosuid", "noexec"},
			}
			g.AddMount(mqueue)
		}

		if hostNetwork {
			g.RemoveMount("/sys")
			g.RemoveMount("/sys/cgroup")

			sysMnt := spec.Mount{
				Destination: "/sys",
				Type:        "bind",
				Source:      "/sys",
				Options:     []string{"nosuid", "noexec", "nodev", "ro", "rbind"},
			}
			g.AddMount(sysMnt)
		}

		if securityContext.GetNamespaceOptions().GetPid() == types.NamespaceMode_NODE {
			g.RemoveMount("/proc")

			proc := spec.Mount{
				Type:        "bind",
				Source:      "/proc",
				Destination: "/proc",
				Options:     []string{"rw", "rbind", "nodev", "nosuid", "noexec"},
			}
			g.AddMount(proc)
		}
	}

	g.SetRootPath(mountPoint)

	if os.Getenv(rootlessEnvName) != "" {
		makeOCIConfigurationRootless(g)
	}

	sb.SetNamespaceOptions(securityContext.GetNamespaceOptions())

	if s.config.Seccomp().IsDisabled() {
		g.Config.Linux.Seccomp = nil
	}

	g.AddAnnotation(annotations.SeccompProfilePath, seccompRef)

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
		log.Debugf(ctx, "Keeping infra container for pod %s", sboxID)
		// pauseImage, as the userRequestedImage parameter, only shows up in CRI values we return.
		container, err = oci.NewContainer(sboxID, containerName, podContainer.RunDir, logPath, labels, g.Config.Annotations, kubeAnnotations, pauseImage.StringForOutOfProcessConsumptionOnly(), nil, nil, "", nil, sboxID, false, false, false, runtimeHandler, podContainer.Dir, created, podContainer.Config.Config.StopSignal)
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
		log.Debugf(ctx, "Dropping infra container for pod %s", sboxID)
		container = oci.NewSpoofedContainer(sboxID, containerName, labels, sboxID, created, podContainer.RunDir)

		g.AddAnnotation(annotations.SpoofedContainer, "true")

		if err := s.config.CgroupManager().CreateSandboxCgroup(cgroupParent, sboxID); err != nil {
			return nil, fmt.Errorf("create dropped infra %s cgroup: %w", sboxID, err)
		}
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
		return nil, fmt.Errorf("failed to save template configuration for pod sandbox %s(%s): %w", sb.Name(), sboxID, err)
	}

	if err = g.SaveToFile(filepath.Join(podContainer.RunDir, "config.json"), saveOptions); err != nil {
		return nil, fmt.Errorf("failed to write runtime configuration for pod sandbox %s(%s): %w", sb.Name(), sboxID, err)
	}

	s.addInfraContainer(ctx, container)
	resourceCleaner.Add(ctx, "runSandbox: removing infra container "+container.ID(), func() error {
		s.removeInfraContainer(ctx, container)

		return nil
	})
	// TODO: Pass interface instead of individual field.
	s.resourceStore.SetStageForResource(ctx, sboxName, "sandbox container runtime creation")

	if err := s.createContainerPlatform(ctx, container, sb.CgroupParent(), sandboxIDMappings); err != nil {
		return nil, err
	}

	if hooks := s.hooksRetriever.Get(ctx, sb.RuntimeHandler(), sb.Annotations()); hooks != nil {
		if err := hooks.PreStart(ctx, container, sb); err != nil {
			return nil, fmt.Errorf("failed to run pre-stop hook for container %q: %w", sb.ID(), err)
		}
	}

	s.generateCRIEvent(ctx, sb.InfraContainer(), types.ContainerEventType_CONTAINER_CREATED_EVENT)

	if err := s.ContainerServer.Runtime().StartContainer(ctx, container); err != nil {
		return nil, err
	}

	resourceCleaner.Add(ctx, "runSandbox: stopping container "+container.ID(), func() error {
		// Clean-up steps from RemovePodSandbox
		if err := s.stopContainer(ctx, container, stopTimeoutFromContext(ctx)); err != nil {
			return errors.New("failed to stop container for removal")
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
			log.Errorf(ctx, "RunSandbox: failed to save progress of sandbox %s: %v", sboxID, err)
		}

		log.Infof(ctx, "RunSandbox: context was either canceled or the deadline was exceeded: %v", ctx.Err())
		// should not cleanup
		storeResource = true

		return nil, ctx.Err()
	}

	// Since it's not a context error, we can delete the resource from the store, it will be tracked in the server from now on.
	s.resourceStore.Delete(sboxName)

	sb.SetCreated()
	s.generateCRIEvent(ctx, sb.InfraContainer(), types.ContainerEventType_CONTAINER_STARTED_EVENT)

	log.Infof(ctx, "Ran pod sandbox %s with infra container: %s", container.ID(), container.Description())

	resp = &types.RunPodSandboxResponse{PodSandboxId: sboxID}

	return resp, nil
}

// populateSandboxLabels adds some fields that Kubelet specifies by default, but other clients (crictl) does not.
// While CRI-O typically only cares about the kubelet, the cost here is low. Adding this code prevents issues
// with the LogLink feature, as the unmounting relies on the existence of the UID in the sandbox labels.
func populateSandboxLabels(labels map[string]string, kubeName, kubePodUID, namespace string) map[string]string {
	if labels == nil {
		labels = make(map[string]string)
	}

	if _, ok := labels[kubeletTypes.KubernetesPodNameLabel]; !ok {
		labels[kubeletTypes.KubernetesPodNameLabel] = kubeName
	}

	if _, ok := labels[kubeletTypes.KubernetesPodNamespaceLabel]; !ok {
		labels[kubeletTypes.KubernetesPodNamespaceLabel] = namespace
	}

	if _, ok := labels[kubeletTypes.KubernetesPodUIDLabel]; !ok {
		labels[kubeletTypes.KubernetesPodUIDLabel] = kubePodUID
	}

	return labels
}

func (s *Server) configureGeneratorForSysctls(ctx context.Context, g *generate.Generator, hostNetwork, hostIPC bool, sandboxIDMappings *idtools.IDMappings, sysctls map[string]string) map[string]string {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	sysctlsToReturn := make(map[string]string)

	defaultSysctls, err := s.config.Sysctls()
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

	return configurePingGroupRangeGivenIDMappings(ctx, g, sandboxIDMappings, sysctlsToReturn)
}

func configurePingGroupRangeGivenIDMappings(ctx context.Context, g *generate.Generator, sandboxIDMappings *idtools.IDMappings, sysctls map[string]string) map[string]string {
	// We have to manually fuss with this specific sysctl.
	// It's commonly set to the max range by default "0 2147483647".
	// However, a pod with GIDMappings may not actually have the upper range set,
	// which means attempting to set this sysctl will fail with EINVAL
	// Instead, update the max of the group range to be the largest group value in the IDMappings.
	const (
		pingGroupRangeKey        = "net.ipv4.ping_group_range"
		pingGroupFullRangeBottom = "0"
		pingGroupFullRangeTop    = "2147483647"
	)

	val, ok := sysctls[pingGroupRangeKey]
	if !ok || sandboxIDMappings == nil {
		return sysctls
	}
	// Only do this if the value is `0 2147483647`
	currentRange := strings.Fields(val)
	if len(currentRange) != 2 || currentRange[0] != pingGroupFullRangeBottom || currentRange[1] != pingGroupFullRangeTop {
		return sysctls
	}

	var maxID int
	for _, mapping := range sandboxIDMappings.GIDs() {
		maxID = max(maxID, mapping.ContainerID+mapping.Size-1)
	}

	newRange := "0 " + strconv.Itoa(maxID)

	log.Debugf(ctx, "Mutating %s sysctl to %s", pingGroupRangeKey, newRange)
	g.AddLinuxSysctl(pingGroupRangeKey, newRange)
	sysctls[pingGroupRangeKey] = newRange

	return sysctls
}

// configureGeneratorForSandboxNamespaces set the linux namespaces for the generator, based on whether the pod is sharing namespaces with the host,
// as well as whether CRI-O should be managing the namespace lifecycle.
// it returns a slice of cleanup funcs, all of which are the respective NamespaceRemove() for the sandbox.
// The caller should defer the cleanup funcs if there is an error, to make sure each namespace we are managing is properly cleaned up.
func (s *Server) configureGeneratorForSandboxNamespaces(ctx context.Context, hostNetwork, hostIPC, hostPID bool, idMappings *idtools.IDMappings, sysctls map[string]string, sb *libsandbox.Sandbox, g *generate.Generator) (cleanupFuncs []func() error, retErr error) {
	_, span := log.StartSpan(ctx)
	defer span.End()
	// Since we need a process to hold open the PID namespace, CRI-O can't manage the NS lifecycle
	if hostPID {
		if err := g.RemoveLinuxNamespace(string(spec.PIDNamespace)); err != nil {
			return nil, err
		}
	}

	namespaceConfig := &nsmgr.PodNamespacesConfig{
		Sysctls:    sysctls,
		IDMappings: idMappings,
		Namespaces: []*nsmgr.PodNamespaceConfig{
			{
				Type: nsmgr.IPCNS,
				Host: hostIPC,
			},
			{
				Type: nsmgr.NETNS,
				Host: hostNetwork,
			},
			{
				Type: nsmgr.UTSNS, // there is no option for host UTSNS
			},
		},
	}
	if idMappings != nil {
		namespaceConfig.Namespaces = append(namespaceConfig.Namespaces, &nsmgr.PodNamespaceConfig{
			Type: nsmgr.USERNS,
		})
	}

	// now that we've configured the namespaces we're sharing, create them
	namespaces, err := s.config.NamespaceManager().NewPodNamespaces(namespaceConfig)
	if err != nil {
		return nil, err
	}

	sb.AddManagedNamespaces(namespaces)

	cleanupFuncs = append(cleanupFuncs, sb.RemoveManagedNamespaces)

	if err := ctrfactory.ConfigureGeneratorGivenNamespacePaths(sb.NamespacePaths(), g); err != nil {
		return cleanupFuncs, err
	}

	return cleanupFuncs, nil
}
