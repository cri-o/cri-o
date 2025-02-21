package container

import (
	"errors"
	"fmt"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/config/nsmgr"
	"github.com/cri-o/cri-o/internal/lib/namespace"
	oci "github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/pkg/config"
)

func (c *container) SpecAddNamespaces(sb SandboxIFace, targetCtr *oci.Container, serverConfig *config.Config) error {
	// Join the namespace paths for the pod sandbox container.
	if err := ConfigureGeneratorGivenNamespacePaths(sb.NamespacePaths(), &c.spec); err != nil {
		return fmt.Errorf("failed to configure namespaces in container create: %w", err)
	}

	sc := c.config.Linux.SecurityContext

	if sc.NamespaceOptions.Network == types.NamespaceMode_NODE {
		if err := c.spec.RemoveLinuxNamespace(string(rspec.NetworkNamespace)); err != nil {
			return err
		}
	}

	switch sc.NamespaceOptions.Pid {
	case types.NamespaceMode_NODE:
		// kubernetes PodSpec specify to use Host PID namespace
		if err := c.spec.RemoveLinuxNamespace(string(rspec.PIDNamespace)); err != nil {
			return err
		}
	case types.NamespaceMode_POD:
		pidNsPath := sb.PidNsPath()
		if pidNsPath == "" {
			if sb.NamespaceOptions().Pid != types.NamespaceMode_POD {
				return errors.New("pod level PID namespace requested for the container, but pod sandbox was not similarly configured, and does not have an infra container")
			}

			return errors.New("PID namespace requested, but sandbox infra container unexpectedly invalid")
		}

		if err := c.spec.AddOrReplaceLinuxNamespace(string(rspec.PIDNamespace), pidNsPath); err != nil {
			return fmt.Errorf("updating container PID namespace to pod: %w", err)
		}
	case types.NamespaceMode_TARGET:
		if targetCtr == nil {
			return errors.New("target PID namespace specified with invalid target ID")
		}

		targetPID, err := targetCtr.Pid()
		if err != nil {
			return fmt.Errorf("target PID namespace find PID: %w", err)
		}

		ns, err := serverConfig.NamespaceManager().NamespaceFromProcEntry(targetPID, nsmgr.PIDNS)
		if err != nil {
			return fmt.Errorf("target PID namespace from proc: %w", err)
		}

		if err := c.spec.AddOrReplaceLinuxNamespace(string(rspec.PIDNamespace), ns.Path()); err != nil {
			return fmt.Errorf("updating container PID namespace to target %s: %w", targetCtr.ID(), err)
		}

		c.pidns = ns
	}

	return nil
}

// ConfigureGeneratorGivenNamespacePaths takes a map of nsType -> nsPath. It configures the generator
// to add or replace the defaults to these paths.
func ConfigureGeneratorGivenNamespacePaths(managedNamespaces []*namespace.ManagedNamespace, g *generate.Generator) error {
	typeToSpec := map[nsmgr.NSType]rspec.LinuxNamespaceType{
		nsmgr.IPCNS:  rspec.IPCNamespace,
		nsmgr.NETNS:  rspec.NetworkNamespace,
		nsmgr.UTSNS:  rspec.UTSNamespace,
		nsmgr.USERNS: rspec.UserNamespace,
	}

	for _, ns := range managedNamespaces {
		// allow for empty paths, as this namespace just shouldn't be configured
		if ns.Path() == "" {
			continue
		}

		nsForSpec := typeToSpec[ns.Type()]
		if nsForSpec == "" {
			return fmt.Errorf("invalid namespace type %s", ns.Type())
		}

		if err := g.AddOrReplaceLinuxNamespace(string(nsForSpec), ns.Path()); err != nil {
			return err
		}
	}

	return nil
}
