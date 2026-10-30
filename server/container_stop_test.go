package server_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/server/metrics"
)

// The actual test suite.
var _ = t.Describe("ContainerStop", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(func() {
		if sut != nil {
			sut.Runtime().SetRuntimeImplForContainer(testContainer, nil)
		}

		afterEach()
	})

	t.Describe("ContainerStop", func() {
		It("should succeed even if runtime DeleteContainer fails", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateStopped},
			})
			sut.Runtime().SetRuntimeImplForContainer(testContainer, ociRuntimeMock)
			ociRuntimeMock.EXPECT().ProbeMonitor(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().
					StopContainer(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil),
				runtimeServerMock.EXPECT().StopContainer(gomock.Any(), gomock.Any()).Return(nil),
				ociRuntimeMock.EXPECT().
					UpdateContainerStatus(gomock.Any(), gomock.Any()).
					Return(nil),
				ociRuntimeMock.EXPECT().DeleteContainer(gomock.Any(), gomock.Any()).
					Return(errors.New("boom")),
			)

			// When
			_, err := sut.StopContainer(context.Background(),
				&types.StopContainerRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).ToNot(HaveOccurred())
		})
		It("should succeed", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateStopped},
			})
			sut.Runtime().SetRuntimeImplForContainer(testContainer, ociRuntimeMock)
			ociRuntimeMock.EXPECT().ProbeMonitor(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().StopContainer(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil),
				runtimeServerMock.EXPECT().StopContainer(gomock.Any(), gomock.Any()).
					Return(nil),
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any(), gomock.Any()).
					Return(nil),
				ociRuntimeMock.EXPECT().DeleteContainer(gomock.Any(), gomock.Any()).
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
				ociRuntimeMock.EXPECT().
					DeleteContainer(gomock.Any(), testContainer).
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

		It("should retain the runtime implementation until container removal", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateStopped},
			})
			testSandbox.SetStopped(context.Background(), true)
			sut.Runtime().SetRuntimeImplForContainer(testContainer, ociRuntimeMock)
			ociRuntimeMock.EXPECT().ProbeMonitor(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().
					StopContainer(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil),
				runtimeServerMock.EXPECT().StopContainer(gomock.Any(), gomock.Any()).Return(nil),
				ociRuntimeMock.EXPECT().
					UpdateContainerStatus(gomock.Any(), gomock.Any()).
					Return(nil),
				ociRuntimeMock.EXPECT().DeleteContainer(gomock.Any(), gomock.Any()).Return(nil),
				ociRuntimeMock.EXPECT().DeleteContainer(gomock.Any(), gomock.Any()).Return(nil),
				runtimeServerMock.EXPECT().DeleteContainer(gomock.Any(), gomock.Any()).Return(nil),
			)

			// When
			_, err := sut.StopContainer(
				context.Background(),
				&types.StopContainerRequest{ContainerId: testContainer.ID()},
			)
			Expect(err).ToNot(HaveOccurred())
			_, err = sut.RemoveContainer(
				context.Background(),
				&types.RemoveContainerRequest{ContainerId: testContainer.ID()},
			)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should report the container as running until post-stop cleanup completed", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State:    specs.State{Status: oci.ContainerStateStopped},
				ExitCode: new(int32(0)),
			})
			sut.Runtime().SetRuntimeImplForContainer(testContainer, ociRuntimeMock)
			ociRuntimeMock.EXPECT().ProbeMonitor(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			runtimeServerMock.EXPECT().GetContainerMetadata(gomock.Any()).
				Return(storage.RuntimeContainerMetadata{}, nil).AnyTimes()
			statusRequest := &types.ContainerStatusRequest{ContainerId: testContainer.ID()}
			gomock.InOrder(
				ociRuntimeMock.EXPECT().
					StopContainer(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil),
				runtimeServerMock.EXPECT().StopContainer(gomock.Any(), gomock.Any()).Return(nil),
				ociRuntimeMock.EXPECT().
					UpdateContainerStatus(gomock.Any(), gomock.Any()).
					Return(nil),
				// Deleting the runtime container runs the OCI poststop hooks.
				ociRuntimeMock.EXPECT().DeleteContainer(gomock.Any(), gomock.Any()).
					DoAndReturn(func(context.Context, *oci.Container) error {
						defer GinkgoRecover()

						Expect(testContainer.CRIContainer().GetState()).
							To(Equal(types.ContainerState_CONTAINER_RUNNING))

						response, err := sut.ContainerStatus(context.Background(), statusRequest)
						Expect(err).ToNot(HaveOccurred())
						Expect(response.GetStatus().GetState()).
							To(Equal(types.ContainerState_CONTAINER_RUNNING))

						return nil
					}),
			)

			// When
			_, err := sut.StopContainer(
				context.Background(),
				&types.StopContainerRequest{ContainerId: testContainer.ID()},
			)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(testContainer.CRIContainer().GetState()).
				To(Equal(types.ContainerState_CONTAINER_EXITED))
			response, err := sut.ContainerStatus(context.Background(), statusRequest)
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetStatus().GetState()).To(Equal(types.ContainerState_CONTAINER_EXITED))
		})

		It("should run the post-stop cleanup only once", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State:    specs.State{Status: oci.ContainerStateStopped},
				ExitCode: new(int32(0)),
			})
			sut.Runtime().SetRuntimeImplForContainer(testContainer, ociRuntimeMock)
			ociRuntimeMock.EXPECT().ProbeMonitor(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			ociRuntimeMock.EXPECT().
				StopContainer(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(nil).
				Times(2)
			// The cleanup expectations allow a single call each.
			runtimeServerMock.EXPECT().StopContainer(gomock.Any(), gomock.Any()).Return(nil)
			ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any(), gomock.Any()).Return(nil)
			ociRuntimeMock.EXPECT().DeleteContainer(gomock.Any(), gomock.Any()).Return(nil)
			request := &types.StopContainerRequest{ContainerId: testContainer.ID()}

			// When
			_, err := sut.StopContainer(context.Background(), request)
			Expect(err).ToNot(HaveOccurred())
			_, err = sut.StopContainer(context.Background(), request)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(testContainer.CRIContainer().GetState()).
				To(Equal(types.ContainerState_CONTAINER_EXITED))
		})

		It("should keep the OOM count until the container is removed", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State:     specs.State{Status: oci.ContainerStateStopped},
				ExitCode:  new(int32(137)),
				OOMKilled: true,
			})
			testSandbox.SetStopped(context.Background(), true)
			sut.Runtime().SetRuntimeImplForContainer(testContainer, ociRuntimeMock)

			oomMetrics := metrics.Instance()
			oomMetrics.MetricContainersOOMCountTotalInc(testContainer.Name())
			DeferCleanup(func() {
				oomMetrics.MetricContainersOOMCountTotalDelete(testContainer.Name())
			})
			oomCount := func() float64 {
				count, recorded := oomMetrics.ContainersOOMCountTotal(testContainer.Name())
				Expect(recorded).To(BeTrue(), "OOM count not recorded")

				return count
			}

			ociRuntimeMock.EXPECT().ProbeMonitor(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			gomock.InOrder(
				ociRuntimeMock.EXPECT().
					StopContainer(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil),
				runtimeServerMock.EXPECT().StopContainer(gomock.Any(), gomock.Any()).Return(nil),
				ociRuntimeMock.EXPECT().
					UpdateContainerStatus(gomock.Any(), gomock.Any()).
					Return(nil),
				// Runtime container deletion on stop.
				ociRuntimeMock.EXPECT().DeleteContainer(gomock.Any(), gomock.Any()).Return(nil),
				// Failing final removal.
				ociRuntimeMock.EXPECT().DeleteContainer(gomock.Any(), gomock.Any()).
					Return(errors.New("boom")),
				// Successful final removal.
				ociRuntimeMock.EXPECT().DeleteContainer(gomock.Any(), gomock.Any()).Return(nil),
				runtimeServerMock.EXPECT().DeleteContainer(gomock.Any(), gomock.Any()).Return(nil),
			)
			removeRequest := &types.RemoveContainerRequest{ContainerId: testContainer.ID()}

			// When
			_, err := sut.StopContainer(
				context.Background(),
				&types.StopContainerRequest{ContainerId: testContainer.ID()},
			)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(oomCount()).To(BeEquivalentTo(1))

			// When
			_, err = sut.RemoveContainer(context.Background(), removeRequest)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(oomCount()).To(BeEquivalentTo(1))

			// When
			_, err = sut.RemoveContainer(context.Background(), removeRequest)

			// Then
			Expect(err).ToNot(HaveOccurred())
			_, recorded := oomMetrics.ContainersOOMCountTotal(testContainer.Name())
			Expect(recorded).To(BeFalse(), "OOM count still recorded after removal")
		})

		It("should skip runtime deletion when the implementation is absent", func() {
			// Given
			addContainerAndSandbox()
			sut.Runtime().SetRuntimeImplForContainer(testContainer, nil)

			// When
			err := sut.Runtime().DeleteRuntimeContainer(context.Background(), testContainer)

			// Then
			Expect(err).ToNot(HaveOccurred())
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
