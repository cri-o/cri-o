package ociartifact_test

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/go-digest"
	"github.com/sirupsen/logrus"
	"go.uber.org/mock/gomock"

	"github.com/cri-o/cri-o/internal/config/ociartifact"
	ociartifactmock "github.com/cri-o/cri-o/test/mocks/ociartifact"
)

var errTest = errors.New("test")

type fakeDirEntry struct{ failOnInfo bool }

func (*fakeDirEntry) IsDir() bool       { return false }
func (*fakeDirEntry) Name() string      { return "fakeDirEntry" }
func (*fakeDirEntry) Type() os.FileMode { return 0o600 }
func (f *fakeDirEntry) Info() (os.FileInfo, error) {
	if f.failOnInfo {
		return nil, errTest
	}

	return &fakeFileInfo{}, nil
}

type fakeFileInfo struct{}

func (*fakeFileInfo) Name() string       { return "fakeFileInfo" }
func (*fakeFileInfo) Size() int64        { return 0 }
func (*fakeFileInfo) Mode() os.FileMode  { return 0o600 }
func (*fakeFileInfo) ModTime() time.Time { return time.Now().Add(-5 * time.Second) }
func (*fakeFileInfo) IsDir() bool        { return false }
func (*fakeFileInfo) Sys() any           { return nil }

// The actual test suite.
var _ = t.Describe("OCIArtifact", func() {
	t.Describe("Pull", func() {
		var (
			sut      *ociartifact.OCIArtifact
			implMock *ociartifactmock.MockImpl
			mockCtrl *gomock.Controller

			testRef            reference.Named
			testArtifact       = []byte{1, 2, 3}
			testArtifactDigest = digest.Digest("sha256:039058c6f2c0cb492c533b0a4d14ef77cc0f78abccced5287d84a1a2011cfb81")
		)

		BeforeEach(func() {
			logrus.SetOutput(io.Discard)

			sut = ociartifact.New()
			Expect(sut).NotTo(BeNil())

			mockCtrl = gomock.NewController(GinkgoT())
			implMock = ociartifactmock.NewMockImpl(mockCtrl)
			sut.SetImpl(implMock)

			var err error
			testRef, err = reference.ParseNormalizedNamed("quay.io/crio/nginx-seccomp:v2")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			mockCtrl.Finish()
		})

		It("should succeed with artifact", func() {
			// Given
			//nolint:dupl
			gomock.InOrder(
				implMock.EXPECT().ParseNormalizedNamed(gomock.Any()).Return(testRef, nil),
				implMock.EXPECT().NewReference(gomock.Any()).Return(nil, nil),
				implMock.EXPECT().NewImageSource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().GetManifest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, "", nil),
				implMock.EXPECT().ManifestFromBlob(gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().LayerInfos(gomock.Any()).Return([]manifest.LayerInfo{
					{BlobInfo: types.BlobInfo{Digest: testArtifactDigest}},
				}),
				implMock.EXPECT().GetBlob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(io.NopCloser(nil), int64(10), nil),
				implMock.EXPECT().ReadAll(gomock.Any()).Return(testArtifact, nil),
			)

			// When
			res, err := sut.Pull(context.Background(), "", nil)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(res).NotTo(BeNil())
			Expect(res.Data).To(BeEquivalentTo(testArtifact))
		})

		It("should succeed with cached artifact", func() {
			// Given

			gomock.InOrder(
				implMock.EXPECT().ParseNormalizedNamed(gomock.Any()).Return(testRef, nil),
				implMock.EXPECT().NewReference(gomock.Any()).Return(nil, nil),
				implMock.EXPECT().NewImageSource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().GetManifest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, "", nil),
				implMock.EXPECT().ManifestFromBlob(gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().LayerInfos(gomock.Any()).Return([]manifest.LayerInfo{
					{BlobInfo: types.BlobInfo{Digest: testArtifactDigest}},
				}),
				implMock.EXPECT().MkdirAll(gomock.Any(), gomock.Any()).Return(nil),
				implMock.EXPECT().ReadDir(gomock.Any()).Return([]os.DirEntry{}, nil),
				implMock.EXPECT().ReadFile(gomock.Any()).Return(testArtifact, nil),
			)

			// When
			res, err := sut.Pull(context.Background(), "", &ociartifact.PullOptions{CachePath: "/cache"})

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(res).NotTo(BeNil())
			Expect(res.Data).To(BeEquivalentTo(testArtifact))
		})

		It("should remove cached item if too old", func() {
			// Given
			gomock.InOrder(
				implMock.EXPECT().ParseNormalizedNamed(gomock.Any()).Return(testRef, nil),
				implMock.EXPECT().NewReference(gomock.Any()).Return(nil, nil),
				implMock.EXPECT().NewImageSource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().GetManifest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, "", nil),
				implMock.EXPECT().ManifestFromBlob(gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().LayerInfos(gomock.Any()).Return([]manifest.LayerInfo{
					{BlobInfo: types.BlobInfo{Digest: testArtifactDigest}},
				}),
				implMock.EXPECT().MkdirAll(gomock.Any(), gomock.Any()).Return(nil),
				implMock.EXPECT().ReadDir(gomock.Any()).Return([]os.DirEntry{&fakeDirEntry{}}, nil),
				implMock.EXPECT().RemoveAll(gomock.Any()).Return(nil),
				implMock.EXPECT().ReadFile(gomock.Any()).Return(testArtifact, nil),
			)

			// When
			res, err := sut.Pull(context.Background(), "", &ociartifact.PullOptions{
				CachePath:        "/cache",
				CacheEntryMaxAge: time.Second,
			})

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(res).NotTo(BeNil())
			Expect(res.Data).To(BeEquivalentTo(testArtifact))
		})

		It("should succeed if cache garbage collection fails", func() {
			// Given
			gomock.InOrder(
				implMock.EXPECT().ParseNormalizedNamed(gomock.Any()).Return(testRef, nil),
				implMock.EXPECT().NewReference(gomock.Any()).Return(nil, nil),
				implMock.EXPECT().NewImageSource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().GetManifest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, "", nil),
				implMock.EXPECT().ManifestFromBlob(gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().LayerInfos(gomock.Any()).Return([]manifest.LayerInfo{
					{BlobInfo: types.BlobInfo{Digest: testArtifactDigest}},
				}),
				implMock.EXPECT().MkdirAll(gomock.Any(), gomock.Any()).Return(nil),
				implMock.EXPECT().ReadDir(gomock.Any()).Return([]os.DirEntry{
					&fakeDirEntry{failOnInfo: true},
					&fakeDirEntry{},
				}, nil),
				implMock.EXPECT().RemoveAll(gomock.Any()).Return(errTest),
				implMock.EXPECT().ReadFile(gomock.Any()).Return(testArtifact, nil),
			)

			// When
			res, err := sut.Pull(context.Background(), "", &ociartifact.PullOptions{
				CachePath:        "/cache",
				CacheEntryMaxAge: time.Second,
			})

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(res).NotTo(BeNil())
			Expect(res.Data).To(BeEquivalentTo(testArtifact))
		})

		//nolint:dupl
		It("should remove cached artifact if it has the wrong digest", func() {
			// Given
			gomock.InOrder(
				implMock.EXPECT().ParseNormalizedNamed(gomock.Any()).Return(testRef, nil),
				implMock.EXPECT().NewReference(gomock.Any()).Return(nil, nil),
				implMock.EXPECT().NewImageSource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().GetManifest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, "", nil),
				implMock.EXPECT().ManifestFromBlob(gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().LayerInfos(gomock.Any()).Return([]manifest.LayerInfo{
					{BlobInfo: types.BlobInfo{Digest: testArtifactDigest}},
				}),
				implMock.EXPECT().MkdirAll(gomock.Any(), gomock.Any()).Return(nil),
				implMock.EXPECT().ReadDir(gomock.Any()).Return([]os.DirEntry{}, nil),
				implMock.EXPECT().ReadFile(gomock.Any()).Return([]byte("wrong"), nil),
				implMock.EXPECT().RemoveAll(gomock.Any()).Return(nil),
				implMock.EXPECT().GetBlob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(io.NopCloser(nil), int64(10), nil),
				implMock.EXPECT().ReadAll(gomock.Any()).Return(testArtifact, nil),
				implMock.EXPECT().WriteFile(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
			)

			// When
			res, err := sut.Pull(context.Background(), "", &ociartifact.PullOptions{CachePath: "/cache"})

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(res).NotTo(BeNil())
			Expect(res.Data).To(BeEquivalentTo(testArtifact))
		})

		It("should remove cached artifact if it read fails and succeed if write fails", func() {
			// Given

			gomock.InOrder(
				implMock.EXPECT().ParseNormalizedNamed(gomock.Any()).Return(testRef, nil),
				implMock.EXPECT().NewReference(gomock.Any()).Return(nil, nil),
				implMock.EXPECT().NewImageSource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().GetManifest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, "", nil),
				implMock.EXPECT().ManifestFromBlob(gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().LayerInfos(gomock.Any()).Return([]manifest.LayerInfo{
					{BlobInfo: types.BlobInfo{Digest: testArtifactDigest}},
				}),
				implMock.EXPECT().MkdirAll(gomock.Any(), gomock.Any()).Return(nil),
				implMock.EXPECT().ReadDir(gomock.Any()).Return([]os.DirEntry{}, nil),
				implMock.EXPECT().ReadFile(gomock.Any()).Return(nil, errTest),
				implMock.EXPECT().RemoveAll(gomock.Any()).Return(errTest),
				implMock.EXPECT().GetBlob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(io.NopCloser(nil), int64(10), nil),
				implMock.EXPECT().ReadAll(gomock.Any()).Return(testArtifact, nil),
				implMock.EXPECT().WriteFile(gomock.Any(), gomock.Any(), gomock.Any()).Return(errTest),
			)

			// When
			res, err := sut.Pull(context.Background(), "", &ociartifact.PullOptions{CachePath: "/cache"})

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(res).NotTo(BeNil())
			Expect(res.Data).To(BeEquivalentTo(testArtifact))
		})

		It("should succeed if cache creation fails", func() {
			// Given

			gomock.InOrder(
				implMock.EXPECT().ParseNormalizedNamed(gomock.Any()).Return(testRef, nil),
				implMock.EXPECT().NewReference(gomock.Any()).Return(nil, nil),
				implMock.EXPECT().NewImageSource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().GetManifest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, "", nil),
				implMock.EXPECT().ManifestFromBlob(gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().LayerInfos(gomock.Any()).Return([]manifest.LayerInfo{
					{BlobInfo: types.BlobInfo{Digest: testArtifactDigest}},
				}),
				implMock.EXPECT().MkdirAll(gomock.Any(), gomock.Any()).Return(errTest),
				implMock.EXPECT().GetBlob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(io.NopCloser(nil), int64(10), nil),
				implMock.EXPECT().ReadAll(gomock.Any()).Return(testArtifact, nil),
			)

			// When
			res, err := sut.Pull(context.Background(), "", &ociartifact.PullOptions{CachePath: "/cache"})

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(res).NotTo(BeNil())
			Expect(res.Data).To(BeEquivalentTo(testArtifact))
		})

		It("should succeed if read cache dir fails", func() {
			// Given

			gomock.InOrder(
				implMock.EXPECT().ParseNormalizedNamed(gomock.Any()).Return(testRef, nil),
				implMock.EXPECT().NewReference(gomock.Any()).Return(nil, nil),
				implMock.EXPECT().NewImageSource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().GetManifest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, "", nil),
				implMock.EXPECT().ManifestFromBlob(gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().LayerInfos(gomock.Any()).Return([]manifest.LayerInfo{
					{BlobInfo: types.BlobInfo{Digest: testArtifactDigest}},
				}),
				implMock.EXPECT().MkdirAll(gomock.Any(), gomock.Any()).Return(nil),
				implMock.EXPECT().ReadDir(gomock.Any()).Return(nil, errTest),
				implMock.EXPECT().GetBlob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(io.NopCloser(nil), int64(10), nil),
				implMock.EXPECT().ReadAll(gomock.Any()).Return(testArtifact, nil),
			)

			// When
			res, err := sut.Pull(context.Background(), "", &ociartifact.PullOptions{CachePath: "/cache"})

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(res).NotTo(BeNil())
			Expect(res.Data).To(BeEquivalentTo(testArtifact))
		})

		//nolint:dupl
		It("should succeed if cache write fails", func() {
			// Given
			gomock.InOrder(
				implMock.EXPECT().ParseNormalizedNamed(gomock.Any()).Return(testRef, nil),
				implMock.EXPECT().NewReference(gomock.Any()).Return(nil, nil),
				implMock.EXPECT().NewImageSource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().GetManifest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, "", nil),
				implMock.EXPECT().ManifestFromBlob(gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().LayerInfos(gomock.Any()).Return([]manifest.LayerInfo{
					{BlobInfo: types.BlobInfo{Digest: testArtifactDigest}},
				}),
				implMock.EXPECT().MkdirAll(gomock.Any(), gomock.Any()).Return(nil),
				implMock.EXPECT().ReadDir(gomock.Any()).Return([]os.DirEntry{}, nil),
				implMock.EXPECT().ReadFile(gomock.Any()).Return([]byte("wrong"), nil),
				implMock.EXPECT().RemoveAll(gomock.Any()).Return(nil),
				implMock.EXPECT().GetBlob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(io.NopCloser(nil), int64(10), nil),
				implMock.EXPECT().ReadAll(gomock.Any()).Return(testArtifact, nil),
				implMock.EXPECT().WriteFile(gomock.Any(), gomock.Any(), gomock.Any()).Return(errTest),
			)

			// When
			res, err := sut.Pull(context.Background(), "", &ociartifact.PullOptions{CachePath: "/cache"})

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(res).NotTo(BeNil())
			Expect(res.Data).To(BeEquivalentTo(testArtifact))
		})

		It("should fail if digests don't match", func() {
			// Given
			gomock.InOrder(
				implMock.EXPECT().ParseNormalizedNamed(gomock.Any()).Return(testRef, nil),
				implMock.EXPECT().NewReference(gomock.Any()).Return(nil, nil),
				implMock.EXPECT().NewImageSource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().GetManifest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, "", nil),
				implMock.EXPECT().ManifestFromBlob(gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().LayerInfos(gomock.Any()).Return([]manifest.LayerInfo{
					{BlobInfo: types.BlobInfo{Digest: testArtifactDigest}},
				}),
				implMock.EXPECT().GetBlob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(io.NopCloser(nil), int64(10), nil),
				// different artifact bytes will result in a different digest
				implMock.EXPECT().ReadAll(gomock.Any()).Return([]byte{3, 2, 1}, nil),
			)

			// When
			res, err := sut.Pull(context.Background(), "", nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("sha256 mismatch between real layer bytes"))
			Expect(res).To(BeNil())
		})

		It("should fail if digests algorithm is not available", func() {
			// Given
			gomock.InOrder(
				implMock.EXPECT().ParseNormalizedNamed(gomock.Any()).Return(testRef, nil),
				implMock.EXPECT().NewReference(gomock.Any()).Return(nil, nil),
				implMock.EXPECT().NewImageSource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().GetManifest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, "", nil),
				implMock.EXPECT().ManifestFromBlob(gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().LayerInfos(gomock.Any()).Return([]manifest.LayerInfo{
					{BlobInfo: types.BlobInfo{Digest: "wrong"}}, // fails
				}),
				implMock.EXPECT().GetBlob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(io.NopCloser(nil), int64(10), nil),
				implMock.EXPECT().ReadAll(gomock.Any()).Return(testArtifact, nil),
			)

			// When
			res, err := sut.Pull(context.Background(), "", nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid digest"))
			Expect(res).To(BeNil())
		})

		It("should fail if read limit is reached", func() {
			// Given
			//nolint:dupl
			gomock.InOrder(
				implMock.EXPECT().ParseNormalizedNamed(gomock.Any()).Return(testRef, nil),
				implMock.EXPECT().NewReference(gomock.Any()).Return(nil, nil),
				implMock.EXPECT().NewImageSource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().GetManifest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, "", nil),
				implMock.EXPECT().ManifestFromBlob(gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().LayerInfos(gomock.Any()).Return([]manifest.LayerInfo{
					{BlobInfo: types.BlobInfo{Digest: testArtifactDigest}},
				}),
				implMock.EXPECT().GetBlob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(io.NopCloser(nil), int64(1), nil),
				implMock.EXPECT().ReadAll(gomock.Any()).Return(testArtifact, nil),
			)

			// When
			res, err := sut.Pull(context.Background(), "", &ociartifact.PullOptions{MaxSize: 2})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exceeded maximum allowed size"))
			Expect(res).To(BeNil())
		})

		It("should fail if ReadAll errors", func() {
			// Given
			//nolint:dupl
			gomock.InOrder(
				implMock.EXPECT().ParseNormalizedNamed(gomock.Any()).Return(testRef, nil),
				implMock.EXPECT().NewReference(gomock.Any()).Return(nil, nil),
				implMock.EXPECT().NewImageSource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().GetManifest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, "", nil),
				implMock.EXPECT().ManifestFromBlob(gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().LayerInfos(gomock.Any()).Return([]manifest.LayerInfo{
					{BlobInfo: types.BlobInfo{Digest: testArtifactDigest}},
				}),
				implMock.EXPECT().GetBlob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(io.NopCloser(nil), int64(10), nil),
				implMock.EXPECT().ReadAll(gomock.Any()).Return(nil, errTest),
			)

			// When
			res, err := sut.Pull(context.Background(), "", nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("read from limit reader"))
			Expect(res).To(BeNil())
		})

		It("should fail if maxArtifactSize is reached on GetBlob", func() {
			// Given
			gomock.InOrder(
				implMock.EXPECT().ParseNormalizedNamed(gomock.Any()).Return(testRef, nil),
				implMock.EXPECT().NewReference(gomock.Any()).Return(nil, nil),
				implMock.EXPECT().NewImageSource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().GetManifest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, "", nil),
				implMock.EXPECT().ManifestFromBlob(gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().LayerInfos(gomock.Any()).Return([]manifest.LayerInfo{
					{BlobInfo: types.BlobInfo{Digest: testArtifactDigest}},
				}),
				implMock.EXPECT().GetBlob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(io.NopCloser(nil), int64(10), nil),
			)

			// When
			res, err := sut.Pull(context.Background(), "", &ociartifact.PullOptions{MaxSize: 5})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exceeded maximum allowed size"))
			Expect(res).To(BeNil())
		})

		It("should fail if GetBlob errors", func() {
			// Given
			gomock.InOrder(
				implMock.EXPECT().ParseNormalizedNamed(gomock.Any()).Return(testRef, nil),
				implMock.EXPECT().NewReference(gomock.Any()).Return(nil, nil),
				implMock.EXPECT().NewImageSource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().GetManifest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, "", nil),
				implMock.EXPECT().ManifestFromBlob(gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().LayerInfos(gomock.Any()).Return([]manifest.LayerInfo{
					{BlobInfo: types.BlobInfo{Digest: testArtifactDigest}},
				}),
				implMock.EXPECT().GetBlob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, int64(0), errTest),
			)

			// When
			res, err := sut.Pull(context.Background(), "", nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("get layer blob"))
			Expect(res).To(BeNil())
		})

		It("should fail if not enough layers available", func() {
			// Given
			gomock.InOrder(
				implMock.EXPECT().ParseNormalizedNamed(gomock.Any()).Return(testRef, nil),
				implMock.EXPECT().NewReference(gomock.Any()).Return(nil, nil),
				implMock.EXPECT().NewImageSource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().GetManifest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, "", nil),
				implMock.EXPECT().ManifestFromBlob(gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().LayerInfos(gomock.Any()).Return([]manifest.LayerInfo{}),
			)

			// When
			res, err := sut.Pull(context.Background(), "", nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("artifact needs at least one layer"))
			Expect(res).To(BeNil())
		})

		It("should fail if wrong config media type enforced", func() {
			// Given
			gomock.InOrder(
				implMock.EXPECT().ParseNormalizedNamed(gomock.Any()).Return(testRef, nil),
				implMock.EXPECT().NewReference(gomock.Any()).Return(nil, nil),
				implMock.EXPECT().NewImageSource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().GetManifest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, "", nil),
				implMock.EXPECT().ManifestFromBlob(gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().ManifestConfigInfo(gomock.Any()).AnyTimes().Return(types.BlobInfo{MediaType: "wrong"}),
			)

			// When
			res, err := sut.Pull(context.Background(), "", &ociartifact.PullOptions{
				EnforceConfigMediaType: "media-type",
			})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("wrong config media type"))
			Expect(res).To(BeNil())
		})

		It("should fail if ManifestFromBlob errors", func() {
			// Given
			gomock.InOrder(
				implMock.EXPECT().ParseNormalizedNamed(gomock.Any()).Return(testRef, nil),
				implMock.EXPECT().NewReference(gomock.Any()).Return(nil, nil),
				implMock.EXPECT().NewImageSource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().GetManifest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, "", nil),
				implMock.EXPECT().ManifestFromBlob(gomock.Any(), gomock.Any()).Return(nil, errTest),
			)

			// When
			res, err := sut.Pull(context.Background(), "", nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parse manifest"))
			Expect(res).To(BeNil())
		})

		It("should fail if GetManifest errors", func() {
			// Given
			gomock.InOrder(
				implMock.EXPECT().ParseNormalizedNamed(gomock.Any()).Return(testRef, nil),
				implMock.EXPECT().NewReference(gomock.Any()).Return(nil, nil),
				implMock.EXPECT().NewImageSource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().GetManifest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, "", errTest),
			)

			// When
			res, err := sut.Pull(context.Background(), "", nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("get manifest"))
			Expect(res).To(BeNil())
		})

		It("should fail if NewImageSource errors", func() {
			// Given
			gomock.InOrder(
				implMock.EXPECT().ParseNormalizedNamed(gomock.Any()).Return(testRef, nil),
				implMock.EXPECT().NewReference(gomock.Any()).Return(nil, nil),
				implMock.EXPECT().NewImageSource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errTest),
			)

			// When
			res, err := sut.Pull(context.Background(), "", nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("build image source"))
			Expect(res).To(BeNil())
		})

		It("should fail if NewImageSource errors", func() {
			// Given
			gomock.InOrder(
				implMock.EXPECT().ParseNormalizedNamed(gomock.Any()).Return(testRef, nil),
				implMock.EXPECT().NewReference(gomock.Any()).Return(nil, errTest),
			)

			// When
			res, err := sut.Pull(context.Background(), "", nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("create docker reference"))
			Expect(res).To(BeNil())
		})

		It("should fail if ParseNormalizedNamed errors", func() {
			// Given
			gomock.InOrder(
				implMock.EXPECT().ParseNormalizedNamed(gomock.Any()).Return(nil, errTest),
			)

			// When
			res, err := sut.Pull(context.Background(), "", nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parse image name"))
			Expect(res).To(BeNil())
		})
	})
})
