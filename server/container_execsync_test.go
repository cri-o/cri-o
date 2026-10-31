package server_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
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
				Return(false)
			sut.Runtime().SetRuntimeImpl(testContainer.ID(), ociRuntimeMock)

			// When
			response, err := sut.ExecSync(context.Background(),
				&types.ExecSyncRequest{
					ContainerId: testContainer.ID(),
					Cmd:         []string{"/bin/sh"},
				})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("container is not created or running"))
			Expect(response).To(BeNil())
		})

		It("should attempt exec when the runtime reports the container alive", func() {
			// Given
			addContainerAndSandbox()

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
				DoAndReturn(func(ctx context.Context, _ *oci.Container) bool {
					_, hasDeadline := ctx.Deadline()
					Expect(hasDeadline).To(BeTrue())

					return true
				})
			ociRuntimeMock.EXPECT().
				ExecSyncContainer(gomock.Any(), testContainer, []string{"/bin/sh"}, int64(0)).
				Return(&types.ExecSyncResponse{}, nil)
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
