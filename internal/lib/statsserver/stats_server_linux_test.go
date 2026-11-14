package statsserver

import (
	"context"
	"errors"
	"testing"
	"time"

	specs "github.com/opencontainers/runtime-spec/specs-go"
	cstorage "go.podman.io/storage"
	drivers "go.podman.io/storage/drivers"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/lib/stats"
	"github.com/cri-o/cri-o/internal/memorystore"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/pkg/config"
)

type fakeRuntimeImpl struct {
	oci.RuntimeImpl

	cgroupStats      *stats.CgroupStats
	cgroupStatsErr   error
	cgroupStatsCalls int
	diskStats        *stats.DiskStats
	diskStatsErr     error
	diskStatsCalls   int
}

func (f *fakeRuntimeImpl) CgroupStats(
	_ context.Context,
	_ *oci.Container,
	_ string,
) (*stats.CgroupStats, error) {
	f.cgroupStatsCalls++

	return f.cgroupStats, f.cgroupStatsErr
}

func (f *fakeRuntimeImpl) DiskStats(
	_ context.Context,
	_ *oci.Container,
	_ string,
) (*stats.DiskStats, error) {
	f.diskStatsCalls++

	return f.diskStats, f.diskStatsErr
}

type fakeStore struct {
	cstorage.Store
}

func (f *fakeStore) GraphDriver() (drivers.Driver, error) {
	return nil, errors.New("no graph driver in test")
}

type fakeParentServer struct {
	runtime *oci.Runtime
	cfg     *config.Config
}

func (f *fakeParentServer) Runtime() *oci.Runtime              { return f.runtime }
func (f *fakeParentServer) Store() cstorage.Store              { return &fakeStore{} }
func (f *fakeParentServer) ListSandboxes() []*sandbox.Sandbox  { return nil }
func (f *fakeParentServer) GetSandbox(string) *sandbox.Sandbox { return nil }
func (f *fakeParentServer) Config() *config.Config             { return f.cfg }

func newTestSandbox(t *testing.T) *sandbox.Sandbox {
	t.Helper()

	const id = "sb-1"

	b := sandbox.NewBuilder()
	b.SetID(id)
	b.SetName("test-sandbox")
	b.SetLogDir("/tmp")
	b.SetShmPath("/dev/shm")
	b.SetNamespace("default")
	b.SetKubeName("test-pod")
	b.SetMountLabel("")
	b.SetProcessLabel("")
	b.SetCgroupParent("/kubepods/test")
	b.SetRuntimeHandler("runc")
	b.SetResolvPath("/etc/resolv.conf")
	b.SetHostname("test-host")
	b.SetPortMappings(nil)
	b.SetPrivileged(false)
	b.SetHostNetwork(false)
	b.SetUsernsMode("")
	b.SetPodLinuxOverhead(nil)
	b.SetPodLinuxResources(nil)
	b.SetCreatedAt(time.Now())

	if err := b.SetCRISandbox(
		id,
		nil,
		nil,
		&types.PodSandboxMetadata{Name: "test-pod", Namespace: "default"},
	); err != nil {
		t.Fatalf("SetCRISandbox: %v", err)
	}

	containers := memorystore.New[*oci.Container]()
	b.SetContainers(containers)

	sb, err := b.GetSandbox()
	if err != nil {
		t.Fatalf("GetSandbox: %v", err)
	}

	return sb
}

func newRunningContainer(t *testing.T) *oci.Container {
	t.Helper()

	const (
		id   = "ctr-1"
		name = "test-container"
	)

	ctr, err := oci.NewContainer(
		id, name, "", "", nil, nil, nil,
		"", nil, nil, "", &types.ContainerMetadata{Name: name}, "",
		false, false, false, "", "", time.Now(), "",
	)
	if err != nil {
		t.Fatalf("NewContainer: %v", err)
	}

	ctr.SetStateAndSpoofPid(&oci.ContainerState{
		State: specs.State{Status: oci.ContainerStateRunning},
	})

	return ctr
}

func newTestStatsServer(t *testing.T, rt *oci.Runtime, cfg *config.Config) *StatsServer {
	t.Helper()

	return &StatsServer{
		parentServerIface: &fakeParentServer{runtime: rt, cfg: cfg},
		sboxStats:         make(map[string]*types.PodSandboxStats),
		ctrStats:          make(map[string]*types.ContainerStats),
		sboxMetrics:       make(map[string]*SandboxMetrics),
		ctx:               context.Background(),
	}
}

func TestUpdateSandboxContainerStatsError(t *testing.T) {
	t.Parallel()

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	rt := oci.NewTestRuntime()
	ctr := newRunningContainer(t)

	rt.SetRuntimeImpl("ctr-1", &fakeRuntimeImpl{
		cgroupStatsErr: errors.New("cgroup deleted"),
	})

	sb := newTestSandbox(t)
	sb.AddContainer(context.Background(), ctr)

	ss := newTestStatsServer(t, rt, cfg)
	result := ss.updateSandbox(sb)

	if result == nil {
		t.Fatal("updateSandbox returned nil")
	}

	if len(result.GetLinux().GetContainers()) != 0 {
		t.Errorf(
			"expected 0 container stats (error should skip), got %d",
			len(result.GetLinux().GetContainers()),
		)
	}
}

func TestUpdateSandboxDiskStatsError(t *testing.T) {
	t.Parallel()

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	rt := oci.NewTestRuntime()
	ctr := newRunningContainer(t)

	rt.SetRuntimeImpl("ctr-1", &fakeRuntimeImpl{
		cgroupStats:  &stats.CgroupStats{SystemNano: time.Now().UnixNano()},
		diskStatsErr: errors.New("disk stats unavailable"),
	})

	sb := newTestSandbox(t)
	sb.AddContainer(context.Background(), ctr)

	ss := newTestStatsServer(t, rt, cfg)
	result := ss.updateSandbox(sb)

	if result == nil {
		t.Fatal("updateSandbox returned nil")
	}

	if len(result.GetLinux().GetContainers()) != 1 {
		t.Fatalf(
			"expected 1 container stats despite disk error, got %d",
			len(result.GetLinux().GetContainers()),
		)
	}

	if result.GetLinux().GetContainers()[0].GetCpu() == nil {
		t.Error("expected CPU stats to be present")
	}

	if result.GetLinux().GetContainers()[0].GetMemory() == nil {
		t.Error("expected memory stats to be present")
	}
}

func testConfigWithMetrics(t *testing.T, metrics ...string) *config.Config {
	t.Helper()

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	cfg.IncludedPodMetrics = metrics //nolint:staticcheck // user-input field is the only way to seed EnabledPodMetrics via Validate
	if err := cfg.StatsConfig.Validate(); err != nil {
		t.Fatalf("StatsConfig.Validate: %v", err)
	}

	return cfg
}

func TestGenerateSandboxContainerMetricsSkipsCollectionWhenDisabled(t *testing.T) {
	t.Parallel()

	rt := oci.NewTestRuntime()
	ctr := newRunningContainer(t)

	fake := &fakeRuntimeImpl{
		cgroupStats: &stats.CgroupStats{SystemNano: time.Now().UnixNano()},
		diskStats:   &stats.DiskStats{},
	}
	rt.SetRuntimeImpl("ctr-1", fake)

	sb := newTestSandbox(t)
	ss := newTestStatsServer(t, rt, testConfigWithMetrics(t, config.SpecMetrics))

	result := ss.GenerateSandboxContainerMetrics(sb, ctr, NewSandboxMetrics(sb))

	if result == nil {
		t.Fatal("GenerateSandboxContainerMetrics returned nil")
	}

	if fake.cgroupStatsCalls != 0 {
		t.Errorf("expected no cgroup stats collection, got %d calls", fake.cgroupStatsCalls)
	}

	if fake.diskStatsCalls != 0 {
		t.Errorf("expected no disk stats collection, got %d calls", fake.diskStatsCalls)
	}
}

func TestGenerateSandboxContainerMetricsCollectsWhenEnabled(t *testing.T) {
	t.Parallel()

	rt := oci.NewTestRuntime()
	ctr := newRunningContainer(t)

	fake := &fakeRuntimeImpl{
		cgroupStats: &stats.CgroupStats{SystemNano: time.Now().UnixNano()},
		diskStats:   &stats.DiskStats{},
	}
	rt.SetRuntimeImpl("ctr-1", fake)

	sb := newTestSandbox(t)
	ss := newTestStatsServer(t, rt, testConfigWithMetrics(t, config.CPUMetrics, config.DiskMetrics))

	result := ss.GenerateSandboxContainerMetrics(sb, ctr, NewSandboxMetrics(sb))

	if result == nil {
		t.Fatal("GenerateSandboxContainerMetrics returned nil")
	}

	if fake.cgroupStatsCalls != 1 {
		t.Errorf("expected cgroup stats collected once, got %d calls", fake.cgroupStatsCalls)
	}

	if fake.diskStatsCalls != 1 {
		t.Errorf("expected disk stats collected once, got %d calls", fake.diskStatsCalls)
	}
}

func TestUpdateSandboxEmitsLastSeenWhenMetricsDisabled(t *testing.T) {
	t.Parallel()

	rt := oci.NewTestRuntime()
	ctr := newRunningContainer(t)

	rt.SetRuntimeImpl("ctr-1", &fakeRuntimeImpl{
		cgroupStats: &stats.CgroupStats{SystemNano: time.Now().UnixNano()},
		diskStats:   &stats.DiskStats{},
	})

	sb := newTestSandbox(t)
	sb.AddContainer(context.Background(), ctr)

	ss := newTestStatsServer(t, rt, testConfigWithMetrics(t))
	ss.updateSandbox(sb)

	sm := ss.sboxMetrics["sb-1"]
	if sm == nil {
		t.Fatal("expected sandbox metrics entry")
	}

	if len(sm.metric.GetContainerMetrics()) != 1 {
		t.Fatalf("expected 1 container metrics entry, got %d", len(sm.metric.GetContainerMetrics()))
	}

	metrics := sm.metric.GetContainerMetrics()[0].GetMetrics()
	if len(metrics) != 1 {
		t.Fatalf("expected only container_last_seen, got %d metrics", len(metrics))
	}

	if metrics[0].GetName() != "container_last_seen" {
		t.Errorf("expected container_last_seen, got %q", metrics[0].GetName())
	}
}

func TestUpdateSandboxGeneratesMetricsWhenEnabled(t *testing.T) {
	t.Parallel()

	rt := oci.NewTestRuntime()
	ctr := newRunningContainer(t)

	rt.SetRuntimeImpl("ctr-1", &fakeRuntimeImpl{
		cgroupStats: &stats.CgroupStats{SystemNano: time.Now().UnixNano()},
		diskStats:   &stats.DiskStats{},
	})

	sb := newTestSandbox(t)
	sb.AddContainer(context.Background(), ctr)

	ss := newTestStatsServer(t, rt, testConfigWithMetrics(t, config.CPUMetrics))
	ss.updateSandbox(sb)

	sm := ss.sboxMetrics["sb-1"]
	if sm == nil {
		t.Fatal("expected sandbox metrics entry")
	}

	if len(sm.metric.GetContainerMetrics()) != 1 {
		t.Fatalf("expected 1 container metrics entry, got %d", len(sm.metric.GetContainerMetrics()))
	}
}

// TestContainerMetricsFromContainerStatsNilSafety enforces that each metric's
// declared source in metricDefinitions matches the stats its generator reads.
// For each available metric it supplies the stats arguments exactly as
// GenerateSandboxContainerMetrics would (nil when CgroupStatsEnabled /
// DiskStatsEnabled report the stats are not needed), then generates the metric.
// A generator reading cgroup stats while declaring a non-cgroup source is passed
// a nil cgroupStats and panics, failing this test.
func TestContainerMetricsFromContainerStatsNilSafety(t *testing.T) {
	t.Parallel()

	for _, m := range config.AvailableMetrics {
		t.Run(m, func(t *testing.T) {
			t.Parallel()

			var cgroupStats *stats.CgroupStats
			if CgroupStatsEnabled([]string{m}) {
				cgroupStats = &stats.CgroupStats{}
			}

			var diskStats *stats.DiskStats
			if DiskStatsEnabled([]string{m}) {
				diskStats = &stats.DiskStats{}
			}

			rt := oci.NewTestRuntime()
			rt.SetRuntimeImpl("test-id", &fakeRuntimeImpl{})

			sb := newTestSandbox(t)
			ss := newTestStatsServer(t, rt, testConfigWithMetrics(t, m))

			defer func() {
				if r := recover(); r != nil {
					t.Fatalf(
						"metric %q read stats skipped by the collection gate; its source in metricDefinitions must match the stats it reads: %v",
						m,
						r,
					)
				}
			}()

			if result := ss.containerMetricsFromContainerStats(
				sb,
				newTestContainer(t),
				cgroupStats,
				diskStats,
			); result == nil {
				t.Fatal("expected non-nil container metrics")
			}
		})
	}
}
