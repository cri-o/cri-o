package ociartifact_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
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

			store, err = ociartifact.NewStore(t.MustTempDir("artifact"), nil)
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
})
