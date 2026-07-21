package storage_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/cri-o/cri-o/internal/storage"
	containerstoragemock "github.com/cri-o/cri-o/test/mocks/containerstorage"
	criostoragemock "github.com/cri-o/cri-o/test/mocks/criostorage"
)

var _ = t.Describe("RuntimeServiceManager", func() {
	var (
		mockCtrl                *gomock.Controller
		storeMock               *containerstoragemock.MockStore
		storageTransportMock    *criostoragemock.MockStorageTransport
		mockRuntimeServer       *criostoragemock.MockRuntimeServer
		mockInternalImageServer *criostoragemock.MockImageServer
		imgMgr                  *storage.ImageServiceManager
		sut                     *storage.RuntimeServiceManager
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		storeMock = containerstoragemock.NewMockStore(mockCtrl)
		storageTransportMock = criostoragemock.NewMockStorageTransport(mockCtrl)
		mockRuntimeServer = criostoragemock.NewMockRuntimeServer(mockCtrl)
		mockInternalImageServer = criostoragemock.NewMockImageServer(mockCtrl)

		cfg := newTestConfig(t.MustTempFile("registries"))

		// Build the image manager with the real *imageService so that
		// GetRuntimeServiceManager's internal type assertion (*runtimeService) succeeds.
		var err error

		imgMgr, err = storage.GetImageServiceManager(
			context.Background(), storeMock, storageTransportMock, cfg,
		)
		Expect(err).ToNot(HaveOccurred())

		// Build the runtime manager while the real *imageService is still set.
		sut, err = storage.GetRuntimeServiceManager(
			context.Background(), imgMgr, storageTransportMock, cfg,
		)
		Expect(err).ToNot(HaveOccurred())

		// Replace the image server with a mock.  Any attempt to create a
		// runtimePulledImageService will now hit GetStore() → ContainerRunDirectory
		// and fail, causing GetImageService to fall back to mockInternalImageServer.
		// AnyTimes() covers tests that don't hit the rp=true path at all.
		imgMgr.SetStorageImageServer(mockInternalImageServer)
		sut.SetStorageRuntimeServer(mockRuntimeServer)

		mockInternalImageServer.EXPECT().GetStore().Return(storeMock).AnyTimes()
		storeMock.EXPECT().
			ContainerRunDirectory(gomock.Any()).
			Return("", errors.New("no run dir for test")).
			AnyTimes()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	t.Describe("GetRuntimeService", func() {
		// Nil sandbox — early return, no routing decision needed.
		It("should return the main service for a nil sandbox", func() {
			Expect(sut.GetRuntimeService(nil)).To(Equal(mockRuntimeServer))
		})

		// A sandbox using a handler without RuntimePullImage skips the rp path entirely.
		It("should return the main service when the handler has RuntimePullImage=false", func() {
			sb := &fakeSandboxInfo{id: "sb-runc", runtimeHandler: "runc"}
			Expect(sut.GetRuntimeService(sb)).To(Equal(mockRuntimeServer))
		})
	})

	t.Describe("RemoveRuntimeService", func() {
		It("should be a no-op for a sandbox that was never registered", func() {
			Expect(func() { sut.RemoveRuntimeService("non-existent-sandbox") }).NotTo(Panic())
		})
	})

	t.Describe("SetStorageRuntimeServer", func() {
		It("should update the service returned by GetRuntimeService for a nil sandbox", func() {
			anotherMock := criostoragemock.NewMockRuntimeServer(mockCtrl)
			sut.SetStorageRuntimeServer(anotherMock)
			Expect(sut.GetRuntimeService(nil)).To(Equal(anotherMock))
		})
	})
})
