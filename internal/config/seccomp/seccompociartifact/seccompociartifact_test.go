package seccompociartifact_test

import (
	"context"
	"errors"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"go.uber.org/mock/gomock"

	"github.com/cri-o/cri-o/internal/config/seccomp/seccompociartifact"
	"github.com/cri-o/cri-o/internal/ociartifact"
	v2 "github.com/cri-o/cri-o/pkg/annotations/v2"
	seccompociartifactmock "github.com/cri-o/cri-o/test/mocks/seccompociartifact"
)

// The actual test suite.
var _ = t.Describe("SeccompOCIArtifact", func() {
	t.Describe("TryPull", func() {
		const testProfileContent = "{}"

		var (
			sut           *seccompociartifact.SeccompOCIArtifact
			testArtifacts []ociartifact.ArtifactData
			implMock      *seccompociartifactmock.MockImpl
			mockCtrl      *gomock.Controller
			errTest       = errors.New("test")
			tempDir       string
			err           error
		)

		BeforeEach(func() {
			logrus.SetOutput(io.Discard)

			tempDir = t.MustTempDir("ociartifact")

			sut, err = seccompociartifact.New(tempDir, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(sut).NotTo(BeNil())

			mockCtrl = gomock.NewController(GinkgoT())
			implMock = seccompociartifactmock.NewMockImpl(mockCtrl)
			sut.SetImpl(implMock)

			testArtifact := ociartifact.ArtifactData{}
			testArtifact.SetData([]byte(testProfileContent))
			testArtifacts = []ociartifact.ArtifactData{testArtifact}
		})

		AfterEach(func() {
			mockCtrl.Finish()
		})

		It("should be a noop without matching annotations", func() {
			// Given
			// When
			res, err := sut.TryPull(context.Background(), "", nil, nil)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(BeNil())
		})

		It("should match image specific annotation for whole pod", func() {
			// Given
			gomock.InOrder(
				implMock.EXPECT().PullData(gomock.Any(), gomock.Any(), gomock.Any()).Return(testArtifacts, nil),
			)

			// When
			res, err := sut.TryPull(context.Background(), "", nil,
				map[string]string{
					seccompociartifact.SeccompProfilePodAnnotation: "test",
				})

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(BeEquivalentTo(testProfileContent))
		})

		It("should match image specific annotation for container", func() {
			// Given
			gomock.InOrder(
				implMock.EXPECT().PullData(gomock.Any(), gomock.Any(), gomock.Any()).Return(testArtifacts, nil),
			)

			// When
			res, err := sut.TryPull(context.Background(), "container", nil,
				map[string]string{
					v2.SeccompProfile + "/container": "test",
				})

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(BeEquivalentTo(testProfileContent))
		})

		It("should match pod specific annotation", func() {
			// Given
			gomock.InOrder(
				implMock.EXPECT().PullData(gomock.Any(), gomock.Any(), gomock.Any()).Return(testArtifacts, nil),
			)

			// When
			res, err := sut.TryPull(context.Background(), "",
				map[string]string{
					seccompociartifact.SeccompProfilePodAnnotation: "test",
				}, nil)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(BeEquivalentTo(testProfileContent))
		})

		It("should match container specific annotation", func() {
			// Given
			gomock.InOrder(
				implMock.EXPECT().PullData(gomock.Any(), gomock.Any(), gomock.Any()).Return(testArtifacts, nil),
			)

			// When
			res, err := sut.TryPull(context.Background(), "container",
				map[string]string{
					v2.SeccompProfile + "/container": "test",
				}, nil)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(BeEquivalentTo(testProfileContent))
		})

		It("should not match if container name is different", func() {
			// Given
			// When
			res, err := sut.TryPull(context.Background(), "another-container",
				map[string]string{
					v2.SeccompProfile + "/container": "test",
				}, nil)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(BeNil())
		})

		It("should fail if artifact pull fails", func() {
			// Given
			gomock.InOrder(
				implMock.EXPECT().PullData(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errTest),
			)

			// When
			res, err := sut.TryPull(context.Background(), "", nil,
				map[string]string{
					seccompociartifact.SeccompProfilePodAnnotation: "test",
				})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(res).To(BeNil())
		})
	})
})
