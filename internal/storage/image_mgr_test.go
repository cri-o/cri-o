package storage_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.podman.io/image/v5/types"
	"go.uber.org/mock/gomock"

	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/pkg/config"
	containerstoragemock "github.com/cri-o/cri-o/test/mocks/containerstorage"
	criostoragemock "github.com/cri-o/cri-o/test/mocks/criostorage"
)

// fakeSandboxInfo is a minimal SandboxInfo test shared across storage tests.
type fakeSandboxInfo struct {
	id             string
	runtimeHandler string
}

func (f *fakeSandboxInfo) ID() string             { return f.id }
func (f *fakeSandboxInfo) RuntimeHandler() string { return f.runtimeHandler }

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
				"runc":        &config.RuntimeHandler{RuntimePullImage: false},
				"kata-remote": &config.RuntimeHandler{RuntimePullImage: true},
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
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	t.Describe("GetImageService", func() {
		// Item 1: a nil sandbox means "no sandbox context" — always routes to main store.
		It("should return the main store for a nil sandbox", func() {
			result, err := sut.GetImageService(nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(mockImageServer))
		})

		// Item 2: the default "runc" handler has RuntimePullImage=false.
		It("should return the main store when the handler has RuntimePullImage=false", func() {
			sb := &fakeSandboxInfo{id: "sb-runc", runtimeHandler: "runc"}
			result, err := sut.GetImageService(sb)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(mockImageServer))
		})

		// An unknown handler (not present in config.Runtimes) falls back to the main store.
		It("should return the main store for an unknown runtime handler", func() {
			sb := &fakeSandboxInfo{id: "sb-unknown", runtimeHandler: "unknown-runtime"}
			result, err := sut.GetImageService(sb)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(mockImageServer))
		})
	})

	t.Describe("RemoveImageService", func() {
		It("should be a no-op for a sandbox that was never registered", func() {
			Expect(func() { sut.RemoveImageService("non-existent-sandbox") }).NotTo(Panic())
		})
	})

	t.Describe("SetStorageImageServer", func() {
		It("should update the store returned by GetImageService for a nil sandbox", func() {
			anotherMock := criostoragemock.NewMockImageServer(mockCtrl)
			sut.SetStorageImageServer(anotherMock)
			result, err := sut.GetImageService(nil)
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
