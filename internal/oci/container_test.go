package oci_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/containers/storage/pkg/idtools"
	"github.com/cri-o/cri-o/internal/findprocess"
	"github.com/cri-o/cri-o/internal/oci"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

const (
	neverRunningPid  = 4194305
	alwaysRunningPid = 1
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
		Expect(sut.StatePath()).To(Equal("dir/state.json"))
		Expect(sut.Metadata()).To(Equal(&pb.ContainerMetadata{}))
		Expect(sut.StateNoLock().Version).To(BeEmpty())
		Expect(sut.GetStopSignal()).To(Equal("15"))
		Expect(sut.CreatedAt().UnixNano()).
			To(BeNumerically("<", time.Now().UnixNano()))
		Expect(sut.Spoofed()).To(Equal(false))
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
		container, err := oci.NewContainer("", "", "", "",
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

	It("should succeed get the non default stop signal", func() {
		// Given
		container, err := oci.NewContainer("", "", "", "",
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

	t.Describe("FromDisk", func() {
		BeforeEach(func() {
			Expect(os.MkdirAll(sut.Dir(), 0o755)).To(BeNil())
		})
		AfterEach(func() {
			os.RemoveAll(sut.Dir())
		})
		It("should succeed to get the state from disk", func() {
			// Given
			Expect(ioutil.WriteFile(path.Join(sut.Dir(), "state.json"),
				[]byte("{}"), 0o644)).To(BeNil())

			// When
			err := sut.FromDisk()

			// Then
			Expect(err).To(BeNil())
		})
		It("should succeed when pid set but initialPid not set", func() {
			// Given
			Expect(ioutil.WriteFile(path.Join(sut.Dir(), "state.json"), []byte(`
			{"pid":`+strconv.Itoa(alwaysRunningPid)+`}`),
				0o644)).To(BeNil())

			// When
			err := sut.FromDisk()

			// Then
			Expect(err).To(BeNil())
			sutState := sut.State()
			Expect(sutState.InitStartTime).NotTo(Equal(0))
			Expect(sutState.InitPid).To(Equal(alwaysRunningPid))
		})
		It("should succeed when pid set but initialPid not set", func() {
			// Given
			Expect(ioutil.WriteFile(path.Join(sut.Dir(), "state.json"), []byte(`
			{"pid":`+strconv.Itoa(alwaysRunningPid)+`}`),
				0o644)).To(BeNil())

			// When
			err := sut.FromDisk()

			// Then
			Expect(err).To(BeNil())
			sutState := sut.State()
			Expect(sutState.InitStartTime).NotTo(Equal(0))
			Expect(sutState.InitPid).To(Equal(alwaysRunningPid))
		})
		It("should fail when pid set and not running", func() {
			// Given
			Expect(ioutil.WriteFile(path.Join(sut.Dir(), "state.json"), []byte(`
			{"pid":`+strconv.Itoa(neverRunningPid)+`}`),
				0o644)).To(BeNil())

			// When
			err := sut.FromDisk()

			// Then
			Expect(err).NotTo(BeNil())
			sutState := sut.State()
			Expect(sutState.InitStartTime).NotTo(Equal(0))
			Expect(sutState.InitPid).To(Equal(0))
		})

		It("should fail to get the state from disk if invalid json", func() {
			// Given
			Expect(ioutil.WriteFile(path.Join(sut.Dir(), "state.json"),
				[]byte("invalid"), 0o644)).To(BeNil())

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
	t.Describe("ShouldBeStopped", func() {
		It("should fail to stop if already stopped", func() {
			// Given
			state := &oci.ContainerState{}
			state.Status = oci.ContainerStateStopped
			sut.SetState(state)
			// When
			err := sut.ShouldBeStopped()

			// Then
			Expect(err).To(Equal(oci.ErrContainerStopped))
		})
		It("should fail to stop if paused", func() {
			// Given
			state := &oci.ContainerState{}
			state.Status = oci.ContainerStatePaused
			sut.SetState(state)
			// When
			err := sut.ShouldBeStopped()

			// Then
			Expect(err).NotTo(Equal(oci.ErrContainerStopped))
			Expect(err).NotTo(BeNil())
		})
		It("should succeed to stop if started", func() {
			// Given
			state := &oci.ContainerState{}
			state.Status = oci.ContainerStateRunning
			sut.SetState(state)
			// When
			err := sut.ShouldBeStopped()

			// Then
			Expect(err).To(BeNil())
		})
	})
	t.Describe("IsAlive", func() {
		It("should be false if pid unintialized", func() {
			// Given
			state := &oci.ContainerState{}
			state.Pid = 0
			sut.SetState(state)
			// When
			err := sut.IsAlive()

			// Then
			Expect(err).To(Equal(false))
		})
		It("should succeed if pid is running", func() {
			// Given
			state := &oci.ContainerState{}
			state.Pid = alwaysRunningPid
			Expect(state.SetInitPid(state.Pid)).To(BeNil())
			sut.SetState(state)
			// When
			err := sut.IsAlive()

			// Then
			Expect(err).To(Equal(true))
		})
		It("should be false if pid is not running", func() {
			// Given
			state := &oci.ContainerState{}
			state.Pid = neverRunningPid
			// SetInitPid will fail because the pid is not running
			Expect(state.SetInitPid(state.Pid)).NotTo(BeNil())
			sut.SetState(state)
			// When
			err := sut.IsAlive()

			// Then
			Expect(err).To(Equal(false))
		})
	})
	t.Describe("Pid", func() {
		It("should fail if container state not set", func() {
			// Given
			// When
			pid, err := sut.Pid()
			// Then
			Expect(pid).To(BeNumerically("<", 0))
			Expect(err).NotTo(BeNil())
		})
		It("should fail when pid is negative", func() {
			// Given
			state := &oci.ContainerState{}
			state.Pid = -1
			// SetInitPid will fail because the pid is not running
			Expect(state.SetInitPid(state.Pid)).NotTo(BeNil())
			sut.SetState(state)

			// When
			pid, err := sut.Pid()
			// Then
			Expect(pid).To(BeNumerically("<", 0))
			Expect(err).NotTo(BeNil())
		})
		It("should fail gracefully when pid has been stopped", func() {
			// Given
			state := &oci.ContainerState{}
			state.Pid = alwaysRunningPid
			Expect(state.SetInitPid(state.Pid)).To(BeNil())
			// a `runtime state ctr` call after the container has been stopped
			// will set the state pid to 0. However, InitPid never changes
			// so we have a separate handle for when Pid is 0 but InitPid is not
			state.Pid = 0
			sut.SetState(state)

			// When
			pid, err := sut.Pid()
			// Then
			Expect(pid).To(Equal(0))
			Expect(errors.Is(err, findprocess.ErrNotFound)).To(Equal(true))
		})
		It("should fail if process is not found", func() {
			// Given
			state := &oci.ContainerState{}
			state.Pid = neverRunningPid
			Expect(state.SetInitPid(state.Pid)).NotTo(BeNil())
			sut.SetState(state)

			// When
			pid, err := sut.Pid()
			// Then
			Expect(pid).To(BeNumerically("<", 0))
			Expect(errors.Is(err, findprocess.ErrNotFound)).To(Equal(true))
		})
		It("should fail when pid has wrapped", func() {
			// Given
			state := &oci.ContainerState{}
			state.Pid = alwaysRunningPid
			Expect(state.SetInitPid(state.Pid)).To(BeNil())
			// if InitStartTime != the time the state.InitPid started
			// pid wrap is assumed to have happened
			state.InitStartTime = 0
			sut.SetState(state)

			// When
			pid, err := sut.Pid()
			// Then
			Expect(pid).To(BeNumerically("<", 0))
			Expect(err).NotTo(BeNil())
		})
		It("should succeed", func() {
			// Given
			state := &oci.ContainerState{}
			state.Pid = alwaysRunningPid
			Expect(state.SetInitPid(state.Pid)).To(BeNil())
			sut.SetState(state)

			// When
			pid, err := sut.Pid()
			// Then
			Expect(pid).To(Equal(alwaysRunningPid))
			Expect(err).To(BeNil())
		})
	})
	t.Describe("findAndReleasePid", func() {
		It("should not be found nor fail if pid doesn't exist", func() {
			// Given
			state := &oci.ContainerState{}
			state.Pid = neverRunningPid
			Expect(state.SetInitPid(state.Pid)).NotTo(BeNil())
			sut.SetState(state)

			// When
			found, err := sut.FindAndReleasePid()
			// Then
			Expect(found).To(Equal(false))
			Expect(err).To(BeNil())
		})
		It("should not be found but should fail when pid wrap occurs", func() {
			// Given
			state := &oci.ContainerState{}
			state.Pid = alwaysRunningPid
			Expect(state.SetInitPid(state.Pid)).To(BeNil())
			// if InitStartTime != the time the state.InitPid started
			// pid wrap is assumed to have happened
			state.InitStartTime = 0
			sut.SetState(state)

			// When
			found, err := sut.FindAndReleasePid()
			// Then
			Expect(found).To(Equal(false))
			Expect(err).NotTo(BeNil())
		})
	})
	t.Describe("SetInitPid", func() {
		It("should suceeed if running", func() {
			// Given
			state := &oci.ContainerState{}
			// When
			state.Pid = alwaysRunningPid
			// Then
			Expect(state.SetInitPid(state.Pid)).To(BeNil())
		})
		It("should fail if already set", func() {
			// Given
			state := &oci.ContainerState{}
			state.Pid = alwaysRunningPid
			// When
			Expect(state.SetInitPid(state.Pid)).To(BeNil())
			// Then
			Expect(state.SetInitPid(state.Pid)).NotTo(BeNil())
		})
		It("should fail if not running", func() {
			// Given
			state := &oci.ContainerState{}
			// When
			state.Pid = neverRunningPid
			// Then
			Expect(state.SetInitPid(state.Pid)).NotTo(BeNil())
		})
	})
})

var _ = t.Describe("SpoofedContainer", func() {
	It("should succeed to get the container fields", func() {
		sut := oci.NewSpoofedContainer("id", "name", map[string]string{"key": "label"}, time.Now(), "dir")
		// Given
		// When
		// Then
		Expect(sut.ID()).To(Equal("id"))
		Expect(sut.Name()).To(Equal("name"))
		labels := sut.Labels()
		Expect(labels["key"]).To(Equal("label"))
		Expect(sut.Spoofed()).To(Equal(true))
		Expect(sut.CreatedAt().UnixNano()).
			To(BeNumerically("<", time.Now().UnixNano()))
		Expect(sut.Dir()).To(Equal("dir"))
	})
})
