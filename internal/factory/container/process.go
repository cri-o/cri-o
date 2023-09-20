package container

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/storage"
	crioann "github.com/cri-o/cri-o/pkg/annotations"
	"github.com/cri-o/cri-o/pkg/config"
	sconfig "github.com/cri-o/cri-o/pkg/config"
	securejoin "github.com/cyphar/filepath-securejoin"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"

)

func (ctr *container) SetContainerWorkingDir(containerImageConfig *v1.Image, containerConfig *types.ContainerConfig) (containerCwd string, err error){
	// Set working directory
	// Pick it up from image config first and override if specified in CRI
	containerCwd = "/"
	imageCwd := containerImageConfig.Config.WorkingDir
	if imageCwd != "" {
		containerCwd = imageCwd
	}
	runtimeCwd := containerConfig.WorkingDir
	if runtimeCwd != "" {
		containerCwd = runtimeCwd
	}

	return containerCwd, err
}

func (ctr *container) SetupProcess(ctx context.Context, serverConfig *sconfig.Config, sb *sandbox.Sandbox, containerInfo storage.ContainerInfo)(){
	containerID := ctr.ID()
	containerConfig := ctr.Config()
	specgen := ctr.Spec()
	specgen.HostSpecific = true
	specgen.ClearProcessRlimits()

	for _, u := range serverConfig.Ulimits() {
		specgen.AddProcessRlimits(u.Name, u.Hard, u.Soft)
	}

	if containerConfig.Linux == nil {
		containerConfig.Linux = &types.LinuxContainerConfig{}
	}
	if containerConfig.Linux.SecurityContext == nil {
		containerConfig.Linux.SecurityContext = newLinuxContainerSecurityContext()
	}
	securityContext := containerConfig.Linux.SecurityContext
	// set this container's apparmor profile if it is set by sandbox
	if serverConfig.AppArmor().IsEnabled() && !ctr.Privileged() {
		profile, err := serverConfig.AppArmor().Apply(
			securityContext.ApparmorProfile,
		)
		if err != nil {
			return nil, fmt.Errorf("applying apparmor profile to container %s: %w", containerID, err)
		}

		log.Debugf(ctx, "Applied AppArmor profile %s to container %s", profile, containerID)
		specgen.SetProcessApparmorProfile(profile)
	}

	specgen.SetProcessTerminal(containerConfig.Tty)
	if containerConfig.Tty {
		specgen.AddProcessEnv("TERM", "xterm")
	}

	resources := containerConfig.Linux.Resources
	specgen.SetProcessOOMScoreAdj(int(resources.OomScoreAdj))

	specgen.SetProcessNoNewPrivileges(securityContext.NoNewPrivs)

	// Set hostname and add env for hostname
	specgen.SetHostname(sb.Hostname())
	specgen.AddProcessEnv("HOSTNAME", sb.Hostname())

	// First add any configured environment variables from crio config.
	// They will get overridden if specified in the image or container config.
	specgen.AddMultipleProcessEnv(serverConfig.DefaultEnv)

	containerImageConfig := containerInfo.Config


	// Add environment variables from image the CRI configuration
	envs := mergeEnvs(containerImageConfig, containerConfig.Envs)
	for _, e := range envs {
		parts := strings.SplitN(e, "=", 2)
		specgen.AddProcessEnv(parts[0], parts[1])
	}

	containerCwd, err := ctr.SetContainerWorkingDir(containerImageConfig, containerConfig)
	if err != nil {
		return nil, err;
	} 

	specgen.SetProcessCwd(containerCwd)
	if err := setupWorkingDirectory(mountPoint, mountLabel, containerCwd); err != nil {
		return nil, err
	}

	if v := sb.Annotations()[crioann.UmaskAnnotation]; v != "" {
		umaskRegexp := regexp.MustCompile(`^[0-7]{1,4}$`)
		if !umaskRegexp.MatchString(v) {
			return nil, fmt.Errorf("invalid umask string %s", v)
		}
		decVal, err := strconv.ParseUint(sb.Annotations()[crioann.UmaskAnnotation], 8, 32)
		if err != nil {
			return nil, err
		}
		umask := uint32(decVal)
		specgen.Config.Process.User.Umask = &umask
	}
	

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

func newLinuxContainerSecurityContext() *types.LinuxContainerSecurityContext {
	return &types.LinuxContainerSecurityContext{
		Capabilities:     &types.Capability{},
		NamespaceOptions: &types.NamespaceOption{},
		SelinuxOptions:   &types.SELinuxOption{},
		RunAsUser:        &types.Int64Value{},
		RunAsGroup:       &types.Int64Value{},
		Seccomp:          &types.SecurityProfile{},
		Apparmor:         &types.SecurityProfile{},
	}
}

func mergeEnvs(imageConfig *v1.Image, kubeEnvs []*types.KeyValue) []string {
	envs := []string{}
	if kubeEnvs == nil && imageConfig != nil {
		envs = imageConfig.Config.Env
	} else {
		for _, item := range kubeEnvs {
			if item.Key == "" {
				continue
			}
			envs = append(envs, item.Key+"="+item.Value)
		}
		if imageConfig != nil {
			for _, imageEnv := range imageConfig.Config.Env {
				var found bool
				parts := strings.SplitN(imageEnv, "=", 2)
				if len(parts) != 2 {
					continue
				}
				imageEnvKey := parts[0]
				if imageEnvKey == "" {
					continue
				}
				for _, kubeEnv := range envs {
					kubeEnvKey := strings.SplitN(kubeEnv, "=", 2)[0]
					if kubeEnvKey == "" {
						continue
					}
					if imageEnvKey == kubeEnvKey {
						found = true
						break
					}
				}
				if !found {
					envs = append(envs, imageEnv)
				}
			}
		}
	}
	return envs
}
