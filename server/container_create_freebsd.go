package server

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	ctrfactory "github.com/cri-o/cri-o/internal/factory/container"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"golang.org/x/net/context"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const (
	bindMountType = "nullfs"
)

var (
	bindMountOptions = []string{}

	tmpfsMounts = map[string]string{
		"/tmp":     "mode=1777",
		"/var/run": "mode=0755",
		"/var/tmp": "mode=1777",
	}
)

func getSecurityContext(containerConfig *types.ContainerConfig) *types.LinuxContainerSecurityContext {
	return &types.LinuxContainerSecurityContext{}
}

func (s *Server) setAppArmorProfile(ctx context.Context, ctr ctrfactory.Container, securityContext *types.LinuxContainerSecurityContext, specgen *generate.Generator) error {
	return nil
}

func (s *Server) setSecurityContextNamespaceOptions(ctx context.Context, ctr ctrfactory.Container, containerConfig *types.ContainerConfig, sb *sandbox.Sandbox) error {
	return nil
}

func addSysfsMounts(ctr ctrfactory.Container, containerConfig *types.ContainerConfig, hostNet bool) {
}

func setOCIBindMountsPrivileged(g *generate.Generator) {
}

func addOCIBindMounts(ctx context.Context, ctr ctrfactory.Container, mountLabel, bindMountPrefix string, absentMountSourcesToReject []string, maybeRelabel, skipRelabel, cgroup2RW, idMapSupport bool, storageRoot string) ([]oci.ContainerVolume, []rspec.Mount, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	volumes := []oci.ContainerVolume{}
	ociMounts := []rspec.Mount{}
	containerConfig := ctr.Config()
	specgen := ctr.Spec()
	mounts := containerConfig.Mounts

	// Sort mounts in number of parts. This ensures that high level mounts don't
	// shadow other mounts.
	sort.Sort(criOrderedMounts(mounts))

	// Copy all mounts from default mounts, except for
	// - mounts overridden by supplied mount;
	// - all mounts under /dev if a supplied /dev is present.
	mountSet := make(map[string]struct{})
	for _, m := range mounts {
		mountSet[filepath.Clean(m.ContainerPath)] = struct{}{}
	}
	defaultMounts := specgen.Mounts()
	specgen.ClearMounts()
	for _, m := range defaultMounts {
		dst := filepath.Clean(m.Destination)
		if _, ok := mountSet[dst]; ok {
			// filter out mount overridden by a supplied mount
			continue
		}
		if _, mountDev := mountSet["/dev"]; mountDev && strings.HasPrefix(dst, "/dev/") {
			// filter out everything under /dev if /dev is a supplied mount
			continue
		}
		if _, mountSys := mountSet["/sys"]; mountSys && strings.HasPrefix(dst, "/sys/") {
			// filter out everything under /sys if /sys is a supplied mount
			continue
		}
		specgen.AddMount(m)
	}

	for _, m := range mounts {
		dest := m.ContainerPath
		if dest == "" {
			return nil, nil, fmt.Errorf("mount.ContainerPath is empty")
		}
		if m.HostPath == "" {
			return nil, nil, fmt.Errorf("mount.HostPath is empty")
		}
		if m.HostPath == "/" && dest == "/" {
			log.Warnf(ctx, "Configuration specifies mounting host root to the container root.  This is dangerous (especially with privileged containers) and should be avoided.")
		}
		src := filepath.Join(bindMountPrefix, m.HostPath)

		resolvedSrc, err := resolveSymbolicLink(bindMountPrefix, src)
		if err == nil {
			src = resolvedSrc
		} else {
			if !os.IsNotExist(err) {
				return nil, nil, fmt.Errorf("failed to resolve symlink %q: %w", src, err)
			}
			for _, toReject := range absentMountSourcesToReject {
				if filepath.Clean(src) == toReject {
					// special-case /etc/hostname, as we don't want it to be created as a directory
					// This can cause issues with node reboot.
					return nil, nil, fmt.Errorf("cannot mount %s: path does not exist and will cause issues as a directory", toReject)
				}
			}
			if !ctr.Restore() {
				// Although this would also be really helpful for restoring containers
				// it is problematic as during restore external bind mounts need to be
				// a file if the destination is a file. Unfortunately it is not easy
				// to tell if the destination is a file or a directory. Especially if
				// the destination is a nested bind mount. For now we will just not
				// create the missing bind mount source for restore and return an
				// error to the user.
				if err = os.MkdirAll(src, 0o755); err != nil {
					return nil, nil, fmt.Errorf("failed to mkdir %s: %s", src, err)
				}
			}
		}

		options := []string{"rw"}
		if m.Readonly {
			options = []string{"ro"}
		}

		volumes = append(volumes, oci.ContainerVolume{
			ContainerPath:  dest,
			HostPath:       src,
			Readonly:       m.Readonly,
			Propagation:    m.Propagation,
			SelinuxRelabel: m.SelinuxRelabel,
		})

		ociMounts = append(ociMounts, rspec.Mount{
			Source:      src,
			Destination: dest,
			Options:     options,
		})
	}

	return volumes, ociMounts, nil
}
