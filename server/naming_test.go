package server_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// The actual test suite.
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
				&types.PodSandboxConfig{
					Metadata: &types.PodSandboxMetadata{
						Namespace: "default",
					},
				})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(name).NotTo(BeEmpty())
		})

		It("should fail without metadata", func() {
			// Given
			// When
			name, err := sut.ReserveSandboxContainerIDAndName(nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(name).To(BeEmpty())
		})

		It("should fail without sandbox config", func() {
			// Given
			// When
			name, err := sut.ReserveSandboxContainerIDAndName(&types.PodSandboxConfig{})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(name).To(BeEmpty())
		})

		It("should fail if name is already reserved", func() {
			// Given
			metadata := &types.PodSandboxMetadata{
				Namespace: "default",
				Name:      "name",
			}
			_, err := sut.ReserveSandboxContainerIDAndName(
				&types.PodSandboxConfig{Metadata: metadata})
			Expect(err).ToNot(HaveOccurred())

			// When
			name, err := sut.ReserveSandboxContainerIDAndName(
				&types.PodSandboxConfig{Metadata: metadata})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(name).To(BeEmpty())
		})
	})
})
