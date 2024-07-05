package storage_test

import (
	"context"

	istorage "github.com/containers/image/v5/storage"
	"github.com/containers/image/v5/types"
	cs "github.com/containers/storage"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/internal/mockutils"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/internal/storage/references"
	containerstoragemock "github.com/cri-o/cri-o/test/mocks/containerstorage"
	criostoragemock "github.com/cri-o/cri-o/test/mocks/criostorage"
)

// The actual test suite.
var _ = t.Describe("Runtime", func() {
	imageID, err := storage.ParseStorageImageIDFromOutOfProcessData("8a788232037eaf17794408ff3df6b922a1aedf9ef8de36afdae3ed0b0381907b")
	Expect(err).ToNot(HaveOccurred())

	var (
		mockCtrl             *gomock.Controller
		storeMock            *containerstoragemock.MockStore
		storageTransportMock *criostoragemock.MockStorageTransport
		imageServerMock      *criostoragemock.MockImageServer
	)

	// The system under test
	var sut storage.RuntimeServer

	var ctx context.Context

	// Prepare the system under test and register a test name and key before
	// each test
	BeforeEach(func() {
		// Setup the mocks
		mockCtrl = gomock.NewController(GinkgoT())
		storeMock = containerstoragemock.NewMockStore(mockCtrl)
		storageTransportMock = criostoragemock.NewMockStorageTransport(mockCtrl)
		imageServerMock = criostoragemock.NewMockImageServer(mockCtrl)

		sut = storage.GetRuntimeService(context.Background(), imageServerMock, storageTransportMock)
		Expect(sut).NotTo(BeNil())

		ctx = context.TODO()
	})
	AfterEach(func() {
		mockCtrl.Finish()
	})

	// The part of runtimeService.CreateContainer before a CreateContainer call, if the image already exists locally.
	mockCreateContainerImageExists := func() mockutils.MockSequence {
		return mockutils.InOrder(
			imageServerMock.EXPECT().GetStore().Return(storeMock),
			mockNewImage(storeMock, "", imageID.IDStringForOutOfProcessConsumptionOnly(), imageID.IDStringForOutOfProcessConsumptionOnly()),
			imageServerMock.EXPECT().GetStore().Return(storeMock),
		)
	}

	// The part of CreatePodSandbox before a CreateContainer call, if the image already exists locally.
	mockCreatePodSandboxImageExists := func() mockutils.MockSequence {
		return mockutils.InOrder(
			imageServerMock.EXPECT().GetStore().Return(storeMock),
			mockResolveReference(storeMock, storageTransportMock,
				"docker.io/library/imagename:latest", "",
				imageID.IDStringForOutOfProcessConsumptionOnly()),
			imageServerMock.EXPECT().GetStore().Return(storeMock),
			mockNewImage(storeMock, "", imageID.IDStringForOutOfProcessConsumptionOnly(), imageID.IDStringForOutOfProcessConsumptionOnly()),
			imageServerMock.EXPECT().GetStore().Return(storeMock),
		)
	}

	//nolint: dupl
	t.Describe("GetRunDir", func() {
		It("should succeed to retrieve the run dir", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Container(gomock.Any()).
					Return(&cs.Container{}, nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().ContainerRunDirectory(gomock.Any()).
					Return("dir", nil),
			)

			// When
			dir, err := sut.GetRunDir("")

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(dir).To(Equal("dir"))
		})

		It("should fail to retrieve the run dir on not existing container", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Container(gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			dir, err := sut.GetRunDir("")

			// Then
			Expect(err).To(HaveOccurred())
			Expect(dir).To(Equal(""))
		})

		It("should fail to retrieve the run dir on invalid container ID", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Container(gomock.Any()).
					Return(nil, cs.ErrContainerUnknown),
			)

			// When
			dir, err := sut.GetRunDir("")

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(storage.ErrInvalidContainerID))
			Expect(dir).To(Equal(""))
		})
	})

	//nolint: dupl
	t.Describe("GetWorkDir", func() {
		It("should succeed to retrieve the work dir", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Container(gomock.Any()).
					Return(&cs.Container{}, nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().ContainerDirectory(gomock.Any()).
					Return("dir", nil),
			)

			// When
			dir, err := sut.GetWorkDir("")

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(dir).To(Equal("dir"))
		})

		It("should fail to retrieve the work dir on not existing container", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Container(gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			dir, err := sut.GetWorkDir("")

			// Then
			Expect(err).To(HaveOccurred())
			Expect(dir).To(Equal(""))
		})

		It("should fail to retrieve the work dir on invalid container ID", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Container(gomock.Any()).
					Return(nil, cs.ErrContainerUnknown),
			)

			// When
			dir, err := sut.GetWorkDir("")

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(storage.ErrInvalidContainerID))
			Expect(dir).To(Equal(""))
		})
	})

	t.Describe("StopContainer", func() {
		It("should succeed to stop a container", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Container(gomock.Any()).
					Return(&cs.Container{}, nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Unmount(gomock.Any(), gomock.Any()).
					Return(true, nil),
			)

			// When
			err := sut.StopContainer(ctx, "id")

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail to stop a container on empty ID", func() {
			// Given
			// When
			err := sut.StopContainer(ctx, "")

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(storage.ErrInvalidContainerID))
		})

		It("should fail to stop a container on unknown container", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Container(gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			err := sut.StopContainer(ctx, "id")

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail to stop a container on unmount error", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Container(gomock.Any()).
					Return(&cs.Container{}, nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Unmount(gomock.Any(), gomock.Any()).
					Return(false, t.TestError),
			)

			// When
			err := sut.StopContainer(ctx, "id")

			// Then
			Expect(err).To(HaveOccurred())
		})
	})

	t.Describe("StartContainer", func() {
		It("should succeed to start a container", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Container(gomock.Any()).
					Return(&cs.Container{Metadata: "{}"}, nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Mount(gomock.Any(), gomock.Any()).
					Return("mount", nil),
			)

			// When
			mount, err := sut.StartContainer("id")

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(mount).To(Equal("mount"))
		})

		It("should fail to start a container on store error", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Container(gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			mount, err := sut.StartContainer("id")

			// Then
			Expect(err).To(HaveOccurred())
			Expect(mount).To(Equal(""))
		})

		It("should fail to start a container on unknown ID", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Container(gomock.Any()).
					Return(nil, cs.ErrContainerUnknown),
			)

			// When
			mount, err := sut.StartContainer("id")

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(storage.ErrInvalidContainerID))
			Expect(mount).To(Equal(""))
		})

		It("should fail to start a container on invalid metadata", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Container(gomock.Any()).
					Return(&cs.Container{Metadata: "invalid"}, nil),
			)

			// When
			mount, err := sut.StartContainer("id")

			// Then
			Expect(err).To(HaveOccurred())
			Expect(mount).To(Equal(""))
		})

		It("should fail to start a container on mount error", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Container(gomock.Any()).
					Return(&cs.Container{Metadata: "{}"}, nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Mount(gomock.Any(), gomock.Any()).
					Return("", t.TestError),
			)

			// When
			mount, err := sut.StartContainer("id")

			// Then
			Expect(err).To(HaveOccurred())
			Expect(mount).To(Equal(""))
		})
	})

	t.Describe("GetContainerMetadata", func() {
		It("should succeed to retrieve the container metadata", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Metadata(gomock.Any()).
					Return(`{"Pod": true}`, nil),
			)

			// When
			metadata, err := sut.GetContainerMetadata("id")

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(metadata).NotTo(BeNil())
			Expect(metadata.Pod).To(BeTrue())
		})

		It("should fail to retrieve the container metadata on store error", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Metadata(gomock.Any()).
					Return("", t.TestError),
			)

			// When
			metadata, err := sut.GetContainerMetadata("id")

			// Then
			Expect(err).To(HaveOccurred())
			Expect(metadata).NotTo(BeNil())
		})

		It("should fail to retrieve the container metadata on invalid JSON", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Metadata(gomock.Any()).
					Return("invalid", nil),
			)

			// When
			metadata, err := sut.GetContainerMetadata("id")

			// Then
			Expect(err).To(HaveOccurred())
			Expect(metadata).NotTo(BeNil())
		})
	})

	t.Describe("SetContainerMetadata", func() {
		It("should succeed to set the container metadata", func() {
			// Given
			metadata := &storage.RuntimeContainerMetadata{Pod: true}
			metadata.SetMountLabel("label")
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().SetMetadata(gomock.Any(), gomock.Any()).
					Return(nil),
			)

			// When
			err := sut.SetContainerMetadata("id", metadata)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail to set the container on store error", func() {
			// Given
			metadata := &storage.RuntimeContainerMetadata{Pod: true}
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().SetMetadata(gomock.Any(), gomock.Any()).
					Return(t.TestError),
			)

			// When
			err := sut.SetContainerMetadata("id", metadata)

			// Then
			Expect(err).To(HaveOccurred())
		})
	})

	t.Describe("DeleteContainer", func() {
		It("should succeed to delete a container", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Container(gomock.Any()).
					Return(&cs.Container{}, nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Layer("").Return(nil, nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().DeleteContainer(gomock.Any()).
					Return(nil),
			)

			// When
			err := sut.DeleteContainer(ctx, "id")

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail to delete a container on invalid ID", func() {
			// Given
			// When
			err := sut.DeleteContainer(ctx, "")

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(storage.ErrInvalidContainerID))
		})

		It("should fail to delete a container on store retrieval error", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Container(gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			err := sut.DeleteContainer(ctx, "id")

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail to delete a container on store deletion error", func() {
			// Given
			gomock.InOrder(
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Container(gomock.Any()).
					Return(&cs.Container{}, nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Layer("").Return(nil, nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().DeleteContainer(gomock.Any()).
					Return(t.TestError),
			)

			// When
			err := sut.DeleteContainer(ctx, "id")

			// Then
			Expect(err).To(HaveOccurred())
		})
	})

	t.Describe("CreateContainer/CreatePodSandbox", func() {
		t.Describe("success", func() {
			var (
				info storage.ContainerInfo
				err  error
			)

			mockUnderlyingCreateContainerSuccess := func() mockutils.MockSequence {
				return mockutils.InOrder(
					storeMock.EXPECT().CreateContainer(gomock.Any(), gomock.Any(),
						gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
						Return(&cs.Container{ID: "id"}, nil),
					imageServerMock.EXPECT().GetStore().Return(storeMock),
					storeMock.EXPECT().AddNames(gomock.Any(), gomock.Any()).Return(nil),
					imageServerMock.EXPECT().GetStore().Return(storeMock),
					storeMock.EXPECT().ContainerDirectory(gomock.Any()).
						Return("dir", nil),
					imageServerMock.EXPECT().GetStore().Return(storeMock),
					storeMock.EXPECT().ContainerRunDirectory(gomock.Any()).
						Return("runDir", nil),
				)
			}

			It("should succeed to create a container", func() {
				// Given
				mockutils.InOrder(
					mockCreateContainerImageExists(),
					mockUnderlyingCreateContainerSuccess(),
				)

				// When
				info, err = sut.CreateContainer(&types.SystemContext{},
					"podName", "podID", "imagename", imageID,
					"containerName", "containerID", "",
					0, nil, []string{"mountLabel"}, false,
				)
			})

			It("should succeed to create a pod sandbox", func() {
				// Given
				pauseImage, err2 := references.ParseRegistryImageReferenceFromOutOfProcessData("imagename:latest")
				Expect(err2).ToNot(HaveOccurred())
				mockutils.InOrder(
					mockCreatePodSandboxImageExists(),
					mockUnderlyingCreateContainerSuccess(),
				)

				// When
				info, err = sut.CreatePodSandbox(&types.SystemContext{},
					"podName", "podID", pauseImage, "",
					"containerName", "metadataName",
					"uid", "namespace", 0, nil, []string{"mountLabel"}, false,
				)
			})

			AfterEach(func() {
				// Then
				Expect(err).ToNot(HaveOccurred())
				Expect(info).NotTo(BeNil())
				Expect(info.ID).To(Equal("id"))
				Expect(info.Dir).To(Equal("dir"))
				Expect(info.RunDir).To(Equal("runDir"))
			})
		})

		It("should fail to create a container on invalid pod ID", func() {
			// Given

			// When
			_, err := sut.CreateContainer(&types.SystemContext{},
				"podName", "", "imagename", imageID,
				"containerName", "containerID", "metadataName",
				0, nil, []string{"mountLabel"}, false,
			)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(storage.ErrInvalidPodName))
		})

		It("should fail to create a container on invalid pod name", func() {
			// Given

			// When
			_, err := sut.CreateContainer(&types.SystemContext{},
				"", "podID", "imagename", imageID,
				"containerName", "containerID", "metadataName",
				0, nil, []string{"mountLabel"}, false,
			)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(storage.ErrInvalidPodName))
		})

		It("should fail to create a container on invalid container name", func() {
			// Given

			// When
			_, err := sut.CreateContainer(&types.SystemContext{},
				"podName", "podID", "imagename", imageID,
				"", "containerID", "metadataName",
				0, nil, []string{"mountLabel"}, false,
			)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(storage.ErrInvalidContainerName))
		})

		It("should fail to create a container on run dir error", func() {
			// Given
			mockutils.InOrder(
				mockCreateContainerImageExists(),
				storeMock.EXPECT().CreateContainer(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&cs.Container{ID: "id"}, nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().AddNames(gomock.Any(), gomock.Any()).Return(nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().ContainerDirectory(gomock.Any()).
					Return("dir", nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().ContainerRunDirectory(gomock.Any()).
					Return("", t.TestError),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().DeleteContainer(gomock.Any()).Return(nil),
			)

			// When
			_, err := sut.CreateContainer(&types.SystemContext{},
				"podName", "podID", "imagename", imageID,
				"containerName", "containerID", "metadataName",
				0, nil, []string{"mountLabel"}, false,
			)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail to create a container on container dir error", func() {
			// Given
			mockutils.InOrder(
				mockCreateContainerImageExists(),
				storeMock.EXPECT().CreateContainer(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&cs.Container{ID: "id"}, nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().AddNames(gomock.Any(), gomock.Any()).Return(nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().ContainerDirectory(gomock.Any()).
					Return("", t.TestError),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().DeleteContainer(gomock.Any()).Return(t.TestError),
			)

			// When
			_, err := sut.CreateContainer(&types.SystemContext{},
				"podName", "podID", "imagename", imageID,
				"containerName", "containerID", "metadataName",
				0, nil, []string{"mountLabel"}, false,
			)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail to create a pod sandbox on set names error", func() {
			// Given
			pauseImage, err := references.ParseRegistryImageReferenceFromOutOfProcessData("imagename:latest")
			Expect(err).ToNot(HaveOccurred())
			mockutils.InOrder(
				mockCreatePodSandboxImageExists(),
				storeMock.EXPECT().CreateContainer(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&cs.Container{ID: "id"}, nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().AddNames(gomock.Any(), gomock.Any()).
					Return(t.TestError),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().DeleteContainer(gomock.Any()).Return(t.TestError),
			)

			// When
			_, err = sut.CreatePodSandbox(&types.SystemContext{},
				"podName", "podID", pauseImage, "",
				"containerName", "metadataName",
				"uid", "namespace", 0, nil, []string{"mountLabel"}, false,
			)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail to create a pod sandbox on main creation error", func() {
			// Given
			pauseImage, err := references.ParseRegistryImageReferenceFromOutOfProcessData("imagename:latest")
			Expect(err).ToNot(HaveOccurred())
			mockutils.InOrder(
				mockCreatePodSandboxImageExists(),
				storeMock.EXPECT().CreateContainer(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			_, err = sut.CreatePodSandbox(&types.SystemContext{},
				"podName", "podID", pauseImage, "",
				"containerName", "metadataName",
				"uid", "namespace", 0, nil, []string{"mountLabel"}, false,
			)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail to create a container on main creation error", func() {
			// Given
			mockutils.InOrder(
				mockCreateContainerImageExists(),
				storeMock.EXPECT().CreateContainer(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			_, err := sut.CreateContainer(&types.SystemContext{},
				"podName", "podID", "imagename", imageID,
				"containerName", "containerID", "metadataName",
				0, nil, []string{"mountLabel"}, false,
			)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail to create a container on error accessing local image", func() {
			// Given
			mockutils.InOrder(
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				// storageReference.newImage:
				mockResolveImage(storeMock, "", imageID.IDStringForOutOfProcessConsumptionOnly(), imageID.IDStringForOutOfProcessConsumptionOnly()),
				storeMock.EXPECT().ImageBigData(gomock.Any(), gomock.Any()).
					Return(testManifest, nil),
				storeMock.EXPECT().ListImageBigData(gomock.Any()).
					Return([]string{""}, nil),
				storeMock.EXPECT().ImageBigDataSize(gomock.Any(), gomock.Any()).
					Return(int64(0), t.TestError),
			)

			// When
			_, err := sut.CreateContainer(&types.SystemContext{},
				"podName", "podID", "imagename", imageID,
				"containerName", "containerID", "metadataName",
				0, nil, []string{"mountLabel"}, false,
			)

			// Then
			Expect(err).To(HaveOccurred())
		})
	})

	t.Describe("pauseImage", func() {
		pauseImage, err := references.ParseRegistryImageReferenceFromOutOfProcessData("pauseimagename:latest")
		Expect(err).ToNot(HaveOccurred())

		var info storage.ContainerInfo

		mockCreatePodSandboxExpectingCopyOptions := func(expectedCopyOptions *storage.ImageCopyOptions) {
			pauseImageRef, err := references.ParseRegistryImageReferenceFromOutOfProcessData("docker.io/library/pauseimagename:latest")
			Expect(err).ToNot(HaveOccurred())
			pulledRef, err := istorage.Transport.NewStoreReference(storeMock, pauseImageRef.Raw(), "")
			Expect(err).ToNot(HaveOccurred())
			mockutils.InOrder(
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				mockResolveReference(storeMock, storageTransportMock,
					"docker.io/library/pauseimagename:latest", "", ""),
				imageServerMock.EXPECT().PullImage(gomock.Any(), pauseImageRef, expectedCopyOptions).Return(pulledRef, nil),
				mockResolveReference(storeMock, storageTransportMock,
					"docker.io/library/pauseimagename:latest", "", imageID.IDStringForOutOfProcessConsumptionOnly()),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				mockNewImage(storeMock, "", imageID.IDStringForOutOfProcessConsumptionOnly(), imageID.IDStringForOutOfProcessConsumptionOnly()),

				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().CreateContainer(gomock.Any(), gomock.Any(),
					imageID.IDStringForOutOfProcessConsumptionOnly(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&cs.Container{ID: "id"}, nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().AddNames(gomock.Any(), gomock.Any()).Return(nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().ContainerDirectory(gomock.Any()).
					Return("dir", nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().ContainerRunDirectory(gomock.Any()).
					Return("runDir", nil),
			)
		}

		It("should pull pauseImage if not available locally, using default credentials", func() {
			// The system under test
			sut := storage.GetRuntimeService(context.Background(), imageServerMock, storageTransportMock)
			Expect(sut).NotTo(BeNil())

			// Given
			mockCreatePodSandboxExpectingCopyOptions(&storage.ImageCopyOptions{
				SourceCtx:      &types.SystemContext{},
				DestinationCtx: &types.SystemContext{},
			})

			// When
			info, err = sut.CreatePodSandbox(&types.SystemContext{},
				"podName", "podID", pauseImage, "",
				"containerName", "metadataName",
				"uid", "namespace", 0, nil, []string{"mountLabel"}, false,
			)
		})

		It("should pull pauseImage if not available locally, using provided credential file", func() {
			// The system under test
			sut := storage.GetRuntimeService(context.Background(), imageServerMock, storageTransportMock)
			Expect(sut).NotTo(BeNil())

			// Given
			mockCreatePodSandboxExpectingCopyOptions(&storage.ImageCopyOptions{
				SourceCtx:      &types.SystemContext{AuthFilePath: "/var/non-default/credentials.json"},
				DestinationCtx: &types.SystemContext{},
			})

			// When
			info, err = sut.CreatePodSandbox(&types.SystemContext{},
				"podName", "podID", pauseImage, "/var/non-default/credentials.json",
				"containerName", "metadataName",
				"uid", "namespace", 0, nil, []string{"mountLabel"}, false,
			)
		})

		AfterEach(func() {
			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(info).NotTo(BeNil())
			Expect(info.ID).To(Equal("id"))
			Expect(info.Dir).To(Equal("dir"))
			Expect(info.RunDir).To(Equal("runDir"))
		})
	})
})
