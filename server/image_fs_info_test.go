package server_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.podman.io/storage"
	drivers "go.podman.io/storage/drivers"
	"go.podman.io/storage/pkg/directory"
)

// fakeGraphDriver is a minimal graph driver that reports canned per-layer disk
// usage, so ImageFsInfo can be tested without a real storage backend.
type fakeGraphDriver struct {
	drivers.Driver

	usage map[string]*directory.DiskUsage
}

func (f *fakeGraphDriver) ReadWriteDiskUsage(id string) (*directory.DiskUsage, error) {
	u, ok := f.usage[id]
	if !ok {
		return nil, fmt.Errorf("no usage for layer %q", id)
	}

	return u, nil
}

// The actual test suite.
var _ = t.Describe("ImageFsInfo", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})
	AfterEach(afterEach)

	t.Describe("ImageFsInfo", func() {
		It("should sum all layer usage for a shared image filesystem", func() {
			// Given
			driver := &fakeGraphDriver{usage: map[string]*directory.DiskUsage{
				"image-layer-1":   {Size: 100, InodeCount: 1},
				"image-layer-2":   {Size: 200, InodeCount: 2},
				"container-layer": {Size: 50, InodeCount: 1},
			}}

			imageServerMock.EXPECT().GetStore().Return(storeMock)
			storeMock.EXPECT().Layers().Return([]storage.Layer{
				{ID: "image-layer-1"},
				{ID: "image-layer-2"},
				{ID: "container-layer"},
			}, nil)
			storeMock.EXPECT().Containers().Return([]storage.Container{
				{LayerID: "container-layer"},
			}, nil)
			storeMock.EXPECT().GraphDriver().Return(driver, nil)
			storeMock.EXPECT().ImageStore().Return("")
			storeMock.EXPECT().GraphRoot().Return("/var/lib/containers/storage")
			storeMock.EXPECT().GraphDriverName().Return("overlay")

			// When
			response, err := sut.ImageFsInfo(context.Background(), nil)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetImageFilesystems()).To(HaveLen(1))
			Expect(
				response.GetImageFilesystems()[0].GetUsedBytes().GetValue(),
			).To(BeEquivalentTo(350))
			Expect(
				response.GetImageFilesystems()[0].GetInodesUsed().GetValue(),
			).To(BeEquivalentTo(4))
			// A shared filesystem reports the same total for the container filesystem.
			Expect(response.GetContainerFilesystems()).To(HaveLen(1))
			Expect(
				response.GetContainerFilesystems()[0].GetUsedBytes().GetValue(),
			).To(BeEquivalentTo(350))
			Expect(
				response.GetContainerFilesystems()[0].GetInodesUsed().GetValue(),
			).To(BeEquivalentTo(4))
		})

		It("should separate image and container usage for a dedicated image store", func() {
			// Given
			driver := &fakeGraphDriver{usage: map[string]*directory.DiskUsage{
				"image-layer-1":   {Size: 100, InodeCount: 1},
				"image-layer-2":   {Size: 200, InodeCount: 2},
				"container-layer": {Size: 50, InodeCount: 1},
			}}

			imageServerMock.EXPECT().GetStore().Return(storeMock)
			storeMock.EXPECT().Layers().Return([]storage.Layer{
				{ID: "image-layer-1"},
				{ID: "image-layer-2"},
				{ID: "container-layer"},
			}, nil)
			storeMock.EXPECT().Containers().Return([]storage.Container{
				{LayerID: "container-layer"},
			}, nil)
			storeMock.EXPECT().GraphDriver().Return(driver, nil)
			storeMock.EXPECT().ImageStore().Return("/var/lib/imagestore")
			storeMock.EXPECT().GraphRoot().Return("/var/lib/containers/storage")
			storeMock.EXPECT().GraphDriverName().Return("overlay")

			// When
			response, err := sut.ImageFsInfo(context.Background(), nil)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetImageFilesystems()).To(HaveLen(1))
			Expect(
				response.GetImageFilesystems()[0].GetUsedBytes().GetValue(),
			).To(BeEquivalentTo(300))
			Expect(
				response.GetImageFilesystems()[0].GetInodesUsed().GetValue(),
			).To(BeEquivalentTo(3))
			Expect(response.GetImageFilesystems()[0].GetFsId().GetMountpoint()).
				To(Equal("/var/lib/imagestore/overlay-images"))
			Expect(response.GetContainerFilesystems()).To(HaveLen(1))
			Expect(
				response.GetContainerFilesystems()[0].GetUsedBytes().GetValue(),
			).To(BeEquivalentTo(50))
			Expect(
				response.GetContainerFilesystems()[0].GetInodesUsed().GetValue(),
			).To(BeEquivalentTo(1))
			Expect(response.GetContainerFilesystems()[0].GetFsId().GetMountpoint()).
				To(Equal("/var/lib/containers/storage/overlay-containers"))
		})

		It("should fail when a layer's disk usage cannot be read", func() {
			// Given
			driver := &fakeGraphDriver{usage: map[string]*directory.DiskUsage{}}

			imageServerMock.EXPECT().GetStore().Return(storeMock)
			storeMock.EXPECT().Layers().Return([]storage.Layer{{ID: "broken-layer"}}, nil)
			storeMock.EXPECT().Containers().Return(nil, nil)
			storeMock.EXPECT().GraphDriver().Return(driver, nil)

			// When
			response, err := sut.ImageFsInfo(context.Background(), nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("broken-layer"))
			Expect(response).To(BeNil())
		})
	})
})
