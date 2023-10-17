package server_test

import (
	"context"

	imageTypes "github.com/containers/image/v5/types"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/internal/storage/references"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	digest "github.com/opencontainers/go-digest"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// The actual test suite
var _ = t.Describe("ImagePull", func() {
	imageCandidate, err := references.ParseRegistryImageReferenceFromOutOfProcessData("docker.io/library/image:latest")
	Expect(err).To(BeNil())

	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})
	AfterEach(afterEach)

	t.Describe("ImagePull", func() {
		It("should succeed with pull", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().CandidatesForPotentiallyShortImageName(
					gomock.Any(), "image").
					Return([]storage.RegistryImageReference{imageCandidate}, nil),
				imageServerMock.EXPECT().PrepareImage(gomock.Any(),
					imageCandidate).Return(imageCloserMock, nil),
				imageServerMock.EXPECT().ImageStatusByName(
					gomock.Any(), imageCandidate).
					Return(&storage.ImageResult{ID: "id"}, nil),
				imageCloserMock.EXPECT().ConfigInfo().
					Return(imageTypes.BlobInfo{Digest: digest.Digest("")}),
				imageServerMock.EXPECT().PullImage(
					gomock.Any(), imageCandidate, gomock.Any()).
					Return(nil, nil),
				imageServerMock.EXPECT().ImageStatusByName(
					gomock.Any(), imageCandidate).
					Return(&storage.ImageResult{
						ID:          "image",
						RepoDigests: []string{"digest"},
					}, nil),
				imageCloserMock.EXPECT().Close().Return(nil),
			)

			// When
			response, err := sut.PullImage(context.Background(),
				&types.PullImageRequest{Image: &types.ImageSpec{
					Image: "image",
				}})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should succeed when already pulled", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().CandidatesForPotentiallyShortImageName(
					gomock.Any(), "image").
					Return([]storage.RegistryImageReference{imageCandidate}, nil),
				imageServerMock.EXPECT().PrepareImage(gomock.Any(),
					imageCandidate).Return(imageCloserMock, nil),
				imageServerMock.EXPECT().ImageStatusByName(
					gomock.Any(), imageCandidate).
					Return(&storage.ImageResult{
						ID:           "id",
						ConfigDigest: digest.Digest("digest"),
					}, nil),
				imageCloserMock.EXPECT().ConfigInfo().
					Return(imageTypes.BlobInfo{Digest: digest.Digest("digest")}),
				imageServerMock.EXPECT().ImageStatusByName(
					gomock.Any(), imageCandidate).
					Return(&storage.ImageResult{
						ID:          "image",
						RepoDigests: []string{"digest"},
					}, nil),
				imageCloserMock.EXPECT().Close().Return(nil),
			)

			// When
			response, err := sut.PullImage(context.Background(),
				&types.PullImageRequest{
					Image: &types.ImageSpec{Image: "image"},
					Auth: &types.AuthConfig{
						Username: "username",
						Password: "password",
						Auth:     "auth",
					},
				})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should fail when second image status retrieval errors", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().CandidatesForPotentiallyShortImageName(
					gomock.Any(), "image").
					Return([]storage.RegistryImageReference{imageCandidate}, nil),
				imageServerMock.EXPECT().PrepareImage(gomock.Any(),
					imageCandidate).Return(imageCloserMock, nil),
				imageServerMock.EXPECT().ImageStatusByName(
					gomock.Any(), imageCandidate).
					Return(&storage.ImageResult{ID: "id"}, nil),
				imageCloserMock.EXPECT().ConfigInfo().
					Return(imageTypes.BlobInfo{Digest: digest.Digest("")}),
				imageServerMock.EXPECT().PullImage(
					gomock.Any(), imageCandidate, gomock.Any()).
					Return(nil, nil),
				imageServerMock.EXPECT().ImageStatusByName(
					gomock.Any(), imageCandidate).
					Return(nil, t.TestError),
				imageCloserMock.EXPECT().Close().Return(nil),
			)

			// When
			response, err := sut.PullImage(context.Background(),
				&types.PullImageRequest{
					Image: &types.ImageSpec{Image: "image"},
					Auth: &types.AuthConfig{
						Username: "username",
						Password: "password",
						Auth:     "YWJjOmFiYw==",
					},
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail credential decode errors", func() {
			// Given
			// When
			response, err := sut.PullImage(context.Background(),
				&types.PullImageRequest{
					Image: &types.ImageSpec{Image: "id"},
					Auth: &types.AuthConfig{
						Auth: "❤️",
					},
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when image pull errors", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().CandidatesForPotentiallyShortImageName(
					gomock.Any(), "image").
					Return([]storage.RegistryImageReference{imageCandidate}, nil),
				imageServerMock.EXPECT().PrepareImage(gomock.Any(),
					imageCandidate).Return(imageCloserMock, nil),
				imageServerMock.EXPECT().ImageStatusByName(
					gomock.Any(), imageCandidate).
					Return(&storage.ImageResult{ID: "id"}, nil),
				imageCloserMock.EXPECT().ConfigInfo().
					Return(imageTypes.BlobInfo{Digest: digest.Digest("")}),
				imageServerMock.EXPECT().PullImage(
					gomock.Any(), imageCandidate, gomock.Any()).
					Return(nil, t.TestError),
				imageCloserMock.EXPECT().Close().Return(nil),
			)

			// When
			response, err := sut.PullImage(context.Background(),
				&types.PullImageRequest{Image: &types.ImageSpec{
					Image: "image",
				}})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when prepare image errors", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().CandidatesForPotentiallyShortImageName(
					gomock.Any(), "image").
					Return([]storage.RegistryImageReference{imageCandidate}, nil),
				imageServerMock.EXPECT().PrepareImage(gomock.Any(),
					imageCandidate).Return(nil, t.TestError),
			)

			// When
			response, err := sut.PullImage(context.Background(),
				&types.PullImageRequest{Image: &types.ImageSpec{
					Image: "image",
				}})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when resolve names errors", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().CandidatesForPotentiallyShortImageName(
					gomock.Any(), "").
					Return(nil, t.TestError),
			)
			// When
			response, err := sut.PullImage(context.Background(),
				&types.PullImageRequest{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})
})
