package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runtime-spec/specs-go"
)

var _ = t.Describe("Inspect", func() {
	var (
		recorder *httptest.ResponseRecorder
		mux      *chi.Mux
	)

	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		mockRuncInLibConfig()
		setupSUT()

		recorder = httptest.NewRecorder()
		mux = sut.GetExtendInterfaceMux(false)
		Expect(mux).NotTo(BeNil())
		Expect(recorder).NotTo(BeNil())
	})
	AfterEach(afterEach)

	t.Describe("GetExtendInterfaceMux", func() {
		It("should succeed with /info route", func() {
			// Given
			// When
			request, err := http.NewRequest(http.MethodGet, "/info", http.NoBody)
			mux.ServeHTTP(recorder, request)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(request).NotTo(BeNil())
			Expect(recorder.Code).To(BeEquivalentTo(http.StatusOK))
		})

		It("should succeed with valid /containers route", func() {
			ctx := context.TODO()
			// Given
			Expect(sut.AddSandbox(ctx, testSandbox)).To(Succeed())
			testContainer.SetStateAndSpoofPid(&oci.ContainerState{})
			Expect(testSandbox.SetInfraContainer(testContainer)).To(Succeed())
			sut.AddContainer(ctx, testContainer)

			// When
			request, err := http.NewRequest(http.MethodGet,
				"/containers/"+testContainer.ID(), http.NoBody)
			mux.ServeHTTP(recorder, request)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(request).NotTo(BeNil())
			Expect(recorder.Code).To(BeEquivalentTo(http.StatusOK))
		})

		It("should fail if sandbox not found on /containers route", func() {
			ctx := context.TODO()
			// Given
			Expect(sut.AddSandbox(ctx, testSandbox)).To(Succeed())
			testContainer.SetStateAndSpoofPid(&oci.ContainerState{})
			Expect(testSandbox.SetInfraContainer(testContainer)).To(Succeed())
			sut.AddContainer(ctx, testContainer)
			Expect(sut.RemoveSandbox(ctx, testSandbox.ID())).To(Succeed())

			// When
			request, err := http.NewRequest(http.MethodGet,
				"/containers/"+testContainer.ID(), http.NoBody)
			mux.ServeHTTP(recorder, request)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(request).NotTo(BeNil())
			Expect(recorder.Code).To(BeEquivalentTo(http.StatusNotFound))
		})

		It("should fail if container state is nil on /containers route", func() {
			ctx := context.TODO()
			// Given
			Expect(sut.AddSandbox(ctx, testSandbox)).To(Succeed())
			Expect(testSandbox.SetInfraContainer(testContainer)).To(Succeed())
			testContainer.SetState(nil)
			sut.AddContainer(ctx, testContainer)

			// When
			request, err := http.NewRequest(http.MethodGet,
				"/containers/"+testContainer.ID(), http.NoBody)
			mux.ServeHTTP(recorder, request)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(request).NotTo(BeNil())
			Expect(recorder.Code).
				To(BeEquivalentTo(http.StatusInternalServerError))
		})

		It("should fail with empty with /containers route", func() {
			// Given
			// When
			request, err := http.NewRequest(http.MethodGet, "/containers", http.NoBody)
			mux.ServeHTTP(recorder, request)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(request).NotTo(BeNil())
			Expect(recorder.Code).To(BeEquivalentTo(http.StatusNotFound))
		})

		It("should fail with invalid container ID on /containers route", func() {
			// Given
			// When
			request, err := http.NewRequest(http.MethodGet, "/containers/123", http.NoBody)
			mux.ServeHTTP(recorder, request)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(request).NotTo(BeNil())
			Expect(recorder.Code).To(BeEquivalentTo(http.StatusNotFound))
		})

		It("should fail with empty on /pause route", func() {
			// Given
			// When
			request, err := http.NewRequest(http.MethodGet, "/pause", http.NoBody)
			mux.ServeHTTP(recorder, request)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(request).NotTo(BeNil())
			Expect(recorder.Code).To(BeEquivalentTo(http.StatusNotFound))
		})

		It("should fail with invalid container ID on /pause route", func() {
			// Given
			// When
			request, err := http.NewRequest(http.MethodGet, "/pause/123", http.NoBody)
			mux.ServeHTTP(recorder, request)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(request).NotTo(BeNil())
			Expect(recorder.Code).To(BeEquivalentTo(http.StatusNotFound))
		})

		It("should fail with valid container ID on /pause route because update status error", func() {
			// Given
			state := &oci.ContainerState{
				State: specs.State{
					Status: oci.ContainerStateRunning,
				},
			}
			testContainer.SetState(state)
			addContainerAndSandbox()

			// When
			request, err := http.NewRequest(http.MethodGet, "/pause/"+testContainer.ID(), http.NoBody)
			mux.ServeHTTP(recorder, request)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(request).NotTo(BeNil())
			Expect(recorder.Code).To(BeEquivalentTo(http.StatusInternalServerError))
		})

		It("should fail with valid container ID on /pause route with already paused container", func() {
			// Given
			state := &oci.ContainerState{
				State: specs.State{
					Status: oci.ContainerStatePaused,
				},
			}
			testContainer.SetState(state)
			addContainerAndSandbox()

			// When
			request, err := http.NewRequest(http.MethodGet, "/pause/"+testContainer.ID(), http.NoBody)
			mux.ServeHTTP(recorder, request)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(request).NotTo(BeNil())
			Expect(recorder.Code).To(BeEquivalentTo(http.StatusConflict))
		})

		It("should fail with empty on /unpause route", func() {
			// Given
			// When
			request, err := http.NewRequest(http.MethodGet, "/unpause", http.NoBody)
			mux.ServeHTTP(recorder, request)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(request).NotTo(BeNil())
			Expect(recorder.Code).To(BeEquivalentTo(http.StatusNotFound))
		})

		It("should fail with invalid container ID on /unpause route", func() {
			// Given
			// When
			request, err := http.NewRequest(http.MethodGet, "/unpause/123", http.NoBody)
			mux.ServeHTTP(recorder, request)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(request).NotTo(BeNil())
			Expect(recorder.Code).To(BeEquivalentTo(http.StatusNotFound))
		})

		It("should fail with valid container ID on /unpause route because update status error", func() {
			// Given
			state := &oci.ContainerState{
				State: specs.State{
					Status: oci.ContainerStatePaused,
				},
			}
			testContainer.SetState(state)
			addContainerAndSandbox()

			// When
			request, err := http.NewRequest(http.MethodGet, "/unpause/"+testContainer.ID(), http.NoBody)
			mux.ServeHTTP(recorder, request)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(request).NotTo(BeNil())
			Expect(recorder.Code).To(BeEquivalentTo(http.StatusInternalServerError))
		})

		It("should fail with valid container ID on /unpause route with running container", func() {
			// Given
			state := &oci.ContainerState{
				State: specs.State{
					Status: oci.ContainerStateRunning,
				},
			}
			testContainer.SetState(state)
			addContainerAndSandbox()

			// When
			request, err := http.NewRequest(http.MethodGet, "/unpause/"+testContainer.ID(), http.NoBody)
			mux.ServeHTTP(recorder, request)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(request).NotTo(BeNil())
			Expect(recorder.Code).To(BeEquivalentTo(http.StatusConflict))
		})
	})
})
