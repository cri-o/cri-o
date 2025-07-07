package ociartifact_test

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/oci/layout"
	"github.com/containers/image/v5/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
	"go.uber.org/mock/gomock"

	"github.com/cri-o/cri-o/internal/ociartifact"
	ociartifactmock "github.com/cri-o/cri-o/test/mocks/ociartifact"
)

// The actual test suite.
var _ = t.Describe("OCIArtifact", func() {
	t.Describe("PullData", func() {
		var (
			sut      *ociartifact.Store
			implMock *ociartifactmock.MockImpl
			mockCtrl *gomock.Controller

			testRef            reference.Named
			testArtifactDigest = digest.Digest("sha256:039058c6f2c0cb492c533b0a4d14ef77cc0f78abccced5287d84a1a2011cfb81")
			testImageRef       types.ImageReference
			testArtifact       = []byte{1, 2, 3}

			errTest = errors.New("test")
		)

		BeforeEach(func() {
			logrus.SetOutput(io.Discard)

			tmpDir := GinkgoT().TempDir()
			sut = ociartifact.NewStore(tmpDir, nil)
			Expect(sut).NotTo(BeNil())

			mockCtrl = gomock.NewController(GinkgoT())
			implMock = ociartifactmock.NewMockImpl(mockCtrl)
			sut.SetImpl(implMock)

			var err error
			testRef, err = reference.ParseNormalizedNamed("quay.io/crio/nginx-seccomp:v2")
			Expect(err).NotTo(HaveOccurred())

			testImageRef, err = layout.NewReference("", testArtifactDigest.Encoded())
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			mockCtrl.Finish()
		})

		type mockOptions struct {
			returnedDigest         digest.Digest
			readAllErr             error
			getBlobSize            int64
			getBlobErr             error
			newImageSourceErrs     [4]error
			layoutNewReferenceErrs [2]error
			toJSONErr              error
			oci1FromManifestErr    error
			getManifestErrs        [2]error
			listErr                error
			copyErr                error
			configMediaType        string
			manifestMimeType       string
		}

		defaultMockOptions := func() *mockOptions {
			return &mockOptions{
				returnedDigest:         testArtifactDigest,
				readAllErr:             nil,
				getBlobSize:            10,
				getBlobErr:             nil,
				newImageSourceErrs:     [4]error{nil, nil, nil, nil},
				layoutNewReferenceErrs: [2]error{nil, nil},
				toJSONErr:              nil,
				oci1FromManifestErr:    nil,
				getManifestErrs:        [2]error{nil, nil},
				listErr:                nil,
				copyErr:                nil,
				configMediaType:        "",
				manifestMimeType:       "",
			}
		}

		mockCalls := func(opts *mockOptions) []any {
			res := []any{
				implMock.EXPECT().ParseNormalizedNamed(gomock.Any()).Return(testRef, nil),
				implMock.EXPECT().DockerNewReference(gomock.Any()).Return(nil, nil),
				implMock.EXPECT().DockerReferenceString(gomock.Any()).Return(""),
				implMock.EXPECT().NewImageSource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, opts.newImageSourceErrs[0]),
				implMock.EXPECT().GetManifest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, opts.manifestMimeType, opts.getManifestErrs[0]),
				implMock.EXPECT().CloseImageSource(gomock.Any()).Return(nil).AnyTimes(),
			}

			if opts.manifestMimeType != "" {
				return res
			}

			res = append(res,
				implMock.EXPECT().ManifestFromBlob(gomock.Any(), gomock.Any()).Return(&manifest.OCI1{
					Manifest: imgspecv1.Manifest{
						Config: imgspecv1.Descriptor{
							Digest:    testArtifactDigest,
							Size:      10,
							MediaType: "application/vnd.oci.image.config.v1+json",
						},
					},
				}, nil),
				implMock.EXPECT().ManifestConfigMediaType(gomock.Any()).Return(opts.configMediaType),
			)

			if opts.configMediaType != "" {
				return res
			}

			res = append(res,
				implMock.EXPECT().NewCopier(gomock.Any(), gomock.Any()).Return(nil, nil),
				implMock.EXPECT().LayoutNewReference(gomock.Any(), gomock.Any()).Return(nil, opts.layoutNewReferenceErrs[0]),
			)

			if opts.layoutNewReferenceErrs[0] != nil {
				return res
			}

			res = append(res, implMock.EXPECT().Copy(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, opts.copyErr))
			if opts.copyErr != nil {
				return res
			}

			res = append(res,
				implMock.EXPECT().CloseCopier(gomock.Any()).Return(nil),
			)

			// Layer extraction happens here in extractArtifactLayers
			mockImageSource := ociartifactmock.NewMockImageSource(mockCtrl)
			res = append(res,
				implMock.EXPECT().NewImageSource(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockImageSource, opts.newImageSourceErrs[1]),
			)

			if opts.newImageSourceErrs[1] != nil {
				return res
			}

			// Mock the layer extraction calls
			res = append(res,
				implMock.EXPECT().LayerInfos(gomock.Any()).Return([]manifest.LayerInfo{
					{
						BlobInfo: types.BlobInfo{
							Digest:    "sha256:test",
							Size:      100,
							MediaType: "application/json",
							Annotations: map[string]string{
								"org.opencontainers.image.title": "test-file.json",
							},
						},
					},
				}),
				implMock.EXPECT().GetBlob(gomock.Any(), mockImageSource, gomock.Any(), gomock.Any()).Return(io.NopCloser(strings.NewReader(`{"test": "data"}`)), int64(13), nil),
				implMock.EXPECT().ReadAll(gomock.Any()).Return([]byte(`{"test": "data"}`), nil),
				implMock.EXPECT().ToJSON(gomock.Any()).Return([]byte(`{"layer_data": "processed"}`), nil),
				mockImageSource.EXPECT().Close().Return(nil).AnyTimes(),
			)

			res = append(res,
				implMock.EXPECT().List(gomock.Any()).Return([]layout.ListResult{{Reference: testImageRef}}, opts.listErr),
			)

			if opts.listErr != nil {
				return res
			}

			mockImageSource3 := ociartifactmock.NewMockImageSource(mockCtrl)
			res = append(res, implMock.EXPECT().NewImageSource(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockImageSource3, opts.newImageSourceErrs[2]))

			if opts.newImageSourceErrs[2] != nil {
				return res
			}

			res = append(res,
				implMock.EXPECT().GetManifest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, "", opts.getManifestErrs[1]),
			)
			if opts.getManifestErrs[1] != nil {
				return append(res, mockImageSource3.EXPECT().Close().Return(nil).AnyTimes())
			}

			res = append(res, implMock.EXPECT().OCI1FromManifest(gomock.Any()).Return(nil, opts.oci1FromManifestErr))

			if opts.oci1FromManifestErr != nil {
				return append(res, mockImageSource3.EXPECT().Close().Return(nil).AnyTimes())
			}

			res = append(res,
				implMock.EXPECT().ToJSON(gomock.Any()).Return(nil, opts.toJSONErr),
			)

			res = append(res, implMock.EXPECT().CloseImageSource(mockImageSource3).Return(nil).AnyTimes())

			if opts.toJSONErr != nil {
				return res
			}

			res = append(res, implMock.EXPECT().LayoutNewReference(gomock.Any(), gomock.Any()).Return(nil, opts.layoutNewReferenceErrs[1]))

			if opts.layoutNewReferenceErrs[1] != nil {
				return res
			}

			// Fourth NewImageSource call for artifactData
			mockImageSource4 := ociartifactmock.NewMockImageSource(mockCtrl)
			res = append(res, implMock.EXPECT().NewImageSource(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockImageSource4, opts.newImageSourceErrs[3]))

			if opts.newImageSourceErrs[3] != nil {
				return res
			}

			res = append(res,
				implMock.EXPECT().LayerInfos(gomock.Any()).Return([]manifest.LayerInfo{{BlobInfo: types.BlobInfo{Digest: opts.returnedDigest}}}),
				implMock.EXPECT().GetBlob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(io.NopCloser(nil), opts.getBlobSize, opts.getBlobErr),
			)

			defaultOpts := defaultMockOptions()
			if opts.getBlobSize == 1 || (opts.getBlobSize == defaultOpts.getBlobSize && opts.getBlobErr == nil) {
				res = append(res, implMock.EXPECT().ReadAll(gomock.Any()).Return(testArtifact, opts.readAllErr))
			}

			res = append(res,
				implMock.EXPECT().CloseImageSource(gomock.Any()).Return(nil).AnyTimes(),
				// mockImageSource4.Close() is not called in the success case
			)

			return res
		}

		It("should succeed with data", func() {
			mockOptions := defaultMockOptions()
			// Remove strict ordering to allow mocks to be called in any order
			mockCalls(mockOptions)

			data, err := sut.PullData(context.Background(), "", nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(data).NotTo(BeNil())
		})

		It("should fail on invalid digest", func() {
			mockOptions := defaultMockOptions()
			mockOptions.returnedDigest = digest.Digest("invalid")
			// Remove strict ordering to allow mocks to be called in any order
			mockCalls(mockOptions)

			data, err := sut.PullData(context.Background(), "", nil)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid digest"))
			Expect(data).To(BeNil())
		})

		It("should fail on wrong digest", func() {
			mockOptions := defaultMockOptions()
			mockOptions.returnedDigest = digest.Digest("sha256:7173b809ca12ec5dee4506cd86be934c4596dd234ee82c0662eac04a8c2c71dc")
			// Remove strict ordering to allow mocks to be called in any order
			mockCalls(mockOptions)

			data, err := sut.PullData(context.Background(), "", nil)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("mismatch between real layer bytes"))
			Expect(data).To(BeNil())
		})

		It("should fail if ReadAll fails", func() {
			mockOptions := defaultMockOptions()
			mockOptions.readAllErr = errTest
			// Remove strict ordering to allow mocks to be called in any order
			mockCalls(mockOptions)

			data, err := sut.PullData(context.Background(), "", nil)

			Expect(err).To(HaveOccurred())
			Expect(data).To(BeNil())
		})

		It("should fail if maximum artifact size exceeded per layer", func() {
			mockOptions := defaultMockOptions()
			mockOptions.getBlobSize = 5 * 1024 * 1024
			// Remove strict ordering to allow mocks to be called in any order
			mockCalls(mockOptions)

			data, err := sut.PullData(context.Background(), "", nil)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exceeded maximum allowed size"))
			Expect(data).To(BeNil())
		})

		It("should fail if GetBlob fails", func() {
			mockOptions := defaultMockOptions()
			mockOptions.getBlobErr = errTest
			// Remove strict ordering to allow mocks to be called in any order
			mockCalls(mockOptions)

			data, err := sut.PullData(context.Background(), "", nil)

			Expect(err).To(HaveOccurred())
			Expect(data).To(BeNil())
		})

		It("should fail if maximum allowed artifact size exceeded", func() {
			mockOptions := defaultMockOptions()
			mockOptions.getBlobSize = 1
			// Remove strict ordering to allow mocks to be called in any order
			mockCalls(mockOptions)

			data, err := sut.PullData(context.Background(), "", &ociartifact.PullOptions{MaxSize: 2})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exceeded maximum allowed artifact size"))
			Expect(data).To(BeNil())
		})

		It("should fail if NewImageSource (2) fails", func() {
			mockOptions := defaultMockOptions()
			mockOptions.newImageSourceErrs[2] = errTest
			// Remove strict ordering to allow mocks to be called in any order
			mockCalls(mockOptions)

			data, err := sut.PullData(context.Background(), "", nil)

			Expect(err).To(HaveOccurred())
			Expect(data).To(BeNil())
		})

		It("should fail if LayoutNewReference (1) fails", func() {
			mockOptions := defaultMockOptions()
			mockOptions.layoutNewReferenceErrs[1] = errTest
			// Remove strict ordering to allow mocks to be called in any order
			mockCalls(mockOptions)

			data, err := sut.PullData(context.Background(), "", nil)

			Expect(err).To(HaveOccurred())
			Expect(data).To(BeNil())
		})

		It("should fail if JSON fails", func() {
			mockOptions := defaultMockOptions()
			mockOptions.toJSONErr = errTest
			// Remove strict ordering to allow mocks to be called in any order
			mockCalls(mockOptions)

			data, err := sut.PullData(context.Background(), "", nil)

			Expect(err).To(HaveOccurred())
			Expect(data).To(BeNil())
		})

		It("should fail if OCI1FromManifest fails", func() {
			mockOptions := defaultMockOptions()
			mockOptions.oci1FromManifestErr = errTest
			// Remove strict ordering to allow mocks to be called in any order
			mockCalls(mockOptions)

			data, err := sut.PullData(context.Background(), "", nil)

			Expect(err).To(HaveOccurred())
			Expect(data).To(BeNil())
		})

		It("should fail if GetManifest (1) fails", func() {
			mockOptions := defaultMockOptions()
			mockOptions.getManifestErrs[1] = errTest
			// Remove strict ordering to allow mocks to be called in any order
			mockCalls(mockOptions)

			data, err := sut.PullData(context.Background(), "", nil)

			Expect(err).To(HaveOccurred())
			Expect(data).To(BeNil())
		})

		It("should fail if NewImageSource (1) fails", func() {
			mockOptions := defaultMockOptions()
			mockOptions.newImageSourceErrs[1] = errTest
			// Remove strict ordering to allow mocks to be called in any order
			mockCalls(mockOptions)

			data, err := sut.PullData(context.Background(), "", nil)

			Expect(err).To(HaveOccurred())
			Expect(data).To(BeNil())
		})

		It("should fail if List fails", func() {
			mockOptions := defaultMockOptions()
			mockOptions.listErr = errTest
			// Remove strict ordering to allow mocks to be called in any order
			mockCalls(mockOptions)

			data, err := sut.PullData(context.Background(), "", nil)

			Expect(err).To(HaveOccurred())
			Expect(data).To(BeNil())
		})

		It("should fail if Copy fails", func() {
			mockOptions := defaultMockOptions()
			mockOptions.copyErr = errTest
			// Remove strict ordering to allow mocks to be called in any order
			mockCalls(mockOptions)

			data, err := sut.PullData(context.Background(), "", nil)

			Expect(err).To(HaveOccurred())
			Expect(data).To(BeNil())
		})

		It("should fail if LayoutNewReference (0) fails", func() {
			mockOptions := defaultMockOptions()
			mockOptions.layoutNewReferenceErrs[0] = errTest
			// Remove strict ordering to allow mocks to be called in any order
			mockCalls(mockOptions)

			data, err := sut.PullData(context.Background(), "", nil)

			Expect(err).To(HaveOccurred())
			Expect(data).To(BeNil())
		})

		It("should fail if config media type does not match", func() {
			mockOptions := defaultMockOptions()
			mockOptions.configMediaType = "foo"
			// Remove strict ordering to allow mocks to be called in any order
			mockCalls(mockOptions)

			data, err := sut.PullData(context.Background(), "", &ociartifact.PullOptions{EnforceConfigMediaType: "bar"})

			Expect(err).To(HaveOccurred())
			Expect(data).To(BeNil())
		})

		It("should fail if manifest mime type indicates an image", func() {
			mockOptions := defaultMockOptions()
			mockOptions.manifestMimeType = manifest.DockerV2ListMediaType
			// Remove strict ordering to allow mocks to be called in any order
			mockCalls(mockOptions)

			data, err := sut.PullData(context.Background(), "", nil)

			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, ociartifact.ErrIsAnImage)).To(BeTrue())
			Expect(data).To(BeNil())
		})
	})
})
