package server

import (
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func TestPodCheckpointHelpers(t *testing.T) {
	t.Run("output validation", func(t *testing.T) {
		output := t.TempDir()
		if err := validateEmptyCheckpointOutput(output); err != nil {
			t.Fatalf("validate empty output: %v", err)
		}

		if err := validateEmptyCheckpointOutput("relative"); err == nil {
			t.Fatal("expected relative output path to fail")
		}

		if err := os.WriteFile(filepath.Join(output, "existing"), []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}

		if err := validateEmptyCheckpointOutput(output); err == nil {
			t.Fatal("expected non-empty output directory to fail")
		}
	})

	t.Run("symlink output", func(t *testing.T) {
		root := t.TempDir()

		target := filepath.Join(root, "target")
		if err := os.Mkdir(target, 0o700); err != nil {
			t.Fatal(err)
		}

		link := filepath.Join(root, "link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}

		if err := validateEmptyCheckpointOutput(link); err == nil {
			t.Fatal("expected symlink output directory to fail")
		}
	})

	t.Run("metadata round trip and strict decoding", func(t *testing.T) {
		dir := t.TempDir()

		config := &types.ContainerConfig{
			Metadata: &types.ContainerMetadata{Name: "app"},
			Image:    &types.ImageSpec{Image: "registry.example/app:latest"},
			Command:  []string{"/app"},
			Envs:     []*types.KeyValue{{Key: "KEY", Value: "value"}},
		}
		if err := persistContainerConfig(dir, config); err != nil {
			t.Fatalf("persist container config: %v", err)
		}

		loaded, err := loadContainerConfig(dir)
		if err != nil {
			t.Fatalf("load container config: %v", err)
		}

		if !proto.Equal(config, loaded) {
			t.Fatalf("round trip mismatch: got %#v, want %#v", loaded, config)
		}

		if err := os.WriteFile(filepath.Join(dir, criContainerConfigFile), []byte(`{"metadata":{"name":"app"},"unknown":true}`), 0o600); err != nil {
			t.Fatal(err)
		}

		if _, err := loadContainerConfig(dir); err == nil {
			t.Fatal("expected unknown JSON field to fail")
		}
	})

	t.Run("container reservation is atomic", func(t *testing.T) {
		server := new(Server)

		release, err := server.reserveContainerCheckpoints([]string{"b", "a"})
		if err != nil {
			t.Fatalf("reserve containers: %v", err)
		}

		if _, err := server.reserveContainerCheckpoints([]string{"a"}); err == nil {
			t.Fatal("expected duplicate reservation to fail")
		}

		release()

		releaseAgain, err := server.reserveContainerCheckpoints([]string{"a"})
		if err != nil {
			t.Fatalf("reserve released container: %v", err)
		}

		releaseAgain()
	})

	t.Run("restore config validation", func(t *testing.T) {
		checkpoint := &types.ContainerConfig{
			Image:      &types.ImageSpec{Image: "example/app:latest"},
			Command:    []string{"/app"},
			Args:       []string{"serve"},
			WorkingDir: "/work",
			Envs:       []*types.KeyValue{{Key: "A", Value: "B"}},
			Linux:      &types.LinuxContainerConfig{SecurityContext: &types.LinuxContainerSecurityContext{}},
		}

		restore := proto.CloneOf(checkpoint)
		if err := validateRestoreContainerConfig(checkpoint, restore); err != nil {
			t.Fatalf("validate identical restore config: %v", err)
		}

		restore.Command = []string{"/different"}
		if err := validateRestoreContainerConfig(checkpoint, restore); err == nil {
			t.Fatal("expected command mismatch to fail")
		}
	})
}
