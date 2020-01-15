package server_test

import (
	"context"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// The actual test suite
var _ = t.Describe("RemovePodSandbox", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		mockRuncInLibConfig()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("RemovePodSandbox", func() {
		It("should succeed when sandbox stop errors", func() {
			// Given
			addContainerAndSandbox()
			gomock.InOrder(
				runtimeServerMock.EXPECT().StopContainer(gomock.Any()).
					Return(t.TestError),
				runtimeServerMock.EXPECT().RemovePodSandbox(gomock.Any()).
					Return(nil),
			)

			// When
			response, err := sut.RemovePodSandbox(context.Background(),
				&pb.RemovePodSandboxRequest{PodSandboxId: testSandbox.ID()})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should fail when sandbox removal errors", func() {
			// Given
			addContainerAndSandbox()
			gomock.InOrder(
				runtimeServerMock.EXPECT().StopContainer(gomock.Any()).
					Return(nil),
				runtimeServerMock.EXPECT().RemovePodSandbox(gomock.Any()).
					Return(t.TestError),
			)

			// When
			response, err := sut.RemovePodSandbox(context.Background(),
				&pb.RemovePodSandboxRequest{PodSandboxId: testSandbox.ID()})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when infra container index removal errors", func() {
			// Given
			Expect(sut.AddSandbox(testSandbox)).To(BeNil())
			Expect(testSandbox.SetInfraContainer(testContainer)).To(BeNil())
			Expect(sut.PodIDIndex().Add(testSandbox.ID())).To(BeNil())

			gomock.InOrder(
				runtimeServerMock.EXPECT().StopContainer(gomock.Any()).
					Return(nil),
				runtimeServerMock.EXPECT().RemovePodSandbox(gomock.Any()).
					Return(nil),
			)

			// When
			response, err := sut.RemovePodSandbox(context.Background(),
				&pb.RemovePodSandboxRequest{PodSandboxId: testSandbox.ID()})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should succeed with not existing sandbox", func() {
			// Given
			// When
			response, err := sut.RemovePodSandbox(context.Background(),
				&pb.RemovePodSandboxRequest{PodSandboxId: testSandbox.ID()})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should fail when sandbox not found", func() {
			// Given
			Expect(sut.PodIDIndex().Add(testSandbox.ID())).To(BeNil())

			// When
			response, err := sut.RemovePodSandbox(context.Background(),
				&pb.RemovePodSandboxRequest{PodSandboxId: testSandbox.ID()})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should fail with empty sandbox ID", func() {
			// Given
			// When
			response, err := sut.RemovePodSandbox(context.Background(),
				&pb.RemovePodSandboxRequest{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})
})
