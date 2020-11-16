package lib_test

import (
	cstorage "github.com/containers/storage"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("ContainerServer", func() {
	// Prepare the sut
	BeforeEach(beforeEach)

	t.Describe("LookupSandbox", func() {
		It("should succeed", func() {
			// Given
			addContainerAndSandbox()

			// When
			sandbox, err := sut.LookupSandbox(sandboxID)

			// Then
			Expect(err).To(BeNil())
			Expect(sandbox).NotTo(BeNil())
		})

		It("should fail with empty ID", func() {
			// Given

			// When
			sandbox, err := sut.LookupSandbox("")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(sandbox).To(BeNil())
		})

		It("should fail when sandbox not within podIDIndex", func() {
			// Given
			Expect(sut.PodNameIndex().Reserve(sandboxID, sandboxID)).To(BeNil())

			// When
			sandbox, err := sut.LookupSandbox(sandboxID)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(sandbox).To(BeNil())
		})

		It("should fail when sandbox not available", func() {
			// Given
			Expect(sut.PodNameIndex().Reserve(sandboxID, sandboxID)).To(BeNil())
			Expect(sut.PodIDIndex().Add(sandboxID)).To(BeNil())

			// When
			sandbox, err := sut.LookupSandbox(sandboxID)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(sandbox).To(BeNil())
		})
	})

	t.Describe("LookupContainer", func() {
		It("should succeed", func() {
			// Given
			addContainerAndSandbox()

			// When
			container, err := sut.LookupContainer(containerID)

			// Then
			Expect(err).To(BeNil())
			Expect(container).NotTo(BeNil())
		})

		It("should fail with empty ID", func() {
			// Given

			// When
			container, err := sut.LookupContainer("")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(container).To(BeNil())
		})
	})

	t.Describe("GetContainerFromShortID", func() {
		It("should succeed", func() {
			// Given
			addContainerAndSandbox()

			// When
			container, err := sut.GetContainerFromShortID(containerID)

			// Then
			Expect(err).To(BeNil())
			Expect(container).NotTo(BeNil())
		})

		It("should fail with empty ID", func() {
			// Given

			// When
			container, err := sut.GetContainerFromShortID("")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(container).To(BeNil())
		})

		It("should fail with invalid ID", func() {
			// Given

			// When
			container, err := sut.GetContainerFromShortID("invalid")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(container).To(BeNil())
		})

		It("should fail if container is not created", func() {
			// Given
			Expect(sut.AddSandbox(mySandbox)).To(BeNil())
			sut.AddContainer(myContainer)
			Expect(sut.CtrIDIndex().Add(containerID)).To(BeNil())
			Expect(sut.PodIDIndex().Add(sandboxID)).To(BeNil())

			// When
			container, err := sut.GetContainerFromShortID(containerID)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(container).To(BeNil())
		})
	})

	t.Describe("GetContainerRootFsSize", func() {
		It("should succeed", func() {
			// Given
			layerSize := int64(10)
			gomock.InOrder(
				storeMock.EXPECT().Container(gomock.Any()).
					Return(&cstorage.Container{}, nil),
				storeMock.EXPECT().Layer(gomock.Any()).
					Return(&cstorage.Layer{}, nil),
				storeMock.EXPECT().Layer(gomock.Any()).
					Return(&cstorage.Layer{Parent: "parent"}, nil),
				storeMock.EXPECT().DiffSize(gomock.Any(), gomock.Any()).
					Return(layerSize, nil),
				storeMock.EXPECT().Layer(gomock.Any()).
					Return(&cstorage.Layer{}, nil),
				storeMock.EXPECT().DiffSize(gomock.Any(), gomock.Any()).
					Return(layerSize, nil),
			)

			// When
			size, err := sut.GetContainerRootFsSize("")

			// Then
			Expect(err).To(BeNil())
			Expect(size).To(BeEquivalentTo(2 * layerSize))
		})

		It("should fail when diffsize of parent fails", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Container(gomock.Any()).
					Return(&cstorage.Container{}, nil),
				storeMock.EXPECT().Layer(gomock.Any()).
					Return(&cstorage.Layer{}, nil),
				storeMock.EXPECT().Layer(gomock.Any()).
					Return(&cstorage.Layer{Parent: "parent"}, nil),
				storeMock.EXPECT().DiffSize(gomock.Any(), gomock.Any()).
					Return(int64(0), t.TestError),
			)

			// When
			size, err := sut.GetContainerRootFsSize("")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(size).To(BeEquivalentTo(0))
		})

		It("should fail when layer retrieval of parent fails", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Container(gomock.Any()).
					Return(&cstorage.Container{}, nil),
				storeMock.EXPECT().Layer(gomock.Any()).
					Return(&cstorage.Layer{}, nil),
				storeMock.EXPECT().Layer(gomock.Any()).
					Return(&cstorage.Layer{Parent: "parent"}, nil),
				storeMock.EXPECT().DiffSize(gomock.Any(), gomock.Any()).
					Return(int64(0), nil),
				storeMock.EXPECT().Layer(gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			size, err := sut.GetContainerRootFsSize("")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(size).To(BeEquivalentTo(0))
		})

		It("should fail when container retrieval fails", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Container(gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			size, err := sut.GetContainerRootFsSize("")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(size).To(BeEquivalentTo(0))
		})

		It("should fail when top layer retrieval fails", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Container(gomock.Any()).
					Return(&cstorage.Container{}, nil),
				storeMock.EXPECT().Layer(gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			size, err := sut.GetContainerRootFsSize("")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(size).To(BeEquivalentTo(0))
		})

		It("should fail when second layer retrieval fails", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Container(gomock.Any()).
					Return(&cstorage.Container{}, nil),
				storeMock.EXPECT().Layer(gomock.Any()).
					Return(&cstorage.Layer{}, nil),
				storeMock.EXPECT().Layer(gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			size, err := sut.GetContainerRootFsSize("")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(size).To(BeEquivalentTo(0))
		})
	})

	t.Describe("GetContainerRootFsSize", func() {
		It("should succeed", func() {
			// Given
			layerSize := int64(10)
			gomock.InOrder(
				storeMock.EXPECT().Container(gomock.Any()).
					Return(&cstorage.Container{}, nil),
				storeMock.EXPECT().Layer(gomock.Any()).
					Return(&cstorage.Layer{}, nil),
				storeMock.EXPECT().DiffSize(gomock.Any(), gomock.Any()).
					Return(layerSize, nil),
			)

			// When
			size, err := sut.GetContainerRwSize("")

			// Then
			Expect(err).To(BeNil())
			Expect(size).To(BeEquivalentTo(layerSize))
		})

		It("should fail if container retrieval fails", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Container(gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			size, err := sut.GetContainerRwSize("")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(size).To(BeEquivalentTo(0))
		})

		It("should fail if layer retrieval fails", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Container(gomock.Any()).
					Return(&cstorage.Container{}, nil),
				storeMock.EXPECT().Layer(gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			size, err := sut.GetContainerRwSize("")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(size).To(BeEquivalentTo(0))
		})

		It("should fail if diffsize fails", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Container(gomock.Any()).
					Return(&cstorage.Container{}, nil),
				storeMock.EXPECT().Layer(gomock.Any()).
					Return(&cstorage.Layer{}, nil),
				storeMock.EXPECT().DiffSize(gomock.Any(), gomock.Any()).
					Return(int64(0), t.TestError),
			)

			// When
			size, err := sut.GetContainerRwSize("")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(size).To(BeEquivalentTo(0))
		})
	})

	t.Describe("GetContainerTopLayerID", func() {
		It("should succeed", func() {
			// Given
			addContainerAndSandbox()
			gomock.InOrder(
				storeMock.EXPECT().Container(gomock.Any()).
					Return(&cstorage.Container{LayerID: containerID}, nil),
			)

			// When
			layerID, err := sut.GetContainerTopLayerID(containerID)

			// Then
			Expect(err).To(BeNil())
			Expect(layerID).To(Equal(containerID))
		})

		It("should fail when container retrieval fails", func() {
			// Given
			addContainerAndSandbox()
			gomock.InOrder(
				storeMock.EXPECT().Container(gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			layerID, err := sut.GetContainerTopLayerID(containerID)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(layerID).To(BeEmpty())
		})

		It("should fail on invalid container ID", func() {
			// Given

			// When
			layerID, err := sut.GetContainerTopLayerID("")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(layerID).To(BeEmpty())
		})
	})
})
