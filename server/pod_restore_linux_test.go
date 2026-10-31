//go:build linux && test

package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/oci"
	containermock "github.com/cri-o/cri-o/test/mocks/containerstorage"
)

func TestPodRestoreCompatibility(t *testing.T) {
	checkpoint := &types.ContainerConfig{
		Metadata: &types.ContainerMetadata{
			Name: "app",
		},
		Image:      &types.ImageSpec{Image: "example/app:tag"},
		Command:    []string{"/app"},
		Args:       []string{"source"},
		WorkingDir: "/work",
		Envs:       []*types.KeyValue{{Key: "POD_NAME", Value: []byte("source")}},
		Linux: &types.LinuxContainerConfig{
			SecurityContext: &types.LinuxContainerSecurityContext{
				RunAsUser: &types.Int64Value{Value: 1000},
			},
		},
	}

	for _, test := range []struct {
		name       string
		change     func(*types.ContainerConfig)
		compatible bool
	}{
		{"pinned image", func(c *types.ContainerConfig) { c.Image.Image = "example/app@sha256:" + strings.Repeat("a", 64) }, true},
		{"destination command", func(c *types.ContainerConfig) { c.Command = []string{"/new"} }, true},
		{"destination args", func(c *types.ContainerConfig) { c.Args = []string{"destination"} }, true},
		{"Downward API", func(c *types.ContainerConfig) { c.Envs[0].Value = []byte("destination") }, true},
		{"working directory", func(c *types.ContainerConfig) { c.WorkingDir = "/other" }, false},
		{"terminal", func(c *types.ContainerConfig) { c.Tty = true }, false},
		{"credentials", func(c *types.ContainerConfig) { c.Linux.SecurityContext.RunAsUser.Value = 0 }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			restore := proto.CloneOf(checkpoint)
			test.change(restore)

			err := validateRestoreContainerConfig(checkpoint, restore)
			if test.compatible {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}

	source := &types.PodSandboxConfig{
		Hostname: "source",
		Linux: &types.LinuxPodSandboxConfig{
			Sysctls: map[string]string{"net.ipv4.ip_forward": "0"},
		},
	}
	destination := proto.CloneOf(source)
	destination.Hostname = "destination"
	require.NoError(t, validateRestoreSandboxConfig(source, destination))
	destination.Linux.Sysctls["net.ipv4.ip_forward"] = "1"
	require.Error(t, validateRestoreSandboxConfig(source, destination))
}

func TestPodCheckpointFiles(t *testing.T) {
	directory := t.TempDir()
	root, err := openCheckpointDirectory(directory)
	require.NoError(t, err)
	t.Cleanup(func() { root.Close() })
	require.NoError(
		t,
		os.WriteFile(filepath.Join(directory, "metadata"), []byte(`{"version":1}`), 0o600),
	)
	require.NoError(t, os.Symlink("metadata", filepath.Join(directory, "link")))

	_, err = openCheckpointFileAt(root, "link")
	require.Error(t, err)
	_, err = openCheckpointFileAt(root, "../metadata")
	require.Error(t, err)
	require.NoError(t, os.Symlink(directory, filepath.Join(directory, "archives")))

	_, err = openCheckpointDirectoryAt(root, "archives")
	require.Error(t, err)
	require.NoError(t, unix.Mkfifo(filepath.Join(directory, "fifo"), 0o600))

	done := make(chan error, 1)

	go func() {
		file, err := openCheckpointFileAt(root, "fifo")
		if file != nil {
			file.Close()
		}

		done <- err
	}()

	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(time.Second):
		fd, err := unix.Open(filepath.Join(directory, "fifo"), unix.O_RDWR|unix.O_NONBLOCK, 0)
		require.NoError(t, err)
		unix.Close(fd)
		t.Fatal("opening a FIFO blocked before validation")
	}

	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(directory, "large"),
			bytes.Repeat([]byte(" "), checkpointMetadataLimit+1),
			0o600,
		),
	)

	var value any
	require.ErrorContains(t, readCheckpointJSONAt(root, "large", &value), "exceeds")
	require.NoError(t, os.WriteFile(filepath.Join(directory, "trailing"), []byte(`{} {}`), 0o600))
	require.ErrorContains(t, readCheckpointJSONAt(root, "trailing", &value), "multiple JSON")
}

func TestPodRestoreStagesValidatedDescriptor(t *testing.T) {
	config := &metadata.ContainerConfig{
		ID:             "source",
		OCIRuntime:     "runc",
		RootfsImageRef: strings.Repeat("a", 64),
	}
	file := writePodArchive(t, config, false, false)
	path := file.Name()
	original, err := os.ReadFile(path)
	require.NoError(t, err)
	digest, err := checkpointFileSHA256(file)
	require.NoError(t, err)

	checkpoint := &podCheckpointManifestContainer{
		ID:      config.ID,
		ImageID: config.RootfsImageRef,
		SHA256:  digest,
	}

	require.NoError(t, os.Rename(path, path+"-original"))
	require.NoError(t, os.WriteFile(path, []byte("replacement"), 0o600))
	destination := filepath.Join(t.TempDir(), "staged")
	require.NoError(t, copyCheckpointArchive(file, destination, checkpoint, "runc"))
	data, err := os.ReadFile(destination)
	require.NoError(t, err)
	assert.Equal(t, original, data)

	checkpoint.ImageID = strings.Repeat("b", 64)
	require.ErrorContains(
		t,
		copyCheckpointArchive(file, destination+"-identity", checkpoint, "runc"),
		"base image",
	)
	_, err = os.Stat(destination + "-identity")
	require.ErrorIs(t, err, os.ErrNotExist)

	checkpoint.ImageID = config.RootfsImageRef

	require.NoError(t, os.WriteFile(path+"-original", []byte("modified"), 0o600))
	require.ErrorContains(
		t,
		copyCheckpointArchive(file, destination+"-bad", checkpoint, "runc"),
		"changed",
	)
	_, err = os.Stat(destination + "-bad")
	require.ErrorIs(t, err, os.ErrNotExist)
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(destination), ".pod-restore-*"))
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func writePodArchive(
	t *testing.T,
	config *metadata.ContainerConfig,
	duplicate, compressed bool,
) *os.File {
	t.Helper()

	var (
		data bytes.Buffer
		gz   *gzip.Writer
	)
	if compressed {
		gz = gzip.NewWriter(&data)
	}

	var tw *tar.Writer
	if gz != nil {
		tw = tar.NewWriter(gz)
	} else {
		tw = tar.NewWriter(&data)
	}

	encoded, err := json.Marshal(config)
	require.NoError(t, err)

	count := 1
	if duplicate {
		count = 2
	}

	for range count {
		require.NoError(
			t,
			tw.WriteHeader(
				&tar.Header{
					Name:     metadata.ConfigDumpFile,
					Typeflag: tar.TypeReg,
					Mode:     0o600,
					Size:     int64(len(encoded)),
				},
			),
		)
		_, err = tw.Write(encoded)
		require.NoError(t, err)
	}

	require.NoError(t, tw.Close())

	if gz != nil {
		require.NoError(t, gz.Close())
	}

	path := filepath.Join(t.TempDir(), "checkpoint.tar")
	require.NoError(t, os.WriteFile(path, data.Bytes(), 0o600))
	file, err := openCheckpointFile(path)
	require.NoError(t, err)
	t.Cleanup(func() { file.Close() })

	return file
}

func TestPodRestoreArchiveIdentity(t *testing.T) {
	config := &metadata.ContainerConfig{
		ID:             "source",
		OCIRuntime:     "runc",
		RootfsImageRef: strings.Repeat("a", 64),
	}
	for _, compressed := range []bool{false, true} {
		file := writePodArchive(t, config, false, compressed)
		entry := podCheckpointManifestContainer{ID: config.ID}
		id, err := checkpointArchiveImageID(file, &entry, "runc")
		require.NoError(t, err)
		assert.Equal(
			t,
			config.RootfsImageRef,
			id,
			"older manifests derive identity from the verified archive",
		)
		entry.ImageID = config.RootfsImageRef
		_, err = checkpointArchiveImageID(file, &entry, "runc")
		require.NoError(t, err)

		entry.ImageID = strings.Repeat("b", 64)
		_, err = checkpointArchiveImageID(file, &entry, "runc")
		require.ErrorContains(t, err, "base image")

		entry.ImageID = ""
		entry.ID = "other"
		_, err = checkpointArchiveImageID(file, &entry, "runc")
		require.ErrorContains(t, err, "identity")

		entry.ID = config.ID
		_, err = checkpointArchiveImageID(file, &entry, "crun")
		require.ErrorContains(t, err, "identity")
	}

	_, err := checkpointArchiveImageID(
		writePodArchive(t, config, true, false),
		&podCheckpointManifestContainer{ID: config.ID},
		"runc",
	)
	require.ErrorContains(t, err, "duplicate")
}

func podRestoreTestServer(t *testing.T) *Server {
	t.Helper()
	store := containermock.NewMockStore(gomock.NewController(t))
	store.EXPECT().GraphRoot().Return(t.TempDir()).AnyTimes()

	return &Server{ContainerServer: lib.NewContainerServerForTest(store)}
}

func TestPodRestoreCommitAndRecovery(t *testing.T) {
	s := podRestoreTestServer(t)
	config := &types.PodSandboxConfig{
		Metadata: &types.PodSandboxMetadata{Name: "pod", Namespace: "ns", Uid: "uid"},
	}
	transaction := &podRestoreTransaction{
		Version:     1,
		Key:         podRestoreKey(config.GetMetadata()),
		SandboxName: "sandbox-name",
		SandboxID:   "sandbox-id",
		Fingerprint: "request",
	}
	require.NoError(t, s.writePodRestoreTransaction(transaction))
	container, err := oci.NewContainer(
		"app-id",
		"app",
		"",
		"",
		nil,
		nil,
		nil,
		"",
		nil,
		nil,
		"",
		&types.ContainerMetadata{Name: "app"},
		transaction.SandboxID,
		false,
		false,
		false,
		"",
		t.TempDir(),
		time.Now(),
		"",
	)
	require.NoError(t, err)
	container.PreparePodRestore(transaction.Key)
	require.Equal(t, codes.FailedPrecondition, status.Code(s.requirePodRestoreCommitted(container)))

	transaction.Response = &types.RestorePodResponse{
		PodSandboxId:       transaction.SandboxID,
		RestoredContainers: []*types.RestoredContainer{{Name: "app", ContainerId: "app-id"}},
	}
	require.NoError(t, s.writePodRestoreTransaction(transaction))
	s.podRestoresInProgress.Store(transaction.Key, struct{}{})
	require.Equal(t, codes.FailedPrecondition, status.Code(s.requirePodRestoreCommitted(container)))
	s.podRestoresInProgress.Delete(transaction.Key)

	require.NoError(t, s.requirePodRestoreCommitted(container))
	require.NoError(
		t,
		s.recoverPodRestores(context.Background()),
		"completed restores must survive startup",
	)
	loaded, err := s.readPodRestoreTransaction(transaction.Key)
	require.NoError(t, err)
	assert.True(t, proto.Equal(transaction.Response, loaded.Response))
	transaction.Response.RestoredContainers[0].ContainerId = "other"
	require.NoError(t, s.writePodRestoreTransaction(transaction))
	require.Equal(t, codes.FailedPrecondition, status.Code(s.requirePodRestoreCommitted(container)))
}

func TestPodRestoreFingerprint(t *testing.T) {
	request := &types.RestorePodRequest{
		CheckpointPath: "/checkpoint",
		Config: &types.PodSandboxConfig{
			Metadata: &types.PodSandboxMetadata{Name: "destination"},
		},
		ContainerConfigs: []*types.ContainerConfig{
			{Metadata: &types.ContainerMetadata{Name: "app"}},
			{Metadata: &types.ContainerMetadata{Name: "sidecar"}},
		},
	}
	manifest := &podCheckpointManifest{
		Containers: []podCheckpointManifestContainer{{ID: "source", SHA256: "original"}},
	}
	sandboxConfig := &types.PodSandboxConfig{Hostname: "source"}
	original, err := podRestoreFingerprint(request, manifest, sandboxConfig)
	require.NoError(t, err)

	reordered := proto.CloneOf(request)
	reordered.ContainerConfigs[0], reordered.ContainerConfigs[1] = reordered.GetContainerConfigs()[1], reordered.GetContainerConfigs()[0]
	fingerprint, err := podRestoreFingerprint(reordered, manifest, sandboxConfig)
	require.NoError(t, err)
	assert.Equal(t, original, fingerprint)

	manifest.Containers[0].SHA256 = "different-checkpoint"
	fingerprint, err = podRestoreFingerprint(request, manifest, sandboxConfig)
	require.NoError(t, err)
	assert.NotEqual(t, original, fingerprint)

	manifest.Containers[0].SHA256 = "original"
	reordered.ContainerConfigs[0].Command = []string{"different"}
	fingerprint, err = podRestoreFingerprint(reordered, manifest, sandboxConfig)
	require.NoError(t, err)
	assert.NotEqual(t, original, fingerprint)
}

func TestPodCheckpointInterruptedOutput(t *testing.T) {
	for _, test := range []struct {
		name     string
		complete bool
		replaced bool
		reserved bool
	}{
		{name: "partial"},
		{name: "concurrent checkpoint", reserved: true},
		{name: "published", complete: true},
		{name: "replaced directory", replaced: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := podRestoreTestServer(t)
			output := filepath.Join(t.TempDir(), "output")
			require.NoError(t, os.Mkdir(output, 0o700))

			var stat unix.Stat_t
			require.NoError(t, unix.Stat(output, &stat))
			marker := podCheckpointRecoveryMarker{
				Version: podCheckpointRecoveryVersion, SandboxID: "source",
				TasksResumed: true, OutputPath: output,
				OutputDevice: stat.Dev, OutputInode: stat.Ino,
				ContainerIDs: []string{"app"},
			}
			require.NoError(t, s.writePodCheckpointRecoveryMarker(marker))

			if test.replaced {
				require.NoError(t, os.Rename(output, output+"-original"))
				require.NoError(t, os.Mkdir(output, 0o700))
			}

			require.NoError(t, os.Mkdir(filepath.Join(output, podCheckpointContainerDir), 0o700))
			require.NoError(
				t,
				os.WriteFile(
					filepath.Join(output, podCheckpointContainerDir, "partial.tar"),
					[]byte("partial"),
					0o600,
				),
			)
			require.NoError(
				t,
				os.WriteFile(filepath.Join(output, podCheckpointConfigFile), []byte("{}"), 0o600),
			)

			if test.complete {
				require.NoError(
					t,
					writeCheckpointJSON(output, podCheckpointManifestFile, &podCheckpointManifest{
						Version:   podCheckpointFormatVersion,
						Runtime:   "cri-o",
						SandboxID: marker.SandboxID,
					}),
				)
			}

			if test.reserved {
				release, err := s.reserveContainerCheckpoints(marker.ContainerIDs)
				require.NoError(t, err)
				err = s.recoverPodCheckpointForSandbox(context.Background(), marker.SandboxID)
				require.Equal(t, codes.Aborted, status.Code(err))
				release()
			}

			err := s.recoverPodCheckpointForSandbox(context.Background(), marker.SandboxID)
			if test.replaced {
				require.ErrorContains(t, err, "replaced")
				_, err = os.Stat(s.podCheckpointRecoveryMarkerPath(marker.SandboxID))
				require.NoError(t, err)
			} else {
				require.NoError(t, err)
				_, err = os.Stat(s.podCheckpointRecoveryMarkerPath(marker.SandboxID))
				require.ErrorIs(t, err, os.ErrNotExist)
			}

			entries, err := os.ReadDir(output)
			require.NoError(t, err)

			if test.complete || test.replaced {
				assert.NotEmpty(t, entries)
			} else {
				assert.Empty(t, entries)
			}
		})
	}
}
