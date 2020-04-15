package lib

import (
	"encoding/json"
	"path/filepath"

	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	"github.com/containers/libpod/pkg/annotations"
	"github.com/containers/storage/pkg/ioutils"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/opencontainers/runtime-tools/generate"
)

const configFile = "config.json"

// ContainerRename renames the given container
func (c *ContainerServer) ContainerRename(container, name string) (retErr error) {
	ctr, err := c.LookupContainer(container)
	if err != nil {
		return err
	}

	oldName := ctr.Name()
	_, err = c.ReserveContainerName(ctr.ID(), name)
	if err != nil {
		return err
	}
	defer func() {
		if retErr != nil {
			c.ReleaseContainerName(name)
		} else {
			c.ReleaseContainerName(oldName)
		}
	}()

	// Update state.json
	if err := c.updateStateName(ctr, name); err != nil {
		return err
	}

	// Update config.json
	configRuntimePath := filepath.Join(ctr.BundlePath(), configFile)
	if err := updateConfigName(configRuntimePath, name); err != nil {
		return err
	}
	configStoragePath := filepath.Join(ctr.Dir(), configFile)
	if err := updateConfigName(configStoragePath, name); err != nil {
		return err
	}

	// Update containers.json
	return c.store.SetNames(ctr.ID(), []string{name})
}

func updateConfigName(configPath, name string) error {
	specgen, err := generate.NewFromFile(configPath)
	if err != nil {
		return err
	}
	specgen.AddAnnotation(annotations.Name, name)
	specgen.AddAnnotation(annotations.Metadata, updateMetadata(specgen.Config.Annotations, name))

	return specgen.SaveToFile(configPath, generate.ExportOptions{})
}

func (c *ContainerServer) updateStateName(ctr *oci.Container, name string) error {
	if ctr != nil && ctr.State() != nil && ctr.State().Annotations != nil {
		ctr.State().Annotations[annotations.Name] = name
		ctr.State().Annotations[annotations.Metadata] = updateMetadata(ctr.State().Annotations, name)
	}
	// This is taken directly from c.ContainerStateToDisk(), which can't be used because of the call to UpdateStatus() in the first line
	jsonSource, err := ioutils.NewAtomicFileWriter(ctr.StatePath(), 0644)
	if err != nil {
		return err
	}
	defer jsonSource.Close()
	enc := json.NewEncoder(jsonSource)
	return enc.Encode(ctr.State())
}

// Attempts to update a metadata annotation
func updateMetadata(specAnnotations map[string]string, name string) string {
	oldMetadata := specAnnotations[annotations.Metadata]
	containerType := specAnnotations[annotations.ContainerType]
	switch containerType {
	case "container":
		metadata := runtime.ContainerMetadata{}
		err := json.Unmarshal([]byte(oldMetadata), &metadata)
		if err != nil {
			return oldMetadata
		}
		metadata.Name = name
		m, err := json.Marshal(metadata)
		if err != nil {
			return oldMetadata
		}
		return string(m)

	case "sandbox":
		metadata := runtime.PodSandboxMetadata{}
		err := json.Unmarshal([]byte(oldMetadata), &metadata)
		if err != nil {
			return oldMetadata
		}
		metadata.Name = name
		m, err := json.Marshal(metadata)
		if err != nil {
			return oldMetadata
		}
		return string(m)

	default:
		return specAnnotations[annotations.Metadata]
	}
}
