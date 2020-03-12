package server_test

import (
	"net/http"
	"net/http/httptest"

	"github.com/cri-o/cri-o/internal/storage"
	"github.com/go-zoo/bone"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = t.Describe("Inspect", func() {
	var (
		recorder *httptest.ResponseRecorder
		mux      *bone.Mux
	)

	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()

		recorder = httptest.NewRecorder()
		mux = sut.GetInfoMux()
		Expect(mux).NotTo(BeNil())
		Expect(recorder).NotTo(BeNil())

	})
	AfterEach(afterEach)

	t.Describe("GetInfoMux", func() {
		It("should succeed with /info route", func() {
			// Given
			// When
			request, err := http.NewRequest("GET", "/info", nil)
			mux.ServeHTTP(recorder, request)

			// Then
			Expect(err).To(BeNil())
			Expect(request).NotTo(BeNil())
			Expect(recorder.Code).To(BeEquivalentTo(http.StatusOK))
		})

		It("should succeed with valid /containers route", func() {
			// Given
			Expect(sut.AddSandbox(testSandbox)).To(BeNil())
			Expect(testSandbox.SetInfraContainer(testContainer)).To(BeNil())
			sut.AddContainer(testContainer)
			gomock.InOrder(
				imageServerMock.EXPECT().ImageStatus(gomock.Any(),
					gomock.Any()).Return(&storage.ImageResult{}, nil),
			)

			// When
			request, err := http.NewRequest("GET",
				"/containers/"+testContainer.ID(), nil)
			mux.ServeHTTP(recorder, request)

			// Then
			Expect(err).To(BeNil())
			Expect(request).NotTo(BeNil())
			Expect(recorder.Code).To(BeEquivalentTo(http.StatusOK))
		})

		It("should fail if sandbox not found on /containers route", func() {
			// Given
			Expect(sut.AddSandbox(testSandbox)).To(BeNil())
			Expect(testSandbox.SetInfraContainer(testContainer)).To(BeNil())
			sut.AddContainer(testContainer)
			Expect(sut.RemoveSandbox(testSandbox.ID())).To(BeNil())

			// When
			request, err := http.NewRequest("GET",
				"/containers/"+testContainer.ID(), nil)
			mux.ServeHTTP(recorder, request)

			// Then
			Expect(err).To(BeNil())
			Expect(request).NotTo(BeNil())
			Expect(recorder.Code).To(BeEquivalentTo(http.StatusNotFound))
		})

		It("should fail if container state is nil on /containers route", func() {
			// Given
			Expect(sut.AddSandbox(testSandbox)).To(BeNil())
			Expect(testSandbox.SetInfraContainer(testContainer)).To(BeNil())
			testContainer.SetState(nil)
			sut.AddContainer(testContainer)

			// When
			request, err := http.NewRequest("GET",
				"/containers/"+testContainer.ID(), nil)
			mux.ServeHTTP(recorder, request)

			// Then
			Expect(err).To(BeNil())
			Expect(request).NotTo(BeNil())
			Expect(recorder.Code).
				To(BeEquivalentTo(http.StatusInternalServerError))
		})

		It("should fail with empty with /containers route", func() {
			// Given
			// When
			request, err := http.NewRequest("GET", "/containers", nil)
			mux.ServeHTTP(recorder, request)

			// Then
			Expect(err).To(BeNil())
			Expect(request).NotTo(BeNil())
			Expect(recorder.Code).To(BeEquivalentTo(http.StatusNotFound))
		})

		It("should fail with invalid container ID on /containers route", func() {
			// Given
			// When
			request, err := http.NewRequest("GET", "/containers/123", nil)
			mux.ServeHTTP(recorder, request)

			// Then
			Expect(err).To(BeNil())
			Expect(request).NotTo(BeNil())
			Expect(recorder.Code).To(BeEquivalentTo(http.StatusNotFound))
		})

	})
})
