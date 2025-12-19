package ociartifact_test

import (
	"context"
	"errors"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"go.podman.io/common/pkg/libartifact"
	"go.podman.io/image/v5/docker/reference"
	"go.podman.io/image/v5/oci/layout"
	"go.podman.io/image/v5/types"
	"go.uber.org/mock/gomock"

	"github.com/cri-o/cri-o/internal/ociartifact"
	ociartifactmock "github.com/cri-o/cri-o/test/mocks/ociartifact"
)

var errTest = errors.New("test")

// The actual test suite.
var _ = t.Describe("OCIArtifact", func() {
	t.Describe("PullData", func() {
		var (
			implMock *ociartifactmock.MockImpl
			mockCtrl *gomock.Controller
			testRef  reference.Named
		)

		BeforeEach(func() {
			logrus.SetOutput(io.Discard)

			mockCtrl = gomock.NewController(GinkgoT())
			implMock = ociartifactmock.NewMockImpl(mockCtrl)

			var err error
			testRef, err = reference.ParseNormalizedNamed("quay.io/crio/nginx-seccomp:v2")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			mockCtrl.Finish()
		})

		It("should fail when ParseNormalizedNamed fails", func() {
			// Given
			store, err := ociartifact.NewStore(t.MustTempDir("artifact"), nil)
			Expect(err).NotTo(HaveOccurred())
			store.SetImpl(implMock)

			implMock.EXPECT().
				ParseNormalizedNamed(gomock.Any()).
				Return(nil, errTest)

			// When
			res, err := store.PullData(context.Background(), "invalid-ref", nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get image reference"))
			Expect(res).To(BeNil())
		})

		It("should fail when DockerNewReference fails", func() {
			// Given
			store, err := ociartifact.NewStore(t.MustTempDir("artifact"), nil)
			Expect(err).NotTo(HaveOccurred())
			store.SetImpl(implMock)

			implMock.EXPECT().
				ParseNormalizedNamed(gomock.Any()).
				Return(testRef, nil)
			implMock.EXPECT().
				DockerNewReference(gomock.Any()).
				Return(nil, errTest)

			// When
			res, err := store.PullData(context.Background(), "quay.io/crio/nginx-seccomp:v2", nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("create docker reference"))
			Expect(res).To(BeNil())
		})
	})

	t.Describe("Status with unqualified names", func() {
		var (
			sut        *ociartifact.Store
			err        error
			implMock   *ociartifactmock.MockImpl
			libartMock *ociartifactmock.MockLibartifactStore
			mockCtrl   *gomock.Controller
		)

		BeforeEach(func() {
			logrus.SetOutput(io.Discard)

			mockCtrl = gomock.NewController(GinkgoT())
			implMock = ociartifactmock.NewMockImpl(mockCtrl)
			libartMock = ociartifactmock.NewMockLibartifactStore(mockCtrl)

			sut, err = ociartifact.NewStore(t.MustTempDir("artifact"), nil)
			Expect(err).NotTo(HaveOccurred())
			sut.SetImpl(implMock)
			sut.SetFakeStore(ociartifact.FakeLibartifactStore{libartMock})
		})

		AfterEach(func() {
			mockCtrl.Finish()
		})

		It("should return ErrNotFound for unqualified names when store is empty", func() {
			// Given - empty artifact store
			implMock.EXPECT().CandidatesForPotentiallyShortImageName(gomock.Any(), "image").
				Return(nil, errors.New(`artifact "image" must be a fully-qualified reference; short names and unqualified-search-registries are not supported for artifacts`))
			libartMock.EXPECT().SystemContext().
				Return(&types.SystemContext{})
			libartMock.EXPECT().List(gomock.Any()).
				Return(libartifact.ArtifactList{}, nil)

			artifact, err := sut.Status(context.Background(), "image")

			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, ociartifact.ErrNotFound)).To(BeTrue())
			Expect(artifact).To(BeNil())
		})

		It("should return validation error for unqualified names when store has artifacts", func() {
			implMock.EXPECT().CandidatesForPotentiallyShortImageName(gomock.Any(), "shortname").
				Return(nil, errors.New(`artifact "shortname" must be a fully-qualified reference; short names and unqualified-search-registries are not supported for artifacts`))
			libartMock.EXPECT().SystemContext().
				Return(&types.SystemContext{})
			libartMock.EXPECT().List(gomock.Any()).
				Return(libartifact.ArtifactList{&libartifact.Artifact{}}, nil)

			artifact, err := sut.Status(context.Background(), "shortname")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must be a fully-qualified reference"))
			Expect(errors.Is(err, ociartifact.ErrNotFound)).To(BeFalse())
			Expect(artifact).To(BeNil())
		})

		It("should succeed with fully-qualified names", func() {
			name := "quay.io/crio/artifact:multiarch"
			testRef, err := reference.ParseNormalizedNamed(name)
			Expect(err).NotTo(HaveOccurred())
			_, err = layout.NewReference("", "test-digest")
			Expect(err).NotTo(HaveOccurred())

			implMock.EXPECT().CandidatesForPotentiallyShortImageName(gomock.Any(), "quay.io/crio/artifact:multiarch").
				Return([]reference.Named{testRef}, nil)
			libartMock.EXPECT().SystemContext().
				Return(&types.SystemContext{})
			libartMock.EXPECT().List(gomock.Any()).
				Return(libartifact.ArtifactList{&libartifact.Artifact{Name: name}}, nil)

			_, err = sut.Status(context.Background(), "quay.io/crio/artifact:multiarch")

			Expect(err).NotTo(HaveOccurred())
		})
	})
})
