package oci_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"time"

	"github.com/containers/storage/pkg/idtools"
	"github.com/cri-o/cri-o/internal/oci"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runtime-spec/specs-go"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// The actual test suite
var _ = t.Describe("Container", func() {
	// The system under test
	var sut *oci.Container

	// Setup the test
	BeforeEach(func() {
		sut = getTestContainer()
	})

	It("should succeed to get the container fields", func() {
		// Given
		// When
		// Then
		Expect(sut.ID()).To(Equal("id"))
		Expect(sut.Name()).To(Equal("name"))
		Expect(sut.BundlePath()).To(Equal("bundlePath"))
		Expect(sut.LogPath()).To(Equal("logPath"))
		Expect(len(sut.Labels())).To(BeEquivalentTo(1))
		Expect(len(sut.Annotations())).To(BeEquivalentTo(1))
		Expect(len(sut.CrioAnnotations())).To(BeEquivalentTo(1))
		Expect(sut.Image()).To(Equal("image"))
		Expect(sut.ImageName()).To(Equal("imageName"))
		Expect(sut.ImageRef()).To(Equal("imageRef"))
		Expect(sut.Sandbox()).To(Equal("sandbox"))
		Expect(sut.Dir()).To(Equal("dir"))
		Expect(sut.NetNsPath()).To(Equal("netns"))
		Expect(sut.StatePath()).To(Equal("dir/state.json"))
		Expect(sut.Metadata()).To(Equal(&pb.ContainerMetadata{}))
		Expect(sut.StateNoLock().Version).To(BeEmpty())
		Expect(sut.GetStopSignal()).To(Equal("15"))
		Expect(sut.CreatedAt().UnixNano()).
			To(BeNumerically("<", time.Now().UnixNano()))
	})

	It("should succeed to set the spec", func() {
		// Given
		newSpec := specs.Spec{Version: "version"}

		// When
		sut.SetSpec(&newSpec)

		// Then
		Expect(sut.Spec()).To(Equal(newSpec))
	})

	It("should succeed to set created", func() {
		// Given
		Expect(sut.Created()).To(BeFalse())

		// When
		sut.SetCreated()

		// Then
		Expect(sut.Created()).To(BeTrue())
	})

	It("should succeed to set ID mappings", func() {
		// Given
		mappings := &idtools.IDMappings{}

		// When
		sut.SetIDMappings(mappings)

		// Then
		Expect(sut.IDMappings()).To(Equal(mappings))
	})

	It("should succeed to add a volume", func() {
		// Given
		volume := oci.ContainerVolume{ContainerPath: "/"}

		// When
		sut.AddVolume(volume)

		// Then
		Expect(len(sut.Volumes())).To(BeEquivalentTo(1))
		Expect(sut.Volumes()[0]).To(Equal(volume))
	})

	It("should succeed to set the seccomp profile path", func() {
		// Given
		path := "path"

		// When
		sut.SetSeccompProfilePath(path)

		// Then
		Expect(sut.SeccompProfilePath()).To(Equal(path))
	})

	It("should succeed to set the mount point", func() {
		// Given
		mp := "mountPoint"

		// When
		sut.SetMountPoint(mp)

		// Then
		Expect(sut.MountPoint()).To(Equal(mp))
	})

	It("should succeed to set start failed", func() {
		// Given
		err := fmt.Errorf("error")

		// When
		sut.SetStartFailed(err)

		// Then
		Expect(sut.State().Error).To(Equal(err.Error()))
	})

	It("should succeed to set start failed with nil error", func() {
		// Given
		// When
		sut.SetStartFailed(nil)

		// Then
		Expect(sut.State().Error).To(BeEmpty())
	})

	It("should succeed get the default stop signal on invalid", func() {
		// Given
		container, err := oci.NewContainer("", "", "", "", "",
			map[string]string{}, map[string]string{}, map[string]string{},
			"", "", "", &pb.ContainerMetadata{}, "",
			false, false, false, "", "", time.Now(), "SIGNO")
		Expect(err).To(BeNil())
		Expect(container).NotTo(BeNil())

		// When
		signal := container.GetStopSignal()

		// Then
		Expect(signal).To(Equal("15"))
	})

	It("should succeed get NetNsPath if not provided", func() {
		// Given
		container, err := oci.NewContainer("", "", "", "", "",
			map[string]string{}, map[string]string{}, map[string]string{},
			"", "", "", &pb.ContainerMetadata{}, "",
			false, false, false, "", "", time.Now(), "")
		Expect(err).To(BeNil())
		Expect(container).NotTo(BeNil())

		// When
		path, err := container.NetNsPath()

		// Then
		Expect(err).To(BeNil())
		Expect(path).To(Equal("/proc/0/ns/net"))
	})

	It("should fail get NetNsPath if container state nil", func() {
		// Given
		container, err := oci.NewContainer("", "", "", "", "",
			map[string]string{}, map[string]string{}, map[string]string{},
			"", "", "", &pb.ContainerMetadata{}, "",
			false, false, false, "", "", time.Now(), "")
		Expect(err).To(BeNil())
		Expect(container).NotTo(BeNil())
		container.SetState(nil)

		// When
		path, err := container.NetNsPath()

		// Then
		Expect(err).NotTo(BeNil())
		Expect(path).To(BeEmpty())
	})

	It("should succeed get the non default stop signal", func() {
		// Given
		container, err := oci.NewContainer("", "", "", "", "",
			map[string]string{}, map[string]string{}, map[string]string{},
			"", "", "", &pb.ContainerMetadata{}, "",
			false, false, false, "", "", time.Now(), "SIGTRAP")
		Expect(err).To(BeNil())
		Expect(container).NotTo(BeNil())

		// When
		signal := container.GetStopSignal()

		// Then
		Expect(signal).To(Equal("5"))
	})

	It("should succeed to get the state from disk", func() {
		// Given
		Expect(os.MkdirAll(sut.Dir(), 0755)).To(BeNil())
		Expect(ioutil.WriteFile(path.Join(sut.Dir(), "state.json"),
			[]byte("{}"), 0644)).To(BeNil())
		defer os.RemoveAll(sut.Dir())

		// When
		err := sut.FromDisk()

		// Then
		Expect(err).To(BeNil())
	})

	It("should fail to get the state from disk if invalid json", func() {
		// Given
		Expect(os.MkdirAll(sut.Dir(), 0755)).To(BeNil())
		Expect(ioutil.WriteFile(path.Join(sut.Dir(), "state.json"),
			[]byte("invalid"), 0644)).To(BeNil())
		defer os.RemoveAll(sut.Dir())

		// When
		err := sut.FromDisk()

		// Then
		Expect(err).NotTo(BeNil())
	})

	It("should fail to get the state from disk if not existing", func() {
		// Given
		// When
		err := sut.FromDisk()

		// Then
		Expect(err).NotTo(BeNil())
	})
})
