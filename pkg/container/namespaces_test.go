package container_test

import (
	"os"

	"github.com/cri-o/cri-o/internal/config/nsmgr"
	nsmgrtest "github.com/cri-o/cri-o/internal/config/nsmgr/test"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/pkg/config"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

var _ = t.Describe("Container:SpecAddNamespaces", func() {
	It("should inherit pod namespaces", func() {
		// Given
		ctrConfig := &types.ContainerConfig{
			Metadata: &types.ContainerMetadata{Name: "name"},
			Linux: &types.LinuxContainerConfig{
				SecurityContext: &types.LinuxContainerSecurityContext{
					NamespaceOptions: &types.NamespaceOption{
						Network: types.NamespaceMode_POD,
						Ipc:     types.NamespaceMode_POD,
						Pid:     types.NamespaceMode_CONTAINER,
					},
				},
			},
		}

		sboxConfig := &types.PodSandboxConfig{}
		sb := &sandbox.Sandbox{}
		sb.AddManagedNamespaces(nsmgrtest.AllSpoofedNamespaces)
		sut.Spec().ClearLinuxNamespaces()

		// When
		Expect(sut.SetConfig(ctrConfig, sboxConfig)).To(BeNil())
		Expect(sut.SpecAddNamespaces(sb, nil, nil)).To(BeNil())

		// Then
		spec := sut.Spec()
		Expect(len(spec.Config.Linux.Namespaces)).To(Equal(len(nsmgrtest.AllSpoofedNamespaces)))
		for _, ns := range nsmgrtest.AllSpoofedNamespaces {
			found := false
			for _, specNs := range spec.Config.Linux.Namespaces {
				if specNs.Path == ns.Path() {
					found = true
				}
			}
			Expect(found).To(Equal(true))
		}
	})
	It("should drop network if hostNet", func() {
		// Given
		ctrConfig := &types.ContainerConfig{
			Metadata: &types.ContainerMetadata{Name: "name"},
			Linux: &types.LinuxContainerConfig{
				SecurityContext: &types.LinuxContainerSecurityContext{
					NamespaceOptions: &types.NamespaceOption{
						Network: types.NamespaceMode_NODE,
						Ipc:     types.NamespaceMode_POD,
						Pid:     types.NamespaceMode_CONTAINER,
					},
				},
			},
		}

		sboxConfig := &types.PodSandboxConfig{}
		sb := &sandbox.Sandbox{}
		sb.AddManagedNamespaces(nsmgrtest.AllSpoofedNamespaces)

		// When
		Expect(sut.SetConfig(ctrConfig, sboxConfig)).To(BeNil())
		sut.Spec().ClearLinuxNamespaces()
		Expect(sut.SpecAddNamespaces(sb, nil, nil)).To(BeNil())

		// Then
		spec := sut.Spec()
		Expect(len(spec.Config.Linux.Namespaces)).To(Equal(len(nsmgrtest.AllSpoofedNamespaces) - 1))

		for _, specNs := range spec.Config.Linux.Namespaces {
			Expect(specNs.Type).NotTo(Equal(rspec.NetworkNamespace))
		}
	})
	It("should drop PID if hostPID", func() {
		// Given
		ctrConfig := &types.ContainerConfig{
			Metadata: &types.ContainerMetadata{Name: "name"},
			Linux: &types.LinuxContainerConfig{
				SecurityContext: &types.LinuxContainerSecurityContext{
					NamespaceOptions: &types.NamespaceOption{
						Network: types.NamespaceMode_POD,
						Ipc:     types.NamespaceMode_POD,
						Pid:     types.NamespaceMode_NODE,
					},
				},
			},
		}

		sboxConfig := &types.PodSandboxConfig{}
		sb := &sandbox.Sandbox{}
		sb.AddManagedNamespaces(nsmgrtest.AllSpoofedNamespaces)

		// When
		Expect(sut.SetConfig(ctrConfig, sboxConfig)).To(BeNil())
		sut.Spec().ClearLinuxNamespaces()
		Expect(sut.SpecAddNamespaces(sb, nil, nil)).To(BeNil())

		// Then
		spec := sut.Spec()
		Expect(len(spec.Config.Linux.Namespaces)).To(Equal(len(nsmgrtest.AllSpoofedNamespaces)))

		for _, specNs := range spec.Config.Linux.Namespaces {
			Expect(specNs.Type).NotTo(Equal(rspec.PIDNamespace))
		}
	})
	It("should use pod PID", func() {
		// Given
		ctrConfig := &types.ContainerConfig{
			Metadata: &types.ContainerMetadata{Name: "name"},
			Linux: &types.LinuxContainerConfig{
				SecurityContext: &types.LinuxContainerSecurityContext{
					NamespaceOptions: &types.NamespaceOption{
						Network: types.NamespaceMode_POD,
						Ipc:     types.NamespaceMode_POD,
						Pid:     types.NamespaceMode_POD,
					},
				},
			},
		}

		sboxConfig := &types.PodSandboxConfig{
			Linux: &types.LinuxPodSandboxConfig{
				SecurityContext: &types.LinuxSandboxSecurityContext{
					NamespaceOptions: &types.NamespaceOption{
						Network: types.NamespaceMode_POD,
						Ipc:     types.NamespaceMode_POD,
						Pid:     types.NamespaceMode_POD,
					},
				},
			},
		}
		sb := &sandbox.Sandbox{}
		infra, err := nsmgrtest.ContainerWithPid(os.Getpid())
		Expect(err).To(BeNil())
		Expect(sb.SetInfraContainer(infra)).To(BeNil())
		sb.AddManagedNamespaces(nsmgrtest.AllSpoofedNamespaces)

		// When
		Expect(sut.SetConfig(ctrConfig, sboxConfig)).To(BeNil())
		sut.Spec().ClearLinuxNamespaces()
		Expect(sut.SpecAddNamespaces(sb, nil, nil)).To(BeNil())

		// Then
		spec := sut.Spec()
		Expect(len(spec.Config.Linux.Namespaces)).To(Equal(len(nsmgrtest.AllSpoofedNamespaces) + 1))

		found := false
		for _, specNs := range spec.Config.Linux.Namespaces {
			if specNs.Type == rspec.PIDNamespace {
				found = true
			}
		}
		Expect(found).To(Equal(true))
	})
	It("should use target PID namespace", func() {
		// Given
		ctrConfig := &types.ContainerConfig{
			Metadata: &types.ContainerMetadata{Name: "name"},
			Linux: &types.LinuxContainerConfig{
				SecurityContext: &types.LinuxContainerSecurityContext{
					NamespaceOptions: &types.NamespaceOption{
						Network: types.NamespaceMode_POD,
						Ipc:     types.NamespaceMode_POD,
						Pid:     types.NamespaceMode_TARGET,
					},
				},
			},
		}

		sboxConfig := &types.PodSandboxConfig{}
		sb := &sandbox.Sandbox{}
		targetCtr, err := nsmgrtest.ContainerWithPid(os.Getpid())
		Expect(err).To(BeNil())

		sb.AddManagedNamespaces(nsmgrtest.AllSpoofedNamespaces)
		cfg := &config.Config{}
		nsMgr := nsmgr.New(t.MustTempDir("ns"), "")
		Expect(nsMgr.Initialize()).To(BeNil())
		cfg.SetNamespaceManager(nsMgr)

		// When
		Expect(sut.SetConfig(ctrConfig, sboxConfig)).To(BeNil())
		sut.Spec().ClearLinuxNamespaces()
		Expect(sut.SpecAddNamespaces(sb, targetCtr, cfg)).To(BeNil())
		defer Expect(sut.PidNamespace().Close()).To(BeNil())
		defer Expect(sut.PidNamespace().Remove()).To(BeNil())

		// Then
		spec := sut.Spec()
		Expect(len(spec.Config.Linux.Namespaces)).To(Equal(len(nsmgrtest.AllSpoofedNamespaces) + 1))

		found := false
		for _, specNs := range spec.Config.Linux.Namespaces {
			if specNs.Type == rspec.PIDNamespace {
				Expect(specNs.Path).To(Equal(sut.PidNamespace().Path()))
				found = true
			}
		}
		Expect(found).To(Equal(true))
	})
	It("should ignore if empty", func() {
		// Given
		ctrConfig := &types.ContainerConfig{
			Metadata: &types.ContainerMetadata{Name: "name"},
			Linux: &types.LinuxContainerConfig{
				SecurityContext: &types.LinuxContainerSecurityContext{
					NamespaceOptions: &types.NamespaceOption{
						Network: types.NamespaceMode_POD,
						Ipc:     types.NamespaceMode_POD,
						Pid:     types.NamespaceMode_CONTAINER,
					},
				},
			},
		}

		sboxConfig := &types.PodSandboxConfig{}
		sb := &sandbox.Sandbox{}
		sb.AddManagedNamespaces([]nsmgr.Namespace{&nsmgrtest.SpoofedNamespace{
			NsType:    nsmgr.IPCNS,
			EmptyPath: true,
		}})

		// When
		Expect(sut.SetConfig(ctrConfig, sboxConfig)).To(BeNil())
		sut.Spec().ClearLinuxNamespaces()
		Expect(sut.SpecAddNamespaces(sb, nil, nil)).To(BeNil())

		// Then
		spec := sut.Spec()
		Expect(len(spec.Config.Linux.Namespaces)).To(Equal(0))
	})
})
