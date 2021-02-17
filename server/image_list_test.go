package server_test

import (
	"context"

	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/server"
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
						{ID: imageID, Size: &size, User: "10"},
					}, nil),
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
					Image: &pb.ImageSpec{Image: "image"},
				}})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
			Expect(len(response.Images)).To(BeEquivalentTo(1))
		})

		It("should fail when image listing errors", func() {
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

	t.Describe("ConvertImage", func() {
		It("should succeed with empty repo tags and digests", func() {
			// Given
			image := &storage.ImageResult{}

			// When
			result := server.ConvertImage(image)

			// Then
			Expect(result).NotTo(BeNil())
			Expect(result.RepoTags).To(HaveLen(1))
			Expect(result.RepoTags).To(ContainElement("<none>:<none>"))
			Expect(result.RepoDigests).To(HaveLen(1))
			Expect(result.RepoDigests).To(ContainElement("<none>@<none>"))
		})

		It("should succeed with repo tags and digests", func() {
			// Given
			size := uint64(100)
			image := &storage.ImageResult{
				RepoTags:    []string{"1", "2"},
				RepoDigests: []string{"3", "4"},
				Size:        &size,
				User:        "10",
			}

			// When
			result := server.ConvertImage(image)

			// Then
			Expect(result).NotTo(BeNil())
			Expect(result.RepoTags).To(HaveLen(2))
			Expect(result.RepoTags).To(ConsistOf("1", "2"))
			Expect(result.RepoDigests).To(HaveLen(2))
			Expect(result.RepoDigests).To(ConsistOf("3", "4"))
			Expect(result.Size_).To(Equal(size))
			Expect(result.Uid.Value).To(BeEquivalentTo(10))
		})

		It("should succeed with previous tag but no current", func() {
			// Given
			image := &storage.ImageResult{PreviousName: "1"}

			// When
			result := server.ConvertImage(image)

			// Then
			Expect(result).NotTo(BeNil())
			Expect(result.RepoTags).To(HaveLen(1))
			Expect(result.RepoTags).To(ContainElement("1:<none>"))
		})

		It("should return nil if input image is nil", func() {
			// Given
			// When
			result := server.ConvertImage(nil)

			// Then
			Expect(result).To(BeNil())
		})
	})
})
