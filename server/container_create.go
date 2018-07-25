package server

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	dockermounts "github.com/docker/docker/pkg/mount"
	"github.com/docker/docker/pkg/stringid"
	"github.com/docker/docker/pkg/symlink"
	"github.com/kubernetes-incubator/cri-o/lib"
	"github.com/kubernetes-incubator/cri-o/lib/sandbox"
	"github.com/kubernetes-incubator/cri-o/pkg/apparmor"
	"github.com/kubernetes-incubator/cri-o/pkg/seccomp"
	"github.com/kubernetes-incubator/cri-o/pkg/storage"
	"github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/runc/libcontainer/user"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

const (
	seccompUnconfined      = "unconfined"
	seccompRuntimeDefault  = "runtime/default"
	seccompDockerDefault   = "docker/default"
	seccompLocalhostPrefix = "localhost/"

	scopePrefix           = "crio"
	defaultCgroupfsParent = "/crio"
	defaultSystemdParent  = "system.slice"
)

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

// Ensure mount point on which path is mounted, is shared.
func ensureShared(path string, mountInfos []*dockermounts.Info) error {
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
func ensureSharedOrSlave(path string, mountInfos []*dockermounts.Info) error {
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

func getMountInfo(mountInfos []*dockermounts.Info, dir string) *dockermounts.Info {
	for _, m := range mountInfos {
		if m.Mountpoint == dir {
			return m
		}
	}
	return nil
}

func getSourceMount(source string, mountInfos []*dockermounts.Info) (string, string, error) {
	mountinfo := getMountInfo(mountInfos, source)
	if mountinfo != nil {
		return source, mountinfo.Optional, nil
	}

	path := source
	for {
		path = filepath.Dir(path)
		mountinfo = getMountInfo(mountInfos, path)
		if mountinfo != nil {
			return path, mountinfo.Optional, nil
		}

		if path == "/" {
			break
		}
	}

	// If we are here, we did not find parent mount. Something is wrong.
	return "", "", fmt.Errorf("Could not find source mount of %s", source)
}

func addImageVolumes(rootfs string, s *Server, containerInfo *storage.ContainerInfo, specgen *generate.Generator, mountLabel string) ([]rspec.Mount, error) {
	mounts := []rspec.Mount{}
	for dest := range containerInfo.Config.Config.Volumes {
		fp, err := symlink.FollowSymlinkInScope(filepath.Join(rootfs, dest), rootfs)
		if err != nil {
			return nil, err
		}
		switch s.config.ImageVolumes {
		case lib.ImageVolumesMkdir:
			if err1 := os.MkdirAll(fp, 0644); err1 != nil {
				return nil, err1
			}
		case lib.ImageVolumesBind:
			volumeDirName := stringid.GenerateNonCryptoID()
			src := filepath.Join(containerInfo.RunDir, "mounts", volumeDirName)
			if err1 := os.MkdirAll(src, 0644); err1 != nil {
				return nil, err1
			}
			// Label the source with the sandbox selinux mount label
			if mountLabel != "" {
				if err1 := securityLabel(src, mountLabel, true); err1 != nil {
					return nil, err1
				}
			}

			logrus.Debugf("Adding bind mounted volume: %s to %s", src, dest)
			mounts = append(mounts, rspec.Mount{
				Source:      src,
				Destination: dest,
				Options:     []string{"rw"},
			})

		case lib.ImageVolumesIgnore:
			logrus.Debugf("Ignoring volume %v", dest)
		default:
			logrus.Fatalf("Unrecognized image volumes setting")
		}
	}
	return mounts, nil
}

// resolveSymbolicLink resolves a possbile symlink path. If the path is a symlink, returns resolved
// path; if not, returns the original path.
func resolveSymbolicLink(path, scope string) (string, error) {
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
	return symlink.FollowSymlinkInScope(path, scope)
}

func addDevices(sb *sandbox.Sandbox, containerConfig *pb.ContainerConfig, specgen *generate.Generator) error {
	return addDevicesPlatform(sb, containerConfig, specgen)
}

// buildOCIProcessArgs build an OCI compatible process arguments slice.
func buildOCIProcessArgs(containerKubeConfig *pb.ContainerConfig, imageOCIConfig *v1.Image) ([]string, error) {
	//# Start the nginx container using the default command, but use custom
	//arguments (arg1 .. argN) for that command.
	//kubectl run nginx --image=nginx -- <arg1> <arg2> ... <argN>

	//# Start the nginx container using a different command and custom arguments.
	//kubectl run nginx --image=nginx --command -- <cmd> <arg1> ... <argN>

	kubeCommands := containerKubeConfig.Command
	kubeArgs := containerKubeConfig.Args

	// merge image config and kube config
	// same as docker does today...
	if imageOCIConfig != nil {
		if len(kubeCommands) == 0 {
			if len(kubeArgs) == 0 {
				kubeArgs = imageOCIConfig.Config.Cmd
			}
			if kubeCommands == nil {
				kubeCommands = imageOCIConfig.Config.Entrypoint
			}
		}
	}

	if len(kubeCommands) == 0 && len(kubeArgs) == 0 {
		return nil, fmt.Errorf("no command specified")
	}

	// create entrypoint and args
	var entrypoint string
	var args []string
	if len(kubeCommands) != 0 {
		entrypoint = kubeCommands[0]
		args = append(kubeCommands[1:], kubeArgs...)
	} else {
		entrypoint = kubeArgs[0]
		args = kubeArgs[1:]
	}

	processArgs := append([]string{entrypoint}, args...)

	logrus.Debugf("OCI process args %v", processArgs)

	return processArgs, nil
}

// addOCIHook look for hooks programs installed in hooksDirPath and add them to spec
func addOCIHook(specgen *generate.Generator, hook lib.HookParams) error {
	logrus.Debugf("AddOCIHook %s", hook.Hook)
	for _, stage := range hook.Stages {
		h := rspec.Hook{
			Path: hook.Hook,
			Args: append([]string{hook.Hook}, hook.Arguments...),
			Env:  []string{fmt.Sprintf("stage=%s", stage)},
		}
		switch stage {
		case "prestart":
			specgen.AddPreStartHook(h)
		case "poststart":
			specgen.AddPostStartHook(h)
		case "poststop":
			specgen.AddPostStopHook(h)
		}
	}
	return nil
}

// setupContainerUser sets the UID, GID and supplemental groups in OCI runtime config
func setupContainerUser(specgen *generate.Generator, rootfs string, sc *pb.LinuxContainerSecurityContext, imageConfig *v1.Image) error {
	if sc != nil {
		containerUser := ""
		// Case 1: run as user is set by kubelet
		if sc.GetRunAsUser() != nil {
			containerUser = strconv.FormatInt(sc.GetRunAsUser().GetValue(), 10)
		} else {
			// Case 2: run as username is set by kubelet
			userName := sc.GetRunAsUsername()
			if userName != "" {
				containerUser = userName
			} else {
				// Case 3: get user from image config
				if imageConfig != nil {
					imageUser := imageConfig.Config.User
					if imageUser != "" {
						containerUser = imageUser
					}
				}
			}
		}
		if sc.GetRunAsUser() == nil && sc.GetRunAsUsername() == "" && sc.GetRunAsGroup() != nil {
			return fmt.Errorf("RunAsGroup should be specified only with RunAsUser or RunAsUsername")
		}
		groupstr := strconv.FormatInt(sc.GetRunAsGroup().GetValue(), 10)
		if groupstr != "" {
			containerUser = containerUser + ":" + groupstr
		}

		logrus.Debugf("CONTAINER USER: %+v", containerUser)

		// Add uid, gid and groups from user
		uid, gid, addGroups, err1 := getUserInfo(rootfs, containerUser)
		if err1 != nil {
			return err1
		}

		logrus.Debugf("UID: %v, GID: %v, Groups: %+v", uid, gid, addGroups)
		specgen.SetProcessUID(uid)
		specgen.SetProcessGID(gid)
		for _, group := range addGroups {
			specgen.AddProcessAdditionalGid(group)
		}

		// Add groups from CRI
		groups := sc.GetSupplementalGroups()
		for _, group := range groups {
			specgen.AddProcessAdditionalGid(uint32(group))
		}
	}
	return nil
}

// setupCapabilities sets process.capabilities in the OCI runtime config.
func setupCapabilities(specgen *generate.Generator, capabilities *pb.Capability) error {
	// Remove all ambient capabilities. Kubernetes is not yet ambient capabilities aware
	// and pods expect that switching to a non-root user results in the capabilities being
	// dropped. This should be revisited in the future.
	specgen.Spec().Process.Capabilities.Ambient = []string{}

	if capabilities == nil {
		return nil
	}

	toCAPPrefixed := func(cap string) string {
		if !strings.HasPrefix(strings.ToLower(cap), "cap_") {
			return "CAP_" + strings.ToUpper(cap)
		}
		return cap
	}

	// Add/drop all capabilities if "all" is specified, so that
	// following individual add/drop could still work. E.g.
	// AddCapabilities: []string{"ALL"}, DropCapabilities: []string{"CHOWN"}
	// will be all capabilities without `CAP_CHOWN`.
	// see https://github.com/kubernetes/kubernetes/issues/51980
	if inStringSlice(capabilities.GetAddCapabilities(), "ALL") {
		for _, c := range getOCICapabilitiesList() {
			if err := specgen.AddProcessCapabilityBounding(c); err != nil {
				return err
			}
			if err := specgen.AddProcessCapabilityEffective(c); err != nil {
				return err
			}
			if err := specgen.AddProcessCapabilityInheritable(c); err != nil {
				return err
			}
			if err := specgen.AddProcessCapabilityPermitted(c); err != nil {
				return err
			}
		}
	}
	if inStringSlice(capabilities.GetDropCapabilities(), "ALL") {
		for _, c := range getOCICapabilitiesList() {
			if err := specgen.DropProcessCapabilityBounding(c); err != nil {
				return err
			}
			if err := specgen.DropProcessCapabilityEffective(c); err != nil {
				return err
			}
			if err := specgen.DropProcessCapabilityInheritable(c); err != nil {
				return err
			}
			if err := specgen.DropProcessCapabilityPermitted(c); err != nil {
				return err
			}
		}
	}

	for _, cap := range capabilities.GetAddCapabilities() {
		if strings.ToUpper(cap) == "ALL" {
			continue
		}
		capPrefixed := toCAPPrefixed(cap)
		// Validate capability
		if !inStringSlice(getOCICapabilitiesList(), capPrefixed) {
			// invalid capability, logging and moving on to next capability in list.
			logrus.Warnf("invalid capability %q, skipping...", cap)
			continue
		}
		if err := specgen.AddProcessCapabilityBounding(capPrefixed); err != nil {
			return err
		}
		if err := specgen.AddProcessCapabilityEffective(capPrefixed); err != nil {
			return err
		}
		if err := specgen.AddProcessCapabilityInheritable(capPrefixed); err != nil {
			return err
		}
		if err := specgen.AddProcessCapabilityPermitted(capPrefixed); err != nil {
			return err
		}
	}

	for _, cap := range capabilities.GetDropCapabilities() {
		if strings.ToUpper(cap) == "ALL" {
			continue
		}
		capPrefixed := toCAPPrefixed(cap)
		if err := specgen.DropProcessCapabilityBounding(capPrefixed); err != nil {
			return fmt.Errorf("failed to drop cap %s %v", capPrefixed, err)
		}
		if err := specgen.DropProcessCapabilityEffective(capPrefixed); err != nil {
			return fmt.Errorf("failed to drop cap %s %v", capPrefixed, err)
		}
		if err := specgen.DropProcessCapabilityInheritable(capPrefixed); err != nil {
			return fmt.Errorf("failed to drop cap %s %v", capPrefixed, err)
		}
		if err := specgen.DropProcessCapabilityPermitted(capPrefixed); err != nil {
			return fmt.Errorf("failed to drop cap %s %v", capPrefixed, err)
		}
	}

	return nil
}

func hostNetwork(containerConfig *pb.ContainerConfig) bool {
	securityContext := containerConfig.GetLinux().GetSecurityContext()
	if securityContext == nil || securityContext.GetNamespaceOptions() == nil {
		return false
	}

	return securityContext.GetNamespaceOptions().GetNetwork() == pb.NamespaceMode_NODE
}

// ensureSaneLogPath is a hack to fix https://issues.k8s.io/44043 which causes
// logPath to be a broken symlink to some magical Docker path. Ideally we
// wouldn't have to deal with this, but until that issue is fixed we have to
// remove the path if it's a broken symlink.
func ensureSaneLogPath(logPath string) error {
	// If the path exists but the resolved path does not, then we have a broken
	// symlink and we need to remove it.
	fi, err := os.Lstat(logPath)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		// Non-existent files and non-symlinks aren't our problem.
		return nil
	}

	_, err = os.Stat(logPath)
	if os.IsNotExist(err) {
		err = os.RemoveAll(logPath)
		if err != nil {
			return fmt.Errorf("ensureSaneLogPath remove bad logPath: %s", err)
		}
	}
	return nil
}

// addSecretsBindMounts mounts user defined secrets to the container
func addSecretsBindMounts(mountLabel, ctrRunDir string, defaultMounts []string, specgen generate.Generator) ([]rspec.Mount, error) {
	containerMounts := specgen.Spec().Mounts
	mounts, err := secretMounts(defaultMounts, mountLabel, ctrRunDir, containerMounts)
	if err != nil {
		return nil, err
	}
	return mounts, nil
}

// CreateContainer creates a new container in specified PodSandbox
func (s *Server) CreateContainer(ctx context.Context, req *pb.CreateContainerRequest) (res *pb.CreateContainerResponse, err error) {
	const operation = "create_container"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()
	logrus.Debugf("CreateContainerRequest %+v", req)

	s.updateLock.RLock()
	defer s.updateLock.RUnlock()

	sbID := req.PodSandboxId
	if sbID == "" {
		return nil, fmt.Errorf("PodSandboxId should not be empty")
	}

	sandboxID, err := s.PodIDIndex().Get(sbID)
	if err != nil {
		return nil, fmt.Errorf("PodSandbox with ID starting with %s not found: %v", sbID, err)
	}

	sb := s.getSandbox(sandboxID)
	if sb == nil {
		return nil, fmt.Errorf("specified sandbox not found: %s", sandboxID)
	}

	// The config of the container
	containerConfig := req.GetConfig()
	if containerConfig == nil {
		return nil, fmt.Errorf("CreateContainerRequest.ContainerConfig is nil")
	}

	if containerConfig.GetMetadata() == nil {
		return nil, fmt.Errorf("CreateContainerRequest.ContainerConfig.Metadata is nil")
	}

	name := containerConfig.GetMetadata().GetName()
	if name == "" {
		return nil, fmt.Errorf("CreateContainerRequest.ContainerConfig.Name is empty")
	}

	containerID, containerName, err := s.generateContainerIDandName(sb.Metadata(), containerConfig)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			s.ReleaseContainerName(containerName)
		}
	}()

	container, err := s.createSandboxContainer(ctx, containerID, containerName, sb, req.GetSandboxConfig(), containerConfig)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			err2 := s.StorageRuntimeServer().DeleteContainer(containerID)
			if err2 != nil {
				logrus.Warnf("Failed to cleanup container directory: %v", err2)
			}
		}
	}()

	s.addContainer(container)
	defer func() {
		if err != nil {
			s.removeContainer(container)
		}
	}()

	if err = s.CtrIDIndex().Add(containerID); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			if err2 := s.CtrIDIndex().Delete(containerID); err2 != nil {
				logrus.Warnf("couldn't delete ctr id %s from idIndex", containerID)
			}
		}
	}()

	if err = s.createContainerPlatform(container, sb.InfraContainer(), sb.CgroupParent()); err != nil {
		return nil, err
	}

	s.ContainerStateToDisk(container)

	resp := &pb.CreateContainerResponse{
		ContainerId: containerID,
	}

	logrus.Debugf("CreateContainerResponse: %+v", resp)
	return resp, nil
}

func (s *Server) setupOCIHooks(specgen *generate.Generator, sb *sandbox.Sandbox, containerConfig *pb.ContainerConfig, command string) error {
	mounts := containerConfig.GetMounts()
	addedHooks := map[string]struct{}{}
	addHook := func(hook lib.HookParams) error {
		// Only add a hook once
		if _, ok := addedHooks[hook.Hook]; !ok {
			if err := addOCIHook(specgen, hook); err != nil {
				return err
			}
			addedHooks[hook.Hook] = struct{}{}
		}
		return nil
	}
	for _, hook := range s.Hooks() {
		logrus.Debugf("SetupOCIHooks: %s", hook.Hook)
		if hook.HasBindMounts && len(mounts) > 0 {
			if err := addHook(hook); err != nil {
				return err
			}
			continue
		}
		for _, cmd := range hook.Cmds {
			match, err := regexp.MatchString(cmd, command)
			if err != nil {
				logrus.Errorf("Invalid regex %q:%q", cmd, err)
				continue
			}
			if match {
				if err := addHook(hook); err != nil {
					return err
				}
			}
		}
		for _, annotationRegex := range hook.Annotations {
			for _, annotation := range containerConfig.GetAnnotations() {
				match, err := regexp.MatchString(annotationRegex, annotation)
				if err != nil {
					logrus.Errorf("Invalid regex %q:%q", annotationRegex, err)
					continue
				}
				if match {
					if err := addHook(hook); err != nil {
						return err
					}
				}
			}
			for _, annotation := range sb.Annotations() {
				match, err := regexp.MatchString(annotationRegex, annotation)
				if err != nil {
					logrus.Errorf("Invalid regex %q:%q", annotationRegex, err)
					continue
				}
				if match {
					if err := addHook(hook); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}
func isInCRIMounts(dst string, mounts []*pb.Mount) bool {
	for _, m := range mounts {
		if m.ContainerPath == dst {
			return true
		}
	}
	return false
}

func (s *Server) setupSeccomp(specgen *generate.Generator, profile string) error {
	if profile == "" {
		// running w/o seccomp, aka unconfined
		specgen.Spec().Linux.Seccomp = nil
		return nil
	}
	if !s.seccompEnabled {
		if profile != seccompUnconfined {
			return fmt.Errorf("seccomp is not enabled in your kernel, cannot run with a profile")
		}
		logrus.Warn("seccomp is not enabled in your kernel, running container without profile")
	}
	if profile == seccompUnconfined {
		// running w/o seccomp, aka unconfined
		specgen.Spec().Linux.Seccomp = nil
		return nil
	}
	if profile == seccompRuntimeDefault || profile == seccompDockerDefault {
		return seccomp.LoadProfileFromStruct(s.seccompProfile, specgen)
	}
	if !strings.HasPrefix(profile, seccompLocalhostPrefix) {
		return fmt.Errorf("unknown seccomp profile option: %q", profile)
	}
	fname := strings.TrimPrefix(profile, "localhost/")
	file, err := ioutil.ReadFile(filepath.FromSlash(fname))
	if err != nil {
		return fmt.Errorf("cannot load seccomp profile %q: %v", fname, err)
	}
	return seccomp.LoadProfileFromBytes(file, specgen)
}

// getAppArmorProfileName gets the profile name for the given container.
func (s *Server) getAppArmorProfileName(profile string) string {
	if profile == "" {
		return ""
	}

	if profile == apparmor.ProfileRuntimeDefault {
		// If the value is runtime/default, then return default profile.
		return s.appArmorProfile
	}

	return strings.TrimPrefix(profile, apparmor.ProfileNamePrefix)
}

// openContainerFile opens a file inside a container rootfs safely
func openContainerFile(rootfs string, path string) (io.ReadCloser, error) {
	fp, err := symlink.FollowSymlinkInScope(filepath.Join(rootfs, path), rootfs)
	if err != nil {
		return nil, err
	}
	return os.Open(fp)
}

// getUserInfo returns UID, GID and additional groups for specified user
// by looking them up in /etc/passwd and /etc/group
func getUserInfo(rootfs string, userName string) (uint32, uint32, []uint32, error) {
	// We don't care if we can't open the file because
	// not all images will have these files
	passwdFile, err := openContainerFile(rootfs, "/etc/passwd")
	if err != nil {
		logrus.Warnf("Failed to open /etc/passwd: %v", err)
	} else {
		defer passwdFile.Close()
	}

	groupFile, err := openContainerFile(rootfs, "/etc/group")
	if err != nil {
		logrus.Warnf("Failed to open /etc/group: %v", err)
	} else {
		defer groupFile.Close()
	}

	execUser, err := user.GetExecUser(userName, nil, passwdFile, groupFile)
	if err != nil {
		return 0, 0, nil, err
	}

	uid := uint32(execUser.Uid)
	gid := uint32(execUser.Gid)
	var additionalGids []uint32
	for _, g := range execUser.Sgids {
		additionalGids = append(additionalGids, uint32(g))
	}

	return uid, gid, additionalGids, nil
}
