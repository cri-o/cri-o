package server_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// The actual test suite
var _ = t.Describe("Server", func() {
	// Prepare the sut
	BeforeEach(beforeEach)
	AfterEach(afterEach)

	t.Describe("ReservePodIDAndName", func() {
		It("should succeed", func() {
			// Given
			// When
			id, name, err := sut.ReservePodIDAndName(
				&pb.PodSandboxConfig{
					Metadata: &pb.PodSandboxMetadata{
						Namespace: "default",
					},
				})

			// Then
			Expect(err).To(BeNil())
			Expect(id).NotTo(BeEmpty())
			Expect(name).NotTo(BeEmpty())
		})

		It("should fail without metadata", func() {
			// Given
			// When
			id, name, err := sut.ReservePodIDAndName(nil)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(id).To(BeEmpty())
			Expect(name).To(BeEmpty())
		})

		It("should fail without sandbox config", func() {
			// Given
			// When
			id, name, err := sut.ReservePodIDAndName(&pb.PodSandboxConfig{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(id).To(BeEmpty())
			Expect(name).To(BeEmpty())
		})

		It("should fail without namespace", func() {
			// Given
			// When
			id, name, err := sut.ReservePodIDAndName(
				&pb.PodSandboxConfig{Metadata: &pb.PodSandboxMetadata{}})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(id).To(BeEmpty())
			Expect(name).To(BeEmpty())
		})

		It("should fail if name is already reserved", func() {
			// Given
			metadata := &pb.PodSandboxMetadata{
				Namespace: "default",
				Name:      "name",
			}
			_, _, err := sut.ReservePodIDAndName(
				&pb.PodSandboxConfig{Metadata: metadata})
			Expect(err).To(BeNil())

			// When
			id, name, err := sut.ReservePodIDAndName(
				&pb.PodSandboxConfig{Metadata: metadata})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(id).To(BeEmpty())
			Expect(name).To(BeEmpty())
		})
	})

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
