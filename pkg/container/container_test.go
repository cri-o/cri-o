package container_test

import (
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("Container", func() {
	t.Describe("SetConfig", func() {
		It("should succeed", func() {
			// Given
			config := &pb.ContainerConfig{
				Metadata: &pb.ContainerMetadata{Name: "name"},
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
			config := &pb.ContainerConfig{}

			// When
			err := sut.SetConfig(config)

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
			err := sut.SetConfig(config)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(sut.Config()).To(BeNil())
		})

		It("should fail with already set config", func() {
			// Given
			config := &pb.ContainerConfig{
				Metadata: &pb.ContainerMetadata{Name: "name"},
			}
			err := sut.SetConfig(config)
			Expect(err).To(BeNil())

			// When
			err = sut.SetConfig(config)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(sut.Config()).To(Equal(config))
		})
	})
})
