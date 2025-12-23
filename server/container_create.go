package server

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	securejoin "github.com/cyphar/filepath-securejoin"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"go.podman.io/common/pkg/subscriptions"
	"go.podman.io/common/pkg/timezone"
	cstorage "go.podman.io/storage"
	"go.podman.io/storage/pkg/idtools"
	"go.podman.io/storage/pkg/mount"
	"go.podman.io/storage/pkg/stringid"
	"go.podman.io/storage/pkg/unshare"
	"golang.org/x/sys/unix"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	kubeletTypes "k8s.io/kubelet/pkg/types"

	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/internal/config/rdt"
	"github.com/cri-o/cri-o/internal/factory/container"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/linklogs"
	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/resourcestore"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/internal/storage/references"
	v2 "github.com/cri-o/cri-o/pkg/annotations/v2"
	"github.com/cri-o/cri-o/pkg/config"
	"github.com/cri-o/cri-o/utils"
)

// sync with https://go.podman.io/storage/blob/7fe03f6c765f2adbc75a5691a1fb4f19e56e7071/pkg/truncindex/truncindex.go#L92
const noSuchID = "no such id"

type orderedMounts []rspec.Mount

// Len returns the number of mounts. Used in sorting.
func (m orderedMounts) Len() int {
	return len(m)
}

// Less returns true if the number of parts (a/b/c would be 3 parts) in the
// mount indexed by parameter 1 is less than that of the mount indexed by
// parameter 2. Used in sorting.
func (m orderedMounts) Less(i, j int) bool {
	return m.parts(i) < m.parts(j)
}

// Swap swaps two items in an array of mounts. Used in sorting.
func (m orderedMounts) Swap(i, j int) {
	m[i], m[j] = m[j], m[i]
}

// parts returns the number of parts in the destination of a mount. Used in sorting.
func (m orderedMounts) parts(i int) int {
	return strings.Count(filepath.Clean(m[i].Destination), string(os.PathSeparator))
}

// mounts defines how to sort runtime.Mount.
// This is the same with the Docker implementation:
//
//	https://github.com/moby/moby/blob/17.05.x/daemon/volumes.go#L26
type criOrderedMounts []*types.Mount

// Len returns the number of mounts. Used in sorting.
func (m criOrderedMounts) Len() int {
	return len(m)
}

// Less returns true if the number of parts (a/b/c would be 3 parts) in the
// mount indexed by parameter 1 is less than that of the mount indexed by
// parameter 2. Used in sorting.
func (m criOrderedMounts) Less(i, j int) bool {
	return m.parts(i) < m.parts(j)
}

// Swap swaps two items in an array of mounts. Used in sorting.
func (m criOrderedMounts) Swap(i, j int) {
	m[i], m[j] = m[j], m[i]
}

// parts returns the number of parts in the destination of a mount. Used in sorting.
func (m criOrderedMounts) parts(i int) int {
	return strings.Count(filepath.Clean(m[i].GetContainerPath()), string(os.PathSeparator))
}

// containerImageResult holds the image resolution and verification results.
type containerImageResult struct {
	userRequestedImage string
	imgResult          *storage.ImageResult
	someNameOfTheImage *references.RegistryImageReference
	imageID            storage.StorageImageID
	someRepoDigest     string
}

// Ensure mount point on which path is mounted, is shared.
func ensureShared(path string, mountInfos []*mount.Info) error {
	sourceMount, optionalOpts, err := getSourceMount(path, mountInfos)
	if err != nil {
		return err
	}

	// Make sure source mount point is shared.
	optsSplit := strings.SplitSeq(optionalOpts, " ")
	for opt := range optsSplit {
		if strings.HasPrefix(opt, "shared:") {
			return nil
		}
	}

	return fmt.Errorf("path %q is mounted on %q but it is not a shared mount", path, sourceMount)
}

// Ensure mount point on which path is mounted, is either shared or slave.
func ensureSharedOrSlave(path string, mountInfos []*mount.Info) error {
	sourceMount, optionalOpts, err := getSourceMount(path, mountInfos)
	if err != nil {
		return err
	}
	// Make sure source mount point is shared.
	optsSplit := strings.SplitSeq(optionalOpts, " ")
	for opt := range optsSplit {
		if strings.HasPrefix(opt, "shared:") {
			return nil
		} else if strings.HasPrefix(opt, "master:") {
			return nil
		}
	}

	return fmt.Errorf("path %q is mounted on %q but it is not a shared or slave mount", path, sourceMount)
}

func addImageVolumes(ctx context.Context, rootfs string, s *Server, containerInfo *storage.ContainerInfo, mountLabel string, specgen *generate.Generator) ([]rspec.Mount, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	mounts := []rspec.Mount{}

	for dest := range containerInfo.Config.Config.Volumes {
		fp, err := securejoin.SecureJoin(rootfs, dest)
		if err != nil {
			return nil, err
		}

		switch s.config.ImageVolumes {
		case config.ImageVolumesMkdir:
			IDs := idtools.IDPair{UID: int(specgen.Config.Process.User.UID), GID: int(specgen.Config.Process.User.GID)}
			if err1 := idtools.MkdirAllAndChownNew(fp, 0o755, IDs); err1 != nil {
				return nil, err1
			}

			if mountLabel != "" {
				if err1 := securityLabel(fp, mountLabel, true, false); err1 != nil {
					return nil, err1
				}
			}
		case config.ImageVolumesBind:
			volumeDirName := stringid.GenerateNonCryptoID()

			src := filepath.Join(containerInfo.RunDir, "mounts", volumeDirName)
			if err1 := os.MkdirAll(src, 0o755); err1 != nil {
				return nil, err1
			}
			// Label the source with the sandbox selinux mount label
			if mountLabel != "" {
				if err1 := securityLabel(src, mountLabel, true, false); err1 != nil {
					return nil, err1
				}
			}

			log.Debugf(ctx, "Adding bind mounted volume: %s to %s", src, dest)
			mounts = append(mounts, rspec.Mount{
				Source:      src,
				Destination: dest,
				Type:        "bind",
				Options:     []string{"private", "bind", "rw"},
			})

		case config.ImageVolumesIgnore:
			log.Debugf(ctx, "Ignoring volume %v", dest)
		default:
			log.Errorf(ctx, "Unrecognized image volumes setting")
		}
	}

	return mounts, nil
}

// resolveSymbolicLink resolves a possible symlink path. If the path is a symlink, returns resolved
// path; if not, returns the original path.
// note: strictly SecureJoin is not sufficient, as it does not error when a part of the path doesn't exist
// but simply moves on. If the last part of the path doesn't exist, it may need to be created.
func resolveSymbolicLink(scope, path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}

	if info.Mode()&os.ModeSymlink != os.ModeSymlink {
		return path, nil
	}

	if scope == "" {
		scope = "/"
	}

	return securejoin.SecureJoin(scope, path)
}

// setupContainerUser sets the UID, GID and supplemental groups in OCI runtime config.
func setupContainerUser(ctx context.Context, specgen *generate.Generator, rootfs, mountLabel, ctrRunDir string, sc *types.LinuxContainerSecurityContext, imageConfig *v1.Image) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	if sc == nil {
		return nil
	}

	if sc.GetRunAsGroup() != nil && sc.GetRunAsUser() == nil && sc.GetRunAsUsername() == "" {
		return errors.New("user group is specified without user or username")
	}

	imageUser := ""
	homedir := ""

	for _, env := range specgen.Config.Process.Env {
		if after, ok := strings.CutPrefix(env, "HOME="); ok {
			homedir = after
			if idx := strings.Index(homedir, `\n`); idx > -1 {
				return errors.New("invalid HOME environment; newline not allowed")
			}

			break
		}
	}

	if homedir == "" {
		homedir = specgen.Config.Process.Cwd
	}

	if imageConfig != nil {
		imageUser = imageConfig.Config.User
	}

	containerUser := generateUserString(
		sc.GetRunAsUsername(),
		imageUser,
		sc.GetRunAsUser(),
	)
	log.Debugf(ctx, "Container user: %q", containerUser)

	// Add uid, gid and groups from user
	uid, gid, addGroups, err := utils.GetUserInfo(rootfs, containerUser)
	if err != nil {
		return err
	}

	genPasswd := true
	genGroup := true

	for _, mount := range specgen.Config.Mounts {
		switch mount.Destination {
		case "/etc", "/etc/":
			genPasswd = false
			genGroup = false
		case "/etc/passwd":
			genPasswd = false
		case "/etc/group":
			genGroup = false
		}

		if !genPasswd && !genGroup {
			break
		}
	}

	if genPasswd {
		// verify uid exists in containers /etc/passwd, else generate a passwd with the user entry
		passwdPath, err := utils.GeneratePasswd(containerUser, uid, gid, homedir, rootfs, ctrRunDir)
		if err != nil {
			return err
		}

		if passwdPath != "" {
			if err := securityLabel(passwdPath, mountLabel, false, false); err != nil {
				return err
			}

			mnt := rspec.Mount{
				Type:        "bind",
				Source:      passwdPath,
				Destination: "/etc/passwd",
				Options:     []string{"rw", "bind", "nodev", "nosuid", "noexec"},
			}
			specgen.AddMount(mnt)
		}
	}

	if genGroup {
		if sc.GetRunAsGroup() != nil {
			gid = uint32(sc.GetRunAsGroup().GetValue())
		}

		// verify gid exists in containers /etc/group, else generate a group with the group entry
		groupPath, err := utils.GenerateGroup(gid, rootfs, ctrRunDir)
		if err != nil {
			return err
		}

		if groupPath != "" {
			if err := securityLabel(groupPath, mountLabel, false, false); err != nil {
				return err
			}

			specgen.AddMount(rspec.Mount{
				Type:        "bind",
				Source:      groupPath,
				Destination: "/etc/group",
				Options:     []string{"rw", "bind", "nodev", "nosuid", "noexec"},
			})
		}
	}

	specgen.SetProcessUID(uid)

	if sc.GetRunAsGroup() != nil {
		gid = uint32(sc.GetRunAsGroup().GetValue())
	}

	specgen.SetProcessGID(gid)
	specgen.AddProcessAdditionalGid(gid)

	supplementalGroupsPolicy := sc.GetSupplementalGroupsPolicy()

	switch supplementalGroupsPolicy {
	case types.SupplementalGroupsPolicy_Merge:
		// Add groups from /etc/passwd and SupplementalGroups defined
		// in security context.
		for _, group := range addGroups {
			specgen.AddProcessAdditionalGid(group)
		}

		for _, group := range sc.GetSupplementalGroups() {
			specgen.AddProcessAdditionalGid(uint32(group))
		}
	case types.SupplementalGroupsPolicy_Strict:
		// Don't merge group defined in /etc/passwd.
		for _, group := range sc.GetSupplementalGroups() {
			specgen.AddProcessAdditionalGid(uint32(group))
		}

	default:
		return fmt.Errorf("not implemented in this CRI-O release: SupplementalGroupsPolicy=%v", supplementalGroupsPolicy)
	}

	return nil
}

// generateUserString generates valid user string based on OCI Image Spec v1.0.0.
func generateUserString(username, imageUser string, uid *types.Int64Value) string {
	var userstr string
	if uid != nil {
		userstr = strconv.FormatInt(uid.GetValue(), 10)
	}

	if username != "" {
		userstr = username
	}
	// We use the user from the image config if nothing is provided
	if userstr == "" {
		userstr = imageUser
	}

	return userstr
}

// CreateContainer creates a new container in specified PodSandbox.
func (s *Server) CreateContainer(ctx context.Context, req *types.CreateContainerRequest) (res *types.CreateContainerResponse, retErr error) {
	if req.GetConfig() == nil {
		return nil, errors.New("config is nil")
	}

	if req.GetConfig().GetImage() == nil {
		return nil, errors.New("config image is nil")
	}

	if req.GetSandboxConfig() == nil {
		return nil, errors.New("sandbox config is nil")
	}

	if req.GetSandboxConfig().GetMetadata() == nil {
		return nil, errors.New("sandbox config metadata is nil")
	}

	log.Infof(ctx, "Creating container: %s", oci.LabelsToDescription(req.GetConfig().GetLabels()))

	// Check if image is a file. If it is a file it might be a checkpoint archive.
	checkpointImage, err := func() (bool, error) {
		if !s.config.CheckpointRestore() {
			// If CRIU support is not enabled return from
			// this check as early as possible.
			return false, nil
		}

		if _, err := os.Stat(req.GetConfig().GetImage().GetImage()); err == nil {
			log.Debugf(
				ctx,
				"%q is a file. Assuming it is a checkpoint archive",
				req.GetConfig().GetImage().GetImage(),
			)

			return true, nil
		}
		// Check if this is an OCI checkpoint image
		imageID, err := s.checkIfCheckpointOCIImage(ctx, req.GetConfig().GetImage().GetImage())
		if err != nil {
			return false, fmt.Errorf("failed to check if this is a checkpoint image: %w", err)
		}

		return imageID != nil, nil
	}()
	if err != nil {
		return nil, err
	}

	sb, err := s.getPodSandboxFromRequest(ctx, req.GetPodSandboxId())
	if err != nil {
		if errors.Is(err, sandbox.ErrIDEmpty) {
			return nil, err
		}

		return nil, fmt.Errorf("specified sandbox not found: %s: %w", req.GetPodSandboxId(), err)
	}

	if checkpointImage {
		// This might be a checkpoint image. Let's pass
		// it to the checkpoint code.
		ctrID, err := s.CRImportCheckpoint(
			ctx,
			req.GetConfig(),
			sb,
			req.GetSandboxConfig().GetMetadata().GetUid(),
		)
		if err != nil {
			return nil, err
		}

		log.Debugf(ctx, "Prepared %s for restore\n", ctrID)

		return &types.CreateContainerResponse{
			ContainerId: ctrID,
		}, nil
	}

	stopMutex := sb.StopMutex()

	stopMutex.RLock()
	defer stopMutex.RUnlock()

	if sb.Stopped() {
		return nil, fmt.Errorf("CreateContainer failed as the sandbox was stopped: %s", sb.ID())
	}

	ctr, err := container.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	if err := ctr.SetConfig(req.GetConfig(), req.GetSandboxConfig()); err != nil {
		return nil, fmt.Errorf("setting container config: %w", err)
	}

	if err := ctr.SetNameAndID(""); err != nil {
		return nil, fmt.Errorf("setting container name and ID: %w", err)
	}

	resourceCleaner := resourcestore.NewResourceCleaner()
	// in some cases, it is still necessary to reserve container resources when an error occurs (such as just a request context timeout error)
	storeResource := false

	defer func() {
		// No errors or resource need to be stored, no need to cleanup
		if retErr == nil || storeResource {
			return
		}

		if err := resourceCleaner.Cleanup(); err != nil {
			log.Errorf(ctx, "Unable to cleanup: %v", err)
		}
	}()

	if _, err = s.ReserveContainerName(ctr.ID(), ctr.Name()); err != nil {
		reservedID, getErr := s.ContainerIDForName(ctr.Name())
		if getErr != nil {
			return nil, fmt.Errorf("failed to get ID of container with reserved name (%s), after failing to reserve name with %w: %w", ctr.Name(), getErr, getErr)
		}
		// if we're able to find the container, and it's created, this is actually a duplicate request
		// Just return that container
		if reservedCtr := s.GetContainer(ctx, reservedID); reservedCtr != nil && reservedCtr.Created() {
			return &types.CreateContainerResponse{ContainerId: reservedID}, nil
		}

		cachedID, resourceErr := s.getResourceOrWait(ctx, ctr.Name(), "container")
		if resourceErr == nil {
			return &types.CreateContainerResponse{ContainerId: cachedID}, nil
		}

		return nil, fmt.Errorf("%w: %w", resourceErr, err)
	}

	s.resourceStore.SetStageForResource(ctx, ctr.Name(), "container creating")

	resourceCleaner.Add(ctx, "createCtr: releasing container name "+ctr.Name(), func() error {
		s.ReleaseContainerName(ctx, ctr.Name())

		return nil
	})

	newContainer, err := s.createSandboxContainer(ctx, ctr, sb)
	if err != nil {
		return nil, err
	}

	resourceCleaner.Add(ctx, "createCtr: deleting container "+ctr.ID()+" from storage", func() error {
		if err := s.ContainerServer.StorageRuntimeServer().DeleteContainer(ctx, ctr.ID()); err != nil {
			return fmt.Errorf("failed to cleanup container storage: %w", err)
		}

		return nil
	})

	s.addContainer(ctx, newContainer)
	resourceCleaner.Add(ctx, "createCtr: removing container "+newContainer.ID(), func() error {
		s.removeContainer(ctx, newContainer)

		return nil
	})

	if err := s.ContainerServer.CtrIDIndex().Add(ctr.ID()); err != nil {
		return nil, err
	}

	resourceCleaner.Add(ctx, "createCtr: deleting container ID "+ctr.ID()+" from idIndex", func() error {
		if err := s.ContainerServer.CtrIDIndex().Delete(ctr.ID()); err != nil && !strings.Contains(err.Error(), noSuchID) {
			return err
		}

		return nil
	})

	mappings, err := s.getSandboxIDMappings(ctx, sb)
	if err != nil {
		return nil, err
	}

	s.resourceStore.SetStageForResource(ctx, ctr.Name(), "container runtime creation")

	if err := s.createContainerPlatform(ctx, newContainer, sb.CgroupParent(), mappings); err != nil {
		return nil, err
	}

	resourceCleaner.Add(ctx, "createCtr: removing container ID "+ctr.ID()+" from runtime", func() error {
		if err := s.ContainerServer.Runtime().DeleteContainer(ctx, newContainer); err != nil {
			return fmt.Errorf("failed to delete container in runtime %s: %w", ctr.ID(), err)
		}

		return nil
	})

	if err := s.ContainerStateToDisk(ctx, newContainer); err != nil {
		log.Warnf(ctx, "Unable to write containers %s state to disk: %v", newContainer.ID(), err)
	}

	if isContextError(ctx.Err()) {
		if err := s.resourceStore.Put(ctr.Name(), newContainer, resourceCleaner); err != nil {
			log.Errorf(ctx, "CreateCtr: failed to save progress of container %s: %v", newContainer.ID(), err)
		}

		log.Infof(ctx, "CreateCtr: context was either canceled or the deadline was exceeded: %v", ctx.Err())
		// should not cleanup
		storeResource = true

		return nil, ctx.Err()
	}

	// Since it's not a context error, we can delete the resource from the store, it will be tracked in the server from now on.
	s.resourceStore.Delete(ctr.Name())

	newContainer.SetCreated()

	if err := s.nri.postCreateContainer(ctx, sb, newContainer); err != nil {
		log.Warnf(ctx, "NRI post-create event failed for container %q: %v",
			newContainer.ID(), err)
	}

	s.generateCRIEvent(ctx, newContainer, types.ContainerEventType_CONTAINER_CREATED_EVENT)

	log.Infof(ctx, "Created container %s: %s", newContainer.ID(), newContainer.Description())

	return &types.CreateContainerResponse{
		ContainerId: ctr.ID(),
	}, nil
}

func isInCRIMounts(dst string, mounts []*types.Mount) bool {
	for _, m := range mounts {
		if m.GetContainerPath() == dst {
			return true
		}
	}

	return false
}

func (s *Server) createSandboxContainer(ctx context.Context, ctr container.Container, sb *sandbox.Sandbox) (cntr *oci.Container, retErr error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	// TODO: factor generating/updating the spec into something other projects can vendor

	// eventually, we'd like to access all of these variables through the interface themselves, and do most
	// of the translation between CRI config -> oci/storage container in the container package

	// TODO: eventually, this should be in the container package, but it's going through a lot of churn
	// and SpecAddAnnotations is already being passed too many arguments
	// Filter early so any use of the annotations don't use the wrong values
	if err := s.FilterDisallowedAnnotations(sb.Annotations(), ctr.Config().GetAnnotations(), sb.RuntimeHandler()); err != nil {
		return nil, err
	}

	containerID := ctr.ID()
	containerName := ctr.Name()
	containerConfig := ctr.Config()

	if err := ctr.SetPrivileged(); err != nil {
		return nil, err
	}

	securityContext := setContainerConfigSecurityContext(containerConfig)

	specgen := s.getSpecGen(ctr, containerConfig)

	imgInfo, err := s.resolveAndVerifyContainerImage(ctx, ctr, sb)
	if err != nil {
		return nil, err
	}

	containerInfo, containerIDMappings, err := s.createStorageContainer(ctx, ctr, sb, imgInfo.userRequestedImage, imgInfo.imageID, containerName, containerID)
	if err != nil {
		return nil, err
	}

	metadata := containerConfig.GetMetadata()

	defer func() {
		if retErr != nil {
			log.Infof(ctx, "CreateCtrLinux: deleting container %s from storage", containerInfo.ID)

			if err := s.ContainerServer.StorageRuntimeServer().DeleteContainer(ctx, containerInfo.ID); err != nil {
				log.Warnf(ctx, "Failed to cleanup container directory: %v", err)
			}
		}
	}()

	mountLabel, processLabel, hostNet, maybeRelabel, skipRelabel := s.configureSELinuxLabels(ctr, sb, containerInfo, securityContext)

	cgroup2RWAnnotation, _ := v2.GetAnnotationValue(sb.Annotations(), v2.Cgroup2MountHierarchyRW)
	cgroup2RW := node.CgroupIsV2() && cgroup2RWAnnotation == "true"

	s.resourceStore.SetStageForResource(ctx, ctr.Name(), "container volume configuration")
	idMapSupport := s.ContainerServer.Runtime().RuntimeSupportsIDMap(sb.RuntimeHandler())
	rroSupport := s.ContainerServer.Runtime().RuntimeSupportsRROMounts(sb.RuntimeHandler())

	var cleanupSafeMounts []*safeMountInfo

	runtime.LockOSThread()

	cleanupFunc := func() {
		runtime.UnlockOSThread()

		for _, s := range cleanupSafeMounts {
			s.Close()
		}
	}

	defer func() {
		if err != nil {
			cleanupFunc()
		}
	}()

	containerVolumes, ociMounts, safeMounts, err := s.addOCIBindMounts(ctx, ctr, containerInfo, maybeRelabel, skipRelabel, cgroup2RW, idMapSupport, rroSupport)
	if err != nil {
		return nil, err
	}

	cleanupSafeMounts = safeMounts

	s.resourceStore.SetStageForResource(ctx, ctr.Name(), "container device creation")

	err = s.specSetDevices(ctr, sb)
	if err != nil {
		return nil, err
	}

	s.resourceStore.SetStageForResource(ctx, ctr.Name(), "container storage start")

	mountPoint, err := s.ContainerServer.StorageRuntimeServer().StartContainer(containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to mount container %s(%s): %w", containerName, containerID, err)
	}

	defer func() {
		if retErr != nil {
			log.Infof(ctx, "CreateCtrLinux: stopping storage container %s", containerID)

			if err := s.ContainerServer.StorageRuntimeServer().StopContainer(ctx, containerID); err != nil {
				log.Warnf(ctx, "Couldn't stop storage container: %v: %v", containerID, err)
			}
		}
	}()

	s.resourceStore.SetStageForResource(ctx, ctr.Name(), "container spec configuration")

	labels := containerConfig.GetLabels()

	if err := validateLabels(labels); err != nil {
		return nil, err
	}

	err = s.specSetApparmorProfile(ctx, specgen, ctr, securityContext)
	if err != nil {
		return nil, err
	}

	err = s.specSetBlockioClass(specgen, metadata.GetName(), containerConfig.GetAnnotations(), sb.Annotations())
	if err != nil {
		log.Warnf(ctx, "Reconfiguring blockio for container %s failed: %v", containerID, err)
	}

	logPath, err := ctr.LogPath(sb.LogDir())
	if err != nil {
		return nil, err
	}

	specgen.SetProcessTerminal(containerConfig.GetTty())

	if containerConfig.GetTty() {
		specgen.AddProcessEnv("TERM", "xterm")
	}

	linux := containerConfig.GetLinux()

	if err := s.setupLinuxResources(ctx, ctr, sb, containerID, containerConfig, securityContext, specgen); err != nil {
		return nil, err
	}

	if err := ctr.AddUnifiedResourcesFromAnnotations(sb.Annotations()); err != nil {
		return nil, err
	}

	var nsTargetCtr *oci.Container
	if target := securityContext.GetNamespaceOptions().GetTargetId(); target != "" {
		nsTargetCtr = s.GetContainer(ctx, target)
	}

	if err := ctr.SpecAddNamespaces(sb, nsTargetCtr, &s.config); err != nil {
		return nil, err
	}

	defer func() {
		if retErr != nil && ctr.PidNamespace() != nil {
			log.Infof(ctx, "CreateCtrLinux: clearing PID namespace for container %s", containerInfo.ID)

			if err := ctr.PidNamespace().Remove(); err != nil {
				log.Warnf(ctx, "Failed to remove PID namespace: %v", err)
			}
		}
	}()

	usernsEnabled := containerIDMappings != nil
	addSysfsMounts(ctr, containerConfig, hostNet, usernsEnabled)

	containerImageConfig := containerInfo.Config
	if containerImageConfig == nil {
		err = fmt.Errorf("empty image config for %s", imgInfo.userRequestedImage)

		return nil, err
	}

	if err := ctr.SpecSetProcessArgs(containerImageConfig); err != nil {
		return nil, err
	}

	if err := s.setupCgroupNamespace(ctr, specgen); err != nil {
		return nil, err
	}

	addShmMount(ctr, sb)

	if err := s.setupContainerMounts(ctr, sb, containerConfig, mountLabel, hostNet, specgen); err != nil {
		return nil, err
	}

	// Set hostname and add env for hostname
	specgen.SetHostname(sb.Hostname())
	specgen.AddProcessEnv("HOSTNAME", sb.Hostname())

	created := time.Now()

	if err := s.FilterDisallowedAnnotations(sb.Annotations(), imgInfo.imgResult.Annotations, sb.RuntimeHandler()); err != nil {
		return nil, fmt.Errorf("filter image annotations: %w", err)
	}

	seccompRef, err := s.setupSeccomp(ctx, ctr, sb, containerID, imgInfo.imgResult, securityContext, specgen)
	if err != nil {
		return nil, err
	}

	runtimePath, stopSignal, err := s.setupContainerRuntimeAndStopSignal(ctx, ctr, sb, containerID, containerInfo, containerConfig, containerImageConfig, metadata, specgen)
	if err != nil {
		return nil, err
	}

	err = ctr.SpecAddAnnotations(ctx, sb, containerVolumes, mountPoint, stopSignal, imgInfo.imgResult, s.config.CgroupManager().IsSystemd(), seccompRef, runtimePath)
	if err != nil {
		return nil, err
	}

	if err := s.config.Workloads.MutateSpecGivenAnnotations(ctr.Config().GetMetadata().GetName(), ctr.Spec(), sb.Annotations()); err != nil {
		return nil, err
	}

	volumeMounts, err := s.setupContainerEnvironmentAndWorkdir(ctx, specgen, containerConfig, containerImageConfig, containerInfo, mountPoint, mountLabel, linux, securityContext)
	if err != nil {
		return nil, err
	}

	processLabel, err = s.setupContainerMountsAndSystemd(ctr, sb, containerInfo, containerIDMappings, mountPoint, mountLabel, processLabel, ociMounts, volumeMounts, specgen)
	if err != nil {
		return nil, err
	}

	if s.Hooks != nil {
		newAnnotations := map[string]string{}
		maps.Copy(newAnnotations, containerConfig.GetAnnotations())

		maps.Copy(newAnnotations, sb.Annotations())

		if _, err := s.Hooks.Hooks(specgen.Config, newAnnotations, len(containerConfig.GetMounts()) > 0); err != nil {
			return nil, err
		}
	}

	if err := ctr.SpecInjectCDIDevices(); err != nil {
		return nil, err
	}

	// Set up pids limit if pids cgroup is mounted
	if node.CgroupHasPid() {
		specgen.SetLinuxResourcesPidsLimit(s.config.PidsLimit)
	}

	// by default, the root path is an empty string. set it now.
	specgen.SetRootPath(mountPoint)

	crioAnnotations := specgen.Config.Annotations

	criMetadata := &types.ContainerMetadata{
		Name:    metadata.GetName(),
		Attempt: metadata.GetAttempt(),
	}

	ociContainer, err := oci.NewContainer(containerID, containerName, containerInfo.RunDir, logPath, labels, crioAnnotations, ctr.Config().GetAnnotations(), imgInfo.userRequestedImage, imgInfo.someNameOfTheImage, &imgInfo.imageID, imgInfo.someRepoDigest, criMetadata, sb.ID(), containerConfig.GetTty(), containerConfig.GetStdin(), containerConfig.GetStdinOnce(), sb.RuntimeHandler(), containerInfo.Dir, created, stopSignal)
	if err != nil {
		return nil, err
	}

	specgen.SetLinuxMountLabel(mountLabel)
	specgen.SetProcessSelinuxLabel(processLabel)

	ociContainer.AddManagedPIDNamespace(ctr.PidNamespace())

	ociContainer.SetIDMappings(containerIDMappings)

	if err := s.setupContainerIDMappings(sb, specgen, containerIDMappings, mountPoint, containerInfo.RunDir); err != nil {
		return nil, err
	}

	if err := s.setupContainerUmask(sb, specgen); err != nil {
		return nil, err
	}

	if err := s.setupContainerEtcDirectory(ctx, ctr, ociContainer, mountPoint, mountLabel, containerIDMappings); err != nil {
		return nil, err
	}

	if os.Getenv(rootlessEnvName) != "" {
		makeOCIConfigurationRootless(specgen)
	}

	hooks := s.hooksRetriever.Get(ctx, sb.RuntimeHandler(), sb.Annotations())

	if err := s.nri.createContainer(ctx, specgen, sb, ociContainer); err != nil {
		return nil, err
	}

	defer func() {
		if retErr != nil {
			s.nri.undoCreateContainer(ctx, specgen, sb, ociContainer)
		}
	}()

	if hooks != nil {
		if err := hooks.PreCreate(ctx, specgen, sb, ociContainer); err != nil {
			return nil, fmt.Errorf("failed to run pre-create hook for container %q: %w", ociContainer.ID(), err)
		}
	}

	if emptyDirVolName, ok := v2.GetAnnotationValue(sb.Annotations(), v2.LinkLogs); ok {
		if err := linklogs.LinkContainerLogs(ctx, sb.Labels()[kubeletTypes.KubernetesPodUIDLabel], emptyDirVolName, ctr.ID(), containerConfig.GetMetadata()); err != nil {
			log.Warnf(ctx, "Failed to link container logs: %v", err)
		}
	}

	saveOptions := generate.ExportOptions{}
	if err := specgen.SaveToFile(filepath.Join(containerInfo.Dir, "config.json"), saveOptions); err != nil {
		return nil, err
	}

	if err := specgen.SaveToFile(filepath.Join(containerInfo.RunDir, "config.json"), saveOptions); err != nil {
		return nil, err
	}

	ociContainer.SetSpec(specgen.Config)
	ociContainer.SetMountPoint(mountPoint)
	ociContainer.SetSeccompProfilePath(seccompRef)

	if runtimePath != "" {
		ociContainer.SetRuntimePathForPlatform(runtimePath)
	}

	for _, cv := range containerVolumes {
		ociContainer.AddVolume(cv)
	}

	return ociContainer, nil
}

func (s *Server) setupContainerMountsAndSystemd(ctr container.Container, sb *sandbox.Sandbox, containerInfo *storage.ContainerInfo, containerIDMappings *idtools.IDMappings, mountPoint, mountLabel, processLabel string, ociMounts, volumeMounts []rspec.Mount, specgen *generate.Generator) (string, error) {
	rootUID, rootGID := 0, 0

	if containerIDMappings != nil {
		rootPair := containerIDMappings.RootPair()
		rootUID, rootGID = rootPair.UID, rootPair.GID
	}

	// Add secrets from the default and override mounts.conf files
	secretMounts := subscriptions.MountsWithUIDGID(
		mountLabel,
		containerInfo.RunDir,
		s.config.DefaultMountsFile,
		mountPoint,
		rootUID,
		rootGID,
		unshare.IsRootless(),
		ctr.DisableFips(),
	)

	disableFIPSAnnotation, _ := v2.GetAnnotationValue(sb.Annotations(), v2.DisableFIPS)
	if ctr.DisableFips() && disableFIPSAnnotation == "true" {
		if err := disableFipsForContainer(ctr, containerInfo.RunDir); err != nil {
			return "", fmt.Errorf("failed to disable FIPS for container %s: %w", ctr.ID(), err)
		}
	}

	mounts := []rspec.Mount{}
	mounts = append(mounts, ociMounts...)
	mounts = append(mounts, volumeMounts...)
	mounts = append(mounts, secretMounts...)

	sort.Sort(orderedMounts(mounts))

	for _, m := range mounts {
		rspecMount := rspec.Mount{
			Type:        "bind",
			Options:     append(m.Options, "bind"),
			Destination: m.Destination,
			Source:      m.Source,
			UIDMappings: m.UIDMappings,
			GIDMappings: m.GIDMappings,
		}
		ctr.SpecAddMount(rspecMount)
	}

	if ctr.WillRunSystemd() {
		var err error

		// Don't override the process label if it was already set.
		// Otherwise, it should be set container_init_t to run the init process
		// in a container.
		if processLabel == "" {
			processLabel, err = InitLabel(processLabel)
			if err != nil {
				return "", err
			}
		}

		setupSystemd(specgen.Mounts(), *specgen)
	}

	return processLabel, nil
}

func (s *Server) setupContainerEnvironmentAndWorkdir(ctx context.Context, specgen *generate.Generator, containerConfig *types.ContainerConfig, containerImageConfig *v1.Image, containerInfo *storage.ContainerInfo, mountPoint, mountLabel string, linux *types.LinuxContainerConfig, securityContext *types.LinuxContainerSecurityContext) ([]rspec.Mount, error) {
	// First add any configured environment variables from crio config.
	// They will get overridden if specified in the image or container config.
	specgen.AddMultipleProcessEnv(s.ContainerServer.Config().DefaultEnv)

	// Add environment variables from image the CRI configuration
	envs := mergeEnvs(containerImageConfig, containerConfig.GetEnvs())
	for _, e := range envs {
		parts := strings.SplitN(e, "=", 2)
		specgen.AddProcessEnv(parts[0], parts[1])
	}

	// Setup user and groups
	if linux != nil {
		if err := setupContainerUser(ctx, specgen, mountPoint, mountLabel, containerInfo.RunDir, securityContext, containerImageConfig); err != nil {
			return nil, err
		}
	}

	// Add image volumes
	volumeMounts, err := addImageVolumes(ctx, mountPoint, s, containerInfo, mountLabel, specgen)
	if err != nil {
		return nil, err
	}

	// Set working directory
	// Pick it up from image config first and override if specified in CRI
	containerCwd := "/"
	imageCwd := containerImageConfig.Config.WorkingDir

	if imageCwd != "" {
		containerCwd = imageCwd
	}

	runtimeCwd := containerConfig.GetWorkingDir()
	if runtimeCwd != "" {
		containerCwd = runtimeCwd
	}

	specgen.SetProcessCwd(containerCwd)

	if err := setupWorkingDirectory(mountPoint, mountLabel, containerCwd); err != nil {
		return nil, err
	}

	return volumeMounts, nil
}

func (s *Server) setupSeccomp(ctx context.Context, ctr container.Container, sb *sandbox.Sandbox, containerID string, imgResult *storage.ImageResult, securityContext *types.LinuxContainerSecurityContext, specgen *generate.Generator) (string, error) {
	seccompRef := types.SecurityProfile_Unconfined.String()

	if s.config.Seccomp().IsDisabled() && specgen.Config.Linux != nil {
		specgen.Config.Linux.Seccomp = nil

		return seccompRef, nil
	}

	setupSeccompForPrivCtr := (ctr.Privileged() && s.config.PrivilegedSeccompProfile != "")

	if !ctr.Privileged() || setupSeccompForPrivCtr {
		if setupSeccompForPrivCtr {
			// Inject a custom seccomp profile for a privileged container
			securityContext.Seccomp = &types.SecurityProfile{
				ProfileType:  types.SecurityProfile_Localhost,
				LocalhostRef: s.config.PrivilegedSeccompProfile,
			}
		}

		seccompConfig, err := s.ContainerServer.Runtime().Seccomp(sb.RuntimeHandler())
		if err != nil {
			return "", err
		}

		notifier, ref, err := seccompConfig.Setup(
			ctx,
			s.config.SystemContext,
			s.seccompNotifierChan,
			containerID,
			ctr.Config().GetMetadata().GetName(),
			sb.Annotations(),
			imgResult.Annotations,
			specgen,
			securityContext.GetSeccomp(),
			s.Store().GraphRoot(),
		)
		if err != nil {
			return "", fmt.Errorf("setup seccomp: %w", err)
		}

		if notifier != nil {
			s.seccompNotifiers.Store(containerID, notifier)
		}

		seccompRef = ref
	}

	return seccompRef, nil
}

func (s *Server) setupContainerMounts(ctr container.Container, sb *sandbox.Sandbox, containerConfig *types.ContainerConfig, mountLabel string, hostNet bool, specgen *generate.Generator) error {
	options := []string{"rw"}
	if ctr.ReadOnly(s.config.ReadOnly) {
		options = []string{"ro"}
	}

	if sb.ResolvPath() != "" {
		if err := securityLabel(sb.ResolvPath(), mountLabel, false, false); err != nil {
			return err
		}

		ctr.SpecAddMount(rspec.Mount{
			Destination: "/etc/resolv.conf",
			Type:        "bind",
			Source:      sb.ResolvPath(),
			Options:     append(options, []string{"bind", "nodev", "nosuid", "noexec"}...),
		})
	}

	if sb.HostnamePath() != "" {
		if err := securityLabel(sb.HostnamePath(), mountLabel, false, false); err != nil {
			return err
		}

		ctr.SpecAddMount(rspec.Mount{
			Destination: "/etc/hostname",
			Type:        "bind",
			Source:      sb.HostnamePath(),
			Options:     append(options, "bind"),
		})
	}

	if sb.ContainerEnvPath() != "" {
		if err := securityLabel(sb.ContainerEnvPath(), mountLabel, false, false); err != nil {
			return err
		}

		ctr.SpecAddMount(rspec.Mount{
			Destination: "/run/.containerenv",
			Type:        "bind",
			Source:      sb.ContainerEnvPath(),
			Options:     append(options, "bind"),
		})
	}

	if !isInCRIMounts("/etc/hosts", containerConfig.GetMounts()) && hostNet {
		// Only bind mount for host netns and when CRI does not give us any hosts file
		ctr.SpecAddMount(rspec.Mount{
			Destination: "/etc/hosts",
			Type:        "bind",
			Source:      "/etc/hosts",
			Options:     append(options, "bind"),
		})
	}

	if ctr.Privileged() {
		setOCIBindMountsPrivileged(specgen)
	}

	return nil
}

// configureSELinuxLabels determines the appropriate SELinux labels for a container based on its
// security context and namespace configuration. It returns the mount and process labels, along with
// flags indicating network mode and whether volume relabeling should be skipped or made optional.
func (s *Server) configureSELinuxLabels(ctr container.Container, sb *sandbox.Sandbox, containerInfo *storage.ContainerInfo, securityContext *types.LinuxContainerSecurityContext) (mountLabel, processLabel string, hostNet, maybeRelabel, skipRelabel bool) {
	mountLabel = containerInfo.MountLabel

	if !ctr.Privileged() {
		processLabel = containerInfo.ProcessLabel
	}

	hostIPC := securityContext.GetNamespaceOptions().GetIpc() == types.NamespaceMode_NODE
	hostPID := securityContext.GetNamespaceOptions().GetPid() == types.NamespaceMode_NODE
	hostNet = securityContext.GetNamespaceOptions().GetNetwork() == types.NamespaceMode_NODE

	// Don't use SELinux separation with Host Pid or IPC Namespace or privileged.
	if hostPID || hostIPC {
		processLabel, mountLabel = "", ""
	}

	if hostNet && s.config.HostNetworkDisableSELinux {
		processLabel = ""
	}

	if val, present := v2.GetAnnotationValue(sb.Annotations(), v2.TrySkipVolumeSELinuxLabel); present && val == "true" {
		maybeRelabel = true
	}

	const superPrivilegedType = "spc_t"

	if securityContext.GetSelinuxOptions().GetType() == superPrivilegedType || // super privileged container
		(ctr.SandboxConfig().GetLinux() != nil &&
			ctr.SandboxConfig().GetLinux().GetSecurityContext() != nil &&
			ctr.SandboxConfig().GetLinux().GetSecurityContext().GetSelinuxOptions() != nil &&
			ctr.SandboxConfig().GetLinux().GetSecurityContext().GetSelinuxOptions().GetType() == superPrivilegedType && // super privileged pod
			securityContext.GetSelinuxOptions().GetType() == "") {
		skipRelabel = true
	}

	return mountLabel, processLabel, hostNet, maybeRelabel, skipRelabel
}

// createStorageContainer creates the storage layer container with the specified image and ID mappings.
// It configures SELinux labels and user namespace mappings as needed for the container.
func (s *Server) createStorageContainer(ctx context.Context, ctr container.Container, sb *sandbox.Sandbox, userRequestedImage string, imageID storage.StorageImageID, containerName, containerID string) (*storage.ContainerInfo, *idtools.IDMappings, error) {
	labelOptions, err := ctr.SelinuxLabel(sb.ProcessLabel())
	if err != nil {
		return nil, nil, err
	}

	containerIDMappings, err := s.getSandboxIDMappings(ctx, sb)
	if err != nil {
		return nil, nil, err
	}

	var idMappingOptions *cstorage.IDMappingOptions
	if containerIDMappings != nil {
		idMappingOptions = &cstorage.IDMappingOptions{UIDMap: containerIDMappings.UIDs(), GIDMap: containerIDMappings.GIDs()}
	}

	metadata := ctr.Config().GetMetadata()

	s.resourceStore.SetStageForResource(ctx, ctr.Name(), "container storage creation")

	containerInfo, err := s.ContainerServer.StorageRuntimeServer().CreateContainer(s.config.SystemContext,
		sb.Name(), sb.ID(),
		userRequestedImage, imageID,
		containerName, containerID,
		metadata.GetName(),
		metadata.GetAttempt(),
		idMappingOptions,
		labelOptions,
		ctr.Privileged(),
	)
	if err != nil {
		return nil, nil, err
	}

	return &containerInfo, containerIDMappings, nil
}

// resolveAndVerifyContainerImage resolves the user-requested image reference to a concrete image,
// verifies its signature policy, and returns detailed image metadata including image ID, names, and digests.
func (s *Server) resolveAndVerifyContainerImage(ctx context.Context, ctr container.Container, sb *sandbox.Sandbox) (*containerImageResult, error) {
	userRequestedImage, err := ctr.UserRequestedImage()
	if err != nil {
		return nil, err
	}

	var imgResult *storage.ImageResult
	if id := s.ContainerServer.StorageImageServer().HeuristicallyTryResolvingStringAsIDPrefix(userRequestedImage); id != nil {
		imgResult, err = s.ContainerServer.StorageImageServer().ImageStatusByID(s.config.SystemContext, *id)
		if err != nil {
			return nil, err
		}
	} else {
		potentialMatches, err := s.ContainerServer.StorageImageServer().CandidatesForPotentiallyShortImageName(s.config.SystemContext, userRequestedImage)
		if err != nil {
			return nil, err
		}

		var imgResultErr error
		for _, name := range potentialMatches {
			imgResult, imgResultErr = s.ContainerServer.StorageImageServer().ImageStatusByName(s.config.SystemContext, name)
			if imgResultErr == nil {
				break
			}
		}

		if imgResultErr != nil {
			return nil, imgResultErr
		}
	}

	if userRequestedImage == "" {
		return nil, errors.New("internal error: successfully found an image, but userRequestedImage is empty")
	}

	someNameOfTheImage := imgResult.SomeNameOfThisImage
	imageID := imgResult.ID

	// For ImageRef, prefer user-requested image if it's a digest in RepoDigests.
	// This ensures manifest list digests appear in ImageRef (not platform-specific ones).
	someRepoDigest := ""
	if len(imgResult.RepoDigests) > 0 {
		someRepoDigest = imgResult.RepoDigests[0]
		if slices.Contains(imgResult.RepoDigests, userRequestedImage) {
			someRepoDigest = userRequestedImage
		}
	}

	if err := s.verifyImageSignature(ctx, sb.Metadata().GetNamespace(), ctr.Config().GetImage().GetUserSpecifiedImage(), imgResult); err != nil {
		return nil, err
	}

	return &containerImageResult{
		userRequestedImage: userRequestedImage,
		imgResult:          imgResult,
		someNameOfTheImage: someNameOfTheImage,
		imageID:            imageID,
		someRepoDigest:     someRepoDigest,
	}, nil
}

func setupWorkingDirectory(rootfs, mountLabel, containerCwd string) error {
	fp, err := securejoin.SecureJoin(rootfs, containerCwd)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(fp, 0o755); err != nil {
		return err
	}

	if mountLabel != "" {
		if err1 := securityLabel(fp, mountLabel, false, false); err1 != nil {
			return err1
		}
	}

	return nil
}

// makeAccessible changes the path permission and each parent directory to have --x--x--x.
func makeAccessible(path string, uid, gid int) error {
	for ; path != "/"; path = filepath.Dir(path) {
		var st unix.Stat_t

		err := unix.Stat(path, &st)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}

			return err
		}

		if int(st.Uid) == uid && int(st.Gid) == gid {
			continue
		}

		perm := os.FileMode(st.Mode) & os.ModePerm
		if perm&0o111 != 0o111 {
			if err := os.Chmod(path, perm|0o111); err != nil {
				return err
			}
		}
	}

	return nil
}

func toContainer(id uint32, idMap []idtools.IDMap) uint32 {
	hostID := int(id)
	if idMap == nil {
		return uint32(hostID)
	}

	for _, m := range idMap {
		if hostID >= m.HostID && hostID < m.HostID+m.Size {
			contID := m.ContainerID + (hostID - m.HostID)

			return uint32(contID)
		}
	}
	// If the ID cannot be mapped, it means the RunAsUser or RunAsGroup was not specified
	// so just use the original value.
	return id
}

func configureTimezone(tz, containerRunDir, mountPoint, mountLabel, etcPath, containerID string, options []string, ctr container.Container) error {
	localTimePath, err := timezone.ConfigureContainerTimeZone(tz, containerRunDir, mountPoint, etcPath, containerID)
	if err != nil {
		return fmt.Errorf("setting timezone for container %s: %w", containerID, err)
	}

	if localTimePath != "" {
		if err := securityLabel(localTimePath, mountLabel, false, false); err != nil {
			return err
		}

		ctr.SpecAddMount(rspec.Mount{
			Destination: "/etc/localtime",
			Type:        "bind",
			Source:      localTimePath,
			Options:     append(options, []string{"bind", "nodev", "nosuid", "noexec"}...),
		})
	}

	return nil
}

// verifyImageSignature verifies the signature of a container image.
func (s *Server) verifyImageSignature(ctx context.Context, namespace, userSpecifiedImage string, status *storage.ImageResult) error {
	systemCtx, err := s.contextForNamespace(namespace)
	if err != nil {
		return fmt.Errorf("get context for namespace: %w", err)
	}

	// WARNING: This hard-codes an assumption that SignaturePolicyPath set specifically for the namespace is never less restrictive
	// than the default system-wide policy, i.e. that if an image is successfully pulled, it always conforms to the system-wide policy.
	if systemCtx.SignaturePolicyPath != "" {
		// This will likely fail in a container restore case.
		// This is okay; in part because container restores are an alpha feature,
		// and it is meaningless to try to verify an image that isn't even an image
		// (like a checkpointed file is).
		if userSpecifiedImage == "" {
			return errors.New("user specified image not specified, cannot verify image signature")
		}

		var userSpecifiedImageRef references.RegistryImageReference

		userSpecifiedImageRef, err = references.ParseRegistryImageReferenceFromOutOfProcessData(userSpecifiedImage)
		if err != nil {
			return fmt.Errorf("unable to get userSpecifiedImageRef from user specified image %q: %w", userSpecifiedImage, err)
		}

		if err := s.ContainerServer.StorageImageServer().IsRunningImageAllowed(ctx, &systemCtx, userSpecifiedImageRef, status.ID); err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) setupContainerIDMappings(sb *sandbox.Sandbox, specgen *generate.Generator, containerIDMappings *idtools.IDMappings, mountPoint, containerRunDir string) error {
	if containerIDMappings != nil {
		s.finalizeUserMapping(sb, specgen, containerIDMappings)

		// Only add ID mappings if we're not joining the sandbox's user namespace.
		// If the sandbox has a user namespace, the container will join it via namespace path
		// (configured in SpecAddNamespaces). In this case, we should NOT add ID mappings to
		// avoid the conflict of having both a userns path and ID mappings in the OCI spec.
		sandboxHasUserNs := sb.UserNsPath() != ""
		if !sandboxHasUserNs {
			for _, uidmap := range containerIDMappings.UIDs() {
				specgen.AddLinuxUIDMapping(uint32(uidmap.HostID), uint32(uidmap.ContainerID), uint32(uidmap.Size))
			}

			for _, gidmap := range containerIDMappings.GIDs() {
				specgen.AddLinuxGIDMapping(uint32(gidmap.HostID), uint32(gidmap.ContainerID), uint32(gidmap.Size))
			}
		}

		rootPair := containerIDMappings.RootPair()
		rootUID, rootGID := rootPair.UID, rootPair.GID

		for _, path := range []string{mountPoint, containerRunDir} {
			if err := makeAccessible(path, rootUID, rootGID); err != nil {
				return fmt.Errorf("cannot make %s accessible to %d:%d: %w", path, rootUID, rootGID, err)
			}
		}
	} else if err := specgen.RemoveLinuxNamespace(string(rspec.UserNamespace)); err != nil {
		return err
	}

	return nil
}

func (s *Server) setupContainerUmask(sb *sandbox.Sandbox, specgen *generate.Generator) error {
	if v, _ := v2.GetAnnotationValue(sb.Annotations(), v2.Umask); v != "" {
		umaskRegexp := regexp.MustCompile(`^[0-7]{1,4}$`)
		if !umaskRegexp.MatchString(v) {
			return fmt.Errorf("invalid umask string %s", v)
		}

		decVal, err := strconv.ParseUint(v, 8, 32)
		if err != nil {
			return err
		}

		umask := uint32(decVal)
		specgen.Config.Process.User.Umask = &umask
	}

	return nil
}

func (s *Server) setupContainerEtcDirectory(ctx context.Context, ctr container.Container, ociContainer *oci.Container, mountPoint, mountLabel string, containerIDMappings *idtools.IDMappings) error {
	etcPath := filepath.Join(mountPoint, "/etc")

	// Warn users if the container /etc directory path points to a location
	// that is not a regular directory. This could indicate that something
	// might be afoot.
	etc, err := os.Lstat(etcPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if err == nil && !etc.IsDir() {
		log.Warnf(ctx, "Detected /etc path for container %s is not a directory", ctr.ID())
	}

	// The /etc directory can be subjected to various attempts on the path (directory)
	// traversal attacks. As such, we need to ensure that its path will be relative to
	// the base (or root, if you wish) of the container to mitigate a container escape.
	etcPath, err = securejoin.SecureJoin(mountPoint, "/etc")
	if err != nil {
		return fmt.Errorf("failed to resolve container /etc directory path: %w", err)
	}

	// Create the /etc directory only when it doesn't exist.
	if _, err := os.Stat(etcPath); err != nil && os.IsNotExist(err) {
		rootPair := idtools.IDPair{UID: 0, GID: 0}
		if containerIDMappings != nil {
			rootPair = containerIDMappings.RootPair()
		}

		if err := idtools.MkdirAllAndChown(etcPath, 0o755, rootPair); err != nil {
			return fmt.Errorf("failed to create container /etc directory: %w", err)
		}
	}

	// Add a symbolic link from /proc/mounts to /etc/mtab to keep compatibility with legacy
	// Linux distributions and Docker.
	//
	// We cannot use SecureJoin here, as the /etc/mtab can already be symlinked from somewhere
	// else in some cases, and doing so would resolve an existing mtab path to the symbolic
	// link target location, for example, the /etc/proc/self/mounts, which breaks container
	// creation.
	if err := os.Symlink("/proc/mounts", filepath.Join(etcPath, "mtab")); err != nil && !os.IsExist(err) {
		return err
	}

	// Configure timezone for the container if it is set.
	options := []string{"rw"}
	if ctr.ReadOnly(s.config.ReadOnly) {
		options = []string{"ro"}
	}

	if err := configureTimezone(s.ContainerServer.Runtime().Timezone(), ociContainer.BundlePath(), mountPoint, mountLabel, etcPath, ociContainer.ID(), options, ctr); err != nil {
		return fmt.Errorf("failed to configure timezone for container %s: %w", ociContainer.ID(), err)
	}

	return nil
}

func (s *Server) setupContainerRuntimeAndStopSignal(ctx context.Context, ctr container.Container, sb *sandbox.Sandbox, containerID string, containerInfo *storage.ContainerInfo, containerConfig *types.ContainerConfig, containerImageConfig *v1.Image, metadata *types.ContainerMetadata, specgen *generate.Generator) (runtimePath, stopSignal string, err error) {
	// Get RDT class
	var rdtClass string

	rdtClass, err = s.ContainerServer.Config().Rdt().ContainerClassFromAnnotations(metadata.GetName(), containerConfig.GetAnnotations(), sb.Annotations())
	if err != nil {
		return "", "", err
	}

	if rdtClass != "" {
		log.Debugf(ctx, "Setting RDT ClosID of container %s to %q", containerID, rdt.ResctrlPrefix+rdtClass)
		// TODO: patch runtime-tools to support setting ClosID via a helper func similar to SetLinuxIntelRdtL3CacheSchema()
		specgen.Config.Linux.IntelRdt = &rspec.LinuxIntelRdt{ClosID: rdt.ResctrlPrefix + rdtClass}
	}
	// compute the runtime path for a given container
	platform := containerInfo.Config.OS + "/" + containerInfo.Config.Architecture

	runtimePath, err = s.ContainerServer.Runtime().PlatformRuntimePath(sb.RuntimeHandler(), platform)
	if err != nil {
		return "", "", err
	}

	// Determine the stop signal for the container. If a custom stop signal is provided
	// via CRI API, use it. Otherwise, fall back to the image's default stop signal as
	// defined in its configuration.
	// https://github.com/kubernetes/enhancements/issues/4960
	stopSignal = containerImageConfig.Config.StopSignal

	if signal := ctr.Config().GetStopSignal(); signal != types.Signal_RUNTIME_DEFAULT {
		log.Debugf(ctx, "Override stop signal to %s", signal)
		stopSignal = signal.String()
	}

	return runtimePath, stopSignal, nil
}

// setupLinuxResources configures Linux-specific resources and security settings for the container,
// including resource limits, cgroup paths, masked paths, and privilege settings.
func (s *Server) setupLinuxResources(ctx context.Context, ctr container.Container, sb *sandbox.Sandbox, containerID string, containerConfig *types.ContainerConfig, securityContext *types.LinuxContainerSecurityContext, specgen *generate.Generator) error {
	linux := containerConfig.GetLinux()
	if linux != nil {
		resources := linux.GetResources()
		if resources != nil {
			containerMinMemory, err := s.ContainerServer.Runtime().GetContainerMinMemory(sb.RuntimeHandler())
			if err != nil {
				return err
			}

			err = ctr.SpecSetLinuxContainerResources(resources, containerMinMemory)
			if err != nil {
				return err
			}
		}

		specgen.SetLinuxCgroupsPath(s.config.CgroupManager().ContainerCgroupPath(sb.CgroupParent(), containerID))

		if len(securityContext.GetMaskedPaths()) != 0 {
			securityContext.MaskedPaths = appendDefaultMaskedPaths(securityContext.GetMaskedPaths())
			log.Debugf(ctx, "Using masked paths: %v", strings.Join(securityContext.GetMaskedPaths(), ", "))
		}

		err := ctr.SpecSetPrivileges(ctx, securityContext, &s.config)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) setupCgroupNamespace(ctr container.Container, specgen *generate.Generator) error {
	// When running on cgroupv2, automatically add a cgroup namespace for not privileged containers.
	if !ctr.Privileged() && node.CgroupIsV2() {
		if err := specgen.AddOrReplaceLinuxNamespace(string(rspec.CgroupNamespace), ""); err != nil {
			return err
		}
	}

	return nil
}
