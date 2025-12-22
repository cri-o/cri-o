package server_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/go-digest"
	"go.podman.io/image/v5/docker/reference"
	"go.uber.org/mock/gomock"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/internal/storage/references"
	"github.com/cri-o/cri-o/server"
)

func mustParseCanonical(s string) reference.Canonical {
	GinkgoHelper()

	ref, err := reference.ParseNormalizedNamed(s)
	Expect(err).ToNot(HaveOccurred())

	canonical, ok := ref.(reference.Canonical)
	Expect(ok).To(BeTrue(), "expected canonical reference for %s", s)

	return canonical
}

// The actual test suite.
var _ = t.Describe("ImageList", func() {
	imageCandidate, err := references.ParseRegistryImageReferenceFromOutOfProcessData("docker.io/library/image:latest")
	Expect(err).ToNot(HaveOccurred())
	imageID, err := storage.ParseStorageImageIDFromOutOfProcessData("2a03a6059f21e150ae84b0973863609494aad70f0a80eaeb64bddd8d92465812")
	Expect(err).ToNot(HaveOccurred())

	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})
	AfterEach(afterEach)

	t.Describe("ImageList", func() {
		It("should succeed", func() {
			// Given
			size := uint64(100)
			gomock.InOrder(
				imageServerMock.EXPECT().ListImages(gomock.Any()).
					Return([]storage.ImageResult{
						{ID: imageID, Size: &size, User: "10"},
					}, nil),
			)

			// When
			response, err := sut.ListImages(context.Background(),
				&types.ListImagesRequest{})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response).NotTo(BeNil())
			Expect(len(response.GetImages())).To(BeEquivalentTo(1))
			Expect(response.GetImages()[0].GetId()).To(Equal(imageID.IDStringForOutOfProcessConsumptionOnly()))
		})

		It("should succeed with filter", func() {
			// Given
			size := uint64(100)
			gomock.InOrder(
				imageServerMock.EXPECT().HeuristicallyTryResolvingStringAsIDPrefix("image").
					Return(nil),
				imageServerMock.EXPECT().CandidatesForPotentiallyShortImageName(
					gomock.Any(), "image").
					Return([]storage.RegistryImageReference{imageCandidate}, nil),
				imageServerMock.EXPECT().ImageStatusByName(
					gomock.Any(), imageCandidate).
					Return(&storage.ImageResult{
						ID:   imageID,
						User: "10", Size: &size,
					}, nil),
			)
			// When
			response, err := sut.ListImages(context.Background(),
				&types.ListImagesRequest{Filter: &types.ImageFilter{
					Image: &types.ImageSpec{Image: "image"},
				}})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response).NotTo(BeNil())
			Expect(len(response.GetImages())).To(BeEquivalentTo(1))
		})

		It("should fail when image listing errors", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().ListImages(gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			response, err := sut.ListImages(context.Background(),
				&types.ListImagesRequest{})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(response).To(BeNil())
		})

		It("should fail with filter status error", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().HeuristicallyTryResolvingStringAsIDPrefix("image").
					Return(nil),
				imageServerMock.EXPECT().CandidatesForPotentiallyShortImageName(
					gomock.Any(), "image").
					Return([]storage.RegistryImageReference{imageCandidate}, nil),
				imageServerMock.EXPECT().ImageStatusByName(
					gomock.Any(), imageCandidate).
					Return(nil, t.TestError),
			)

			// When
			response, err := sut.ListImages(context.Background(),
				&types.ListImagesRequest{Filter: &types.ImageFilter{
					Image: &types.ImageSpec{Image: "image"},
				}})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(response).To(BeNil())
		})
	})

	t.Describe("ConvertImage", func() {
		It("should succeed with empty repo tags and digests", func() {
			// Given
			image := &storage.ImageResult{ID: imageID}

			// When
			result := server.ConvertImage(image)

			// Then
			Expect(result).NotTo(BeNil())
			Expect(result.GetRepoTags()).To(BeEmpty())
			Expect(result.GetRepoDigests()).To(BeEmpty())
		})

		It("should succeed with repo tags and digests", func() {
			// Given
			size := uint64(100)
			digest1 := mustParseCanonical("docker.io/library/image@sha256:2a03a6059f21e150ae84b0973863609494aad70f0a80eaeb64bddd8d92465812")
			digest2 := mustParseCanonical("docker.io/library/other@sha256:3b14b7168f21e150ae84b0973863609494aad70f0a80eaeb64bddd8d92465813")
			image := &storage.ImageResult{
				ID:          imageID,
				RepoTags:    []string{"1", "2"},
				RepoDigests: []reference.Canonical{digest1, digest2},
				Size:        &size,
				User:        "10",
			}

			// When
			result := server.ConvertImage(image)

			// Then
			Expect(result).NotTo(BeNil())
			Expect(result.GetRepoTags()).To(HaveLen(2))
			Expect(result.GetRepoTags()).To(ConsistOf("1", "2"))
			Expect(result.GetRepoDigests()).To(HaveLen(2))
			Expect(result.GetRepoDigests()).To(ConsistOf(digest1.String(), digest2.String()))
			Expect(result.GetSize()).To(Equal(size))
			Expect(result.GetUid().GetValue()).To(BeEquivalentTo(10))
		})

		It("should succeed with previous tag but no current", func() {
			// Given
			image := &storage.ImageResult{
				ID:           imageID,
				PreviousName: "1",
				Digest:       digest.Digest("2"),
			}

			// When
			result := server.ConvertImage(image)

			// Then
			Expect(result).NotTo(BeNil())
			Expect(result.GetRepoTags()).To(BeEmpty())
			Expect(result.GetRepoDigests()).To(HaveLen(1))
			Expect(result.GetRepoDigests()).To(ContainElement("1@2"))
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
