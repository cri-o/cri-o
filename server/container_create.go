package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/cri-o/cri-o/internal/factory/container"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/resourcestore"
	"github.com/cri-o/cri-o/utils"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// sync with https://github.com/containers/storage/blob/7fe03f6c765f2adbc75a5691a1fb4f19e56e7071/pkg/truncindex/truncindex.go#L92
const noSuchID = "no such id"

// setupContainerUser sets the UID, GID and supplemental groups in OCI runtime config
func setupContainerUser(ctx context.Context, specgen *generate.Generator, rootfs, mountLabel, ctrRunDir string, sc *types.LinuxContainerSecurityContext, imageConfig *v1.Image) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

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
			if idx := strings.Index(homedir, `\n`); idx > -1 {
				return fmt.Errorf("invalid HOME environment; newline not allowed")
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
	if req.Config == nil {
		return nil, errors.New("config is nil")
	}
	if req.Config.Image == nil {
		return nil, errors.New("config image is nil")
	}
	if req.SandboxConfig == nil {
		return nil, errors.New("sandbox config is nil")
	}
	if req.SandboxConfig.Metadata == nil {
		return nil, errors.New("sandbox config metadata is nil")
	}

	log.Infof(ctx, "Creating container: %s", translateLabelsToDescription(req.GetConfig().GetLabels()))

	// Check if image is a file. If it is a file it might be a checkpoint archive.
	checkpointImage, err := func() (bool, error) {
		if !s.config.CheckpointRestore() {
			// If CRIU support is not enabled return from
			// this check as early as possible.
			return false, nil
		}
		if _, err := os.Stat(req.Config.Image.Image); err == nil {
			log.Debugf(
				ctx,
				"%q is a file. Assuming it is a checkpoint archive",
				req.Config.Image.Image,
			)
			return true, nil
		}
		// Check if this is an OCI checkpoint image
		imageID, err := s.checkIfCheckpointOCIImage(ctx, req.Config.Image.Image)
		if err != nil {
			return false, fmt.Errorf("failed to check if this is a checkpoint image: %w", err)
		}

		return imageID != nil, nil
	}()
	if err != nil {
		return nil, err
	}

	if checkpointImage {
		// This might be a checkpoint image. Let's pass
		// it to the checkpoint code.
		ctrID, err := s.CRImportCheckpoint(
			ctx,
			req.Config,
			req.PodSandboxId,
			req.SandboxConfig.Metadata.Uid,
		)
		if err != nil {
			return nil, err
		}
		log.Debugf(ctx, "Prepared %s for restore\n", ctrID)

		return &types.CreateContainerResponse{
			ContainerId: ctrID,
		}, nil
	}

	sb, err := s.getPodSandboxFromRequest(ctx, req.PodSandboxId)
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

	if err := ctr.SetNameAndID(""); err != nil {
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
		// Just return that container
		if reservedCtr := s.GetContainer(ctx, reservedID); reservedCtr != nil && reservedCtr.Created() {
			return &types.CreateContainerResponse{ContainerId: reservedID}, nil
		}
		cachedID, resourceErr := s.getResourceOrWait(ctx, ctr.Name(), "container")
		if resourceErr == nil {
			return &types.CreateContainerResponse{ContainerId: cachedID}, nil
		}
		return nil, fmt.Errorf("%v: %w", resourceErr, err)
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
		if err := s.StorageRuntimeServer().DeleteContainer(ctx, ctr.ID()); err != nil {
			return fmt.Errorf("failed to cleanup container storage: %w", err)
		}
		return nil
	})

	s.addContainer(ctx, newContainer)
	resourceCleaner.Add(ctx, "createCtr: removing container "+newContainer.ID(), func() error {
		s.removeContainer(ctx, newContainer)
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

	mappings, err := s.getSandboxIDMappings(ctx, sb)
	if err != nil {
		return nil, err
	}

	s.resourceStore.SetStageForResource(ctx, ctr.Name(), "container runtime creation")
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
