/*
Copyright The CRI-O Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package podoperation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRestoreDoesNotReplayCompletedAdmission(t *testing.T) {
	m, err := New(t.TempDir(), "boot")
	if err != nil {
		t.Fatal(err)
	}

	calls := 0

	fn := func(context.Context) ([]byte, error) {
		calls++

		return []byte("prepared"), nil
	}
	if _, err := m.Run(context.Background(), 2, "restore", "request", fn); err != nil {
		t.Fatal(err)
	}

	if _, err := m.Run(context.Background(), 2, "restore", "request", fn); err == nil {
		t.Fatal("duplicate restore returned cached success")
	}

	if calls != 1 {
		t.Fatalf("restore executed %d times", calls)
	}
}

func TestRestoreMetadataOwnershipSurvivesRestartAndCleanupFailure(t *testing.T) {
	dir := t.TempDir()

	m, err := New(dir, "boot")
	if err != nil {
		t.Fatal(err)
	}

	const target = "namespace/pod/uid/1"

	resource := filepath.Join(t.TempDir(), "prepared")
	calls := 0

	create := func(context.Context) ([]byte, error) {
		calls++

		return nil, os.WriteFile(resource, []byte("original"), 0o600)
	}
	if _, err := m.RunTarget(
		context.Background(),
		2,
		"original",
		"request",
		target,
		create,
	); err != nil {
		t.Fatal(err)
	}

	m, err = New(dir, "boot")
	if err != nil {
		t.Fatal(err)
	}

	duplicate := func(context.Context) ([]byte, error) {
		t.Fatal("duplicate reached backend")

		return nil, nil
	}
	if _, err := m.RunTarget(
		context.Background(),
		2,
		"duplicate",
		"request",
		target,
		duplicate,
	); !errors.Is(
		err,
		ErrExists,
	) {
		t.Fatal(err)
	}

	failedCleanup := errors.New("owned mount is still busy")
	if err := m.Cancel(
		context.Background(),
		2,
		"original",
		func(context.Context, *Record) error { return failedCleanup },
	); !errors.Is(
		err,
		failedCleanup,
	) {
		t.Fatal(err)
	}

	if _, err := m.RunTarget(
		context.Background(),
		2,
		"duplicate",
		"request",
		target,
		duplicate,
	); !errors.Is(
		err,
		ErrExists,
	) {
		t.Fatal(err)
	}

	data, err := os.ReadFile(resource)
	if err != nil || string(data) != "original" || calls != 1 {
		t.Fatalf("duplicate or cleanup changed original: %q %v calls=%d", data, err, calls)
	}

	if err := m.Cancel(
		context.Background(),
		2,
		"original",
		func(context.Context, *Record) error { return os.Remove(resource) },
	); err != nil {
		t.Fatal(err)
	}

	if _, err := m.RunTarget(
		context.Background(),
		2,
		"original",
		"request",
		target,
		duplicate,
	); !errors.Is(
		err,
		ErrExists,
	) {
		t.Fatal(err)
	}

	if _, err := m.RunTarget(
		context.Background(),
		2,
		"fresh",
		"request",
		target,
		create,
	); err != nil {
		t.Fatal(err)
	}

	if calls != 2 {
		t.Fatalf("fresh operation executed %d times", calls)
	}
}

func TestRestoreMetadataRejectsConcurrentAdmission(t *testing.T) {
	m, err := New(t.TempDir(), "boot")
	if err != nil {
		t.Fatal(err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	returned := make(chan error, 1)

	go func() {
		_, err := m.RunTarget(
			context.Background(),
			2,
			"original",
			"request",
			"ns/pod/uid/0",
			func(context.Context) ([]byte, error) {
				close(entered)
				<-release

				return []byte("original"), nil
			},
		)
		returned <- err
	}()

	<-entered

	_, err = m.RunTarget(
		context.Background(),
		2,
		"duplicate",
		"other",
		"ns/pod/uid/0",
		func(context.Context) ([]byte, error) {
			t.Fatal("duplicate ran")

			return nil, nil
		},
	)
	if !errors.Is(err, ErrExists) {
		t.Fatal(err)
	}

	close(release)

	if err := <-returned; err != nil {
		t.Fatal(err)
	}

	record, err := Read(m.dir, 2, "original")
	if err != nil || !record.Succeeded || record.Canceled || string(record.Response) != "original" {
		t.Fatalf("duplicate disturbed original: %+v %v", record, err)
	}
}

func TestBackendSettlementRequiredBeforeReleasingMetadata(t *testing.T) {
	m, err := New(t.TempDir(), "boot")
	if err != nil {
		t.Fatal(err)
	}

	_, err = m.RunTarget(
		context.Background(),
		2,
		"original",
		"request",
		"ns/pod/uid/0",
		func(ctx context.Context) ([]byte, error) {
			return nil, Join(
				ctx,
				func(context.Context) error { return errors.New("conmon response was lost") },
			)
		},
	)
	if err == nil {
		t.Fatal("expected backend uncertainty")
	}

	cleaned := false

	cleanup := func(context.Context, *Record) error {
		cleaned = true

		return nil
	}
	if err := m.Cancel(
		context.Background(),
		2,
		"original",
		cleanup,
	); !errors.Is(err, ErrUncertain) ||
		cleaned {
		t.Fatalf("transport error acknowledged: %v cleaned=%v", err, cleaned)
	}

	backendErr := errors.New("backend cannot yet prove completion")
	if err := m.Settle(
		context.Background(),
		2,
		"original",
		func(context.Context, *Record) error { return backendErr },
		cleanup,
	); !errors.Is(err, backendErr) ||
		cleaned {
		t.Fatalf("failed backend proof acknowledged: %v cleaned=%v", err, cleaned)
	}

	if err := m.Settle(
		context.Background(),
		2,
		"original",
		func(context.Context, *Record) error { return nil },
		cleanup,
	); err != nil ||
		!cleaned {
		t.Fatalf("settled backend not cleaned: %v cleaned=%v", err, cleaned)
	}

	if _, err := m.RunTarget(
		context.Background(),
		2,
		"fresh",
		"request",
		"ns/pod/uid/0",
		func(context.Context) ([]byte, error) { return nil, nil },
	); err != nil {
		t.Fatal(err)
	}
}

func TestSuccessfulPreparationAndStartsSurviveRequestContextClosure(t *testing.T) {
	m, err := New(t.TempDir(), "boot")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	var owner context.Context

	_, err = m.RunTarget(
		ctx,
		2,
		"restore",
		"request",
		"ns/pod/uid/0",
		func(ctx context.Context) ([]byte, error) {
			owner = ctx //nolint:fatcontext // checked after the RPC returns

			return []byte("prepared"), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	cancel()

	if owner.Err() != nil {
		t.Fatal("normal preparation RPC teardown canceled runtime ownership")
	}

	for _, resource := range []string{"first", "second"} {
		ctx, cancel := context.WithCancel(context.Background())
		if err := m.AttachResource(
			ctx,
			2,
			"restore",
			resource,
			func(context.Context) error { return nil },
		); err != nil {
			t.Fatal(err)
		}

		cancel()

		if owner.Err() != nil {
			t.Fatal("normal start RPC teardown canceled runtime ownership")
		}
	}

	record, err := Read(m.dir, 2, "restore")
	if err != nil || record.Canceled || record.Uncertain || !record.Succeeded {
		t.Fatalf("successful RPC teardown changed ownership: %+v %v", record, err)
	}
}

func TestCancellationWinsBeforePreparationHandoff(t *testing.T) {
	m, err := New(t.TempDir(), "boot")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	entered := make(chan struct{})
	release := make(chan struct{})
	returned := make(chan error, 1)

	go func() {
		_, err := m.RunTarget(
			ctx,
			2,
			"restore",
			"request",
			"ns/pod/uid/0",
			func(context.Context) ([]byte, error) {
				close(entered)
				<-release

				return []byte("prepared"), nil
			},
		)
		returned <- err
	}()

	<-entered
	cancel()
	close(release)

	if err := <-returned; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}

	record, err := Read(m.dir, 2, "restore")
	if err != nil || record.Succeeded {
		t.Fatalf("interrupted preparation committed success: %+v %v", record, err)
	}
}

func TestAmbiguousAdmissionWriteRetainsMetadataOwnership(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires directory read permission checks")
	}

	dir := t.TempDir()

	m, err := New(dir, "boot")
	if err != nil {
		t.Fatal(err)
	}

	requireNoDispatch := func(context.Context) ([]byte, error) {
		t.Fatal("backend dispatched after failed durable admission")

		return nil, nil
	}

	if err := os.Chmod(dir, 0o300); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := os.Chmod(dir, 0o700); err != nil {
			t.Error(err)
		}
	})

	_, err = m.RunTarget(
		context.Background(),
		2,
		"original",
		"request",
		"ns/pod/uid/0",
		requireNoDispatch,
	)
	if err == nil {
		t.Fatal("expected directory sync/open failure after record rename")
	}

	if _, err := Read(dir, 2, "original"); err != nil {
		t.Fatalf("test did not reach the renamed-record boundary: %v", err)
	}

	_, err = m.RunTarget(
		context.Background(),
		2,
		"duplicate",
		"request",
		"ns/pod/uid/0",
		requireNoDispatch,
	)
	if !errors.Is(err, ErrExists) {
		t.Fatalf("ambiguous admission released the metadata identity: %v", err)
	}

	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := m.Cancel(
		context.Background(),
		2,
		"original",
		func(context.Context, *Record) error { return nil },
	); err != nil {
		t.Fatal(err)
	}

	if _, err := m.RunTarget(
		context.Background(),
		2,
		"fresh",
		"request",
		"ns/pod/uid/0",
		func(context.Context) ([]byte, error) { return nil, nil },
	); err != nil {
		t.Fatal(err)
	}
}

func TestContainerRemovalFencesOnlyOwnedResource(t *testing.T) {
	m, err := New(t.TempDir(), "boot")
	if err != nil {
		t.Fatal(err)
	}

	_, err = m.RunTarget(
		context.Background(),
		2,
		"restore",
		"request",
		"metadata",
		func(ctx context.Context) ([]byte, error) { return nil, SetResource(ctx, "sandbox") },
	)
	if err != nil {
		t.Fatal(err)
	}

	entered, release := make(chan struct{}), make(chan struct{})

	started := make(chan error, 1)
	go func() {
		started <- m.AttachResource(context.Background(), 2, "restore", "first", func(ctx context.Context) error {
			if err := RegisterBackend(ctx, "first"); err != nil {
				return err
			}

			close(entered)
			<-release

			return nil
		})
	}()

	<-entered

	if err := m.AttachResource(
		context.Background(),
		2,
		"restore",
		"first",
		func(context.Context) error {
			t.Error("duplicate start ran")

			return nil
		},
	); !errors.Is(
		err,
		ErrExists,
	) {
		t.Fatal(err)
	}

	short, cancel := context.WithCancel(context.Background())
	cancel()

	removed := false
	if err := m.RemoveResource(short, 2, "restore", "first", nil, func(context.Context) error {
		removed = true

		return nil
	}); !errors.Is(err, context.Canceled) || removed {
		t.Fatalf("removal overtook start: %v removed=%v", err, removed)
	}

	if err := m.AttachResource(
		context.Background(),
		2,
		"restore",
		"first",
		func(context.Context) error {
			t.Error("fenced start ran")

			return nil
		},
	); !errors.Is(
		err,
		ErrExists,
	) {
		t.Fatal(err)
	}

	if err := m.AttachResource(
		context.Background(),
		2,
		"restore",
		"sibling",
		func(ctx context.Context) error { return ctx.Err() },
	); err != nil {
		t.Fatal(err)
	}

	close(release)

	if err := <-started; err != nil {
		t.Fatal(err)
	}

	proofs := 0

	removeErr := errors.New("storage is busy")
	if err := m.RemoveResource(
		context.Background(),
		2,
		"restore",
		"first",
		func(context.Context, *Record) error {
			proofs++

			return nil
		},
		func(context.Context) error { return removeErr },
	); !errors.Is(
		err,
		removeErr,
	) {
		t.Fatal(err)
	}

	m, err = New(m.dir, "boot")
	if err != nil {
		t.Fatal(err)
	}

	if err := m.RemoveResource(
		context.Background(),
		2,
		"restore",
		"first",
		func(context.Context, *Record) error {
			t.Error("repeated settled backend")

			return nil
		},
		func(context.Context) error {
			removed = true

			return nil
		},
	); err != nil {
		t.Fatal(err)
	}

	if !removed || proofs != 1 {
		t.Fatalf("removed=%v proofs=%d", removed, proofs)
	}

	record, err := Read(m.dir, 2, "restore")
	if err != nil || record.Canceled || record.Settled || len(record.RemovedResources) != 1 {
		t.Fatalf("single-container removal changed Pod owner: %+v %v", record, err)
	}

	if _, err := m.RunTarget(
		context.Background(),
		2,
		"new",
		"request",
		"metadata",
		func(context.Context) ([]byte, error) {
			t.Error("target released before sandbox removal")

			return nil, nil
		},
	); !errors.Is(
		err,
		ErrExists,
	) {
		t.Fatal(err)
	}

	if err := m.AttachResource(
		context.Background(),
		2,
		"restore",
		"later-sibling",
		func(context.Context) error {
			t.Error("failed cleanup uncertainty was discarded")

			return nil
		},
	); !errors.Is(
		err,
		ErrUncertain,
	) {
		t.Fatal(err)
	}

	if owner, err := m.OperationForResource(2, "sandbox"); err != nil || owner != "restore" {
		t.Fatalf("resource lookup: %q %v", owner, err)
	}
}

func TestResourceRemovalPanicReleasesWaitersAndRetainsUncertainty(t *testing.T) {
	m, err := New(t.TempDir(), "boot")
	if err != nil {
		t.Fatal(err)
	}

	_, err = m.Run(
		context.Background(),
		2,
		"restore",
		"request",
		func(context.Context) ([]byte, error) { return nil, nil },
	)
	if err != nil {
		t.Fatal(err)
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Error("missing removal panic")
			}
		}()

		_ = m.RemoveResource( //nolint:errcheck // the call panics before returning
			context.Background(),
			2,
			"restore",
			"container",
			nil,
			func(context.Context) error { panic("remove panicked") },
		)
	}()

	if err := m.Cancel(context.Background(), 2, "restore", func(context.Context, *Record) error {
		t.Error("cleanup bypassed uncertainty")

		return nil
	}); !errors.Is(err, ErrUncertain) {
		t.Fatal(err)
	}

	record, err := Read(m.dir, 2, "restore")
	if err != nil || record.Active || !record.Uncertain {
		t.Fatalf("panic lost ownership: %+v %v", record, err)
	}
}

func TestCleanupIdentityDoesNotBypassAnotherOperation(t *testing.T) {
	m, err := New(t.TempDir(), "boot")
	if err != nil {
		t.Fatal(err)
	}

	if err := m.Cancel(
		context.Background(),
		2,
		"restore",
		func(ctx context.Context, _ *Record) error {
			if !IsCleanupFor(ctx, 2, "restore") || IsCleanupFor(ctx, 2, "other") ||
				IsCleanupFor(ctx, 1, "restore") {
				t.Error("cleanup owner identity was not enforced")
			}

			return nil
		},
	); err != nil {
		t.Fatal(err)
	}
}

func TestContainerRemovalAfterWholeSettlementIsIdempotent(t *testing.T) {
	m, err := New(t.TempDir(), "boot")
	if err != nil {
		t.Fatal(err)
	}

	_, err = m.RunTarget(
		context.Background(),
		2,
		"restore",
		"request",
		"metadata",
		func(context.Context) ([]byte, error) { return nil, nil },
	)
	if err != nil {
		t.Fatal(err)
	}

	entered, release := make(chan struct{}), make(chan struct{})

	settled := make(chan error, 1)
	go func() {
		settled <- m.Cancel(context.Background(), 2, "restore", func(context.Context, *Record) error {
			close(entered)
			<-release

			return nil
		})
	}()

	<-entered

	removed := make(chan error, 1)
	go func() {
		removed <- m.RemoveResource(context.Background(), 2, "restore", "container", nil, func(context.Context) error {
			t.Error("removed a resource after whole-target cleanup")

			return errors.New("must not dispatch")
		})
	}()

	entry, err := m.load(2, "restore")
	if err != nil {
		t.Fatal(err)
	}
	// Removal has no signal for reaching the barrier. Allow a slow race-enabled
	// run to get there; a successful run stops polling immediately.
	deadline := time.Now().Add(30 * time.Second)

	for {
		entry.mu.Lock()
		waiting := len(entry.record.FencedResources) > 0
		entry.mu.Unlock()

		if waiting {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal("container removal never reached the whole-cleanup barrier")
		}

		time.Sleep(time.Millisecond)
	}

	close(release)

	if err := <-settled; err != nil {
		t.Fatal(err)
	}

	if err := <-removed; err != nil {
		t.Fatal(err)
	}

	record, err := Read(m.dir, 2, "restore")
	if err != nil || !record.Settled || record.Active || record.Uncertain || record.ChildWork != 0 {
		t.Fatalf("settled journal was reactivated: %+v %v", record, err)
	}
}

func TestResourceRemovalWaitsForRunningRestore(t *testing.T) {
	m, err := New(t.TempDir(), "boot")
	if err != nil {
		t.Fatal(err)
	}

	prepared := make(chan struct{})
	release := make(chan struct{})
	restored := make(chan error, 1)

	go func() {
		_, err := m.Run(
			context.Background(),
			2,
			"restore",
			"request",
			func(ctx context.Context) ([]byte, error) {
				if err := SetResource(ctx, "sandbox"); err != nil {
					return nil, err
				}

				close(prepared)
				<-release

				return nil, nil
			},
		)
		restored <- err
	}()

	<-prepared

	removed := make(chan error, 1)
	go func() {
		removed <- m.RemoveResource(context.Background(), 2, "restore", "container", nil, func(context.Context) error { return nil })
	}()

	select {
	case err := <-removed:
		t.Fatalf("removal overtook the running restore: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	if err := <-restored; err != nil {
		t.Fatal(err)
	}

	if err := <-removed; err != nil {
		t.Fatal(err)
	}
}

func TestFailedStartBlocksLaterStartsWhileSiblingRuns(t *testing.T) {
	m, err := New(t.TempDir(), "boot")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.Run(
		context.Background(),
		2,
		"restore",
		"request",
		func(context.Context) ([]byte, error) { return nil, nil },
	); err != nil {
		t.Fatal(err)
	}

	running := make(chan struct{})
	release := make(chan struct{})

	sibling := make(chan error, 1)
	go func() {
		sibling <- m.AttachResource(context.Background(), 2, "restore", "running", func(context.Context) error {
			close(running)
			<-release

			return nil
		})
	}()

	<-running

	startErr := errors.New("restore failed")
	if err := m.AttachResource(
		context.Background(),
		2,
		"restore",
		"failed",
		func(ctx context.Context) error {
			return Join(ctx, func(context.Context) error { return startErr })
		},
	); !errors.Is(
		err,
		startErr,
	) {
		t.Fatal(err)
	}

	if err := m.AttachResource(
		context.Background(),
		2,
		"restore",
		"later",
		func(context.Context) error {
			t.Error("a start was admitted after a failed sibling start")

			return nil
		},
	); !errors.Is(
		err,
		ErrUncertain,
	) {
		t.Fatal(err)
	}

	close(release)

	if err := <-sibling; !errors.Is(err, ErrUncertain) {
		t.Fatal(err)
	}
}
