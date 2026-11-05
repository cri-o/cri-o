//go:build linux && test

package server

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	cstorage "go.podman.io/storage"
	"go.uber.org/mock/gomock"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	libconfig "github.com/cri-o/cri-o/pkg/config"
	containermock "github.com/cri-o/cri-o/test/mocks/containerstorage"
)

// podCheckpointOutput creates an output directory and a recovery marker that
// identifies it, as checkpointPod does before it pauses the Pod.
func podCheckpointOutput(t *testing.T, sandboxID string) (string, *podCheckpointRecoveryMarker) {
	t.Helper()

	output := filepath.Join(t.TempDir(), "output")
	require.NoError(t, os.Mkdir(output, 0o700))

	marker := &podCheckpointRecoveryMarker{
		Version:      podCheckpointRecoveryVersion,
		SandboxID:    sandboxID,
		ContainerIDs: []string{"app"},
	}
	require.NoError(t, marker.setOutputDirectory(output))

	return output, marker
}

func writePartialPodCheckpoint(t *testing.T, output string) {
	t.Helper()

	require.NoError(t, os.Mkdir(filepath.Join(output, podCheckpointContainerDir), 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(output, podCheckpointContainerDir, "partial.tar"),
		[]byte("partial"),
		0o600,
	))
	require.NoError(t, os.WriteFile(filepath.Join(output, podCheckpointConfigFile), nil, 0o600))
}

func TestPodCheckpointRequestPrerequisites(t *testing.T) {
	s := podRestoreTestServer(t)

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	s.config.ContainerLevelEnabled = libconfig.ContainerCheckpointRestoreLevelNone
	_, err := s.CheckpointPod(ctx, &types.CheckpointPodRequest{})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
	_, err = s.RestorePod(ctx, &types.RestorePodRequest{})
	assert.Equal(t, codes.Unimplemented, status.Code(err))

	s.config.ContainerLevelEnabled = libconfig.ContainerCheckpointRestoreLevelCheckpointRestore
	_, err = s.CheckpointPod(ctx, nil)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = s.RestorePod(ctx, nil)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	_, err = s.CheckpointPod(t.Context(), &types.CheckpointPodRequest{})
	require.ErrorContains(t, err, "finite deadline")
	_, err = s.RestorePod(t.Context(), &types.RestorePodRequest{})
	require.ErrorContains(t, err, "finite deadline")
}

func TestPodCheckpointReservation(t *testing.T) {
	s := podRestoreTestServer(t)
	output := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(output, link))

	_, err := s.reservePodCheckpoint("sandbox", filepath.Join(output, "missing"), nil)
	require.ErrorContains(t, err, "resolve checkpoint output path")

	release, err := s.reservePodCheckpoint("sandbox", output, []string{"app"})
	require.NoError(t, err)

	_, err = s.reservePodCheckpoint("other", link, []string{"other"})
	require.ErrorContains(t, err, "already in use", "a symlink names the same output")

	_, err = s.reservePodCheckpoint("sandbox", t.TempDir(), []string{"other"})
	require.ErrorContains(t, err, "already in progress")

	otherOutput := t.TempDir()
	_, err = s.reservePodCheckpoint("other", otherOutput, []string{"other", "app"})
	require.ErrorContains(t, err, `container "app"`)

	// Failed reservations release everything they acquired.
	releaseOther, err := s.reservePodCheckpoint("other", otherOutput, []string{"other"})
	require.NoError(t, err)
	releaseOther()

	release()

	release, err = s.reservePodCheckpoint("sandbox", link, []string{"app"})
	require.NoError(t, err)
	release()
}

func TestPodCheckpointRecoveryMarker(t *testing.T) {
	s := podRestoreTestServer(t)
	output, marker := podCheckpointOutput(t, "sandbox")

	resolved, err := filepath.EvalSymlinks(output)
	require.NoError(t, err)
	assert.Equal(t, resolved, marker.OutputPath)

	var stat unix.Stat_t
	require.NoError(t, unix.Stat(output, &stat))
	assert.Equal(t, stat.Dev, marker.OutputDevice)
	assert.Equal(t, stat.Ino, marker.OutputInode)

	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(output, link))

	linked := new(podCheckpointRecoveryMarker)
	require.NoError(t, linked.setOutputDirectory(link))
	assert.Equal(t, marker.OutputPath, linked.OutputPath)

	require.Error(
		t,
		new(podCheckpointRecoveryMarker).setOutputDirectory(filepath.Join(output, "x")),
	)

	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	require.Error(t, new(podCheckpointRecoveryMarker).setOutputDirectory(file))

	require.NoError(t, s.writePodCheckpointRecoveryMarker(marker))
	require.ErrorContains(t, s.writePodCheckpointRecoveryMarker(marker), "already exists")

	require.NoError(t, s.removePodCheckpointRecoveryMarker(marker.SandboxID))
	require.NoError(
		t,
		s.removePodCheckpointRecoveryMarker(marker.SandboxID),
		"removing a missing marker succeeds",
	)
	assert.NoFileExists(t, s.podCheckpointRecoveryMarkerPath(marker.SandboxID))
}

func TestRecoverPodCheckpoints(t *testing.T) {
	t.Run("no recovery directory", func(t *testing.T) {
		require.NoError(t, podRestoreTestServer(t).recoverPodCheckpoints(t.Context()))
	})

	t.Run("removed sandbox", func(t *testing.T) {
		s := podRestoreTestServer(t)
		store, ok := s.Store().(*containermock.MockStore)
		require.True(t, ok)
		store.EXPECT().Container(gomock.Any()).Return(nil, cstorage.ErrContainerUnknown).Times(2)

		output, marker := podCheckpointOutput(t, "sandbox")
		require.NoError(t, s.writePodCheckpointRecoveryMarker(marker))
		writePartialPodCheckpoint(t, output)

		dir := s.podCheckpointRecoveryDirectory()
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".temporary"), nil, 0o600))
		require.NoError(t, os.Mkdir(filepath.Join(dir, "directory"), 0o700))

		require.NoError(t, s.recoverPodCheckpoints(t.Context()))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Len(t, entries, 1)
		assert.Equal(t, "directory", entries[0].Name())

		entries, err = os.ReadDir(output)
		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	for _, test := range []struct {
		name    string
		marker  podCheckpointRecoveryMarker
		file    string
		storage func(*containermock.MockStore)
		message string
	}{
		{
			name:    "unsupported version",
			marker:  podCheckpointRecoveryMarker{SandboxID: "sandbox", ContainerIDs: []string{"app"}},
			message: "invalid Pod checkpoint recovery marker",
		},
		{
			name: "no containers",
			marker: podCheckpointRecoveryMarker{
				Version: podCheckpointRecoveryVersion, SandboxID: "sandbox",
			},
			message: "invalid Pod checkpoint recovery marker",
		},
		{
			name: "renamed marker",
			marker: podCheckpointRecoveryMarker{
				Version: podCheckpointRecoveryVersion, SandboxID: "sandbox",
				ContainerIDs: []string{"app"},
			},
			file:    "renamed.json",
			message: "invalid filename",
		},
		{
			name: "storage still has the sandbox",
			marker: podCheckpointRecoveryMarker{
				Version: podCheckpointRecoveryVersion, SandboxID: "sandbox",
				ContainerIDs: []string{"app"},
			},
			storage: func(store *containermock.MockStore) {
				store.EXPECT().Container("sandbox").Return(&cstorage.Container{}, nil)
			},
			message: `"sandbox" has not been loaded`,
		},
		{
			name: "storage error",
			marker: podCheckpointRecoveryMarker{
				Version: podCheckpointRecoveryVersion, SandboxID: "sandbox",
				ContainerIDs: []string{"app"},
			},
			storage: func(store *containermock.MockStore) {
				store.EXPECT().Container("sandbox").Return(nil, errors.New("storage failed"))
			},
			message: "storage failed",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := podRestoreTestServer(t)
			if test.storage != nil {
				store, ok := s.Store().(*containermock.MockStore)
				require.True(t, ok)
				test.storage(store)
			}

			dir := s.podCheckpointRecoveryDirectory()
			require.NoError(t, os.MkdirAll(dir, 0o700))

			name := filepath.Base(s.podCheckpointRecoveryMarkerPath(test.marker.SandboxID))
			if test.file != "" {
				name = test.file
			}

			require.NoError(t, writeCheckpointJSON(dir, name, &test.marker))

			err := s.recoverPodCheckpoints(t.Context())
			require.ErrorContains(t, err, test.message)
			assert.FileExists(t, filepath.Join(dir, name), "a failed recovery keeps the marker")
		})
	}

	t.Run("malformed marker", func(t *testing.T) {
		s := podRestoreTestServer(t)
		dir := s.podCheckpointRecoveryDirectory()
		require.NoError(t, os.MkdirAll(dir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "marker.json"), []byte("{"), 0o600))

		require.ErrorContains(t, s.recoverPodCheckpoints(t.Context()), "decode")
	})
}

func TestRecoverPodCheckpointForSandbox(t *testing.T) {
	t.Run("no marker", func(t *testing.T) {
		s := podRestoreTestServer(t)
		require.NoError(t, s.recoverPodCheckpointForSandbox(t.Context(), "sandbox"))
	})

	t.Run("active checkpoint", func(t *testing.T) {
		s := podRestoreTestServer(t)
		_, marker := podCheckpointOutput(t, "sandbox")
		require.NoError(t, s.writePodCheckpointRecoveryMarker(marker))

		s.podCheckpointsInProgress.Store("sandbox", struct{}{})
		err := s.recoverPodCheckpointForSandbox(t.Context(), "sandbox")
		assert.Equal(t, codes.Aborted, status.Code(err))
		assert.FileExists(t, s.podCheckpointRecoveryMarkerPath("sandbox"))
	})

	t.Run("output in use", func(t *testing.T) {
		s := podRestoreTestServer(t)
		_, marker := podCheckpointOutput(t, "sandbox")
		require.NoError(t, s.writePodCheckpointRecoveryMarker(marker))

		s.podCheckpointOutputs.Store(marker.OutputPath, struct{}{})
		err := s.recoverPodCheckpointForSandbox(t.Context(), "sandbox")
		assert.Equal(t, codes.Aborted, status.Code(err))

		_, loaded := s.podCheckpointsInProgress.Load("sandbox")
		assert.False(t, loaded, "the sandbox reservation is released")
	})

	t.Run("replaced output", func(t *testing.T) {
		s := podRestoreTestServer(t)
		output, marker := podCheckpointOutput(t, "sandbox")
		marker.TasksResumed = true
		require.NoError(t, s.writePodCheckpointRecoveryMarker(marker))

		// Keep the original directory so the replacement has another inode.
		require.NoError(t, os.Rename(output, output+".moved"))
		require.NoError(t, os.Mkdir(output, 0o700))
		writePartialPodCheckpoint(t, output)

		require.NoError(t, s.recoverPodCheckpointForSandbox(t.Context(), "sandbox"))
		assert.NoFileExists(t, s.podCheckpointRecoveryMarkerPath("sandbox"))
		assert.DirExists(t, filepath.Join(output, podCheckpointContainerDir),
			"recovery must not clean a directory it does not own")
	})

	t.Run("marker of another sandbox", func(t *testing.T) {
		s := podRestoreTestServer(t)
		_, marker := podCheckpointOutput(t, "other")
		require.NoError(t, s.writePodCheckpointRecoveryMarker(marker))
		require.NoError(t, os.Rename(
			s.podCheckpointRecoveryMarkerPath("other"),
			s.podCheckpointRecoveryMarkerPath("sandbox"),
		))

		err := s.recoverPodCheckpointForSandbox(t.Context(), "sandbox")
		require.ErrorContains(t, err, "invalid Pod checkpoint recovery marker")
	})

	t.Run("malformed marker", func(t *testing.T) {
		s := podRestoreTestServer(t)
		path := s.podCheckpointRecoveryMarkerPath("sandbox")
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))

		require.ErrorContains(t, s.recoverPodCheckpointForSandbox(t.Context(), "sandbox"), "decode")
	})
}

func TestCleanupInterruptedPodCheckpoint(t *testing.T) {
	t.Run("older marker without output", func(t *testing.T) {
		require.NoError(t, cleanupInterruptedPodCheckpoint(&podCheckpointRecoveryMarker{}))
	})

	t.Run("removed output", func(t *testing.T) {
		output, marker := podCheckpointOutput(t, "sandbox")
		require.NoError(t, os.Remove(output))
		require.NoError(t, cleanupInterruptedPodCheckpoint(marker))
	})

	t.Run("output replaced by a file", func(t *testing.T) {
		output, marker := podCheckpointOutput(t, "sandbox")
		require.NoError(t, os.Remove(output))
		require.NoError(t, os.WriteFile(output, nil, 0o600))
		require.Error(t, cleanupInterruptedPodCheckpoint(marker))
	})

	t.Run("completed checkpoint removes scratch but retains archives", func(t *testing.T) {
		output, marker := podCheckpointOutput(t, "sandbox")
		scratch := filepath.Join(output, podCheckpointScratchDir)
		require.NoError(t, os.Mkdir(scratch, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(scratch, "pages.img"), []byte("bulk scratch"), 0600))
		require.NoError(t, writeCheckpointJSON(output, podCheckpointManifestFile, &podCheckpointManifest{Version: podCheckpointFormatVersion, Runtime: "cri-o", SandboxID: "sandbox"}))
		require.NoError(t, os.WriteFile(filepath.Join(output, "retained-archive"), []byte("archive"), 0600))
		require.NoError(t, cleanupInterruptedPodCheckpoint(marker))
		require.NoDirExists(t, scratch)
		require.FileExists(t, filepath.Join(output, podCheckpointManifestFile))
		require.FileExists(t, filepath.Join(output, "retained-archive"))
	})
	t.Run("temporary metadata", func(t *testing.T) {
		output, marker := podCheckpointOutput(t, "sandbox")
		writePartialPodCheckpoint(t, output)

		for _, name := range []string{
			"." + podCheckpointConfigFile + "-1",
			"." + podCheckpointManifestFile + "-2",
			"unrelated",
		} {
			require.NoError(t, os.WriteFile(filepath.Join(output, name), nil, 0o600))
		}

		require.NoError(t, cleanupInterruptedPodCheckpoint(marker))

		entries, err := os.ReadDir(output)
		require.NoError(t, err)
		require.Len(t, entries, 1)
		assert.Equal(t, "unrelated", entries[0].Name())
	})

	for _, test := range []struct {
		name     string
		manifest any
		message  string
	}{
		{
			name: "manifest of another sandbox",
			manifest: &podCheckpointManifest{
				Version: podCheckpointFormatVersion, Runtime: "cri-o", SandboxID: "other",
			},
			message: "does not match the recovery marker",
		},
		{
			name:     "malformed manifest",
			manifest: map[string]any{"unknown": true},
			message:  "decode",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, marker := podCheckpointOutput(t, "sandbox")
			writePartialPodCheckpoint(t, output)
			require.NoError(
				t,
				writeCheckpointJSON(output, podCheckpointManifestFile, test.manifest),
			)

			require.ErrorContains(t, cleanupInterruptedPodCheckpoint(marker), test.message)
			assert.DirExists(t, filepath.Join(output, podCheckpointContainerDir))
		})
	}
}

func TestPodCheckpointArchiveFiles(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "archive.tar")
	require.NoError(t, os.WriteFile(path, []byte("archive"), 0o600))

	digest := sha256.Sum256([]byte("archive"))
	actual, err := fileSHA256(path)
	require.NoError(t, err)
	assert.Equal(t, hex.EncodeToString(digest[:]), actual)

	_, err = fileSHA256(directory)
	require.ErrorContains(t, err, "not a regular file")
	_, err = fileSHA256(filepath.Join(directory, "missing"))
	require.Error(t, err)

	require.NoError(t, syncCheckpointFile(path))
	require.ErrorContains(t, syncCheckpointFile(filepath.Join(directory, "missing")), "open")
	require.ErrorContains(t, syncCheckpointDirectory(filepath.Join(directory, "missing")), "open")

	name := checkpointArchiveName("container")
	assert.Equal(t, podCheckpointContainerDir, filepath.Dir(name))
	assert.Equal(t, name, checkpointArchiveName("container"))
	assert.NotEqual(t, name, checkpointArchiveName("other"))

	s := podRestoreTestServer(t)
	s.config.DefaultRuntime = "runc"
	assert.Equal(t, "runc", s.resolvedRuntimeHandler(""))
	assert.Equal(t, "crun", s.resolvedRuntimeHandler("crun"))
}

func TestPodCheckpointInputValidation(t *testing.T) {
	directory := t.TempDir()
	file := filepath.Join(directory, "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	require.NoError(t, validateCheckpointInput(directory))
	require.ErrorContains(t, validateCheckpointInput("relative"), "absolute")
	require.ErrorContains(t, validateCheckpointInput(file), "real directory")
	require.ErrorContains(t, validateCheckpointInput(filepath.Join(directory, "x")), "inspect")

	require.ErrorContains(t, validateEmptyCheckpointOutput(file), "real directory")
	require.ErrorContains(
		t,
		validateEmptyCheckpointOutput(filepath.Join(directory, "x")),
		"inspect",
	)

	if os.Geteuid() != 0 {
		unreadable := filepath.Join(directory, "unreadable")
		require.NoError(t, os.Mkdir(unreadable, 0))
		require.ErrorContains(t, validateEmptyCheckpointOutput(unreadable), "read")
	}
}

func TestPodCheckpointMetadata(t *testing.T) {
	directory := t.TempDir()
	config := &types.PodSandboxConfig{
		Metadata: &types.PodSandboxMetadata{Name: "pod", Namespace: "ns", Uid: "uid"},
		Hostname: "pod",
	}

	require.NoError(t, persistPodSandboxConfig(directory, config))
	require.NoError(t, persistPodSandboxConfig(directory, config), "repeating creation succeeds")

	loaded, err := loadPodSandboxConfig(directory)
	require.NoError(t, err)
	assert.Equal(t, config.GetHostname(), loaded.GetHostname())

	changed := &types.PodSandboxConfig{Metadata: config.GetMetadata(), Hostname: "other"}
	require.ErrorContains(t, persistPodSandboxConfig(directory, changed), "does not match")

	container := &types.ContainerConfig{Metadata: &types.ContainerMetadata{Name: "app"}}
	require.NoError(t, persistContainerConfig(directory, container))
	require.NoError(t, persistContainerConfig(directory, container))
	require.ErrorContains(
		t,
		persistContainerConfig(directory, &types.ContainerConfig{
			Metadata: &types.ContainerMetadata{Name: "other"},
		}),
		"does not match",
	)

	broken := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(broken, criPodSandboxConfigFile), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(broken, criContainerConfigFile), nil, 0o600))
	require.Error(t, persistPodSandboxConfig(broken, config))
	require.Error(t, persistContainerConfig(broken, container))

	_, err = loadPodSandboxConfig(t.TempDir())
	require.Error(t, err)

	require.ErrorContains(t, writeCheckpointJSON(directory, "channel", make(chan int)), "marshal")
	require.ErrorContains(
		t,
		writeCheckpointJSON(directory, "large", strings.Repeat("x", checkpointMetadataLimit)),
		"exceeds",
	)
	require.ErrorContains(
		t,
		writeCheckpointJSON(filepath.Join(directory, "missing"), "value", true),
		"create temporary",
	)
	require.ErrorContains(
		t,
		writeCheckpointJSON(directory, filepath.Join("missing", "value"), true),
		"create temporary",
	)

	require.NoError(t, os.Mkdir(filepath.Join(directory, "occupied"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "occupied", "entry"), nil, 0o600))
	require.ErrorContains(t, writeCheckpointJSON(directory, "occupied", true), "publish")

	entries, err := os.ReadDir(directory)
	require.NoError(t, err)

	for _, entry := range entries {
		assert.False(t, strings.HasPrefix(entry.Name(), "."), "temporary file %s", entry.Name())
	}

	var value map[string]any

	require.NoError(t, os.WriteFile(filepath.Join(directory, "invalid"), []byte(`{} x`), 0o600))
	require.ErrorContains(
		t,
		readCheckpointJSON(filepath.Join(directory, "invalid"), &value),
		"decode",
	)

	root, err := openCheckpointDirectory(directory)
	require.NoError(t, err)

	t.Cleanup(func() { root.Close() })

	for _, name := range []string{"", ".", "..", "a/b"} {
		_, err = openCheckpointDirectoryAt(root, name)
		require.ErrorContains(t, err, "invalid checkpoint directory name")
	}

	child, err := openCheckpointDirectoryAt(root, "occupied")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(directory, "occupied"), child.Name())
	child.Close()

	_, err = openCheckpointDirectoryAt(root, "missing")
	require.Error(t, err)
}

func TestValidateCheckpointArchiveEntry(t *testing.T) {
	for _, test := range []struct {
		name     string
		typeflag byte
		valid    bool
	}{
		{name: ".", typeflag: tar.TypeDir, valid: true},
		{name: metadata.CheckpointDirectory, typeflag: tar.TypeDir, valid: true},
		{name: metadata.CheckpointDirectory + "/pages-1.img", typeflag: tar.TypeReg, valid: true},
		{name: metadata.ConfigDumpFile, typeflag: tar.TypeReg, valid: true},
		{name: metadata.RootFsDiffTar, typeflag: tar.TypeReg, valid: true},
		{name: "state.json", typeflag: tar.TypeReg},
		{name: "config.json", typeflag: tar.TypeReg},
		{name: metadata.ConfigDumpFile, typeflag: tar.TypeSymlink},
		{name: metadata.CheckpointDirectory + "/device", typeflag: tar.TypeChar},
		{name: metadata.CheckpointDirectory + "/nested/pages-1.img", typeflag: tar.TypeReg},
	} {
		err := validateCheckpointArchiveEntry(test.name, test.typeflag)
		if test.valid {
			assert.NoError(t, err, test.name)
		} else {
			assert.Error(t, err, test.name)
		}
	}
}
