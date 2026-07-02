package lib

import (
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"go.podman.io/storage/pkg/idtools"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ConvertOCIToStorageIDMappings converts OCI runtime spec ID mappings to storage ID mappings.
func ConvertOCIToStorageIDMappings(uidMappings, gidMappings []rspec.LinuxIDMapping) *idtools.IDMappings {
	// FreeBSD doesn't support user namespaces, so this is a no-op
	return nil
}

// ConvertCRIToOCIMappings converts CRI ID mappings to OCI runtime spec ID mappings.
func ConvertCRIToOCIMappings(m []*types.IDMapping) []rspec.LinuxIDMapping {
	// FreeBSD doesn't support user namespaces, so this is a no-op
	return nil
}
