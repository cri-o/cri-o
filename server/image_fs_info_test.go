package server_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// writeLayerContent creates dir and fills it with files of the given sizes.
func writeLayerContent(dir string, fileSizes ...int) {
	ExpectWithOffset(1, os.MkdirAll(dir, 0o755)).To(Succeed())

	for i, size := range fileSizes {
		file := filepath.Join(dir, fmt.Sprintf("file-%d", i))
		ExpectWithOffset(1, os.WriteFile(file, make([]byte, size), 0o644)).To(Succeed())
	}
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
		It("should sum layer diff content for both filesystems", func() {
			// Given
			root := GinkgoT().TempDir()
			home := filepath.Join(root, "overlay")
			// One diff dir plus two files = 3 inodes, at least 300 bytes.
			writeLayerContent(filepath.Join(home, "layer1", "diff"), 100, 200)
			// The link directory is not a layer and must be ignored.
			Expect(os.MkdirAll(filepath.Join(home, "l"), 0o755)).To(Succeed())

			imageServerMock.EXPECT().GetStore().Return(storeMock)
			storeMock.EXPECT().GraphRoot().Return(root)
			storeMock.EXPECT().ImageStore().Return("")
			storeMock.EXPECT().GraphDriverName().Return("overlay")

			// When
			response, err := sut.ImageFsInfo(context.Background(), nil)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetImageFilesystems()).To(HaveLen(1))
			Expect(response.GetContainerFilesystems()).To(HaveLen(1))

			image := response.GetImageFilesystems()[0]
			container := response.GetContainerFilesystems()[0]

			Expect(image.GetFsId().GetMountpoint()).To(Equal(home))
			Expect(image.GetUsedBytes().GetValue()).To(BeNumerically(">=", 300))
			Expect(image.GetInodesUsed().GetValue()).To(BeEquivalentTo(3))
			// A shared filesystem reports the same usage for both.
			Expect(container.GetFsId().GetMountpoint()).To(Equal(image.GetFsId().GetMountpoint()))
			Expect(container.GetUsedBytes().GetValue()).To(Equal(image.GetUsedBytes().GetValue()))
			Expect(container.GetInodesUsed().GetValue()).To(Equal(image.GetInodesUsed().GetValue()))
		})

		It("should count hard-linked files once", func() {
			// Given
			root := GinkgoT().TempDir()
			home := filepath.Join(root, "overlay")
			diff := filepath.Join(home, "layer1", "diff")
			writeLayerContent(diff, 4096)
			// A second name for the same inode must count once.
			Expect(os.Link(
				filepath.Join(diff, "file-0"),
				filepath.Join(diff, "file-0-link"),
			)).To(Succeed())

			imageServerMock.EXPECT().GetStore().Return(storeMock)
			storeMock.EXPECT().GraphRoot().Return(root)
			storeMock.EXPECT().ImageStore().Return("")
			storeMock.EXPECT().GraphDriverName().Return("overlay")

			// When
			response, err := sut.ImageFsInfo(context.Background(), nil)

			// Then
			Expect(err).ToNot(HaveOccurred())

			image := response.GetImageFilesystems()[0]
			// One diff dir plus one unique file inode = 2 inodes.
			Expect(image.GetInodesUsed().GetValue()).To(BeEquivalentTo(2))
		})

		It("should sum whole layer content for non-overlay drivers", func() {
			// Given a vfs layout, which stores content directly in the layer
			// directory with no "diff" subdirectory.
			root := GinkgoT().TempDir()
			home := filepath.Join(root, "vfs")
			writeLayerContent(filepath.Join(home, "dir", "layer1"), 100, 200)

			imageServerMock.EXPECT().GetStore().Return(storeMock)
			storeMock.EXPECT().GraphRoot().Return(root)
			storeMock.EXPECT().ImageStore().Return("")
			storeMock.EXPECT().GraphDriverName().Return("vfs")

			// When
			response, err := sut.ImageFsInfo(context.Background(), nil)

			// Then
			Expect(err).ToNot(HaveOccurred())

			image := response.GetImageFilesystems()[0]
			Expect(image.GetFsId().GetMountpoint()).To(Equal(home))
			Expect(image.GetUsedBytes().GetValue()).To(BeNumerically(">=", 300))
			// The "dir" and "layer1" directories plus two files = 4 inodes.
			Expect(image.GetInodesUsed().GetValue()).To(BeEquivalentTo(4))
		})

		It("should exclude mounted overlay views from usage", func() {
			// Given
			root := GinkgoT().TempDir()
			home := filepath.Join(root, "overlay")
			// Stored content: one diff dir plus one file = 2 inodes.
			writeLayerContent(filepath.Join(home, "layer1", "diff"), 100)
			// A mounted overlay view and its work dir must not be counted.
			writeLayerContent(filepath.Join(home, "layer1", "merged"), 10_000_000)
			writeLayerContent(filepath.Join(home, "layer1", "work"), 4096)

			imageServerMock.EXPECT().GetStore().Return(storeMock)
			storeMock.EXPECT().GraphRoot().Return(root)
			storeMock.EXPECT().ImageStore().Return("")
			storeMock.EXPECT().GraphDriverName().Return("overlay")

			// When
			response, err := sut.ImageFsInfo(context.Background(), nil)

			// Then
			Expect(err).ToNot(HaveOccurred())

			image := response.GetImageFilesystems()[0]
			Expect(image.GetInodesUsed().GetValue()).To(BeEquivalentTo(2))
			Expect(image.GetUsedBytes().GetValue()).To(BeNumerically("<", 10_000_000))
		})

		It("should separate image and container usage for a dedicated image store", func() {
			// Given
			root := GinkgoT().TempDir()
			imageStore := GinkgoT().TempDir()

			writeLayerContent(filepath.Join(root, "overlay", "ctr1", "diff"), 50)
			writeLayerContent(filepath.Join(imageStore, "overlay", "img1", "diff"), 100, 200)

			imageServerMock.EXPECT().GetStore().Return(storeMock)
			storeMock.EXPECT().GraphRoot().Return(root)
			storeMock.EXPECT().ImageStore().Return(imageStore)
			storeMock.EXPECT().GraphDriverName().Return("overlay")

			// When
			response, err := sut.ImageFsInfo(context.Background(), nil)

			// Then
			Expect(err).ToNot(HaveOccurred())

			image := response.GetImageFilesystems()[0]
			container := response.GetContainerFilesystems()[0]

			Expect(image.GetFsId().GetMountpoint()).To(Equal(filepath.Join(imageStore, "overlay")))
			Expect(image.GetUsedBytes().GetValue()).To(BeNumerically(">=", 300))
			Expect(image.GetInodesUsed().GetValue()).To(BeEquivalentTo(3))
			Expect(container.GetFsId().GetMountpoint()).To(Equal(filepath.Join(root, "overlay")))
			Expect(container.GetUsedBytes().GetValue()).To(BeNumerically(">=", 50))
			Expect(container.GetInodesUsed().GetValue()).To(BeEquivalentTo(2))
			// The dedicated image store holds more content than the container store.
			Expect(
				image.GetUsedBytes().GetValue(),
			).To(BeNumerically(">", container.GetUsedBytes().GetValue()))
		})

		It("should fail when the graph root content cannot be read", func() {
			// Given a graph root whose driver content directory does not exist.
			imageServerMock.EXPECT().GetStore().Return(storeMock)
			storeMock.EXPECT().GraphRoot().Return(GinkgoT().TempDir())
			storeMock.EXPECT().ImageStore().Return("")
			storeMock.EXPECT().GraphDriverName().Return("overlay")

			// When
			response, err := sut.ImageFsInfo(context.Background(), nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(response).To(BeNil())
		})

		It("should fail when the image store content cannot be read", func() {
			// Given a valid graph root but a missing image store content directory.
			root := GinkgoT().TempDir()
			writeLayerContent(filepath.Join(root, "overlay", "ctr1", "diff"), 50)

			imageServerMock.EXPECT().GetStore().Return(storeMock)
			storeMock.EXPECT().GraphRoot().Return(root)
			storeMock.EXPECT().ImageStore().Return(GinkgoT().TempDir())
			storeMock.EXPECT().GraphDriverName().Return("overlay")

			// When
			response, err := sut.ImageFsInfo(context.Background(), nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(response).To(BeNil())
		})
	})
})
