package server_test

import (
	"context"

	"github.com/cri-o/cri-o/pkg/storage"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// The actual test suite
var _ = t.Describe("ImageList", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})
	AfterEach(afterEach)

	const imageID = "imageID"

	t.Describe("ImageList", func() {
		It("should succeed", func() {
			// Given
			size := uint64(100)
			gomock.InOrder(
				imageServerMock.EXPECT().ListImages(
					gomock.Any(), gomock.Any()).
					Return([]storage.ImageResult{
						{ID: imageID, Size: &size, User: "10"}}, nil),
			)

			// When
			response, err := sut.ListImages(context.Background(),
				&pb.ListImagesRequest{})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
			Expect(len(response.Images)).To(BeEquivalentTo(1))
			Expect(response.Images[0].GetId()).To(Equal(imageID))
		})

		It("should succed with filter", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().ListImages(
					gomock.Any(), gomock.Any()).
					Return([]storage.ImageResult{{ID: imageID}}, nil),
			)

			// When
			response, err := sut.ListImages(context.Background(),
				&pb.ListImagesRequest{Filter: &pb.ImageFilter{
					Image: &pb.ImageSpec{Image: "image"}}})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
			Expect(len(response.Images)).To(BeEquivalentTo(1))
		})

		It("should fail when image listing erros", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().ListImages(gomock.Any(),
					gomock.Any()).Return(nil, t.TestError),
			)

			// When
			response, err := sut.ListImages(context.Background(),
				&pb.ListImagesRequest{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})
})
