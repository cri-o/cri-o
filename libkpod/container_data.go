package libkpod

import (
	"encoding/json"
	"os"

	"k8s.io/apimachinery/pkg/fields"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"

	"github.com/kubernetes-incubator/cri-o/libkpod/driver"
	libkpodimage "github.com/kubernetes-incubator/cri-o/libkpod/image"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/opencontainers/image-spec/specs-go/v1"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

// ContainerData handles the data used when inspecting a container
type ContainerData struct {
	ID               string
	Name             string
	LogPath          string
	Labels           fields.Set
	Annotations      fields.Set
	State            *oci.ContainerState
	Metadata         *pb.ContainerMetadata
	BundlePath       string
	StopSignal       string
	FromImage        string `json:"Image,omitempty"`
	FromImageID      string `json:"ImageID"`
	MountPoint       string `json:"Mountpoint,omitempty"`
	MountLabel       string
	Mounts           []specs.Mount
	AppArmorProfile  string
	ImageAnnotations map[string]string `json:"Annotations,omitempty"`
	ImageCreatedBy   string            `json:"CreatedBy,omitempty"`
	Config           v1.ImageConfig    `json:"Config,omitempty"`
	SizeRw           uint              `json:"SizeRw,omitempty"`
	SizeRootFs       uint              `json:"SizeRootFs,omitempty"`
	Args             []string
	ResolvConfPath   string
	HostnamePath     string
	HostsPath        string
	GraphDriver      driverData
}

type driverData struct {
	Name string
	Data map[string]string
}

// GetContainerData gets the ContainerData for a container with the given name in the given store.
// If size is set to true, it will also determine the size of the container
func (c *ContainerServer) GetContainerData(name string, size bool) (*ContainerData, error) {
	ctr, err := c.inspectContainer(name)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading build container %q", name)
	}
	container, err := c.store.Container(name)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading container data")
	}

	// The runtime configuration won't exist if the container has never been started by cri-o or kpod,
	// so treat a not-exist error as non-fatal.
	m := getBlankSpec()
	config, err := c.store.FromContainerDirectory(ctr.ID(), "config.json")
	if err != nil && !os.IsNotExist(errors.Cause(err)) {
		return nil, err
	}
	if len(config) > 0 {
		if err = json.Unmarshal(config, &m); err != nil {
			return nil, err
		}
	}

	if container.ImageID == "" {
		return nil, errors.Errorf("error reading container image data: container is not based on an image")
	}
	imageData, err := libkpodimage.GetImageData(c.store, container.ImageID)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading container image data")
	}

	driverName, err := driver.GetDriverName(c.store)
	if err != nil {
		return nil, err
	}
	topLayer, err := c.GetContainerTopLayerID(ctr.ID())
	if err != nil {
		return nil, err
	}
	layer, err := c.store.Layer(topLayer)
	if err != nil {
		return nil, err
	}
	driverMetadata, err := driver.GetDriverMetadata(c.store, topLayer)
	if err != nil {
		return nil, err
	}
	imageName := ""
	if len(imageData.Tags) > 0 {
		imageName = imageData.Tags[0]
	} else if len(imageData.Digests) > 0 {
		imageName = imageData.Digests[0]
	}
	data := &ContainerData{
		ID:               ctr.ID(),
		Name:             ctr.Name(),
		LogPath:          ctr.LogPath(),
		Labels:           ctr.Labels(),
		Annotations:      ctr.Annotations(),
		State:            ctr.State(),
		Metadata:         ctr.Metadata(),
		BundlePath:       ctr.BundlePath(),
		StopSignal:       ctr.GetStopSignal(),
		Args:             m.Process.Args,
		FromImage:        imageName,
		FromImageID:      container.ImageID,
		MountPoint:       layer.MountPoint,
		ImageAnnotations: imageData.Annotations,
		ImageCreatedBy:   imageData.CreatedBy,
		Config:           imageData.Config,
		GraphDriver: driverData{
			Name: driverName,
			Data: driverMetadata,
		},
		MountLabel:      m.Linux.MountLabel,
		Mounts:          m.Mounts,
		AppArmorProfile: m.Process.ApparmorProfile,
		ResolvConfPath:  "",
		HostnamePath:    "",
		HostsPath:       "",
	}

	if size {
		sizeRootFs, err := c.GetContainerRootFsSize(data.ID)
		if err != nil {

			return nil, errors.Wrapf(err, "error reading size for container %q", name)
		}
		data.SizeRootFs = uint(sizeRootFs)
		sizeRw, err := c.GetContainerRwSize(data.ID)
		if err != nil {
			return nil, errors.Wrapf(err, "error reading RWSize for container %q", name)
		}
		data.SizeRw = uint(sizeRw)
	}

	return data, nil
}

// Get an oci.Container and update its status
func (c *ContainerServer) inspectContainer(container string) (*oci.Container, error) {
	ociCtr, err := c.LookupContainer(container)
	if err != nil {
		return nil, err
	}
	// call runtime.UpdateStatus()
	err = c.Runtime().UpdateStatus(ociCtr)
	if err != nil {
		return nil, err
	}
	return ociCtr, nil
}

func getBlankSpec() specs.Spec {
	return specs.Spec{
		Process:     &specs.Process{},
		Root:        &specs.Root{},
		Mounts:      []specs.Mount{},
		Hooks:       &specs.Hooks{},
		Annotations: make(map[string]string),
		Linux:       &specs.Linux{},
		Solaris:     &specs.Solaris{},
		Windows:     &specs.Windows{},
	}
}
