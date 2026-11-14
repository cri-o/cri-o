package server

import (
	"testing"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/pkg/config"
)

// TestSetupSandboxSeccompPrivilegedEmptyProfileClearsDefaultFilter is a
// regression test for https://github.com/cri-o/cri-o/issues/9675.
//
// generate.New() injects a default-deny seccomp filter. setupSandboxSeccomp
// used to skip Setup() for privileged sandboxes when PrivilegedSeccompProfile
// is unset, without clearing Linux.Seccomp, so the sandbox stayed confined.
func TestSetupSandboxSeccompPrivilegedEmptyProfileClearsDefaultFilter(t *testing.T) {
	t.Parallel()

	g, err := generate.New("linux")
	if err != nil {
		t.Fatalf("generate.New: %v", err)
	}

	if g.Config == nil || g.Config.Linux == nil || g.Config.Linux.Seccomp == nil {
		t.Fatal("generate.New() did not inject a default seccomp filter; cannot reproduce #9675")
	}

	if g.Config.Linux.Seccomp.DefaultAction != rspec.ActErrno {
		t.Fatalf(
			"default filter action = %q, want %q",
			g.Config.Linux.Seccomp.DefaultAction,
			rspec.ActErrno,
		)
	}

	sut := &Server{config: &config.Config{}} // PrivilegedSeccompProfile is empty

	ref, err := sut.setupSandboxSeccomp(
		t.Context(),
		&g,
		"",
		true,
		&types.LinuxSandboxSecurityContext{},
	)
	if err != nil {
		t.Fatalf("setupSandboxSeccomp: %v", err)
	}

	if ref != types.SecurityProfile_Unconfined.String() {
		t.Errorf("seccomp ref = %q, want Unconfined", ref)
	}

	if g.Config.Linux.Seccomp != nil {
		t.Fatalf(
			"privileged sandbox with empty profile still has seccomp filter (defaultAction=%q); want nil/unconfined",
			g.Config.Linux.Seccomp.DefaultAction,
		)
	}
}

// TestRuntimeHandlerEmptyHandlerReturnsSnapshot checks that an empty runtime
// handler still returns the snapshot it resolves the default runtime from,
// so that all steps of one sandbox creation use the same default runtime even
// if a reload publishes a different one in between.
func TestRuntimeHandlerEmptyHandlerReturnsSnapshot(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	cfg.Runtimes = config.Runtimes{
		"runc": &config.RuntimeHandler{RuntimePath: "/bin/runc"},
		"crun": &config.RuntimeHandler{RuntimePath: "/bin/crun"},
	}
	cfg.DefaultRuntime = "runc"
	cfg.PublishRuntimeSnapshot()

	sut := &Server{config: cfg}

	handler, snapshot, err := sut.runtimeHandler(&types.RunPodSandboxRequest{})
	if err != nil {
		t.Fatalf("runtimeHandler: %v", err)
	}

	if handler != "" {
		t.Fatalf("handler = %q, want empty", handler)
	}

	if snapshot == nil {
		t.Fatal("runtimeHandler returned a nil snapshot for an empty handler")
	}

	// A reload changes the default runtime after the handler was resolved.
	cfg.DefaultRuntime = "crun"
	cfg.PublishRuntimeSnapshot()

	rh := snapshot.RuntimeHandler("")
	if rh == nil || rh.RuntimePath != "/bin/runc" {
		t.Fatalf("default runtime from the returned snapshot = %+v, want runc", rh)
	}
}
