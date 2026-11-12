package storage_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.podman.io/image/v5/types"
	"go.uber.org/mock/gomock"

	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/pkg/config"
	containerstoragemock "github.com/cri-o/cri-o/test/mocks/containerstorage"
	criostoragemock "github.com/cri-o/cri-o/test/mocks/criostorage"
)

// NOTE(backend port of upstream PR #9907): these tests are adapted to the
// release-1.35 model where the service managers are keyed by the *runtime
// handler* instead of upstream's SandboxInfo. The adapted API they exercise is:
//
//	func GetImageServiceManager(ctx, store, storageTransport, cfg) (*ImageServiceManager, error)
//	func (m *ImageServiceManager) GetImageService(runtimeHandler string) (ImageServer, error)
//	func (m *ImageServiceManager) SetStorageImageServer(ImageServer)
//	func (m *ImageServiceManager) RemoveImageService(runtimeHandler string)
//	func (m *ImageServiceManager) HeuristicallyTryResolvingStringAsIDPrefix(string) []IDPrefixMatch
//	type IDPrefixMatch struct { ID *StorageImageID; Server ImageServer }
//
//	func GetRuntimeServiceManager(ctx, imageServiceMgr, storageTransport, cfg) (*RuntimeServiceManager, error)
//	func (m *RuntimeServiceManager) GetRuntimeService(runtimeHandler string) (RuntimeServer, error)
//	func (m *RuntimeServiceManager) SetStorageRuntimeServer(RuntimeServer)
//	func (m *RuntimeServiceManager) RemoveRuntimeService(runtimeHandler string)
//
// An empty handler is the equivalent of upstream's "nil sandbox" (no sandbox
// context) and must always route to the main service. Related ported call-site
// changes live in internal/lib/container_server_test{,_inject}.go (managers
// stored as storageImgSvcMgr/storageRuntimeSvcMgr; StorageImageServer(nil)/
// StorageRuntimeServer(nil) return errors) and server/container_create_linux_test.go
// (addOCIBindMounts takes one extra trailing sandbox/ImageServer argument).

// Runtime handler names used by the ImageServiceManager / RuntimeServiceManager
// tests. They match the handlers set up by newTestConfig below.
const (
	testRuntimeHandler        = "runc"        // RuntimePullImage = false
	testRuntimePulledHandler  = "kata-remote" // RuntimePullImage = true
	testUnknownRuntimeHandler = "unknown-runtime"
)

// newTestConfig returns a minimal *config.Config for ImageServiceManager / RuntimeServiceManager tests.
// The registriesFile must be a path to a (possibly empty) file that will be used as the
// SystemRegistriesConfPath.
func newTestConfig(registriesFile string) *config.Config {
	return &config.Config{
		SystemContext: &types.SystemContext{
			SystemRegistriesConfPath: registriesFile,
		},
		ImageConfig: config.ImageConfig{
			DefaultTransport: "docker://",
		},
		RuntimeConfig: config.RuntimeConfig{
			Runtimes: config.Runtimes{
				testRuntimeHandler:       &config.RuntimeHandler{RuntimePullImage: false},
				testRuntimePulledHandler: &config.RuntimeHandler{RuntimePullImage: true},
			},
		},
	}
}

var _ = t.Describe("ImageServiceManager", func() {
	var (
		mockCtrl             *gomock.Controller
		storeMock            *containerstoragemock.MockStore
		storageTransportMock *criostoragemock.MockStorageTransport
		mockImageServer      *criostoragemock.MockImageServer
		sut                  *storage.ImageServiceManager
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		storeMock = containerstoragemock.NewMockStore(mockCtrl)
		storageTransportMock = criostoragemock.NewMockStorageTransport(mockCtrl)
		mockImageServer = criostoragemock.NewMockImageServer(mockCtrl)

		var err error

		sut, err = storage.GetImageServiceManager(
			context.Background(), storeMock, storageTransportMock,
			newTestConfig(t.MustTempFile("registries")),
		)
		Expect(err).ToNot(HaveOccurred())

		// Inject the mock so routing tests don't need a real store.
		sut.SetStorageImageServer(mockImageServer)

		// Routing for a runtime-pulled handler asks the main store for the
		// runtime-pulled image service location; make it fail deterministically
		// so the manager is expected to fall back to the main store.
		mockImageServer.EXPECT().GetStore().Return(storeMock).AnyTimes()
		storeMock.EXPECT().
			ContainerRunDirectory(gomock.Any()).
			Return("", errors.New("no run dir for test")).
			AnyTimes()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	t.Describe("GetImageService", func() {
		// Item 1: an empty runtime handler means "no sandbox context" — always routes to main store.
		It("should return the main store for an empty runtime handler", func() {
			result, err := sut.GetImageService("")
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(mockImageServer))
		})

		// Item 2: the default "runc" handler has RuntimePullImage=false.
		It("should return the main store when the handler has RuntimePullImage=false", func() {
			result, err := sut.GetImageService(testRuntimeHandler)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(mockImageServer))
		})

		// An unknown handler (not present in config.Runtimes) falls back to the main store.
		It("should return the main store for an unknown runtime handler", func() {
			result, err := sut.GetImageService(testUnknownRuntimeHandler)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(mockImageServer))
		})
	})

	t.Describe("RemoveImageService", func() {
		It("should be a no-op for a runtime handler that was never registered", func() {
			Expect(func() { sut.RemoveImageService(testRuntimePulledHandler) }).NotTo(Panic())
		})
	})

	t.Describe("SetStorageImageServer", func() {
		It("should update the store returned by GetImageService for an empty runtime handler", func() {
			anotherMock := criostoragemock.NewMockImageServer(mockCtrl)
			sut.SetStorageImageServer(anotherMock)
			result, err := sut.GetImageService("")
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(anotherMock))
		})
	})

	t.Describe("HeuristicallyTryResolvingStringAsIDPrefix", func() {
		It("should return an empty slice when the main store reports no match", func() {
			mockImageServer.EXPECT().HeuristicallyTryResolvingStringAsIDPrefix("cafe").Return(nil)
			Expect(sut.HeuristicallyTryResolvingStringAsIDPrefix("cafe")).To(BeEmpty())
		})

		It("should return a match paired with the server that owns it", func() {
			const testHex = "2a03a6059f21e150ae84b0973863609494aad70f0a80eaeb64bddd8d92465812"

			testID, err := storage.ParseStorageImageIDFromOutOfProcessData(testHex)
			Expect(err).ToNot(HaveOccurred())

			mockImageServer.EXPECT().
				HeuristicallyTryResolvingStringAsIDPrefix("2a03a").
				Return(&testID)

			matches := sut.HeuristicallyTryResolvingStringAsIDPrefix("2a03a")
			Expect(matches).To(HaveLen(1))
			Expect(matches[0].Server).To(Equal(mockImageServer))
			Expect(matches[0].ID).To(Equal(&testID))
		})
	})
})
