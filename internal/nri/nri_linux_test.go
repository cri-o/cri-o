package nri

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/containerd/nri/pkg/api"
	"github.com/containerd/nri/pkg/stub"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/require"

	config "github.com/cri-o/cri-o/internal/config/nri"
)

func TestContainerNotificationsWithBlockedStatus(t *testing.T) {
	l := newTestLocal(t)
	pod := &testPodSandbox{PodSandbox: &api.PodSandbox{Id: "pod"}}

	for name, notify := range containerNotifications(l) {
		t.Run(name, func(t *testing.T) {
			entered := make(chan struct{})
			release := make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })

			var workers sync.WaitGroup

			t.Cleanup(func() {
				unblock()
				workers.Wait()
			})

			ctr := newTestContainer("blocked")
			ctr.status = func() *ContainerStatus {
				close(entered)
				<-release

				return &ContainerStatus{State: api.ContainerState_CONTAINER_RUNNING}
			}
			l.state[ctr.GetID()] = Running

			result := make(chan error, 1)

			workers.Go(func() { result <- notify(t.Context(), pod, ctr) })
			waitFor(t, entered)

			// Exercise unrelated lifecycle calls while the first container's
			// status read is blocked, independently of stop cancellation.
			other := make(chan error, 1)

			workers.Go(func() {
				if err := l.RunPodSandbox(t.Context(), pod); err != nil {
					other <- err

					return
				}

				other <- l.PostUpdateContainer(t.Context(), pod, newTestContainer("other"))
			})
			require.NoError(t, waitFor(t, other))

			unblock()
			require.NoError(t, waitFor(t, result))
		})
	}
}

func TestContainerNotificationsWithPluginUpdate(t *testing.T) {
	l := newTestLocal(t)
	pod := &testPodSandbox{PodSandbox: &api.PodSandbox{Id: "pod"}}

	for name, notify := range containerNotifications(l) {
		t.Run(name, func(t *testing.T) {
			updating := make(chan struct{})
			release := make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })

			var workers sync.WaitGroup

			oldDomain := domains.domain

			t.Cleanup(func() {
				unblock()
				workers.Wait()

				domains.domain = oldDomain
			})

			ctr := newTestContainer("updating")
			l.state[ctr.GetID()] = Running
			statusRead := make(chan struct{})
			ctr.status = func() *ContainerStatus {
				close(statusRead)

				return &ContainerStatus{State: api.ContainerState_CONTAINER_RUNNING}
			}
			// Remove converts the container twice (stop, then remove). Only the
			// first conversion is checked, so never block the notification on
			// the later ones: a blocked worker would hang the cleanup.
			resourcesRead := make(chan *api.LinuxResources, 1)
			ctr.linux.readResources = func(resources *api.LinuxResources) {
				select {
				case resourcesRead <- resources:
				default:
				}
			}

			// Model the in-place resource writes performed by the server for
			// a plugin update. Conversion must wait and see the updated values.
			domains.domain = &testDomain{
				update: func(_ context.Context, _ *api.ContainerUpdate) error {
					close(updating)
					<-release

					ctr.linux.resources.CPU.Cpus = "1"
					ctr.linux.resources.Unified["memory.high"] = "2000"

					return nil
				},
			}

			updated := make(chan error, 1)

			workers.Go(func() {
				_, err := l.updateFromPlugin(
					t.Context(),
					[]*api.ContainerUpdate{{ContainerId: ctr.GetID()}},
				)
				updated <- err
			})
			waitFor(t, updating)

			result := make(chan error, 1)

			workers.Go(func() { result <- notify(t.Context(), pod, ctr) })
			waitFor(t, statusRead)

			var early *api.LinuxResources
			select {
			case early = <-resourcesRead:
			case <-time.After(50 * time.Millisecond):
			}

			unblock()
			require.NoError(t, waitFor(t, updated))
			require.NoError(t, waitFor(t, result))
			require.Nil(t, early, "converted resources while a plugin update held the NRI mutex")

			resources := waitFor(t, resourcesRead)
			require.Equal(t, "1", resources.GetCpu().GetCpus())
			require.Equal(t, "2000", resources.GetUnified()["memory.high"])
		})
	}
}

func newTestLocal(t *testing.T) *local {
	t.Helper()

	l, err := New(config.New())
	require.NoError(t, err)
	t.Cleanup(l.Stop)

	return l
}

// TestUntrackedContainerSkipsConversion checks that stop and remove
// notifications for a container NRI does not track return without converting
// the pod and container. That conversion runs under the NRI mutex, so doing it
// for a container with nothing to stop or remove only lengthens the hold that
// unrelated lifecycle operations queue behind.
func TestUntrackedContainerSkipsConversion(t *testing.T) {
	pod := &testPodSandbox{PodSandbox: &api.PodSandbox{Id: "pod"}}

	for _, call := range []string{"stop", "remove"} {
		for name, tracked := range map[string]bool{"untracked": false, "tracked": true} {
			t.Run(call+"/"+name, func(t *testing.T) {
				l := newTestLocal(t)

				notify := map[string]func(context.Context, PodSandbox, Container) error{
					"stop":   l.StopContainer,
					"remove": l.RemoveContainer,
				}[call]

				ctr := newTestContainer("ctr")

				var converted atomic.Bool

				ctr.linux.readResources = func(*api.LinuxResources) { converted.Store(true) }

				if tracked {
					l.state[ctr.GetID()] = Running
				}

				require.NoError(t, notify(t.Context(), pod, ctr))
				require.Equal(t, tracked, converted.Load(),
					"container spec conversion under the NRI mutex")
			})
		}
	}
}

// TestPostUpdatePodSandbox checks that a successful pod sandbox update is
// relayed to plugins as a PostUpdatePodSandbox event after the
// UpdatePodSandbox request.
func TestPostUpdatePodSandbox(t *testing.T) {
	dir := t.TempDir()
	cfg := config.New()
	cfg.SocketPath = filepath.Join(dir, "nri.sock")
	cfg.PluginPath = filepath.Join(dir, "plugins")
	cfg.PluginConfigPath = filepath.Join(dir, "conf.d")

	l, err := New(cfg)
	require.NoError(t, err)
	t.Cleanup(l.Stop)

	SetDomain(&emptyDomain{})
	require.NoError(t, l.Start())

	p := &podUpdatePlugin{
		updates:     make(chan string, 16),
		postUpdates: make(chan string, 16),
	}
	s, err := stub.New(p,
		stub.WithSocketPath(cfg.SocketPath),
		stub.WithPluginName("post-update-test"),
		stub.WithPluginIdx("00"),
	)
	require.NoError(t, err)
	require.NoError(t, s.Start(t.Context()))
	t.Cleanup(s.Stop)

	pod := &testPodSandbox{PodSandbox: &api.PodSandbox{
		Id:    "pod",
		Linux: &api.LinuxPodSandbox{},
	}}

	// The plugin only starts receiving events once the runtime side of
	// its synchronization has finished, so retry the update until the
	// plugin sees it.
	require.Eventually(t, func() bool {
		require.NoError(t, l.UpdatePodSandbox(t.Context(), pod, nil, nil))

		select {
		case id := <-p.updates:
			return id == pod.GetId()
		default:
			return false
		}
	}, 5*time.Second, 10*time.Millisecond, "plugin did not receive UpdatePodSandbox")

	require.NoError(t, l.PostUpdatePodSandbox(t.Context(), pod))
	require.Equal(t, pod.GetId(), waitFor(t, p.postUpdates),
		"plugin did not receive PostUpdatePodSandbox")
}

type podUpdatePlugin struct {
	updates     chan string
	postUpdates chan string
}

func (p *podUpdatePlugin) UpdatePodSandbox(
	_ context.Context,
	pod *api.PodSandbox,
	_, _ *api.LinuxResources,
) error {
	p.updates <- pod.GetId()

	return nil
}

func (p *podUpdatePlugin) PostUpdatePodSandbox(_ context.Context, pod *api.PodSandbox) error {
	p.postUpdates <- pod.GetId()

	return nil
}

type emptyDomain struct {
	Domain
}

func (*emptyDomain) GetName() string                               { return "test" }
func (*emptyDomain) ListPodSandboxes(context.Context) []PodSandbox { return nil }
func (*emptyDomain) ListContainers() []Container                   { return nil }

func containerNotifications(
	l *local,
) map[string]func(context.Context, PodSandbox, Container) error {
	return map[string]func(context.Context, PodSandbox, Container) error{
		"create": func(ctx context.Context, pod PodSandbox, ctr Container) error {
			_, err := l.CreateContainer(ctx, pod, ctr)

			return err
		},
		"post-create": l.PostCreateContainer,
		"start":       l.StartContainer,
		"post-start":  l.PostStartContainer,
		"update": func(ctx context.Context, pod PodSandbox, ctr Container) error {
			_, err := l.UpdateContainer(ctx, pod, ctr, &api.LinuxResources{})

			return err
		},
		"post-update": l.PostUpdateContainer,
		"stop":        l.StopContainer,
		"remove":      l.RemoveContainer,
	}
}

func waitFor[T any](t *testing.T, ch <-chan T) T {
	t.Helper()

	select {
	case result := <-ch:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for NRI operation")

		var zero T

		return zero
	}
}

type testContainer struct {
	*api.Container

	status func() *ContainerStatus
	linux  *testLinuxContainer
}

func newTestContainer(id string) *testContainer {
	return &testContainer{
		Container: &api.Container{Id: id},
		linux: &testLinuxContainer{
			resources: &specs.LinuxResources{
				CPU:     &specs.LinuxCPU{Cpus: "0"},
				Unified: map[string]string{"memory.high": "1000"},
			},
		},
	}
}

func (c *testContainer) GetID() string                     { return c.GetId() }
func (c *testContainer) GetDomain() string                 { return "test" }
func (c *testContainer) GetPodSandboxID() string           { return c.GetPodSandboxId() }
func (c *testContainer) GetSpec() *specs.Spec              { return nil }
func (c *testContainer) GetLinuxContainer() LinuxContainer { return c.linux }

func (c *testContainer) GetStatus() *ContainerStatus {
	if c.status != nil {
		return c.status()
	}

	return &ContainerStatus{State: api.ContainerState_CONTAINER_RUNNING}
}

type testLinuxContainer struct {
	*api.LinuxContainer

	resources     *specs.LinuxResources
	readResources func(*api.LinuxResources)
}

func (c *testLinuxContainer) GetLinuxNamespaces() []*api.LinuxNamespace { return c.GetNamespaces() }
func (c *testLinuxContainer) GetLinuxDevices() []*api.LinuxDevice       { return c.GetDevices() }
func (c *testLinuxContainer) GetOOMScoreAdj() *int                      { return nil }
func (c *testLinuxContainer) GetIOPriority() *api.LinuxIOPriority       { return c.GetIoPriority() }

func (c *testLinuxContainer) GetLinuxResources() *api.LinuxResources {
	resources := api.FromOCILinuxResources(c.resources, nil)
	if c.readResources != nil {
		c.readResources(resources)
	}

	return resources
}

type testPodSandbox struct {
	*api.PodSandbox
}

func (p *testPodSandbox) GetDomain() string { return "test" }
func (p *testPodSandbox) GetID() string     { return p.GetId() }
func (p *testPodSandbox) GetUID() string    { return p.GetUid() }
func (p *testPodSandbox) GetIPs() []string  { return p.GetIps() }

func (p *testPodSandbox) GetLinuxPodSandbox() LinuxPodSandbox {
	return &testLinuxPodSandbox{LinuxPodSandbox: p.GetLinux()}
}

type testLinuxPodSandbox struct {
	*api.LinuxPodSandbox
}

func (p *testLinuxPodSandbox) GetLinuxNamespaces() []*api.LinuxNamespace { return p.GetNamespaces() }
func (p *testLinuxPodSandbox) GetLinuxResources() *api.LinuxResources    { return p.GetResources() }

func (p *testLinuxPodSandbox) GetPodLinuxOverhead() *api.LinuxResources { return p.GetPodOverhead() }

func (p *testLinuxPodSandbox) GetPodLinuxResources() *api.LinuxResources { return p.GetPodResources() }

type testDomain struct {
	Domain

	update func(context.Context, *api.ContainerUpdate) error
}

func (d *testDomain) UpdateContainer(ctx context.Context, update *api.ContainerUpdate) error {
	return d.update(ctx, update)
}
