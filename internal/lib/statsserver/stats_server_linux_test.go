package statsserver

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/opencontainers/cgroups"
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

// TestComputeMemoryStatsCgroupVersion verifies that computeMemoryStats selects
// the stat keys for the cgroup version it is given, not the host's. VM
// containers can report a guest cgroup version different from the host, so the
// version must come from the stats rather than node.CgroupIsV2().
func TestComputeMemoryStatsCgroupVersion(t *testing.T) {
	t.Parallel()

	memStats := &cgroups.MemoryStats{
		Usage: cgroups.MemoryData{Usage: 1000},
		Stats: map[string]uint64{
			"anon":                100,
			"total_rss":           200,
			"inactive_file":       10,
			"total_inactive_file": 20,
			"pgfault":             7,
			"pgmajfault":          3,
		},
	}

	tests := []struct {
		name            string
		isCgroupV2      bool
		wantRSS         uint64
		wantWorkingSet  uint64
		wantPageFaults  uint64
		wantMajorFaults uint64
	}{
		{
			name:            "cgroup v2",
			isCgroupV2:      true,
			wantRSS:         100,
			wantWorkingSet:  990, // usage(1000) - inactive_file(10)
			wantPageFaults:  7,
			wantMajorFaults: 3,
		},
		{
			name:            "cgroup v1",
			isCgroupV2:      false,
			wantRSS:         200,
			wantWorkingSet:  980, // usage(1000) - total_inactive_file(20)
			wantPageFaults:  0,
			wantMajorFaults: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			workingSet, rss, pageFaults, majorFaults, _ := computeMemoryStats(
				memStats,
				tt.isCgroupV2,
			)
			if rss != tt.wantRSS {
				t.Errorf("rss: got %d, want %d", rss, tt.wantRSS)
			}

			if workingSet != tt.wantWorkingSet {
				t.Errorf("workingSet: got %d, want %d", workingSet, tt.wantWorkingSet)
			}

			if pageFaults != tt.wantPageFaults {
				t.Errorf("pageFaults: got %d, want %d", pageFaults, tt.wantPageFaults)
			}

			if majorFaults != tt.wantMajorFaults {
				t.Errorf("majorFaults: got %d, want %d", majorFaults, tt.wantMajorFaults)
			}
		})
	}
}
