package oci

import (
	"context"
	"errors"
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

			c.state.InitStartTime = "invalid"

			alive, err = impl.IsContainerAlive(context.Background(), c)
			if err != nil || alive {
				t.Fatalf("stale PID: alive=%v, err=%v", alive, err)
			}
		})
	}
}

func TestExecRejectsNotRunningContainer(t *testing.T) {
	sync := func(impl RuntimeImpl, c *Container) error {
		_, err := impl.ExecSyncContainer(context.Background(), c, []string{"true"}, 0)

		return err
	}

	for name, tc := range map[string]struct {
		impl RuntimeImpl
		exec func(RuntimeImpl, *Container) error
	}{
		"oci exec": {&runtimeOCI{}, func(impl RuntimeImpl, c *Container) error {
			return impl.ExecContainer(
				context.Background(), c, []string{"true"}, nil, nil, nil, false, nil,
			)
		}},
		"oci exec sync": {&runtimeOCI{}, sync},
		"pod exec sync": {&runtimePod{}, sync},
	} {
		t.Run(name, func(t *testing.T) {
			c := &Container{state: &ContainerState{}}

			err := tc.exec(tc.impl, c)
			if !errors.Is(err, ErrContainerNotRunning) {
				t.Fatalf("expected ErrContainerNotRunning, got %v", err)
			}
		})
	}
}
