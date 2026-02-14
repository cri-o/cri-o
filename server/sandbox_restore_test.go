package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/internal/storage/references"
)

// setupPodCheckpointImage creates the mock expectations for a pod checkpoint
// image lookup and mount. It writes a pod.options file into mountDir.
// Returns the imageID for use in UnmountImage expectations.
func setupPodCheckpointImage(
	imageName string,
	mountDir string,
	podOpts *metadata.CheckpointedPodOptions,
) storage.StorageImageID {
	size := uint64(100)

	checkpointImageName, err := references.ParseRegistryImageReferenceFromOutOfProcessData(
		"docker.io/library/" + imageName + ":latest")
	Expect(err).ToNot(HaveOccurred())

	imageID, err := storage.ParseStorageImageIDFromOutOfProcessData(
		"8a788232037eaf17794408ff3df6b922a1aedf9ef8de36afdae3ed0b0381907b")
	Expect(err).ToNot(HaveOccurred())

	if podOpts != nil {
		optsJSON, err := json.Marshal(podOpts)
		Expect(err).ToNot(HaveOccurred())
		Expect(os.WriteFile(
			filepath.Join(mountDir, metadata.PodOptionsFile),
			optsJSON, 0o644)).To(Succeed())
	}

	gomock.InOrder(
		imageServerMock.EXPECT().HeuristicallyTryResolvingStringAsIDPrefix(imageName).
			Return(nil),
		imageServerMock.EXPECT().CandidatesForPotentiallyShortImageName(
			gomock.Any(), imageName).
			Return([]storage.RegistryImageReference{checkpointImageName}, nil),
		imageServerMock.EXPECT().ImageStatusByName(
			gomock.Any(), checkpointImageName).
			Return(&storage.ImageResult{
				ID:   imageID,
				User: "10", Size: &size,
				Annotations: map[string]string{
					metadata.CheckpointAnnotationPod:       "my-pod",
					metadata.CheckpointAnnotationPodID:     "old-pod-id",
					metadata.CheckpointAnnotationNamespace: "test-ns",
					metadata.CheckpointAnnotationPodUID:    "test-uid",
				},
			}, nil),
		imageServerMock.EXPECT().GetStore().Return(storeMock),
		storeMock.EXPECT().MountImage(
			imageID.IDStringForOutOfProcessConsumptionOnly(),
			gomock.Any(), gomock.Any()).
			Return(mountDir, nil),
	)

	// The deferred UnmountImage uses the store captured before mount
	storeMock.EXPECT().UnmountImage(
		imageID.IDStringForOutOfProcessConsumptionOnly(), true).
		Return(false, nil)

	return imageID
}

var _ = t.Describe("RestorePod", func() {
	BeforeEach(func() {
		beforeEach()
		createDummyConfig()
		mockRuntimeInLibConfig()
		serverConfig.SetCheckpointRestore(true)
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("RestorePod validation", func() {
		It("should fail with empty path", func() {
			// When
			_, err := sut.RestorePod(
				context.Background(),
				&types.RestorePodRequest{
					Path: "",
				},
			)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("path is required for pod restore"))
		})

		It("should fail when path is an existing path on disk", func() {
			// Given
			tempDir := t.MustTempDir("not-a-checkpoint")

			// When
			_, err := sut.RestorePod(
				context.Background(),
				&types.RestorePodRequest{
					Path: tempDir,
				},
			)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not refer to a pod checkpoint image"))
		})

		It("should fail when image has no pod checkpoint annotation", func() {
			// Given
			size := uint64(100)
			checkpointImageName, err := references.ParseRegistryImageReferenceFromOutOfProcessData(
				"docker.io/library/not-pod-cp:latest")
			Expect(err).ToNot(HaveOccurred())
			imageID, err := storage.ParseStorageImageIDFromOutOfProcessData(
				"8a788232037eaf17794408ff3df6b922a1aedf9ef8de36afdae3ed0b0381907b")
			Expect(err).ToNot(HaveOccurred())

			gomock.InOrder(
				imageServerMock.EXPECT().HeuristicallyTryResolvingStringAsIDPrefix("not-pod-cp").
					Return(nil),
				imageServerMock.EXPECT().CandidatesForPotentiallyShortImageName(
					gomock.Any(), "not-pod-cp").
					Return([]storage.RegistryImageReference{checkpointImageName}, nil),
				imageServerMock.EXPECT().ImageStatusByName(
					gomock.Any(), checkpointImageName).
					Return(&storage.ImageResult{
						ID:          imageID,
						User:        "10",
						Size:        &size,
						Annotations: map[string]string{},
					}, nil),
			)

			// When
			_, err = sut.RestorePod(
				context.Background(),
				&types.RestorePodRequest{
					Path: "not-pod-cp",
				},
			)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not refer to a pod checkpoint image"))
		})
	})

	t.Describe("RestorePod mount and pod.options", func() {
		It("should fail when checkpoint image mount fails", func() {
			// Given
			size := uint64(100)
			checkpointImageName, err := references.ParseRegistryImageReferenceFromOutOfProcessData(
				"docker.io/library/pod-cp-mount-fail:latest")
			Expect(err).ToNot(HaveOccurred())
			imageID, err := storage.ParseStorageImageIDFromOutOfProcessData(
				"8a788232037eaf17794408ff3df6b922a1aedf9ef8de36afdae3ed0b0381907b")
			Expect(err).ToNot(HaveOccurred())

			gomock.InOrder(
				imageServerMock.EXPECT().HeuristicallyTryResolvingStringAsIDPrefix("pod-cp-mount-fail").
					Return(nil),
				imageServerMock.EXPECT().CandidatesForPotentiallyShortImageName(
					gomock.Any(), "pod-cp-mount-fail").
					Return([]storage.RegistryImageReference{checkpointImageName}, nil),
				imageServerMock.EXPECT().ImageStatusByName(
					gomock.Any(), checkpointImageName).
					Return(&storage.ImageResult{
						ID:   imageID,
						User: "10", Size: &size,
						Annotations: map[string]string{
							metadata.CheckpointAnnotationPod:       "my-pod",
							metadata.CheckpointAnnotationPodID:     "old-pod-id",
							metadata.CheckpointAnnotationNamespace: "test-ns",
							metadata.CheckpointAnnotationPodUID:    "test-uid",
						},
					}, nil),
				imageServerMock.EXPECT().GetStore().Return(storeMock),
				storeMock.EXPECT().MountImage(
					imageID.IDStringForOutOfProcessConsumptionOnly(),
					gomock.Any(), gomock.Any()).
					Return("", errors.New("mount error")),
			)

			// When
			_, err = sut.RestorePod(
				context.Background(),
				&types.RestorePodRequest{
					Path: "pod-cp-mount-fail",
				},
			)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to mount checkpoint image"))
		})

		It("should fail when pod.options file is missing", func() {
			// Given
			mountDir := t.MustTempDir("empty-mount")
			setupPodCheckpointImage("pod-cp-no-opts", mountDir, nil)

			// When
			_, err := sut.RestorePod(
				context.Background(),
				&types.RestorePodRequest{
					Path: "pod-cp-no-opts",
				},
			)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read pod options"))
		})

		It("should fail when pod checkpoint version is unsupported", func() {
			// Given
			mountDir := t.MustTempDir("bad-version-mount")
			setupPodCheckpointImage("pod-cp-bad-ver", mountDir, &metadata.CheckpointedPodOptions{
				Version:    99,
				Containers: map[string]string{"ctr1": "ctr1-dir"},
			})

			// When
			_, err := sut.RestorePod(
				context.Background(),
				&types.RestorePodRequest{
					Path: "pod-cp-bad-ver",
				},
			)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported pod checkpoint version 99"))
		})

		It("should fail when pod checkpoint contains no containers", func() {
			// Given
			mountDir := t.MustTempDir("empty-containers-mount")
			setupPodCheckpointImage("pod-cp-empty", mountDir, &metadata.CheckpointedPodOptions{
				Version:    1,
				Containers: map[string]string{},
			})

			// When
			_, err := sut.RestorePod(
				context.Background(),
				&types.RestorePodRequest{
					Path: "pod-cp-empty",
				},
			)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("pod checkpoint contains no containers"))
		})
	})

	t.Describe("RestorePod sandbox creation", func() {
		It("should fail to create pod sandbox with provided PodSandboxConfig", func() {
			// Given
			mountDir := t.MustTempDir("provided-config-mount")

			setupPodCheckpointImage("pod-cp-with-config", mountDir, &metadata.CheckpointedPodOptions{
				Version:    1,
				Containers: map[string]string{"ctr1": "ctr1-id-ctr1"},
			})

			// Mock CreatePodSandbox to return an error
			runtimeServerMock.EXPECT().CreatePodSandbox(gomock.Any(),
				gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(storage.ContainerInfo{}, errors.New("mock sandbox creation error"))

			// When — use a provided PodSandboxConfig (covers lines 78-80)
			_, err := sut.RestorePod(
				context.Background(),
				&types.RestorePodRequest{
					Path: "pod-cp-with-config",
					Config: &types.PodSandboxConfig{
						Metadata: &types.PodSandboxMetadata{
							Name:      "restored-pod",
							Namespace: "restored-ns",
							Uid:       "restored-uid",
						},
						Labels:      map[string]string{"app": "test"},
						Annotations: map[string]string{"note": "restored"},
						Linux: &types.LinuxPodSandboxConfig{
							SecurityContext: &types.LinuxSandboxSecurityContext{
								NamespaceOptions: &types.NamespaceOption{},
							},
						},
					},
				},
			)

			// Then — fails at RunPodSandbox, but we exercised the
			// provided-config path (lines 78-80) and RunPodSandbox call (lines 99-106)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create pod sandbox"))
		})

		It("should fail to create pod sandbox with default constructed config", func() {
			// Given
			mountDir := t.MustTempDir("default-config-mount")

			setupPodCheckpointImage("pod-cp-default-cfg", mountDir, &metadata.CheckpointedPodOptions{
				Version:    1,
				Containers: map[string]string{"ctr1": "ctr1-id-ctr1"},
			})

			// Mock CreatePodSandbox to return an error
			runtimeServerMock.EXPECT().CreatePodSandbox(gomock.Any(),
				gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(storage.ContainerInfo{}, errors.New("mock sandbox creation error"))

			// When — no Config provided, exercises default config construction
			// (lines 82-94) using checkpoint metadata
			_, err := sut.RestorePod(
				context.Background(),
				&types.RestorePodRequest{
					Path: "pod-cp-default-cfg",
				},
			)

			// Then — fails at RunPodSandbox
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create pod sandbox"))
		})
	})
})

var _ = t.Describe("RestorePod with CheckpointRestore set to false", func() {
	BeforeEach(func() {
		beforeEach()
		createDummyConfig()
		mockRuntimeInLibConfig()
		serverConfig.SetCheckpointRestore(false)
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("RestorePod", func() {
		It("should fail when checkpoint/restore disabled", func() {
			// When
			_, err := sut.RestorePod(
				context.Background(),
				&types.RestorePodRequest{
					Path: "some-checkpoint-image",
				},
			)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("checkpoint/restore support not available"))
		})
	})
})
