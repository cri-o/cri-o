package oci_test

import (
	"errors"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/containers/storage/pkg/idtools"
	"github.com/containers/storage/pkg/unshare"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runtime-spec/specs-go"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/storage"
)

const (
	neverRunningPid  = 4194305
	alwaysRunningPid = 1
)

// The actual test suite.
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
		Expect(sut.UserRequestedImage()).To(Equal("image"))
		Expect(sut.SomeNameOfTheImage().StringForOutOfProcessConsumptionOnly()).To(Equal("docker.io/library/image-name:latest"))
		Expect(sut.ImageID().IDStringForOutOfProcessConsumptionOnly()).To(Equal("2a03a6059f21e150ae84b0973863609494aad70f0a80eaeb64bddd8d92465812"))
		Expect(sut.Sandbox()).To(Equal("sandbox"))
		Expect(sut.Dir()).To(Equal("dir"))
		Expect(sut.CheckpointPath()).To(Equal("dir/checkpoint"))
		Expect(sut.StatePath()).To(Equal("dir/state.json"))
		Expect(sut.Metadata()).To(Equal(&types.ContainerMetadata{}))
		Expect(sut.StateNoLock().Version).To(BeEmpty())
		Expect(sut.GetStopSignal()).To(Equal("15"))
		Expect(sut.CreatedAt().UnixNano()).
			To(BeNumerically("<", time.Now().UnixNano()))
		Expect(sut.Spoofed()).To(BeFalse())
		Expect(sut.Restore()).To(BeFalse())
		Expect(sut.RestoreArchivePath()).To(Equal(""))
		Expect(sut.RestoreStorageImageID()).To(BeNil())
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
		err := errors.New("error")

		// When
		sut.SetStartFailed(err)

		// Then
		Expect(sut.State().Error).To(Equal(err.Error()))
	})

	It("should succeed to set restore", func() {
		// Given
		restore := true

		// When
		sut.SetRestore(restore)

		// Then
		Expect(sut.Restore()).To(Equal(restore))
	})

	It("should succeed to set restore is oci image", func() {
		// Given
		storageImageID, err := storage.ParseStorageImageIDFromOutOfProcessData("1111111111111111111111111111111111111111111111111111111111111111")
		Expect(err).ToNot(HaveOccurred())

		// When
		sut.SetRestoreStorageImageID(&storageImageID)

		// Then
		Expect(sut.RestoreStorageImageID()).To(Equal(&storageImageID))
	})

	It("should succeed to set restore archive", func() {
		// Given
		restoreArchive := "image-name"

		// When
		sut.SetRestoreArchivePath(restoreArchive)

		// Then
		Expect(sut.RestoreArchivePath()).To(Equal(restoreArchive))
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
			"", nil, nil, "", &types.ContainerMetadata{}, "",
			false, false, false, "", "", time.Now(), "SIGNO")
		Expect(err).ToNot(HaveOccurred())
		Expect(container).NotTo(BeNil())

		// When
		signal := container.GetStopSignal()

		// Then
		Expect(signal).To(Equal("15"))
	})

	It("should succeed get the right stop signal on SIGRTMIN", func() {
		// Given
		container, err := oci.NewContainer("", "", "", "",
			map[string]string{}, map[string]string{}, map[string]string{},
			"", nil, nil, "", &types.ContainerMetadata{}, "",
			false, false, false, "", "", time.Now(), "RTMIN+1")
		Expect(err).ToNot(HaveOccurred())
		Expect(container).NotTo(BeNil())

		// When
		signal := container.GetStopSignal()

		// Then
		Expect(signal).To(Equal("35"))
	})

	It("should succeed get the non default stop signal", func() {
		// Given
		container, err := oci.NewContainer("", "", "", "",
			map[string]string{}, map[string]string{}, map[string]string{},
			"", nil, nil, "", &types.ContainerMetadata{}, "",
			false, false, false, "", "", time.Now(), "SIGTRAP")
		Expect(err).ToNot(HaveOccurred())
		Expect(container).NotTo(BeNil())

		// When
		signal := container.GetStopSignal()

		// Then
		Expect(signal).To(Equal("5"))
	})

	It("should succeed to set the all container resources", func() {
		// Given
		var cpuPeriod uint64 = 100000
		var cpuQuota int64 = 20000
		var cpuShares uint64 = 1024
		cpusetCpus := "0-3,12-15"
		cpusetMems := "0,1"
		oomScoreAdj := 100
		var memoryLimitInBytes int64 = 1024
		var memorySwapLimitInBytes int64 = 1024
		hugepageLimits := []specs.LinuxHugepageLimit{
			{
				Pagesize: "1KB",
				Limit:    1024,
			},
			{
				Pagesize: "2KB",
				Limit:    2048,
			},
		}
		unified := map[string]string{
			"key1": "value1",
			"key2": "value2",
		}

		newSpec := specs.Spec{
			Linux: &specs.Linux{
				Resources: &specs.LinuxResources{
					CPU: &specs.LinuxCPU{
						Shares: &cpuShares,
						Quota:  &cpuQuota,
						Period: &cpuPeriod,
						Cpus:   cpusetCpus,
						Mems:   cpusetMems,
					},
					Memory: &specs.LinuxMemory{
						Limit: &memoryLimitInBytes,
						Swap:  &memorySwapLimitInBytes,
					},
					HugepageLimits: hugepageLimits,
					Unified:        unified,
				},
			},
			Process: &specs.Process{
				OOMScoreAdj: &oomScoreAdj,
			},
		}

		// When
		sut.SetSpec(&newSpec)
		containerResources := sut.GetResources()

		// Then
		Expect(containerResources.GetLinux().GetCpuPeriod()).To(Equal(int64(cpuPeriod)))
		Expect(containerResources.GetLinux().GetCpuQuota()).To(Equal(cpuQuota))
		Expect(containerResources.GetLinux().GetCpuShares()).To(Equal(int64(cpuShares)))
		Expect(containerResources.GetLinux().GetCpusetCpus()).To(Equal(cpusetCpus))
		Expect(containerResources.GetLinux().GetCpusetMems()).To(Equal(cpusetMems))
		Expect(containerResources.GetLinux().GetOomScoreAdj()).To(Equal(int64(oomScoreAdj)))
		Expect(containerResources.GetLinux().GetMemoryLimitInBytes()).To(Equal(memoryLimitInBytes))
		Expect(containerResources.GetLinux().GetMemorySwapLimitInBytes()).To(Equal(memorySwapLimitInBytes))
		Expect(containerResources.GetLinux().GetUnified()).To(Equal(unified))
		for i := range len(containerResources.GetLinux().GetHugepageLimits()) {
			Expect(containerResources.GetLinux().GetHugepageLimits()[i].GetPageSize()).To(Equal(hugepageLimits[i].Pagesize))
			Expect(containerResources.GetLinux().GetHugepageLimits()[i].GetLimit()).To(Equal(hugepageLimits[i].Limit))
		}
	})

	It("should succeed to set the fewer container resources", func() {
		// Given
		var cpuPeriod uint64 = 100000
		var cpuQuota int64 = 20000
		var cpuShares uint64 = 1024
		cpusetCpus := "0-3,12-15"
		cpusetMems := "0,1"
		var memoryLimitInBytes int64 = 1024
		var memorySwapLimitInBytes int64 = 1024

		newSpec := specs.Spec{
			Linux: &specs.Linux{
				Resources: &specs.LinuxResources{
					CPU: &specs.LinuxCPU{
						Shares: &cpuShares,
						Quota:  &cpuQuota,
						Period: &cpuPeriod,
						Cpus:   cpusetCpus,
						Mems:   cpusetMems,
					},
					Memory: &specs.LinuxMemory{
						Limit: &memoryLimitInBytes,
						Swap:  &memorySwapLimitInBytes,
					},
				},
			},
		}

		// When
		sut.SetSpec(&newSpec)
		containerResources := sut.GetResources()

		// Then
		Expect(containerResources.GetLinux().GetCpuPeriod()).To(Equal(int64(cpuPeriod)))
		Expect(containerResources.GetLinux().GetCpuQuota()).To(Equal(cpuQuota))
		Expect(containerResources.GetLinux().GetCpuShares()).To(Equal(int64(cpuShares)))
		Expect(containerResources.GetLinux().GetCpusetCpus()).To(Equal(cpusetCpus))
		Expect(containerResources.GetLinux().GetCpusetMems()).To(Equal(cpusetMems))
		Expect(containerResources.GetLinux().GetMemoryLimitInBytes()).To(Equal(memoryLimitInBytes))
		Expect(containerResources.GetLinux().GetMemorySwapLimitInBytes()).To(Equal(memorySwapLimitInBytes))
		Expect(containerResources.GetLinux().GetUnified()).To(BeEmpty())
		Expect(containerResources.GetLinux().GetHugepageLimits()).To(BeEmpty())
	})

	t.Describe("FromDisk", func() {
		BeforeEach(func() {
			Expect(os.MkdirAll(sut.Dir(), 0o755)).To(Succeed())
		})
		AfterEach(func() {
			os.RemoveAll(sut.Dir())
		})
		It("should succeed to get the state from disk", func() {
			// Given
			Expect(os.WriteFile(path.Join(sut.Dir(), "state.json"),
				[]byte("{}"), 0o644)).To(Succeed())

			// When
			err := sut.FromDisk()

			// Then
			Expect(err).ToNot(HaveOccurred())
		})
		It("should succeed when pid set but initialPid not set", func() {
			// Given
			Expect(os.WriteFile(path.Join(sut.Dir(), "state.json"), []byte(`
			{"pid":`+strconv.Itoa(alwaysRunningPid)+`}`),
				0o644)).To(Succeed())

			// When
			err := sut.FromDisk()

			// Then
			Expect(err).ToNot(HaveOccurred())
			sutState := sut.State()
			Expect(sutState.InitStartTime).NotTo(Equal(""))
			Expect(sutState.InitPid).To(Equal(alwaysRunningPid))
		})
		It("should succeed when pid set but initialPid not set", func() {
			// Given
			Expect(os.WriteFile(path.Join(sut.Dir(), "state.json"), []byte(`
			{"pid":`+strconv.Itoa(alwaysRunningPid)+`}`),
				0o644)).To(Succeed())

			// When
			err := sut.FromDisk()

			// Then
			Expect(err).ToNot(HaveOccurred())
			sutState := sut.State()
			Expect(sutState.InitStartTime).NotTo(Equal(""))
			Expect(sutState.InitPid).To(Equal(alwaysRunningPid))
		})
		It("should fail when pid set and not running", func() {
			// Given
			Expect(os.WriteFile(path.Join(sut.Dir(), "state.json"), []byte(`
			{"pid":`+strconv.Itoa(neverRunningPid)+`}`),
				0o644)).To(Succeed())

			// When
			err := sut.FromDisk()

			// Then
			Expect(err).To(HaveOccurred())
			sutState := sut.State()
			Expect(sutState.InitStartTime).To(Equal(""))
			Expect(sutState.InitPid).To(Equal(0))
		})

		It("should fail to get the state from disk if invalid json", func() {
			// Given
			Expect(os.WriteFile(path.Join(sut.Dir(), "state.json"),
				[]byte("invalid"), 0o644)).To(Succeed())

			// When
			err := sut.FromDisk()

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail to get the state from disk if not existing", func() {
			// Given
			// When
			err := sut.FromDisk()

			// Then
			Expect(err).To(HaveOccurred())
		})
	})
	t.Describe("Living", func() {
		It("should be false if pid uninitialized", func() {
			// Given
			state := &oci.ContainerState{}
			state.Pid = 0
			sut.SetState(state)
			// When
			err := sut.Living()

			// Then
			Expect(err).To(HaveOccurred())
		})
		It("should succeed if pid is running", func() {
			if unshare.IsRootless() {
				Skip("need to run as root")
			}

			// Given
			state := &oci.ContainerState{}
			state.Pid = alwaysRunningPid
			Expect(state.SetInitPid(state.Pid)).To(Succeed())
			sut.SetState(state)
			// When
			err := sut.Living()

			// Then
			Expect(err).ToNot(HaveOccurred())
		})
		It("should be false if pid is not running", func() {
			// Given
			state := &oci.ContainerState{}
			state.Pid = neverRunningPid
			// SetInitPid will fail because the pid is not running
			Expect(state.SetInitPid(state.Pid)).NotTo(Succeed())
			sut.SetState(state)
			// When
			err := sut.Living()

			// Then
			Expect(err).To(HaveOccurred())
		})
	})
	t.Describe("ProcessState", func() {
		It("should be false if pid uninitialized", func() {
			// Given
			state := &oci.ContainerState{}
			state.Pid = 0
			sut.SetState(state)
			// When
			processState, err := sut.ProcessState()

			// Then
			Expect(err).To(HaveOccurred())
			Expect(processState).To(BeEmpty())
		})
		It("should succeed if pid is running", func() {
			if unshare.IsRootless() {
				Skip("need to run as root")
			}

			// Given
			state := &oci.ContainerState{}
			state.Pid = alwaysRunningPid
			Expect(state.SetInitPid(state.Pid)).To(Succeed())
			sut.SetState(state)
			// When
			processState, err := sut.ProcessState()

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(processState).To(Equal("S")) // A process will be sleeping most of the time.
		})
		It("should be false if pid is not running", func() {
			// Given
			state := &oci.ContainerState{}
			state.Pid = neverRunningPid
			// SetInitPid will fail because the pid is not running
			Expect(state.SetInitPid(state.Pid)).NotTo(Succeed())
			sut.SetState(state)
			// When
			processState, err := sut.ProcessState()

			// Then
			Expect(err).To(HaveOccurred())
			Expect(processState).To(BeEmpty())
		})
	})
	t.Describe("Pid", func() {
		It("should fail if container state not set", func() {
			// Given
			// When
			pid, err := sut.Pid()
			// Then
			Expect(pid).To(Equal(0))
			Expect(err).To(HaveOccurred())
		})
		It("should fail when pid is negative", func() {
			// Given
			state := &oci.ContainerState{}
			state.Pid = -1
			// SetInitPid will fail because the pid is not running
			Expect(state.SetInitPid(state.Pid)).NotTo(Succeed())
			sut.SetState(state)

			// When
			pid, err := sut.Pid()
			// Then
			Expect(pid).To(Equal(0))
			Expect(err).To(HaveOccurred())
		})
		It("should fail gracefully when pid has been stopped", func() {
			// Given
			state := &oci.ContainerState{}
			state.Pid = alwaysRunningPid
			Expect(state.SetInitPid(state.Pid)).To(Succeed())
			// a `runtime state ctr` call after the container has been stopped
			// will set the state pid to 0. However, InitPid never changes
			// so we have a separate handle for when Pid is 0 but InitPid is not
			state.Pid = 0
			sut.SetState(state)

			// When
			pid, err := sut.Pid()
			// Then
			Expect(pid).To(Equal(0))
			Expect(errors.Is(err, oci.ErrNotFound)).To(BeTrue())
		})
		It("should fail if process is not found", func() {
			// Given
			state := &oci.ContainerState{}
			state.Pid = neverRunningPid
			Expect(state.SetInitPid(state.Pid)).NotTo(Succeed())
			sut.SetState(state)

			// When
			pid, err := sut.Pid()
			// Then
			Expect(pid).To(Equal(0))
			Expect(errors.Is(err, oci.ErrNotFound)).To(BeTrue())
		})
		It("should fail when pid has wrapped", func() {
			// Given
			state := &oci.ContainerState{}
			state.Pid = alwaysRunningPid
			Expect(state.SetInitPid(state.Pid)).To(Succeed())
			// if InitStartTime != the time the state.InitPid started
			// pid wrap is assumed to have happened
			state.InitStartTime = ""
			sut.SetState(state)

			// When
			pid, err := sut.Pid()
			// Then
			Expect(pid).To(Equal(0))
			Expect(err).To(HaveOccurred())
		})
		It("should succeed", func() {
			if unshare.IsRootless() {
				Skip("need to run as root")
			}

			// Given
			state := &oci.ContainerState{}
			state.Pid = alwaysRunningPid
			Expect(state.SetInitPid(state.Pid)).To(Succeed())
			sut.SetState(state)

			// When
			pid, err := sut.Pid()
			// Then
			Expect(pid).To(Equal(alwaysRunningPid))
			Expect(err).ToNot(HaveOccurred())
		})
	})
	t.Describe("SetInitPid", func() {
		It("should suceeed if running", func() {
			// Given
			state := &oci.ContainerState{}
			// When
			state.Pid = alwaysRunningPid
			// Then
			Expect(state.SetInitPid(state.Pid)).To(Succeed())
		})
		It("should fail if already set", func() {
			// Given
			state := &oci.ContainerState{}
			state.Pid = alwaysRunningPid
			// When
			Expect(state.SetInitPid(state.Pid)).To(Succeed())
			// Then
			Expect(state.SetInitPid(state.Pid)).NotTo(Succeed())
		})
		It("should fail if not running", func() {
			// Given
			state := &oci.ContainerState{}
			// When
			state.Pid = neverRunningPid
			// Then
			Expect(state.SetInitPid(state.Pid)).NotTo(Succeed())
		})
	})
	t.Describe("GetPidStartTimeFromFile", func() {
		var statFile string
		BeforeEach(func() {
			statFile = t.MustTempFile("stat")
		})
		It("should fail if file doesn't exist", func() {
			// When
			stime, err := oci.GetPidStartTimeFromFile("not-there")

			// Then
			Expect(stime).To(BeEmpty())
			Expect(err).To(HaveOccurred())
		})
		It("should fail when there are no parenthesis", func() {
			contents := []byte("1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 24 25 26 27 28 29 30 31 32 33 34 35 36 37 38 39 40 41 42 43 44 45 46 47 48 49 50 51 52")
			Expect(os.WriteFile(statFile, contents, 0o644)).To(Succeed())

			// When
			stime, err := oci.GetPidStartTimeFromFile(statFile)

			// Then
			Expect(stime).To(BeEmpty())
			Expect(err).To(HaveOccurred())
		})
		It("should fail with short file", func() {
			contents := []byte("1 (2) 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21")
			Expect(os.WriteFile(statFile, contents, 0o644)).To(Succeed())

			// When
			stime, err := oci.GetPidStartTimeFromFile(statFile)

			// Then
			Expect(stime).To(BeEmpty())
			Expect(err).To(HaveOccurred())
		})
		It("should succeed to get start time", func() {
			contents := []byte("1 (2) 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 24 25 26 27 28 29 30 31 32 33 34 35 36 37 38 39 40 41 42 43 44 45 46 47 48 49 50 51 52")
			Expect(os.WriteFile(statFile, contents, 0o644)).To(Succeed())

			// When
			stime, err := oci.GetPidStartTimeFromFile(statFile)

			// Then
			Expect(stime).To(Equal("22"))
			Expect(err).ToNot(HaveOccurred())
		})
	})
})

var _ = t.Describe("SpoofedContainer", func() {
	It("should succeed to get the container fields", func() {
		sut := oci.NewSpoofedContainer("id", "name", map[string]string{"key": "label"}, "sbox", time.Now(), "dir")
		// Given
		// When
		// Then
		Expect(sut.ID()).To(Equal("id"))
		Expect(sut.Name()).To(Equal("name"))
		labels := sut.Labels()
		Expect(labels["key"]).To(Equal("label"))
		Expect(sut.Spoofed()).To(BeTrue())
		Expect(sut.CreatedAt().UnixNano()).
			To(BeNumerically("<", time.Now().UnixNano()))
		Expect(sut.Dir()).To(Equal("dir"))
		Expect(sut.Sandbox()).To(Equal("sbox"))
	})
})
