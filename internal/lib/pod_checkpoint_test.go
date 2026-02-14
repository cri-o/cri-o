package lib_test

import (
	"context"
	"encoding/json"
	"os"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"

	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/oci"
)

var _ = t.Describe("CheckpointedPodOptions", func() {
	t.Describe("serialization", func() {
		It("should marshal to JSON with correct fields", func() {
			opts := &metadata.CheckpointedPodOptions{
				Version:    1,
				Containers: map[string]string{"name1": "ctr1-name1", "name2": "ctr2-name2"},
			}

			data, err := json.Marshal(opts)
			Expect(err).ToNot(HaveOccurred())

			var raw map[string]any
			Expect(json.Unmarshal(data, &raw)).To(Succeed())
			Expect(raw).To(HaveKey("version"))
			Expect(raw).To(HaveKey("containers"))
			Expect(raw["version"]).To(BeEquivalentTo(1))
		})

		It("should unmarshal from JSON correctly", func() {
			jsonData := []byte(`{"version":1,"containers":{"name1":"ctr1-name1","name2":"ctr2-name2"}}`)

			opts := &metadata.CheckpointedPodOptions{}
			Expect(json.Unmarshal(jsonData, opts)).To(Succeed())
			Expect(opts.Version).To(Equal(1))
			Expect(opts.Containers).To(Equal(map[string]string{"name1": "ctr1-name1", "name2": "ctr2-name2"}))
		})

		It("should roundtrip through WriteJSONFile/ReadJSONFile", func() {
			opts := &metadata.CheckpointedPodOptions{
				Version:    1,
				Containers: map[string]string{"name1": "ctr1-name1"},
			}

			tempDir := t.MustTempDir("checkpoint-test")

			_, err := metadata.WriteJSONFile(opts, tempDir, metadata.PodOptionsFile)
			Expect(err).ToNot(HaveOccurred())

			readBack := &metadata.CheckpointedPodOptions{}
			_, err = metadata.ReadJSONFile(readBack, tempDir, metadata.PodOptionsFile)
			Expect(err).ToNot(HaveOccurred())
			Expect(readBack.Version).To(Equal(1))
			Expect(readBack.Containers).To(Equal(map[string]string{"name1": "ctr1-name1"}))
		})
	})
})

var _ = t.Describe("PodCheckpoint", func() {
	BeforeEach(func() {
		beforeEach()
		createDummyConfig()
		mockRuntimeInLibConfig()
	})

	AfterEach(func() {
		os.RemoveAll("dump.log")
	})

	t.Describe("PodCheckpoint", func() {
		It("should fail with invalid sandbox ID", func() {
			// Given
			config := &metadata.ContainerConfig{
				ID: "invalid-sandbox",
			}

			// When
			res, err := sut.PodCheckpoint(
				context.Background(),
				config,
				&lib.PodCheckpointOptions{
					TargetFile: "/tmp/pod-checkpoint.tar",
				},
			)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(res).To(Equal(""))
			Expect(err.Error()).To(Equal(`failed to find sandbox invalid-sandbox`))
		})

		It("should fail with no containers in sandbox", func() {
			// Given
			// Add sandbox but no containers
			ctx := context.TODO()
			Expect(sut.AddSandbox(ctx, mySandbox)).To(Succeed())
			Expect(sut.PodIDIndex().Add(sandboxID)).To(Succeed())

			config := &metadata.ContainerConfig{
				ID: sandboxID,
			}

			// When
			res, err := sut.PodCheckpoint(
				context.Background(),
				config,
				&lib.PodCheckpointOptions{
					TargetFile: "/tmp/pod-checkpoint.tar",
				},
			)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(res).To(Equal(""))
			Expect(err.Error()).To(Equal(`no containers to checkpoint in sandbox sandboxID`))
		})

		It("should fail to pause container with /bin/false runtime", func() {
			// Given
			mockRuntimeToFalseInLibConfig()
			addContainerAndSandbox()

			myContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			myContainer.SetSpec(&specs.Spec{Version: "1.0.0"})

			config := &metadata.ContainerConfig{
				ID: sandboxID,
			}

			// When
			_, err := sut.PodCheckpoint(
				context.Background(),
				config,
				&lib.PodCheckpointOptions{
					TargetFile: "/tmp/pod-checkpoint.tar",
				},
			)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`failed to pause container`))
		})
	})
})
