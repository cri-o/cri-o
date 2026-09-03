package oci

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/containerd/ttrpc"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"google.golang.org/protobuf/types/known/emptypb"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/pkg/config"
)

// countExecWaitSenders counts goroutines blocked sending inside the
// execContainerCommon wait worker. It is a secondary signal; the primary
// oracle is that Wait can return after the parent took the timeout
// Kill-failure path without stranding the worker.
func countExecWaitSenders() int {
	return countGoroutines("execContainerCommon", "chan send")
}

// fakeVMTask is a controllable task.TaskService for exec tests. Wait blocks
// until releaseWait is closed and then returns waitErr, mimicking a pending
// remote Wait that resolves after the parent timed out. Kill fails with
// killErr to mimic the ttrpc connection closing before/during the timeout
// kill, which makes the parent return without draining execCh.
type fakeVMTask struct {
	task.TaskService

	mu          sync.Mutex
	waitStarted chan struct{}
	releaseWait chan struct{}
	waitErr     error
	killErr     error
	waitCalls   int
	killCalls   int

	once sync.Once
}

func newFakeVMTask(waitErr, killErr error) *fakeVMTask {
	return &fakeVMTask{
		waitStarted: make(chan struct{}),
		releaseWait: make(chan struct{}),
		waitErr:     waitErr,
		killErr:     killErr,
	}
}

func (f *fakeVMTask) Exec(_ context.Context, _ *task.ExecProcessRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (f *fakeVMTask) Start(_ context.Context, _ *task.StartRequest) (*task.StartResponse, error) {
	return &task.StartResponse{}, nil
}

func (f *fakeVMTask) Delete(_ context.Context, _ *task.DeleteRequest) (*task.DeleteResponse, error) {
	return &task.DeleteResponse{}, nil
}

func (f *fakeVMTask) CloseIO(_ context.Context, _ *task.CloseIORequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (f *fakeVMTask) Wait(_ context.Context, _ *task.WaitRequest) (*task.WaitResponse, error) {
	f.mu.Lock()
	f.waitCalls++
	f.mu.Unlock()
	f.once.Do(func() { close(f.waitStarted) })

	<-f.releaseWait

	if f.waitErr != nil {
		return nil, f.waitErr
	}

	return &task.WaitResponse{ExitStatus: 0}, nil
}

func (f *fakeVMTask) Kill(_ context.Context, _ *task.KillRequest) (*emptypb.Empty, error) {
	f.mu.Lock()
	f.killCalls++
	f.mu.Unlock()

	if f.killErr != nil {
		return nil, f.killErr
	}

	return &emptypb.Empty{}, nil
}

// discardWriteCloser sinks exec output; Close is a no-op because the test
// never observes close signals.
type discardWriteCloser struct{}

func (discardWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (discardWriteCloser) Close() error                { return nil }

func newTestVMContainer(t *testing.T, id string) *Container {
	t.Helper()

	c, err := NewContainer(id, "name", t.TempDir(), t.TempDir()+"/log",
		map[string]string{}, map[string]string{}, map[string]string{},
		"image", nil, nil, "", &types.ContainerMetadata{}, "sandbox",
		false, false, false, "", t.TempDir(), time.Now(), "")
	if err != nil {
		t.Fatalf("create test container: %v", err)
	}

	c.SetSpec(&rspec.Spec{
		Process: &rspec.Process{
			Args: []string{"echo", "hi"},
			Cwd:  "/",
			Env:  []string{},
		},
	})

	return c
}

// TestRuntimeVMExecContainerCommonTimeoutKillFailure is a regression test for
// a goroutine leak in execContainerCommon: with an unbuffered execCh, a
// timeout followed by a failed Kill makes the parent return without ever
// receiving from execCh, so the wait worker blocks forever on its one-shot
// send once the pending Wait resolves. Buffering execCh (capacity 1) lets
// that late send complete.
func TestRuntimeVMExecContainerCommonTimeoutKillFailure(t *testing.T) {
	before := countExecWaitSenders()

	// The pending Wait resolves with a closed-connection error only after
	// the timeout Kill has failed.
	fake := newFakeVMTask(ttrpc.ErrClosed, errors.New("injected kill failure"))

	// Build through the constructor so typeurl registrations for the exec
	// spec survive, then swap in the controllable task service.
	root := t.TempDir()

	rr, ok := newRuntimeVM(&config.RuntimeHandler{RuntimeRoot: root}, t.TempDir()).(*runtimeVM)
	if !ok {
		t.Fatal("newRuntimeVM did not return *runtimeVM")
	}

	rr.task = fake
	rr.ctx = context.Background()
	r := rr
	c := newTestVMContainer(t, "vm-exec-leak")

	done := make(chan struct {
		code int32
		err  error
	}, 1)
	go func() {
		code, err := r.execContainerCommon(context.Background(), c, []string{"echo", "hi"}, 1, nil, discardWriteCloser{}, discardWriteCloser{}, false, nil)
		done <- struct {
			code int32
			err  error
		}{code, err}
	}()

	// Wait until the worker is inside the pending Wait, then let the
	// 1-second timeout fire and take the Kill-failure return. Surface an
	// early return (e.g. spec marshal or Exec/Start failure) instead of
	// hanging on waitStarted.
	select {
	case <-fake.waitStarted:
	case res := <-done:
		t.Fatalf("execContainerCommon returned early: code=%d err=%v", res.code, res.err)
	case <-time.After(10 * time.Second):
		t.Fatal("Wait never started")
	}

	var res struct {
		code int32
		err  error
	}
	select {
	case res = <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("execContainerCommon did not return after timeout")
	}

	if res.err == nil || !strings.Contains(res.err.Error(), "injected kill failure") {
		t.Fatalf("execContainerCommon error = %v, want injected kill failure", res.err)
	}

	// Release the pending Wait after the parent returned. Before the fix
	// this strands the worker on `execCh <- err`; after the fix the
	// buffered send completes and the worker exits.
	close(fake.releaseWait)

	deadline := time.Now().Add(10 * time.Second)
	for countExecWaitSenders()-before != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("execContainerCommon stranded %d sender goroutine(s)", countExecWaitSenders()-before)
		}

		time.Sleep(10 * time.Millisecond)
	}
}
