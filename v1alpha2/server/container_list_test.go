package server_test

import (
	"context"

	"github.com/cri-o/cri-o/v1alpha2/oci"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
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
			expectedState pb.ContainerState,
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
				&pb.ListContainersRequest{Filter: &pb.ContainerFilter{}})

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
			}, pb.ContainerState_CONTAINER_CREATED, true),
			Entry("Created 2", &oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateCreated},
			}, pb.ContainerState_CONTAINER_CREATED, false),
			Entry("Running", &oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			}, pb.ContainerState_CONTAINER_RUNNING, true),
			Entry("Stopped", &oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateStopped},
			}, pb.ContainerState_CONTAINER_EXITED, true),
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
					&pb.ListContainersRequest{Filter: &pb.ContainerFilter{
						Id: "id",
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
					&pb.ListContainersRequest{Filter: &pb.ContainerFilter{
						Id: testContainer.ID(),
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
					&pb.ListContainersRequest{Filter: &pb.ContainerFilter{
						Id:           testContainer.ID(),
						PodSandboxId: "id",
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
					&pb.ListContainersRequest{Filter: &pb.ContainerFilter{
						Id:           testContainer.ID(),
						PodSandboxId: testSandbox.ID(),
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
					&pb.ListContainersRequest{Filter: &pb.ContainerFilter{
						PodSandboxId: testSandbox.ID(),
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
					&pb.ListContainersRequest{Filter: &pb.ContainerFilter{
						State: &pb.ContainerStateValue{
							State: pb.ContainerState_CONTAINER_RUNNING,
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
					&pb.ListContainersRequest{Filter: &pb.ContainerFilter{
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
