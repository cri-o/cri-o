package storage_test

import (
	"context"
	"io/ioutil"
	"os"

	"github.com/containers/image/copy"
	"github.com/containers/image/types"
	cs "github.com/containers/storage"
	"github.com/cri-o/cri-o/pkg/storage"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/go-digest"
)

// The actual test suite
var _ = t.Describe("Image", func() {
	// Test constants
	const (
		testRegistry  = "docker.io"
		testImageName = "image"
		testSHA256    = "2a03a6059f21e150ae84b0973863609494aad70f0a80eaeb64bddd8d92465812"
	)

	// The system under test
	var sut storage.ImageServer

	// Prepare the system under test
	BeforeEach(func() {
		var err error
		sut, err = storage.GetImageService(
			context.Background(), nil, storeMock, "",
			[]string{}, []string{testRegistry},
		)
		Expect(err).To(BeNil())
		Expect(sut).NotTo(BeNil())
	})

	mockParseStoreReference := func() {
		gomock.InOrder(
			storeMock.EXPECT().GraphOptions().Return([]string{}),
			storeMock.EXPECT().GraphDriverName().Return(""),
			storeMock.EXPECT().GraphRoot().Return(""),
			storeMock.EXPECT().RunRoot().Return(""),
		)
	}

	mockGetRef := func() {
		gomock.InOrder(
			storeMock.EXPECT().Image(gomock.Any()).
				Return(&cs.Image{ID: testImageName}, nil),
		)
		mockParseStoreReference()
	}

	mockListImage := func() {
		gomock.InOrder(
			storeMock.EXPECT().Image(gomock.Any()).
				Return(&cs.Image{ID: testImageName}, nil),
			storeMock.EXPECT().ListImageBigData(gomock.Any()).
				Return([]string{""}, nil),
			storeMock.EXPECT().ImageBigDataSize(gomock.Any(), gomock.Any()).
				Return(int64(0), nil),
			storeMock.EXPECT().ImageBigData(gomock.Any(), gomock.Any()).
				Return(testManifest, nil),
			storeMock.EXPECT().Image(gomock.Any()).
				Return(&cs.Image{ID: testImageName}, nil),
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
					SystemRegistriesConfPath: "../../test/registries.conf"},
				storeMock, "", []string{}, []string{},
			)

			// Then
			Expect(err).To(BeNil())
			Expect(imageService).NotTo(BeNil())
		})

		It("should fail to retrieve an image service without storage", func() {
			// Given
			cs.DefaultStoreOptions.GraphRoot = ""

			// When
			imageService, err := storage.GetImageService(
				context.Background(), nil, nil, "", []string{}, []string{},
			)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(imageService).To(BeNil())
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
			names, err := sut.ResolveNames(testImageName)

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
			names, err := sut.ResolveNames(imageName)

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
			names, err := sut.ResolveNames(testImageName)

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
			names, err := sut.ResolveNames(testSHA256)

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
			names, err := sut.ResolveNames("camelCaseName")

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
			names, err := sut.ResolveNames(testImageName)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(err).To(Equal(storage.ErrNoRegistriesConfigured))
			Expect(names).To(BeNil())
		})
	})

	t.Describe("RemoveImage", func() {
		It("should succeed to remove an image on first store ref", func() {
			// Given
			mockGetRef()
			gomock.InOrder(
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{ID: testImageName}, nil),
				storeMock.EXPECT().DeleteImage(gomock.Any(), gomock.Any()).
					Return(nil, nil),
			)

			// When
			err := sut.RemoveImage(&types.SystemContext{}, testImageName)

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed to remove an image on second store ref", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Image(gomock.Any()).Return(nil, t.TestError),
			)
			mockGetRef()
			gomock.InOrder(
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{ID: testImageName}, nil),
				storeMock.EXPECT().DeleteImage(gomock.Any(), gomock.Any()).
					Return(nil, nil),
			)

			// When
			err := sut.RemoveImage(&types.SystemContext{}, testImageName)

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail to remove an image with invalid name", func() {
			// Given
			// When
			err := sut.RemoveImage(&types.SystemContext{}, "")

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("UntagImage", func() {
		It("should succeed to untag an image", func() {
			// Given
			mockGetRef()
			gomock.InOrder(
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{ID: testImageName}, nil),
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{ID: testImageName}, nil),
				storeMock.EXPECT().DeleteImage(gomock.Any(), gomock.Any()).
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

		It("should fail to untag an image with invalid name", func() {
			// Given
			mockGetRef()
			gomock.InOrder(
				storeMock.EXPECT().Image(gomock.Any()).Return(nil, t.TestError),
				storeMock.EXPECT().Image(gomock.Any()).Return(nil, t.TestError),
			)

			// When
			err := sut.UntagImage(&types.SystemContext{}, testImageName)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail to untag an image with failed reference preparation", func() {
			// Given
			mockGetRef()
			gomock.InOrder(
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{ID: "otherImage"}, nil),
			)

			// When
			err := sut.UntagImage(&types.SystemContext{}, testImageName)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail to untag an image with docker reference", func() {
			// Given
			const imageName = "docker://localhost/busybox:latest"
			gomock.InOrder(
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{ID: testImageName}, nil),
			)

			// When
			err := sut.UntagImage(&types.SystemContext{}, imageName)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail to untag an image with digest docker reference", func() {
			// Given
			const imageName = "docker://localhost/busybox@sha256:" + testSHA256
			gomock.InOrder(
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{ID: testImageName}, nil),
			)

			// When
			err := sut.UntagImage(&types.SystemContext{}, imageName)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail to untag an image with multiple names", func() {
			// Given
			const imageName = "docker://localhost/busybox:latest"
			gomock.InOrder(
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{
						ID:    testImageName,
						Names: []string{"a", "b", "c"},
					}, nil),
				storeMock.EXPECT().SetNames(gomock.Any(), gomock.Any()).
					Return(t.TestError),
			)

			// When
			err := sut.UntagImage(&types.SystemContext{}, imageName)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("ImageStatus", func() {
		It("should succeed to get the image status with digest", func() {
			// Given
			mockGetRef()
			gomock.InOrder(
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{ID: testImageName,
						Names: []string{"a@sha256:" + testSHA256,
							"b@sha256:" + testSHA256, "c"},
					}, nil),
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{ID: testImageName}, nil),
				storeMock.EXPECT().ImageBigData(gomock.Any(), gomock.Any()).
					Return(testManifest, nil),
				storeMock.EXPECT().ListImageBigData(gomock.Any()).
					Return([]string{""}, nil),
				storeMock.EXPECT().ImageBigDataSize(gomock.Any(), gomock.Any()).
					Return(int64(0), nil),
			)
			mockListImage()
			gomock.InOrder(
				storeMock.EXPECT().ImageBigDataDigest(gomock.Any(), gomock.Any()).
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

		It("should fail to get on wrong store image", func() {
			// Given
			mockGetRef()
			gomock.InOrder(
				storeMock.EXPECT().Image(gomock.Any()).Return(nil, t.TestError),
				storeMock.EXPECT().Image(gomock.Any()).Return(nil, t.TestError),
			)

			// When
			res, err := sut.ImageStatus(&types.SystemContext{}, testImageName)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})

		It("should fail to get on wrong image search", func() {
			// Given
			mockGetRef()
			gomock.InOrder(
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{ID: testImageName}, nil),
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{ID: testImageName}, nil),
				storeMock.EXPECT().ImageBigData(gomock.Any(), gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			res, err := sut.ImageStatus(&types.SystemContext{}, testImageName)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})

		It("should fail to get on wrong image config digest", func() {
			// Given
			mockGetRef()
			gomock.InOrder(
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{ID: testImageName}, nil),
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{ID: testImageName}, nil),
				storeMock.EXPECT().ImageBigData(gomock.Any(), gomock.Any()).
					Return(testManifest, nil),
				storeMock.EXPECT().ListImageBigData(gomock.Any()).
					Return([]string{""}, nil),
				storeMock.EXPECT().ImageBigDataSize(gomock.Any(), gomock.Any()).
					Return(int64(0), nil),
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{ID: testImageName}, nil),
				storeMock.EXPECT().ListImageBigData(gomock.Any()).
					Return([]string{""}, nil),
				storeMock.EXPECT().ImageBigDataSize(gomock.Any(), gomock.Any()).
					Return(int64(0), nil),
				storeMock.EXPECT().ImageBigData(gomock.Any(), gomock.Any()).
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
			mockLoop := func() {
				gomock.InOrder(
					storeMock.EXPECT().ImageBigData(gomock.Any(), gomock.Any()).
						Return(testManifest, nil),
					storeMock.EXPECT().ListImageBigData(gomock.Any()).
						Return([]string{""}, nil),
					storeMock.EXPECT().ImageBigDataSize(gomock.Any(), gomock.Any()).
						Return(int64(0), nil),
					storeMock.EXPECT().Image(gomock.Any()).
						Return(&cs.Image{ID: testImageName}, nil),
					storeMock.EXPECT().ImageBigDataDigest(gomock.Any(), gomock.Any()).
						Return(digest.Digest(""), nil),
				)
			}
			gomock.InOrder(
				storeMock.EXPECT().Images().Return(
					[]cs.Image{
						{ID: testSHA256, Names: []string{"a", "b", "c@sha256:" + testSHA256}},
						{ID: testSHA256}},
					nil),
			)
			mockParseStoreReference()
			mockListImage()
			mockLoop()
			mockParseStoreReference()
			mockListImage()
			mockLoop()

			// When
			res, err := sut.ListImages(&types.SystemContext{}, "")

			// Then
			Expect(err).To(BeNil())
			Expect(len(res)).To(Equal(2))
		})

		It("should succeed to list images with filter", func() {
			// Given
			mockGetRef()
			gomock.InOrder(
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{ID: testImageName}, nil),
			)
			mockListImage()
			gomock.InOrder(
				storeMock.EXPECT().ImageBigData(gomock.Any(), gomock.Any()).
					Return(testManifest, nil),
				storeMock.EXPECT().ListImageBigData(gomock.Any()).
					Return([]string{""}, nil),
				storeMock.EXPECT().ImageBigDataSize(gomock.Any(), gomock.Any()).
					Return(int64(0), nil),
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{ID: testImageName,
						Names:  []string{"a", "b", "c"},
						Digest: "digest"}, nil),
			)

			// When
			res, err := sut.ListImages(&types.SystemContext{}, testImageName)

			// Then
			Expect(err).To(BeNil())
			Expect(len(res)).To(Equal(1))
			Expect(res[0].ID).To(Equal(testImageName))
		})

		It("should succeed to list images on wrong image retrieval", func() {
			// Given
			mockGetRef()
			gomock.InOrder(
				storeMock.EXPECT().Image(gomock.Any()).Return(nil, t.TestError),
				storeMock.EXPECT().Image(gomock.Any()).Return(nil, t.TestError),
			)

			// When
			res, err := sut.ListImages(&types.SystemContext{}, testImageName)

			// Then
			Expect(err).To(BeNil())
			Expect(len(res)).To(Equal(0))
		})

		It("should fail to list images with filter on wrong reference", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Image(gomock.Any()).Return(nil, t.TestError),
			)
			// When
			res, err := sut.ListImages(&types.SystemContext{}, "wrong://image")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})

		It("should fail to list images with filter on wrong append cache", func() {
			// Given
			mockGetRef()
			gomock.InOrder(
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{ID: testImageName}, nil),
			)
			mockListImage()
			gomock.InOrder(
				storeMock.EXPECT().ImageBigData(gomock.Any(), gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			res, err := sut.ListImages(&types.SystemContext{}, testImageName)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})

		It("should fail to list images witout filter on wrong store", func() {
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

		It("should fail to list multiple images without filter on invalid ref", func() {
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
			gomock.InOrder(
				storeMock.EXPECT().Images().Return(
					[]cs.Image{{ID: testSHA256}}, nil),
			)
			mockParseStoreReference()
			gomock.InOrder(
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
			const imageName = "tarball:../../test/testdata/image.tar"

			// When
			res, err := sut.PrepareImage(imageName, &copy.Options{})

			// Then
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
		})

		It("should fail on invalid image name", func() {
			// Given
			// When
			res, err := sut.PrepareImage("", &copy.Options{})

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
			mockParseStoreReference()

			// When
			res, err := sut.PullImage(&types.SystemContext{
				SignaturePolicyPath: "../../test/policy.json",
			}, imageName, &copy.Options{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})

		It("should fail on canonical copy image", func() {
			// Given
			const imageName = "docker://localhost/busybox@sha256:" + testSHA256
			mockParseStoreReference()

			// When
			res, err := sut.PullImage(&types.SystemContext{
				SignaturePolicyPath: "../../test/policy.json",
			}, imageName, &copy.Options{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})
	})
})
