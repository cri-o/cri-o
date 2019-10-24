package server_test

import (
	"context"

	"github.com/cri-o/cri-o/internal/pkg/storage"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
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
				imageServerMock.EXPECT().ResolveNames(
					gomock.Any(), gomock.Any()).
					Return([]string{"image"}, nil),
				imageServerMock.EXPECT().UntagImage(gomock.Any(),
					gomock.Any()).Return(nil),
			)
			// When
			response, err := sut.RemoveImage(context.Background(),
				&pb.RemoveImageRequest{Image: &pb.ImageSpec{Image: "image"}})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should succeed when image id cannot be parsed", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().ResolveNames(
					gomock.Any(), gomock.Any()).
					Return(nil, storage.ErrCannotParseImageID),
				imageServerMock.EXPECT().UntagImage(gomock.Any(),
					gomock.Any()).Return(nil),
			)
			// When
			response, err := sut.RemoveImage(context.Background(),
				&pb.RemoveImageRequest{Image: &pb.ImageSpec{Image: "image"}})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should fail when image untag errors", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().ResolveNames(
					gomock.Any(), gomock.Any()).
					Return([]string{"image"}, nil),
				imageServerMock.EXPECT().UntagImage(gomock.Any(),
					gomock.Any()).Return(t.TestError),
			)
			// When
			response, err := sut.RemoveImage(context.Background(),
				&pb.RemoveImageRequest{Image: &pb.ImageSpec{Image: "image"}})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when name resolving errors", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().ResolveNames(
					gomock.Any(), gomock.Any()).
					Return(nil, t.TestError),
			)
			// When
			response, err := sut.RemoveImage(context.Background(),
				&pb.RemoveImageRequest{Image: &pb.ImageSpec{Image: "image"}})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail without specified image", func() {
			// Given
			// When
			response, err := sut.RemoveImage(context.Background(),
				&pb.RemoveImageRequest{Image: &pb.ImageSpec{Image: ""}})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})
})
