package server_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runtime-spec/specs-go"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
)

// The actual test suite.
var _ = t.Describe("ContainerExecSync", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerExecSync", func() {
		It("should fail with invalid container ID", func() {
			// Given
			// When
			response, err := sut.ExecSync(context.Background(),
				&types.ExecSyncRequest{})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(response).To(BeNil())
		})

		It("should fail when the container is stopped", func() {
			// Given
			addContainerAndSandbox()

			// The server's background monitor may probe the container while
			// this test installs its runtime implementation.
			ociRuntimeMock.EXPECT().
				ProbeMonitor(gomock.Any(), testContainer).
				Return(nil).
				AnyTimes()
			ociRuntimeMock.EXPECT().
				IsContainerAlive(gomock.Any(), testContainer).
				Return(false, nil)
			sut.Runtime().SetRuntimeImpl(testContainer.ID(), ociRuntimeMock)

			// When
			response, err := sut.ExecSync(context.Background(),
				&types.ExecSyncRequest{
					ContainerId: testContainer.ID(),
					Cmd:         []string{"/bin/sh"},
				})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.NotFound))
			Expect(err.Error()).To(ContainSubstring("container is not created or running"))
			Expect(response).To(BeNil())
		})

		It("should report why the liveness check failed", func() {
			// Given
			addContainerAndSandbox()

			// The server's background monitor may probe the container while
			// this test installs its runtime implementation.
			ociRuntimeMock.EXPECT().
				ProbeMonitor(gomock.Any(), testContainer).
				Return(nil).
				AnyTimes()
			ociRuntimeMock.EXPECT().
				IsContainerAlive(gomock.Any(), testContainer).
				Return(false, errors.New("shim for container is not connected"))
			sut.Runtime().SetRuntimeImpl(testContainer.ID(), ociRuntimeMock)

			// When
			response, err := sut.ExecSync(context.Background(),
				&types.ExecSyncRequest{
					ContainerId: testContainer.ID(),
					Cmd:         []string{"/bin/sh"},
				})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.Unavailable))
			Expect(err.Error()).To(ContainSubstring("shim for container is not connected"))
			Expect(response).To(BeNil())
		})

		It("should exec with a stale pid when the runtime reports alive", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State:   specs.State{Status: oci.ContainerStateRunning},
				InitPid: 4194305,
			})

			// The server's background monitor may probe the container while
			// this test installs its runtime implementation.
			ociRuntimeMock.EXPECT().
				ProbeMonitor(gomock.Any(), testContainer).
				Return(nil).
				AnyTimes()

			// The liveness check has to be bounded, so that a runtime which
			// does not answer cannot hold the request open.
			ociRuntimeMock.EXPECT().
				IsContainerAlive(gomock.Any(), testContainer).
				DoAndReturn(func(ctx context.Context, _ *oci.Container) (bool, error) {
					_, hasDeadline := ctx.Deadline()
					Expect(hasDeadline).To(BeTrue())

					return true, nil
				})
			ociRuntimeMock.EXPECT().
				ExecSyncContainer(gomock.Any(), testContainer, []string{"/bin/sh"}, int64(0)).
				DoAndReturn(func(ctx context.Context, _ *oci.Container, _ []string, _ int64) (*types.ExecSyncResponse, error) {
					_, hasDeadline := ctx.Deadline()
					Expect(hasDeadline).To(BeFalse())

					return &types.ExecSyncResponse{}, nil
				})
			sut.Runtime().SetRuntimeImpl(testContainer.ID(), ociRuntimeMock)

			// When
			response, err := sut.ExecSync(context.Background(),
				&types.ExecSyncRequest{
					ContainerId: testContainer.ID(),
					Cmd:         []string{"/bin/sh"},
				})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response).ToNot(BeNil())
		})
	})
})
