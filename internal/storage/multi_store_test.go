package storage_test

import (
	cstorage "github.com/containers/storage"
	"github.com/cri-o/cri-o/internal/storage"
	containerstoragemock "github.com/cri-o/cri-o/test/mocks/containerstorage"
	criostoragemock "github.com/cri-o/cri-o/test/mocks/criostorage"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MultiStore", func() {
	// Prepare the system under test
	const (
		defaultStorage    = "default"
		additionalStorage = "additionalStorage"
	)
	var (
		mockCtrl            *gomock.Controller
		storeDefaultMock    *containerstoragemock.MockStore
		storeAdditionalMock *containerstoragemock.MockStore
		runRoot             string
		graphRoot           string
	)

	// The system under test
	var sut storage.MultiStore

	BeforeEach(func() {
		// Setup the mocks
		mockCtrl = gomock.NewController(GinkgoT())
		storeDefaultMock = containerstoragemock.NewMockStore(mockCtrl)
		storeAdditionalMock = containerstoragemock.NewMockStore(mockCtrl)
		storeMap := make(map[string]cstorage.Store)
		storeMap[defaultStorage] = storeDefaultMock
		storeMap[additionalStorage] = storeAdditionalMock
		sut = storage.NewMultiStore(storeMap, defaultStorage, runRoot, graphRoot)
	})
	AfterEach(func() {
		mockCtrl.Finish()
	})
	t.Describe("Containers", func() {
		It("should succeed to get all the containers", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Containers().
					Return([]cstorage.Container{{}}, nil),
				storeAdditionalMock.EXPECT().Containers().
					Return([]cstorage.Container{{}}, nil),
			)
			// When
			containers, err := sut.Containers()

			// Then
			Expect(err).To(BeNil())
			Expect(len(containers)).To(Equal(2))
		})
		It("should fail to get containers from default storage", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Containers().
					Return([]cstorage.Container{}, t.TestError),
				storeAdditionalMock.EXPECT().Containers().
					Return([]cstorage.Container{{}}, nil),
			)
			// When
			containers, err := sut.Containers()

			// Then
			Expect(err).NotTo(BeNil())
			Expect(len(containers)).To(Equal(1))
		})
		It("should fail to get containers from additional storage", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Containers().
					Return([]cstorage.Container{{}}, nil),
				storeAdditionalMock.EXPECT().Containers().
					Return([]cstorage.Container{}, t.TestError),
			)
			// When
			containers, err := sut.Containers()

			// Then
			Expect(err).NotTo(BeNil())
			Expect(len(containers)).To(Equal(1))
		})
	})

	t.Describe("Unmount", func() {
		It("should succeed to unmount container from default storage", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Lookup(gomock.Any()).
					Return("test", nil),
				storeDefaultMock.EXPECT().Unmount(gomock.Any(), gomock.Any()).
					Return(true, nil),
			)
			// When
			_, err := sut.Unmount("", false)

			// Then
			Expect(err).To(BeNil())
		})
		It("should succeed to unmount container from additional storage", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Lookup(gomock.Any()).
					Return("", nil),
				storeAdditionalMock.EXPECT().Lookup(gomock.Any()).
					Return("test", nil),
				storeAdditionalMock.EXPECT().Unmount(gomock.Any(), gomock.Any()).
					Return(true, nil),
			)
			// When
			_, err := sut.Unmount("", false)

			// Then
			Expect(err).To(BeNil())
		})
		It("should fail to unmount container from default storage", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Lookup(gomock.Any()).
					Return("test", nil),
				storeDefaultMock.EXPECT().Unmount(gomock.Any(), gomock.Any()).
					Return(false, t.TestError),
			)
			// When
			_, err := sut.Unmount("", false)

			// Then
			Expect(err).NotTo(BeNil())
		})
		It("should fail to unmount container from additional storage", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Lookup(gomock.Any()).
					Return("", nil),
				storeAdditionalMock.EXPECT().Lookup(gomock.Any()).
					Return("test", nil),
				storeAdditionalMock.EXPECT().Unmount(gomock.Any(), gomock.Any()).
					Return(false, t.TestError),
			)
			// When
			_, err := sut.Unmount("", false)

			// Then
			Expect(err).NotTo(BeNil())
		})
		It("should fail in finding the container", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Lookup(gomock.Any()).
					Return("", nil),
				storeAdditionalMock.EXPECT().Lookup(gomock.Any()).
					Return("", nil),
			)
			// When
			_, err := sut.Unmount("", false)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
	t.Describe("Metadata", func() {
		It("should succeed to get the metadata for a container from default storage", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Lookup(gomock.Any()).
					Return("test", nil),
				storeDefaultMock.EXPECT().Metadata(gomock.Any()).
					Return("", nil),
			)
			// When
			_, err := sut.Metadata("")

			// Then
			Expect(err).To(BeNil())
		})
		It("should succeed to get the metadata for a container from additional storage", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Lookup(gomock.Any()).
					Return("", nil),
				storeAdditionalMock.EXPECT().Lookup(gomock.Any()).
					Return("test", nil),
				storeAdditionalMock.EXPECT().Metadata(gomock.Any()).
					Return("", nil),
			)
			// When
			_, err := sut.Metadata("")

			// Then
			Expect(err).To(BeNil())
		})
		It("should fail to get the metadata for a container from default storage", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Lookup(gomock.Any()).
					Return("test", nil),
				storeDefaultMock.EXPECT().Metadata(gomock.Any()).
					Return("", t.TestError),
			)
			// When
			_, err := sut.Metadata("")

			// Then
			Expect(err).NotTo(BeNil())
		})
		It("should fail to get the metadata for a container from additional storage", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Lookup(gomock.Any()).
					Return("", nil),
				storeAdditionalMock.EXPECT().Lookup(gomock.Any()).
					Return("test", nil),
				storeAdditionalMock.EXPECT().Metadata(gomock.Any()).
					Return("", t.TestError),
			)
			// When
			_, err := sut.Metadata("")

			// Then
			Expect(err).NotTo(BeNil())
		})
		It("should fail in finding the container", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Lookup(gomock.Any()).
					Return("", nil),
				storeAdditionalMock.EXPECT().Lookup(gomock.Any()).
					Return("", nil),
			)
			// When
			_, err := sut.Metadata("")

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
	t.Describe("DeleteContainer", func() {
		It("should succeed deleting a container from default storage", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Lookup(gomock.Any()).
					Return("test", nil),
				storeDefaultMock.EXPECT().DeleteContainer(gomock.Any()).
					Return(nil),
			)
			// When
			err := sut.DeleteContainer("")

			// Then
			Expect(err).To(BeNil())
		})
		It("should succeed deleting a container from additional storage", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Lookup(gomock.Any()).
					Return("", nil),
				storeAdditionalMock.EXPECT().Lookup(gomock.Any()).
					Return("test", nil),
				storeAdditionalMock.EXPECT().DeleteContainer(gomock.Any()).
					Return(nil),
			)
			// When
			err := sut.DeleteContainer("")

			// Then
			Expect(err).To(BeNil())
		})
		It("should fail to delete a container from default storage", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Lookup(gomock.Any()).
					Return("test", nil),
				storeDefaultMock.EXPECT().DeleteContainer(gomock.Any()).
					Return(t.TestError),
			)
			// When
			err := sut.DeleteContainer("")

			// Then
			Expect(err).NotTo(BeNil())
		})
		It("should fail to delete a container from additional storage", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Lookup(gomock.Any()).
					Return("", nil),
				storeAdditionalMock.EXPECT().Lookup(gomock.Any()).
					Return("test", nil),
				storeAdditionalMock.EXPECT().DeleteContainer(gomock.Any()).
					Return(t.TestError),
			)
			// When
			err := sut.DeleteContainer("")

			// Then
			Expect(err).NotTo(BeNil())
		})
		It("should fail in finding the container", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Lookup(gomock.Any()).
					Return("", nil),
				storeAdditionalMock.EXPECT().Lookup(gomock.Any()).
					Return("", nil),
			)
			// When
			err := sut.DeleteContainer("")

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
	t.Describe("DeleteImage", func() {
		It("should succeed deleting an image from default storage", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Lookup(gomock.Any()).
					Return("test", nil),
				storeDefaultMock.EXPECT().DeleteImage(gomock.Any(), gomock.Any()).
					Return([]string{}, nil),
			)
			// When
			_, err := sut.DeleteImage("", false)

			// Then
			Expect(err).To(BeNil())
		})
		It("should succeed deleting an image from additional storage", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Lookup(gomock.Any()).
					Return("", nil),
				storeAdditionalMock.EXPECT().Lookup(gomock.Any()).
					Return("test", nil),
				storeAdditionalMock.EXPECT().DeleteImage(gomock.Any(), gomock.Any()).
					Return([]string{}, nil),
			)
			// When
			_, err := sut.DeleteImage("", false)

			// Then
			Expect(err).To(BeNil())
		})
		It("should fail to delete an image from default storage", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Lookup(gomock.Any()).
					Return("test", nil),
				storeDefaultMock.EXPECT().DeleteImage(gomock.Any(), gomock.Any()).
					Return([]string{}, t.TestError),
			)
			// When
			_, err := sut.DeleteImage("", false)

			// Then
			Expect(err).NotTo(BeNil())
		})
		It("should fail to delete an image from additional storage", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Lookup(gomock.Any()).
					Return("", nil),
				storeAdditionalMock.EXPECT().Lookup(gomock.Any()).
					Return("test", nil),
				storeAdditionalMock.EXPECT().DeleteImage(gomock.Any(), gomock.Any()).
					Return([]string{}, t.TestError),
			)
			// When
			_, err := sut.DeleteImage("", false)

			// Then
			Expect(err).NotTo(BeNil())
		})
		It("should fail in finding the container", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Lookup(gomock.Any()).
					Return("", nil),
				storeAdditionalMock.EXPECT().Lookup(gomock.Any()).
					Return("", nil),
			)
			// When
			_, err := sut.DeleteImage("", false)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
	t.Describe("GetStoreForContainer", func() {
		It("should succeed getting the store for a container from the default storage", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Container(gomock.Any()).
					Return(nil, nil),
			)
			// When
			_, err := sut.GetStoreForContainer("default")

			// Then
			Expect(err).To(BeNil())
		})
		It("should succeed getting the store for a container from the additional storage", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Container(gomock.Any()).
					Return(nil, t.TestError),
				storeAdditionalMock.EXPECT().Container(gomock.Any()).
					Return(nil, nil),
			)
			// When
			_, err := sut.GetStoreForContainer("")

			// Then
			Expect(err).To(BeNil())
		})
		It("should fail getting the store", func() {
			gomock.InOrder(
				storeDefaultMock.EXPECT().Container(gomock.Any()).
					Return(nil, t.TestError),
				storeAdditionalMock.EXPECT().Container(gomock.Any()).
					Return(nil, t.TestError),
			)
			// When
			_, err := sut.GetStoreForContainer("")

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})

var _ = Describe("MultiStoreServer", func() {
	// Prepare the system under test
	const (
		defaultStorage    = "default"
		additionalStorage = "additionalStorage"
	)
	var (
		mockCtrl        *gomock.Controller
		storeMock       *containerstoragemock.MockStore
		imageServerMock *criostoragemock.MockImageServer
		multiStoreMock  *criostoragemock.MockMultiStore
	)

	// The system under test
	var sut storage.MultiStoreServer

	BeforeEach(func() {
		// Setup the mocks
		mockCtrl = gomock.NewController(GinkgoT())
		multiStoreMock = criostoragemock.NewMockMultiStore(mockCtrl)
		storeMock = containerstoragemock.NewMockStore(mockCtrl)
		imageServerMock = criostoragemock.NewMockImageServer(mockCtrl)
		storeServerMap := make(map[string]storage.ImageServer)
		storeServerMap[defaultStorage] = imageServerMock
		storeServerMap[additionalStorage] = imageServerMock
		sut = storage.NewMultiStoreServer(storeServerMap, multiStoreMock)
	})
	AfterEach(func() {
		mockCtrl.Finish()
	})

	mockCreateMultiStoreServerIterator := func() mockSequence {
		return inOrder(
			multiStoreMock.EXPECT().GetDefaultStorageDriver().Return(defaultStorage),
		)
	}

	mockGetStoreForContainer := func() mockSequence {
		return inOrder(
			multiStoreMock.EXPECT().GetDefaultStorageDriver().Return(defaultStorage),
			imageServerMock.EXPECT().GetStore().
				Return(storeMock),
			storeMock.EXPECT().Container(gomock.Any()).
				Return(nil, nil),
			imageServerMock.EXPECT().GetStore().
				Return(storeMock),
		)
	}

	t.Describe("GetImageServer", func() {
		It("should succeed getting image server for default storage driver", func() {
			// When
			is, err := sut.GetImageServer(defaultStorage)

			// Then
			Expect(err).To(BeNil())
			Expect(is).To(Equal(imageServerMock))
		})
		It("should succeed getting image server for additional storage driver", func() {
			// When
			is, err := sut.GetImageServer(additionalStorage)

			// Then
			Expect(err).To(BeNil())
			Expect(is).To(Equal(imageServerMock))
		})
		It("should get default image server when the requested storage driver is not found", func() {
			// When
			is, err := sut.GetImageServer(defaultStorage)

			// Then
			Expect(err).To(BeNil())
			Expect(is).To(Equal(imageServerMock))
		})
	})
	t.Describe("ListAllImages", func() {
		It("should succeed listing all the images", func() {
			mockCreateMultiStoreServerIterator()
			gomock.InOrder(
				imageServerMock.EXPECT().ListImages(gomock.Any(), gomock.Any()).
					Return([]storage.ImageResult{{}}, nil),
				imageServerMock.EXPECT().ListImages(gomock.Any(), gomock.Any()).
					Return([]storage.ImageResult{{}}, nil),
			)
			// When
			images, err := sut.ListAllImages(nil, "")
			// Then
			Expect(err).To(BeNil())
			Expect(len(images)).To(Equal(2))
		})
		It("should fail listing the images for the default storage", func() {
			mockCreateMultiStoreServerIterator()
			gomock.InOrder(
				imageServerMock.EXPECT().ListImages(gomock.Any(), gomock.Any()).
					Return([]storage.ImageResult{}, t.TestError),
				imageServerMock.EXPECT().ListImages(gomock.Any(), gomock.Any()).
					Return([]storage.ImageResult{{}}, nil),
			)
			// When
			images, err := sut.ListAllImages(nil, "")
			// Then
			Expect(err).NotTo(BeNil())
			Expect(len(images)).To(Equal(1))
		})
		It("should fail listing the images for the additional storage", func() {
			mockCreateMultiStoreServerIterator()
			gomock.InOrder(
				imageServerMock.EXPECT().ListImages(gomock.Any(), gomock.Any()).
					Return([]storage.ImageResult{{}}, nil),
				imageServerMock.EXPECT().ListImages(gomock.Any(), gomock.Any()).
					Return([]storage.ImageResult{}, t.TestError),
			)
			// When
			images, err := sut.ListAllImages(nil, "")
			// Then
			Expect(err).NotTo(BeNil())
			Expect(len(images)).To(Equal(1))
		})
		It("should fail listing all images", func() {
			mockCreateMultiStoreServerIterator()
			gomock.InOrder(
				imageServerMock.EXPECT().ListImages(gomock.Any(), gomock.Any()).
					Return([]storage.ImageResult{}, t.TestError),
				imageServerMock.EXPECT().ListImages(gomock.Any(), gomock.Any()).
					Return([]storage.ImageResult{}, t.TestError),
			)
			// When
			_, err := sut.ListAllImages(nil, "")
			// Then
			Expect(err).NotTo(BeNil())
		})
	})
	t.Describe("GetAllStores", func() {
		It("should get a single store", func() {
			gomock.InOrder(
				multiStoreMock.EXPECT().GetStore().
					Return(map[string]cstorage.Store{
						defaultStorage:    nil,
						additionalStorage: nil,
					}),
			)
			// When
			stores := sut.GetAllStores()
			// Then
			Expect(len(stores)).To(Equal(2))
		})
	})

	t.Describe("GetStoreForImage", func() {
		It("should succeed getting store for an image from standard storage", func() {
			mockCreateMultiStoreServerIterator()
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().
					Return(storeMock),
				storeMock.EXPECT().Image(gomock.Any()).
					Return(nil, nil),
				imageServerMock.EXPECT().GetStore().
					Return(storeMock).Times(2),
				storeMock.EXPECT().Image(gomock.Any()).
					Return(nil, nil),
				imageServerMock.EXPECT().GetStore().
					Return(storeMock).Times(1),
			)
			// When
			_, err := sut.GetStoreForImage("")
			// Then
			Expect(err).To(BeNil())
		})
		It("should succeed getting store for an image from additional storage", func() {
			mockCreateMultiStoreServerIterator()
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().
					Return(storeMock),
				storeMock.EXPECT().Image(gomock.Any()).
					Return(nil, t.TestError),
				imageServerMock.EXPECT().GetStore().
					Return(storeMock),
				storeMock.EXPECT().Image(gomock.Any()).
					Return(nil, nil),
				imageServerMock.EXPECT().GetStore().
					Return(storeMock),
			)
			// When
			_, err := sut.GetStoreForImage("")
			// Then
			Expect(err).To(BeNil())
		})
		It("should fail to get the store for an image", func() {
			mockCreateMultiStoreServerIterator()
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().
					Return(storeMock),
				storeMock.EXPECT().Image(gomock.Any()).
					Return(nil, t.TestError),
				imageServerMock.EXPECT().GetStore().
					Return(storeMock),
				storeMock.EXPECT().Image(gomock.Any()).
					Return(nil, t.TestError),
			)
			// When
			_, err := sut.GetStoreForImage("")
			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("GetStoreForContainer", func() {
		It("should succeed getting the store for a container from standard storage", func() {
			mockCreateMultiStoreServerIterator()
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().
					Return(storeMock),
				storeMock.EXPECT().Container(gomock.Any()).
					Return(nil, nil),
				imageServerMock.EXPECT().GetStore().
					Return(storeMock),
			)
			// When
			_, err := sut.GetStoreForContainer("")
			// Then
			Expect(err).To(BeNil())
		})
		It("should succeed getting the store for a container from additional storage", func() {
			mockCreateMultiStoreServerIterator()
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().
					Return(storeMock),
				storeMock.EXPECT().Container(gomock.Any()).
					Return(nil, t.TestError),
				imageServerMock.EXPECT().GetStore().
					Return(storeMock),
				storeMock.EXPECT().Container(gomock.Any()).
					Return(nil, nil),
				imageServerMock.EXPECT().GetStore().
					Return(storeMock),
			)
			// When
			_, err := sut.GetStoreForContainer("")
			// Then
			Expect(err).To(BeNil())
		})
		It("should fail to get the store for a container", func() {
			mockCreateMultiStoreServerIterator()
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().
					Return(storeMock),
				storeMock.EXPECT().Container(gomock.Any()).
					Return(nil, t.TestError),
				imageServerMock.EXPECT().GetStore().
					Return(storeMock),
				storeMock.EXPECT().Container(gomock.Any()).
					Return(nil, t.TestError),
			)
			// When
			_, err := sut.GetStoreForContainer("")
			// Then
			Expect(err).NotTo(BeNil())
		})
	})
	t.Describe("GetImageServerForImage", func() {
		It("should succeed getting image server for an image from standard storage", func() {
			mockCreateMultiStoreServerIterator()
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().
					Return(storeMock),
				storeMock.EXPECT().Image(gomock.Any()).
					Return(nil, nil),
				imageServerMock.EXPECT().GetStore().
					Return(storeMock),
				storeMock.EXPECT().Image(gomock.Any()).
					Return(nil, t.TestError),
			)
			// When
			_, err := sut.GetImageServerForImage("")
			// Then
			Expect(err).To(BeNil())
		})
		It("should succeed getting image server for an image from additional storage", func() {
			mockCreateMultiStoreServerIterator()
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().
					Return(storeMock),
				storeMock.EXPECT().Image(gomock.Any()).
					Return(nil, t.TestError),
				imageServerMock.EXPECT().GetStore().
					Return(storeMock),
				storeMock.EXPECT().Image(gomock.Any()).
					Return(nil, nil),
			)
			// When
			_, err := sut.GetImageServerForImage("")
			// Then
			Expect(err).To(BeNil())
		})
		It("should fail to get the image server for an image", func() {
			mockCreateMultiStoreServerIterator()
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().
					Return(storeMock),
				storeMock.EXPECT().Image(gomock.Any()).
					Return(nil, t.TestError),
				imageServerMock.EXPECT().GetStore().
					Return(storeMock),
				storeMock.EXPECT().Image(gomock.Any()).
					Return(nil, t.TestError),
			)
			// When
			_, err := sut.GetImageServerForImage("")
			// Then
			Expect(err).NotTo(BeNil())
		})
	})
	t.Describe("FromContainerDirectory", func() {
		It("should succeed getting from container directory ", func() {
			mockGetStoreForContainer()
			gomock.InOrder(
				storeMock.EXPECT().FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return(nil, nil),
			)
			// When
			_, err := sut.FromContainerDirectory("", "")
			// Then
			Expect(err).To(BeNil())
		})
		It("should fail to get from container directory ", func() {
			mockGetStoreForContainer()
			gomock.InOrder(
				storeMock.EXPECT().FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return(nil, t.TestError),
			)
			// When
			_, err := sut.FromContainerDirectory("", "")
			// Then
			Expect(err).NotTo(BeNil())
		})
	})
	t.Describe("ContainerRunDirectory", func() {
		It("should succeed getting run container directory ", func() {
			mockGetStoreForContainer()
			gomock.InOrder(
				storeMock.EXPECT().ContainerRunDirectory(gomock.Any()).
					Return("", nil),
			)
			// When
			_, err := sut.ContainerRunDirectory("")
			// Then
			Expect(err).To(BeNil())
		})
		It("should fail to get run container directory ", func() {
			mockGetStoreForContainer()
			gomock.InOrder(
				storeMock.EXPECT().ContainerRunDirectory(gomock.Any()).
					Return("", t.TestError),
			)
			// When
			_, err := sut.ContainerRunDirectory("")
			// Then
			Expect(err).NotTo(BeNil())
		})
	})
	t.Describe("ContainerDirectory", func() {
		It("should succeed getting container directory ", func() {
			mockGetStoreForContainer()
			gomock.InOrder(
				storeMock.EXPECT().ContainerDirectory(gomock.Any()).
					Return("", nil),
			)
			// When
			_, err := sut.ContainerDirectory("")
			// Then
			Expect(err).To(BeNil())
		})
		It("should fail to get container directory ", func() {
			mockGetStoreForContainer()
			gomock.InOrder(
				storeMock.EXPECT().ContainerDirectory(gomock.Any()).
					Return("", t.TestError),
			)
			// When
			_, err := sut.ContainerDirectory("")
			// Then
			Expect(err).NotTo(BeNil())
		})
	})
	t.Describe("Shutdown", func() {
		It("should succeed the shutdown", func() {
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().
					Return(storeMock),
				storeMock.EXPECT().Shutdown(gomock.Any()).
					Return([]string{}, nil),
				imageServerMock.EXPECT().GetStore().
					Return(storeMock),
				storeMock.EXPECT().Shutdown(gomock.Any()).
					Return([]string{}, nil),
			)
			// When
			_, err := sut.Shutdown(false)
			// Then
			Expect(err).To(BeNil())
		})
		It("should fail to shutdown for one of the storage driver", func() {
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().
					Return(storeMock),
				storeMock.EXPECT().Shutdown(gomock.Any()).
					Return([]string{}, t.TestError),
				imageServerMock.EXPECT().GetStore().
					Return(storeMock),
				storeMock.EXPECT().Shutdown(gomock.Any()).
					Return([]string{}, nil),
			)
			// When
			_, err := sut.Shutdown(false)
			// Then
			Expect(err).NotTo(BeNil())
		})
	})
	t.Describe("ResolveNames", func() {
		It("should succeed resolving names for the default storage driver", func() {
			mockCreateMultiStoreServerIterator()
			gomock.InOrder(
				imageServerMock.EXPECT().ResolveNames(gomock.Any(), gomock.Any()).
					Return([]string{}, nil),
			)
			// When
			_, err := sut.ResolveNames(nil, "")
			// Then
			Expect(err).To(BeNil())
		})
		It("should succeed resolving names for the additional storage driver", func() {
			mockCreateMultiStoreServerIterator()
			gomock.InOrder(
				imageServerMock.EXPECT().ResolveNames(gomock.Any(), gomock.Any()).
					Return([]string{}, t.TestError),
				imageServerMock.EXPECT().ResolveNames(gomock.Any(), gomock.Any()).
					Return([]string{}, nil),
			)
			// When
			_, err := sut.ResolveNames(nil, "")
			// Then
			Expect(err).To(BeNil())
		})
		It("should fail to resolve names", func() {
			mockCreateMultiStoreServerIterator()
			gomock.InOrder(
				imageServerMock.EXPECT().ResolveNames(gomock.Any(), gomock.Any()).
					Return([]string{}, t.TestError).Times(2),
			)
			// When
			_, err := sut.ResolveNames(nil, "")
			// Then
			Expect(err).NotTo(BeNil())
		})
	})
	t.Describe("ImageStatus", func() {
		It("should succeed getting the image status from the default storage driver", func() {
			gomock.InOrder(
				imageServerMock.EXPECT().ImageStatus(gomock.Any(), gomock.Any()).
					Return(nil, nil),
			)
			// When
			_, err := sut.ImageStatus(nil, "")
			// Then
			Expect(err).To(BeNil())
		})
		It("should succeed getting the image status from the additional storage driver", func() {
			gomock.InOrder(
				imageServerMock.EXPECT().ImageStatus(gomock.Any(), gomock.Any()).
					Return(nil, t.TestError),
				imageServerMock.EXPECT().ImageStatus(gomock.Any(), gomock.Any()).
					Return(nil, nil),
			)
			// When
			_, err := sut.ImageStatus(nil, "")
			// Then
			Expect(err).To(BeNil())
		})
		It("should fail to get status of the image", func() {
			gomock.InOrder(
				imageServerMock.EXPECT().ImageStatus(gomock.Any(), gomock.Any()).
					Return(nil, t.TestError).Times(2),
			)
			// When
			_, err := sut.ImageStatus(nil, "")
			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
