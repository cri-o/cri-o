package server_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runtime-spec/specs-go"
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

		It("should exec with a stale pid when the runtime reports alive", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetState(&oci.ContainerState{
				State:   specs.State{Status: oci.ContainerStateRunning},
				InitPid: 4194305,
			})

			ociRuntimeMock.EXPECT().
				ProbeMonitor(gomock.Any(), testContainer).
				Return(nil).
				AnyTimes()

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
