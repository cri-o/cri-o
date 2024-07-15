package nri

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/containerd/nri/pkg/api"
	"github.com/stretchr/testify/require"
)

var (
	pluginSyncTimeout = 5 * time.Second
	eventTimeout      = 3 * time.Second
)

func TestPluginRegistration(stdT *testing.T) {
	skipTestForCondition(stdT,
		map[string]bool{
			"no runtime connection": crio == nil,
		},
	)

	var (
		t = &nriTest{
			plugins: []*plugin{nil},
		}
		expected = []*event{
			PluginConfigEvent,
			PluginSyncedEvent,
		}
	)

	t.Setup(stdT)
	t.StartPlugins(!WaitForPluginSync)
	require.NoError(t, t.plugins[0].VerifyEventStream(expected, true, pluginSyncTimeout),
		"received plugin registration event stream")
}

func TestPluginSynchronization(stdT *testing.T) {
	skipTestForCondition(stdT,
		map[string]bool{
			"no runtime connection": crio == nil,
		},
	)

	t := nriTest{
		plugins: []*plugin{nil},
	}
	const containerCount = 3
	pods := make([]string, 0, containerCount)
	ctrs := make([]string, 0, containerCount)

	t.Setup(stdT)

	for range containerCount {
		pod, ctr := t.runContainer()
		pods = append(pods, pod)
		ctrs = append(ctrs, ctr)
	}

	synced := t.StartPlugins(WaitForPluginSync)
	t.verifyPodIDs(pods, synced[0].pods, "pods synchronized with plugin")
	t.verifyContainerIDs(ctrs, synced[0].ctrs, "containers synchronized with plugin")
}

func TestPodEvents(stdT *testing.T) {
	skipTestForCondition(stdT,
		map[string]bool{
			"no runtime connection": crio == nil,
		},
	)

	t := &nriTest{
		plugins: []*plugin{nil},
	}

	t.Setup(stdT)
	t.StartPlugins(WaitForPluginSync)

	pod := t.createPod()
	t.stopPod(pod)
	t.removePod(pod)

	expected := []*event{
		RunPodEvent(pod),
		StopPodEvent(pod),
		RemovePodEvent(pod),
	}
	require.NoError(t, t.plugins[0].VerifyEventStream(expected, true, eventTimeout),
		"received pod event stream")
}

func TestContainerEvents(stdT *testing.T) {
	skipTestForCondition(stdT,
		map[string]bool{
			"no runtime connection": crio == nil,
		},
	)

	var (
		timeout = eventTimeout
		t       = &nriTest{
			plugins: []*plugin{nil},
		}
	)

	t.Setup(stdT)
	t.StartPlugins(WaitForPluginSync)
	p := t.plugins[0]

	pod := t.createPod()
	require.NotNil(t, p.WaitEvent(RunPodEvent(pod), timeout), "pod creation event")

	ctr := t.createContainer(pod)
	require.NotNil(t, p.WaitEvent(CreateContainerEvent(pod, ctr), 0), "container creation event")
	require.NotNil(t, p.WaitEvent(PostCreateContainerEvent(pod, ctr), timeout), "container post-creation event")

	t.startContainer(ctr)
	require.NotNil(t, p.WaitEvent(StartContainerEvent(pod, ctr), 0), "container start event")
	require.NotNil(t, p.WaitEvent(PostStartContainerEvent(pod, ctr), timeout), "container post-start event")

	t.stopContainer(ctr)
	require.NotNil(t, p.WaitEvent(StopContainerEvent(pod, ctr), 0), "container stop event")

	t.removeContainer(ctr)
	require.NotNil(t, p.WaitEvent(RemoveContainerEvent(pod, ctr), 0), "container removal event")

	t.stopPod(pod)
	t.removePod(pod)
}

func TestMountInjection(stdT *testing.T) {
	skipTestForCondition(stdT,
		map[string]bool{
			"no runtime connection": crio == nil,
		},
	)

	var (
		testDir     = stdT.TempDir()
		testFile    = "test.out"
		injectMount = func(p *plugin, pod *api.PodSandbox, ctr *api.Container) (*api.ContainerAdjustment, []*api.ContainerUpdate, error) {
			if err := os.Chmod(testDir, 0o777); err != nil {
				return nil, nil, fmt.Errorf("failed to change permissions: %w", err)
			}
			adjust := &api.ContainerAdjustment{}
			adjust.AddMount(
				&api.Mount{
					Destination: "/out",
					Source:      testDir,
					Type:        "bind",
					Options:     []string{"bind"},
				},
			)
			return adjust, nil, nil
		}

		t = &nriTest{
			plugins: []*plugin{
				nil,
			},
			options: [][]PluginOption{
				{WithCreateHandler(injectMount)},
			},
		}
	)

	t.Setup(stdT)
	t.StartPlugins(WaitForPluginSync)

	msg := fmt.Sprintf("Hello, process %d...", os.Getpid())
	cmd := WithShellScript("set -e; echo " + msg + "> /out/" + testFile + "; sleep 3600")
	t.runContainer(cmd)

	chk, err := waitForFileAndRead(filepath.Join(testDir, testFile))
	require.NoError(t, err, "read test output file")
	require.Equal(t, msg+"\n", string(chk), "check test output")
}

func TestEnvironmentInjection(stdT *testing.T) {
	skipTestForCondition(stdT,
		map[string]bool{
			"no runtime connection": crio == nil,
		},
	)

	var (
		injectEnv = func(p *plugin, pod *api.PodSandbox, ctr *api.Container) (*api.ContainerAdjustment, []*api.ContainerUpdate, error) {
			adjust := &api.ContainerAdjustment{}
			adjust.AddEnv("TEST_VARIABLE", "TEST_VALUE")
			return adjust, nil, nil
		}

		t = &nriTest{
			plugins: []*plugin{
				nil,
			},
			options: [][]PluginOption{
				{WithCreateHandler(injectEnv)},
			},
		}
	)

	t.Setup(stdT)
	t.StartPlugins(WaitForPluginSync)
	_, ctr := t.runContainer()

	stdout, _, exitCode := t.execShellScript(ctr, "set -e; echo $TEST_VARIABLE")
	expected := "TEST_VALUE\n"

	require.Equal(t, int32(0), exitCode, "exit code 0")
	require.Equal(t, expected, string(stdout), "test output")
}

func TestAnnotationInjection(stdT *testing.T) {
	skipTestForCondition(stdT,
		map[string]bool{
			"no runtime connection": crio == nil,
		},
	)

	var (
		testKey          = "TEST_KEY"
		testValue        = "TEST_VALUE"
		annotated        *api.Container
		injectAnnotation = func(p *plugin, pod *api.PodSandbox, ctr *api.Container) (*api.ContainerAdjustment, []*api.ContainerUpdate, error) {
			adjust := &api.ContainerAdjustment{}
			adjust.AddAnnotation(testKey, testValue)
			return adjust, nil, nil
		}
		saveContainer = func(p *plugin, pod *api.PodSandbox, ctr *api.Container) error {
			annotated = ctr
			return nil
		}

		t = &nriTest{
			plugins: []*plugin{
				nil,
			},
			options: [][]PluginOption{
				{
					WithCreateHandler(injectAnnotation),
					WithPostCreateHandler(saveContainer),
				},
			},
		}
	)

	t.Setup(stdT)
	t.StartPlugins(WaitForPluginSync)
	pod, ctr := t.runContainer()
	require.NotNil(t, t.plugins[0].WaitEvent(PostCreateContainerEvent(pod, ctr), eventTimeout), "container post-creation event")

	require.NotNil(t, annotated, "received post-create event")
	require.Equal(t, annotated.GetAnnotations()[testKey], testValue, "annotation updated")
}

func TestDeviceInjection(stdT *testing.T) {
	skipTestForCondition(stdT,
		map[string]bool{
			"no runtime connection": crio == nil,
		},
	)

	var (
		injectDevice = func(p *plugin, pod *api.PodSandbox, ctr *api.Container) (*api.ContainerAdjustment, []*api.ContainerUpdate, error) {
			adjust := &api.ContainerAdjustment{}
			adjust.AddDevice(&api.LinuxDevice{
				Path:     "/dev/pie",
				Type:     "c",
				Major:    31,
				Minor:    41,
				Uid:      api.UInt32(uint32(11)),
				Gid:      api.UInt32(uint32(22)),
				FileMode: api.FileMode(uint32(0o0664)),
			})
			return adjust, nil, nil
		}

		t = &nriTest{
			plugins: []*plugin{
				nil,
			},
			options: [][]PluginOption{
				{WithCreateHandler(injectDevice)},
			},
		}
	)

	t.Setup(stdT)
	t.StartPlugins(WaitForPluginSync)
	_, ctr := t.runContainer()

	stdout, _, exitCode := t.execShellScript(ctr, "set -e; stat -c %F-%a-%u:%g-%t:%T /dev/pie")
	expected := "character special file-664-11:22-1f:29\n"

	require.Equal(t, int32(0), exitCode, "exit code 0")
	require.Equal(t, expected, string(stdout), "test output")
}

func TestCpusetAdjustment(stdT *testing.T) {
	skipTestForCondition(stdT,
		map[string]bool{
			"no runtime connection":           crio == nil,
			"not enough online CPUs for test": len(getAvailableCpuset(stdT)) < 2,
		},
	)

	testXxxsetAdjustment(
		stdT,
		func() *api.ContainerAdjustment {
			adjust := &api.ContainerAdjustment{}
			adjust.SetLinuxCPUSetCPUs(availableCpuset[1])
			return adjust
		},
		"set -e; grep Cpus_allowed_list: /proc/self/status",
		"Cpus_allowed_list:\t"+availableCpuset[1]+"\n",
	)
}

func TestMemsetAdjustment(stdT *testing.T) {
	skipTestForCondition(stdT,
		map[string]bool{
			"no runtime connection":                   crio == nil,
			"not enough online memory nodes for test": len(getAvailableMemset(stdT)) < 2,
		},
	)

	testXxxsetAdjustment(
		stdT,
		func() *api.ContainerAdjustment {
			adjust := &api.ContainerAdjustment{}
			adjust.SetLinuxCPUSetMems(availableMemset[1])
			return adjust
		},
		"set -e; grep Mems_allowed_list: /proc/self/status",
		"Mems_allowed_list:\t"+availableMemset[1]+"\n",
	)
}

func testXxxsetAdjustment(stdT *testing.T, adjust func() *api.ContainerAdjustment, testScript, expectedResult string) {
	handler := func(*plugin, *api.PodSandbox, *api.Container) (*api.ContainerAdjustment, []*api.ContainerUpdate, error) {
		return adjust(), nil, nil
	}

	t := &nriTest{
		plugins: []*plugin{
			nil,
		},
		options: [][]PluginOption{
			{WithCreateHandler(handler)},
		},
	}

	t.Setup(stdT)
	t.StartPlugins(WaitForPluginSync)
	_, ctr := t.runContainer()

	stdout, _, exitCode := t.execShellScript(ctr, testScript)
	t.Logf("*** got stdout %s, exitCode %d", stdout, exitCode)
	require.Equal(t, int32(0), exitCode, "exit code 0")
	require.Equal(t, expectedResult, string(stdout), "test output")
}

func TestCpusetAdjustmentUpdate(stdT *testing.T) {
	skipTestForCondition(stdT,
		map[string]bool{
			"no runtime connection":           crio == nil,
			"not enough online CPUs for test": len(getAvailableCpuset(stdT)) < 2,
		},
	)

	testXxxsetAdjustmentUpdate(
		stdT,
		func() *api.ContainerAdjustment {
			adjust := &api.ContainerAdjustment{}
			adjust.SetLinuxCPUSetCPUs(availableCpuset[1])
			return adjust
		},
		func(ctr0 string) *api.ContainerUpdate {
			update := &api.ContainerUpdate{}
			update.SetContainerId(ctr0)
			update.SetLinuxCPUSetCPUs(availableCpuset[0])
			return update
		},
		"set -e; grep Cpus_allowed_list: /proc/self/status",
		"Cpus_allowed_list:\t"+availableCpuset[1]+"\n",
		"Cpus_allowed_list:\t"+availableCpuset[0]+"\n",
	)
}

func TestMemsetAdjustmentUpdate(stdT *testing.T) {
	skipTestForCondition(stdT,
		map[string]bool{
			"no runtime connection":                   crio == nil,
			"not enough online memory nodes for test": len(getAvailableMemset(stdT)) < 2,
		},
	)

	testXxxsetAdjustmentUpdate(
		stdT,
		func() *api.ContainerAdjustment {
			adjust := &api.ContainerAdjustment{}
			adjust.SetLinuxCPUSetMems(availableMemset[1])
			return adjust
		},
		func(ctr0 string) *api.ContainerUpdate {
			update := &api.ContainerUpdate{}
			update.SetContainerId(ctr0)
			update.SetLinuxCPUSetMems(availableMemset[0])
			return update
		},
		"set -e; grep Mems_allowed_list: /proc/self/status",
		"Mems_allowed_list:\t"+availableMemset[1]+"\n",
		"Mems_allowed_list:\t"+availableMemset[0]+"\n",
	)
}

func testXxxsetAdjustmentUpdate(stdT *testing.T, adjust func() *api.ContainerAdjustment, update func(string) *api.ContainerUpdate, testScript, expectedAdjustResult, expectedUpdateResult string) {
	skipTestForCondition(stdT,
		map[string]bool{
			"no runtime connection":           crio == nil,
			"not enough online CPUs for test": len(getAvailableCpuset(stdT)) < 2,
		},
	)

	ctr0 := ""
	handler := func(p *plugin, pod *api.PodSandbox, ctr *api.Container) (*api.ContainerAdjustment, []*api.ContainerUpdate, error) {
		if ctr0 == "" {
			ctr0 = ctr.GetId()
			return adjust(), nil, nil
		} else {
			return nil, []*api.ContainerUpdate{update(ctr0)}, nil
		}
	}

	t := &nriTest{
		plugins: []*plugin{
			nil,
		},
		options: [][]PluginOption{
			{WithCreateHandler(handler)},
		},
	}

	t.Setup(stdT)
	t.StartPlugins(WaitForPluginSync)
	_, ctr := t.runContainer()

	stdout, _, exitCode := t.execShellScript(ctr, testScript)
	t.Logf("*** got stdout %s, exitCode %d", stdout, exitCode)
	require.Equal(t, int32(0), exitCode, "exit code 0")
	require.Equal(t, expectedAdjustResult, string(stdout), "test output")

	t.runContainer()
	stdout, _, exitCode = t.execShellScript(ctr, testScript)
	t.Logf("*** got stdout %s, exitCode %d", stdout, exitCode)
	require.Equal(t, int32(0), exitCode, "exit code 0")
	require.Equal(t, expectedUpdateResult, string(stdout), "test output")
}

// skipTestForCondition skips the test if any of the conditions are true.
func skipTestForCondition(t *testing.T, skipChecks ...map[string]bool) {
	_, err := os.Stat(strings.TrimPrefix(*nriSocket, "unix://"))
	if err != nil {
		t.Skip("cri-o test instance does not have NRI enabled")
	}

	for _, checks := range skipChecks {
		for check, skip := range checks {
			if skip {
				t.Skip(check)
			}
		}
	}
}

type idgen struct {
	sync.Mutex
	uid int
	pod int
	ctr int
}

var ids = &idgen{}

func (g *idgen) GenUID() string {
	g.Lock()
	defer g.Unlock()
	defer func() { g.uid += 1 }()

	return "uid-" + strconv.Itoa(g.uid)
}

func (g *idgen) GenPodName() string {
	g.Lock()
	defer g.Unlock()
	defer func() { g.pod += 1 }()

	return "pod-" + strconv.Itoa(g.pod)
}

func (g *idgen) GenCtrName() string {
	g.Lock()
	defer g.Unlock()
	defer func() { g.ctr += 1 }()

	return "ctr-" + strconv.Itoa(g.ctr)
}
