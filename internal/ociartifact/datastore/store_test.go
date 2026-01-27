package datastore_test

import (
	"context"
	"errors"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"go.podman.io/image/v5/docker/reference"
	"go.uber.org/mock/gomock"

	"github.com/cri-o/cri-o/internal/ociartifact"
	"github.com/cri-o/cri-o/internal/ociartifact/datastore"
	datastoremock "github.com/cri-o/cri-o/test/mocks/ociartifact/datastore"
)

var errTest = errors.New("test")

// The actual test suite.
var _ = t.Describe("DataStore", func() {
	t.Describe("PullData", func() {
		var (
			implMock *datastoremock.MockImpl
			mockCtrl *gomock.Controller
			testRef  reference.Named
		)

		BeforeEach(func() {
			logrus.SetOutput(io.Discard)

			mockCtrl = gomock.NewController(GinkgoT())
			implMock = datastoremock.NewMockImpl(mockCtrl)

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
			dataStore := datastore.New(store)
			dataStore.SetImpl(implMock)

			implMock.EXPECT().
				ParseNormalizedNamed(gomock.Any()).
				Return(nil, errTest)

			// When
			res, err := dataStore.PullData(context.Background(), "invalid-ref", nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get image reference"))
			Expect(res).To(BeNil())
		})

		It("should fail when DockerNewReference fails", func() {
			// Given
			store, err := ociartifact.NewStore(t.MustTempDir("artifact"), nil)
			Expect(err).NotTo(HaveOccurred())
			dataStore := datastore.New(store)
			dataStore.SetImpl(implMock)

			implMock.EXPECT().
				ParseNormalizedNamed(gomock.Any()).
				Return(testRef, nil)
			implMock.EXPECT().
				DockerNewReference(gomock.Any()).
				Return(nil, errTest)

			// When
			res, err := dataStore.PullData(context.Background(), "quay.io/crio/nginx-seccomp:v2", nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("create docker reference"))
			Expect(res).To(BeNil())
		})
	})
})
