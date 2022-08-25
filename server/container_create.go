package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/containers/storage/pkg/idtools"
	"github.com/containers/storage/pkg/mount"
	"github.com/containers/storage/pkg/stringid"
	"github.com/cri-o/cri-o/internal/factory/container"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/resourcestore"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/pkg/config"
	"github.com/cri-o/cri-o/utils"
	securejoin "github.com/cyphar/filepath-securejoin"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// sync with https://github.com/containers/storage/blob/7fe03f6c765f2adbc75a5691a1fb4f19e56e7071/pkg/truncindex/truncindex.go#L92
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

// Swap swaps two items in an array of mounts. Used in sorting
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

// Swap swaps two items in an array of mounts. Used in sorting
func (m criOrderedMounts) Swap(i, j int) {
	m[i], m[j] = m[j], m[i]
}

// parts returns the number of parts in the destination of a mount. Used in sorting.
func (m criOrderedMounts) parts(i int) int {
	return strings.Count(filepath.Clean(m[i].ContainerPath), string(os.PathSeparator))
}

// Ensure mount point on which path is mounted, is shared.
func ensureShared(path string, mountInfos []*mount.Info) error {
	sourceMount, optionalOpts, err := getSourceMount(path, mountInfos)
	if err != nil {
		return err
	}

	// Make sure source mount point is shared.
	optsSplit := strings.Split(optionalOpts, " ")
	for _, opt := range optsSplit {
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
	optsSplit := strings.Split(optionalOpts, " ")
	for _, opt := range optsSplit {
		if strings.HasPrefix(opt, "shared:") {
			return nil
		} else if strings.HasPrefix(opt, "master:") {
			return nil
		}
	}
	return fmt.Errorf("path %q is mounted on %q but it is not a shared or slave mount", path, sourceMount)
}

func addImageVolumes(ctx context.Context, rootfs string, s *Server, containerInfo *storage.ContainerInfo, mountLabel string, specgen *generate.Generator) ([]rspec.Mount, error) {
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

// setupContainerUser sets the UID, GID and supplemental groups in OCI runtime config
func setupContainerUser(ctx context.Context, specgen *generate.Generator, rootfs, mountLabel, ctrRunDir string, sc *types.LinuxContainerSecurityContext, imageConfig *v1.Image) error {
	if sc == nil {
		return nil
	}
	if sc.RunAsGroup != nil && sc.RunAsUser == nil && sc.RunAsUsername == "" {
		return fmt.Errorf("user group is specified without user or username")
	}
	imageUser := ""
	homedir := ""
	for _, env := range specgen.Config.Process.Env {
		if strings.HasPrefix(env, "HOME=") {
			homedir = strings.TrimPrefix(env, "HOME=")
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
		sc.RunAsUsername,
		imageUser,
		sc.RunAsUser,
	)
	log.Debugf(ctx, "CONTAINER USER: %+v", containerUser)

	// Add uid, gid and groups from user
	uid, gid, addGroups, err := utils.GetUserInfo(rootfs, containerUser)
	if err != nil {
		return err
	}

	genPasswd := true
	for _, mount := range specgen.Config.Mounts {
		if mount.Destination == "/etc" ||
			mount.Destination == "/etc/" ||
			mount.Destination == "/etc/passwd" {
			genPasswd = false
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

	specgen.SetProcessUID(uid)
	if sc.RunAsGroup != nil {
		gid = uint32(sc.RunAsGroup.Value)
	}
	specgen.SetProcessGID(gid)
	specgen.AddProcessAdditionalGid(gid)

	for _, group := range addGroups {
		specgen.AddProcessAdditionalGid(group)
	}

	// Add groups from CRI
	groups := sc.SupplementalGroups
	for _, group := range groups {
		specgen.AddProcessAdditionalGid(uint32(group))
	}
	return nil
}

// generateUserString generates valid user string based on OCI Image Spec v1.0.0.
func generateUserString(username, imageUser string, uid *types.Int64Value) string {
	var userstr string
	if uid != nil {
		userstr = strconv.FormatInt(uid.Value, 10)
	}
	if username != "" {
		userstr = username
	}
	// We use the user from the image config if nothing is provided
	if userstr == "" {
		userstr = imageUser
	}
	if userstr == "" {
		return ""
	}
	return userstr
}

// CreateContainer creates a new container in specified PodSandbox
func (s *Server) CreateContainer(ctx context.Context, req *types.CreateContainerRequest) (res *types.CreateContainerResponse, retErr error) {
	log.Infof(ctx, "Creating container: %s", translateLabelsToDescription(req.GetConfig().GetLabels()))

	sb, err := s.getPodSandboxFromRequest(req.PodSandboxId)
	if err != nil {
		if err == sandbox.ErrIDEmpty {
			return nil, err
		}
		return nil, fmt.Errorf("specified sandbox not found: %s: %w", req.PodSandboxId, err)
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

	if err := ctr.SetConfig(req.Config, req.SandboxConfig); err != nil {
		return nil, fmt.Errorf("setting container config: %w", err)
	}

	if err := ctr.SetNameAndID(); err != nil {
		return nil, fmt.Errorf("setting container name and ID: %w", err)
	}

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

	if _, err = s.ReserveContainerName(ctr.ID(), ctr.Name()); err != nil {
		reservedID, getErr := s.ContainerIDForName(ctr.Name())
		if getErr != nil {
			return nil, fmt.Errorf("failed to get ID of container with reserved name (%s), after failing to reserve name with %v: %w", ctr.Name(), getErr, getErr)
		}
		// if we're able to find the container, and it's created, this is actually a duplicate request
		// from a client that does not behave like the kubelet (like crictl)
		if reservedCtr := s.GetContainer(reservedID); reservedCtr != nil && reservedCtr.Created() {
			return nil, err
		}
		cachedID, resourceErr := s.getResourceOrWait(ctx, ctr.Name(), "container")
		if resourceErr == nil {
			return &types.CreateContainerResponse{ContainerId: cachedID}, nil
		}
		return nil, fmt.Errorf("%v: %w", resourceErr, err)
	}

	resourceCleaner.Add(ctx, "createCtr: releasing container name "+ctr.Name(), func() error {
		s.ReleaseContainerName(ctr.Name())
		return nil
	})

	newContainer, err := s.createSandboxContainer(ctx, ctr, sb)
	if err != nil {
		return nil, err
	}
	resourceCleaner.Add(ctx, "createCtr: deleting container "+ctr.ID()+" from storage", func() error {
		if err := s.StorageRuntimeServer().DeleteContainer(ctr.ID()); err != nil {
			return fmt.Errorf("failed to cleanup container storage: %w", err)
		}
		return nil
	})

	s.addContainer(newContainer)
	resourceCleaner.Add(ctx, "createCtr: removing container "+newContainer.ID(), func() error {
		s.removeContainer(newContainer)
		return nil
	})

	if err := s.CtrIDIndex().Add(ctr.ID()); err != nil {
		return nil, err
	}
	resourceCleaner.Add(ctx, "createCtr: deleting container ID "+ctr.ID()+" from idIndex", func() error {
		if err := s.CtrIDIndex().Delete(ctr.ID()); err != nil && !strings.Contains(err.Error(), noSuchID) {
			return err
		}
		return nil
	})

	mappings, err := s.getSandboxIDMappings(sb)
	if err != nil {
		return nil, err
	}

	if err := s.createContainerPlatform(ctx, newContainer, sb.CgroupParent(), mappings); err != nil {
		return nil, err
	}
	resourceCleaner.Add(ctx, "createCtr: removing container ID "+ctr.ID()+" from runtime", func() error {
		if err := s.Runtime().DeleteContainer(ctx, newContainer); err != nil {
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
		return nil, ctx.Err()
	}

	newContainer.SetCreated()

	log.Infof(ctx, "Created container %s: %s", newContainer.ID(), newContainer.Description())
	return &types.CreateContainerResponse{
		ContainerId: ctr.ID(),
	}, nil
}

func isInCRIMounts(dst string, mounts []*types.Mount) bool {
	for _, m := range mounts {
		if m.ContainerPath == dst {
			return true
		}
	}
	return false
}
