package lib_test

import (
	"context"
	"fmt"
	"os"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	criu "github.com/checkpoint-restore/go-criu/v8/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	cstorage "go.podman.io/storage"
	"go.podman.io/storage/pkg/archive"
	"go.uber.org/mock/gomock"

	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/oci"
)

// The actual test suite.
var _ = t.Describe("ContainerCheckpoint", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		createDummyConfig()
		mockRuntimeInLibConfig()

		if err := criu.CheckForCriu(criu.PodCriuVersion); err != nil {
			Skip("Check CRIU: " + err.Error())
		}
	})

	AfterEach(func() {
		os.RemoveAll("dump.log")
		os.RemoveAll("bind.mounts")
		removeState()
	})

	t.Describe("ContainerCheckpoint", func() {
		It("should fail with container not running", func() {
			// Given
			addContainerAndSandbox()

			config := &metadata.ContainerConfig{
				ID: containerID,
			}

			// When
			res, err := sut.ContainerCheckpoint(
				context.Background(),
				config,
				&lib.ContainerCheckpointOptions{},
			)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(res).To(Equal(""))
			Expect(err.Error()).To(Equal(`container containerID is not running`))
		})
	})
	t.Describe("ContainerCheckpoint", func() {
		It("should succeed", func() {
			// Given
			addContainerAndSandbox()

			config := &metadata.ContainerConfig{
				ID: containerID,
			}

			myContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			myContainer.SetSpec(&specs.Spec{Version: "1.0.0"})

			gomock.InOrder(
				storeMock.EXPECT().Container(gomock.Any()).Return(&cstorage.Container{}, nil),
				storeMock.EXPECT().Unmount(gomock.Any(), gomock.Any()).Return(true, nil),
			)

			// When
			res, err := sut.ContainerCheckpoint(
				context.Background(),
				config,
				&lib.ContainerCheckpointOptions{},
			)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(config.ID))
		})
	})
	t.Describe("ContainerCheckpoint", func() {
		It("should fail because runtime failure (/bin/false)", func() {
			// Given
			mockRuntimeToFalseInLibConfig()

			addContainerAndSandbox()

			config := &metadata.ContainerConfig{
				ID: containerID,
			}

			myContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			myContainer.SetSpec(&specs.Spec{Version: "1.0.0"})

			// When
			_, err := sut.ContainerCheckpoint(
				context.Background(),
				config,
				&lib.ContainerCheckpointOptions{},
			)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(
				err.Error(),
			).To(ContainSubstring(`failed to pause container "containerID" before checkpointing`))
		})
	})
	t.Describe("ContainerCheckpoint", func() {
		It("should fail with export", func() {
			// Given
			// Overwrite container config to add external bind mounts
			tmpFile, err := os.CreateTemp("", "restore-test-file")
			Expect(err).ToNot(HaveOccurred())
			tmpDir, err := os.MkdirTemp("", "restore-test-directory")
			Expect(err).ToNot(HaveOccurred())

			defer os.RemoveAll(tmpFile.Name())
			defer os.RemoveAll(tmpDir)

			containerConfig := fmt.Sprintf( //nolint:gocritic // initial JSON fragment declaration
				`{"linux":{},"process":{},"mounts":[{"source":"%s","destination":"/dir","type":"bind"},`,
				tmpDir,
			)
			containerConfig = fmt.Sprintf( //nolint:gocritic // appendAssign style is clearer for incremental JSON building
				`%s{"source":"%s","destination":"/file","type":"bind"},`,
				containerConfig,
				tmpFile.Name(),
			)
			containerConfig = fmt.Sprintf( //nolint:perfsprint // string concatenation via Sprintf is clearer for JSON building
				`%s{"source":"/tmp","destination":"/tmp","type":"no-bind"},`,
				containerConfig,
			)
			containerConfig = fmt.Sprintf( //nolint:perfsprint // string concatenation via Sprintf is clearer for JSON building
				`%s{"source":"/proc","destination":"/proc","type":"bind"}]}`,
				containerConfig,
			)

			Expect(os.WriteFile("config.json", []byte(containerConfig), 0o644)).To(Succeed())

			addContainerAndSandbox()

			config := &metadata.ContainerConfig{
				ID: containerID,
			}
			opts := &lib.ContainerCheckpointOptions{
				TargetFile: "cp.tar",
			}

			defer os.RemoveAll("cp.tar")

			myContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			myContainer.SetSpec(&specs.Spec{Version: "1.0.0"})

			gomock.InOrder(
				storeMock.EXPECT().Container(gomock.Any()).Return(&cstorage.Container{}, nil),
				storeMock.EXPECT().
					Changes(gomock.Any(), gomock.Any()).
					Return([]archive.Change{{Kind: archive.ChangeDelete, Path: "deleted.file"}}, nil),
				storeMock.EXPECT().Mount(gomock.Any(), gomock.Any()).Return("/tmp/", nil),
				storeMock.EXPECT().Container(gomock.Any()).Return(&cstorage.Container{}, nil),
				storeMock.EXPECT().Unmount(gomock.Any(), gomock.Any()).Return(true, nil),
			)

			// When
			res, err := sut.ContainerCheckpoint(context.Background(), config, opts)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(ContainSubstring(config.ID))
		})

		It("should defer archive export", func() {
			addContainerAndSandbox()

			config := &metadata.ContainerConfig{ID: containerID}
			target := "deferred-cp.tar"

			defer os.Remove(target)

			myContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			myContainer.SetSpec(&specs.Spec{Version: "1.0.0"})

			gomock.InOrder(
				storeMock.EXPECT().Container(gomock.Any()).Return(&cstorage.Container{}, nil),
				storeMock.EXPECT().Changes(gomock.Any(), gomock.Any()).Return(nil, nil),
				storeMock.EXPECT().Mount(gomock.Any(), gomock.Any()).Return("/tmp/", nil),
			)

			res, err := sut.ContainerCheckpoint(
				context.Background(),
				config,
				&lib.ContainerCheckpointOptions{
					KeepRunning:  true,
					TargetFile:   target,
					DeferArchive: true,
				},
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(config.ID))
			Expect(target).ToNot(BeAnExistingFile())

			Expect(
				sut.ExportContainerCheckpoint(context.Background(), config, target),
			).To(Succeed())
			Expect(target).To(BeAnExistingFile())
			Expect(sut.CleanupContainerCheckpoint(context.Background(), config)).To(Succeed())
			Expect(myContainer.CheckpointPath()).ToNot(BeADirectory())
		})

		It("should not archive artifacts left by an earlier checkpoint", func() {
			addContainerAndSandbox()

			config := &metadata.ContainerConfig{ID: containerID}

			// An earlier failed checkpoint left a rootfs diff behind. This run
			// has no file system changes, so it must not export the old diff.
			Expect(os.WriteFile(metadata.RootFsDiffTar, []byte("stale"), 0o600)).To(Succeed())
			defer os.Remove(metadata.RootFsDiffTar)

			myContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			myContainer.SetSpec(&specs.Spec{Version: "1.0.0"})

			gomock.InOrder(
				storeMock.EXPECT().Container(gomock.Any()).Return(&cstorage.Container{}, nil),
				storeMock.EXPECT().Changes(gomock.Any(), gomock.Any()).Return(nil, nil),
				storeMock.EXPECT().Mount(gomock.Any(), gomock.Any()).Return("/tmp/", nil),
			)

			_, err := sut.ContainerCheckpoint(
				context.Background(),
				config,
				&lib.ContainerCheckpointOptions{
					KeepRunning:  true,
					TargetFile:   "stale-cp.tar",
					DeferArchive: true,
				},
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(metadata.RootFsDiffTar).ToNot(BeAnExistingFile())
			Expect(sut.CleanupContainerCheckpoint(context.Background(), config)).To(Succeed())
		})

		It("should fail when the caller did not pause the container", func() {
			addContainerAndSandbox()

			myContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})

			_, err := sut.ContainerCheckpoint(
				context.Background(),
				&metadata.ContainerConfig{ID: containerID},
				&lib.ContainerCheckpointOptions{ContainerIsPaused: true},
			)
			Expect(err).To(MatchError(ContainSubstring("is running, expected paused")))
		})

		It("should fail to export or clean up an unknown container", func() {
			config := &metadata.ContainerConfig{ID: "unknown"}

			Expect(
				sut.ExportContainerCheckpoint(context.Background(), config, "cp.tar"),
			).To(MatchError(ContainSubstring("failed to find container")))
			Expect(
				sut.CleanupContainerCheckpoint(context.Background(), config),
			).To(MatchError(ContainSubstring("failed to find container")))
		})

		It("should fail to write the archive into a missing directory", func() {
			addContainerAndSandbox()

			// The failed export leaves the prepared metadata in the container directory.
			defer os.Remove(metadata.ConfigDumpFile)
			defer os.Remove(metadata.SpecDumpFile)

			myContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			myContainer.SetSpec(&specs.Spec{Version: "1.0.0"})

			gomock.InOrder(
				storeMock.EXPECT().Container(gomock.Any()).Return(&cstorage.Container{}, nil),
				storeMock.EXPECT().Changes(gomock.Any(), gomock.Any()).Return(nil, nil),
				storeMock.EXPECT().Mount(gomock.Any(), gomock.Any()).Return("/tmp/", nil),
			)

			_, err := sut.ContainerCheckpoint(
				context.Background(),
				&metadata.ContainerConfig{ID: containerID},
				&lib.ContainerCheckpointOptions{
					KeepRunning: true,
					TargetFile:  "missing/cp.tar",
				},
			)
			Expect(err).To(MatchError(ContainSubstring("failed to export checkpoint")))
		})

		It("should fail to export into a missing directory", func() {
			addContainerAndSandbox()

			Expect(sut.ExportContainerCheckpoint(
				context.Background(),
				&metadata.ContainerConfig{ID: containerID},
				"missing/cp.tar",
			)).To(MatchError(ContainSubstring("error creating checkpoint export file")))
		})
	})
	t.Describe("ContainerCheckpoint", func() {
		It("should fail during unmount", func() {
			// Given
			addContainerAndSandbox()

			config := &metadata.ContainerConfig{
				ID: containerID,
			}

			myContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			myContainer.SetSpec(&specs.Spec{Version: "1.0.0"})

			gomock.InOrder(
				storeMock.EXPECT().Container(gomock.Any()).Return(&cstorage.Container{}, nil),
				storeMock.EXPECT().Unmount(gomock.Any(), gomock.Any()).Return(true, t.TestError),
			)

			// When
			_, err := sut.ContainerCheckpoint(
				context.Background(),
				config,
				&lib.ContainerCheckpointOptions{},
			)

			// Then
			Expect(err.Error()).To(Equal(`failed to unmount container containerID: error`))
		})
	})
})

var _ = t.Describe("ContainerCheckpoint", func() {
	// Prepare the sut
	BeforeEach(beforeEach)

	t.Describe("ContainerCheckpoint", func() {
		It("should fail with invalid container ID", func() {
			// Given
			config := &metadata.ContainerConfig{
				ID: "invalid",
			}

			// When
			res, err := sut.ContainerCheckpoint(
				context.Background(),
				config,
				&lib.ContainerCheckpointOptions{},
			)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(res).To(Equal(""))
			Expect(
				err.Error(),
			).To(Equal(`failed to find container invalid: container with ID starting with invalid not found: ID does not exist`))
		})
	})
	t.Describe("ContainerCheckpoint", func() {
		It("should fail with invalid config", func() {
			// Given
			addContainerAndSandbox()

			config := &metadata.ContainerConfig{
				ID: containerID,
			}

			// When
			res, err := sut.ContainerCheckpoint(
				context.Background(),
				config,
				&lib.ContainerCheckpointOptions{},
			)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(res).To(Equal(""))
			Expect(
				err.Error(),
			).To(ContainSubstring(`not able to read config for container "containerID"`))
		})
	})
})
