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
	"go.podman.io/common/pkg/libartifact"
	"go.podman.io/image/v5/manifest"
	"go.uber.org/mock/gomock"

	"github.com/cri-o/cri-o/internal/ociartifact"
	ociartifactmock "github.com/cri-o/cri-o/test/mocks/ociartifact"
)

var errTest = errors.New("test")

const testDigest = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

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
})
