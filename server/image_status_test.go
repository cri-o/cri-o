package server_test

import (
	"context"

	cstorage "github.com/containers/storage"
	"github.com/cri-o/cri-o/internal/pkg/storage"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// The actual test suite
var _ = t.Describe("ImageStatus", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})
	AfterEach(afterEach)

	t.Describe("ImageStatus", func() {
		It("should succeed", func() {
			// Given
			size := uint64(100)
			gomock.InOrder(
				imageServerMock.EXPECT().ResolveNames(
					gomock.Any(), gomock.Any()).
					Return([]string{"image"}, nil),
				imageServerMock.EXPECT().ImageStatus(
					gomock.Any(), gomock.Any()).
					Return(&storage.ImageResult{ID: "image",
						User: "10", Size: &size}, nil),
			)

			// When
			response, err := sut.ImageStatus(context.Background(),
				&pb.ImageStatusRequest{Image: &pb.ImageSpec{Image: "image"}})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should succeed with wrong image id", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().ResolveNames(
					gomock.Any(), gomock.Any()).
					Return(nil, storage.ErrCannotParseImageID),
				imageServerMock.EXPECT().ImageStatus(
					gomock.Any(), gomock.Any()).
					Return(&storage.ImageResult{ID: "image", User: "me"}, nil),
			)

			// When
			response, err := sut.ImageStatus(context.Background(),
				&pb.ImageStatusRequest{Image: &pb.ImageSpec{Image: "image"}})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should succeed with unknown image", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().ResolveNames(
					gomock.Any(), gomock.Any()).
					Return([]string{"image"}, nil),
				imageServerMock.EXPECT().ImageStatus(
					gomock.Any(), gomock.Any()).
					Return(nil, cstorage.ErrImageUnknown),
			)

			// When
			response, err := sut.ImageStatus(context.Background(),
				&pb.ImageStatusRequest{Image: &pb.ImageSpec{Image: "image"}})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should fail with wrong image status retrieval", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().ResolveNames(
					gomock.Any(), gomock.Any()).
					Return([]string{"image"}, nil),
				imageServerMock.EXPECT().ImageStatus(
					gomock.Any(), gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			response, err := sut.ImageStatus(context.Background(),
				&pb.ImageStatusRequest{Image: &pb.ImageSpec{Image: "image"}})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail if resolve names failed", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().ResolveNames(
					gomock.Any(), gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			response, err := sut.ImageStatus(context.Background(),
				&pb.ImageStatusRequest{Image: &pb.ImageSpec{Image: "image"}})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail if no image specified", func() {
			// Given
			// When
			response, err := sut.ImageStatus(context.Background(),
				&pb.ImageStatusRequest{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})
})
