package container_test

import (
	"github.com/cri-o/cri-o/internal/factory/container"
	sconfig "github.com/cri-o/cri-o/pkg/config"
	containermock "github.com/cri-o/cri-o/test/mocks/container"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
)

var _ = t.Describe("Container", func() {
	var (
		//containerConfig *types.ContainerConfig
		//sboxConfig *types.PodSandboxConfig
		serverConfig  *sconfig.Config
		actualMount   *rspec.Mount
		expectedMount *rspec.Mount
		secLabel      *container.SecLabel
		implMock      *containermock.MockImpl
		mockCtrl      *gomock.Controller
	)
	BeforeEach(func() {
		// setup mountInfo
		container.GetContainerInfo(sut).NewMountInfo()

		// Setup configs
		serverConfig = &sconfig.Config{}
		serverConfig.RuntimeConfig.BindMountPrefix = ""
		serverConfig.Root = ""
		serverConfig.AbsentMountSourcesToReject = nil
		/*
			containerConfig = &types.ContainerConfig{
				Metadata: &types.ContainerMetadata{Name: "name"},
			}
			sboxConfig = &types.PodSandboxConfig{}*/

		secLabel = container.NewSecLabel()
		Expect(secLabel).NotTo(BeNil())

		mockCtrl = gomock.NewController(GinkgoT())
		implMock = containermock.NewMockImpl(mockCtrl)
		secLabel.SetImpl(implMock)
		gomock.InOrder(
			implMock.EXPECT().SecurityLabel(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
		)
	})
	AfterEach(func() {
		container.GetContainerInfo(sut).ClearMountInfo()
		mockCtrl.Finish()
	})
	t.Describe("Test setupReadOnlyMounts", func() {
		expectedMount = &rspec.Mount{
			Destination: "",
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options:     []string{"rw", "noexec", "nosuid", "nodev", "tmpcopyup"},
		}
		container.GetContainerInfo(sut).SetupReadOnlyMounts(true)
		It("should match the expected results /run", func() {
			destination := "/run"
			mode := "mode=0755"
			expectedMount.Destination = destination
			expectedMount.Options = append(expectedMount.Options, mode)
			actualMount = getMount(destination)
			Expect(expectedMount).To(Equal(actualMount))
		})
		It("should match the expected results /tmp", func() {
			destination := "/tmp"
			mode := "mode=1777"
			expectedMount.Destination = destination
			expectedMount.Options = append(expectedMount.Options, mode)
			actualMount = getMount(destination)
			Expect(expectedMount).To(Equal(actualMount))
		})
		It("should match the expected results /var/tmp", func() {
			destination := "/var/tmp"
			mode := "mode=1777"
			expectedMount.Destination = destination
			expectedMount.Options = append(expectedMount.Options, mode)
			actualMount = getMount(destination)
			Expect(expectedMount).To(Equal(actualMount))
		})
	})
	t.Describe("Test setupHostNetworkMounts", func() {
		optionsRW := []string{"rw"}
		container.GetContainerInfo(sut).SetupHostNetworkMounts(true, optionsRW)
		It("should match the expected results /sys", func() {
			expectedMount = &rspec.Mount{
				Destination: "/sys",
				Type:        "sysfs",
				Source:      "sysfs",
				Options:     []string{"nosuid", "noexec", "nodev", "ro"},
			}
			actualMount = getMount("/sys")
			Expect(expectedMount).To(Equal(actualMount))
		})
		It("should match the expected results /sys/fs/cgroup", func() {
			expectedMount = &rspec.Mount{
				Destination: "/sys/fs/cgroup",
				Type:        "cgroup",
				Source:      "cgroup",
				Options:     []string{"nosuid", "noexec", "nodev", "relatime", "ro"},
			}
			actualMount = getMount("/sys/fs/cgroup")
			Expect(expectedMount).To(Equal(actualMount))
		})
		It("should match the expected results /etc/hosts", func() {
			expectedMount = &rspec.Mount{
				Destination: "/etc/hosts",
				Type:        "bind",
				Source:      "/etc/hosts",
				Options:     append(optionsRW, "bind"),
			}
			actualMount = getMount("/etc/hosts")
			Expect(expectedMount).To(Equal(actualMount))
		})
	})
	Describe("Test setupPrivilegedMounts", func() {
		container.GetContainerInfo(sut).SetupPrivilegedMounts()
		It("should match the expected result /sys", func() {
			expectedMount = &rspec.Mount{
				Destination: "/sys",
				Type:        "sysfs",
				Source:      "sysfs",
				Options:     []string{"nosuid", "noexec", "nodev", "rw", "rslave"},
			}
			actualMount = getMount("/sys")
			Expect(expectedMount).To(Equal(actualMount))
		})
		It("should match the expected result /sys", func() {
			expectedMount = &rspec.Mount{
				Destination: "/sys/fs/cgroup",
				Type:        "cgroup",
				Source:      "cgroup",
				Options:     []string{"nosuid", "noexec", "nodev", "rw", "relatime", "rslave"},
			}
			actualMount = getMount("/sys/fs/cgroup")
			Expect(expectedMount).To(Equal(actualMount))
		})
		It("should not match the expected result /sys", func() {
			expectedMount = &rspec.Mount{
				Destination: "/sys/fs/cgroup",
				Type:        "cgroup",
				Source:      "cgroup",
				Options:     []string{"nosuid", "noexec", "nodev", "rw", "rslave"},
			}
			actualMount = getMount("/sys/fs/cgroup")
			Expect(expectedMount).ToNot(Equal(actualMount))
		})
	})
	Describe("Test setupShmMounts", func() {
		container.GetContainerInfo(sut).SetupShmMounts("/root/shm")
		It("should match the expected result", func() {
			expectedMount = &rspec.Mount{
				Destination: "/dev/shm",
				Type:        "bind",
				Source:      "/root/shm",
				Options:     []string{"rw", "bind"},
			}
			actualMount = getMount("/dev/shm")
			Expect(expectedMount).To(Equal(actualMount))
		})
		It("should not match the expected result", func() {
			expectedMount = &rspec.Mount{
				Destination: "/dev/shm",
				Type:        "bind",
				Source:      "/root/shm",
				Options:     []string{"ro", "bind"},
			}
			actualMount = getMount("/dev/shm")
			Expect(expectedMount).ToNot(Equal(actualMount))
		})
	})
	Describe("Test setupHostPropMounts", func() {
		propMounts := []*rspec.Mount{
			&rspec.Mount{
				Destination: "/etc/resolv.conf",
				Type:        "bind",
				Source:      "/test/resolv.conf",
				Options:     []string{"bind", "nodev", "nosuid", "noexec", "rw"},
			},
			&rspec.Mount{
				Destination: "/etc/hostname",
				Type:        "bind",
				Source:      "/test/hostname",
				Options:     []string{"bind", "rw"},
			},
			&rspec.Mount{
				Destination: "/run/.containerenv",
				Type:        "bind",
				Source:      "/test/env",
				Options:     []string{"bind", "rw"},
			},
		}
		optionsRW := []string{"rw"}
		container.GetContainerInfo(sut).SetupHostPropMounts("/test/resolv.conf", "/test/hostname", "/test/env", "", optionsRW)
		It("should match the expected results", func() {
			for _, expectedMount = range propMounts {
				actualMount = getMount(expectedMount.Destination)
				Expect(actualMount).To(Equal(expectedMount))
			}
		})
	})
})

func getMount(dst string) *rspec.Mount {
	for _, mount := range sut.Spec().Mounts() {
		if mount.Destination == dst {
			return &mount
		}
	}
	return nil
}

/*
func TestAddOCIBindsForDev(t *testing.T) {
	ctr, err := New()
	c := ctr.getContainerInfo()
	if err != nil {
		t.Error(err)
	}
	if err := ctr.SetConfig(&types.ContainerConfig{
		Mounts: []*types.Mount{
			{
				ContainerPath: "/dev",
				HostPath:      "/dev",
			},
		},
		Metadata: &types.ContainerMetadata{
			Name: "testctr",
		},
	}, &types.PodSandboxConfig{
		Metadata: &types.PodSandboxMetadata{
			Name: "testpod",
		},
	}); err != nil {
		t.Error(err)
	}

	config := sconfig.Config{}
	config.RuntimeConfig.BindMountPrefix = ""
	config.Root = ""
	config.AbsentMountSourcesToReject = nil
	_, err = c.addOCIBindMounts(context.Background(), "", &config, false, false, false)
	if err != nil {
		t.Error(err)
	}
	specAddMounts(c)
	for _, m := range ctr.Spec().Mounts() {
		if m.Destination == "/dev" {
			t.Error("/dev shouldn't be in the spec if it's bind mounted from kube")
		}
	}
	var foundDev bool
	for _, b := range ctr.getContainerInfo().mountInfo.criMounts {
		if b.Destination == "/dev" {
			foundDev = true
			break
		}
	}
	if !foundDev {
		t.Error("no /dev mount found in spec mounts")
	}
}

func TestAddOCIBindsForSys(t *testing.T) {
	ctr, err := New()
	c := ctr.getContainerInfo()
	if err != nil {
		t.Error(err)
	}
	if err := ctr.SetConfig(&types.ContainerConfig{
		Mounts: []*types.Mount{
			{
				ContainerPath: "/sys",
				HostPath:      "/sys",
			},
		},
		Metadata: &types.ContainerMetadata{
			Name: "testctr",
		},
	}, &types.PodSandboxConfig{
		Metadata: &types.PodSandboxMetadata{
			Name: "testpod",
		},
	}); err != nil {
		t.Error(err)
	}

	config := sconfig.Config{}
	config.RuntimeConfig.BindMountPrefix = ""
	config.Root = ""
	config.AbsentMountSourcesToReject = nil
	_, err = c.addOCIBindMounts(context.Background(), "", &config, false, false, false)
	if err != nil {
		t.Error(err)
	}
	var howManySys int
	for _, b := range c.mountInfo.criMounts {
		if b.Destination == "/sys" && b.Type != "sysfs" {
			howManySys++
		}
	}
	if howManySys != 1 {
		t.Error("there is not a single /sys bind mount")
	}
}

func TestAddOCIBindsCGroupRW(t *testing.T) {
	ctr, err := New()
	c := ctr.getContainerInfo()
	if err != nil {
		t.Error(err)
	}
	if err := ctr.SetConfig(&types.ContainerConfig{
		Metadata: &types.ContainerMetadata{
			Name: "testctr",
		},
	}, &types.PodSandboxConfig{
		Metadata: &types.PodSandboxMetadata{
			Name: "testpod",
		},
	}); err != nil {
		t.Error(err)
	}

	config := sconfig.Config{}
	config.RuntimeConfig.BindMountPrefix = ""
	config.Root = ""
	config.AbsentMountSourcesToReject = nil
	_, err = c.addOCIBindMounts(context.Background(), "", &config, false, false, true)
	if err != nil {
		t.Error(err)
	}
	specAddMounts(c)
	var hasCgroupRW bool
	for _, m := range ctr.Spec().Mounts() {
		if m.Destination == "/sys/fs/cgroup" {
			for _, o := range m.Options {
				if o == "rw" {
					hasCgroupRW = true
				}
			}
		}
	}
	if !hasCgroupRW {
		t.Error("Cgroup mount not added with RW.")
	}

	ctr, err = New()
	if err != nil {
		t.Error(err)
	}
	if err := ctr.SetConfig(&types.ContainerConfig{
		Metadata: &types.ContainerMetadata{
			Name: "testctr",
		},
	}, &types.PodSandboxConfig{
		Metadata: &types.PodSandboxMetadata{
			Name: "testpod",
		},
	}); err != nil {
		t.Error(err)
	}
	var hasCgroupRO bool
	_, err = c.addOCIBindMounts(context.Background(), "", &config, false, false, false)
	if err != nil {
		t.Error(err)
	}
	specAddMounts(c)
	for _, m := range ctr.Spec().Mounts() {
		if m.Destination == "/sys/fs/cgroup" {
			for _, o := range m.Options {
				if o == "ro" {
					hasCgroupRO = true
				}
			}
		}
	}
	if !hasCgroupRO {
		t.Error("Cgroup mount not added with RO.")
	}
}

func TestAddOCIBindsErrorWithoutIDMap(t *testing.T) {
	ctr, err := New()
	c := ctr.getContainerInfo()
	if err != nil {
		t.Fatal(err)
	}

	if err := ctr.SetConfig(&types.ContainerConfig{
		Mounts: []*types.Mount{
			{
				ContainerPath: "/sys",
				HostPath:      "/sys",
				UidMappings: []*types.IDMapping{
					{
						HostId:      1000,
						ContainerId: 1,
						Length:      1000,
					},
				},
			},
		},
		Metadata: &types.ContainerMetadata{
			Name: "testctr",
		},
	}, &types.PodSandboxConfig{
		Metadata: &types.PodSandboxMetadata{
			Name: "testpod",
		},
	}); err != nil {
		t.Fatal(err)
	}
	config := sconfig.Config{}
	config.RuntimeConfig.BindMountPrefix = ""
	config.Root = ""
	config.AbsentMountSourcesToReject = nil
	_, err = c.addOCIBindMounts(context.Background(), "", &config, false, false, false)
	if err == nil {
		t.Errorf("Should have failed to create id mapped mount with no id map support")
	}

	_, err = c.addOCIBindMounts(context.Background(), "", &config, false, true, false)
	if err != nil {
		t.Errorf("%v", err)
	}
}
*/
