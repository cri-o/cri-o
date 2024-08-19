package server_test

import (
	"context"
	"fmt"

	storagetypes "github.com/containers/storage"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/internal/storage/references"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// The actual test suite
var _ = t.Describe("ImageRemove", func() {
	resolvedImageName, err := references.ParseRegistryImageReferenceFromOutOfProcessData("docker.io/library/image:latest")
	Expect(err).To(BeNil())

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
				imageServerMock.EXPECT().HeuristicallyTryResolvingStringAsIDPrefix("image").
					Return(nil),
				imageServerMock.EXPECT().CandidatesForPotentiallyShortImageName(
					gomock.Any(), "image").
					Return([]storage.RegistryImageReference{resolvedImageName}, nil),
				imageServerMock.EXPECT().UntagImage(gomock.Any(),
					resolvedImageName).Return(nil),
			)
			// When
			_, err := sut.RemoveImage(context.Background(),
				&types.RemoveImageRequest{Image: &types.ImageSpec{Image: "image"}})

			// Then
			Expect(err).To(BeNil())
		})

		// Given
		It("should succeed with a full image id", func() {
			const testSHA256 = "2a03a6059f21e150ae84b0973863609494aad70f0a80eaeb64bddd8d92465812"
			parsedTestSHA256, err := storage.ParseStorageImageIDFromOutOfProcessData(testSHA256)
			Expect(err).To(BeNil())
			gomock.InOrder(
				imageServerMock.EXPECT().HeuristicallyTryResolvingStringAsIDPrefix(testSHA256).
					Return(&parsedTestSHA256),
				imageServerMock.EXPECT().DeleteImage(
					gomock.Any(), parsedTestSHA256).
					Return(nil),
			)
			// When
			_, err = sut.RemoveImage(context.Background(),
				&types.RemoveImageRequest{Image: &types.ImageSpec{Image: testSHA256}})

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail when image untag errors", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().HeuristicallyTryResolvingStringAsIDPrefix("image").
					Return(nil),
				imageServerMock.EXPECT().CandidatesForPotentiallyShortImageName(
					gomock.Any(), "image").
					Return([]storage.RegistryImageReference{resolvedImageName}, nil),
				imageServerMock.EXPECT().UntagImage(gomock.Any(),
					resolvedImageName).Return(t.TestError),
			)
			// When
			_, err := sut.RemoveImage(context.Background(),
				&types.RemoveImageRequest{Image: &types.ImageSpec{Image: "image"}})

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail when name resolving errors", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().HeuristicallyTryResolvingStringAsIDPrefix("image").
					Return(nil),
				imageServerMock.EXPECT().CandidatesForPotentiallyShortImageName(
					gomock.Any(), "image").
					Return(nil, t.TestError),
			)
			// When
			_, err := sut.RemoveImage(context.Background(),
				&types.RemoveImageRequest{Image: &types.ImageSpec{Image: "image"}})

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail without specified image", func() {
			// Given
			// When
			_, err := sut.RemoveImage(context.Background(),
				&types.RemoveImageRequest{Image: &types.ImageSpec{Image: ""}})

			// Then
			Expect(err).NotTo(BeNil())
		})

		// https://github.com/kubernetes/cri-api/blob/c20fa40/pkg/apis/runtime/v1/api.proto#L156-L157
		It("should succeed if image is not found", func() {
			// Given
			const testSHA256 = "2a03a6059f21e150ae84b0973863609494aad70f0a80eaeb64bddd8d92465812"
			parsedTestSHA256, err := storage.ParseStorageImageIDFromOutOfProcessData(testSHA256)
			Expect(err).ToNot(HaveOccurred())
			gomock.InOrder(
				imageServerMock.EXPECT().HeuristicallyTryResolvingStringAsIDPrefix(testSHA256).
					Return(&parsedTestSHA256),
				imageServerMock.EXPECT().DeleteImage(
					gomock.Any(), parsedTestSHA256).
					Return(fmt.Errorf("invalid image: %w", storagetypes.ErrImageUnknown)),
			)

			// When
			_, err = sut.RemoveImage(context.Background(),
				&types.RemoveImageRequest{Image: &types.ImageSpec{Image: testSHA256}})

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		// https://github.com/kubernetes/cri-api/blob/c20fa40/pkg/apis/runtime/v1/api.proto#L156-L157
		It("should succeed if image is not found", func() {
			// Given
			const testSHA256 = "2a03a6059f21e150ae84b0973863609494aad70f0a80eaeb64bddd8d92465812"
			parsedTestSHA256, err := storage.ParseStorageImageIDFromOutOfProcessData(testSHA256)
			Expect(err).ToNot(HaveOccurred())
			gomock.InOrder(
				imageServerMock.EXPECT().HeuristicallyTryResolvingStringAsIDPrefix(testSHA256).
					Return(&parsedTestSHA256),
				imageServerMock.EXPECT().DeleteImage(
					gomock.Any(), parsedTestSHA256).
					Return(fmt.Errorf("invalid image: %w", storagetypes.ErrImageUnknown)),
			)

			// When
			_, err = sut.RemoveImage(context.Background(),
				&types.RemoveImageRequest{Image: &types.ImageSpec{Image: testSHA256}})

			// Then
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
