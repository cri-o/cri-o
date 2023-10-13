package server_test

import (
	"context"

	cstorage "github.com/containers/storage"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/internal/storage/references"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// The actual test suite
var _ = t.Describe("ImageStatus", func() {
	imageCandidate, err := references.ParseRegistryImageReferenceFromOutOfProcessData("docker.io/library/image:latest")
	Expect(err).To(BeNil())
	imageID, err := storage.ParseStorageImageIDFromOutOfProcessData("2a03a6059f21e150ae84b0973863609494aad70f0a80eaeb64bddd8d92465812")
	Expect(err).To(BeNil())

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
				imageServerMock.EXPECT().HeuristicallyTryResolvingStringAsIDPrefix("image").
					Return(nil),
				imageServerMock.EXPECT().CandidatesForPotentiallyShortImageName(
					gomock.Any(), "image").
					Return([]storage.RegistryImageReference{imageCandidate}, nil),
				imageServerMock.EXPECT().ImageStatusByName(
					gomock.Any(), imageCandidate,
				).Return(
					&storage.ImageResult{
						ID:   imageID,
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

		It("should succeed with a full image ID", func() {
			const testSHA256 = "2a03a6059f21e150ae84b0973863609494aad70f0a80eaeb64bddd8d92465812"
			// Given
			parsedTestSHA256, err := storage.ParseStorageImageIDFromOutOfProcessData(testSHA256)
			Expect(err).To(BeNil())
			gomock.InOrder(
				imageServerMock.EXPECT().HeuristicallyTryResolvingStringAsIDPrefix(testSHA256).
					Return(&parsedTestSHA256),
				imageServerMock.EXPECT().ImageStatusByID(
					gomock.Any(), parsedTestSHA256).
					Return(&storage.ImageResult{ID: parsedTestSHA256, User: "me"}, nil),
			)

			// When
			response, err := sut.ImageStatus(context.Background(),
				&types.ImageStatusRequest{Image: &types.ImageSpec{Image: testSHA256}})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should succeed with unknown image", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().HeuristicallyTryResolvingStringAsIDPrefix("image").
					Return(nil),
				imageServerMock.EXPECT().CandidatesForPotentiallyShortImageName(
					gomock.Any(), "image").
					Return([]storage.RegistryImageReference{imageCandidate}, nil),
				imageServerMock.EXPECT().ImageStatusByName(
					gomock.Any(), imageCandidate).
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
			response, err := sut.ImageStatus(context.Background(),
				&types.ImageStatusRequest{Image: &types.ImageSpec{Image: "image"}})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail if short name resolution failed", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().HeuristicallyTryResolvingStringAsIDPrefix("image").
					Return(nil),
				imageServerMock.EXPECT().CandidatesForPotentiallyShortImageName(
					gomock.Any(), "image").
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
