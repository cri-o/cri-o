package server_test

import (
	"context"
	"fmt"
	"io"
	"os"

	criu "github.com/checkpoint-restore/go-criu/v7/utils"
	"github.com/containers/storage/pkg/archive"
	"github.com/containers/storage/pkg/unshare"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"go.uber.org/mock/gomock"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	kubetypes "k8s.io/kubelet/pkg/types"

	"github.com/cri-o/cri-o/internal/mockutils"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/internal/storage/references"
	crioann "github.com/cri-o/cri-o/pkg/annotations"
)

var _ = t.Describe("ContainerRestore", func() {
	// Prepare the sut
	BeforeEach(func() {
		if err := criu.CheckForCriu(criu.PodCriuVersion); err != nil {
			Skip("Check CRIU: " + err.Error())
		}
		beforeEach()
		createDummyConfig()
		mockRuntimeInLibConfig()
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
			checkpointImageName, err := references.ParseRegistryImageReferenceFromOutOfProcessData("docker.io/library/does-not-exist.tar:latest")
			Expect(err).ToNot(HaveOccurred())
			imageID, err := storage.ParseStorageImageIDFromOutOfProcessData("8a788232037eaf17794408ff3df6b922a1aedf9ef8de36afdae3ed0b0381907b")
			Expect(err).ToNot(HaveOccurred())
			gomock.InOrder(
				imageServerMock.EXPECT().HeuristicallyTryResolvingStringAsIDPrefix("does-not-exist.tar").
					Return(nil),
				imageServerMock.EXPECT().CandidatesForPotentiallyShortImageName(
					gomock.Any(), "does-not-exist.tar").
					Return([]storage.RegistryImageReference{checkpointImageName}, nil),
				imageServerMock.EXPECT().ImageStatusByName(
					gomock.Any(), checkpointImageName).
					Return(&storage.ImageResult{
						ID:   imageID,
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
			_, err = sut.CRImportCheckpoint(
				context.Background(),
				containerConfig,
				testSandbox,
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
			Expect(err).ToNot(HaveOccurred())
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
				testSandbox,
				"",
			)
			// Then
			Expect(err.Error()).To(ContainSubstring(`failed to read "spec.dump": open `))
		})
	})
	t.Describe("ContainerRestore from archive into new pod", func() {
		It("should fail because archive is not a tar file", func() {
			// Given
			err := os.WriteFile("no.tar", []byte("notar"), 0o644)
			Expect(err).ToNot(HaveOccurred())
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
				testSandbox,
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
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll("spec.dump")
			outFile, err := os.Create("archive.tar")
			Expect(err).ToNot(HaveOccurred())
			defer outFile.Close()
			input, err := archive.TarWithOptions(".", &archive.TarOptions{
				Compression:      archive.Uncompressed,
				IncludeSourceDir: true,
				IncludeFiles:     []string{"spec.dump"},
			})
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll("archive.tar")
			_, err = io.Copy(outFile, input)
			Expect(err).ToNot(HaveOccurred())
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
				testSandbox,
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
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll("spec.dump")
			err = os.WriteFile("config.dump", []byte("{}"), 0o644)
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll("config.dump")
			outFile, err := os.Create("archive.tar")
			Expect(err).ToNot(HaveOccurred())
			defer outFile.Close()
			input, err := archive.TarWithOptions(".", &archive.TarOptions{
				Compression:      archive.Uncompressed,
				IncludeSourceDir: true,
				IncludeFiles:     []string{"spec.dump", "config.dump"},
			})
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll("archive.tar")
			_, err = io.Copy(outFile, input)
			Expect(err).ToNot(HaveOccurred())
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
				testSandbox,
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
			Expect(err).ToNot(HaveOccurred())
			defer outFile.Close()
			err = os.WriteFile("config.dump", []byte("not json"), 0o644)
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll("config.dump")
			err = os.WriteFile("spec.dump", []byte("{}"), 0o644)
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll("spec.dump")
			input, err := archive.TarWithOptions(".", &archive.TarOptions{
				Compression:      archive.Uncompressed,
				IncludeSourceDir: true,
				IncludeFiles:     []string{"spec.dump", "config.dump"},
			})
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll("archive.tar")
			_, err = io.Copy(outFile, input)
			Expect(err).ToNot(HaveOccurred())
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
				testSandbox,
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
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll("spec.dump")
			err = os.WriteFile("config.dump", []byte("{}"), 0o644)
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll("config.dump")
			outFile, err := os.Create("archive.tar")
			Expect(err).ToNot(HaveOccurred())
			defer outFile.Close()
			input, err := archive.TarWithOptions(".", &archive.TarOptions{
				Compression:      archive.Uncompressed,
				IncludeSourceDir: true,
				IncludeFiles:     []string{"spec.dump", "config.dump"},
			})
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll("archive.tar")
			_, err = io.Copy(outFile, input)
			Expect(err).ToNot(HaveOccurred())
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
				testSandbox,
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
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll("spec.dump")
			err = os.WriteFile("config.dump", []byte(`{"rootfsImageName": "image"}`), 0o644)
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll("config.dump")
			outFile, err := os.Create("archive.tar")
			Expect(err).ToNot(HaveOccurred())
			defer outFile.Close()
			input, err := archive.TarWithOptions(".", &archive.TarOptions{
				Compression:      archive.Uncompressed,
				IncludeSourceDir: true,
				IncludeFiles:     []string{"spec.dump", "config.dump"},
			})
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll("archive.tar")
			_, err = io.Copy(outFile, input)
			Expect(err).ToNot(HaveOccurred())
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
				testSandbox,
				"",
			)

			// Then
			Expect(err.Error()).To(Equal(`failed to read "io.kubernetes.cri-o.Annotations": unexpected end of JSON input`))
		})
	})
	t.Describe("ContainerRestore from archive into new pod", func() {
		images := []struct {
			config string
			byID   bool
		}{
			{`{"rootfsImageName": "image"}`, false},
			{`{"rootfsImageRef": "8a788232037eaf17794408ff3df6b922a1aedf9ef8de36afdae3ed0b0381907b"}`, true},
		}
		for _, image := range images {
			It(fmt.Sprintf("should succeed (%s)", image.config), func() {
				if unshare.IsRootless() {
					Skip("should run as root")
				}

				// Given
				addContainerAndSandbox()
				testContainer.SetStateAndSpoofPid(&oci.ContainerState{
					State: specs.State{Status: oci.ContainerStateRunning},
				})

				err := os.WriteFile(
					"spec.dump",
					[]byte(`{"annotations":{"io.kubernetes.cri-o.Metadata"`+
						`:"{\"name\":\"container-to-restore\"}",`+
						`"io.kubernetes.cri-o.Annotations": "{\"name\":\"NAME\",`+
						`\"io.kubernetes.container.hash\":\"b4eeb97f\",`+
						`\"io.kubernetes.pod.uid\":\"old-sandbox-uid\"}",`+
						`"io.kubernetes.cri-o.Labels": "{\"io.kubernetes.container.name\":\"counter\",`+
						`\"io.kubernetes.pod.uid\":\"old-sandbox-uid\"}",`+
						`"io.kubernetes.cri-o.SandboxID": "sandboxID"},`+
						`"mounts": [{"destination": "/proc"},`+
						`{"destination":"/data","source":"/data","options":`+
						`["rw","ro","rbind","rprivate","rshared","rslaved"]}],`+
						`"linux": {"maskedPaths": ["/proc/acpi"], "readonlyPaths": ["/proc/asound"]}}`),
					0o644,
				)
				Expect(err).ToNot(HaveOccurred())
				defer os.RemoveAll("spec.dump")
				err = os.WriteFile("config.dump", []byte(image.config), 0o644)
				Expect(err).ToNot(HaveOccurred())
				defer os.RemoveAll("config.dump")
				outFile, err := os.Create("archive.tar")
				Expect(err).ToNot(HaveOccurred())
				defer outFile.Close()
				input, err := archive.TarWithOptions(".", &archive.TarOptions{
					Compression:      archive.Uncompressed,
					IncludeSourceDir: true,
					IncludeFiles:     []string{"spec.dump", "config.dump"},
				})
				Expect(err).ToNot(HaveOccurred())
				defer os.RemoveAll("archive.tar")
				_, err = io.Copy(outFile, input)
				Expect(err).ToNot(HaveOccurred())
				containerConfig := &types.ContainerConfig{
					Image: &types.ImageSpec{
						Image: "archive.tar",
					},
					Linux: &types.LinuxContainerConfig{
						Resources:       &types.LinuxContainerResources{},
						SecurityContext: &types.LinuxContainerSecurityContext{},
					},
					Labels: map[string]string{
						kubetypes.KubernetesContainerNameLabel: "NEW-NAME",
					},
					Annotations: map[string]string{
						kubetypes.KubernetesPodUIDLabel: "new-sandbox-uid",
						"io.kubernetes.container.hash":  "new-hash",
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
				imageID, err := storage.ParseStorageImageIDFromOutOfProcessData("8a788232037eaf17794408ff3df6b922a1aedf9ef8de36afdae3ed0b0381907b")
				Expect(err).ToNot(HaveOccurred())
				var imageLookup mockutils.MockSequence
				if image.byID {
					imageLookup = mockutils.InOrder(
						imageServerMock.EXPECT().HeuristicallyTryResolvingStringAsIDPrefix(imageID.IDStringForOutOfProcessConsumptionOnly()).
							Return(&imageID),

						imageServerMock.EXPECT().ImageStatusByID(
							gomock.Any(), imageID).
							Return(&storage.ImageResult{
								ID:   imageID,
								User: "10", Size: &size,
								Annotations: map[string]string{
									crioann.CheckpointAnnotationName: "foo",
								},
							}, nil),
					)
				} else {
					checkpointImageName, err := references.ParseRegistryImageReferenceFromOutOfProcessData("docker.io/library/image:latest")
					Expect(err).ToNot(HaveOccurred())
					imageLookup = mockutils.InOrder(
						imageServerMock.EXPECT().HeuristicallyTryResolvingStringAsIDPrefix("image").
							Return(nil),
						imageServerMock.EXPECT().CandidatesForPotentiallyShortImageName(
							gomock.Any(), "image").
							Return([]storage.RegistryImageReference{checkpointImageName}, nil),
						imageServerMock.EXPECT().ImageStatusByName(
							gomock.Any(), checkpointImageName).
							Return(&storage.ImageResult{
								ID:   imageID,
								User: "10", Size: &size,
								Annotations: map[string]string{
									crioann.CheckpointAnnotationName: "foo",
								},
							}, nil),
					)
				}
				mockutils.InOrder(
					imageLookup,

					runtimeServerMock.EXPECT().CreateContainer(gomock.Any(), gomock.Any(),
						gomock.Any(), gomock.Any(), imageID, gomock.Any(),
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
					storeMock.EXPECT().GraphRoot().Return(""),
				)

				// When

				_, err = sut.CRImportCheckpoint(
					context.Background(),
					containerConfig,
					testSandbox,
					"new-sandbox-id",
				)

				// Then
				Expect(err).ToNot(HaveOccurred())
			})
		}
	})
	t.Describe("ContainerRestore from OCI archive", func() {
		It("should fail because archive does not exist", func() {
			// Given
			addContainerAndSandbox()
			size := uint64(100)
			checkpointImageName, err := references.ParseRegistryImageReferenceFromOutOfProcessData("localhost/checkpoint-image:tag1")
			Expect(err).ToNot(HaveOccurred())
			imageID, err := storage.ParseStorageImageIDFromOutOfProcessData("8a788232037eaf17794408ff3df6b922a1aedf9ef8de36afdae3ed0b0381907b")
			Expect(err).ToNot(HaveOccurred())
			gomock.InOrder(
				imageServerMock.EXPECT().HeuristicallyTryResolvingStringAsIDPrefix("localhost/checkpoint-image:tag1").
					Return(nil),
				imageServerMock.EXPECT().CandidatesForPotentiallyShortImageName(
					gomock.Any(), "localhost/checkpoint-image:tag1").
					Return([]storage.RegistryImageReference{checkpointImageName}, nil),
				imageServerMock.EXPECT().ImageStatusByName(
					gomock.Any(), checkpointImageName).
					Return(&storage.ImageResult{
						ID:   imageID,
						User: "10", Size: &size,
						Annotations: map[string]string{
							crioann.CheckpointAnnotationName: "foo",
						},
					}, nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().MountImage(imageID.IDStringForOutOfProcessConsumptionOnly(), gomock.Any(), gomock.Any()).
					Return("", nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().UnmountImage(imageID.IDStringForOutOfProcessConsumptionOnly(), true).
					Return(false, nil),
			)
			containerConfig := &types.ContainerConfig{
				Metadata: &types.ContainerMetadata{Name: "name"},
				Image: &types.ImageSpec{
					Image: "localhost/checkpoint-image:tag1",
				},
			}
			// When
			_, err = sut.CRImportCheckpoint(
				context.Background(),
				containerConfig,
				testSandbox,
				"",
			)

			// Then
			Expect(err.Error()).To(ContainSubstring(`failed to read "spec.dump": open spec.dump: no such file or directory`))
		})
	})
})
