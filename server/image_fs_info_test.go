package server_test

import (
	"context"
	"os"

	cstorage "github.com/containers/storage"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("ImageFsInfo", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})
	AfterEach(afterEach)

	t.Describe("ImageFsInfo", func() {
		It("should succeed", func() {
			// Given
			gomock.InOrder(
				multiStoreServerMock.EXPECT().GetAllStores().Return([]cstorage.Store{storeMock}),
				storeMock.EXPECT().GraphRoot().Return(""),
				storeMock.EXPECT().GraphDriverName().Return("test"),
			)
			testImageDir := "test-images"
			Expect(os.MkdirAll(testImageDir, 0o755)).To(BeNil())
			defer os.RemoveAll(testImageDir)

			// When
			response, err := sut.ImageFsInfo(context.Background())

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
			Expect(len(response.ImageFilesystems)).To(BeEquivalentTo(1))
		})

		It("should fail on invalid image dir", func() {
			// Given
			gomock.InOrder(
				multiStoreServerMock.EXPECT().GetAllStores().Return([]cstorage.Store{storeMock}),
				storeMock.EXPECT().GraphRoot().Return(""),
				storeMock.EXPECT().GraphDriverName().Return(""),
			)

			// When
			response, err := sut.ImageFsInfo(context.Background())

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})
})
