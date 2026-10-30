package oci

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	sandbox "github.com/containerd/containerd/api/runtime/sandbox/v1"
	"github.com/containerd/ttrpc"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/pkg/config"
)

func TestSandboxAPIRequestPreservesCRIConfiguration(t *testing.T) {
	bundle := t.TempDir()
	c := NewSpoofedContainer("sandbox", "test", nil, "sandbox", time.Now(), bundle)
	c.ConfigureSandboxRuntime("kata", bundle)
	c.SetSpec(&specs.Spec{Process: &specs.Process{}, Linux: &specs.Linux{
		CgroupsPath: "system.slice:crio:sandbox",
		Namespaces:  []specs.LinuxNamespace{{Type: specs.NetworkNamespace, Path: "/run/netns/pod"}},
	}})
	original := &types.PodSandboxConfig{
		Hostname: "test-host",
		Metadata: &types.PodSandboxMetadata{Name: "pod", Uid: "uid"},
		DnsConfig: &types.DNSConfig{
			Servers:  []string{"192.0.2.53"},
			Searches: []string{"test.local"},
			Options:  []string{"ndots:2"},
		},
		Annotations: map[string]string{"io.katacontainers.config.hypervisor.default_vcpus": "2"},
		Linux:       &types.LinuxPodSandboxConfig{CgroupParent: "system.slice"},
	}

	wire, err := proto.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(bundle, SandboxConfigFile), wire, 0o600); err != nil {
		t.Fatal(err)
	}

	req, err := sandboxRequest(c, "system.slice")
	if err != nil {
		t.Fatal(err)
	}

	if req.GetSandboxID() != "sandbox" || req.GetBundlePath() != bundle ||
		req.GetNetnsPath() != "/run/netns/pod" ||
		req.GetOptions().GetTypeUrl() != "runtime.v1.PodSandboxConfig" {
		t.Fatalf("incorrect sandbox request: %v", req)
	}

	got := &types.PodSandboxConfig{}
	if err := proto.Unmarshal(req.GetOptions().GetValue(), got); err != nil {
		t.Fatal(err)
	}

	original.Linux.CgroupParent = "system.slice:crio:sandbox"
	if !proto.Equal(original, got) {
		t.Fatalf("CRI configuration changed unexpectedly: %v", got)
	}

	c.SetSpec(&specs.Spec{Process: &specs.Process{}, Linux: &specs.Linux{}})

	if _, err := sandboxRequest(c, "system.slice"); err == nil {
		t.Fatal("host networking unexpectedly accepted")
	}

	if err := os.WriteFile(
		filepath.Join(bundle, SandboxConfigFile),
		[]byte{0xff},
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := sandboxRequest(c, ""); err == nil {
		t.Fatal("malformed CRI config accepted")
	}
}

type sandboxAPITestServer struct {
	sandbox.TTRPCSandboxService

	mu            sync.Mutex
	calls         []string
	stopTimeout   uint32
	shutdownError bool
	wait          func(context.Context) (*sandbox.WaitSandboxResponse, error)
	status        string
	statusError   error
}

func (s *sandboxAPITestServer) StartSandbox(
	context.Context,
	*sandbox.StartSandboxRequest,
) (*sandbox.StartSandboxResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, "start")

	return &sandbox.StartSandboxResponse{
		Pid:       uint32(os.Getpid()),
		CreatedAt: timestamppb.Now(),
	}, nil
}

func (s *sandboxAPITestServer) StopSandbox(
	_ context.Context,
	req *sandbox.StopSandboxRequest,
) (*sandbox.StopSandboxResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, "stop")
	s.stopTimeout = req.GetTimeoutSecs()

	return &sandbox.StopSandboxResponse{}, nil
}

func (s *sandboxAPITestServer) ShutdownSandbox(
	context.Context,
	*sandbox.ShutdownSandboxRequest,
) (*sandbox.ShutdownSandboxResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, "shutdown")
	if s.shutdownError {
		return nil, errors.New("injected shutdown failure")
	}

	return &sandbox.ShutdownSandboxResponse{}, nil
}

func (s *sandboxAPITestServer) WaitSandbox(
	ctx context.Context,
	_ *sandbox.WaitSandboxRequest,
) (*sandbox.WaitSandboxResponse, error) {
	if s.wait != nil {
		return s.wait(ctx)
	}

	<-ctx.Done()

	return nil, ctx.Err()
}

func (s *sandboxAPITestServer) SandboxStatus(
	context.Context,
	*sandbox.SandboxStatusRequest,
) (*sandbox.SandboxStatusResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, "status")

	state := s.status
	if state == "" {
		state = "SANDBOX_READY"
	}

	return &sandbox.SandboxStatusResponse{Pid: uint32(os.Getpid()), State: state}, s.statusError
}

func sandboxAPIEndpoint(t *testing.T) (*Container, *runtimeVM, *sandboxAPITestServer) {
	t.Helper()
	bundle := t.TempDir()
	socket := filepath.Join(bundle, "sandbox.sock")

	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}

	srv, err := ttrpc.NewServer()
	if err != nil {
		t.Fatal(err)
	}

	fake := &sandboxAPITestServer{}

	sandbox.RegisterTTRPCSandboxService(srv, fake)

	serveDone := make(chan struct{})
	go func() {
		defer close(serveDone)

		if err := srv.Serve(
			context.Background(),
			listener,
		); err != nil &&
			!errors.Is(err, net.ErrClosed) && !errors.Is(err, ttrpc.ErrServerClosed) {
			t.Errorf("serve sandbox: %v", err)
		}
	}()

	t.Cleanup(func() { srv.Close(); listener.Close(); <-serveDone })

	if err := os.WriteFile(
		filepath.Join(bundle, "address"),
		[]byte("unix://"+socket),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	c := NewSpoofedContainer("sandbox", "test", nil, "sandbox", time.Now(), bundle)
	c.ConfigureSandboxRuntime("kata", bundle)

	drop := true
	r := newRuntimeVM(&config.RuntimeHandler{DropInfraCtr: &drop, ContainerCreateTimeout: 2}, "")
	ctx, cancel := context.WithCancel(context.Background())
	r.ctx = ctx
	r.exitsPath = t.TempDir()
	t.Cleanup(func() {
		cancel()
		c.opLock.Lock()
		if r.client != nil {
			r.client.Close()
		}
		c.opLock.Unlock()
		waitSandboxCondition(t, c, func() bool { return !r.sandboxWatch })
	})

	return c, r, fake
}

func TestSandboxAPIReconnectLifecycle(t *testing.T) {
	c, r, fake := sandboxAPIEndpoint(t)
	// A fresh runtime instance must reconnect using the persisted shim address.
	if err := r.UpdateContainerStatus(context.Background(), c); err != nil {
		t.Fatal(err)
	}

	if c.state.Status != ContainerStateRunning {
		t.Fatal("sandbox not restored as running")
	}

	if err := r.StartContainer(context.Background(), c); err != nil {
		t.Fatal(err)
	}

	if err := r.StopContainer(context.Background(), c, 7); err != nil {
		t.Fatal(err)
	}

	if c.state.Status != ContainerStateStopped {
		t.Fatal("sandbox not stopped")
	}

	if err := r.DeleteContainer(context.Background(), c); err != nil {
		t.Fatal(err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()

	if fake.stopTimeout != 7 {
		t.Fatalf("stop timeout = %d", fake.stopTimeout)
	}

	if len(fake.calls) != 4 || fake.calls[0] != "status" || fake.calls[1] != "start" ||
		fake.calls[2] != "stop" ||
		fake.calls[3] != "shutdown" {
		t.Fatalf("unexpected lifecycle: %v", fake.calls)
	}
}

func TestSandboxAPIShutdownFailureReported(t *testing.T) {
	c, r, fake := sandboxAPIEndpoint(t)
	fake.shutdownError = true

	if err := r.DeleteContainer(context.Background(), c); err == nil {
		t.Fatal("shutdown failure ignored")
	}
}

func TestSandboxAPIRuntimeDropInfraOverride(t *testing.T) {
	for _, tc := range []struct {
		name, input  string
		wantOverride *bool
	}{
		{name: "omitted"},
		{name: "enabled", input: "drop_infra_ctr = true", wantOverride: new(true)},
		{name: "disabled", input: "drop_infra_ctr = false", wantOverride: new(false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var handler config.RuntimeHandler
			if _, err := toml.Decode(tc.input, &handler); err != nil {
				t.Fatal(err)
			}

			if (handler.DropInfraCtr == nil) != (tc.wantOverride == nil) {
				t.Fatal("override presence changed")
			}

			if handler.DropInfraCtr != nil && *handler.DropInfraCtr != *tc.wantOverride {
				t.Fatal("override value changed")
			}

			var wire bytes.Buffer
			if err := toml.NewEncoder(&wire).Encode(handler); err != nil {
				t.Fatal(err)
			}

			var restored config.RuntimeHandler
			if _, err := toml.Decode(wire.String(), &restored); err != nil {
				t.Fatal(err)
			}

			if (restored.DropInfraCtr == nil) != (handler.DropInfraCtr == nil) {
				t.Fatal("round trip lost override presence")
			}

			if restored.DropInfraCtr != nil && *restored.DropInfraCtr != *handler.DropInfraCtr {
				t.Fatal("round trip lost override value")
			}
		})
	}
}

func waitSandboxCondition(t *testing.T, c *Container, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c.opLock.Lock()
		ok := condition()
		c.opLock.Unlock()

		if ok {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("sandbox condition did not become true")
}

func TestSandboxAPIDeleteRetries(t *testing.T) {
	for _, mode := range []string{"closed client", "missing address", "refused connection"} {
		t.Run(mode, func(t *testing.T) {
			c, r, _ := sandboxAPIEndpoint(t)

			switch mode {
			case "closed client":
				if err := r.connectSandbox(c); err != nil {
					t.Fatal(err)
				}

				if err := r.client.Close(); err != nil {
					t.Fatal(err)
				}
			case "missing address":
				if err := os.Remove(filepath.Join(c.BundlePath(), "address")); err != nil {
					t.Fatal(err)
				}
			case "refused connection":
				// A bound Unix socket with no listening process returns ECONNREFUSED.
				socket := filepath.Join(c.BundlePath(), "dead.sock")
				addr := &net.UnixAddr{Name: socket, Net: "unix"}

				listener, err := net.ListenUnix("unix", addr)
				if err != nil {
					t.Fatal(err)
				}

				listener.SetUnlinkOnClose(false)

				if err := listener.Close(); err != nil {
					t.Fatal(err)
				}

				if err := os.WriteFile(
					filepath.Join(c.BundlePath(), "address"),
					[]byte("unix://"+socket),
					0o600,
				); err != nil {
					t.Fatal(err)
				}
			}

			for range 2 {
				if err := r.DeleteContainer(context.Background(), c); err != nil {
					t.Fatal(err)
				}

				if r.sandbox != nil || r.client != nil {
					t.Fatal("deleted sandbox retained a closed client")
				}
			}
		})
	}
}

func TestSandboxAPIWaitRecovery(t *testing.T) {
	for _, mode := range []string{"transient wait", "closed client", "transient status"} {
		t.Run(mode, func(t *testing.T) {
			c, r, fake := sandboxAPIEndpoint(t)
			entered := make(chan struct{}, 4)
			exited := make(chan struct{})

			fake.wait = func(ctx context.Context) (*sandbox.WaitSandboxResponse, error) {
				entered <- struct{}{}

				select {
				case <-exited:
					return &sandbox.WaitSandboxResponse{
						ExitStatus: 17,
						ExitedAt:   timestamppb.Now(),
					}, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(50 * time.Millisecond):
					return nil, errors.New("temporary wait error")
				}
			}
			if mode == "transient status" {
				fake.statusError = errors.New("temporary status error")
			}

			if err := r.UpdateContainerStatus(
				context.Background(),
				c,
			); err != nil &&
				mode != "transient status" {
				t.Fatal(err)
			}

			if mode == "transient status" {
				c.opLock.Lock()
				c.state.Status = ContainerStateRunning
				r.watchSandbox(c)
				c.opLock.Unlock()
			}

			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("wait did not start")
			}

			if mode == "closed client" {
				c.opLock.Lock()
				if err := r.client.Close(); err != nil {
					t.Error(err)
				}
				c.opLock.Unlock()
			}

			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("wait was not restarted")
			}

			c.opLock.Lock()
			if c.state.Status != ContainerStateRunning {
				t.Error("live sandbox marked stopped")
			}
			c.opLock.Unlock()
			close(exited)
			waitSandboxCondition(
				t,
				c,
				func() bool { return !r.sandboxWatch && c.state.ExitCode != nil && *c.state.ExitCode == 17 },
			)

			if _, err := os.Stat(filepath.Join(r.exitsPath, c.ID())); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSandboxAPIWaitFailurePreservesExit(t *testing.T) {
	c, r, fake := sandboxAPIEndpoint(t)
	fake.status = "STOPPED"
	fake.wait = func(context.Context) (*sandbox.WaitSandboxResponse, error) { return nil, errors.New("failed wait") }
	finished := time.Now().Add(-time.Minute)
	c.state.Status = ContainerStateStopped
	c.state.Finished = finished
	c.state.ExitCode = new(int32(42))
	c.opLock.Lock()
	r.watchSandbox(c)
	c.opLock.Unlock()
	waitSandboxCondition(t, c, func() bool { return !r.sandboxWatch })

	if !c.state.Finished.Equal(finished) || *c.state.ExitCode != 42 {
		t.Fatal("existing exit state lost")
	}

	if _, err := os.Stat(filepath.Join(r.exitsPath, c.ID())); err != nil {
		t.Fatal(err)
	}
}

func TestSandboxAPIRestoredMode(t *testing.T) {
	for _, configured := range []*bool{nil, new(false), new(true)} {
		for _, persisted := range []bool{false, true} {
			c, _, _ := sandboxAPIEndpoint(t)
			if !persisted {
				c.bundlePath = ""
			}

			cfg := &config.Config{RuntimeConfig: config.RuntimeConfig{Runtimes: config.Runtimes{
				"kata": {
					RuntimePath:  "/bin/true",
					RuntimeType:  config.RuntimeTypeVM,
					DropInfraCtr: configured,
				},
			}}}
			runtime := &Runtime{config: cfg, runtimeImplMap: make(map[string]RuntimeImpl)}

			pod, err := runtime.RuntimeImpl(c)
			if err != nil {
				t.Fatal(err)
			}

			vm, ok := pod.(*runtimeVM)
			if !ok || vm.sandboxAPI != persisted {
				t.Fatal("persisted sandbox mode lost")
			}

			workload := &Container{
				criContainer:   &types.Container{Id: "workload", PodSandboxId: c.ID()},
				runtimeHandler: "kata",
			}

			impl, err := runtime.RuntimeImpl(workload)
			if err != nil {
				t.Fatal(err)
			}

			vm, ok = impl.(*runtimeVM)
			if !ok || vm.sandboxAPI != persisted {
				t.Fatal("workload lost sandbox mode")
			}
		}
	}
}

func TestSandboxAPIWaitShimGone(t *testing.T) {
	c, r, fake := sandboxAPIEndpoint(t)
	entered := make(chan struct{})
	fake.wait = func(ctx context.Context) (*sandbox.WaitSandboxResponse, error) {
		close(entered)
		<-ctx.Done()

		return nil, ctx.Err()
	}

	if err := r.UpdateContainerStatus(context.Background(), c); err != nil {
		t.Fatal(err)
	}

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("wait did not start")
	}

	c.opLock.Lock()
	if err := os.Remove(filepath.Join(c.BundlePath(), "address")); err != nil {
		t.Error(err)
	}

	if err := r.client.Close(); err != nil {
		t.Error(err)
	}
	c.opLock.Unlock()
	waitSandboxCondition(
		t,
		c,
		func() bool { return !r.sandboxWatch && c.state.Status == ContainerStateStopped },
	)

	if c.state.ExitCode == nil || *c.state.ExitCode != 255 {
		t.Fatal("missing shim did not record exit status")
	}

	if _, err := os.Stat(filepath.Join(r.exitsPath, c.ID())); err != nil {
		t.Fatal(err)
	}
}

func TestSandboxAPIFailedCleanupReconnects(t *testing.T) {
	c, r, _ := sandboxAPIEndpoint(t)
	if err := r.connectSandbox(c); err != nil {
		t.Fatal(err)
	}

	r.cleanupFailedSandbox(context.Background(), c)

	if r.sandbox != nil || r.client != nil {
		t.Fatal("failed sandbox retained a closed client")
	}

	if err := r.DeleteContainer(context.Background(), c); err != nil {
		t.Fatal(err)
	}
}
