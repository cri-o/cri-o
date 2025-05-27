package server_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
)

// The actual test suite.
var _ = t.Describe("ListPodSandbox", func() {
	ctx := context.TODO()
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ListPodSandbox", func() {
		It("should succeed", func() {
			// Given
			Expect(sut.AddSandbox(ctx, testSandbox)).To(Succeed())
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			testSandbox.SetCreated()
			Expect(testSandbox.SetInfraContainer(testContainer)).To(Succeed())

			// When
			response, err := sut.ListPodSandbox(context.Background(),
				&types.ListPodSandboxRequest{})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response).NotTo(BeNil())
			Expect(len(response.Items)).To(BeEquivalentTo(1))
		})

		It("should succeed without infra container", func() {
			// Given
			Expect(sut.AddSandbox(ctx, testSandbox)).To(Succeed())
			testSandbox.SetCreated()

			// When
			response, err := sut.ListPodSandbox(context.Background(),
				&types.ListPodSandboxRequest{})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response).NotTo(BeNil())
			// the sandbox is created, and even though it has no infra container, it should be displayed
			Expect(response.Items).To(HaveLen(1))
		})

		It("should skip not created sandboxes", func() {
			// Given
			Expect(sut.AddSandbox(ctx, testSandbox)).To(Succeed())
			Expect(testSandbox.SetInfraContainer(testContainer)).To(Succeed())

			// When
			response, err := sut.ListPodSandbox(context.Background(),
				&types.ListPodSandboxRequest{})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response).NotTo(BeNil())
			Expect(response.Items).To(BeEmpty())
		})

		It("should succeed with filter", func() {
			// Given
			mockDirs(testManifest)
			createDummyState()
			_, err := sut.LoadSandbox(context.Background(), sandboxID)
			Expect(err).ToNot(HaveOccurred())

			// When
			response, err := sut.ListPodSandbox(context.Background(),
				&types.ListPodSandboxRequest{Filter: &types.PodSandboxFilter{
					Id: sandboxID,
				}})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response).NotTo(BeNil())
			Expect(len(response.Items)).To(BeEquivalentTo(1))
		})

		It("should succeed with filter for state", func() {
			// Given
			mockDirs(testManifest)
			createDummyState()
			sb, err := sut.LoadSandbox(context.Background(), sandboxID)
			Expect(err).ToNot(HaveOccurred())
			sb.SetStopped(context.Background(), false)

			// When
			response, err := sut.ListPodSandbox(context.Background(),
				&types.ListPodSandboxRequest{Filter: &types.PodSandboxFilter{
					Id: sandboxID,
					State: &types.PodSandboxStateValue{
						State: types.PodSandboxState_SANDBOX_READY,
					},
				}})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response).NotTo(BeNil())
			Expect(response.Items).To(BeEmpty())
		})

		It("should succeed with filter for label", func() {
			// Given
			mockDirs(testManifest)
			createDummyState()
			_, err := sut.LoadSandbox(context.Background(), sandboxID)
			Expect(err).ToNot(HaveOccurred())

			// When
			response, err := sut.ListPodSandbox(context.Background(),
				&types.ListPodSandboxRequest{Filter: &types.PodSandboxFilter{
					Id:            sandboxID,
					LabelSelector: map[string]string{"label": "value"},
				}})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response).NotTo(BeNil())
			Expect(response.Items).To(BeEmpty())
		})

		It("should succeed with filter but when not finding id", func() {
			// Given
			Expect(sut.AddSandbox(ctx, testSandbox)).To(Succeed())

			// When
			response, err := sut.ListPodSandbox(context.Background(),
				&types.ListPodSandboxRequest{Filter: &types.PodSandboxFilter{
					Id: sandboxID,
				}})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response).NotTo(BeNil())
			Expect(response.Items).To(BeEmpty())
		})
	})
})
