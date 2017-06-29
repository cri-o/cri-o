package main

import (
	"encoding/json"
	"time"

	"k8s.io/apimachinery/pkg/fields"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"

	"github.com/containers/storage"
	"github.com/kubernetes-incubator/cri-o/cmd/kpod/docker"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/kubernetes-incubator/cri-o/pkg/annotations"
	"github.com/kubernetes-incubator/cri-o/server"
	"github.com/opencontainers/image-spec/specs-go/v1"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

type containerData struct {
	ID               string
	Name             string
	LogPath          string
	Labels           fields.Set
	Annotations      fields.Set
	State            *oci.ContainerState
	Metadata         *pb.ContainerMetadata
	BundlePath       string
	StopSignal       string
	Type             string `json:"type"`
	FromImage        string `json:"image,omitempty"`
	FromImageID      string `json:"image-id"`
	MountPoint       string `json:"mountpoint,omitempty"`
	MountLabel       string
	Mounts           []specs.Mount
	AppArmorProfile  string
	ImageAnnotations map[string]string `json:"annotations,omitempty"`
	ImageCreatedBy   string            `json:"created-by,omitempty"`
	OCIv1            v1.Image          `json:"ociv1,omitempty"`
	Docker           docker.V2Image    `json:"docker,omitempty"`
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

func getContainerData(store storage.Store, name string, size bool) (*containerData, error) {
	ctr, err := inspectContainer(store, name)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading build container %q", name)
	}
	cid, err := openContainer(store, name)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading container image data")
	}
	config, err := store.FromContainerDirectory(ctr.ID(), "config.json")
	if err != nil {
		return nil, err
	}

	var m specs.Spec
	if err = json.Unmarshal(config, &m); err != nil {
		return nil, err
	}

	driverName, err := getDriverName(store)
	if err != nil {
		return nil, err
	}
	topLayer, err := getContainerTopLayerID(store, ctr.ID())
	if err != nil {
		return nil, err
	}
	driverMetadata, err := getDriverMetadata(store, topLayer)
	if err != nil {
		return nil, err
	}
	data := &containerData{
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
		Type:             cid.Type,
		FromImage:        cid.FromImage,
		FromImageID:      cid.FromImageID,
		MountPoint:       cid.MountPoint,
		ImageAnnotations: cid.ImageAnnotations,
		ImageCreatedBy:   cid.ImageCreatedBy,
		OCIv1:            cid.OCIv1,
		Docker:           cid.Docker,
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
		sizeRootFs, err := getRootFsSize(store, data.ID)
		if err != nil {

			return nil, errors.Wrapf(err, "error reading size for container %q", name)
		}
		data.SizeRootFs = uint(sizeRootFs)
		sizeRw, err := getContainerRwSize(store, data.ID)
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

// get an oci.Container instance for a given container ID
func getOCIContainer(store storage.Store, container string) (*oci.Container, error) {
	ctr, err := findContainer(store, container)
	if err != nil {
		return nil, err
	}
	config, err := store.FromContainerDirectory(ctr.ID, "config.json")
	if err != nil {
		return nil, err
	}

	var m specs.Spec
	if err = json.Unmarshal(config, &m); err != nil {
		return nil, err
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

	tty := isTrue(m.Annotations[annotations.TTY])
	stdin := isTrue(m.Annotations[annotations.Stdin])
	stdinOnce := isTrue(m.Annotations[annotations.StdinOnce])

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
	config := server.DefaultConfig()
	return oci.New(config.Runtime, config.RuntimeUntrustedWorkload, config.DefaultWorkloadTrust, config.Conmon, config.ConmonEnv, config.CgroupManager)
}
