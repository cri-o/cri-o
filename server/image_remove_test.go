package server_test

import (
	"context"

	"github.com/cri-o/cri-o/internal/storage"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// The actual test suite
var _ = t.Describe("ImageRemove", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})
	AfterEach(afterEach)

	t.Describe("ImageRemove", func() {
		It("should succeed", func() {
			// Given
			gomock.InOrder(
				multiStoreServerMock.EXPECT().ResolveNames(
					gomock.Any(), gomock.Any()).
					Return([]string{"image"}, nil),
				multiStoreServerMock.EXPECT().GetImageServerForImage(gomock.Any()).Return([]storage.ImageServer{imageServerMock}, nil),
				imageServerMock.EXPECT().UntagImage(gomock.Any(),
					gomock.Any()).Return(nil),
			)
			// When
			err := sut.RemoveImage(context.Background(),
				&types.RemoveImageRequest{Image: &types.ImageSpec{Image: "image"}})

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed when image id cannot be parsed", func() {
			// Given
			gomock.InOrder(
				multiStoreServerMock.EXPECT().ResolveNames(
					gomock.Any(), gomock.Any()).
					Return(nil, storage.ErrCannotParseImageID),
				multiStoreServerMock.EXPECT().GetImageServerForImage(gomock.Any()).Return([]storage.ImageServer{imageServerMock}, nil),
				imageServerMock.EXPECT().UntagImage(gomock.Any(),
					gomock.Any()).Return(nil),
			)
			// When
			err := sut.RemoveImage(context.Background(),
				&types.RemoveImageRequest{Image: &types.ImageSpec{Image: "image"}})

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail when image untag errors", func() {
			// Given
			gomock.InOrder(
				multiStoreServerMock.EXPECT().ResolveNames(
					gomock.Any(), gomock.Any()).
					Return([]string{"image"}, nil),
				multiStoreServerMock.EXPECT().GetImageServerForImage(gomock.Any()).Return([]storage.ImageServer{imageServerMock}, nil),
				imageServerMock.EXPECT().UntagImage(gomock.Any(),
					gomock.Any()).Return(t.TestError),
			)
			// When
			err := sut.RemoveImage(context.Background(),
				&types.RemoveImageRequest{Image: &types.ImageSpec{Image: "image"}})

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail when name resolving errors", func() {
			// Given
			gomock.InOrder(
				multiStoreServerMock.EXPECT().ResolveNames(
					gomock.Any(), gomock.Any()).
					Return(nil, t.TestError),
			)
			// When
			err := sut.RemoveImage(context.Background(),
				&types.RemoveImageRequest{Image: &types.ImageSpec{Image: "image"}})

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail without specified image", func() {
			// Given
			// When
			err := sut.RemoveImage(context.Background(),
				&types.RemoveImageRequest{Image: &types.ImageSpec{Image: ""}})

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
