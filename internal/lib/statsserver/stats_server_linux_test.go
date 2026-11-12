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

	cgroupStats    *stats.CgroupStats
	cgroupStatsErr error
	diskStats      *stats.DiskStats
	diskStatsErr   error
	diskStatsCalls int
}

func (f *fakeRuntimeImpl) CgroupStats(
	_ context.Context,
	_ *oci.Container,
	_ string,
) (*stats.CgroupStats, error) {
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

func newTestSandbox(t *testing.T, id string) *sandbox.Sandbox {
	t.Helper()

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

func newRunningContainer(t *testing.T, id, name string) *oci.Container {
	t.Helper()

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
	ctr := newRunningContainer(t, "ctr-1", "test-container")

	rt.SetRuntimeImpl("ctr-1", &fakeRuntimeImpl{
		cgroupStatsErr: errors.New("cgroup deleted"),
	})

	sb := newTestSandbox(t, "sb-1")
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
	ctr := newRunningContainer(t, "ctr-1", "test-container")

	rt.SetRuntimeImpl("ctr-1", &fakeRuntimeImpl{
		cgroupStats:  &stats.CgroupStats{SystemNano: time.Now().UnixNano()},
		diskStatsErr: errors.New("disk stats unavailable"),
	})

	sb := newTestSandbox(t, "sb-1")
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

// TestUpdateContainerStatsDoesNotCollectDiskStats verifies that the container
// CRI stats path does not invoke DiskStats: populateWritableLayer always
// overwrites the writable layer with the graph driver numbers, so collecting
// disk stats there is wasted work.
func TestUpdateContainerStatsDoesNotCollectDiskStats(t *testing.T) {
	t.Parallel()

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	rt := oci.NewTestRuntime()
	ctr := newRunningContainer(t, "ctr-2", "disk-stats-container")

	fake := &fakeRuntimeImpl{
		cgroupStats: &stats.CgroupStats{SystemNano: time.Now().UnixNano()},
		diskStats:   &stats.DiskStats{},
	}
	rt.SetRuntimeImpl("ctr-2", fake)

	sb := newTestSandbox(t, "sb-2")

	ss := newTestStatsServer(t, rt, cfg)
	result := ss.updateContainerStats(ctr, sb)

	if result == nil {
		t.Fatal("updateContainerStats returned nil")
	}

	if fake.diskStatsCalls != 0 {
		t.Errorf(
			"expected no disk stats collection on the CRI stats path, got %d calls",
			fake.diskStatsCalls,
		)
	}

	if result.GetWritableLayer() == nil {
		t.Error("expected writable layer stats to be populated")
	}
}

// TestUpdateSandboxSkipsDiskStatsWhenDiskMetricsDisabled verifies that
// updateSandbox does not invoke DiskStats unless the disk pod metric is
// enabled: the CRI writable layer comes from the graph driver and the only
// other consumer is the disk metrics collector.
func TestUpdateSandboxSkipsDiskStatsWhenDiskMetricsDisabled(t *testing.T) {
	t.Parallel()

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	rt := oci.NewTestRuntime()
	ctr := newRunningContainer(t, "ctr-3", "no-disk-metrics-container")

	fake := &fakeRuntimeImpl{
		cgroupStats: &stats.CgroupStats{SystemNano: time.Now().UnixNano()},
		diskStats:   &stats.DiskStats{},
	}
	rt.SetRuntimeImpl("ctr-3", fake)

	sb := newTestSandbox(t, "sb-3")
	sb.AddContainer(context.Background(), ctr)

	ss := newTestStatsServer(t, rt, cfg)
	result := ss.updateSandbox(sb)

	if result == nil {
		t.Fatal("updateSandbox returned nil")
	}

	containers := result.GetLinux().GetContainers()
	if len(containers) != 1 {
		t.Fatalf("expected 1 container stats, got %d", len(containers))
	}

	if fake.diskStatsCalls != 0 {
		t.Errorf(
			"expected no disk stats collection without enabled disk metrics, got %d calls",
			fake.diskStatsCalls,
		)
	}

	if containers[0].GetWritableLayer() == nil {
		t.Error("expected writable layer stats to be populated")
	}
}

// TestUpdateSandboxCollectsDiskStatsWhenDiskMetricsEnabled verifies that the
// disk metrics collector still gets disk stats when the disk pod metric is
// enabled.
//
//nolint:staticcheck // testing deprecated field
func TestUpdateSandboxCollectsDiskStatsWhenDiskMetricsEnabled(t *testing.T) {
	t.Parallel()

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	cfg.IncludedPodMetrics = []string{config.DiskMetrics}

	if err := cfg.StatsConfig.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	rt := oci.NewTestRuntime()
	ctr := newRunningContainer(t, "ctr-4", "disk-metrics-container")

	fake := &fakeRuntimeImpl{
		cgroupStats: &stats.CgroupStats{SystemNano: time.Now().UnixNano()},
		diskStats:   &stats.DiskStats{},
	}
	rt.SetRuntimeImpl("ctr-4", fake)

	sb := newTestSandbox(t, "sb-4")
	sb.AddContainer(context.Background(), ctr)

	ss := newTestStatsServer(t, rt, cfg)
	if result := ss.updateSandbox(sb); result == nil {
		t.Fatal("updateSandbox returned nil")
	}

	if fake.diskStatsCalls != 1 {
		t.Fatalf(
			"expected disk stats collected once for enabled disk metrics, got %d calls",
			fake.diskStatsCalls,
		)
	}

	var foundFsMetric bool

	for _, cm := range ss.sboxMetrics["sb-4"].GetMetric().GetContainerMetrics() {
		for _, m := range cm.GetMetrics() {
			if m.GetName() == "container_fs_usage_bytes" {
				foundFsMetric = true
			}
		}
	}

	if !foundFsMetric {
		t.Error("expected container_fs_usage_bytes metric to be generated")
	}
}
