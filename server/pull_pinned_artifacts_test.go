package server_test

import (
	"context"
	"encoding/json"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	godigest "github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"go.uber.org/mock/gomock"

	"github.com/cri-o/cri-o/internal/ociartifact"
	"github.com/cri-o/cri-o/server"
	ociartifactmock "github.com/cri-o/cri-o/test/mocks/ociartifact"
)

const pinnedArtifactTestDigest = godigest.Digest("sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")

// makePinnedArtifactManifest returns a minimal OCI manifest that passes
// EnsureNotContainerImage (artifactType is set).
func makePinnedArtifactManifest() []byte {
	b, err := json.Marshal(map[string]any{
		"schemaVersion": 2,
		"mediaType":     specs.MediaTypeImageManifest,
		"artifactType":  "application/vnd.test.artifact",
		"config": map[string]any{
			"mediaType": specs.MediaTypeImageConfig,
			"digest":    string(pinnedArtifactTestDigest),
			"size":      0,
		},
		"layers": []any{},
	})
	if err != nil {
		panic(err)
	}

	return b
}

var _ = t.Describe("PullPinnedArtifacts", func() {
	// Wire up the standard mock lifecycle so mockCtrl, cniPluginMock etc.
	// are freshly initialised and properly finished for each spec.
	BeforeEach(beforeEach)
	AfterEach(afterEach)

	var (
		ctx        = context.Background()
		implMock   *ociartifactmock.MockImpl
		libartMock *ociartifactmock.MockLibartifactStore
		srv        *server.Server
	)

	// JustBeforeEach runs after all BeforeEach hooks, so mockCtrl is ready.
	JustBeforeEach(func() {
		mockNewServer()

		var err error

		srv, err = server.New(ctx, libMock)
		Expect(err).NotTo(HaveOccurred())

		srv.SetStorageImageServer(imageServerMock)
		srv.SetStorageRuntimeServer(runtimeServerMock)

		implMock = ociartifactmock.NewMockImpl(mockCtrl)
		libartMock = ociartifactmock.NewMockLibartifactStore(mockCtrl)

		artStore, err := ociartifact.NewStore(t.MustTempDir("artifact-store"), nil, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		artStore.SetFakeImpl(implMock)
		artStore.SetFakeStore(&ociartifact.FakeLibartifactStore{MockLibartifactStore: libartMock})

		srv.SetArtifactStoreForTest(artStore)
	})

	It("should do nothing when PinnedArtifacts is empty", func() {
		// No mock expectations — the store must not be touched.
		srv.SetPinnedArtifactsForTest([]string{})
		srv.PullPinnedArtifactsForTest(ctx)
	})

	It("should call Store.Pull for a valid artifact reference", func() {
		libartMock.EXPECT().SystemContext().Return(nil)
		implMock.EXPECT().
			GetManifestFromRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(makePinnedArtifactManifest(), specs.MediaTypeImageManifest, nil)
		libartMock.EXPECT().
			Pull(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(pinnedArtifactTestDigest, nil)

		srv.SetPinnedArtifactsForTest([]string{"registry.example.com/models/llama:v1"})
		srv.PullPinnedArtifactsForTest(ctx)
	})

	It("should skip an unparsable reference without touching the store", func() {
		// ":::bad:::" cannot be parsed by ParseNormalizedNamed — no store calls.
		srv.SetPinnedArtifactsForTest([]string{":::bad:::"})
		srv.PullPinnedArtifactsForTest(ctx)
	})

	It("should skip a reference with both tag and digest (docker.NewReference rejects it)", func() {
		// docker.NewReference rejects refs with both a tag and a digest; no store calls.
		srv.SetPinnedArtifactsForTest([]string{
			"registry.example.com/models/llama:v1@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		})
		srv.PullPinnedArtifactsForTest(ctx)
	})

	It("should continue pulling remaining refs after a store error", func() {
		// Both refs pass EnsureNotContainerImage; first pull fails, second succeeds.
		libartMock.EXPECT().SystemContext().Return(nil).Times(2)
		implMock.EXPECT().
			GetManifestFromRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(makePinnedArtifactManifest(), specs.MediaTypeImageManifest, nil).
			Times(2)
		libartMock.EXPECT().
			Pull(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(pinnedArtifactTestDigest, errors.New("network timeout"))
		libartMock.EXPECT().
			Pull(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(pinnedArtifactTestDigest, nil)

		srv.SetPinnedArtifactsForTest([]string{
			"registry.example.com/models/bad:v1",
			"registry.example.com/models/good:v1",
		})
		srv.PullPinnedArtifactsForTest(ctx)
	})
})
