package server_test

import (
	"context"

	"github.com/cri-o/cri-o/v1alpha2/oci"
	"github.com/opencontainers/runtime-spec/specs-go"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// The actual test suite
var _ = t.Describe("UpdateContainerResources", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		mockRuncInLibConfig()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("UpdateContainerResources", func() {
		It("should succeed", func() {
			// Given
			testContainer.SetSpec(&specs.Spec{
				Linux: &specs.Linux{
					Resources: &specs.LinuxResources{},
				},
			})
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			addContainerAndSandbox()

			// When
			response, err := sut.UpdateContainerResources(context.Background(),
				&pb.UpdateContainerResourcesRequest{
					ContainerId: testContainer.ID(),
				},
			)

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

		It("should update the container spec", func() {
			// Given
			testContainer.SetSpec(&specs.Spec{
				Linux: &specs.Linux{
					Resources: &specs.LinuxResources{},
				},
			})
			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			addContainerAndSandbox()

			// When
			response, err := sut.UpdateContainerResources(context.Background(),
				&pb.UpdateContainerResourcesRequest{
					ContainerId: testContainer.ID(),
					Linux: &pb.LinuxContainerResources{
						CpuPeriod:  100000,
						CpuQuota:   20000,
						CpuShares:  1024,
						CpusetCpus: "0-3,12-15",
						CpusetMems: "0,1",
					},
				},
			)

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())

			c := sut.GetContainer(testContainer.ID())
			Expect(int(*c.Spec().Linux.Resources.CPU.Period)).To(Equal(100000))
			Expect(int(*c.Spec().Linux.Resources.CPU.Quota)).To(Equal(20000))
			Expect(int(*c.Spec().Linux.Resources.CPU.Shares)).To(Equal(1024))
			Expect(c.Spec().Linux.Resources.CPU.Cpus).To(Equal("0-3,12-15"))
			Expect(c.Spec().Linux.Resources.CPU.Mems).To(Equal("0,1"))
		})

		It("should fail when container is not in created/running state", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.UpdateContainerResources(context.Background(),
				&pb.UpdateContainerResourcesRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail with invalid container id", func() {
			// Given
			// When
			response, err := sut.UpdateContainerResources(context.Background(),
				&pb.UpdateContainerResourcesRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail with empty container ID", func() {
			// Given
			// When
			response, err := sut.UpdateContainerResources(context.Background(),
				&pb.UpdateContainerResourcesRequest{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})
})
