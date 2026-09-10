package server_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	graphdriver "go.podman.io/storage/drivers"
	"go.uber.org/mock/gomock"
	"golang.org/x/sys/unix"

	"github.com/cri-o/cri-o/server"
)

var _ = t.Describe("Dedup", func() {
	// Prepare the sut
	BeforeEach(beforeEach)
	AfterEach(afterEach)

	t.Describe("RunDedup", func() {
		It("should succeed when dedup saves bytes", func() {
			// Given
			storeMock.EXPECT().Dedup(gomock.Any()).
				Return(graphdriver.DedupResult{Deduped: 1024}, nil)

			// When
			err := server.RunDedup(context.Background(), storeMock, false)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should succeed when dedup has no savings", func() {
			// Given
			storeMock.EXPECT().Dedup(gomock.Any()).
				Return(graphdriver.DedupResult{Deduped: 0}, nil)

			// When
			err := server.RunDedup(context.Background(), storeMock, false)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return error on ENOTSUP", func() {
			// Given
			storeMock.EXPECT().Dedup(gomock.Any()).
				Return(graphdriver.DedupResult{}, unix.ENOTSUP)

			// When
			err := server.RunDedup(context.Background(), storeMock, false)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not supported"))
		})

		It("should return error on EOPNOTSUPP", func() {
			// Given
			storeMock.EXPECT().Dedup(gomock.Any()).
				Return(graphdriver.DedupResult{}, unix.EOPNOTSUPP)

			// When
			err := server.RunDedup(context.Background(), storeMock, false)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not supported"))
		})

		It("should return error on other failures", func() {
			// Given
			storeMock.EXPECT().Dedup(gomock.Any()).
				Return(graphdriver.DedupResult{}, t.TestError)

			// When
			err := server.RunDedup(context.Background(), storeMock, false)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed"))
		})
	})
})
