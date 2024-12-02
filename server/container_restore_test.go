package server_test

import (
	"context"
	"io"
	"os"

	"github.com/containers/podman/v4/pkg/criu"
	cs "github.com/containers/storage"
	"github.com/containers/storage/pkg/archive"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/storage"
	crioann "github.com/cri-o/cri-o/pkg/annotations"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

var _ = t.Describe("ContainerRestore", func() {
	// Prepare the sut
	BeforeEach(func() {
		if !criu.CheckForCriu(criu.PodCriuVersion) {
			Skip("CRIU is missing or too old.")
		}
		beforeEach()
		createDummyConfig()
		mockRuncInLibConfig()
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

	t.Describe("ContainerRestore from archive into new pod", func() {
		It("should fail because archive does not exist", func() {
			// Given
			size := uint64(100)
			gomock.InOrder(
				imageServerMock.EXPECT().ResolveNames(
					gomock.Any(), gomock.Any()).
					Return([]string{"image"}, nil),
				imageServerMock.EXPECT().ImageStatus(
					gomock.Any(), gomock.Any()).
					Return(&storage.ImageResult{
						ID:   "image",
						User: "10", Size: &size,
					}, nil),
			)

			containerConfig := &types.ContainerConfig{
				Metadata: &types.ContainerMetadata{Name: "name"},
				Image: &types.ImageSpec{
					Image: "does-not-exist.tar",
				},
			}

			// When
			_, err := sut.CRImportCheckpoint(
				context.Background(),
				containerConfig,
				"",
				"",
			)

			// Then
			Expect(err.Error()).To(Equal(`failed to open checkpoint archive does-not-exist.tar for import: open does-not-exist.tar: no such file or directory`))
		})
	})
	t.Describe("ContainerRestore from archive into new pod", func() {
		It("should fail because archive is an empty file", func() {
			// Given
			archive, err := os.OpenFile("empty.tar", os.O_RDONLY|os.O_CREATE, 0o644)
			Expect(err).To(BeNil())
			archive.Close()
			defer os.RemoveAll("empty.tar")
			containerConfig := &types.ContainerConfig{
				Metadata: &types.ContainerMetadata{Name: "name"},
				Image: &types.ImageSpec{
					Image: "empty.tar",
				},
			}
			// When
			_, err = sut.CRImportCheckpoint(
				context.Background(),
				containerConfig,
				"",
				"",
			)
			// Then
			Expect(err.Error()).To(ContainSubstring(`failed to read "spec.dump": `))
		})
	})
	t.Describe("ContainerRestore from archive into new pod", func() {
		It("should fail because archive is not a tar file", func() {
			// Given
			err := os.WriteFile("no.tar", []byte("notar"), 0o644)
			Expect(err).To(BeNil())
			defer os.RemoveAll("no.tar")
			containerConfig := &types.ContainerConfig{
				Metadata: &types.ContainerMetadata{Name: "name"},
				Image: &types.ImageSpec{
					Image: "no.tar",
				},
			}
			// When
			_, err = sut.CRImportCheckpoint(
				context.Background(),
				containerConfig,
				"",
				"",
			)
			// Then
			Expect(err.Error()).To(ContainSubstring(`unpacking of checkpoint archive`))
		})
	})
	t.Describe("ContainerRestore from archive into new pod", func() {
		It("should fail because archive contains broken spec.dump", func() {
			// Given
			err := os.WriteFile("spec.dump", []byte("not json"), 0o644)
			Expect(err).To(BeNil())
			defer os.RemoveAll("spec.dump")
			outFile, err := os.Create("archive.tar")
			Expect(err).To(BeNil())
			defer outFile.Close()
			input, err := archive.TarWithOptions(".", &archive.TarOptions{
				Compression:      archive.Uncompressed,
				IncludeSourceDir: true,
				IncludeFiles:     []string{"spec.dump"},
			})
			Expect(err).To(BeNil())
			defer os.RemoveAll("archive.tar")
			_, err = io.Copy(outFile, input)
			Expect(err).To(BeNil())
			containerConfig := &types.ContainerConfig{
				Metadata: &types.ContainerMetadata{Name: "name"},
				Image: &types.ImageSpec{
					Image: "archive.tar",
				},
			}
			// When
			_, err = sut.CRImportCheckpoint(
				context.Background(),
				containerConfig,
				"",
				"",
			)
			// Then
			Expect(err.Error()).To(ContainSubstring(`failed to read "spec.dump": failed to unmarshal `))
		})
	})
	t.Describe("ContainerRestore from archive into new pod", func() {
		It("should fail because archive contains empty config.dump and spec.dump", func() {
			// Given
			err := os.WriteFile("spec.dump", []byte("{}"), 0o644)
			Expect(err).To(BeNil())
			defer os.RemoveAll("spec.dump")
			err = os.WriteFile("config.dump", []byte("{}"), 0o644)
			Expect(err).To(BeNil())
			defer os.RemoveAll("config.dump")
			outFile, err := os.Create("archive.tar")
			Expect(err).To(BeNil())
			defer outFile.Close()
			input, err := archive.TarWithOptions(".", &archive.TarOptions{
				Compression:      archive.Uncompressed,
				IncludeSourceDir: true,
				IncludeFiles:     []string{"spec.dump", "config.dump"},
			})
			Expect(err).To(BeNil())
			defer os.RemoveAll("archive.tar")
			_, err = io.Copy(outFile, input)
			Expect(err).To(BeNil())
			containerConfig := &types.ContainerConfig{
				Metadata: &types.ContainerMetadata{Name: "name"},
				Image: &types.ImageSpec{
					Image: "archive.tar",
				},
			}
			// When
			_, err = sut.CRImportCheckpoint(
				context.Background(),
				containerConfig,
				"",
				"",
			)

			// Then
			Expect(err.Error()).To(ContainSubstring(`failed to read "io.kubernetes.cri-o.Annotations": unexpected end of JSON input`))
		})
	})
	t.Describe("ContainerRestore from archive into new pod", func() {
		It("should fail because archive contains broken config.dump", func() {
			// Given
			outFile, err := os.Create("archive.tar")
			Expect(err).To(BeNil())
			defer outFile.Close()
			err = os.WriteFile("config.dump", []byte("not json"), 0o644)
			Expect(err).To(BeNil())
			defer os.RemoveAll("config.dump")
			err = os.WriteFile("spec.dump", []byte("{}"), 0o644)
			Expect(err).To(BeNil())
			defer os.RemoveAll("spec.dump")
			input, err := archive.TarWithOptions(".", &archive.TarOptions{
				Compression:      archive.Uncompressed,
				IncludeSourceDir: true,
				IncludeFiles:     []string{"spec.dump", "config.dump"},
			})
			Expect(err).To(BeNil())
			defer os.RemoveAll("archive.tar")
			_, err = io.Copy(outFile, input)
			Expect(err).To(BeNil())
			containerConfig := &types.ContainerConfig{
				Metadata: &types.ContainerMetadata{Name: "name"},
				Image: &types.ImageSpec{
					Image: "archive.tar",
				},
			}
			// When

			_, err = sut.CRImportCheckpoint(
				context.Background(),
				containerConfig,
				"",
				"",
			)

			// Then
			Expect(err.Error()).To(ContainSubstring(`failed to read "config.dump": failed to unmarshal`))
		})
	})
	t.Describe("ContainerRestore from archive into new pod", func() {
		It("should fail because archive contains empty config.dump", func() {
			// Given
			addContainerAndSandbox()

			err := os.WriteFile(
				"spec.dump",
				[]byte(`{"annotations":{"io.kubernetes.cri-o.Metadata":"{\"name\":\"container-to-restore\"}"}}`),
				0o644,
			)
			Expect(err).To(BeNil())
			defer os.RemoveAll("spec.dump")
			err = os.WriteFile("config.dump", []byte("{}"), 0o644)
			Expect(err).To(BeNil())
			defer os.RemoveAll("config.dump")
			outFile, err := os.Create("archive.tar")
			Expect(err).To(BeNil())
			defer outFile.Close()
			input, err := archive.TarWithOptions(".", &archive.TarOptions{
				Compression:      archive.Uncompressed,
				IncludeSourceDir: true,
				IncludeFiles:     []string{"spec.dump", "config.dump"},
			})
			Expect(err).To(BeNil())
			defer os.RemoveAll("archive.tar")
			_, err = io.Copy(outFile, input)
			Expect(err).To(BeNil())
			containerConfig := &types.ContainerConfig{
				Metadata: &types.ContainerMetadata{Name: "name"},
				Image: &types.ImageSpec{
					Image: "archive.tar",
				},
			}
			// When

			_, err = sut.CRImportCheckpoint(
				context.Background(),
				containerConfig,
				"",
				"",
			)

			// Then
			Expect(err.Error()).To(Equal(`failed to read "io.kubernetes.cri-o.Annotations": unexpected end of JSON input`))
		})
	})
	t.Describe("ContainerRestore from archive into new pod", func() {
		It("should fail because archive contains no actual checkpoint", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetStateAndSpoofPid(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})

			err := os.WriteFile(
				"spec.dump",
				[]byte(`{"annotations":{"io.kubernetes.cri-o.Metadata":"{\"name\":\"container-to-restore\"}"}}`),
				0o644,
			)
			Expect(err).To(BeNil())
			defer os.RemoveAll("spec.dump")
			err = os.WriteFile("config.dump", []byte(`{"rootfsImageName": "image"}`), 0o644)
			Expect(err).To(BeNil())
			defer os.RemoveAll("config.dump")
			outFile, err := os.Create("archive.tar")
			Expect(err).To(BeNil())
			defer outFile.Close()
			input, err := archive.TarWithOptions(".", &archive.TarOptions{
				Compression:      archive.Uncompressed,
				IncludeSourceDir: true,
				IncludeFiles:     []string{"spec.dump", "config.dump"},
			})
			Expect(err).To(BeNil())
			defer os.RemoveAll("archive.tar")
			_, err = io.Copy(outFile, input)
			Expect(err).To(BeNil())
			containerConfig := &types.ContainerConfig{
				Metadata: &types.ContainerMetadata{Name: "name"},
				Image: &types.ImageSpec{
					Image: "archive.tar",
				},
			}
			// When

			_, err = sut.CRImportCheckpoint(
				context.Background(),
				containerConfig,
				"",
				"",
			)

			// Then
			Expect(err.Error()).To(Equal(`failed to read "io.kubernetes.cri-o.Annotations": unexpected end of JSON input`))
		})
	})
	t.Describe("ContainerRestore from archive into new pod", func() {
		It("should fail with 'PodSandboxId should not be empty'", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetStateAndSpoofPid(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})

			err := os.WriteFile(
				"spec.dump",
				[]byte(
					`{"annotations":{"io.kubernetes.cri-o.Metadata"`+
						`:"{\"name\":\"container-to-restore\"}",`+
						`"io.kubernetes.cri-o.Annotations": "{\"name\":\"NAME\"}",`+
						`"io.kubernetes.cri-o.Labels": "{\"io.kubernetes.container.name\":\"counter\"}"}}`),
				0o644,
			)
			Expect(err).To(BeNil())
			defer os.RemoveAll("spec.dump")
			err = os.WriteFile("config.dump", []byte(`{"rootfsImageName": "image"}`), 0o644)
			Expect(err).To(BeNil())
			defer os.RemoveAll("config.dump")
			outFile, err := os.Create("archive.tar")
			Expect(err).To(BeNil())
			defer outFile.Close()
			input, err := archive.TarWithOptions(".", &archive.TarOptions{
				Compression:      archive.Uncompressed,
				IncludeSourceDir: true,
				IncludeFiles:     []string{"spec.dump", "config.dump"},
			})
			Expect(err).To(BeNil())
			defer os.RemoveAll("archive.tar")
			_, err = io.Copy(outFile, input)
			Expect(err).To(BeNil())
			containerConfig := &types.ContainerConfig{
				Metadata: &types.ContainerMetadata{Name: "name"},
				Image: &types.ImageSpec{
					Image: "archive.tar",
				},
			}
			// When

			_, err = sut.CRImportCheckpoint(
				context.Background(),
				containerConfig,
				"",
				"",
			)

			// Then
			Expect(err.Error()).To(Equal(`PodSandboxId should not be empty`))
		})
	})
	t.Describe("ContainerRestore from archive into new pod", func() {
		It("should succeed", func() {
			// Given
			addContainerAndSandbox()
			testContainer.SetStateAndSpoofPid(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})

			err := os.WriteFile(
				"spec.dump",
				[]byte(`{"annotations":{"io.kubernetes.cri-o.Metadata"`+
					`:"{\"name\":\"container-to-restore\"}",`+
					`"io.kubernetes.cri-o.Annotations": "{\"name\":\"NAME\"}",`+
					`"io.kubernetes.cri-o.Labels": "{\"io.kubernetes.container.name\":\"counter\"}",`+
					`"io.kubernetes.cri-o.SandboxID": "sandboxID"},`+
					`"mounts": [{"destination": "/proc"},`+
					`{"destination":"/data","source":"/data","options":`+
					`["rw","ro","rbind","rprivate","rshared","rslaved"]}],`+
					`"linux": {"maskedPaths": ["/proc/acpi"], "readonlyPaths": ["/proc/asound"]}}`),
				0o644,
			)
			Expect(err).To(BeNil())
			defer os.RemoveAll("spec.dump")
			err = os.WriteFile("config.dump", []byte(`{"rootfsImageName": "image"}`), 0o644)
			Expect(err).To(BeNil())
			defer os.RemoveAll("config.dump")
			outFile, err := os.Create("archive.tar")
			Expect(err).To(BeNil())
			defer outFile.Close()
			input, err := archive.TarWithOptions(".", &archive.TarOptions{
				Compression:      archive.Uncompressed,
				IncludeSourceDir: true,
				IncludeFiles:     []string{"spec.dump", "config.dump"},
			})
			Expect(err).To(BeNil())
			defer os.RemoveAll("archive.tar")
			_, err = io.Copy(outFile, input)
			Expect(err).To(BeNil())
			containerConfig := &types.ContainerConfig{
				Image: &types.ImageSpec{
					Image: "archive.tar",
				},
				Linux: &types.LinuxContainerConfig{
					Resources:       &types.LinuxContainerResources{},
					SecurityContext: &types.LinuxContainerSecurityContext{},
				},
				Metadata: &types.ContainerMetadata{
					Name: "new-container-name",
				},
				Mounts: []*types.Mount{{
					ContainerPath: "/data",
					HostPath:      "/data",
				}},
			}

			size := uint64(100)
			gomock.InOrder(
				imageServerMock.EXPECT().ResolveNames(
					gomock.Any(), gomock.Any()).
					Return([]string{"image"}, nil),

				imageServerMock.EXPECT().ImageStatus(
					gomock.Any(), gomock.Any()).
					Return(&storage.ImageResult{
						ID:   "image",
						User: "10", Size: &size,
						Annotations: map[string]string{
							crioann.CheckpointAnnotationName: "foo",
						},
					}, nil),

				runtimeServerMock.EXPECT().CreateContainer(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).
					Return(storage.ContainerInfo{
						Config: &v1.Image{
							Config: v1.ImageConfig{
								Entrypoint: []string{"sh"},
							},
						},
					},
						nil,
					),
				runtimeServerMock.EXPECT().StartContainer(gomock.Any()).
					Return(emptyDir, nil),
			)

			// When

			_, err = sut.CRImportCheckpoint(
				context.Background(),
				containerConfig,
				"",
				"",
			)

			// Then
			Expect(err).To(BeNil())
		})
	})
	t.Describe("ContainerRestore from OCI archive", func() {
		It("should fail because archive does not exist", func() {
			// Given
			size := uint64(100)
			gomock.InOrder(
				imageServerMock.EXPECT().ResolveNames(
					gomock.Any(), gomock.Any()).
					Return([]string{"image"}, nil),
				imageServerMock.EXPECT().ImageStatus(
					gomock.Any(), gomock.Any()).
					Return(&storage.ImageResult{
						ID:   "image",
						User: "10", Size: &size,
						Annotations: map[string]string{
							crioann.CheckpointAnnotationName: "foo",
						},
					}, nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().GraphOptions().Return([]string{}),
				storeMock.EXPECT().GraphDriverName().Return(""),
				storeMock.EXPECT().GraphRoot().Return(""),
				storeMock.EXPECT().RunRoot().Return(""),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().Image(gomock.Any()).
					Return(&cs.Image{
						ID: "abcdef",
						Names: []string{
							"localhost/checkpoint-image:tag1",
						},
					}, nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().MountImage(gomock.Any(), gomock.Any(), gomock.Any()).
					Return("", nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().UnmountImage(gomock.Any(), true).
					Return(false, nil),
			)
			containerConfig := &types.ContainerConfig{
				Metadata: &types.ContainerMetadata{Name: "name"},
				Image: &types.ImageSpec{
					Image: "localhost/checkpoint-image:tag1",
				},
			}
			// When
			_, err := sut.CRImportCheckpoint(
				context.Background(),
				containerConfig,
				"",
				"",
			)

			// Then
			Expect(err.Error()).To(ContainSubstring(`failed to read "spec.dump": open spec.dump: no such file or directory`))
		})
	})
})
