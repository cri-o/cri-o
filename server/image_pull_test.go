package server_test

import (
	"context"

	"github.com/containers/image/v5/docker/reference"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	digest "github.com/opencontainers/go-digest"
	"go.uber.org/mock/gomock"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/internal/storage/references"
)

// The actual test suite.
var _ = t.Describe("ImagePull", func() {
	imageCandidate, err := references.ParseRegistryImageReferenceFromOutOfProcessData("docker.io/library/image:latest")
	Expect(err).ToNot(HaveOccurred())
	canonicalImageCandidate, err := reference.WithDigest(imageCandidate.Raw(), digest.Digest("sha256:340d9b015b194dc6e2a13938944e0d016e57b9679963fdeb9ce021daac430221"))
	Expect(err).ToNot(HaveOccurred())

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
				imageServerMock.EXPECT().PullImage(gomock.Any(), imageCandidate, gomock.Any()).
					Return(nil, canonicalImageCandidate, nil),
			)

			// When
			response, err := sut.PullImage(context.Background(),
				&types.PullImageRequest{Image: &types.ImageSpec{
					Image: "image",
				}})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response).NotTo(BeNil())
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
			Expect(err).To(HaveOccurred())
			Expect(response).To(BeNil())
		})

		It("should fail when image pull errors", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().CandidatesForPotentiallyShortImageName(
					gomock.Any(), "image").
					Return([]storage.RegistryImageReference{imageCandidate}, nil),
				imageServerMock.EXPECT().PullImage(gomock.Any(), imageCandidate, gomock.Any()).
					Return(nil, nil, t.TestError),
			)

			// When
			response, err := sut.PullImage(context.Background(),
				&types.PullImageRequest{Image: &types.ImageSpec{
					Image: "image",
				}})

			// Then
			Expect(err).To(HaveOccurred())
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
			Expect(err).To(HaveOccurred())
			Expect(response).To(BeNil())
		})
	})
})
