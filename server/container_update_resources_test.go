package server_test

import (
	"context"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/opencontainers/runtime-spec/specs-go"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
			testContainer.SetStateAndSpoofPid(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			addContainerAndSandbox()

			// When
			err := sut.UpdateContainerResources(context.Background(),
				&types.UpdateContainerResourcesRequest{
					ContainerId: testContainer.ID(),
				},
			)

			// Then
			Expect(err).To(BeNil())
		})

		It("should update the container spec", func() {
			// Given
			testContainer.SetSpec(&specs.Spec{
				Linux: &specs.Linux{
					Resources: &specs.LinuxResources{},
				},
			})
			testContainer.SetStateAndSpoofPid(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			addContainerAndSandbox()

			// When
			err := sut.UpdateContainerResources(context.Background(),
				&types.UpdateContainerResourcesRequest{
					ContainerId: testContainer.ID(),
					Linux: &types.LinuxContainerResources{
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
			err := sut.UpdateContainerResources(context.Background(),
				&types.UpdateContainerResourcesRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid container id", func() {
			// Given
			// When
			err := sut.UpdateContainerResources(context.Background(),
				&types.UpdateContainerResourcesRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with empty container ID", func() {
			// Given
			// When
			err := sut.UpdateContainerResources(context.Background(),
				&types.UpdateContainerResourcesRequest{})

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
