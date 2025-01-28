package nri

import (
	"flag"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/containerd/nri/pkg/api"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

var crio *runtime

func setup() {
	if *crioSocket == "" && *nriSocket == "" {
		return
	}

	r, err := ConnectRuntime()
	if err != nil {
		logrus.WithError(err).Fatal("failed to connect to runtime")
	}

	err = r.PullImages()
	if err != nil {
		logrus.WithError(err).Fatal("failed to pull test images")
	}

	crio = r
}

func cleanup() {
	if crio != nil {
		crio.Disconnect()
		crio = nil
	}
}

func TestMain(m *testing.M) {
	flag.Parse()

	if list := flag.CommandLine.Lookup("test.list"); list != nil {
		if list.Value.String() != "" {
			os.Exit(m.Run())
		}
	}

	setupLogging()
	setup()

	status := m.Run()

	cleanup()

	os.Exit(status)
}

//
// testLogger is used to bridge logrus to golang test logging
//

type testLogger struct {
	Out io.Writer
}

func (t *testLogger) Write(p []byte) (n int, err error) {
	fmt.Printf("%s", string(p))

	return len(p), nil
}

func setupLogging() {
	std := logrus.StandardLogger()
	tst := &testLogger{
		Out: std.Out,
	}
	std.Out = tst
}

//
// nriTest keeps the state for an individual (set of) NRI test(s)
//

type nriTest struct {
	*testing.T
	namespace string
	plugins   []*plugin
	options   [][]PluginOption
}

func (t *nriTest) Setup(stdT *testing.T) {
	stdT.Helper()

	t.namespace = getTestNamespace()
	t.T = stdT

	t.purgePodsAndContainers()

	for i := range t.plugins {
		if len(t.options) >= i+1 {
			t.plugins[i] = NewPlugin(t.namespace, t.options[i]...)
		} else {
			t.plugins[i] = NewPlugin(t.namespace)
		}

		require.NotNil(t, t.plugins[i], "create plugin #%d for test %s", t.namespace)
	}

	t.T.Cleanup(t.Cleanup)
}

const (
	WaitForPluginSync = true
)

func (t *nriTest) StartPlugins(waitSync bool) []*event {
	events := []*event{}

	t.T.Helper()

	for i, p := range t.plugins {
		require.NoError(t, p.Start(), "start test %s plugin #%d (%s)", t.namespace, i, p.Name())

		if !waitSync {
			continue
		}

		e := p.WaitEvent(PluginSyncedEvent, pluginSyncTimeout)
		require.NotNil(t, e, "test %s plugin #%d (%s) startup", t.namespace, i, p.Name())
		events = append(events, e)
	}

	return events
}

func (t *nriTest) Cleanup() {
	t.T.Helper()

	for _, p := range t.plugins {
		if p != nil {
			p.Stop()
		}
	}

	t.purgePodsAndContainers()
}

func (t *nriTest) purgePodsAndContainers() {
	t.T.Helper()

	var (
		readyPods   []string
		otherPods   []string
		runningCtrs []string
		otherCtrs   []string
	)

	runningCtrs, otherCtrs, readyPods, otherPods, err := crio.ListContainers(t.namespace)
	require.NoError(t, err, "list pods and containers for test %s", t.namespace)

	for _, ctr := range runningCtrs {
		if err := crio.StopContainer(ctr); err != nil {
			t.Logf("failed to stop container %s: %v", ctr, err)
		}

		require.NoError(t, crio.RemoveContainer(ctr), "remove container %s", ctr)
	}

	for _, ctr := range otherCtrs {
		require.NoError(t, crio.RemoveContainer(ctr), "remove container %s", ctr)
	}

	for _, pod := range readyPods {
		if err := crio.StopPod(pod); err != nil {
			t.Logf("failed to stop pod %s: %v", pod, err)
		}

		require.NoError(t, crio.RemovePod(pod), "remove test %s pod %s", t.namespace, pod)
	}

	for _, pod := range otherPods {
		require.NoError(t, crio.RemovePod(pod), "remove test %s pod %s", t.namespace, pod)
	}
}

func (t *nriTest) runContainer(options ...ContainerOption) (pod, ctr string) {
	var (
		podName = ids.GenPodName()
		ctrName = ids.GenCtrName()
		err     error
	)

	pod, err = crio.CreatePod(t.namespace, podName, ids.GenUID())
	require.NoError(t, err, "create pod")

	defaultCmd := WithShellScript(
		fmt.Sprintf("echo %s/%s/%s $(sleep 3600)", t.namespace, podName, ctrName),
	)
	options = append([]ContainerOption{defaultCmd}, options...)

	ctr, err = crio.CreateContainer(pod, ctrName, ids.GenUID(), options...)
	require.NoError(t, err, "create container")
	require.NoError(t, crio.StartContainer(ctr), "start container")

	return pod, ctr
}

func (t *nriTest) createPod() string {
	pod, err := crio.CreatePod(t.namespace, ids.GenPodName(), ids.GenUID())
	require.NoError(t, err, "create pod")

	return pod
}

func (t *nriTest) stopPod(pod string) {
	require.NoError(t, crio.StopPod(pod), "stop pod")
}

func (t *nriTest) removePod(pod string) {
	require.NoError(t, crio.RemovePod(pod), "remove pod")
}

func (t *nriTest) createContainer(pod string) string {
	ctr, err := crio.CreateContainer(pod, ids.GenCtrName(), ids.GenUID())
	require.NoError(t, err, "create container")

	return ctr
}

func (t *nriTest) startContainer(ctr string) {
	require.NoError(t, crio.StartContainer(ctr), "start container")
}

func (t *nriTest) stopContainer(ctr string) {
	require.NoError(t, crio.StopContainer(ctr), "stop container")
}

func (t *nriTest) removeContainer(ctr string) {
	require.NoError(t, crio.RemoveContainer(ctr), "remove container")
}

func (t *nriTest) execShellScript(ctr, cmd string) (stdout, stderr []byte, exitCode int32) {
	var err error
	stdout, stderr, exitCode, err = crio.ExecSync(ctr, []string{"sh", "-c", cmd})
	require.NoError(t, err, "exec in container %s", ctr)

	return stdout, stderr, exitCode
}

func (t *nriTest) verifyPodIDs(ids []string, pods []*api.PodSandbox, description string) {
	idMap := map[string]struct{}{}
	for _, id := range ids {
		idMap[id] = struct{}{}
	}

	for _, pod := range pods {
		_, ok := idMap[pod.GetId()]
		require.True(t, ok, description+"(missing pod ID %s)", pod.GetId())
	}
}

func (t *nriTest) verifyContainerIDs(ids []string, ctrs []*api.Container, description string) {
	idMap := map[string]struct{}{}
	for _, id := range ids {
		idMap[id] = struct{}{}
	}

	for _, ctr := range ctrs {
		_, ok := idMap[ctr.GetId()]
		require.True(t, ok, description)
	}
}
