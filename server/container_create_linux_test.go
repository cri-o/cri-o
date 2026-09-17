package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cri-o/cri-o/internal/factory/container"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func TestAddOCIBindsRejectAbsentSources(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		hostPath   string
		rejectPath string
		unprefixed bool
		exists     bool
		wantReject bool
	}{
		{name: "original path through intermediate symlink", rejectPath: "etc/hostname", wantReject: true},
		{name: "resolved path through intermediate symlink", rejectPath: "real-etc/hostname", wantReject: true},
		{name: "unprefixed original path", rejectPath: "etc/hostname", unprefixed: true},
		{name: "unprefixed resolved path", rejectPath: "real-etc/hostname", unprefixed: true},
		{name: "existing source is allowed", rejectPath: "etc/hostname", exists: true},
		{
			name:       "parent traversal matches resolved source",
			hostPath:   "../etc/hostname",
			rejectPath: "real-etc/hostname",
			wantReject: true,
		},
		{
			name:       "traversal beyond prefix depth matches resolved source",
			hostPath:   "../../../../../../etc/hostname",
			rejectPath: "real-etc/hostname",
			wantReject: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			prefix := t.TempDir()
			if err := os.Mkdir(filepath.Join(prefix, "real-etc"), 0o755); err != nil {
				t.Fatal(err)
			}

			// The requested etc/hostname and resolved real-etc/hostname refer to
			// the same source; rejection must work with either prefixed spelling.
			if err := os.Symlink("real-etc", filepath.Join(prefix, "etc")); err != nil {
				t.Fatal(err)
			}

			src := filepath.Join(prefix, "real-etc", "hostname")
			if tc.exists {
				if err := os.WriteFile(src, []byte("hostname"), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			// Reject entries use CRI-O's filesystem view, so unprefixed entries
			// must not match these sources beneath the bind mount prefix.
			toReject := filepath.Join(prefix, tc.rejectPath)
			if tc.unprefixed {
				toReject = filepath.Join(string(filepath.Separator), tc.rejectPath)
			}

			hostPath := tc.hostPath
			if hostPath == "" {
				hostPath = testHostnamePath
			}

			ctr, err := container.New()
			if err != nil {
				t.Fatal(err)
			}

			if err := ctr.SetConfig(&types.ContainerConfig{
				Metadata: &types.ContainerMetadata{Name: "testctr"},
				Mounts: []*types.Mount{{
					HostPath:      hostPath,
					ContainerPath: "/mnt/hostname",
				}},
			}, &types.PodSandboxConfig{
				Metadata: &types.PodSandboxMetadata{Name: "testpod"},
			}); err != nil {
				t.Fatal(err)
			}

			_, binds, err := addOCIBindMounts(
				context.Background(),
				ctr,
				"",
				prefix,
				[]string{toReject},
				false,
				false,
				false,
			)

			if tc.wantReject {
				wantError := "cannot mount " + toReject + ": path does not exist and will cause issues as a directory"
				if err == nil || err.Error() != wantError {
					t.Errorf("got error %v, want %q", err, wantError)
				}

				// Rejection must happen before the missing source is created as a directory.
				if _, err := os.Stat(src); !os.IsNotExist(err) {
					t.Errorf("rejected source must remain absent, got: %v", err)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if len(binds) != 1 || binds[0].Source != src {
				t.Errorf("expected bind source %q, got: %+v", src, binds)
			}

			info, err := os.Stat(src)
			if err != nil {
				t.Fatal(err)
			}

			// Allowed missing sources become directories; existing files stay files.
			if info.IsDir() == tc.exists {
				t.Errorf("source IsDir() = %v, want %v", info.IsDir(), !tc.exists)
			}
		})
	}
}

func TestAddOCIBindsForDev(t *testing.T) {
	ctr, err := container.New()
	if err != nil {
		t.Error(err)
	}
	if err := ctr.SetConfig(&types.ContainerConfig{
		Mounts: []*types.Mount{
			{
				ContainerPath: "/dev",
				HostPath:      "/dev",
			},
		},
		Metadata: &types.ContainerMetadata{
			Name: "testctr",
		},
	}, &types.PodSandboxConfig{
		Metadata: &types.PodSandboxMetadata{
			Name: "testpod",
		},
	}); err != nil {
		t.Error(err)
	}

	_, binds, err := addOCIBindMounts(context.Background(), ctr, "", "", nil, false, false, false)
	if err != nil {
		t.Error(err)
	}
	for _, m := range ctr.Spec().Mounts() {
		if m.Destination == "/dev" {
			t.Error("/dev shouldn't be in the spec if it's bind mounted from kube")
		}
	}
	var foundDev bool
	for _, b := range binds {
		if b.Destination == "/dev" {
			foundDev = true
			break
		}
	}
	if !foundDev {
		t.Error("no /dev mount found in spec mounts")
	}
}

func TestAddOCIBindsForSys(t *testing.T) {
	ctr, err := container.New()
	if err != nil {
		t.Error(err)
	}
	if err := ctr.SetConfig(&types.ContainerConfig{
		Mounts: []*types.Mount{
			{
				ContainerPath: "/sys",
				HostPath:      "/sys",
			},
		},
		Metadata: &types.ContainerMetadata{
			Name: "testctr",
		},
	}, &types.PodSandboxConfig{
		Metadata: &types.PodSandboxMetadata{
			Name: "testpod",
		},
	}); err != nil {
		t.Error(err)
	}

	_, binds, err := addOCIBindMounts(context.Background(), ctr, "", "", nil, false, false, false)
	if err != nil {
		t.Error(err)
	}
	var howManySys int
	for _, b := range binds {
		if b.Destination == "/sys" && b.Type != "sysfs" {
			howManySys++
		}
	}
	if howManySys != 1 {
		t.Error("there is not a single /sys bind mount")
	}
}

func TestAddOCIBindsCGroupRW(t *testing.T) {
	ctr, err := container.New()
	if err != nil {
		t.Error(err)
	}

	if err := ctr.SetConfig(&types.ContainerConfig{
		Metadata: &types.ContainerMetadata{
			Name: "testctr",
		},
	}, &types.PodSandboxConfig{
		Metadata: &types.PodSandboxMetadata{
			Name: "testpod",
		},
	}); err != nil {
		t.Error(err)
	}
	_, _, err = addOCIBindMounts(context.Background(), ctr, "", "", nil, false, false, true)
	if err != nil {
		t.Error(err)
	}
	var hasCgroupRW bool
	for _, m := range ctr.Spec().Mounts() {
		if m.Destination == "/sys/fs/cgroup" {
			for _, o := range m.Options {
				if o == "rw" {
					hasCgroupRW = true
				}
			}
		}
	}
	if !hasCgroupRW {
		t.Error("Cgroup mount not added with RW.")
	}

	ctr, err = container.New()
	if err != nil {
		t.Error(err)
	}
	if err := ctr.SetConfig(&types.ContainerConfig{
		Metadata: &types.ContainerMetadata{
			Name: "testctr",
		},
	}, &types.PodSandboxConfig{
		Metadata: &types.PodSandboxMetadata{
			Name: "testpod",
		},
	}); err != nil {
		t.Error(err)
	}
	var hasCgroupRO bool
	_, _, err = addOCIBindMounts(context.Background(), ctr, "", "", nil, false, false, false)
	if err != nil {
		t.Error(err)
	}
	for _, m := range ctr.Spec().Mounts() {
		if m.Destination == "/sys/fs/cgroup" {
			for _, o := range m.Options {
				if o == "ro" {
					hasCgroupRO = true
				}
			}
		}
	}
	if !hasCgroupRO {
		t.Error("Cgroup mount not added with RO.")
	}
}
