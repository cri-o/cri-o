package lib_test

import (
	"context"
	"fmt"
	"os"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	criu "github.com/checkpoint-restore/go-criu/v7/utils"
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
			Expect(err.Error()).To(ContainSubstring(`failed to pause container "containerID" before checkpointing`))
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

			containerConfig := fmt.Sprintf( //nolint:gocritic
				`{"linux":{},"process":{},"mounts":[{"source":"%s","destination":"/dir","type":"bind"},`,
				tmpDir,
			)
			containerConfig = fmt.Sprintf( //nolint:gocritic
				`%s{"source":"%s","destination":"/file","type":"bind"},`,
				containerConfig,
				tmpFile.Name(),
			)
			containerConfig = fmt.Sprintf( //nolint:perfsprint
				`%s{"source":"/tmp","destination":"/tmp","type":"no-bind"},`,
				containerConfig,
			)
			containerConfig = fmt.Sprintf( //nolint:perfsprint
				`%s{"source":"/proc","destination":"/proc","type":"bind"}]}`,
				containerConfig,
			)
			containerConfig = fmt.Sprintf( //nolint:perfsprint
				`%s]}`,
				containerConfig,
			)

			fmt.Printf("json:%s\n", containerConfig)

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
				storeMock.EXPECT().Changes(gomock.Any(), gomock.Any()).Return([]archive.Change{{Kind: archive.ChangeDelete, Path: "deleted.file"}}, nil),
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
			Expect(err.Error()).To(Equal(`failed to find container invalid: container with ID starting with invalid not found: ID does not exist`))
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
			Expect(err.Error()).To(ContainSubstring(`not able to read config for container "containerID"`))
		})
	})
})
