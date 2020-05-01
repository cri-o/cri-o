package server_test

import (
	"context"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// The actual test suite
var _ = t.Describe("PodSandboxStatus", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("PodSandboxStatus", func() {
		It("should succeed with already stopped sandbox", func() {
			// Given
			addContainerAndSandbox()
			testSandbox.SetStopped(false)
			Expect(testSandbox.SetNetworkStopped(false)).To(BeNil())

			// When
			response, err := sut.StopPodSandbox(context.Background(),
				&pb.StopPodSandboxRequest{PodSandboxId: testSandbox.ID()})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should succeed with inavailable sandbox", func() {
			// Given
			// When
			response, err := sut.StopPodSandbox(context.Background(),
				&pb.StopPodSandboxRequest{PodSandboxId: "invalid"})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should fail when container is not stopped", func() {
			// Given
			addContainerAndSandbox()
			gomock.InOrder(
				cniPluginMock.EXPECT().GetDefaultNetworkName().Return(""),
				cniPluginMock.EXPECT().TearDownPodWithContext(gomock.Any(), gomock.Any()).Return(t.TestError),
			)

			// When
			response, err := sut.StopPodSandbox(context.Background(),
				&pb.StopPodSandboxRequest{PodSandboxId: testSandbox.ID()})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail with empty sandbox ID", func() {
			// Given
			// When
			response, err := sut.StopPodSandbox(context.Background(),
				&pb.StopPodSandboxRequest{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})
})
