package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/factory/container"
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

			sut := &Server{}
			sut.config.Root = filepath.Join(prefix, "storage")
			sut.config.BindMountPrefix = prefix
			sut.config.AbsentMountSourcesToReject = []string{toReject}

			_, binds, err := sut.addOCIBindMounts(
				context.Background(),
				ctr,
				"",
				prefix,
				[]string{toReject},
				false,
				false,
				false,
				false,
				false,
				filepath.Join(prefix, "storage"),
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

	sut := &Server{}
	_, binds, err := sut.addOCIBindMounts(context.Background(), ctr, "", "", nil, false, false, false, false, false, "")
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

	sut := &Server{}
	_, binds, err := sut.addOCIBindMounts(context.Background(), ctr, "", "", nil, false, false, false, false, false, "")
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

func TestAddOCIBindsRROMounts(t *testing.T) {
	t.Parallel()

	const hostPath = "/mnt"

	ctr, err := container.New()
	if err != nil {
		t.Fatalf("Should create a container, got: %v", err)
	}

	err = ctr.SetConfig(&types.ContainerConfig{
		Mounts: []*types.Mount{
			{
				HostPath:          hostPath,
				ContainerPath:     "/host",
				Readonly:          true,
				RecursiveReadOnly: true,
				Propagation:       0,
			},
		},
		Metadata: &types.ContainerMetadata{
			Name: "test-container",
		},
	}, &types.PodSandboxConfig{
		Metadata: &types.PodSandboxMetadata{
			Name: "test-pod",
		},
	})
	if err != nil {
		t.Fatalf("Should set container configuration, got: %v", err)
	}

	ctx := context.TODO()

	sut := &Server{}
	_, binds, err := sut.addOCIBindMounts(ctx, ctr, "", "", nil, false, false, false, false, true, "")
	if err != nil {
		t.Errorf("Should not fail to create RRO mount, got: %v", err)
	}

	hasRRO := false
	for _, m := range binds {
		if m.Source == hostPath {
			for _, o := range m.Options {
				if o == "rro" {
					hasRRO = true
				}
			}
		}
	}

	if !hasRRO {
		t.Errorf("Should add an RRO mount to be created, got: %#v", binds)
	}
}

func TestAddOCIBindsRROMountsError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		description string
		rroSupport  bool
		given       *types.Mount
		want        string
	}{
		{
			"should fail to add an RRO mount without RRO mounts support",
			false,
			&types.Mount{
				HostPath:          "/mnt",
				ContainerPath:     "/host",
				Readonly:          true,
				RecursiveReadOnly: true,
				Propagation:       0,
			},
			`recursive read-only mount support is not available for hostPath "/mnt"`,
		},
		{
			"should fail to add an RRO mount without readonly option",
			true,
			&types.Mount{
				HostPath:          "/mnt",
				ContainerPath:     "/host",
				Readonly:          false,
				RecursiveReadOnly: true,
				Propagation:       0,
			},
			`recursive read-only mount conflicts with read-write mount for hostPath "/mnt"`,
		},
		{
			"should fail to add an RRO mount without private propagation",
			true,
			&types.Mount{
				HostPath:          "/mnt",
				ContainerPath:     "/host",
				Readonly:          true,
				RecursiveReadOnly: true,
				Propagation:       2,
			},
			`recursive read-only mount requires private propagation for hostPath "/mnt", got: PROPAGATION_BIDIRECTIONAL`,
		},
	}

	ctx := context.TODO()

	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()

			ctr, err := container.New()
			if err != nil {
				t.Fatalf("Should create a container, got: %v", err)
			}

			err = ctr.SetConfig(&types.ContainerConfig{
				Mounts: []*types.Mount{
					tc.given,
				},
				Metadata: &types.ContainerMetadata{
					Name: "test-container",
				},
			}, &types.PodSandboxConfig{
				Metadata: &types.PodSandboxMetadata{
					Name: "test-pod",
				},
			})
			if err != nil {
				t.Fatalf("Should set container configuration, got: %v", err)
			}

			sut := &Server{}
			_, _, err = sut.addOCIBindMounts(ctx, ctr, "", "", nil, false, false, false, false, tc.rroSupport, "")
			if err == nil {
				t.Error("Should fail to add an RRO mount with a specific error")
			}

			if tc.want != err.Error() {
				t.Errorf("Should fail to add an RRO mount with error %s, got %v", tc.want, err)
			}
		})
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
	sut := &Server{}
	_, _, err = sut.addOCIBindMounts(context.Background(), ctr, "", "", nil, false, false, true, false, false, "")
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
	_, _, err = sut.addOCIBindMounts(context.Background(), ctr, "", "", nil, false, false, false, false, false, "")
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

func TestAddOCIBindsErrorWithoutIDMap(t *testing.T) {
	ctr, err := container.New()
	if err != nil {
		t.Fatal(err)
	}

	if err := ctr.SetConfig(&types.ContainerConfig{
		Mounts: []*types.Mount{
			{
				ContainerPath: "/sys",
				HostPath:      "/sys",
				UidMappings: []*types.IDMapping{
					{
						HostId:      1000,
						ContainerId: 1,
						Length:      1000,
					},
				},
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
		t.Fatal(err)
	}
	sut := &Server{}
	_, _, err = sut.addOCIBindMounts(context.Background(), ctr, "", "", nil, false, false, false, false, false, "")
	if err == nil {
		t.Errorf("Should have failed to create id mapped mount with no id map support")
	}

	_, _, err = sut.addOCIBindMounts(context.Background(), ctr, "", "", nil, false, false, false, true, false, "")
	if err != nil {
		t.Errorf("%v", err)
	}
}

func TestIsSubDirectoryOf(t *testing.T) {
	tests := []struct {
		base, target string
		want         bool
	}{
		{"/var/lib/containers/storage", "/", true},
		{"/var/lib/containers/storage", "/var/lib", true},
		{"/var/lib/containers/storage", "/var/lib/containers", true},
		{"/var/lib/containers/storage", "/var/lib/containers/storage", true},
		{"/var/lib/containers/storage", "/var/lib/containers/storage/extra", false},
		{"/var/lib/containers/storage", "/va", false},
		{"/var/lib/containers/storage", "/var/tmp/containers", false},
	}

	for _, tt := range tests {
		testname := tt.base + " " + tt.target
		t.Run(testname, func(t *testing.T) {
			res := isSubDirectoryOf(tt.base, tt.target)
			if res != tt.want {
				t.Errorf("got %v, want %v", res, tt.want)
			}
		})
	}
}
