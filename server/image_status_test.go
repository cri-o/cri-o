package server_test

import (
	"context"

	cstorage "github.com/containers/storage"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
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
					Return(&storage.ImageResult{
						ID:   "image",
						User: "10", Size: &size,
					}, nil),
			)

			// When
			response, err := sut.ImageStatus(context.Background(),
				&types.ImageStatusRequest{Image: &types.ImageSpec{Image: "image"}})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should succeed verbose", func() {
			// Given
			size := uint64(100)
			gomock.InOrder(
				imageServerMock.EXPECT().ResolveNames(
					gomock.Any(), gomock.Any(),
				).Return(
					[]string{"image"}, nil,
				),
				imageServerMock.EXPECT().ImageStatus(
					gomock.Any(), gomock.Any(),
				).Return(
					&storage.ImageResult{
						ID:   "image",
						User: "10",
						Size: &size,
						OCIConfig: &specs.Image{
							Platform: specs.Platform{
								Architecture: "arch",
								OS:           "os",
							},
						},
					},
					nil,
				),
			)

			// When
			response, err := sut.ImageStatus(context.Background(),
				&types.ImageStatusRequest{
					Image:   &types.ImageSpec{Image: "image"},
					Verbose: true,
				})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
			Expect(response.Info).To(HaveKey("info"))
			Expect(response.Info["info"]).To(ContainSubstring(
				`{"imageSpec":{"architecture":"arch","os":"os","config":{}`,
			))
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
				&types.ImageStatusRequest{Image: &types.ImageSpec{Image: "image"}})

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
				&types.ImageStatusRequest{Image: &types.ImageSpec{Image: "image"}})

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
				&types.ImageStatusRequest{Image: &types.ImageSpec{Image: "image"}})

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
				&types.ImageStatusRequest{Image: &types.ImageSpec{Image: "image"}})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail if no image specified", func() {
			// Given
			// When
			response, err := sut.ImageStatus(context.Background(),
				&types.ImageStatusRequest{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})
})
