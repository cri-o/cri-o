package server

import (
	"context"
	"testing"

	"github.com/cri-o/cri-o/internal/factory/container"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

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

	_, binds, err := addOCIBindMounts(context.Background(), ctr, "", "", nil, false, false, false, false, false, "")
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

	_, binds, err := addOCIBindMounts(context.Background(), ctr, "", "", nil, false, false, false, false, false, "")
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

	_, binds, err := addOCIBindMounts(ctx, ctr, "", "", nil, false, false, false, false, true, "")
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
		tc := tc
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

			_, _, err = addOCIBindMounts(ctx, ctr, "", "", nil, false, false, false, false, tc.rroSupport, "")
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
	_, _, err = addOCIBindMounts(context.Background(), ctr, "", "", nil, false, false, true, false, false, "")
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
	_, _, err = addOCIBindMounts(context.Background(), ctr, "", "", nil, false, false, false, false, false, "")
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
	_, _, err = addOCIBindMounts(context.Background(), ctr, "", "", nil, false, false, false, false, false, "")
	if err == nil {
		t.Errorf("Should have failed to create id mapped mount with no id map support")
	}

	_, _, err = addOCIBindMounts(context.Background(), ctr, "", "", nil, false, false, false, true, false, "")
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
