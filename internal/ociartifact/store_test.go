package ociartifact_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"regexp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
	"go.podman.io/common/libimage"
	"go.podman.io/common/pkg/libartifact"
	"go.podman.io/image/v5/docker/reference"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/types"
	"go.uber.org/mock/gomock"

	"github.com/cri-o/cri-o/internal/ociartifact"
	ociartifactmock "github.com/cri-o/cri-o/test/mocks/ociartifact"
)

var errTest = errors.New("test")

const testDigest = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

type fakeImageRef struct {
	ref reference.Named
}

func newFakeImageRef() *fakeImageRef {
	ref, err := reference.ParseNormalizedNamed("docker.io/library/nginx:latest")
	if err != nil {
		panic(err)
	}

	return &fakeImageRef{ref: reference.TagNameOnly(ref)}
}

func (f *fakeImageRef) Transport() types.ImageTransport         { return nil }
func (f *fakeImageRef) StringWithinTransport() string           { return f.ref.String() }
func (f *fakeImageRef) DockerReference() reference.Named        { return f.ref }
func (f *fakeImageRef) PolicyConfigurationIdentity() string     { return "" }
func (f *fakeImageRef) PolicyConfigurationNamespaces() []string { return nil }
func (f *fakeImageRef) NewImage(_ context.Context, _ *types.SystemContext) (types.ImageCloser, error) {
	return nil, nil
}

func (f *fakeImageRef) NewImageSource(_ context.Context, _ *types.SystemContext) (types.ImageSource, error) {
	return nil, nil
}

func (f *fakeImageRef) NewImageDestination(_ context.Context, _ *types.SystemContext) (types.ImageDestination, error) {
	return nil, nil
}
func (f *fakeImageRef) DeleteImage(_ context.Context, _ *types.SystemContext) error { return nil }

func makeOCIManifest(configMediaType, artifactType string) []byte {
	m := map[string]any{
		"schemaVersion": 2,
		"mediaType":     specs.MediaTypeImageManifest,
		"config": map[string]any{
			"mediaType": configMediaType,
			"digest":    testDigest,
			"size":      0,
		},
		"layers": []any{},
	}
	if artifactType != "" {
		m["artifactType"] = artifactType
	}

	b, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}

	return b
}

func makeDockerV2S2Manifest() []byte {
	b, err := json.Marshal(map[string]any{
		"schemaVersion": 2,
		"mediaType":     manifest.DockerV2Schema2MediaType,
		"config": map[string]any{
			"mediaType": manifest.DockerV2Schema2ConfigMediaType,
			"digest":    testDigest,
			"size":      0,
		},
		"layers": []any{},
	})
	if err != nil {
		panic(err)
	}

	return b
}

func makeDockerV2S1SignedManifest() []byte {
	b, err := json.Marshal(map[string]any{
		"schemaVersion": 1,
		"name":          "test",
		"tag":           "latest",
		"architecture":  "amd64",
		"fsLayers": []any{
			map[string]any{"blobSum": testDigest},
		},
		"history": []any{
			map[string]any{"v1Compatibility": `{"id":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}`},
		},
	})
	if err != nil {
		panic(err)
	}

	return b
}

func makeOCIIndex() []byte {
	b, err := json.Marshal(map[string]any{
		"schemaVersion": 2,
		"mediaType":     specs.MediaTypeImageIndex,
		"manifests": []any{
			map[string]any{
				"mediaType": specs.MediaTypeImageManifest,
				"digest":    testDigest,
				"size":      100,
				"platform": map[string]any{
					"os":           "linux",
					"architecture": "amd64",
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}

	return b
}

// The actual test suite.
var _ = t.Describe("Store", func() {
	t.Describe("EnsureNotContainerImage", func() {
		var (
			implMock *ociartifactmock.MockImpl
			mockCtrl *gomock.Controller
			store    *ociartifact.Store
		)

		BeforeEach(func() {
			logrus.SetOutput(io.Discard)

			mockCtrl = gomock.NewController(GinkgoT())
			implMock = ociartifactmock.NewMockImpl(mockCtrl)

			var err error

			store, err = ociartifact.NewStore(t.MustTempDir("artifact"), nil, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			store.SetFakeImpl(implMock)
		})

		AfterEach(func() {
			mockCtrl.Finish()
		})

		It("should fail when GetManifestFromRef fails", func() {
			// Given
			implMock.EXPECT().
				GetManifestFromRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(nil, "", errTest)

			// When
			err := store.EnsureNotContainerImage(context.Background(), nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("get manifest from ref"))
		})

		It("should return ErrIsAnImage for OCI manifest with OCI image config", func() {
			// Given
			implMock.EXPECT().
				GetManifestFromRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(makeOCIManifest(specs.MediaTypeImageConfig, ""), specs.MediaTypeImageManifest, nil)

			// When
			err := store.EnsureNotContainerImage(context.Background(), nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, ociartifact.ErrIsAnImage)).To(BeTrue())
		})

		It("should return ErrIsAnImage for OCI manifest with empty config media type", func() {
			// Given
			implMock.EXPECT().
				GetManifestFromRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(makeOCIManifest("", ""), specs.MediaTypeImageManifest, nil)

			// When
			err := store.EnsureNotContainerImage(context.Background(), nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, ociartifact.ErrIsAnImage)).To(BeTrue())
		})

		It("should return ErrIsAnImage for Docker v2 schema 2 manifest", func() {
			// Given
			implMock.EXPECT().
				GetManifestFromRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(makeDockerV2S2Manifest(), manifest.DockerV2Schema2MediaType, nil)

			// When
			err := store.EnsureNotContainerImage(context.Background(), nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, ociartifact.ErrIsAnImage)).To(BeTrue())
		})

		It("should return ErrIsAnImage for Docker v2 schema 1 signed manifest", func() {
			// Given
			implMock.EXPECT().
				GetManifestFromRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(makeDockerV2S1SignedManifest(), manifest.DockerV2Schema1SignedMediaType, nil)

			// When
			err := store.EnsureNotContainerImage(context.Background(), nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, ociartifact.ErrIsAnImage)).To(BeTrue())
		})

		It("should succeed for OCI manifest with artifactType set", func() {
			// Given
			implMock.EXPECT().
				GetManifestFromRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(
					makeOCIManifest(specs.MediaTypeImageConfig, "application/vnd.example.artifact"),
					specs.MediaTypeImageManifest,
					nil,
				)

			// When
			err := store.EnsureNotContainerImage(context.Background(), nil)

			// Then
			Expect(err).NotTo(HaveOccurred())
		})

		It("should succeed for non-container-image config media type", func() {
			// Given
			implMock.EXPECT().
				GetManifestFromRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(
					makeOCIManifest("application/vnd.custom.config", ""),
					specs.MediaTypeImageManifest,
					nil,
				)

			// When
			err := store.EnsureNotContainerImage(context.Background(), nil)

			// Then
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail when manifest bytes are unparsable", func() {
			// Given
			implMock.EXPECT().
				GetManifestFromRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return([]byte("invalid"), specs.MediaTypeImageManifest, nil)

			// When
			err := store.EnsureNotContainerImage(context.Background(), nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parse manifest"))
		})

		It("should fail when manifest list bytes are unparsable", func() {
			// Given
			implMock.EXPECT().
				GetManifestFromRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return([]byte("invalid"), specs.MediaTypeImageIndex, nil)

			// When
			err := store.EnsureNotContainerImage(context.Background(), nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parse manifest list"))
		})

		It("should fail when ChooseInstance fails", func() {
			// Given
			implMock.EXPECT().
				GetManifestFromRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(makeOCIIndex(), specs.MediaTypeImageIndex, nil)
			implMock.EXPECT().
				ChooseInstance(gomock.Any(), gomock.Any()).
				Return(digest.Digest(""), errTest)

			// When
			err := store.EnsureNotContainerImage(context.Background(), nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("choose manifest instance"))
		})

		It("should fail when getting instance manifest fails", func() {
			// Given
			gomock.InOrder(
				implMock.EXPECT().
					GetManifestFromRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(makeOCIIndex(), specs.MediaTypeImageIndex, nil),
				implMock.EXPECT().
					GetManifestFromRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, "", errTest),
			)
			implMock.EXPECT().
				ChooseInstance(gomock.Any(), gomock.Any()).
				Return(digest.Digest(testDigest), nil)

			// When
			err := store.EnsureNotContainerImage(context.Background(), nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("get instance manifest"))
		})

		It("should return ErrIsAnImage for multi-arch image resolving to container image", func() {
			// Given
			gomock.InOrder(
				implMock.EXPECT().
					GetManifestFromRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(makeOCIIndex(), specs.MediaTypeImageIndex, nil),
				implMock.EXPECT().
					GetManifestFromRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(makeOCIManifest(specs.MediaTypeImageConfig, ""), specs.MediaTypeImageManifest, nil),
			)
			implMock.EXPECT().
				ChooseInstance(gomock.Any(), gomock.Any()).
				Return(digest.Digest(testDigest), nil)

			// When
			err := store.EnsureNotContainerImage(context.Background(), nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, ociartifact.ErrIsAnImage)).To(BeTrue())
		})

		It("should succeed for multi-arch artifact with artifactType", func() {
			// Given
			gomock.InOrder(
				implMock.EXPECT().
					GetManifestFromRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(makeOCIIndex(), specs.MediaTypeImageIndex, nil),
				implMock.EXPECT().
					GetManifestFromRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(
						makeOCIManifest(specs.MediaTypeImageConfig, "application/vnd.example.artifact"),
						specs.MediaTypeImageManifest,
						nil,
					),
			)
			implMock.EXPECT().
				ChooseInstance(gomock.Any(), gomock.Any()).
				Return(digest.Digest(testDigest), nil)

			// When
			err := store.EnsureNotContainerImage(context.Background(), nil)

			// Then
			Expect(err).NotTo(HaveOccurred())
		})
	})

	t.Describe("Artifact pinning", func() {
		var (
			storeMock *ociartifactmock.MockLibartifactStore
			mockCtrl  *gomock.Controller
			store     *ociartifact.Store
		)

		makeLibArtifact := func(name string) *libartifact.Artifact {
			return &libartifact.Artifact{
				Name:     name,
				Digest:   testDigest,
				Manifest: &manifest.OCI1{},
			}
		}

		BeforeEach(func() {
			logrus.SetOutput(io.Discard)

			mockCtrl = gomock.NewController(GinkgoT())
			storeMock = ociartifactmock.NewMockLibartifactStore(mockCtrl)

			var err error

			store, err = ociartifact.NewStore(t.MustTempDir("artifact"), nil, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			store.SetFakeStore(&ociartifact.FakeLibartifactStore{storeMock})
		})

		AfterEach(func() {
			mockCtrl.Finish()
		})

		It("should not pin artifacts when no pinned regexps are configured", func() {
			// Given
			storeMock.EXPECT().
				List(gomock.Any()).
				Return(libartifact.ArtifactList{makeLibArtifact("docker.io/library/nginx:latest")}, nil)

			// When
			arts, err := store.List(context.Background())

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(arts).To(HaveLen(1))
			Expect(arts[0].CRIImage().GetPinned()).To(BeFalse())
		})

		It("should pin artifacts matching a pinned image regexp", func() {
			// Given
			store.SetPinnedImageRegexps([]*regexp.Regexp{
				regexp.MustCompile(`nginx`),
			})
			storeMock.EXPECT().
				List(gomock.Any()).
				Return(libartifact.ArtifactList{makeLibArtifact("registry.example.com/nginx:latest")}, nil)

			// When
			arts, err := store.List(context.Background())

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(arts).To(HaveLen(1))
			Expect(arts[0].CRIImage().GetPinned()).To(BeTrue())
		})

		It("should not pin artifacts that do not match any pinned image regexp", func() {
			// Given
			store.SetPinnedImageRegexps([]*regexp.Regexp{
				regexp.MustCompile(`redis`),
			})
			storeMock.EXPECT().
				List(gomock.Any()).
				Return(libartifact.ArtifactList{makeLibArtifact("docker.io/library/nginx:latest")}, nil)

			// When
			arts, err := store.List(context.Background())

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(arts).To(HaveLen(1))
			Expect(arts[0].CRIImage().GetPinned()).To(BeFalse())
		})

		It("should pin artifacts passed through NewStore constructor regexps", func() {
			// Given
			var err error

			store, err = ociartifact.NewStore(t.MustTempDir("artifact"), nil, nil, []*regexp.Regexp{
				regexp.MustCompile(`nginx`),
			})
			Expect(err).NotTo(HaveOccurred())
			store.SetFakeStore(&ociartifact.FakeLibartifactStore{storeMock})

			storeMock.EXPECT().
				List(gomock.Any()).
				Return(libartifact.ArtifactList{makeLibArtifact("docker.io/library/nginx:latest")}, nil)

			// When
			arts, err := store.List(context.Background())

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(arts).To(HaveLen(1))
			Expect(arts[0].CRIImage().GetPinned()).To(BeTrue())
		})

		It("should pin artifact matching by canonical name", func() {
			// Given
			store.SetPinnedImageRegexps([]*regexp.Regexp{
				regexp.MustCompile(testDigest),
			})
			storeMock.EXPECT().
				List(gomock.Any()).
				Return(libartifact.ArtifactList{makeLibArtifact("docker.io/library/nginx:latest")}, nil)

			// When
			arts, err := store.List(context.Background())

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(arts).To(HaveLen(1))
			Expect(arts[0].CRIImage().GetPinned()).To(BeTrue())
		})

		It("should pin Status result when matching", func() {
			// Given
			store.SetPinnedImageRegexps([]*regexp.Regexp{
				regexp.MustCompile(`nginx`),
			})
			storeMock.EXPECT().
				Inspect(gomock.Any(), gomock.Any()).
				Return(makeLibArtifact("docker.io/library/nginx:latest"), nil)

			// When
			art, err := store.Status(context.Background(), testDigest)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(art.CRIImage().GetPinned()).To(BeTrue())
		})

		It("should not pin Status result when not matching", func() {
			// Given
			storeMock.EXPECT().
				Inspect(gomock.Any(), gomock.Any()).
				Return(makeLibArtifact("docker.io/library/nginx:latest"), nil)

			// When
			art, err := store.Status(context.Background(), testDigest)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(art.CRIImage().GetPinned()).To(BeFalse())
		})

		It("should reflect updated regexps after SetPinnedImageRegexps", func() {
			// Given
			storeMock.EXPECT().
				List(gomock.Any()).
				Return(libartifact.ArtifactList{makeLibArtifact("docker.io/library/nginx:latest")}, nil).
				Times(2)

			// Initially no pinning
			arts, err := store.List(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(arts[0].CRIImage().GetPinned()).To(BeFalse())

			// When - update regexps
			store.SetPinnedImageRegexps([]*regexp.Regexp{
				regexp.MustCompile(`nginx`),
			})

			arts, err = store.List(context.Background())

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(arts[0].CRIImage().GetPinned()).To(BeTrue())
		})
	})

	t.Describe("Additional stores", func() {
		var (
			mainStoreMock *ociartifactmock.MockLibartifactStore
			addStoreMock1 *ociartifactmock.MockLibartifactStore
			addStoreMock2 *ociartifactmock.MockLibartifactStore
			implMock      *ociartifactmock.MockImpl
			mockCtrl      *gomock.Controller
			store         *ociartifact.Store
		)

		const (
			addStorePath1 = "/additional/store1/artifacts"
			addStorePath2 = "/additional/store2/artifacts"
		)

		makeLibArtifact := func(name string) *libartifact.Artifact {
			return &libartifact.Artifact{
				Name:     name,
				Digest:   testDigest,
				Manifest: &manifest.OCI1{},
			}
		}

		BeforeEach(func() {
			logrus.SetOutput(io.Discard)

			mockCtrl = gomock.NewController(GinkgoT())
			mainStoreMock = ociartifactmock.NewMockLibartifactStore(mockCtrl)
			addStoreMock1 = ociartifactmock.NewMockLibartifactStore(mockCtrl)
			addStoreMock2 = ociartifactmock.NewMockLibartifactStore(mockCtrl)
			implMock = ociartifactmock.NewMockImpl(mockCtrl)

			var err error

			store, err = ociartifact.NewStore(t.MustTempDir("artifact"), nil, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			store.SetFakeStore(&ociartifact.FakeLibartifactStore{mainStoreMock})
			store.SetFakeImpl(implMock)
		})

		AfterEach(func() {
			mockCtrl.Finish()
		})

		t.Describe("List", func() {
			It("should return artifacts from additional store when main store is empty", func() {
				// Given
				store.SetFakeAdditionalStores(ociartifact.FakeAdditionalStore{
					Path:  addStorePath1,
					Store: addStoreMock1,
				})
				addStoreMock1.EXPECT().
					List(gomock.Any()).
					Return(libartifact.ArtifactList{makeLibArtifact("docker.io/library/nginx:latest")}, nil)
				mainStoreMock.EXPECT().
					List(gomock.Any()).
					Return(libartifact.ArtifactList{}, nil)

				// When
				arts, err := store.List(context.Background())

				// Then
				Expect(err).NotTo(HaveOccurred())
				Expect(arts).To(HaveLen(1))
				Expect(arts[0].Reference()).To(Equal("docker.io/library/nginx:latest"))
				Expect(arts[0].CRIImage().GetPinned()).To(BeTrue())
				Expect(arts[0].RootPath()).To(Equal(addStorePath1))
			})

			It("should return artifacts from main store when no additional stores configured", func() {
				// Given (no SetFakeAdditionalStores call)
				mainStoreMock.EXPECT().
					List(gomock.Any()).
					Return(libartifact.ArtifactList{makeLibArtifact("docker.io/library/nginx:latest")}, nil)

				// When
				arts, err := store.List(context.Background())

				// Then
				Expect(err).NotTo(HaveOccurred())
				Expect(arts).To(HaveLen(1))
				Expect(arts[0].CRIImage().GetPinned()).To(BeFalse())
				Expect(arts[0].RootPath()).To(Equal(store.RootPath()))
			})

			It("should merge artifacts from additional and main stores", func() {
				// Given
				store.SetFakeAdditionalStores(ociartifact.FakeAdditionalStore{
					Path:  addStorePath1,
					Store: addStoreMock1,
				})
				addStoreMock1.EXPECT().
					List(gomock.Any()).
					Return(libartifact.ArtifactList{makeLibArtifact("docker.io/library/nginx:latest")}, nil)
				mainStoreMock.EXPECT().
					List(gomock.Any()).
					Return(libartifact.ArtifactList{makeLibArtifact("docker.io/library/redis:latest")}, nil)

				// When
				arts, err := store.List(context.Background())

				// Then
				Expect(err).NotTo(HaveOccurred())
				Expect(arts).To(HaveLen(2))
				Expect(arts[0].RootPath()).To(Equal(addStorePath1))
				Expect(arts[1].RootPath()).To(Equal(store.RootPath()))
			})

			It("should deduplicate by reference with additional store winning", func() {
				// Given
				store.SetFakeAdditionalStores(ociartifact.FakeAdditionalStore{
					Path:  addStorePath1,
					Store: addStoreMock1,
				})
				addStoreMock1.EXPECT().
					List(gomock.Any()).
					Return(libartifact.ArtifactList{makeLibArtifact("docker.io/library/nginx:latest")}, nil)
				mainStoreMock.EXPECT().
					List(gomock.Any()).
					Return(libartifact.ArtifactList{makeLibArtifact("docker.io/library/nginx:latest")}, nil)

				// When
				arts, err := store.List(context.Background())

				// Then
				Expect(err).NotTo(HaveOccurred())
				Expect(arts).To(HaveLen(1))
				Expect(arts[0].CRIImage().GetPinned()).To(BeTrue())
				Expect(arts[0].RootPath()).To(Equal(addStorePath1))
			})

			It("should continue listing main store when additional store returns error", func() {
				// Given
				store.SetFakeAdditionalStores(ociartifact.FakeAdditionalStore{
					Path:  addStorePath1,
					Store: addStoreMock1,
				})
				addStoreMock1.EXPECT().
					List(gomock.Any()).
					Return(nil, errTest)
				mainStoreMock.EXPECT().
					List(gomock.Any()).
					Return(libartifact.ArtifactList{makeLibArtifact("docker.io/library/redis:latest")}, nil)

				// When
				arts, err := store.List(context.Background())

				// Then
				Expect(err).NotTo(HaveOccurred())
				Expect(arts).To(HaveLen(1))
				Expect(arts[0].Reference()).To(Equal("docker.io/library/redis:latest"))
			})

			It("should list artifacts from multiple additional stores", func() {
				// Given
				store.SetFakeAdditionalStores(
					ociartifact.FakeAdditionalStore{
						Path:  addStorePath1,
						Store: addStoreMock1,
					},
					ociartifact.FakeAdditionalStore{
						Path:  addStorePath2,
						Store: addStoreMock2,
					},
				)
				addStoreMock1.EXPECT().
					List(gomock.Any()).
					Return(libartifact.ArtifactList{makeLibArtifact("docker.io/library/nginx:latest")}, nil)
				addStoreMock2.EXPECT().
					List(gomock.Any()).
					Return(libartifact.ArtifactList{makeLibArtifact("docker.io/library/redis:latest")}, nil)
				mainStoreMock.EXPECT().
					List(gomock.Any()).
					Return(libartifact.ArtifactList{}, nil)

				// When
				arts, err := store.List(context.Background())

				// Then
				Expect(err).NotTo(HaveOccurred())
				Expect(arts).To(HaveLen(2))
				Expect(arts[0].RootPath()).To(Equal(addStorePath1))
				Expect(arts[1].RootPath()).To(Equal(addStorePath2))
			})
		})

		t.Describe("Status", func() {
			It("should find artifact in additional store first with correct pinning and RootPath", func() {
				// Given
				store.SetFakeAdditionalStores(ociartifact.FakeAdditionalStore{
					Path:  addStorePath1,
					Store: addStoreMock1,
				})
				addStoreMock1.EXPECT().
					Inspect(gomock.Any(), gomock.Any()).
					Return(makeLibArtifact("docker.io/library/nginx:latest"), nil)

				// When
				art, err := store.Status(context.Background(), "docker.io/library/nginx:latest")

				// Then
				Expect(err).NotTo(HaveOccurred())
				Expect(art.Reference()).To(Equal("docker.io/library/nginx:latest"))
				Expect(art.RootPath()).To(Equal(addStorePath1))
				Expect(art.CRIImage().GetPinned()).To(BeTrue())
			})

			It("should fall back to main store when not in additional stores", func() {
				// Given
				store.SetFakeAdditionalStores(ociartifact.FakeAdditionalStore{
					Path:  addStorePath1,
					Store: addStoreMock1,
				})
				addStoreMock1.EXPECT().
					Inspect(gomock.Any(), gomock.Any()).
					Return(nil, ociartifact.ErrNotFound)
				mainStoreMock.EXPECT().
					Inspect(gomock.Any(), gomock.Any()).
					Return(makeLibArtifact("docker.io/library/nginx:latest"), nil)

				// When
				art, err := store.Status(context.Background(), "docker.io/library/nginx:latest")

				// Then
				Expect(err).NotTo(HaveOccurred())
				Expect(art.RootPath()).To(Equal(store.RootPath()))
				Expect(art.CRIImage().GetPinned()).To(BeFalse())
			})

			It("should return ErrNotFound-wrapped error when not in any store", func() {
				// Given
				store.SetFakeAdditionalStores(ociartifact.FakeAdditionalStore{
					Path:  addStorePath1,
					Store: addStoreMock1,
				})
				addStoreMock1.EXPECT().
					Inspect(gomock.Any(), gomock.Any()).
					Return(nil, ociartifact.ErrNotFound)
				mainStoreMock.EXPECT().
					Inspect(gomock.Any(), gomock.Any()).
					Return(nil, ociartifact.ErrNotFound)

				// When
				_, err := store.Status(context.Background(), "docker.io/library/nginx:latest")

				// Then
				Expect(err).To(HaveOccurred())
				Expect(errors.Is(err, ociartifact.ErrNotFound)).To(BeTrue())
			})

			It("should warn on non-ErrNotFound errors from additional store and fall back", func() {
				// Given
				store.SetFakeAdditionalStores(ociartifact.FakeAdditionalStore{
					Path:  addStorePath1,
					Store: addStoreMock1,
				})
				addStoreMock1.EXPECT().
					Inspect(gomock.Any(), gomock.Any()).
					Return(nil, errTest)
				mainStoreMock.EXPECT().
					Inspect(gomock.Any(), gomock.Any()).
					Return(makeLibArtifact("docker.io/library/nginx:latest"), nil)

				// When
				art, err := store.Status(context.Background(), "docker.io/library/nginx:latest")

				// Then
				Expect(err).NotTo(HaveOccurred())
				Expect(art.RootPath()).To(Equal(store.RootPath()))
			})
		})

		t.Describe("Remove", func() {
			It("should only remove from main store", func() {
				// Given
				store.SetFakeAdditionalStores(ociartifact.FakeAdditionalStore{
					Path:  addStorePath1,
					Store: addStoreMock1,
				})
				// addStoreMock1.Remove is NOT expected (gomock strict mode
				// will fail the test if it gets called unexpectedly).
				dgst := digest.Digest(testDigest)
				mainStoreMock.EXPECT().
					Remove(gomock.Any(), gomock.Any()).
					Return(&dgst, nil)

				// When
				err := store.Remove(context.Background(), "docker.io/library/nginx:latest")

				// Then
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return error from main store on remove failure", func() {
				// Given
				store.SetFakeAdditionalStores(ociartifact.FakeAdditionalStore{
					Path:  addStorePath1,
					Store: addStoreMock1,
				})
				mainStoreMock.EXPECT().
					Remove(gomock.Any(), gomock.Any()).
					Return(nil, ociartifact.ErrNotFound)

				// When
				err := store.Remove(context.Background(), "docker.io/library/nginx:latest")

				// Then
				Expect(err).To(HaveOccurred())
				Expect(errors.Is(err, ociartifact.ErrNotFound)).To(BeTrue())
			})
		})

		t.Describe("Pull", func() {
			artifactManifest := makeOCIManifest(specs.MediaTypeImageConfig, "application/vnd.test.artifact")

			It("should skip pull when artifact exists in additional store and return correct digest", func() {
				// Given
				store.SetFakeAdditionalStores(ociartifact.FakeAdditionalStore{
					Path:  addStorePath1,
					Store: addStoreMock1,
				})
				mainStoreMock.EXPECT().SystemContext().Return(nil)
				implMock.EXPECT().
					GetManifestFromRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(artifactManifest, specs.MediaTypeImageManifest, nil)
				addStoreMock1.EXPECT().
					Inspect(gomock.Any(), gomock.Any()).
					Return(makeLibArtifact("docker.io/library/nginx:latest"), nil)

				// When
				dgst, err := store.Pull(context.Background(), newFakeImageRef(), &libimage.CopyOptions{})

				// Then
				Expect(err).NotTo(HaveOccurred())
				Expect(dgst).NotTo(BeNil())
				Expect(*dgst).To(Equal(digest.Digest(testDigest)))
			})

			It("should fall through to normal pull when artifact not in additional store", func() {
				// Given
				store.SetFakeAdditionalStores(ociartifact.FakeAdditionalStore{
					Path:  addStorePath1,
					Store: addStoreMock1,
				})
				mainStoreMock.EXPECT().SystemContext().Return(nil)
				implMock.EXPECT().
					GetManifestFromRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(artifactManifest, specs.MediaTypeImageManifest, nil)
				addStoreMock1.EXPECT().
					Inspect(gomock.Any(), gomock.Any()).
					Return(nil, ociartifact.ErrNotFound)
				mainStoreMock.EXPECT().
					Pull(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(digest.Digest(testDigest), nil)

				// When
				dgst, err := store.Pull(context.Background(), newFakeImageRef(), &libimage.CopyOptions{})

				// Then
				Expect(err).NotTo(HaveOccurred())
				Expect(dgst).NotTo(BeNil())
				Expect(*dgst).To(Equal(digest.Digest(testDigest)))
			})

			It("should fall through to normal pull when additional store inspect errors", func() {
				// Given
				store.SetFakeAdditionalStores(ociartifact.FakeAdditionalStore{
					Path:  addStorePath1,
					Store: addStoreMock1,
				})
				mainStoreMock.EXPECT().SystemContext().Return(nil)
				implMock.EXPECT().
					GetManifestFromRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(artifactManifest, specs.MediaTypeImageManifest, nil)
				addStoreMock1.EXPECT().
					Inspect(gomock.Any(), gomock.Any()).
					Return(nil, errTest)
				mainStoreMock.EXPECT().
					Pull(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(digest.Digest(testDigest), nil)

				// When
				dgst, err := store.Pull(context.Background(), newFakeImageRef(), &libimage.CopyOptions{})

				// Then
				Expect(err).NotTo(HaveOccurred())
				Expect(dgst).NotTo(BeNil())
				Expect(*dgst).To(Equal(digest.Digest(testDigest)))
			})

			It("should check additional stores in order and stop at first match", func() {
				// Given
				store.SetFakeAdditionalStores(
					ociartifact.FakeAdditionalStore{
						Path:  addStorePath1,
						Store: addStoreMock1,
					},
					ociartifact.FakeAdditionalStore{
						Path:  addStorePath2,
						Store: addStoreMock2,
					},
				)
				mainStoreMock.EXPECT().SystemContext().Return(nil)
				implMock.EXPECT().
					GetManifestFromRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(artifactManifest, specs.MediaTypeImageManifest, nil)
				addStoreMock1.EXPECT().
					Inspect(gomock.Any(), gomock.Any()).
					Return(makeLibArtifact("docker.io/library/nginx:latest"), nil)
				// addStoreMock2.Inspect is NOT expected (gomock strict mode
				// will fail the test if it gets called unexpectedly).

				// When
				dgst, err := store.Pull(context.Background(), newFakeImageRef(), &libimage.CopyOptions{})

				// Then
				Expect(err).NotTo(HaveOccurred())
				Expect(dgst).NotTo(BeNil())
				Expect(*dgst).To(Equal(digest.Digest(testDigest)))
			})

			It("should not skip pull when artifact exists only in main store", func() {
				// Given (no additional stores configured)
				mainStoreMock.EXPECT().SystemContext().Return(nil)
				implMock.EXPECT().
					GetManifestFromRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(artifactManifest, specs.MediaTypeImageManifest, nil)
				mainStoreMock.EXPECT().
					Pull(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(digest.Digest(testDigest), nil)

				// When
				dgst, err := store.Pull(context.Background(), newFakeImageRef(), &libimage.CopyOptions{})

				// Then
				Expect(err).NotTo(HaveOccurred())
				Expect(dgst).NotTo(BeNil())
				Expect(*dgst).To(Equal(digest.Digest(testDigest)))
			})
		})
	})
})
