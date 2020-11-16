package container_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

var _ = t.Describe("Container:Privileged", func() {
	It("should succeed in setting privileged flag", func() {
		// Given
		config := &pb.ContainerConfig{
			Metadata: &pb.ContainerMetadata{Name: "name"},
			Linux: &pb.LinuxContainerConfig{
				SecurityContext: &pb.LinuxContainerSecurityContext{
					Privileged: true,
				},
			},
		}

		sboxConfig := &pb.PodSandboxConfig{
			Linux: &pb.LinuxPodSandboxConfig{
				SecurityContext: &pb.LinuxSandboxSecurityContext{
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
		config := &pb.ContainerConfig{
			Metadata: &pb.ContainerMetadata{Name: "name"},
			Linux: &pb.LinuxContainerConfig{
				SecurityContext: &pb.LinuxContainerSecurityContext{
					Privileged: false,
				},
			},
		}

		sboxConfig := &pb.PodSandboxConfig{
			Linux: &pb.LinuxPodSandboxConfig{
				SecurityContext: &pb.LinuxSandboxSecurityContext{
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
		config := &pb.ContainerConfig{
			Metadata: &pb.ContainerMetadata{Name: "name"},
			Linux: &pb.LinuxContainerConfig{
				SecurityContext: &pb.LinuxContainerSecurityContext{
					Privileged: true,
				},
			},
		}

		sboxConfig := &pb.PodSandboxConfig{
			Linux: &pb.LinuxPodSandboxConfig{
				SecurityContext: &pb.LinuxSandboxSecurityContext{
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
		config := &pb.ContainerConfig{
			Metadata: &pb.ContainerMetadata{Name: "name"},
			Linux: &pb.LinuxContainerConfig{
				SecurityContext: &pb.LinuxContainerSecurityContext{
					Privileged: true,
				},
			},
		}

		sboxConfig := &pb.PodSandboxConfig{}

		// When
		Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

		// Then
		Expect(sut.SetPrivileged()).To(BeNil())
		Expect(sut.Privileged()).To(Equal(false))
	})
	It("should not be privileged if pod has no security context", func() {
		// Given
		config := &pb.ContainerConfig{
			Metadata: &pb.ContainerMetadata{Name: "name"},
			Linux: &pb.LinuxContainerConfig{
				SecurityContext: &pb.LinuxContainerSecurityContext{
					Privileged: true,
				},
			},
		}

		sboxConfig := &pb.PodSandboxConfig{
			Linux: &pb.LinuxPodSandboxConfig{},
		}

		// When
		Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

		// Then
		Expect(sut.SetPrivileged()).To(BeNil())
		Expect(sut.Privileged()).To(Equal(false))
	})
	It("should not be privileged if container has no linux config", func() {
		// Given
		config := &pb.ContainerConfig{
			Metadata: &pb.ContainerMetadata{Name: "name"},
		}

		sboxConfig := &pb.PodSandboxConfig{
			Linux: &pb.LinuxPodSandboxConfig{},
		}

		// When
		Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

		// Then
		Expect(sut.SetPrivileged()).To(BeNil())
		Expect(sut.Privileged()).To(Equal(false))
	})
	It("should not be privileged if container has no security context", func() {
		// Given
		config := &pb.ContainerConfig{
			Metadata: &pb.ContainerMetadata{Name: "name"},
		}

		sboxConfig := &pb.PodSandboxConfig{
			Linux: &pb.LinuxPodSandboxConfig{},
		}

		// When
		Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())

		// Then
		Expect(sut.SetPrivileged()).To(BeNil())
		Expect(sut.Privileged()).To(Equal(false))
	})
})
