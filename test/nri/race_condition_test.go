package nri

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRunPodSandboxRemoveRaceCondition tests the race condition where
// RemovePodSandbox is called while RunPodSandbox is still in progress.
func TestRunPodSandboxRemoveRaceCondition(t *testing.T) {
	oldTimeout := requestTimeout
	requestTimeout = 30 * time.Second

	defer func() { requestTimeout = oldTimeout }()

	test := &nriTest{
		plugins: make([]*plugin, 1),
		options: [][]PluginOption{
			{
				// Configure 10 second delay in RunPodSandbox hook
				// This keeps the sandbox in "not created" state
				WithRunPodSandboxDelay(10 * time.Second),
			},
		},
	}

	test.Setup(t)
	test.StartPlugins(WaitForPluginSync)

	pod := test.createPod()
	require.NotEmpty(t, pod, "create pod should return pod ID")

	time.Sleep(2 * time.Second)

	err := crio.RemovePod(pod)
	if err != nil {
		t.Logf("RemovePod during creation returned error (expected): %v", err)
	} else {
		t.Logf("RemovePod during creation succeeded (may indicate race handling)")
	}

	time.Sleep(9 * time.Second)

	err = crio.RemovePod(pod)
	if err != nil {
		t.Logf("RemovePod after creation completed: %v", err)
	}

	runEvent := test.plugins[0].WaitEvent(RunPodEvent(pod), 1*time.Second)
	require.NotNil(t, runEvent, "should receive RunPodSandbox event")
	require.Equal(t, "RunPodSandbox", runEvent.kind, "event should be RunPodSandbox")
}

// TestRunPodSandboxWithDelayTiming verifies that the NRI delay
// actually works as expected and the timing is correct.
func TestRunPodSandboxWithDelayTiming(t *testing.T) {
	oldTimeout := requestTimeout
	requestTimeout = 30 * time.Second

	defer func() { requestTimeout = oldTimeout }()

	delayDuration := 5 * time.Second

	test := &nriTest{
		plugins: make([]*plugin, 1),
		options: [][]PluginOption{
			{
				WithRunPodSandboxDelay(delayDuration),
			},
		},
	}

	test.Setup(t)
	test.StartPlugins(WaitForPluginSync)

	startTime := time.Now()
	pod := test.createPod()
	elapsed := time.Since(startTime)

	require.NotEmpty(t, pod, "create pod should return pod ID")

	t.Logf("Pod creation took %v (expected >= %v)", elapsed, delayDuration)
	require.GreaterOrEqual(t, elapsed, delayDuration,
		"pod creation should take at least %v due to NRI delay", delayDuration)

	test.removePod(pod)
}
