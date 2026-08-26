package server

import (
	"strings"
	"testing"
	"time"

	"github.com/opencontainers/runtime-tools/generate"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/factory/container"
	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/memorystore"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/storage"
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

	sut := &Server{}
	ctrInfo := &storage.ContainerInfo{
		MountLabel: "",
	}

	_, binds, _, err := sut.addOCIBindMounts(t.Context(), ctr, ctrInfo, false, false, false, false, false)
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
	ctrInfo := &storage.ContainerInfo{
		MountLabel: "",
	}

	_, binds, _, err := sut.addOCIBindMounts(t.Context(), ctr, ctrInfo, false, false, false, false, false)
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

	ctx := t.Context()

	sut := &Server{}
	ctrInfo := &storage.ContainerInfo{
		MountLabel: "",
	}

	_, binds, _, err := sut.addOCIBindMounts(ctx, ctr, ctrInfo, false, false, false, false, true)
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

	ctx := t.Context()

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
			ctrInfo := &storage.ContainerInfo{
				MountLabel: "",
			}

			_, _, _, err = sut.addOCIBindMounts(ctx, ctr, ctrInfo, false, false, false, false, tc.rroSupport)
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
	ctrInfo := &storage.ContainerInfo{
		MountLabel: "",
	}

	//nolint:dogsled // test only needs the error return
	_, _, _, err = sut.addOCIBindMounts(t.Context(), ctr, ctrInfo, false, false, true, false, false)
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

	//nolint:dogsled // test only needs the error return
	_, _, _, err = sut.addOCIBindMounts(t.Context(), ctr, ctrInfo, false, false, false, false, false)
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
	ctrInfo := &storage.ContainerInfo{
		MountLabel: "",
	}

	//nolint:dogsled // test only needs the error return
	_, _, _, err = sut.addOCIBindMounts(t.Context(), ctr, ctrInfo, false, false, false, false, false)
	if err == nil {
		t.Errorf("Should have failed to create id mapped mount with no id map support")
	}

	//nolint:dogsled // test only needs the error return
	_, _, _, err = sut.addOCIBindMounts(t.Context(), ctr, ctrInfo, false, false, false, true, false)
	if err != nil {
		t.Errorf("%v", err)
	}
}

func TestSetupContainerUserRejectsNewlineInHOME(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		homeVal string
		wantErr bool
	}{
		{
			name:    "actual newline byte in HOME",
			homeVal: "/root\nmalicious::0:0::/:/bin/bash",
			wantErr: true,
		},
		{
			name:    "carriage return in HOME",
			homeVal: "/root\r\nmalicious::0:0::/:/bin/bash",
			wantErr: true,
		},
		{
			name:    "valid HOME value",
			homeVal: "/home/user",
			wantErr: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			specgen, err := generate.New("linux")
			if err != nil {
				t.Fatalf("failed to create spec generator: %v", err)
			}

			specgen.AddProcessEnv("HOME", tc.homeVal)

			sc := &types.LinuxContainerSecurityContext{
				RunAsUser: &types.Int64Value{Value: 1000},
			}

			err = setupContainerUser(t.Context(), &specgen, t.TempDir(), "", t.TempDir(), sc, nil)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error for HOME with newline, got nil")
				}

				if !strings.Contains(err.Error(), "newline") {
					t.Errorf("expected newline-related error, got: %v", err)
				}
			} else if err != nil {
				// Valid HOME may still fail on missing rootfs files; that's fine.
				// Only fail if the error is the newline check.
				if strings.Contains(err.Error(), "newline") {
					t.Errorf("unexpected newline error for valid HOME: %v", err)
				}
			}
		})
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

// newTestSandbox builds the minimal valid *sandbox.Sandbox that
// configureSELinuxLabels needs (it only reads sb.Annotations()).
func newTestSandbox(t *testing.T) *sandbox.Sandbox {
	t.Helper()

	b := sandbox.NewBuilder()
	b.SetID("sandboxid")
	b.SetName("sandboxname")
	b.SetLogDir(t.TempDir())
	b.SetShmPath("test")
	b.SetNamespace("")
	b.SetKubeName("")
	b.SetMountLabel("")
	b.SetProcessLabel("")
	b.SetCgroupParent("")
	b.SetRuntimeHandler("")
	b.SetResolvPath("")
	b.SetHostname("")
	b.SetPortMappings([]*hostport.PortMapping{})
	b.SetHostNetwork(false)
	b.SetUsernsMode("")
	b.SetPodLinuxOverhead(nil)
	b.SetPodLinuxResources(nil)
	b.SetCreatedAt(time.Now())

	if err := b.SetCRISandbox(b.ID(), map[string]string{}, map[string]string{}, &types.PodSandboxMetadata{}); err != nil {
		t.Fatalf("SetCRISandbox: %v", err)
	}

	b.SetPrivileged(false)
	b.SetContainers(memorystore.New[*oci.Container]())

	sb, err := b.GetSandbox()
	if err != nil {
		t.Fatalf("GetSandbox: %v", err)
	}

	return sb
}

// newSELinuxTestContainer builds a container.Container whose configured process
// is either a systemd/init entrypoint or a regular command, with the given
// container-level and pod-level SELinux types (empty string means unset).
func newSELinuxTestContainer(t *testing.T, systemd bool, containerSelinuxType, podSelinuxType string) container.Container {
	t.Helper()

	command := []string{"/bin/sh"}
	if systemd {
		command = []string{"/sbin/init"}
	}

	cfg := &types.ContainerConfig{
		Metadata: &types.ContainerMetadata{Name: "testctr"},
		Command:  command,
		Linux: &types.LinuxContainerConfig{
			SecurityContext: &types.LinuxContainerSecurityContext{
				NamespaceOptions: &types.NamespaceOption{},
				SelinuxOptions:   &types.SELinuxOption{Type: containerSelinuxType},
			},
		},
	}

	sboxCfg := &types.PodSandboxConfig{
		Metadata: &types.PodSandboxMetadata{Name: "testpod"},
		Linux: &types.LinuxPodSandboxConfig{
			SecurityContext: &types.LinuxSandboxSecurityContext{
				SelinuxOptions: &types.SELinuxOption{Type: podSelinuxType},
			},
		},
	}

	ctr, err := container.New()
	if err != nil {
		t.Fatalf("container.New: %v", err)
	}

	if err := ctr.SetConfig(cfg, sboxCfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	if err := ctr.SpecSetProcessArgs(nil); err != nil {
		t.Fatalf("SpecSetProcessArgs: %v", err)
	}

	return ctr
}

func TestConfigureSELinuxLabels(t *testing.T) {
	const originalProcessLabel = "system_u:system_r:container_t:s0:c1,c2"

	cases := []struct {
		name                  string
		systemd               bool
		containerSelinuxType  string
		podSelinuxType        string
		wantProcessLabelUnset bool // true: InitLabel must NOT fire, processLabel stays as-is
		wantSkipRelabel       bool
	}{
		{
			name:                  "systemd, no type requested anywhere: promoted to init label",
			systemd:               true,
			wantProcessLabelUnset: false,
			wantSkipRelabel:       false,
		},
		{
			name:                  "systemd, explicit non-spc_t container type: respected",
			systemd:               true,
			containerSelinuxType:  "container_t",
			wantProcessLabelUnset: true,
			wantSkipRelabel:       false,
		},
		{
			name:                  "systemd, explicit spc_t container type: respected and skips relabel",
			systemd:               true,
			containerSelinuxType:  "spc_t",
			wantProcessLabelUnset: true,
			wantSkipRelabel:       true,
		},
		{
			name:                  "systemd, spc_t requested only at pod level: respected and skips relabel",
			systemd:               true,
			podSelinuxType:        "spc_t",
			wantProcessLabelUnset: true,
			wantSkipRelabel:       true,
		},
		{
			name:                  "systemd, container-level type overrides pod-level spc_t",
			systemd:               true,
			containerSelinuxType:  "container_t",
			podSelinuxType:        "spc_t",
			wantProcessLabelUnset: true,
			wantSkipRelabel:       false,
		},
		{
			name:                  "non-systemd container, no type requested: never promoted",
			systemd:               false,
			wantProcessLabelUnset: true,
			wantSkipRelabel:       false,
		},
	}

	sb := newTestSandbox(t)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctr := newSELinuxTestContainer(t, tc.systemd, tc.containerSelinuxType, tc.podSelinuxType)
			containerInfo := &storage.ContainerInfo{ProcessLabel: originalProcessLabel}

			s := &Server{}

			_, processLabel, _, skipRelabel, err := s.configureSELinuxLabels(ctr, sb, containerInfo)
			if err != nil {
				t.Fatalf("configureSELinuxLabels: %v", err)
			}

			gotUnset := processLabel == originalProcessLabel
			if gotUnset != tc.wantProcessLabelUnset {
				t.Errorf("processLabel = %q (unchanged=%v), want unchanged=%v", processLabel, gotUnset, tc.wantProcessLabelUnset)
			}

			if skipRelabel != tc.wantSkipRelabel {
				t.Errorf("skipRelabel = %v, want %v", skipRelabel, tc.wantSkipRelabel)
			}
		})
	}
}
