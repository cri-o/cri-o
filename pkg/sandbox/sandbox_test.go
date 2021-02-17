package sandbox_test

import (
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Sandbox", func() {
	t.Describe("SetConfig", func() {
		It("should succeed", func() {
			// Given
			config := &pb.PodSandboxConfig{
				Metadata: &pb.PodSandboxMetadata{Name: "name"},
			}

			// When
			err := sut.SetConfig(config)

			// Then
			Expect(err).To(BeNil())
			Expect(sut.Config()).To(Equal(config))
		})

		It("should fail with nil config", func() {
			// Given
			// When
			err := sut.SetConfig(nil)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(sut.Config()).To(BeNil())
		})

		It("should fail with empty config", func() {
			// Given
			config := &pb.PodSandboxConfig{}

			// When
			err := sut.SetConfig(config)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(sut.Config()).To(BeNil())
		})

		It("should fail with an empty name", func() {
			// Given
			config := &pb.PodSandboxConfig{
				Metadata: &pb.PodSandboxMetadata{},
			}

			// When
			err := sut.SetConfig(config)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(sut.Config()).To(BeNil())
		})

		It("should fail with config already set", func() {
			// Given
			config := &pb.PodSandboxConfig{
				Metadata: &pb.PodSandboxMetadata{Name: "name"},
			}
			err := sut.SetConfig(config)
			Expect(err).To(BeNil())

			// When
			err = sut.SetConfig(config)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(sut.Config()).NotTo(BeNil())
		})
	})
})
