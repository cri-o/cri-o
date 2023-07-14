package lib_test

import (
	"context"
	"fmt"
	"os"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/containers/podman/v4/libpod"
	"github.com/containers/podman/v4/pkg/criu"
	cstorage "github.com/containers/storage"
	"github.com/containers/storage/pkg/archive"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// The actual test suite
var _ = t.Describe("ContainerCheckpoint", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		createDummyConfig()
		mockRuncInLibConfig()
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
				&libpod.ContainerCheckpointOptions{},
			)

			// Then
			Expect(err).NotTo(BeNil())
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
				&libpod.ContainerCheckpointOptions{},
			)

			// Then
			Expect(err).To(BeNil())
			Expect(res).To(Equal(config.ID))
		})
	})
	t.Describe("ContainerCheckpoint", func() {
		It("should fail because runtime failure (/bin/false)", func() {
			// Given
			mockRuncToFalseInLibConfig()

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
				&libpod.ContainerCheckpointOptions{},
			)

			// Then
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring(`failed to checkpoint container containerID`))
		})
	})
	t.Describe("ContainerCheckpoint", func() {
		It("should fail with export", func() {
			// Given
			// Overwrite container config to add external bind mounts
			tmpFile, err := os.CreateTemp("", "restore-test-file")
			Expect(err).To(BeNil())
			tmpDir, err := os.MkdirTemp("", "restore-test-directory")
			Expect(err).To(BeNil())
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
			containerConfig = fmt.Sprintf(
				`%s{"source":"/tmp","destination":"/tmp","type":"no-bind"},`,
				containerConfig,
			)
			containerConfig = fmt.Sprintf(
				`%s{"source":"/proc","destination":"/proc","type":"bind"}]}`,
				containerConfig,
			)
			containerConfig = fmt.Sprintf(
				`%s]}`,
				containerConfig,
			)

			fmt.Printf("json:%s\n", containerConfig)

			Expect(os.WriteFile("config.json", []byte(containerConfig), 0o644)).To(BeNil())

			addContainerAndSandbox()
			config := &metadata.ContainerConfig{
				ID: containerID,
			}
			opts := &libpod.ContainerCheckpointOptions{
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
			Expect(err).To(BeNil())
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
				&libpod.ContainerCheckpointOptions{},
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
				&libpod.ContainerCheckpointOptions{},
			)

			// Then
			Expect(err).NotTo(BeNil())
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
				&libpod.ContainerCheckpointOptions{},
			)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(Equal(""))
			Expect(err.Error()).To(Equal(`not able to read config for container "containerID": template configuration at config.json not found`))
		})
	})
})
