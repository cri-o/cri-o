package server_test

import (
	"context"
	"time"

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
			func(endRequest func() context.Context, stopErr error, code codes.Code) {
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

				// When - the request itself ends before the container stops.
				response, err := sut.StopContainer(endRequest(), request)

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
			Entry("cancellation", canceledContext, context.Canceled, codes.Canceled),
			Entry("deadline", expiredContext, context.DeadlineExceeded, codes.DeadlineExceeded),
		)

		It("should not report a runtime context error as an interrupted request", func() {
			// Given - a live request whose runtime fails with a context error
			// of its own. The VM runtime does this when it relays a shim
			// deadline back through errdefs.FromGRPC, and that is a runtime
			// failure rather than a request that ended.
			addContainerAndSandbox()

			defer sut.ContainerServer.RemoveContainer(context.Background(), testContainer)

			ociRuntimeMock.EXPECT().
				ProbeMonitor(gomock.Any(), testContainer).
				Return(nil).
				AnyTimes()
			sut.Runtime().SetRuntimeImpl(testContainer.ID(), ociRuntimeMock)
			ociRuntimeMock.EXPECT().
				StopContainer(gomock.Any(), testContainer, int64(30)).
				Return(context.DeadlineExceeded)

			// When
			response, err := sut.StopContainer(context.Background(),
				&types.StopContainerRequest{ContainerId: testContainer.ID(), Timeout: 30})

			// Then - the caller must not be told its request timed out, and no
			// post-stop cleanup may run (no expectations are set for it).
			Expect(response).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).NotTo(Equal(codes.DeadlineExceeded))
			Expect(status.Code(err)).NotTo(Equal(codes.Canceled))
		})

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

// canceledContext returns a request context that the caller already canceled.
func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	return ctx
}

// expiredContext returns a request context whose deadline has already passed.
func expiredContext() context.Context {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	DeferCleanup(cancel)

	return ctx
}
