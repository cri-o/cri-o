package server_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// The actual test suite.
var _ = t.Describe("ReopenContainerLog", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ReopenContainerLog", func() {
		It("should use an existing live runtime", func() {
			addContainerAndSandbox()
			ociRuntimeMock.EXPECT().ProbeMonitor(gomock.Any(), testContainer).Return(nil).AnyTimes()
			ociRuntimeMock.EXPECT().IsContainerAlive(gomock.Any(), testContainer).Return(true, nil)
			ociRuntimeMock.EXPECT().ReopenContainerLog(gomock.Any(), testContainer).Return(nil)
			sut.Runtime().SetRuntimeImpl(testContainer.ID(), ociRuntimeMock)

			_, err := sut.ReopenContainerLog(context.Background(), &types.ReopenContainerLogRequest{
				ContainerId: testContainer.ID(),
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not create a runtime for log reopening", func() {
			addContainerAndSandbox()

			_, err := sut.ReopenContainerLog(context.Background(), &types.ReopenContainerLogRequest{
				ContainerId: testContainer.ID(),
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no runtime implementation"))
			Expect(sut.Runtime().HasRuntimeImpl(testContainer.ID())).To(BeFalse())
		})

		It("should fail on invalid container ID", func() {
			// Given
			// When
			_, err := sut.ReopenContainerLog(
				context.Background(),
				&types.ReopenContainerLogRequest{},
			)

			// Then
			Expect(err).To(HaveOccurred())
		})
	})
})
