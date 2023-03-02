package storage_test

import (
	"os"

	"github.com/containers/image/v5/types"
	"github.com/cri-o/cri-o/internal/storage"
	criostoragemock "github.com/cri-o/cri-o/test/mocks/criostorage"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("Image", func() {
	// Test constants
	const (
		firstContainerID   = "containerID1"
		secondContainerID  = "containerID2"
		unknownContainerID = "unknownContainerID"
	)

	var (
		mockCtrl             *gomock.Controller
		dfltImageServerMock  *criostoragemock.MockImageServer
		ctridImageServerMock *criostoragemock.MockImageServer

		// The system under test
		sut storage.ImageServerList

		// The empty system context
		ctx *types.SystemContext
	)

	// Prepare the system under test
	BeforeEach(func() {
		// Setup the mocks
		mockCtrl = gomock.NewController(GinkgoT())
		dfltImageServerMock = criostoragemock.NewMockImageServer(mockCtrl)
		ctridImageServerMock = criostoragemock.NewMockImageServer(mockCtrl)

		// Setup the SUT
		ctx = &types.SystemContext{
			SystemRegistriesConfPath: t.MustTempFile("registries"),
		}

		sut = storage.GetImageServiceList(dfltImageServerMock)
		Expect(sut).NotTo(BeNil())

		sut.SetImageServer(firstContainerID, ctridImageServerMock)
	})
	AfterEach(func() {
		mockCtrl.Finish()
		Expect(os.Remove(ctx.SystemRegistriesConfPath)).To(BeNil())
	})

	// mockGetRef := func() mockSequence {
	// 	return inOrder(
	// 		// parseStoreReference ("@"+testImageName) will fail, recognizing it as an invalid image ID
	// 		storeMock.EXPECT().Image(testImageName).
	// 			Return(&cs.Image{ID: testSHA256}, nil),

	// 		mockParseStoreReference(storeMock, testImageName),
	// 	)
	// }

	t.Describe("GetImageServiceList", func() {
		It("should succeed to retrieve an image service", func() {
			// Given
			// When
			imageServiceList := storage.GetImageServiceList(dfltImageServerMock)

			// Then
			Expect(imageServiceList).NotTo(BeNil())
		})
	})

	t.Describe("GetDefaultImageServer", func() {
		It("should succeed to retrieve the default ImageServer", func() {
			// Given
			gomock.InOrder(
				dfltImageServerMock.EXPECT().GetStore().Return(nil),
			)

			// When
			is := sut.GetDefaultImageServer()

			// Then
			Expect(is).NotTo(BeNil())
			Expect(is.GetStore()).To(BeNil())
		})
	})

	t.Describe("SetDefaultImageServer", func() {
		It("should succeed to change default ImageServer", func() {
			// Given
			newImageServerMock := criostoragemock.NewMockImageServer(mockCtrl)
			gomock.InOrder(
				newImageServerMock.EXPECT().GetStore().Return(nil),
			)

			// When
			sut.SetDefaultImageServer(newImageServerMock)
			is := sut.GetDefaultImageServer()

			// Then
			Expect(is).NotTo(BeNil())
			Expect(is.GetStore()).To(BeNil())
		})
	})

	t.Describe("SetImageServer", func() {
		It("should succeed to register a new image server for a container", func() {
			// Given
			newImageServerMock := criostoragemock.NewMockImageServer(mockCtrl)
			gomock.InOrder(
				newImageServerMock.EXPECT().GetStore().Return(nil),
			)

			// When
			sut.SetImageServer(secondContainerID, newImageServerMock)
			is := sut.GetImageServer(secondContainerID)

			// Then
			Expect(is).NotTo(BeNil())
			Expect(is.GetStore()).To(BeNil())
		})
	})

	t.Describe("GetImageServer", func() {
		It("should return the default ImageServer for unknown container", func() {
			// Given
			gomock.InOrder(
				dfltImageServerMock.EXPECT().GetStore().Return(nil),
			)

			// When
			is := sut.GetImageServer(unknownContainerID)

			// Then
			Expect(is).NotTo(BeNil())
			Expect(is.GetStore()).To(BeNil())
		})

		It("should return the ImageServer associated to a container ID", func() {
			// Given
			gomock.InOrder(
				ctridImageServerMock.EXPECT().GetStore().Return(nil),
			)

			// When
			is := sut.GetImageServer(firstContainerID)

			// Then
			Expect(is).NotTo(BeNil())
			Expect(is.GetStore()).To(BeNil())
		})
	})

	t.Describe("DeleteImageServer", func() {
		It("should succeed to remove an ImageServer", func() {
			// Given
			gomock.InOrder(
				dfltImageServerMock.EXPECT().GetStore().Return(nil),
			)

			// When
			sut.DeleteImageServer(firstContainerID)
			is := sut.GetImageServer(firstContainerID) // should return the default IS

			// Then
			Expect(is).NotTo(BeNil())
			Expect(is.GetStore()).To(BeNil())
		})
	})

	t.Describe("ResolveNames", func() {
		It("should succeed to resolve with default ImageServer", func() {
			// Given
			gomock.InOrder(
				dfltImageServerMock.EXPECT().ResolveNames(gomock.Any(), gomock.Any()).
					Return([]string{}, nil),
			)

			// When
			names, err := sut.ResolveNames(nil, "")

			// Then
			Expect(err).To(BeNil())
			Expect(names).NotTo(BeNil())
		})

		It("should succeed to resolve, and return resolving ImageServer", func() {
			// Given
			gomock.InOrder(
				dfltImageServerMock.EXPECT().ResolveNames(gomock.Any(), gomock.Any()).
					Return(nil, t.TestError),
				ctridImageServerMock.EXPECT().ResolveNames(gomock.Any(), gomock.Any()).
					Return([]string{}, nil),
			)

			// When
			names, err := sut.ResolveNames(nil, "")

			// Then
			Expect(err).To(BeNil())
			Expect(names).NotTo(BeNil())
		})

		It("should fail when no ImageServer can resolve", func() {
			// Given
			gomock.InOrder(
				dfltImageServerMock.EXPECT().ResolveNames(gomock.Any(), gomock.Any()).
					Return(nil, t.TestError),
				ctridImageServerMock.EXPECT().ResolveNames(gomock.Any(), gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			_, err := sut.ResolveNames(nil, "")

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("ListImages", func() {
		It("should list images from all registered ImageServer", func() {
			gomock.InOrder(
				dfltImageServerMock.EXPECT().ListImages(gomock.Any(), gomock.Any()).
					Return([]storage.ImageResult{{}}, nil),
				ctridImageServerMock.EXPECT().ListImages(gomock.Any(), gomock.Any()).
					Return([]storage.ImageResult{{}}, nil),
			)
			// When
			images, err := sut.ListImages(nil, "")
			// Then
			Expect(err).To(BeNil())
			Expect(len(images)).To(Equal(2))
		})
		It("should return the list even if the default ImageServer fail", func() {
			gomock.InOrder(
				dfltImageServerMock.EXPECT().ListImages(gomock.Any(), gomock.Any()).
					Return([]storage.ImageResult{}, t.TestError),
				ctridImageServerMock.EXPECT().ListImages(gomock.Any(), gomock.Any()).
					Return([]storage.ImageResult{{}}, nil),
			)
			// When
			images, err := sut.ListImages(nil, "")
			// Then
			Expect(err).NotTo(BeNil())
			Expect(len(images)).To(Equal(1))
		})
		It("should return the list even if an additional ImageServer fail", func() {
			gomock.InOrder(
				dfltImageServerMock.EXPECT().ListImages(gomock.Any(), gomock.Any()).
					Return([]storage.ImageResult{{}}, nil),
				ctridImageServerMock.EXPECT().ListImages(gomock.Any(), gomock.Any()).
					Return([]storage.ImageResult{}, t.TestError),
			)
			// When
			images, err := sut.ListImages(nil, "")
			// Then
			Expect(err).NotTo(BeNil())
			Expect(len(images)).To(Equal(1))
		})
		It("should fail when all ImageServer fail", func() {
			gomock.InOrder(
				dfltImageServerMock.EXPECT().ListImages(gomock.Any(), gomock.Any()).
					Return([]storage.ImageResult{}, t.TestError),
				ctridImageServerMock.EXPECT().ListImages(gomock.Any(), gomock.Any()).
					Return([]storage.ImageResult{}, t.TestError),
			)
			// When
			_, err := sut.ListImages(nil, "")
			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
