package server_test

import (
	"context"
	"os"

	criu "github.com/checkpoint-restore/go-criu/v7/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
)

var _ = t.Describe("ContainerCheckpoint", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		createDummyConfig()
		mockRuncInLibConfig()
		if err := criu.CheckForCriu(criu.PodCriuVersion); err != nil {
			Skip("Check CRIU: " + err.Error())
		}
		serverConfig.SetCheckpointRestore(true)
		setupSUT()
	})

	AfterEach(func() {
		afterEach()
		os.RemoveAll("config.dump")
		os.RemoveAll("cp.tar")
		os.RemoveAll("dump.log")
		os.RemoveAll("spec.dump")
	})

	t.Describe("ContainerCheckpoint", func() {
		It("should succeed", func() {
			// Given
			addContainerAndSandbox()

			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			testContainer.SetSpec(&specs.Spec{Version: "1.0.0"})

			// When
			_, err := sut.CheckpointContainer(
				context.Background(),
				&types.CheckpointContainerRequest{
					ContainerId: testContainer.ID(),
				},
			)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail with invalid container id", func() {
			// Given
			// When
			_, err := sut.CheckpointContainer(
				context.Background(),
				&types.CheckpointContainerRequest{
					ContainerId: testContainer.ID(),
				},
			)

			// Then
			Expect(err).To(HaveOccurred())
		})
	})
})

var _ = t.Describe("ContainerCheckpoint with CheckpointRestore set to false", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		createDummyConfig()
		mockRuncInLibConfig()
		serverConfig.SetCheckpointRestore(false)
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerCheckpoint", func() {
		It("should fail with checkpoint/restore support not available", func() {
			// Given
			// When
			_, err := sut.CheckpointContainer(
				context.Background(),
				&types.CheckpointContainerRequest{
					ContainerId: testContainer.ID(),
				},
			)

			// Then
			Expect(err.Error()).To(Equal(`checkpoint/restore support not available`))
		})
	})
})
