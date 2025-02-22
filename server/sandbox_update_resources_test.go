package server_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// The actual test suite.
var _ = t.Describe("UpdatePodSandbox", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("UpdatePodSandbox", func() {
		It("should succeed ", func() {
			// Given
			addContainerAndSandbox()

			// When
			_, err := sut.UpdatePodSandboxResources(context.Background(),
				&types.UpdatePodSandboxResourcesRequest{PodSandboxId: testSandbox.ID()})

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should error with unavailable sandbox", func() {
			// Given
			// When
			_, err := sut.UpdatePodSandboxResources(context.Background(),
				&types.UpdatePodSandboxResourcesRequest{PodSandboxId: "invalid"})

			// Then
			Expect(err).To(HaveOccurred())
		})
	})
})
