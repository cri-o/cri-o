package server_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runtime-spec/specs-go"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
)

// The actual test suite.
var _ = t.Describe("UpdateContainerResources", func() {
	AfterEach(afterEach)
	t.Describe("UpdateContainerResources", func() {
		// Prepare the sut
		BeforeEach(func() {
			beforeEach()
			mockRuncInLibConfig()
			setupSUT()
		})
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
			_, err := sut.UpdateContainerResources(context.Background(),
				&types.UpdateContainerResourcesRequest{
					ContainerId: testContainer.ID(),
				},
			)

			// Then
			Expect(err).ToNot(HaveOccurred())
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
			_, err := sut.UpdateContainerResources(context.Background(),
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
			Expect(err).ToNot(HaveOccurred())

			ctx := context.TODO()
			c := sut.GetContainer(ctx, testContainer.ID())
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
			_, err := sut.UpdateContainerResources(context.Background(),
				&types.UpdateContainerResourcesRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail with invalid container id", func() {
			// Given
			// When
			_, err := sut.UpdateContainerResources(context.Background(),
				&types.UpdateContainerResourcesRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail with empty container ID", func() {
			// Given
			// When
			_, err := sut.UpdateContainerResources(context.Background(),
				&types.UpdateContainerResourcesRequest{})

			// Then
			Expect(err).To(HaveOccurred())
		})
	})

	t.Describe("UpdateContainerResources with nri enabled", func() {
		// Prepare the sut
		BeforeEach(func() {
			beforeEach()
			serverConfig.NRI.Enabled = true
			mockRuncInLibConfig()
			setupSUT()
		})
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
			_, err := sut.UpdateContainerResources(context.Background(),
				&types.UpdateContainerResourcesRequest{
					ContainerId: testContainer.ID(),
				},
			)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
