package server_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
)

// The actual test suite.
var _ = t.Describe("ContainerStop", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerStop", func() {
		It("should succeed", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateStopped},
			})
			gomock.InOrder(
				runtimeServerMock.EXPECT().StopContainer(gomock.Any(), gomock.Any()).
					Return(nil),
			)

			// When
			_, err := sut.StopContainer(context.Background(),
				&types.StopContainerRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		DescribeTable("should skip post-stop cleanup after an interrupted stop and allow retry",
			func(stopErr error, code codes.Code) {
				// Given
				addContainerAndSandbox()

				defer sut.ContainerServer.RemoveContainer(context.Background(), testContainer)

				// The server's background monitor may probe the container while
				// this test installs its runtime implementation.
				ociRuntimeMock.EXPECT().
					ProbeMonitor(gomock.Any(), testContainer).
					Return(nil).
					AnyTimes()
				sut.Runtime().SetRuntimeImpl(testContainer.ID(), ociRuntimeMock)
				ociRuntimeMock.EXPECT().
					StopContainer(gomock.Any(), testContainer, int64(30)).
					Return(stopErr)

				request := &types.StopContainerRequest{ContainerId: testContainer.ID(), Timeout: 30}

				// When
				response, err := sut.StopContainer(context.Background(), request)

				// Then - no storage unmount or state write may happen for a
				// container that is still running (no expectations are set for
				// them), and the caller sees the matching status code.
				Expect(response).To(BeNil())
				Expect(status.Code(err)).To(Equal(code))

				// A retry after the container stopped completes the cleanup.
				testContainer.SetState(&oci.ContainerState{
					State: specs.State{Status: oci.ContainerStateStopped},
				})
				ociRuntimeMock.EXPECT().
					StopContainer(gomock.Any(), testContainer, int64(30)).
					Return(nil)
				ociRuntimeMock.EXPECT().
					UpdateContainerStatus(gomock.Any(), testContainer).
					Return(nil).
					AnyTimes()
				runtimeServerMock.EXPECT().
					StopContainer(gomock.Any(), testContainer.ID()).
					Return(nil)

				response, err = sut.StopContainer(context.Background(), request)
				Expect(err).NotTo(HaveOccurred())
				Expect(response).NotTo(BeNil())
			},
			Entry("cancellation", context.Canceled, codes.Canceled),
			Entry("deadline", context.DeadlineExceeded, codes.DeadlineExceeded),
		)

		It("should succeed with not existing container ID", func() {
			// Given
			// When
			_, err := sut.StopContainer(context.Background(),
				&types.StopContainerRequest{ContainerId: "id"})

			// Then
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
