package server_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// The actual test suite.
var _ = t.Describe("PodSandboxStatus", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("PodSandboxStatus", func() {
		It("should succeed with already stopped sandbox", func() {
			ctx := context.TODO()
			// Given
			addContainerAndSandbox()
			testSandbox.SetStopped(ctx, false)
			Expect(testSandbox.SetNetworkStopped(ctx, true)).To(Succeed())

			// When
			_, err := sut.StopPodSandbox(context.Background(),
				&types.StopPodSandboxRequest{PodSandboxId: testSandbox.ID()})

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should succeed with inavailable sandbox", func() {
			// Given
			// When
			_, err := sut.StopPodSandbox(context.Background(),
				&types.StopPodSandboxRequest{PodSandboxId: "invalid"})

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail with empty sandbox ID", func() {
			// Given
			// When
			_, err := sut.StopPodSandbox(context.Background(),
				&types.StopPodSandboxRequest{})

			// Then
			Expect(err).To(HaveOccurred())
		})
	})
})
