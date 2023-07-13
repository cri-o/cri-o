package lib_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/containers/podman/v4/libpod"
	"github.com/containers/podman/v4/pkg/criu"
	"github.com/containers/storage/pkg/archive"
	"github.com/cri-o/cri-o/internal/oci"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"go.uber.org/mock/gomock"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

var _ = t.Describe("ContainerRestore", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		createDummyConfig()
		mockRuncInLibConfigCheckpoint()
		if !criu.CheckForCriu(criu.PodCriuVersion) {
			Skip("CRIU is missing or too old.")
		}
	})

	t.Describe("ContainerRestore", func() {
		It("should fail with invalid container ID", func() {
			// Given
			config := &metadata.ContainerConfig{
				ID: "invalid",
			}

			// When
			res, err := sut.ContainerRestore(
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
	t.Describe("ContainerRestore", func() {
		It("should fail with container not running", func() {
			// Given
			addContainerAndSandbox()

			myContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			myContainer.SetSpec(&specs.Spec{Version: "1.0.0"})

			config := &metadata.ContainerConfig{
				ID: containerID,
			}
			// When
			res, err := sut.ContainerRestore(
				context.Background(),
				config,
				&libpod.ContainerCheckpointOptions{},
			)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(Equal(""))
			Expect(err.Error()).To(Equal(`cannot restore running container containerID`))
		})
	})
	t.Describe("ContainerRestore", func() {
		It("should fail with invalid config", func() {
			// Given
			addContainerAndSandbox()

			gomock.InOrder(
				storeMock.EXPECT().Mount(gomock.Any(), gomock.Any()).Return("/tmp/", nil),
			)

			config := &metadata.ContainerConfig{
				ID: containerID,
			}

			// When
			res, err := sut.ContainerRestore(
				context.Background(),
				config,
				&libpod.ContainerCheckpointOptions{},
			)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(Equal(""))
			Expect(err.Error()).To(Equal(`failed to restore container containerID: a complete checkpoint for this container cannot be found, cannot restore: stat checkpoint/inventory.img: no such file or directory`))
		})
	})
	t.Describe("ContainerRestore", func() {
		It("should fail with failed to restore container", func() {
			// Given
			createDummyConfig()
			addContainerAndSandbox()
			config := &metadata.ContainerConfig{
				ID: containerID,
			}
			myContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateStopped},
			})
			myContainer.SetSpec(&specs.Spec{
				Version: "1.0.0",
				Process: &specs.Process{},
				Linux:   &specs.Linux{},
			})

			gomock.InOrder(
				storeMock.EXPECT().Mount(gomock.Any(), gomock.Any()).Return("/tmp/", nil),
			)

			err := os.Mkdir("bundle", 0o700)
			Expect(err).To(BeNil())
			setupInfraContainerWithPid(42, "bundle")
			defer os.RemoveAll("bundle")
			err = os.Mkdir("checkpoint", 0o700)
			Expect(err).To(BeNil())
			defer os.RemoveAll("checkpoint")
			inventory, err := os.OpenFile("checkpoint/inventory.img", os.O_RDONLY|os.O_CREATE, 0o644)
			Expect(err).To(BeNil())
			inventory.Close()
			// When
			res, err := sut.ContainerRestore(
				context.Background(),
				config,
				&libpod.ContainerCheckpointOptions{},
			)

			defer os.RemoveAll("restore.log")
			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(Equal(""))
			Expect(err.Error()).To(ContainSubstring(`failed to restore container containerID`))
		})
	})
	t.Describe("ContainerRestore from archive", func() {
		It("should fail with failed to restore", func() {
			// Given
			config := &metadata.ContainerConfig{
				ID: containerID,
			}

			Expect(os.WriteFile("config.json", []byte(`{"linux":{},"process":{},"mounts":[{"type":"not-bind"},{"type":"bind","source":"/"}]}`), 0o644)).To(BeNil())
			addContainerAndSandbox()

			myContainer.SetStateAndSpoofPid(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateStopped},
			})

			myContainer.SetSpec(&specs.Spec{
				Version: "1.0.0",
				Process: &specs.Process{},
				Linux:   &specs.Linux{},
			})

			gomock.InOrder(
				storeMock.EXPECT().Mount(gomock.Any(), gomock.Any()).Return("/tmp/", nil),
			)

			err := os.WriteFile("spec.dump", []byte(`{"annotations":{"io.kubernetes.cri-o.Metadata":"{\"name\":\"container-to-restore\"}"}}`), 0o644)
			Expect(err).To(BeNil())
			defer os.RemoveAll("spec.dump")
			err = os.WriteFile("config.dump", []byte(`{"rootfsImageName": "image"}`), 0o644)
			Expect(err).To(BeNil())
			defer os.RemoveAll("config.dump")

			err = os.Mkdir("checkpoint", 0o700)
			Expect(err).To(BeNil())
			defer os.RemoveAll("checkpoint")
			inventory, err := os.OpenFile("checkpoint/inventory.img", os.O_RDONLY|os.O_CREATE, 0o644)
			Expect(err).To(BeNil())
			inventory.Close()

			rootfs, err := os.OpenFile("rootfs-diff.tar", os.O_RDONLY|os.O_CREATE, 0o644)
			Expect(err).To(BeNil())
			defer os.RemoveAll("rootfs-diff.tar")
			rootfs.Close()

			err = os.WriteFile("deleted.files", []byte(`[]`), 0o644)
			Expect(err).To(BeNil())
			defer os.RemoveAll("deleted.files")

			outFile, err := os.Create("archive.tar")
			Expect(err).To(BeNil())
			defer outFile.Close()
			input, err := archive.TarWithOptions(".", &archive.TarOptions{
				Compression:      archive.Uncompressed,
				IncludeSourceDir: true,
				IncludeFiles:     []string{"spec.dump", "config.dump", "checkpoint", "deleted.files"},
			})
			Expect(err).To(BeNil())
			defer os.RemoveAll("archive.tar")
			_, err = io.Copy(outFile, input)
			Expect(err).To(BeNil())

			myContainer.SetRestoreArchive("archive.tar")
			err = os.Mkdir("bundle", 0o700)
			Expect(err).To(BeNil())
			setupInfraContainerWithPid(42, "bundle")
			defer os.RemoveAll("bundle")

			// When
			res, err := sut.ContainerRestore(
				context.Background(),
				config,
				&libpod.ContainerCheckpointOptions{},
			)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(Equal(""))
			Expect(err.Error()).To(ContainSubstring(`failed to restore container containerID: failed to`))
		})
	})
	t.Describe("ContainerRestore from OCI images", func() {
		It("should fail with failed to restore", func() {
			// Given
			config := &metadata.ContainerConfig{
				ID: containerID,
			}

			createDummyConfig()
			addContainerAndSandbox()

			myContainer.SetStateAndSpoofPid(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateStopped},
			})

			myContainer.SetSpec(&specs.Spec{
				Version: "1.0.0",
				Process: &specs.Process{},
				Linux:   &specs.Linux{},
			})

			myContainer.SetRestoreIsOCIImage(true)

			gomock.InOrder(
				storeMock.EXPECT().Mount(gomock.Any(), gomock.Any()).Return("/tmp/", nil),
				storeMock.EXPECT().MountImage(gomock.Any(), gomock.Any(), gomock.Any()).
					Return("", nil),
				storeMock.EXPECT().UnmountImage(gomock.Any(), true).
					Return(false, nil),
			)

			err := os.WriteFile("spec.dump", []byte(`{"annotations":{"io.kubernetes.cri-o.Metadata":"{\"name\":\"container-to-restore\"}"}}`), 0o644)
			Expect(err).To(BeNil())
			defer os.RemoveAll("spec.dump")
			err = os.WriteFile("config.dump", []byte(`{"rootfsImageName": "image"}`), 0o644)
			Expect(err).To(BeNil())
			defer os.RemoveAll("config.dump")

			err = os.Mkdir("checkpoint", 0o700)
			Expect(err).To(BeNil())
			defer os.RemoveAll("checkpoint")
			inventory, err := os.OpenFile("checkpoint/inventory.img", os.O_RDONLY|os.O_CREATE, 0o644)
			Expect(err).To(BeNil())
			inventory.Close()

			rootfs, err := os.OpenFile("rootfs-diff.tar", os.O_RDONLY|os.O_CREATE, 0o644)
			Expect(err).To(BeNil())
			defer os.RemoveAll("rootfs-diff.tar")
			rootfs.Close()

			err = os.WriteFile("deleted.files", []byte(`[]`), 0o644)
			Expect(err).To(BeNil())
			defer os.RemoveAll("deleted.files")

			tmpFile, err := os.CreateTemp("", "restore-test-file")
			Expect(err).To(BeNil())
			tmpDir, err := os.MkdirTemp("", "restore-test-directory")
			Expect(err).To(BeNil())
			// Remove it now and later as during restore it should be recreated
			os.RemoveAll(tmpFile.Name())
			defer os.RemoveAll(tmpFile.Name())
			os.RemoveAll(tmpDir)
			defer os.RemoveAll(tmpDir)

			bindMounts := fmt.Sprintf( //nolint:gocritic
				`[{"source": "%s","destination": "/data","file_type": "directory","permissions": 493},`,
				tmpDir,
			)
			bindMounts = fmt.Sprintf( //nolint:gocritic
				`%s{"source": "%s","destination": "/file","file_type": "file","permissions": 384}]`,
				bindMounts,
				tmpFile.Name(),
			)

			err = os.WriteFile("bind.mounts", []byte(bindMounts), 0o644)
			Expect(err).To(BeNil())
			defer os.RemoveAll("bind.mounts")

			myContainer.SetRestoreArchive("localhost/checkpoint-image:tag1")
			err = os.Mkdir("bundle", 0o700)
			Expect(err).To(BeNil())
			setupInfraContainerWithPid(42, "bundle")
			defer os.RemoveAll("bundle")

			// When
			res, err := sut.ContainerRestore(
				context.Background(),
				config,
				&libpod.ContainerCheckpointOptions{},
			)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(Equal(""))
			Expect(err.Error()).To(ContainSubstring(`failed to restore container containerID: failed to`))
		})
	})
})

func setupInfraContainerWithPid(pid int, bundle string) {
	testContainer, err := oci.NewContainer("testid", "testname", bundle,
		"/container/logs", map[string]string{},
		map[string]string{}, map[string]string{}, "image",
		"imageName", "imageRef", &types.ContainerMetadata{},
		"testsandboxid", false, false, false, "",
		"/root/for/container", time.Now(), "SIGKILL")
	Expect(err).To(BeNil())
	Expect(testContainer).NotTo(BeNil())

	cstate := &oci.ContainerState{}
	cstate.State = specs.State{
		Pid: pid,
	}
	// eat error here because callers may send invalid pids to test against
	_ = cstate.SetInitPid(pid) // nolint:errcheck
	testContainer.SetState(cstate)
	testContainer.SetSpec(&specs.Spec{
		Version:     "1.0.0",
		Annotations: map[string]string{"io.kubernetes.cri-o.SandboxID": "sandboxID"},
	})
	spec := testContainer.Spec()
	g := generate.Generator{Config: &spec}
	err = g.SaveToFile(filepath.Join(bundle, "config.json"), generate.ExportOptions{})
	Expect(err).To(BeNil())

	Expect(mySandbox.SetInfraContainer(testContainer)).To(BeNil())
}
