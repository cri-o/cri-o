package server_test

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

var _ = t.Describe("CheckpointPod", func() {
	BeforeEach(func() {
		beforeEach()
		createDummyConfig()
		mockRuntimeInLibConfig()
		serverConfig.SetCheckpointRestore(true)
		setupSUT()
	})

	AfterEach(func() {
		afterEach()
		os.RemoveAll("config.dump")
		os.RemoveAll("dump.log")
		os.RemoveAll("spec.dump")
	})

	t.Describe("CheckpointPod", func() {
		It("should fail with empty path", func() {
			// Given
			addContainerAndSandbox()

			// When
			_, err := sut.CheckpointPod(
				context.Background(),
				&types.CheckpointPodRequest{
					PodSandboxId: testSandbox.ID(),
					Path:         "",
				},
			)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("path (target file) is required"))
		})

		It("should fail with invalid pod sandbox ID", func() {
			// Given
			// When
			_, err := sut.CheckpointPod(
				context.Background(),
				&types.CheckpointPodRequest{
					PodSandboxId: "invalid-sandbox-id",
					Path:         "/tmp/checkpoint.tar",
				},
			)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("could not find pod sandbox"))
		})

		It("should fail with empty pod sandbox ID", func() {
			// Given
			// When
			_, err := sut.CheckpointPod(
				context.Background(),
				&types.CheckpointPodRequest{
					PodSandboxId: "",
					Path:         "/tmp/checkpoint.tar",
				},
			)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("could not find pod sandbox"))
		})

		It("should fail when PodCheckpoint fails with no containers", func() {
			// Given
			// Add sandbox but no containers so PodCheckpoint returns
			// "no containers to checkpoint"
			ctx := context.TODO()
			Expect(sut.AddSandbox(ctx, testSandbox)).To(Succeed())
			Expect(testSandbox.SetInfraContainer(testContainer)).To(Succeed())
			Expect(sut.PodIDIndex().Add(testSandbox.ID())).To(Succeed())
			testSandbox.SetCreated()

			// When
			_, err := sut.CheckpointPod(
				context.Background(),
				&types.CheckpointPodRequest{
					PodSandboxId: testSandbox.ID(),
					Path:         "/tmp/checkpoint.tar",
				},
			)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("PodCheckpoint failed"))
			Expect(err.Error()).To(ContainSubstring("no containers to checkpoint"))
		})
	})
})

var _ = t.Describe("CheckpointPod with CheckpointRestore set to false", func() {
	BeforeEach(func() {
		beforeEach()
		createDummyConfig()
		mockRuntimeInLibConfig()
		serverConfig.SetCheckpointRestore(false)
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("CheckpointPod", func() {
		It("should fail when checkpoint/restore disabled", func() {
			// Given
			// When
			_, err := sut.CheckpointPod(
				context.Background(),
				&types.CheckpointPodRequest{
					PodSandboxId: testSandbox.ID(),
					Path:         "/tmp/checkpoint.tar",
				},
			)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("checkpoint/restore support not available"))
		})
	})
})
