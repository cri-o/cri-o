package server_test

import (
	"context"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/server/cri/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
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
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
			if created {
				Expect(len(response.Containers)).To(BeEquivalentTo(1))
				Expect(response.Containers[0].State).To(Equal(expectedState))
			}
		},
			Entry("Created 1", &oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateCreated},
			}, types.ContainerStateContainerCreated, true),
			Entry("Created 2", &oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateCreated},
			}, types.ContainerStateContainerCreated, false),
			Entry("Running", &oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			}, types.ContainerStateContainerRunning, true),
			Entry("Stopped", &oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateStopped},
			}, types.ContainerStateContainerExited, true),
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
						ID: "id",
					}})

				// Then
				Expect(err).To(BeNil())
				Expect(response).NotTo(BeNil())
				Expect(len(response.Containers)).To(BeEquivalentTo(0))
			})

			It("should succeed with matching filter", func() {
				// Given
				// When
				response, err := sut.ListContainers(context.Background(),
					&types.ListContainersRequest{Filter: &types.ContainerFilter{
						ID: testContainer.ID(),
					}})

				// Then
				Expect(err).To(BeNil())
				Expect(response).NotTo(BeNil())
				Expect(len(response.Containers)).To(BeEquivalentTo(1))
			})

			It("should succeed with non matching filter for sandbox ID", func() {
				// Given
				// When
				response, err := sut.ListContainers(context.Background(),
					&types.ListContainersRequest{Filter: &types.ContainerFilter{
						ID:           testContainer.ID(),
						PodSandboxID: "id",
					}})

				// Then
				Expect(err).To(BeNil())
				Expect(response).NotTo(BeNil())
				Expect(len(response.Containers)).To(BeEquivalentTo(0))
			})

			It("should succeed with matching filter for sandbox and container ID", func() {
				// Given
				// When
				response, err := sut.ListContainers(context.Background(),
					&types.ListContainersRequest{Filter: &types.ContainerFilter{
						ID:           testContainer.ID(),
						PodSandboxID: testSandbox.ID(),
					}})

				// Then
				Expect(err).To(BeNil())
				Expect(response).NotTo(BeNil())
				Expect(len(response.Containers)).To(BeEquivalentTo(1))
			})

			It("should succeed with matching filter for sandbox ID", func() {
				// Given
				// When
				response, err := sut.ListContainers(context.Background(),
					&types.ListContainersRequest{Filter: &types.ContainerFilter{
						PodSandboxID: testSandbox.ID(),
					}})

				// Then
				Expect(err).To(BeNil())
				Expect(response).NotTo(BeNil())
				Expect(len(response.Containers)).To(BeEquivalentTo(1))
			})

			It("should succeed with state filter", func() {
				// Given
				// When
				response, err := sut.ListContainers(context.Background(),
					&types.ListContainersRequest{Filter: &types.ContainerFilter{
						State: &types.ContainerStateValue{
							State: types.ContainerStateContainerRunning,
						},
					}})

				// Then
				Expect(err).To(BeNil())
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
				Expect(err).To(BeNil())
				Expect(response).NotTo(BeNil())
				Expect(len(response.Containers)).To(BeEquivalentTo(0))
			})
		})
	})
})
