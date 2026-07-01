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
			err := server.RunDedup(context.Background(), storeMock)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should succeed when dedup has no savings", func() {
			// Given
			storeMock.EXPECT().Dedup(gomock.Any()).
				Return(graphdriver.DedupResult{Deduped: 0}, nil)

			// When
			err := server.RunDedup(context.Background(), storeMock)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return error on ENOTSUP", func() {
			// Given
			storeMock.EXPECT().Dedup(gomock.Any()).
				Return(graphdriver.DedupResult{}, unix.ENOTSUP)

			// When
			err := server.RunDedup(context.Background(), storeMock)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not supported"))
		})

		It("should return error on EOPNOTSUPP", func() {
			// Given
			storeMock.EXPECT().Dedup(gomock.Any()).
				Return(graphdriver.DedupResult{}, unix.EOPNOTSUPP)

			// When
			err := server.RunDedup(context.Background(), storeMock)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not supported"))
		})

		It("should return error on other failures", func() {
			// Given
			storeMock.EXPECT().Dedup(gomock.Any()).
				Return(graphdriver.DedupResult{}, t.TestError)

			// When
			err := server.RunDedup(context.Background(), storeMock)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed"))
		})
	})

	t.Describe("Server with dedup enabled", func() {
		It("should succeed creating server with dedup enabled", func() {
			// Given
			serverConfig.EnableStorageDedup = true

			graphroot := t.MustTempDir("graphroot")
			gomock.InOrder(
				cniPluginMock.EXPECT().Status().Return(nil),
				libMock.EXPECT().GetData().Times(2).Return(serverConfig),
				libMock.EXPECT().GetStore().Return(storeMock, nil),
				libMock.EXPECT().GetData().Return(serverConfig),
				storeMock.EXPECT().GraphRoot().Return(graphroot),
				storeMock.EXPECT().Containers().Return(nil, nil),
				cniPluginMock.EXPECT().GC(gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes(),
			)
			Expect(serverConfig.SetCNIPlugin(cniPluginMock)).To(Succeed())

			// Expect dedup to be called for startup trigger
			storeMock.EXPECT().Dedup(gomock.Any()).
				Return(graphdriver.DedupResult{Deduped: 0}, nil).
				AnyTimes()

			// When
			srv, err := server.New(context.Background(), libMock)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(srv).NotTo(BeNil())
		})
	})
})
