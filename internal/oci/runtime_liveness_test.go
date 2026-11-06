package oci

import (
	"context"
	"os"
	"testing"

	specs "github.com/opencontainers/runtime-spec/specs-go"
)

func TestNonVMLiveness(t *testing.T) {
	for name, impl := range map[string]RuntimeImpl{
		"oci": &runtimeOCI{},
		"pod": &runtimePod{},
	} {
		t.Run(name, func(t *testing.T) {
			c := &Container{state: &ContainerState{
				State: specs.State{Status: ContainerStateRunning, Pid: os.Getpid()},
			}}
			if err := c.state.SetInitPid(os.Getpid()); err != nil {
				t.Fatal(err)
			}

			alive, err := impl.IsContainerAlive(context.Background(), c)
			if err != nil || !alive {
				t.Fatalf("live PID: alive=%v, err=%v", alive, err)
			}

			// A reused PID must still be rejected even if that process is alive.
			c.state.InitStartTime = "invalid"

			alive, err = impl.IsContainerAlive(context.Background(), c)
			if err != nil || alive {
				t.Fatalf("stale PID: alive=%v, err=%v", alive, err)
			}
		})
	}
}
