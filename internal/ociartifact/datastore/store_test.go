package datastore_test

import (
	"context"
	"errors"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/go-digest"
	"github.com/sirupsen/logrus"
	"go.podman.io/common/libimage"
	"go.podman.io/common/pkg/libartifact"
	libartTypes "go.podman.io/common/pkg/libartifact/types"
	"go.uber.org/mock/gomock"

	"github.com/cri-o/cri-o/internal/ociartifact/datastore"
	datastoremock "github.com/cri-o/cri-o/test/mocks/ociartifact/datastore"
)

var errTest = errors.New("test")

// The actual test suite.
var _ = t.Describe("DataStore", func() {
	t.Describe("PullData", func() {
		var (
			implMock  *datastoremock.MockImpl
			storeMock *datastoremock.MockLibartifactStore
			mockCtrl  *gomock.Controller
			testRef   libartifact.ArtifactReference
		)

		BeforeEach(func() {
			logrus.SetOutput(io.Discard)

			mockCtrl = gomock.NewController(GinkgoT())
			implMock = datastoremock.NewMockImpl(mockCtrl)
			storeMock = datastoremock.NewMockLibartifactStore(mockCtrl)

			var err error

			testRef, err = libartifact.NewArtifactReference("quay.io/crio/nginx-seccomp:v2")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			mockCtrl.Finish()
		})

		It("should fail when NewArtifactReference fails", func() {
			// Given
			dataStore, err := datastore.New(t.MustTempDir("artifact"), nil)
			Expect(err).NotTo(HaveOccurred())
			dataStore.SetImpl(implMock)

			implMock.EXPECT().
				NewArtifactReference(gomock.Any()).
				Return(libartifact.ArtifactReference{}, errTest)

			// When
			res, err := dataStore.PullData(context.Background(), "invalid-ref", nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("create artifact reference"))
			Expect(res).To(BeNil())
		})

		It("should fail when Pull fails", func() {
			// Given
			dataStore, err := datastore.New(t.MustTempDir("artifact"), nil)
			Expect(err).NotTo(HaveOccurred())
			dataStore.SetImpl(implMock)
			dataStore.SetStore(storeMock)

			implMock.EXPECT().
				NewArtifactReference(gomock.Any()).
				Return(testRef, nil)
			storeMock.EXPECT().
				Pull(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(digest.Digest(""), errTest)

			// When
			res, err := dataStore.PullData(context.Background(), "quay.io/crio/nginx-seccomp:v2", nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("pull artifact"))
			Expect(res).To(BeNil())
		})

		It("should fail when BlobMountPaths fails", func() {
			// Given
			dataStore, err := datastore.New(t.MustTempDir("artifact"), nil)
			Expect(err).NotTo(HaveOccurred())
			dataStore.SetImpl(implMock)
			dataStore.SetStore(storeMock)

			implMock.EXPECT().
				NewArtifactReference(gomock.Any()).
				Return(testRef, nil)
			storeMock.EXPECT().
				Pull(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(digest.Digest("sha256:abc"), nil)
			storeMock.EXPECT().
				BlobMountPaths(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(nil, errTest)

			// When
			res, err := dataStore.PullData(context.Background(), "quay.io/crio/nginx-seccomp:v2", nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("get blob mount paths"))
			Expect(res).To(BeNil())
		})

		It("should fail when ReadFile fails", func() {
			// Given
			dataStore, err := datastore.New(t.MustTempDir("artifact"), nil)
			Expect(err).NotTo(HaveOccurred())
			dataStore.SetImpl(implMock)
			dataStore.SetStore(storeMock)

			implMock.EXPECT().
				NewArtifactReference(gomock.Any()).
				Return(testRef, nil)
			storeMock.EXPECT().
				Pull(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(digest.Digest("sha256:abc"), nil)
			storeMock.EXPECT().
				BlobMountPaths(gomock.Any(), gomock.Any(), gomock.Any()).
				Return([]libartTypes.BlobMountPath{{SourcePath: "/nonexistent", Name: "blob"}}, nil)
			implMock.EXPECT().
				ReadFile("/nonexistent", gomock.Any()).
				Return(nil, errTest)

			// When
			res, err := dataStore.PullData(context.Background(), "quay.io/crio/nginx-seccomp:v2", nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("read blob file"))
			Expect(res).To(BeNil())
		})

		It("should fail when artifact exceeds max size", func() {
			// Given
			dataStore, err := datastore.New(t.MustTempDir("artifact"), nil)
			Expect(err).NotTo(HaveOccurred())
			dataStore.SetImpl(implMock)
			dataStore.SetStore(storeMock)

			implMock.EXPECT().
				NewArtifactReference(gomock.Any()).
				Return(testRef, nil)
			storeMock.EXPECT().
				Pull(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(digest.Digest("sha256:abc"), nil)
			storeMock.EXPECT().
				BlobMountPaths(gomock.Any(), gomock.Any(), gomock.Any()).
				Return([]libartTypes.BlobMountPath{{SourcePath: "/blob", Name: "blob"}}, nil)

			largeData := make([]byte, 2*1024*1024) // 2 MiB, exceeds default 1 MiB
			implMock.EXPECT().
				ReadFile("/blob", gomock.Any()).
				Return(largeData, nil)

			// When
			res, err := dataStore.PullData(context.Background(), "quay.io/crio/nginx-seccomp:v2", nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exceeded maximum allowed artifact size"))
			Expect(res).To(BeNil())
		})

		It("should succeed with valid data", func() {
			// Given
			dataStore, err := datastore.New(t.MustTempDir("artifact"), nil)
			Expect(err).NotTo(HaveOccurred())
			dataStore.SetImpl(implMock)
			dataStore.SetStore(storeMock)

			implMock.EXPECT().
				NewArtifactReference(gomock.Any()).
				Return(testRef, nil)
			storeMock.EXPECT().
				Pull(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(digest.Digest("sha256:abc"), nil)
			storeMock.EXPECT().
				BlobMountPaths(gomock.Any(), gomock.Any(), gomock.Any()).
				Return([]libartTypes.BlobMountPath{{SourcePath: "/blob", Name: "profile.json"}}, nil)

			blobData := []byte(`{"defaultAction": "SCMP_ACT_ERRNO"}`)
			implMock.EXPECT().
				ReadFile("/blob", gomock.Any()).
				Return(blobData, nil)

			// When
			res, err := dataStore.PullData(context.Background(), "quay.io/crio/nginx-seccomp:v2", nil)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(HaveLen(1))
			Expect(res[0].Data()).To(Equal(blobData))
		})

		It("should pass copy options through", func() {
			// Given
			dataStore, err := datastore.New(t.MustTempDir("artifact"), nil)
			Expect(err).NotTo(HaveOccurred())
			dataStore.SetImpl(implMock)
			dataStore.SetStore(storeMock)

			implMock.EXPECT().
				NewArtifactReference(gomock.Any()).
				Return(testRef, nil)
			storeMock.EXPECT().
				Pull(gomock.Any(), gomock.Any(), libimage.CopyOptions{}).
				Return(digest.Digest("sha256:abc"), nil)
			storeMock.EXPECT().
				BlobMountPaths(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(nil, nil)

			// When
			res, err := dataStore.PullData(context.Background(), "quay.io/crio/nginx-seccomp:v2", &datastore.PullOptions{
				CopyOptions: &libimage.CopyOptions{},
			})

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(BeNil())
		})
	})
})
