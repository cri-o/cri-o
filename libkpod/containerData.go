package libkpod

import (
	"encoding/json"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/fields"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"

	"github.com/containers/storage"
	"github.com/kubernetes-incubator/cri-o/libkpod/common"
	"github.com/kubernetes-incubator/cri-o/libkpod/driver"
	libkpodimage "github.com/kubernetes-incubator/cri-o/libkpod/image"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/kubernetes-incubator/cri-o/pkg/annotations"
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
func GetContainerData(store storage.Store, name string, size bool) (*ContainerData, error) {
	ctr, err := inspectContainer(store, name)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading build container %q", name)
	}
	container, err := store.Container(name)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading container data")
	}

	// The runtime configuration won't exist if the container has never been started by cri-o or kpod,
	// so treat a not-exist error as non-fatal.
	m := getBlankSpec()
	config, err := store.FromContainerDirectory(ctr.ID(), "config.json")
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
	imageData, err := libkpodimage.GetImageData(store, container.ImageID)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading container image data")
	}

	driverName, err := driver.GetDriverName(store)
	if err != nil {
		return nil, err
	}
	topLayer, err := GetContainerTopLayerID(store, ctr.ID())
	if err != nil {
		return nil, err
	}
	layer, err := store.Layer(topLayer)
	if err != nil {
		return nil, err
	}
	driverMetadata, err := driver.GetDriverMetadata(store, topLayer)
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
		sizeRootFs, err := GetContainerRootFsSize(store, data.ID)
		if err != nil {

			return nil, errors.Wrapf(err, "error reading size for container %q", name)
		}
		data.SizeRootFs = uint(sizeRootFs)
		sizeRw, err := GetContainerRwSize(store, data.ID)
		if err != nil {
			return nil, errors.Wrapf(err, "error reading RWSize for container %q", name)
		}
		data.SizeRw = uint(sizeRw)
	}

	return data, nil
}

// Get an oci.Container and update its status
func inspectContainer(store storage.Store, container string) (*oci.Container, error) {
	ociCtr, err := getOCIContainer(store, container)
	if err != nil {
		return nil, err
	}
	// call oci.New() to get the runtime
	runtime, err := getOCIRuntime(store, container)
	if err != nil {
		return nil, err
	}
	// call runtime.UpdateStatus()
	err = runtime.UpdateStatus(ociCtr)
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

// get an oci.Container instance for a given container ID
func getOCIContainer(store storage.Store, container string) (*oci.Container, error) {
	ctr, err := FindContainer(store, container)
	if err != nil {
		return nil, err
	}

	// The runtime configuration won't exist if the container has never been started by cri-o or kpod,
	// so treat a not-exist error as non-fatal.
	m := getBlankSpec()
	config, err := store.FromContainerDirectory(ctr.ID, "config.json")
	if err != nil && !os.IsNotExist(errors.Cause(err)) {
		return nil, err
	}
	if len(config) > 0 {
		if err = json.Unmarshal(config, &m); err != nil {
			return nil, err
		}
	}

	labels := make(map[string]string)
	err = json.Unmarshal([]byte(m.Annotations[annotations.Labels]), &labels)
	if len(m.Annotations[annotations.Labels]) > 0 && err != nil {
		return nil, err
	}
	name := ctr.Names[0]

	var metadata pb.ContainerMetadata
	err = json.Unmarshal([]byte(m.Annotations[annotations.Metadata]), &metadata)
	if len(m.Annotations[annotations.Metadata]) > 0 && err != nil {
		return nil, err
	}

	tty := common.IsTrue(m.Annotations[annotations.TTY])
	stdin := common.IsTrue(m.Annotations[annotations.Stdin])
	stdinOnce := common.IsTrue(m.Annotations[annotations.StdinOnce])

	containerPath, err := store.ContainerRunDirectory(ctr.ID)
	if err != nil {
		return nil, err
	}

	containerDir, err := store.ContainerDirectory(ctr.ID)
	if err != nil {
		return nil, err
	}

	img, _ := m.Annotations[annotations.Image]

	kubeAnnotations := make(map[string]string)
	err = json.Unmarshal([]byte(m.Annotations[annotations.Annotations]), &kubeAnnotations)
	if len(m.Annotations[annotations.Annotations]) > 0 && err != nil {
		return nil, err
	}

	created := time.Time{}
	if len(m.Annotations[annotations.Created]) > 0 {
		created, err = time.Parse(time.RFC3339Nano, m.Annotations[annotations.Created])
		if err != nil {
			return nil, err
		}
	}

	// create a new OCI Container.  kpod currently doesn't deal with pod sandboxes, so the fields for netns, privileged, and trusted are left empty
	return oci.NewContainer(ctr.ID, name, containerPath, m.Annotations[annotations.LogPath], nil, labels, kubeAnnotations, img, &metadata, ctr.ImageID, tty, stdin, stdinOnce, false, false, containerDir, created, m.Annotations["org.opencontainers.image.stopSignal"])
}

func getOCIRuntime(store storage.Store, container string) (*oci.Runtime, error) {
	// TODO: Move server default config out of server so that it can be used instead of this
	return oci.New("/usr/bin/runc", "", "runtime", "/usr/local/libexec/crio/conmon", []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}, "cgroupfs")
}
