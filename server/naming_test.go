package server

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// The actual test suite
var _ = t.Describe("Server", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})
	AfterEach(afterEach)

	t.Describe("ReserveSandboxContainerIDAndName", func() {
		It("should succeed", func() {
			// Given
			// When
			name, err := sut.ReserveSandboxContainerIDAndName(
				&pb.PodSandboxConfig{
					Metadata: &pb.PodSandboxMetadata{
						Namespace: "default",
					},
				})

			// Then
			Expect(err).To(BeNil())
			Expect(name).NotTo(BeEmpty())
		})

		It("should fail without metadata", func() {
			// Given
			// When
			name, err := sut.ReserveSandboxContainerIDAndName(nil)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(name).To(BeEmpty())
		})

		It("should fail without sandbox config", func() {
			// Given
			// When
			name, err := sut.ReserveSandboxContainerIDAndName(&pb.PodSandboxConfig{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(name).To(BeEmpty())
		})

		It("should fail if name is already reserved", func() {
			// Given
			metadata := &pb.PodSandboxMetadata{
				Namespace: "default",
				Name:      "name",
			}
			_, err := sut.ReserveSandboxContainerIDAndName(
				&pb.PodSandboxConfig{Metadata: metadata})
			Expect(err).To(BeNil())

			// When
			name, err := sut.ReserveSandboxContainerIDAndName(
				&pb.PodSandboxConfig{Metadata: metadata})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(name).To(BeEmpty())
		})
	})
})
