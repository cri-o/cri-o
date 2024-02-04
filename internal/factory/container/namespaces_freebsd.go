package container

import (
	"github.com/cri-o/cri-o/internal/config/nsmgr"
	oci "github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/pkg/config"
)

func (c *container) SpecAddNamespaces(sb SandboxIFace, targetCtr *oci.Container, serverConfig *config.Config) error {
	// Join the namespace paths for the pod sandbox container.
	managedNamespaces := sb.NamespacePaths()

	for _, ns := range managedNamespaces {
		if ns.Type() == nsmgr.NETNS {
			c.Spec().AddAnnotation("org.freebsd.parentJail", ns.Path())
		}
	}

	return nil
}
