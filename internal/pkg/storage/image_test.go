package storage_test

import (
	"context"
	"io/ioutil"
	"os"

	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/types"
	"github.com/containers/libpod/pkg/rootless"
	cs "github.com/containers/storage"
	"github.com/cri-o/cri-o/internal/pkg/storage"
	containerstoragemock "github.com/cri-o/cri-o/test/mocks/containerstorage"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	digest "github.com/opencontainers/go-digest"
)

// The actual test suite
var _ = t.Describe("Image", func() {
	// Test constants
	const (
		testRegistry            = "docker.io"
		testImageName           = "image"
		testNormalizedImageName = "docker.io/library/image:latest" // Keep in sync with testImageName!
		testSHA256              = "2a03a6059f21e150ae84b0973863609494aad70f0a80eaeb64bddd8d92465812"
	)

	var (
		mockCtrl  *gomock.Controller
		storeMock *containerstoragemock.MockStore
	)

	// The system under test
	var sut storage.ImageServer

	// Prepare the system under test
	BeforeEach(func() {
		// Setup the mocks
		mockCtrl = gomock.NewController(GinkgoT())
		storeMock = containerstoragemock.NewMockStore(mockCtrl)

		var err error
		sut, err = storage.GetImageService(
			context.Background(), nil, storeMock, "docker://",
			[]string{}, []string{testRegistry},
		)
		Expect(err).To(BeNil())
		Expect(sut).NotTo(BeNil())
	})
	AfterEach(func() {
		mockCtrl.Finish()
	})

	mockGetRef := func() mockSequence {
		return inOrder(
			// parseStoreReference ("@"+testImageName) will fail, recognizing it as an invalid image ID
			storeMock.EXPECT().Image(testImageName).
				Return(&cs.Image{ID: testSHA256}, nil),

			mockParseStoreReference(storeMock, testImageName),
		)
	}

	t.Describe("GetImageService", func() {
		It("should succeed to retrieve an image service", func() {
			// Given
			// When
			imageService, err := storage.GetImageService(
				context.Background(), nil, storeMock, "",
				[]string{"reg1", "reg1", "reg2"},
				[]string{"reg3", "reg3", "reg4"},
			)

			// Then
			Expect(err).To(BeNil())
			Expect(imageService).NotTo(BeNil())
		})

		It("should succeed with custom registries.conf", func() {
			// Given
			// When
			imageService, err := storage.GetImageService(
				context.Background(),
				&types.SystemContext{
					SystemRegistriesConfPath: "../../../test/registries.conf"},
				storeMock, "", []string{}, []string{},
			)

			// Then
			Expect(err).To(BeNil())
			Expect(imageService).NotTo(BeNil())
		})

		It("should fail to retrieve an image service without storage", func() {
			// Given
			storeOptions, err := cs.DefaultStoreOptions(rootless.IsRootless(), rootless.GetRootlessUID())
			Expect(err).To(BeNil())
			storeOptions.GraphRoot = ""

			// When
			_, err = cs.GetStore(storeOptions)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail if unqualified search registries errors", func() {
			// Given
			// When
			imageService, err := storage.GetImageService(
				context.Background(),
				&types.SystemContext{SystemRegistriesConfPath: "/invalid"},
				storeMock, "", []string{}, []string{},
			)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(imageService).To(BeNil())
		})
	})

	t.Describe("GetStore", func() {
		It("should succeed to retrieve the store", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Delete(gomock.Any()).Return(nil),
			)

			// When
			store := sut.GetStore()

			// Then
			Expect(store).NotTo(BeNil())
			Expect(store.Delete("")).To(BeNil())
		})
	})

	t.Describe("ResolveNames", func() {
		It("should succeed to resolve", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{ID: "id"}, nil),
			)

			// When
			names, err := sut.ResolveNames(nil, testImageName)

			// Then
			Expect(err).To(BeNil())
			Expect(len(names)).To(Equal(1))
			Expect(names[0]).To(Equal(testRegistry + "/library/" + testImageName))
		})

		It("should succeed to resolve with full qualified image name", func() {
			// Given
			const imageName = "docker.io/library/busybox:latest"
			gomock.InOrder(
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{ID: "id"}, nil),
			)

			// When
			names, err := sut.ResolveNames(nil, imageName)

			// Then
			Expect(err).To(BeNil())
			Expect(len(names)).To(Equal(1))
			Expect(names[0]).To(Equal(imageName))
		})

		It("should succeed to resolve with a local copy", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{ID: testImageName}, nil),
			)

			// When
			names, err := sut.ResolveNames(nil, testImageName)

			// Then
			Expect(err).To(BeNil())
			Expect(len(names)).To(Equal(1))
			Expect(names[0]).To(Equal(testImageName))
		})

		It("should fail to resolve with invalid image id", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{ID: testImageName}, nil),
			)

			// When
			names, err := sut.ResolveNames(nil, testSHA256)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(err).To(Equal(storage.ErrCannotParseImageID))
			Expect(names).To(BeNil())
		})

		It("should fail to resolve with invalid registry name", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{ID: testImageName}, nil),
			)

			// When
			names, err := sut.ResolveNames(nil, "camelCaseName")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(names).To(BeNil())
		})

		It("should fail to resolve without configured registries", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{ID: "id"}, nil),
			)

			// Create an empty file for the registries config path
			file, err := ioutil.TempFile(".", "registries")
			Expect(err).To(BeNil())
			defer os.Remove(file.Name())

			sut, err := storage.GetImageService(context.Background(),
				&types.SystemContext{SystemRegistriesConfPath: file.Name()},
				storeMock, "", []string{}, []string{})
			Expect(err).To(BeNil())
			Expect(sut).NotTo(BeNil())

			// When
			names, err := sut.ResolveNames(nil, testImageName)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(err).To(Equal(storage.ErrNoRegistriesConfigured))
			Expect(names).To(BeNil())
		})
	})

	t.Describe("UntagImage", func() {
		It("should succeed to untag an image", func() {
			// Given
			inOrder(
				mockGetRef(),
				mockGetStoreImage(storeMock, testNormalizedImageName, testSHA256),
				mockResolveImage(storeMock, testNormalizedImageName, testSHA256),
				storeMock.EXPECT().DeleteImage(testSHA256, true).
					Return(nil, nil),
			)

			// When
			err := sut.UntagImage(&types.SystemContext{}, testImageName)

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail to untag an image with invalid name", func() {
			// Given
			// When
			err := sut.UntagImage(&types.SystemContext{}, "")

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail to untag an image that can't be found", func() {
			// Given
			inOrder(
				mockGetRef(),
				mockGetStoreImage(storeMock, testNormalizedImageName, ""),
			)

			// When
			err := sut.UntagImage(&types.SystemContext{}, testImageName)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail to untag an image with a docker:// reference", func() {
			// Given
			const imageName = "docker://localhost/busybox:latest"
			inOrder(
				mockGetStoreImage(storeMock, "localhost/busybox:latest", testSHA256),
			)

			// When
			err := sut.UntagImage(&types.SystemContext{}, imageName)

			// Then
			Expect(err).NotTo(BeNil()) // FIXME: this actually fails because it tries to untag the image at the docker://localhost registry!
		})

		It("should fail to untag an image with a docker:// digest reference", func() {
			// Given
			const imageName = "docker://localhost/busybox@sha256:" + testSHA256
			inOrder(
				mockGetStoreImage(storeMock, "localhost/busybox@sha256:"+testSHA256, testSHA256),
			)

			// When
			err := sut.UntagImage(&types.SystemContext{}, imageName)

			// Then
			Expect(err).NotTo(BeNil()) // FIXME: this actually fails because it tries to untag the image at the docker://localhost registry!
		})

		It("should fail to untag an image with multiple names", func() {
			// Given
			inOrder(
				mockGetRef(),
				// storage.Transport.GetStoreImage:
				storeMock.EXPECT().Image(testNormalizedImageName).
					Return(&cs.Image{
						ID:    testSHA256,
						Names: []string{testNormalizedImageName, "localhost/b:latest", "localhost/c:latest"},
					}, nil),

				storeMock.EXPECT().SetNames(testSHA256, []string{"localhost/b:latest", "localhost/c:latest"}).
					Return(t.TestError),
			)

			// When
			err := sut.UntagImage(&types.SystemContext{}, testImageName)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("ImageStatus", func() {
		It("should succeed to get the image status with digest", func() {
			// Given
			inOrder(
				mockGetRef(),
				// storage.Transport.GetStoreImage:
				storeMock.EXPECT().Image(testNormalizedImageName).
					Return(&cs.Image{
						ID: testSHA256,
						Names: []string{testNormalizedImageName,
							"localhost/a@sha256:" + testSHA256,
							"localhost/b@sha256:" + testSHA256,
							"localhost/c:latest"},
					}, nil),
				// buildImageCacheItem
				mockNewImage(storeMock, testNormalizedImageName, testSHA256),
				// makeRepoDigests
				storeMock.EXPECT().ImageBigDataDigest(testSHA256, gomock.Any()).
					Return(digest.Digest("a:"+testSHA256), nil),
			)

			// When
			res, err := sut.ImageStatus(&types.SystemContext{}, testImageName)

			// Then
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
		})

		It("should fail to get on wrong reference", func() {
			// Given
			// When
			res, err := sut.ImageStatus(&types.SystemContext{}, "")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})

		It("should fail to get on missing store image", func() {
			// Given
			inOrder(
				mockGetRef(),
				mockGetStoreImage(storeMock, testNormalizedImageName, ""),
			)

			// When
			res, err := sut.ImageStatus(&types.SystemContext{}, testImageName)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})

		It("should fail to get on corrupt image", func() {
			// Given
			inOrder(
				mockGetRef(),
				mockGetStoreImage(storeMock, testNormalizedImageName, testSHA256),
				// In buildImageCacheItem, storageReference.NewImage fails reading the manifest:
				mockResolveImage(storeMock, testNormalizedImageName, testSHA256),
				storeMock.EXPECT().ImageBigData(testSHA256, gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			res, err := sut.ImageStatus(&types.SystemContext{}, testImageName)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})
	})

	t.Describe("ListImages", func() {
		It("should succeed to list images without filter", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Images().Return([]cs.Image{}, nil),
			)

			// When
			res, err := sut.ListImages(&types.SystemContext{}, "")

			// Then
			Expect(err).To(BeNil())
			Expect(len(res)).To(Equal(0))
		})

		It("should succeed to list multiple images without filter", func() {
			// Given
			mockLoop := func() mockSequence {
				return inOrder(
					// buildImageCacheItem:
					mockNewImage(storeMock, testSHA256, testSHA256),
					// makeRepoDigests:
					storeMock.EXPECT().ImageBigDataDigest(testSHA256, gomock.Any()).
						Return(digest.Digest(""), nil),
				)
			}
			inOrder(
				storeMock.EXPECT().Images().Return(
					[]cs.Image{
						{ID: testSHA256, Names: []string{"a", "b", "c@sha256:" + testSHA256}},
						{ID: testSHA256}},
					nil),
				mockParseStoreReference(storeMock, "@"+testSHA256),
				mockLoop(),
				mockParseStoreReference(storeMock, "@"+testSHA256),
				mockLoop(),
			)

			// When
			res, err := sut.ListImages(&types.SystemContext{}, "")

			// Then
			Expect(err).To(BeNil())
			Expect(len(res)).To(Equal(2))
		})

		It("should succeed to list images with filter", func() {
			// Given
			inOrder(
				mockGetRef(),
				mockGetStoreImage(storeMock, testNormalizedImageName, testSHA256),
				// buildImageCacheItem:
				mockNewImage(storeMock, testNormalizedImageName, testSHA256),
				// makeRepoDigests:
				storeMock.EXPECT().ImageBigDataDigest(testSHA256, gomock.Any()).
					Return(digest.Digest(""), nil),
			)

			// When
			res, err := sut.ListImages(&types.SystemContext{}, testImageName)

			// Then
			Expect(err).To(BeNil())
			Expect(len(res)).To(Equal(1))
			Expect(res[0].ID).To(Equal(testSHA256))
		})

		It("should succeed to list images on failure to access an image", func() {
			// Given
			inOrder(
				mockGetRef(),
				mockGetStoreImage(storeMock, testNormalizedImageName, ""),
			)

			// When
			res, err := sut.ListImages(&types.SystemContext{}, testImageName)

			// Then
			Expect(err).To(BeNil())
			Expect(len(res)).To(Equal(0))
		})

		It("should fail to list images with filter an invalid reference", func() {
			// Given
			gomock.InOrder(
				// parseStoreReference("@wrong://image") tries this before failing in parseNormalizedNamed:
				storeMock.EXPECT().Image("wrong://image").Return(nil, cs.ErrImageUnknown),
			)
			// When
			res, err := sut.ListImages(&types.SystemContext{}, "wrong://image")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})

		It("should fail to list images with filter on failing appendCachedResult", func() {
			// Given
			inOrder(
				mockGetRef(),
				mockGetStoreImage(storeMock, testNormalizedImageName, testSHA256),
				// in buildImageCacheItem, NewImage is failing:
				mockResolveImage(storeMock, testNormalizedImageName, testSHA256),
				storeMock.EXPECT().ImageBigData(testSHA256, gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			res, err := sut.ListImages(&types.SystemContext{}, testImageName)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})

		It("should fail to list images without a filter on failing store", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Images().Return(nil, t.TestError),
			)

			// When
			res, err := sut.ListImages(&types.SystemContext{}, "")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})

		It("should fail to list multiple images without filter on invalid image ID in results", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Images().Return(
					[]cs.Image{{ID: ""}}, nil),
			)

			// When
			res, err := sut.ListImages(&types.SystemContext{}, "")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})

		It("should fail to list multiple images without filter on append", func() {
			// Given
			inOrder(
				storeMock.EXPECT().Images().Return(
					[]cs.Image{{ID: testSHA256}}, nil),
				mockParseStoreReference(storeMock, "@"+testSHA256),
				storeMock.EXPECT().Image(gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			res, err := sut.ListImages(&types.SystemContext{}, "")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})

	})

	t.Describe("PrepareImage", func() {
		It("should succeed with testimage", func() {
			// Given
			const imageName = "tarball:../../../test/testdata/image.tar"

			// When
			res, err := sut.PrepareImage(&types.SystemContext{}, imageName)

			// Then
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
		})

		It("should fail on invalid image name", func() {
			// Given
			// When
			res, err := sut.PrepareImage(&types.SystemContext{}, "")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})
	})

	t.Describe("PullImage", func() {
		It("should fail on invalid image name", func() {
			// Given
			// When
			res, err := sut.PullImage(&types.SystemContext{}, "",
				&copy.Options{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})

		It("should fail on invalid policy path", func() {
			// Given
			// When
			res, err := sut.PullImage(&types.SystemContext{
				SignaturePolicyPath: "/not-existing",
			}, "", &copy.Options{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})

		It("should fail on copy image", func() {
			// Given
			const imageName = "docker://localhost/busybox:latest"
			mockParseStoreReference(storeMock, "localhost/busybox:latest")

			// When
			res, err := sut.PullImage(&types.SystemContext{
				SignaturePolicyPath: "../../../test/policy.json",
			}, imageName, &copy.Options{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})

		It("should fail on canonical copy image", func() {
			// Given
			const imageName = "docker://localhost/busybox@sha256:" + testSHA256
			mockParseStoreReference(storeMock, "localhost/busybox@sha256:"+testSHA256)

			// When
			res, err := sut.PullImage(&types.SystemContext{
				SignaturePolicyPath: "../../../test/policy.json",
			}, imageName, &copy.Options{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})
	})
})
