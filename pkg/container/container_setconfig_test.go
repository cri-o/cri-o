package container_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// The actual test suite
var _ = t.Describe("Container:SetConfig", func() {
	It("should succeed", func() {
		// Given
		config := &pb.ContainerConfig{
			Metadata: &pb.ContainerMetadata{Name: "name"},
		}

		sboxConfig := &pb.PodSandboxConfig{}

		// When
		err := sut.SetConfig(config, sboxConfig)

		// Then
		Expect(err).To(BeNil())
		Expect(sut.Config()).To(Equal(config))
		Expect(sut.SandboxConfig()).To(Equal(sboxConfig))
	})

	It("should fail with nil config", func() {
		// Given
		// When
		err := sut.SetConfig(nil, nil)

		// Then
		Expect(err).NotTo(BeNil())
		Expect(sut.Config()).To(BeNil())
	})

	It("should fail with empty config", func() {
		// Given
		config := &pb.ContainerConfig{}

		// When
		err := sut.SetConfig(config, nil)

		// Then
		Expect(err).NotTo(BeNil())
		Expect(sut.Config()).To(BeNil())
	})

	It("should fail with empty name", func() {
		// Given
		config := &pb.ContainerConfig{
			Metadata: &pb.ContainerMetadata{},
		}

		// When
		err := sut.SetConfig(config, nil)

		// Then
		Expect(err).NotTo(BeNil())
		Expect(sut.Config()).To(BeNil())
	})

	It("should fail with already set config", func() {
		// Given
		config := &pb.ContainerConfig{
			Metadata: &pb.ContainerMetadata{Name: "name"},
		}
		sboxConfig := &pb.PodSandboxConfig{}
		err := sut.SetConfig(config, sboxConfig)
		Expect(err).To(BeNil())

		// When
		err = sut.SetConfig(config, nil)

		// Then
		Expect(err).NotTo(BeNil())
		Expect(sut.Config()).To(Equal(config))
	})

	It("should fail with empty sandbox config", func() {
		// Given
		config := &pb.ContainerConfig{
			Metadata: &pb.ContainerMetadata{Name: "name"},
		}

		// Then
		err := sut.SetConfig(config, nil)
		Expect(err).NotTo(BeNil())
	})
})
