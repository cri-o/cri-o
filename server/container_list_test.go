package server_test

import (
	"context"

	"github.com/cri-o/cri-o/internal/oci"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// The actual test suite
var _ = t.Describe("ContainerList", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerList", func() {
		DescribeTable("should succeed", func(
			givenState *oci.ContainerState,
			expectedState types.ContainerState,
			created bool,
		) {
			// Given
			addContainerAndSandbox()
			if created {
				testContainer.SetCreated()
			}
			testContainer.SetState(givenState)

			// When
			response, err := sut.ListContainers(context.Background(),
				&types.ListContainersRequest{Filter: &types.ContainerFilter{}})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response).NotTo(BeNil())
			if created {
				Expect(len(response.Containers)).To(BeEquivalentTo(1))
				Expect(response.Containers[0].State).To(Equal(expectedState))
			}
		},
			Entry("Created 1", &oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateCreated},
			}, types.ContainerState_CONTAINER_CREATED, true),
			Entry("Created 2", &oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateCreated},
			}, types.ContainerState_CONTAINER_CREATED, false),
			Entry("Running", &oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			}, types.ContainerState_CONTAINER_RUNNING, true),
			Entry("Stopped", &oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateStopped},
			}, types.ContainerState_CONTAINER_EXITED, true),
		)

		t.Describe("ContainerList Filter", func() {
			BeforeEach(func() {
				addContainerAndSandbox()
				testContainer.SetCreated()
			})

			It("should succeed with non matching filter", func() {
				// Given
				// When
				response, err := sut.ListContainers(context.Background(),
					&types.ListContainersRequest{Filter: &types.ContainerFilter{
						Id: "id",
					}})

				// Then
				Expect(err).ToNot(HaveOccurred())
				Expect(response).NotTo(BeNil())
				Expect(len(response.Containers)).To(BeEquivalentTo(0))
			})

			It("should succeed with matching filter", func() {
				// Given
				// When
				response, err := sut.ListContainers(context.Background(),
					&types.ListContainersRequest{Filter: &types.ContainerFilter{
						Id: testContainer.ID(),
					}})

				// Then
				Expect(err).ToNot(HaveOccurred())
				Expect(response).NotTo(BeNil())
				Expect(len(response.Containers)).To(BeEquivalentTo(1))
			})

			It("should succeed with non matching filter for sandbox ID", func() {
				// Given
				// When
				response, err := sut.ListContainers(context.Background(),
					&types.ListContainersRequest{Filter: &types.ContainerFilter{
						Id:           testContainer.ID(),
						PodSandboxId: "id",
					}})

				// Then
				Expect(err).ToNot(HaveOccurred())
				Expect(response).NotTo(BeNil())
				Expect(len(response.Containers)).To(BeEquivalentTo(0))
			})

			It("should succeed with matching filter for sandbox and container ID", func() {
				// Given
				// When
				response, err := sut.ListContainers(context.Background(),
					&types.ListContainersRequest{Filter: &types.ContainerFilter{
						Id:           testContainer.ID(),
						PodSandboxId: testSandbox.ID(),
					}})

				// Then
				Expect(err).ToNot(HaveOccurred())
				Expect(response).NotTo(BeNil())
				Expect(len(response.Containers)).To(BeEquivalentTo(1))
			})

			It("should succeed with matching filter for sandbox ID", func() {
				// Given
				// When
				response, err := sut.ListContainers(context.Background(),
					&types.ListContainersRequest{Filter: &types.ContainerFilter{
						PodSandboxId: testSandbox.ID(),
					}})

				// Then
				Expect(err).ToNot(HaveOccurred())
				Expect(response).NotTo(BeNil())
				Expect(len(response.Containers)).To(BeEquivalentTo(1))
			})

			It("should succeed with state filter", func() {
				// Given
				// When
				response, err := sut.ListContainers(context.Background(),
					&types.ListContainersRequest{Filter: &types.ContainerFilter{
						State: &types.ContainerStateValue{
							State: types.ContainerState_CONTAINER_RUNNING,
						},
					}})

				// Then
				Expect(err).ToNot(HaveOccurred())
				Expect(response).NotTo(BeNil())
				Expect(len(response.Containers)).To(BeEquivalentTo(0))
			})

			It("should succeed with label filter", func() {
				// Given
				// When
				response, err := sut.ListContainers(context.Background(),
					&types.ListContainersRequest{Filter: &types.ContainerFilter{
						LabelSelector: map[string]string{"label": "label"},
					}})

				// Then
				Expect(err).ToNot(HaveOccurred())
				Expect(response).NotTo(BeNil())
				Expect(len(response.Containers)).To(BeEquivalentTo(0))
			})
		})
	})
})
