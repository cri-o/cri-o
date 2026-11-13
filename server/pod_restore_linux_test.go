//go:build linux && test

package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	cstorage "go.podman.io/storage"
	"go.uber.org/mock/gomock"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/memorystore"
	"github.com/cri-o/cri-o/internal/oci"
	pkgstorage "github.com/cri-o/cri-o/internal/storage"
	libconfig "github.com/cri-o/cri-o/pkg/config"
	containermock "github.com/cri-o/cri-o/test/mocks/containerstorage"
	criostoragemock "github.com/cri-o/cri-o/test/mocks/criostorage"
	libmock "github.com/cri-o/cri-o/test/mocks/lib"
	ocicnimock "github.com/cri-o/cri-o/test/mocks/ocicni"
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
	destinationSpec := &rspec.Spec{Process: &rspec.Process{}, Linux: &rspec.Linux{}}
	require.NoError(
		t,
		copyCheckpointArchive(file, destination, checkpoint, "runc", destinationSpec),
	)
	data, err := os.ReadFile(destination)
	require.NoError(t, err)
	assert.Equal(t, original, data)

	checkpoint.ImageID = strings.Repeat("b", 64)
	require.ErrorContains(
		t,
		copyCheckpointArchive(file, destination+"-identity", checkpoint, "runc", destinationSpec),
		"base image",
	)
	_, err = os.Stat(destination + "-identity")
	require.ErrorIs(t, err, os.ErrNotExist)

	checkpoint.ImageID = config.RootfsImageRef

	require.NoError(t, os.WriteFile(path+"-original", []byte("modified"), 0o600))
	require.ErrorContains(
		t,
		copyCheckpointArchive(file, destination+"-bad", checkpoint, "runc", destinationSpec),
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
	specs ...*rspec.Spec,
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

	spec := &rspec.Spec{Process: &rspec.Process{}, Linux: &rspec.Linux{}}
	if len(specs) != 0 {
		spec = specs[0]
	}

	if spec != nil {
		encodedSpec, err := json.Marshal(spec)
		require.NoError(t, err)
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name:     metadata.SpecDumpFile,
			Typeflag: tar.TypeReg,
			Mode:     0o600,
			Size:     int64(len(encodedSpec)),
		}))
		_, err = tw.Write(encodedSpec)
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

func TestPodRestoreResolvedSeccompPolicy(t *testing.T) {
	// A Localhost reference compares equal even when its file has changed.
	config := podRestoreContainerConfig("app")
	config.Linux = &types.LinuxContainerConfig{
		SecurityContext: &types.LinuxContainerSecurityContext{
			Seccomp: &types.SecurityProfile{
				ProfileType:  types.SecurityProfile_Localhost,
				LocalhostRef: "/policy.json",
			},
		},
	}
	require.NoError(t, validateRestoreContainerConfig(config, proto.CloneOf(config)))

	allow := &rspec.LinuxSeccomp{DefaultAction: rspec.ActAllow}

	denyKill := &rspec.LinuxSeccomp{DefaultAction: rspec.ActAllow, Syscalls: []rspec.LinuxSyscall{{
		Names: []string{"kill"}, Action: rspec.ActErrno,
	}}}
	for _, tc := range []struct {
		name        string
		captured    *rspec.Spec
		destination *rspec.Spec
		allowed     bool
	}{
		{name: "same resolved filter", captured: &rspec.Spec{Process: &rspec.Process{}, Linux: &rspec.Linux{Seccomp: denyKill}}, destination: &rspec.Spec{Process: &rspec.Process{}, Linux: &rspec.Linux{Seccomp: denyKill}}, allowed: true},
		{name: "same unconfined policy", captured: &rspec.Spec{Process: &rspec.Process{}, Linux: &rspec.Linux{}}, destination: &rspec.Spec{Process: &rspec.Process{}, Linux: &rspec.Linux{}}, allowed: true},
		{name: "tightened policy", captured: &rspec.Spec{Process: &rspec.Process{}, Linux: &rspec.Linux{Seccomp: allow}}, destination: &rspec.Spec{Process: &rspec.Process{}, Linux: &rspec.Linux{Seccomp: denyKill}}},
		{name: "removed policy", captured: &rspec.Spec{Process: &rspec.Process{}, Linux: &rspec.Linux{Seccomp: denyKill}}, destination: &rspec.Spec{Process: &rspec.Process{}, Linux: &rspec.Linux{}}},
		{name: "new policy", captured: &rspec.Spec{Process: &rspec.Process{}, Linux: &rspec.Linux{}}, destination: &rspec.Spec{Process: &rspec.Process{}, Linux: &rspec.Linux{Seccomp: denyKill}}},
		{name: "missing captured spec", destination: &rspec.Spec{Process: &rspec.Process{}, Linux: &rspec.Linux{}}},
		{name: "missing captured Linux", captured: &rspec.Spec{}, destination: &rspec.Spec{Process: &rspec.Process{}, Linux: &rspec.Linux{}}},
		{name: "missing destination Linux", captured: &rspec.Spec{Process: &rspec.Process{}, Linux: &rspec.Linux{}}, destination: &rspec.Spec{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			archive := writePodArchive(
				t,
				&metadata.ContainerConfig{
					ID:             "source",
					OCIRuntime:     "runc",
					RootfsImageRef: strings.Repeat("a", 64),
				},
				false,
				false,
				tc.captured,
			)

			_, err := checkpointArchiveImageID(
				archive,
				&podCheckpointManifestContainer{ID: "source"},
				"runc",
				tc.destination,
			)
			if tc.allowed {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}

	captured := &rspec.Spec{Process: &rspec.Process{}, Linux: &rspec.Linux{Seccomp: allow}}
	archive := writePodArchive(
		t,
		&metadata.ContainerConfig{
			ID:             "source",
			OCIRuntime:     "runc",
			RootfsImageRef: strings.Repeat("a", 64),
		},
		false,
		false,
		captured,
	)
	require.Error(t, validateCheckpointSecurity(captured, nil))
	require.Error(
		t,
		copyCheckpointArchive(
			archive,
			filepath.Join(t.TempDir(), "missing-policy"),
			&podCheckpointManifestContainer{},
			"runc",
			nil,
		),
	)
	digest, err := checkpointFileSHA256(archive)
	require.NoError(t, err)
	directory := t.TempDir()
	destination := filepath.Join(directory, "checkpoint.tar")
	err = copyCheckpointArchive(
		archive,
		destination,
		&podCheckpointManifestContainer{ID: "source", SHA256: digest},
		"runc",
		&rspec.Spec{Process: &rspec.Process{}, Linux: &rspec.Linux{Seccomp: denyKill}},
	)
	require.ErrorContains(t, err, "resolved policy")
	entries, err := os.ReadDir(directory)
	require.NoError(t, err)
	require.Empty(t, entries, "incompatible checkpoint data must not remain staged")
}

func TestPodRestoreRejectsInvalidSeccompMetadata(t *testing.T) {
	for _, tc := range []struct {
		name     string
		data     []byte
		typeflag byte
		copies   int
		message  string
	}{
		{name: "malformed", data: []byte(`{`), typeflag: tar.TypeReg, copies: 1, message: "decode checkpoint OCI spec"},
		{name: "too large", data: bytes.Repeat([]byte(" "), checkpointMetadataLimit+1), typeflag: tar.TypeReg, copies: 1, message: "invalid or duplicate OCI spec"},
		{name: "duplicate", data: []byte(`{"linux":{}}`), typeflag: tar.TypeReg, copies: 2, message: "invalid or duplicate OCI spec"},
		{name: "symlink", typeflag: tar.TypeSymlink, copies: 1, message: "unexpected entry"},
		{name: "hardlink", typeflag: tar.TypeLink, copies: 1, message: "unexpected entry"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := os.CreateTemp(t.TempDir(), "archive")
			require.NoError(t, err)
			t.Cleanup(func() { file.Close() })

			writer := tar.NewWriter(file)

			for range tc.copies {
				header := &tar.Header{
					Name:     metadata.SpecDumpFile,
					Typeflag: tc.typeflag,
					Mode:     0o600,
					Size:     int64(len(tc.data)),
				}
				if tc.typeflag != tar.TypeReg {
					header.Linkname = "/outside"
				}

				require.NoError(t, writer.WriteHeader(header))
				_, err = writer.Write(tc.data)
				require.NoError(t, err)
			}

			require.NoError(t, writer.Close())

			_, err = checkpointArchiveImageID(
				file,
				&podCheckpointManifestContainer{},
				"runc",
				&rspec.Spec{Process: &rspec.Process{}, Linux: &rspec.Linux{}},
			)
			require.ErrorContains(t, err, tc.message)
		})
	}
}

func TestPodRestoreResolvedProcessSecurity(t *testing.T) {
	source := &rspec.Spec{
		Process: &rspec.Process{
			User: rspec.User{UID: 1000, GID: 1000, AdditionalGids: []uint32{1234, 5678}},
			Capabilities: &rspec.LinuxCapabilities{
				Bounding:  []string{"CAP_CHOWN", "CAP_KILL"},
				Effective: []string{"CAP_CHOWN"},
				Permitted: []string{
					"CAP_CHOWN",
				},
				Inheritable: []string{"CAP_CHOWN"},
				Ambient:     []string{"CAP_CHOWN"},
			},
			NoNewPrivileges: true,
			ApparmorProfile: "checkpoint-profile",
			SelinuxLabel:    "system_u:system_r:container_t:s0:c1,c2",
		},
		Linux: &rspec.Linux{},
	}
	encoded, err := json.Marshal(source)
	require.NoError(t, err)

	for _, tc := range []struct {
		name    string
		change  func(*rspec.Spec)
		allowed bool
	}{
		{name: "same policy", change: func(*rspec.Spec) {}, allowed: true},
		{name: "destination process config", change: func(s *rspec.Spec) {
			s.Process.Args, s.Process.Env, s.Process.Cwd = []string{"/new"}, []string{"POD_NAME=destination"}, "/new"
		}, allowed: true},
		{name: "reordered sets", change: func(s *rspec.Spec) {
			s.Process.User.AdditionalGids = []uint32{5678, 1234}
			s.Process.Capabilities.Bounding = []string{"CAP_KILL", "CAP_CHOWN"}
		}, allowed: true},
		{name: "UID", change: func(s *rspec.Spec) { s.Process.User.UID = 2000 }},
		{name: "GID", change: func(s *rspec.Spec) { s.Process.User.GID = 2000 }},
		{name: "supplementary groups", change: func(s *rspec.Spec) { s.Process.User.AdditionalGids = []uint32{1234} }},
		{name: "bounding capabilities", change: func(s *rspec.Spec) { s.Process.Capabilities.Bounding = nil }},
		{name: "effective capabilities", change: func(s *rspec.Spec) { s.Process.Capabilities.Effective = nil }},
		{name: "permitted capabilities", change: func(s *rspec.Spec) { s.Process.Capabilities.Permitted = nil }},
		{name: "inheritable capabilities", change: func(s *rspec.Spec) { s.Process.Capabilities.Inheritable = nil }},
		{name: "ambient capabilities", change: func(s *rspec.Spec) { s.Process.Capabilities.Ambient = nil }},
		{name: "no new privileges", change: func(s *rspec.Spec) { s.Process.NoNewPrivileges = false }},
		{name: "AppArmor profile", change: func(s *rspec.Spec) { s.Process.ApparmorProfile = "tightened-profile" }},
		{name: "SELinux label", change: func(s *rspec.Spec) { s.Process.SelinuxLabel = "system_u:system_r:container_t:s0:c3,c4" }},
		{name: "missing process", change: func(s *rspec.Spec) { s.Process = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var destination rspec.Spec
			require.NoError(t, json.Unmarshal(encoded, &destination))
			tc.change(&destination)

			err := validateCheckpointSecurity(source, &destination)
			if tc.allowed {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}

	after, err := json.Marshal(source)
	require.NoError(t, err)
	require.Equal(t, encoded, after, "set comparison must not mutate the captured policy")
	require.NoError(t, validateCheckpointSecurity(
		&rspec.Spec{
			Process: &rspec.Process{Capabilities: &rspec.LinuxCapabilities{Bounding: []string{}}},
			Linux:   &rspec.Linux{},
		},
		&rspec.Spec{Process: &rspec.Process{}, Linux: &rspec.Linux{}},
	))
	require.Error(t, validateCheckpointSecurity(&rspec.Spec{Linux: &rspec.Linux{}}, source))
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
		id, err := checkpointArchiveImageID(file, &entry, "runc", nil)
		require.NoError(t, err)
		assert.Equal(
			t,
			config.RootfsImageRef,
			id,
			"older manifests derive identity from the verified archive",
		)
		entry.ImageID = config.RootfsImageRef
		_, err = checkpointArchiveImageID(file, &entry, "runc", nil)
		require.NoError(t, err)

		entry.ImageID = strings.Repeat("b", 64)
		_, err = checkpointArchiveImageID(file, &entry, "runc", nil)
		require.ErrorContains(t, err, "base image")

		entry.ImageID = ""
		entry.ID = "other"
		_, err = checkpointArchiveImageID(file, &entry, "runc", nil)
		require.ErrorContains(t, err, "identity")

		entry.ID = config.ID
		_, err = checkpointArchiveImageID(file, &entry, "crun", nil)
		require.ErrorContains(t, err, "identity")
	}

	_, err := checkpointArchiveImageID(
		writePodArchive(t, config, true, false),
		&podCheckpointManifestContainer{ID: config.ID},
		"runc",
		nil,
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
			require.NoError(t, s.writePodCheckpointRecoveryMarker(&marker))

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
			// A replaced directory is left unchanged, but its marker must not
			// block later checkpoints of the sandbox.
			require.NoError(t, err)
			_, err = os.Stat(s.podCheckpointRecoveryMarkerPath(marker.SandboxID))
			require.ErrorIs(t, err, os.ErrNotExist)

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

const podRestoreTestImageID = "2a03a6059f21e150ae84b0973863609494aad70f0a80eaeb64bddd8d92465812"

type podRestoreMocks struct {
	store   *containermock.MockStore
	images  *criostoragemock.MockImageServer
	runtime *criostoragemock.MockRuntimeServer
}

// newPodRestoreServer creates a Server through New, as the Ginkgo server suite
// does, so restore code can use the runtime, name indexes, and image service
// without a container engine.
func newPodRestoreServer(
	t *testing.T,
	configure ...func(*libconfig.Config),
) (*Server, *podRestoreMocks) {
	t.Helper()

	ctrl := gomock.NewController(t)
	mocks := &podRestoreMocks{
		store:   containermock.NewMockStore(ctrl),
		images:  criostoragemock.NewMockImageServer(ctrl),
		runtime: criostoragemock.NewMockRuntimeServer(ctrl),
	}

	root := t.TempDir()
	emptyDir := t.TempDir()

	config, err := libconfig.DefaultConfig()
	require.NoError(t, err)

	config.ContainerAttachSocketDir = root
	config.ContainerExitsDir = filepath.Join(root, "exits")
	config.LogDir = filepath.Join(root, "log")
	config.CleanShutdownFile = filepath.Join(root, "clean.shutdown")
	config.IrqBalanceConfigRestoreFile = filepath.Join(root, "orig_irq_banned_cpus")
	config.Seccomp().SetNotifierPath(t.TempDir())
	config.NRI.SocketPath = filepath.Join(t.TempDir(), "nri.sock")
	config.DisableHostPortMapping = true
	config.NetworkDir = emptyDir
	config.PluginDirs = []string{emptyDir}
	config.HooksDir = []string{emptyDir}
	config.SetCheckpointRestore(true)

	for _, change := range configure {
		change(config)
	}

	cniPlugin := ocicnimock.NewMockCNIPlugin(ctrl)
	cniPlugin.EXPECT().StatusWithContext(gomock.Any()).Return(nil).AnyTimes()
	cniPlugin.EXPECT().GC(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	require.NoError(t, config.SetCNIPlugin(cniPlugin))

	graphroot := t.TempDir()
	configIface := libmock.NewMockIface(ctrl)
	configIface.EXPECT().GetData().Return(config).AnyTimes()
	configIface.EXPECT().GetStore().Return(mocks.store, nil)
	mocks.store.EXPECT().GraphRoot().Return(graphroot).AnyTimes()
	mocks.store.EXPECT().Containers().Return([]cstorage.Container{}, nil)

	s, err := New(context.Background(), configIface)
	require.NoError(t, err)

	s.SetStorageImageServer(mocks.images)
	s.SetStorageRuntimeServer(mocks.runtime)

	return s, mocks
}

func podRestoreTestContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	t.Cleanup(cancel)

	return ctx
}

func podRestoreSandboxConfig() *types.PodSandboxConfig {
	return &types.PodSandboxConfig{
		Metadata: &types.PodSandboxMetadata{Name: "pod", Namespace: "ns", Uid: "uid"},
		Linux: &types.LinuxPodSandboxConfig{
			SecurityContext: &types.LinuxSandboxSecurityContext{
				NamespaceOptions: &types.NamespaceOption{Network: types.NamespaceMode_NODE},
			},
		},
	}
}

func podRestoreContainerConfig(name string) *types.ContainerConfig {
	return &types.ContainerConfig{
		Metadata:   &types.ContainerMetadata{Name: name},
		Image:      &types.ImageSpec{Image: podRestoreTestImageID},
		WorkingDir: "/work",
	}
}

// writePodRestoreCheckpoint writes a Pod checkpoint with one archive for each
// container config and returns the matching RestorePod request.
func writePodRestoreCheckpoint(
	t *testing.T,
	configs ...*types.ContainerConfig,
) *types.RestorePodRequest {
	t.Helper()

	directory := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(directory, podCheckpointContainerDir), 0o700))
	require.NoError(
		t,
		writeCheckpointJSON(directory, podCheckpointConfigFile, podRestoreSandboxConfig()),
	)

	manifest := podCheckpointManifest{
		Version:        podCheckpointFormatVersion,
		Runtime:        "cri-o",
		RuntimeHandler: libconfig.DefaultRuntime,
		SandboxID:      "source",
	}

	request := &types.RestorePodRequest{
		CheckpointPath: directory,
		Config:         podRestoreSandboxConfig(),
	}

	for _, config := range configs {
		id := "source-" + config.GetMetadata().GetName()
		archive := writePodArchive(t, &metadata.ContainerConfig{
			ID:             id,
			OCIRuntime:     libconfig.DefaultRuntime,
			RootfsImageRef: podRestoreTestImageID,
		}, false, false)
		data, err := os.ReadFile(archive.Name())
		require.NoError(t, err)

		name := checkpointArchiveName(id)
		require.NoError(t, os.WriteFile(filepath.Join(directory, name), data, 0o600))

		digest, err := fileSHA256(filepath.Join(directory, name))
		require.NoError(t, err)

		manifest.Containers = append(manifest.Containers, podCheckpointManifestContainer{
			Name:    config.GetMetadata().GetName(),
			ID:      id,
			Archive: name,
			SHA256:  digest,
			Config:  proto.CloneOf(config),
			ImageID: podRestoreTestImageID,
		})
		request.ContainerConfigs = append(request.ContainerConfigs, proto.CloneOf(config))
	}

	require.NoError(t, writeCheckpointJSON(directory, podCheckpointManifestFile, &manifest))

	return request
}

func readPodRestoreManifest(t *testing.T, request *types.RestorePodRequest) *podCheckpointManifest {
	t.Helper()

	manifest := new(podCheckpointManifest)
	require.NoError(
		t,
		readCheckpointJSON(
			filepath.Join(request.GetCheckpointPath(), podCheckpointManifestFile),
			manifest,
		),
	)

	return manifest
}

func changePodRestoreManifest(
	t *testing.T,
	request *types.RestorePodRequest,
	change func(*podCheckpointManifest),
) {
	t.Helper()

	manifest := readPodRestoreManifest(t, request)
	change(manifest)
	require.NoError(
		t,
		os.Remove(filepath.Join(request.GetCheckpointPath(), podCheckpointManifestFile)),
	)
	require.NoError(
		t,
		writeCheckpointJSON(request.GetCheckpointPath(), podCheckpointManifestFile, manifest),
	)
}

// expectPodRestoreImage makes the image service report imageID as the local
// image for the checkpoint's base image reference.
func expectPodRestoreImage(t *testing.T, mocks *podRestoreMocks, imageID string, statusErr error) {
	t.Helper()

	requested, err := pkgstorage.ParseStorageImageIDFromOutOfProcessData(podRestoreTestImageID)
	require.NoError(t, err)

	local, err := pkgstorage.ParseStorageImageIDFromOutOfProcessData(imageID)
	require.NoError(t, err)

	mocks.images.EXPECT().
		HeuristicallyTryResolvingStringAsIDPrefix(podRestoreTestImageID).
		Return(&requested)

	if statusErr != nil {
		mocks.images.EXPECT().ImageStatusByID(gomock.Any(), requested).Return(nil, statusErr)

		return
	}

	mocks.images.EXPECT().
		ImageStatusByID(gomock.Any(), requested).
		Return(&pkgstorage.ImageResult{ID: local}, nil)
}

func TestRestorePodRejectsInvalidCheckpoints(t *testing.T) {
	for _, test := range []struct {
		name    string
		change  func(*testing.T, *types.RestorePodRequest)
		code    codes.Code
		message string
	}{
		{
			name:    "relative path",
			change:  func(_ *testing.T, r *types.RestorePodRequest) { r.CheckpointPath = "relative" },
			code:    codes.InvalidArgument,
			message: "checkpoint path must be absolute",
		},
		{
			name:    "missing sandbox metadata",
			change:  func(_ *testing.T, r *types.RestorePodRequest) { r.Config.Metadata = nil },
			code:    codes.InvalidArgument,
			message: "sandbox config and metadata are required",
		},
		{
			name: "too many containers",
			change: func(_ *testing.T, r *types.RestorePodRequest) {
				for range podCheckpointMaxContainers {
					r.ContainerConfigs = append(r.ContainerConfigs, podRestoreContainerConfig("app"))
				}
			},
			code:    codes.InvalidArgument,
			message: "between 1 and 1024 container configs are required",
		},
		{
			name:    "unknown runtime handler",
			change:  func(_ *testing.T, r *types.RestorePodRequest) { r.RuntimeHandler = "unknown" },
			code:    codes.InvalidArgument,
			message: "resolve runtime handler",
		},
		{
			name: "missing manifest",
			change: func(t *testing.T, r *types.RestorePodRequest) {
				require.NoError(
					t,
					os.Remove(filepath.Join(r.GetCheckpointPath(), podCheckpointManifestFile)),
				)
			},
			code:    codes.InvalidArgument,
			message: "read Pod checkpoint manifest",
		},
		{
			name: "unsupported format",
			change: func(t *testing.T, r *types.RestorePodRequest) {
				changePodRestoreManifest(t, r, func(m *podCheckpointManifest) { m.Version++ })
			},
			code:    codes.FailedPrecondition,
			message: "unsupported Pod checkpoint format version",
		},
		{
			name: "different runtime handler",
			change: func(t *testing.T, r *types.RestorePodRequest) {
				changePodRestoreManifest(t, r, func(m *podCheckpointManifest) {
					m.RuntimeHandler = "other"
				})
			},
			code:    codes.FailedPrecondition,
			message: "does not match checkpoint runtime handler",
		},
		{
			name: "missing sandbox config",
			change: func(t *testing.T, r *types.RestorePodRequest) {
				require.NoError(
					t,
					os.Remove(filepath.Join(r.GetCheckpointPath(), podCheckpointConfigFile)),
				)
			},
			code:    codes.InvalidArgument,
			message: "read checkpoint sandbox config",
		},
		{
			name: "incompatible sandbox config",
			change: func(_ *testing.T, r *types.RestorePodRequest) {
				r.Config.Linux.SecurityContext.Privileged = true
			},
			code:    codes.FailedPrecondition,
			message: "validate restore sandbox config",
		},
		{
			name: "container without name",
			change: func(_ *testing.T, r *types.RestorePodRequest) {
				r.ContainerConfigs[0].Metadata = nil
			},
			code:    codes.InvalidArgument,
			message: "every restore container config must have a metadata name",
		},
		{
			name:    "container without image",
			change:  func(_ *testing.T, r *types.RestorePodRequest) { r.ContainerConfigs[0].Image = nil },
			code:    codes.InvalidArgument,
			message: `restore container "app" has no image`,
		},
		{
			name: "duplicate container name",
			change: func(_ *testing.T, r *types.RestorePodRequest) {
				r.ContainerConfigs[1].Metadata.Name = "app"
			},
			code:    codes.InvalidArgument,
			message: `restore container name "app" is duplicated`,
		},
		{
			name: "different container set",
			change: func(_ *testing.T, r *types.RestorePodRequest) {
				r.ContainerConfigs = r.GetContainerConfigs()[:1]
			},
			code:    codes.FailedPrecondition,
			message: "restore container set does not match checkpoint",
		},
		{
			name: "missing archive directory",
			change: func(t *testing.T, r *types.RestorePodRequest) {
				require.NoError(
					t,
					os.RemoveAll(filepath.Join(r.GetCheckpointPath(), podCheckpointContainerDir)),
				)
			},
			code:    codes.InvalidArgument,
			message: "open checkpoint archives",
		},
		{
			name: "duplicate manifest ID",
			change: func(t *testing.T, r *types.RestorePodRequest) {
				changePodRestoreManifest(t, r, func(m *podCheckpointManifest) {
					m.Containers[1].ID = m.Containers[0].ID
				})
			},
			code:    codes.InvalidArgument,
			message: "duplicate IDs or inconsistent container names",
		},
		{
			name: "incomplete manifest entry",
			change: func(t *testing.T, r *types.RestorePodRequest) {
				changePodRestoreManifest(t, r, func(m *podCheckpointManifest) { m.Containers[0].ID = "" })
			},
			code:    codes.InvalidArgument,
			message: "incomplete container entry",
		},
		{
			name: "duplicate manifest name",
			change: func(t *testing.T, r *types.RestorePodRequest) {
				changePodRestoreManifest(t, r, func(m *podCheckpointManifest) {
					m.Containers[1].Name = m.Containers[0].Name
				})
			},
			code:    codes.InvalidArgument,
			message: `duplicate container name "app"`,
		},
		{
			name: "inconsistent manifest name",
			change: func(t *testing.T, r *types.RestorePodRequest) {
				changePodRestoreManifest(t, r, func(m *podCheckpointManifest) {
					m.Containers[0].Config.Metadata.Name = "other"
				})
			},
			code:    codes.InvalidArgument,
			message: "duplicate IDs or inconsistent container names",
		},
		{
			name: "invalid archive name",
			change: func(t *testing.T, r *types.RestorePodRequest) {
				changePodRestoreManifest(t, r, func(m *podCheckpointManifest) {
					m.Containers[0].Archive = podCheckpointContainerDir + "/other.tar"
				})
			},
			code:    codes.InvalidArgument,
			message: `checkpoint archive path for container "app" is invalid`,
		},
		{
			name: "missing archive",
			change: func(t *testing.T, r *types.RestorePodRequest) {
				manifest := readPodRestoreManifest(t, r)
				require.NoError(
					t,
					os.Remove(filepath.Join(r.GetCheckpointPath(), manifest.Containers[0].Archive)),
				)
			},
			code:    codes.InvalidArgument,
			message: `open checkpoint archive for "app"`,
		},
		{
			name: "changed archive",
			change: func(t *testing.T, r *types.RestorePodRequest) {
				changePodRestoreManifest(t, r, func(m *podCheckpointManifest) {
					m.Containers[0].SHA256 = strings.Repeat("0", 64)
				})
			},
			code:    codes.InvalidArgument,
			message: `checkpoint archive checksum for container "app" does not match manifest`,
		},
		{
			name: "container missing from request",
			change: func(_ *testing.T, r *types.RestorePodRequest) {
				r.ContainerConfigs[1].Metadata.Name = "other"
			},
			code:    codes.FailedPrecondition,
			message: `checkpoint container "sidecar" is missing from restore request`,
		},
		{
			name: "incompatible container config",
			change: func(_ *testing.T, r *types.RestorePodRequest) {
				r.ContainerConfigs[0].WorkingDir = "/other"
			},
			code:    codes.FailedPrecondition,
			message: `validate restore config for container "app"`,
		},
		{
			name: "archive base image differs from manifest",
			change: func(t *testing.T, r *types.RestorePodRequest) {
				changePodRestoreManifest(t, r, func(m *podCheckpointManifest) {
					m.Containers[0].ImageID = strings.Repeat("b", 64)
				})
			},
			code:    codes.InvalidArgument,
			message: `validate checkpoint archive for "app"`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, _ := newPodRestoreServer(t)
			request := writePodRestoreCheckpoint(
				t,
				podRestoreContainerConfig("app"),
				podRestoreContainerConfig("sidecar"),
			)
			test.change(t, request)

			response, err := s.restorePod(podRestoreTestContext(t), request)
			require.Nil(t, response)
			require.Equal(t, test.code, status.Code(err), err)
			assert.Contains(t, status.Convert(err).Message(), test.message)

			_, err = os.Stat(s.podRestoreTransactionDirectory())
			require.ErrorIs(t, err, os.ErrNotExist, "validation must not start a transaction")
		})
	}
}

func addPodRestoreSandbox(
	t *testing.T,
	s *Server,
	id string,
	podMetadata *types.PodSandboxMetadata,
) *sandbox.Sandbox {
	t.Helper()

	builder := sandbox.NewBuilder()
	builder.SetID(id)
	builder.SetName(id + "-name")
	builder.SetLogDir(t.TempDir())
	builder.SetShmPath("")
	builder.SetNamespace(podMetadata.GetNamespace())
	builder.SetKubeName(podMetadata.GetName())
	builder.SetMountLabel("")
	builder.SetProcessLabel("")
	builder.SetCgroupParent("")
	builder.SetRuntimeHandler("")
	builder.SetResolvPath("")
	builder.SetHostname("")
	builder.SetPortMappings([]*hostport.PortMapping{})
	builder.SetHostNetwork(true)
	builder.SetUsernsMode("")
	builder.SetPodLinuxOverhead(nil)
	builder.SetPodLinuxResources(nil)
	builder.SetCreatedAt(time.Now())
	require.NoError(t, builder.SetCRISandbox(id, nil, nil, podMetadata))
	builder.SetPrivileged(false)
	builder.SetContainers(memorystore.New[*oci.Container]())

	sb, err := builder.GetSandbox()
	require.NoError(t, err)
	require.NoError(t, s.AddSandbox(context.Background(), sb))
	require.NoError(t, s.PodIDIndex().Add(id))
	sb.SetCreated()

	return sb
}

func addPodRestoreContainer(
	t *testing.T,
	s *Server,
	id, name, sandboxID, restoreKey string,
) *oci.Container {
	t.Helper()

	container, err := oci.NewContainer(
		id, id+"-name", "", "", nil, nil, nil, "", nil, nil, "",
		&types.ContainerMetadata{Name: name}, sandboxID, false, false, false,
		"", t.TempDir(), time.Now(), "",
	)
	require.NoError(t, err)

	// Match a container that RestorePod has prepared: created, with the
	// checkpoint archive staged in its directory.
	archive := filepath.Join(container.Dir(), podRestoreArchiveFile)
	require.NoError(t, os.WriteFile(archive, []byte("archive"), 0o600))
	container.PreparePodRestore(restoreKey)
	container.SetRestoreArchivePath(archive)
	container.SetCreated()
	s.AddContainer(context.Background(), container)
	require.NoError(t, s.CtrIDIndex().Add(id))

	return container
}

// podRestoreTransactionFor returns the transaction RestorePod saves for
// request, with the fingerprint that RestorePod computes from the checkpoint.
func podRestoreTransactionFor(
	t *testing.T,
	request *types.RestorePodRequest,
) *podRestoreTransaction {
	t.Helper()

	fingerprint, err := podRestoreFingerprint(
		request,
		readPodRestoreManifest(t, request),
		podRestoreSandboxConfig(),
	)
	require.NoError(t, err)

	return &podRestoreTransaction{
		Version:        podRestoreTransactionVersion,
		Key:            podRestoreKey(request.GetConfig().GetMetadata()),
		SandboxName:    makeSandboxContainerName(request.GetConfig()),
		Fingerprint:    fingerprint,
		RuntimeHandler: libconfig.DefaultRuntime,
	}
}

// expectPodRestoreSandboxFailure fails sandbox storage creation and lets the
// rollback find no storage for the attempted sandbox.
func expectPodRestoreSandboxFailure(mocks *podRestoreMocks) {
	mocks.runtime.EXPECT().CreatePodSandbox(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(pkgstorage.ContainerInfo{}, errors.New("storage unavailable"))
	mocks.store.EXPECT().Container(gomock.Any()).Return(nil, cstorage.ErrContainerUnknown)
}

func TestRestorePodRejectsUnsupportedRuntimeType(t *testing.T) {
	s, _ := newPodRestoreServer(t, func(config *libconfig.Config) {
		config.Runtimes["vm"] = &libconfig.RuntimeHandler{
			RuntimeType: libconfig.RuntimeTypeVM,
			RuntimePath: "/usr/bin/vm-runtime",
		}
	})
	request := writePodRestoreCheckpoint(t, podRestoreContainerConfig("app"))
	request.RuntimeHandler = "vm"

	response, err := s.restorePod(podRestoreTestContext(t), request)
	require.Nil(t, response)
	require.Equal(t, codes.Unimplemented, status.Code(err), err)
	assert.Contains(t, status.Convert(err).Message(), "does not support runtime type")
}

func TestRestorePodKeepsTransactionWhenRollbackFails(t *testing.T) {
	s, mocks := newPodRestoreServer(t)
	request := writePodRestoreCheckpoint(t, podRestoreContainerConfig("app"))
	expectPodRestoreImage(t, mocks, podRestoreTestImageID, nil)
	mocks.runtime.EXPECT().CreatePodSandbox(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(pkgstorage.ContainerInfo{}, errors.New("storage unavailable"))
	// The failed attempt left storage that the server has not loaded, so the
	// rollback cannot tell whether it is safe to remove.
	mocks.store.EXPECT().Container(gomock.Any()).Return(&cstorage.Container{}, nil)

	response, err := s.restorePod(podRestoreTestContext(t), request)
	require.Nil(t, response)
	require.ErrorContains(t, err, "create restore sandbox")
	require.ErrorContains(t, err, "roll back Pod restore transaction")

	saved := new(podRestoreTransaction)
	require.NoError(t, readCheckpointJSON(
		s.podRestoreTransactionPath(podRestoreKey(request.GetConfig().GetMetadata())),
		saved,
	))
	assert.NotEmpty(t, saved.SandboxID, "startup recovery needs the sandbox owner")
}

func TestRestorePodVerifiesBaseImages(t *testing.T) {
	for _, test := range []struct {
		name      string
		imageID   string
		statusErr error
		code      codes.Code
		message   string
	}{
		{
			name:      "image status error",
			imageID:   podRestoreTestImageID,
			statusErr: errors.New("image store unavailable"),
			code:      codes.Unknown,
			message:   `inspect image for container "app"`,
		},
		{
			name:      "image not pulled",
			imageID:   podRestoreTestImageID,
			statusErr: cstorage.ErrImageUnknown,
			code:      codes.FailedPrecondition,
			message:   `base image for container "app" must be pulled before RestorePod`,
		},
		{
			name:    "different local image",
			imageID: strings.Repeat("c", 64),
			code:    codes.FailedPrecondition,
			message: `base image for container "app" does not match the checkpoint`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, mocks := newPodRestoreServer(t)
			request := writePodRestoreCheckpoint(t, podRestoreContainerConfig("app"))
			expectPodRestoreImage(t, mocks, test.imageID, test.statusErr)

			response, err := s.restorePod(podRestoreTestContext(t), request)
			require.Nil(t, response)
			require.Equal(t, test.code, status.Code(err), err)
			assert.Contains(t, err.Error(), test.message)

			_, err = os.Stat(s.podRestoreTransactionDirectory())
			require.ErrorIs(t, err, os.ErrNotExist, "image checks must not start a transaction")
		})
	}
}

func TestRestorePodRollsBackFailedSandboxCreation(t *testing.T) {
	s, mocks := newPodRestoreServer(t)
	request := writePodRestoreCheckpoint(t, podRestoreContainerConfig("app"))
	key := podRestoreKey(request.GetConfig().GetMetadata())

	// A retry after the rollback must start a new attempt.
	for range 2 {
		expectPodRestoreImage(t, mocks, podRestoreTestImageID, nil)
		expectPodRestoreSandboxFailure(mocks)

		response, err := s.restorePod(podRestoreTestContext(t), request)
		require.Nil(t, response)
		require.ErrorContains(t, err, "create restore sandbox")
		require.ErrorContains(t, err, "storage unavailable")

		_, err = os.Stat(s.podRestoreTransactionPath(key))
		require.ErrorIs(t, err, os.ErrNotExist)
		_, err = s.PodIDForName(makeSandboxContainerName(request.GetConfig()))
		require.Error(t, err, "the failed attempt must release the sandbox name")
	}
}

func TestRestorePodRejectsConcurrentRestore(t *testing.T) {
	s, mocks := newPodRestoreServer(t)
	request := writePodRestoreCheckpoint(t, podRestoreContainerConfig("app"))

	requested, err := pkgstorage.ParseStorageImageIDFromOutOfProcessData(podRestoreTestImageID)
	require.NoError(t, err)

	entered := make(chan struct{})
	release := make(chan struct{})

	mocks.images.EXPECT().
		HeuristicallyTryResolvingStringAsIDPrefix(podRestoreTestImageID).
		Return(&requested)
	mocks.images.EXPECT().
		ImageStatusByID(gomock.Any(), requested).
		DoAndReturn(func(any, pkgstorage.StorageImageID) (*pkgstorage.ImageResult, error) {
			close(entered)
			<-release

			return nil, errors.New("first restore stopped")
		})

	ctx := podRestoreTestContext(t)
	firstErr := make(chan error, 1)

	go func() {
		_, err := s.restorePod(ctx, request)
		firstErr <- err
	}()

	<-entered

	response, err := s.restorePod(ctx, request)
	require.Nil(t, response)
	require.Equal(t, codes.AlreadyExists, status.Code(err), err)
	assert.Contains(t, status.Convert(err).Message(), "already in progress")

	close(release)
	require.ErrorContains(t, <-firstErr, "first restore stopped")
}

// Native preparation rejects every saved admission; operation retries are
// handled by the operation manager before reaching this backend.
func TestRestorePodWithPreviousTransaction(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*podRestoreTransaction)
	}{
		{name: "interrupted attempt"},
		{name: "different request", change: func(p *podRestoreTransaction) { p.Fingerprint = "different" }},
		{name: "different operation", change: func(p *podRestoreTransaction) { p.OperationID = "other" }},
		{name: "different runtime", change: func(p *podRestoreTransaction) { p.RuntimeHandler = "other" }},
		{name: "completed attempt", change: func(p *podRestoreTransaction) {
			p.SandboxID = "restored"
			p.Response = &types.RestorePodResponse{PodSandboxId: "restored"}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, _ := newPodRestoreServer(t)
			request := writePodRestoreCheckpoint(t, podRestoreContainerConfig("app"))
			transaction := podRestoreTransactionFor(t, request)

			if test.change != nil {
				test.change(transaction)
			}

			require.NoError(t, s.writePodRestoreTransaction(transaction))
			path := s.podRestoreTransactionPath(transaction.Key)
			before, err := os.ReadFile(path)
			require.NoError(t, err)

			response, err := s.restorePod(podRestoreTestContext(t), request)
			require.Nil(t, response)
			require.Equal(t, codes.AlreadyExists, status.Code(err), err)
			require.ErrorContains(t, err, "already admitted")
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, before, after, "a duplicate must preserve the original receipt")
		})
	}

	t.Run("unrelated sandbox", func(t *testing.T) {
		s, _ := newPodRestoreServer(t)
		request := writePodRestoreCheckpoint(t, podRestoreContainerConfig("app"))
		name := makeSandboxContainerName(request.GetConfig())
		_, err := s.ReservePodName("unrelated", name)
		require.NoError(t, err)

		response, err := s.restorePod(podRestoreTestContext(t), request)
		require.Nil(t, response)
		require.Equal(t, codes.AlreadyExists, status.Code(err), err)
		require.ErrorContains(t, err, "sandbox already owns")
		owner, err := s.PodIDForName(name)
		require.NoError(t, err)
		assert.Equal(t, "unrelated", owner)
	})

	t.Run("unreadable transaction", func(t *testing.T) {
		s, _ := newPodRestoreServer(t)
		request := writePodRestoreCheckpoint(t, podRestoreContainerConfig("app"))
		path := s.podRestoreTransactionPath(podRestoreKey(request.GetConfig().GetMetadata()))
		require.NoError(t, os.MkdirAll(s.podRestoreTransactionDirectory(), 0o700))
		require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))

		response, err := s.restorePod(podRestoreTestContext(t), request)
		require.Nil(t, response)
		require.ErrorContains(t, err, "read Pod restore transaction")
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "{", string(data))
	})
}

func TestExistingPodRestoreResponse(t *testing.T) {
	const key = "ns/pod/uid/0"

	configs := []*types.ContainerConfig{podRestoreContainerConfig("app")}

	for _, test := range []struct {
		name    string
		change  func(*testing.T, *podRestoreTransaction, *oci.Container)
		message string
	}{
		{name: "complete"},
		{
			name: "missing response",
			change: func(_ *testing.T, transaction *podRestoreTransaction, _ *oci.Container) {
				transaction.Response = nil
			},
			message: "incomplete container mapping",
		},
		{
			name: "other container name",
			change: func(_ *testing.T, transaction *podRestoreTransaction, _ *oci.Container) {
				transaction.Response.RestoredContainers[0].Name = "other"
			},
			message: "inconsistent container mapping",
		},
		{
			name: "container of another restore",
			change: func(_ *testing.T, _ *podRestoreTransaction, container *oci.Container) {
				container.PreparePodRestore("other")
			},
			message: `restored container "app" is missing or changed`,
		},
		{
			name: "created without pending restore",
			change: func(_ *testing.T, _ *podRestoreTransaction, container *oci.Container) {
				container.SetRestore(false)
			},
			message: `container "app" has no pending restore`,
		},
		{
			name: "pending restore without archive",
			change: func(t *testing.T, _ *podRestoreTransaction, container *oci.Container) {
				require.NoError(t, os.Remove(container.RestoreArchivePath()))
			},
			message: `container "app" has no staged restore archive`,
		},
		{
			name: "started after restore",
			change: func(t *testing.T, transaction *podRestoreTransaction, container *oci.Container) {
				require.NoError(t, os.Remove(container.RestoreArchivePath()))
				container.SetState(&oci.ContainerState{
					State:         rspec.State{Status: oci.ContainerStateRunning},
					PodRestoreKey: transaction.Key,
				})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, _ := newPodRestoreServer(t)
			addPodRestoreSandbox(t, s, "restored", podRestoreSandboxConfig().GetMetadata())
			container := addPodRestoreContainer(t, s, "restored-app", "app", "restored", key)
			transaction := &podRestoreTransaction{
				Key:            key,
				SandboxID:      "restored",
				RuntimeHandler: libconfig.DefaultRuntime,
				Response: &types.RestorePodResponse{
					PodSandboxId: "restored",
					RestoredContainers: []*types.RestoredContainer{
						{Name: "app", ContainerId: "restored-app"},
					},
				},
			}

			if test.change != nil {
				test.change(t, transaction, container)
			}

			response, err := s.existingPodRestoreResponse(transaction, configs)
			if test.message == "" {
				require.NoError(t, err)
				assert.True(t, proto.Equal(transaction.Response, response))
				assert.NotSame(t, transaction.Response, response, "callers must get a copy")

				return
			}

			require.Nil(t, response)
			require.Equal(t, codes.FailedPrecondition, status.Code(err), err)
			assert.Contains(t, status.Convert(err).Message(), test.message)
		})
	}
}

func TestBeginPodRestoreTransactionChecksPreviousAttempt(t *testing.T) {
	for _, test := range []struct {
		name     string
		previous func(*podRestoreTransaction)
		code     codes.Code
		message  string
	}{
		{
			name:     "different target",
			previous: func(p *podRestoreTransaction) { p.SandboxName = "other" },
			code:     codes.Unknown,
			message:  "does not match the restore target",
		},
		{
			name: "committed",
			previous: func(p *podRestoreTransaction) {
				p.SandboxID = "restored"
				p.Response = &types.RestorePodResponse{PodSandboxId: "restored"}
			},
			code:    codes.AlreadyExists,
			message: "a committed restore already exists",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, _ := newPodRestoreServer(t)
			transaction := &podRestoreTransaction{
				Version:     podRestoreTransactionVersion,
				Key:         "ns/pod/uid/0",
				SandboxName: "sandbox-name",
				SandboxID:   "new",
				Fingerprint: "request",
			}
			previous := *transaction
			previous.SandboxID = ""
			test.previous(&previous)
			require.NoError(t, s.writePodRestoreTransaction(&previous))

			err := s.beginPodRestoreTransaction(context.Background(), transaction)
			require.Equal(t, test.code, status.Code(err), err)
			require.ErrorContains(t, err, test.message)

			saved := new(podRestoreTransaction)
			require.NoError(
				t,
				readCheckpointJSON(s.podRestoreTransactionPath(transaction.Key), saved),
			)
			assert.Equal(t, previous, *saved, "the previous transaction must stay in place")
		})
	}

	s, _ := newPodRestoreServer(t)
	transaction := &podRestoreTransaction{
		Version:     podRestoreTransactionVersion,
		Key:         "ns/pod/uid/0",
		SandboxName: "sandbox-name",
	}

	require.NoError(t, os.MkdirAll(s.podRestoreTransactionDirectory(), 0o700))
	require.NoError(
		t,
		os.WriteFile(s.podRestoreTransactionPath(transaction.Key), []byte("{"), 0o600),
	)
	require.ErrorContains(
		t,
		s.beginPodRestoreTransaction(context.Background(), transaction),
		"read previous Pod restore transaction",
	)
}

func TestRollbackPodRestore(t *testing.T) {
	storageErr := errors.New("storage unavailable")

	for _, test := range []struct {
		name    string
		prepare func(*testing.T, *Server, *podRestoreMocks, *podRestoreTransaction)
		message string
	}{
		{
			name: "unowned attempt without storage",
			prepare: func(_ *testing.T, _ *Server, mocks *podRestoreMocks, p *podRestoreTransaction) {
				p.SandboxID = ""
				mocks.store.EXPECT().Container(p.SandboxName).Return(nil, cstorage.ErrContainerUnknown)
			},
		},
		{
			name: "unowned attempt with reserved name",
			prepare: func(t *testing.T, s *Server, _ *podRestoreMocks, p *podRestoreTransaction) {
				p.SandboxID = ""
				_, err := s.ReservePodName("unrelated", p.SandboxName)
				require.NoError(t, err)
			},
			message: "no recorded sandbox owner",
		},
		{
			name: "unowned attempt with storage",
			prepare: func(_ *testing.T, _ *Server, mocks *podRestoreMocks, p *podRestoreTransaction) {
				p.SandboxID = ""
				mocks.store.EXPECT().Container(p.SandboxName).Return(&cstorage.Container{}, nil)
			},
			message: "unclaimed sandbox storage",
		},
		{
			name: "unowned attempt with storage error",
			prepare: func(_ *testing.T, _ *Server, mocks *podRestoreMocks, p *podRestoreTransaction) {
				p.SandboxID = ""
				mocks.store.EXPECT().Container(p.SandboxName).Return(nil, storageErr)
			},
			message: storageErr.Error(),
		},
		{
			name: "sandbox never created",
			prepare: func(_ *testing.T, _ *Server, mocks *podRestoreMocks, p *podRestoreTransaction) {
				mocks.store.EXPECT().Container(p.SandboxID).Return(nil, cstorage.ErrContainerUnknown)
			},
		},
		{
			name: "sandbox storage not loaded",
			prepare: func(_ *testing.T, _ *Server, mocks *podRestoreMocks, p *podRestoreTransaction) {
				mocks.store.EXPECT().Container(p.SandboxID).Return(&cstorage.Container{}, nil)
			},
			message: `restore sandbox "restored" has not been loaded`,
		},
		{
			name: "sandbox storage error",
			prepare: func(_ *testing.T, _ *Server, mocks *podRestoreMocks, p *podRestoreTransaction) {
				mocks.store.EXPECT().Container(p.SandboxID).Return(nil, storageErr)
			},
			message: storageErr.Error(),
		},
		{
			name: "container list error",
			prepare: func(t *testing.T, s *Server, mocks *podRestoreMocks, p *podRestoreTransaction) {
				addPodRestoreSandbox(t, s, p.SandboxID, podRestoreSandboxConfig().GetMetadata())
				mocks.store.EXPECT().Containers().Return(nil, storageErr)
			},
			message: storageErr.Error(),
		},
		{
			name: "container metadata error",
			prepare: func(t *testing.T, s *Server, mocks *podRestoreMocks, p *podRestoreTransaction) {
				addPodRestoreSandbox(t, s, p.SandboxID, podRestoreSandboxConfig().GetMetadata())
				mocks.store.EXPECT().Containers().Return([]cstorage.Container{{ID: "app"}}, nil)
				mocks.store.EXPECT().Metadata("app").Return("", storageErr)
			},
			message: storageErr.Error(),
		},
		{
			name: "invalid container metadata",
			prepare: func(t *testing.T, s *Server, mocks *podRestoreMocks, p *podRestoreTransaction) {
				addPodRestoreSandbox(t, s, p.SandboxID, podRestoreSandboxConfig().GetMetadata())
				mocks.store.EXPECT().Containers().Return([]cstorage.Container{{ID: "app"}}, nil)
				mocks.store.EXPECT().Metadata("app").Return("{", nil)
			},
			message: "unexpected end of JSON input",
		},
		{
			name: "restored container not loaded",
			prepare: func(t *testing.T, s *Server, mocks *podRestoreMocks, p *podRestoreTransaction) {
				addPodRestoreSandbox(t, s, p.SandboxID, podRestoreSandboxConfig().GetMetadata())
				mocks.store.EXPECT().Containers().Return(
					[]cstorage.Container{{ID: p.SandboxID}, {ID: "other"}, {ID: "app"}},
					nil,
				)
				mocks.store.EXPECT().Metadata("other").Return(`{"pod-id":"unrelated"}`, nil)
				mocks.store.EXPECT().Metadata("app").Return(`{"pod-id":"restored"}`, nil)
			},
			message: `restore container "app" has not been loaded`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, mocks := newPodRestoreServer(t)
			transaction := &podRestoreTransaction{
				Version:     podRestoreTransactionVersion,
				Key:         "ns/pod/uid/0",
				SandboxName: "sandbox-name",
				SandboxID:   "restored",
			}
			test.prepare(t, s, mocks, transaction)

			err := s.rollbackPodRestore(context.Background(), transaction)
			if test.message == "" {
				require.NoError(t, err)

				return
			}

			require.ErrorContains(t, err, test.message)
		})
	}
}

func TestRecoverPodRestores(t *testing.T) {
	s, mocks := newPodRestoreServer(t)
	require.NoError(t, s.recoverPodRestores(context.Background()), "no transactions")

	committed := &podRestoreTransaction{
		Version:     podRestoreTransactionVersion,
		Key:         "ns/committed/uid/0",
		SandboxName: "committed-name",
		SandboxID:   "committed",
		Fingerprint: "request",
		Response:    &types.RestorePodResponse{PodSandboxId: "committed"},
	}
	interrupted := &podRestoreTransaction{
		Version:     podRestoreTransactionVersion,
		Key:         "ns/interrupted/uid/0",
		SandboxName: "interrupted-name",
	}

	require.NoError(t, s.writePodRestoreTransaction(committed))
	require.NoError(t, s.writePodRestoreTransaction(interrupted))

	directory := s.podRestoreTransactionDirectory()
	temporary := filepath.Join(directory, ".checkpoint-temporary")
	require.NoError(t, os.WriteFile(temporary, []byte("partial"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(directory, "subdirectory"), 0o700))
	mocks.store.EXPECT().
		Container(interrupted.SandboxName).
		Return(nil, cstorage.ErrContainerUnknown)

	require.NoError(t, s.recoverPodRestores(context.Background()))

	_, err := os.Stat(s.podRestoreTransactionPath(committed.Key))
	require.NoError(t, err, "startup must keep completed restores")
	_, err = os.Stat(s.podRestoreTransactionPath(interrupted.Key))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(temporary)
	require.ErrorIs(t, err, os.ErrNotExist)

	mocks.store.EXPECT().
		Container(interrupted.SandboxName).
		Return(&cstorage.Container{}, nil)
	require.NoError(t, s.writePodRestoreTransaction(interrupted))
	require.ErrorContains(
		t,
		s.recoverPodRestores(context.Background()),
		"unclaimed sandbox storage",
	)
	_, err = os.Stat(s.podRestoreTransactionPath(interrupted.Key))
	require.NoError(t, err, "a failed rollback must keep the transaction")
}

func TestRecoverPodRestoresRejectsInvalidTransactions(t *testing.T) {
	for _, test := range []struct {
		name    string
		write   func(*testing.T, *Server)
		message string
	}{
		{
			name: "malformed",
			write: func(t *testing.T, s *Server) {
				require.NoError(
					t,
					os.WriteFile(filepath.Join(s.podRestoreTransactionDirectory(), "bad.json"), []byte("{"), 0o600),
				)
			},
			message: "decode checkpoint metadata",
		},
		{
			name: "unsupported version",
			write: func(t *testing.T, s *Server) {
				require.NoError(t, s.writePodRestoreTransaction(&podRestoreTransaction{
					Key:         "ns/pod/uid/0",
					SandboxName: "sandbox-name",
				}))
			},
			message: "invalid Pod restore transaction",
		},
		{
			name: "different filename",
			write: func(t *testing.T, s *Server) {
				require.NoError(t, writeCheckpointJSON(
					s.podRestoreTransactionDirectory(),
					"other.json",
					&podRestoreTransaction{
						Version:     podRestoreTransactionVersion,
						Key:         "ns/pod/uid/0",
						SandboxName: "sandbox-name",
					},
				))
			},
			message: "has an invalid filename",
		},
		{
			name: "completed without fingerprint",
			write: func(t *testing.T, s *Server) {
				require.NoError(t, s.writePodRestoreTransaction(&podRestoreTransaction{
					Version:     podRestoreTransactionVersion,
					Key:         "ns/pod/uid/0",
					SandboxName: "sandbox-name",
					SandboxID:   "restored",
					Response:    &types.RestorePodResponse{PodSandboxId: "restored"},
				}))
			},
			message: "invalid completed Pod restore transaction",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, _ := newPodRestoreServer(t)
			require.NoError(t, os.MkdirAll(s.podRestoreTransactionDirectory(), 0o700))
			test.write(t, s)

			require.ErrorContains(t, s.recoverPodRestores(context.Background()), test.message)
		})
	}
}

func TestRemoveCompletedPodRestore(t *testing.T) {
	podMetadata := podRestoreSandboxConfig().GetMetadata()
	key := podRestoreKey(podMetadata)

	for _, test := range []struct {
		name        string
		transaction *podRestoreTransaction
		removed     bool
	}{
		{name: "no transaction"},
		{
			name: "interrupted",
			transaction: &podRestoreTransaction{
				SandboxID: "restored",
			},
		},
		{
			name: "completed for another sandbox",
			transaction: &podRestoreTransaction{
				SandboxID:   "other",
				Fingerprint: "request",
				Response:    &types.RestorePodResponse{PodSandboxId: "other"},
			},
		},
		{
			name: "completed",
			transaction: &podRestoreTransaction{
				SandboxID:   "restored",
				Fingerprint: "request",
				Response:    &types.RestorePodResponse{PodSandboxId: "restored"},
			},
			removed: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, _ := newPodRestoreServer(t)
			sb := addPodRestoreSandbox(t, s, "restored", podMetadata)

			if test.transaction != nil {
				test.transaction.Version = podRestoreTransactionVersion
				test.transaction.Key = key
				test.transaction.SandboxName = "sandbox-name"
				require.NoError(t, s.writePodRestoreTransaction(test.transaction))
			}

			require.NoError(t, s.removeCompletedPodRestore(sb))

			_, err := os.Stat(s.podRestoreTransactionPath(key))
			if test.removed || test.transaction == nil {
				require.ErrorIs(t, err, os.ErrNotExist)
			} else {
				require.NoError(t, err)
			}
		})
	}

	s, _ := newPodRestoreServer(t)
	sb := addPodRestoreSandbox(t, s, "restored", podMetadata)
	require.NoError(t, os.MkdirAll(s.podRestoreTransactionDirectory(), 0o700))
	require.NoError(t, os.WriteFile(s.podRestoreTransactionPath(key), []byte("{"), 0o600))
	require.Error(t, s.removeCompletedPodRestore(sb))
}

func TestStartContainerRequiresCommittedPodRestore(t *testing.T) {
	s, _ := newPodRestoreServer(t)
	podMetadata := podRestoreSandboxConfig().GetMetadata()
	addPodRestoreSandbox(t, s, "restored", podMetadata)
	container := addPodRestoreContainer(
		t,
		s,
		"restored-app",
		"app",
		"restored",
		podRestoreKey(podMetadata),
	)

	response, err := s.StartContainer(
		podRestoreTestContext(t),
		&types.StartContainerRequest{ContainerId: container.ID()},
	)
	require.Nil(t, response)
	require.ErrorContains(t, err, "read container Pod restore transaction")
}

func TestPodRestoreArchiveRejectsInvalidIdentity(t *testing.T) {
	file := writePodArchive(t, &metadata.ContainerConfig{
		ID:             "source",
		OCIRuntime:     "runc",
		RootfsImageRef: "not-an-image-id",
	}, false, false)
	_, err := checkpointArchiveImageID(
		file,
		&podCheckpointManifestContainer{ID: "source"},
		"runc",
		nil,
	)
	require.ErrorContains(t, err, "no valid immutable base image ID")

	path := filepath.Join(t.TempDir(), "corrupt.tar")
	require.NoError(t, os.WriteFile(path, bytes.Repeat([]byte("x"), 1024), 0o600))
	corrupt, err := openCheckpointFile(path)
	require.NoError(t, err)
	t.Cleanup(func() { corrupt.Close() })

	_, err = checkpointArchiveImageID(
		corrupt,
		&podCheckpointManifestContainer{ID: "source"},
		"runc",
		nil,
	)
	require.ErrorContains(t, err, "read checkpoint archive")
}
