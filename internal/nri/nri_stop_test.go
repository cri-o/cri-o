package nri_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	nriadapt "github.com/containerd/nri/pkg/adaptation"

	config "github.com/cri-o/cri-o/internal/config/nri"
	nrilib "github.com/cri-o/cri-o/internal/nri"
)

// stubDomain satisfies the NRI domain with empty state so plugin
// synchronization during Start has nothing to apply.
type stubDomain struct{}

func (stubDomain) GetName() string { return "test" }

func (stubDomain) ListPodSandboxes(context.Context) []nrilib.PodSandbox { return nil }
func (stubDomain) ListContainers() []nrilib.Container                   { return nil }

func (stubDomain) GetPodSandbox(context.Context, string) (nrilib.PodSandbox, bool) {
	return nil, false
}

func (stubDomain) GetContainer(string) (nrilib.Container, bool) { return nil, false }

func (stubDomain) UpdateContainer(context.Context, *nriadapt.ContainerUpdate) error {
	return nil
}

func (stubDomain) EvictContainer(context.Context, *nriadapt.ContainerEviction) error {
	return nil
}

// testConfig returns an enabled NRI config rooted at dir, so the test never
// touches the host default socket or plugin directories.
func testConfig(dir string) *config.Config {
	cfg := config.New()
	cfg.Enabled = true
	cfg.SocketPath = filepath.Join(dir, "nri.sock")
	cfg.PluginPath = filepath.Join(dir, "plugins")
	cfg.PluginConfigPath = filepath.Join(dir, "plugin-config")

	return cfg
}

func dialSocket(t *testing.T, path string) error {
	t.Helper()

	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		return err
	}

	conn.Close()

	return nil
}

// TestStopReleasesListener proves the shutdown leak mechanism: while the
// adaptation runs its socket is servable, so a Shutdown that never calls
// Stop leaves the old listener accepting. Stop must close the listener
// (dial refuses afterwards), be safe to call twice (Shutdown and
// constructor rollback can both reach it), and still allow a same-process
// restart to bind the same pathname.
func TestStopReleasesListener(t *testing.T) {
	nrilib.SetDomain(stubDomain{})

	dir := t.TempDir()
	cfg := testConfig(dir)

	api, err := nrilib.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := api.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// While started the socket must be servable: this is the leaked state
	// a Shutdown without Stop would leave behind.
	if err := dialSocket(t, cfg.SocketPath); err != nil {
		t.Fatalf("dial while started: %v", err)
	}

	api.Stop()

	// After Stop no listener may serve the pathname anymore.
	if err := dialSocket(t, cfg.SocketPath); err == nil {
		t.Fatal("dial succeeded after Stop, listener was not released")
	}

	// Second Stop must be a safe no-op (upstream Stop is idempotent and the
	// local adaptation pointer is retained).
	api.Stop()

	// A same-process restart rebinds the same pathname and serves again.
	restarted, err := nrilib.New(cfg)
	if err != nil {
		t.Fatalf("New for restart: %v", err)
	}

	if err := restarted.Start(); err != nil {
		t.Fatalf("restart Start: %v", err)
	}

	if err := dialSocket(t, cfg.SocketPath); err != nil {
		t.Fatalf("dial after restart: %v", err)
	}

	restarted.Stop()

	if err := dialSocket(t, cfg.SocketPath); err == nil {
		t.Fatal("dial succeeded after restart Stop, listener was not released")
	}
}

// stubPodSandbox and stubLinuxPodSandbox provide the minimal PodSandbox
// surface needed to relay one event after Stop: with no plugins connected
// the adaptation has nothing to fan out to, so the call must return nil
// instead of panicking on a cleared adaptation pointer.
type stubPodSandbox struct{}

func (stubPodSandbox) GetDomain() string                 { return "test" }
func (stubPodSandbox) GetID() string                     { return "pod-id" }
func (stubPodSandbox) GetName() string                   { return "pod" }
func (stubPodSandbox) GetUID() string                    { return "uid" }
func (stubPodSandbox) GetNamespace() string              { return "ns" }
func (stubPodSandbox) GetLabels() map[string]string      { return nil }
func (stubPodSandbox) GetAnnotations() map[string]string { return nil }
func (stubPodSandbox) GetRuntimeHandler() string         { return "" }
func (stubPodSandbox) GetPid() uint32                    { return 0 }
func (stubPodSandbox) GetIPs() []string                  { return nil }

func (stubPodSandbox) GetLinuxPodSandbox() nrilib.LinuxPodSandbox {
	return stubLinuxPodSandbox{}
}

type stubLinuxPodSandbox struct{}

func (stubLinuxPodSandbox) GetLinuxNamespaces() []*nriadapt.LinuxNamespace { return nil }
func (stubLinuxPodSandbox) GetPodLinuxOverhead() *nriadapt.LinuxResources  { return nil }
func (stubLinuxPodSandbox) GetPodLinuxResources() *nriadapt.LinuxResources {
	return nil
}
func (stubLinuxPodSandbox) GetCgroupParent() string                     { return "" }
func (stubLinuxPodSandbox) GetCgroupsPath() string                      { return "" }
func (stubLinuxPodSandbox) GetLinuxResources() *nriadapt.LinuxResources { return nil }

// TestEventAfterStopDoesNotPanic covers the racy in-flight CRI call: an
// event relayed after Stop already passed IsEnabled must still resolve the
// adaptation instead of dereferencing a nil pointer.
func TestEventAfterStopDoesNotPanic(t *testing.T) {
	nrilib.SetDomain(stubDomain{})

	dir := t.TempDir()
	cfg := testConfig(dir)

	api, err := nrilib.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := api.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	api.Stop()

	// Must not panic; with no plugins connected there is nothing to fan
	// out to, so the relay succeeds trivially.
	if err := api.StopPodSandbox(context.Background(), stubPodSandbox{}); err != nil {
		t.Fatalf("StopPodSandbox after Stop: %v", err)
	}
}

// TestStopDisabledIsNoOp keeps the disabled configuration a safe no-op
// across the same Start/Stop/Stop sequence Shutdown may invoke.
func TestStopDisabledIsNoOp(t *testing.T) {
	cfg := config.New()
	cfg.Enabled = false

	api, err := nrilib.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := api.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	api.Stop()
	api.Stop()

	if _, err := os.Stat(cfg.SocketPath); !os.IsNotExist(err) {
		t.Fatalf("disabled NRI must not create a socket, stat err: %v", err)
	}
}
