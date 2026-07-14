package lib

import (
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"go.podman.io/storage/pkg/idtools"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ConvertOCIToStorageIDMappings converts OCI runtime spec ID mappings to storage ID mappings.
func ConvertOCIToStorageIDMappings(uidMappings, gidMappings []rspec.LinuxIDMapping) *idtools.IDMappings {
	if len(uidMappings) == 0 || len(gidMappings) == 0 {
		return nil
	}

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

// ConvertCRIToOCIMappings converts CRI ID mappings to OCI runtime spec ID mappings.
func ConvertCRIToOCIMappings(m []*types.IDMapping) []rspec.LinuxIDMapping {
	if len(m) == 0 {
		return nil
	}

	ids := make([]rspec.LinuxIDMapping, 0, len(m))
	for _, mapping := range m {
		ids = append(ids, rspec.LinuxIDMapping{
			ContainerID: mapping.GetContainerId(),
			HostID:      mapping.GetHostId(),
			Size:        mapping.GetLength(),
		})
	}

	return ids
}
