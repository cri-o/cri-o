package seccompociartifact_test

import (
	"context"
	"errors"
	"io"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/cri-o/cri-o/internal/config/ociartifact"
	"github.com/cri-o/cri-o/internal/config/seccomp/seccompociartifact"
	"github.com/cri-o/cri-o/pkg/annotations"
	ociartifactmock "github.com/cri-o/cri-o/test/mocks/ociartifact"
)

// The actual test suite
var _ = t.Describe("SeccompOCIArtifact", func() {
	t.Describe("TryPull", func() {
		const testProfileContent = "{}"

		var (
			sut                 *seccompociartifact.SeccompOCIArtifact
			testArtifact        *ociartifact.Artifact
			ociArtifactImplMock *ociartifactmock.MockImpl
			mockCtrl            *gomock.Controller
			errTest             = errors.New("test")
		)

		BeforeEach(func() {
			logrus.SetOutput(io.Discard)

			sut = seccompociartifact.New()
			Expect(sut).NotTo(BeNil())

			mockCtrl = gomock.NewController(GinkgoT())
			ociArtifactImplMock = ociartifactmock.NewMockImpl(mockCtrl)
			sut.SetOCIArtifactImpl(ociArtifactImplMock)

			testArtifact = &ociartifact.Artifact{
				Data: []byte(testProfileContent),
			}
		})

		AfterEach(func() {
			mockCtrl.Finish()
		})

		It("should be a noop without matching annotations", func() {
			// Given
			// When
			res, err := sut.TryPull(context.Background(), nil, "", nil, nil)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(BeNil())
		})

		It("should match image specific annotation for whole pod", func() {
			// Given
			gomock.InOrder(
				ociArtifactImplMock.EXPECT().Pull(gomock.Any(), gomock.Any(), gomock.Any()).Return(testArtifact, nil),
			)

			// When
			res, err := sut.TryPull(context.Background(), nil, "", nil,
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
				ociArtifactImplMock.EXPECT().Pull(gomock.Any(), gomock.Any(), gomock.Any()).Return(testArtifact, nil),
			)

			// When
			res, err := sut.TryPull(context.Background(), nil, "container", nil,
				map[string]string{
					annotations.SeccompProfileAnnotation + "/container": "test",
				})

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(BeEquivalentTo(testProfileContent))
		})

		It("should match pod specific annotation", func() {
			// Given
			gomock.InOrder(
				ociArtifactImplMock.EXPECT().Pull(gomock.Any(), gomock.Any(), gomock.Any()).Return(testArtifact, nil),
			)

			// When
			res, err := sut.TryPull(context.Background(), nil, "",
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
				ociArtifactImplMock.EXPECT().Pull(gomock.Any(), gomock.Any(), gomock.Any()).Return(testArtifact, nil),
			)

			// When
			res, err := sut.TryPull(context.Background(), nil, "container",
				map[string]string{
					annotations.SeccompProfileAnnotation + "/container": "test",
				}, nil)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(BeEquivalentTo(testProfileContent))
		})

		It("should not match if container name is different", func() {
			// Given
			// When
			res, err := sut.TryPull(context.Background(), nil, "another-container",
				map[string]string{
					annotations.SeccompProfileAnnotation + "/container": "test",
				}, nil)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(BeNil())
		})

		It("should fail if artifact pull fails", func() {
			// Given
			gomock.InOrder(
				ociArtifactImplMock.EXPECT().Pull(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errTest),
			)

			// When
			res, err := sut.TryPull(context.Background(), nil, "", nil,
				map[string]string{
					seccompociartifact.SeccompProfilePodAnnotation: "test",
				})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(res).To(BeNil())
		})
	})
})
