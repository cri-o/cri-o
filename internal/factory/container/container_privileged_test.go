package container_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

var _ = t.Describe("Container:Privileged", func() {
	It("should succeed in setting privileged flag", func() {
		// Given
		config := &types.ContainerConfig{
			Metadata: &types.ContainerMetadata{Name: "name"},
			Linux: &types.LinuxContainerConfig{
				SecurityContext: &types.LinuxContainerSecurityContext{
					Privileged: true,
				},
			},
		}

		sboxConfig := &types.PodSandboxConfig{
			Linux: &types.LinuxPodSandboxConfig{
				SecurityContext: &types.LinuxSandboxSecurityContext{
					Privileged: true,
				},
			},
		}

		// When
		Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

		// Then
		Expect(sut.SetPrivileged()).To(BeNil())
		Expect(sut.Privileged()).To(Equal(true))
	})
	It("should not be privileged if not set so", func() {
		// Given
		config := &types.ContainerConfig{
			Metadata: &types.ContainerMetadata{Name: "name"},
			Linux: &types.LinuxContainerConfig{
				SecurityContext: &types.LinuxContainerSecurityContext{
					Privileged: false,
				},
			},
		}

		sboxConfig := &types.PodSandboxConfig{
			Linux: &types.LinuxPodSandboxConfig{
				SecurityContext: &types.LinuxSandboxSecurityContext{
					Privileged: true,
				},
			},
		}

		// When
		Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

		// Then
		Expect(sut.SetPrivileged()).To(BeNil())
		Expect(sut.Privileged()).To(Equal(false))
	})
	It("should not be privileged if pod not set so", func() {
		// Given
		config := &types.ContainerConfig{
			Metadata: &types.ContainerMetadata{Name: "name"},
			Linux: &types.LinuxContainerConfig{
				SecurityContext: &types.LinuxContainerSecurityContext{
					Privileged: true,
				},
			},
		}

		sboxConfig := &types.PodSandboxConfig{
			Linux: &types.LinuxPodSandboxConfig{
				SecurityContext: &types.LinuxSandboxSecurityContext{
					Privileged: false,
				},
			},
		}

		// When
		Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

		// Then
		Expect(sut.SetPrivileged()).NotTo(BeNil())
		Expect(sut.Privileged()).To(Equal(false))
	})

	It("should not be privileged if pod has no linux config", func() {
		// Given
		config := &types.ContainerConfig{
			Metadata: &types.ContainerMetadata{Name: "name"},
			Linux: &types.LinuxContainerConfig{
				SecurityContext: &types.LinuxContainerSecurityContext{
					Privileged: true,
				},
			},
		}

		sboxConfig := &types.PodSandboxConfig{}

		// When
		Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

		// Then
		Expect(sut.SetPrivileged()).To(BeNil())
		Expect(sut.Privileged()).To(Equal(false))
	})
	It("should not be privileged if pod has no security context", func() {
		// Given
		config := &types.ContainerConfig{
			Metadata: &types.ContainerMetadata{Name: "name"},
			Linux: &types.LinuxContainerConfig{
				SecurityContext: &types.LinuxContainerSecurityContext{
					Privileged: true,
				},
			},
		}

		sboxConfig := &types.PodSandboxConfig{
			Linux: &types.LinuxPodSandboxConfig{},
		}

		// When
		Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

		// Then
		Expect(sut.SetPrivileged()).To(BeNil())
		Expect(sut.Privileged()).To(Equal(false))
	})
	It("should not be privileged if container has no linux config", func() {
		// Given
		config := &types.ContainerConfig{
			Metadata: &types.ContainerMetadata{Name: "name"},
		}

		sboxConfig := &types.PodSandboxConfig{
			Linux: &types.LinuxPodSandboxConfig{},
		}

		// When
		Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

		// Then
		Expect(sut.SetPrivileged()).To(BeNil())
		Expect(sut.Privileged()).To(Equal(false))
	})
	It("should not be privileged if container has no security context", func() {
		// Given
		config := &types.ContainerConfig{
			Metadata: &types.ContainerMetadata{Name: "name"},
		}

		sboxConfig := &types.PodSandboxConfig{
			Linux: &types.LinuxPodSandboxConfig{},
		}

		// When
		Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

		// Then
		Expect(sut.SetPrivileged()).To(BeNil())
		Expect(sut.Privileged()).To(Equal(false))
	})
})
