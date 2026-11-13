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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCancellationFencesLateAdmission(t *testing.T) {
	m, err := New(t.TempDir(), "boot")
	if err != nil {
		t.Fatal(err)
	}

	if err = m.Cancel(
		context.Background(),
		1,
		"operation",
		func(context.Context, *Record) error { return nil },
	); err != nil {
		t.Fatal(err)
	}

	_, err = m.Run(
		context.Background(),
		1,
		"operation",
		"fingerprint",
		func(context.Context) ([]byte, error) {
			t.Fatal("late work executed")

			return nil, nil
		},
	)
	if !errors.Is(err, ErrExists) {
		t.Fatalf("got %v", err)
	}

	reopened, err := New(m.dir, "boot")
	if err != nil {
		t.Fatal(err)
	}

	_, err = reopened.Run(
		context.Background(),
		1,
		"operation",
		"fingerprint",
		func(context.Context) ([]byte, error) {
			t.Fatal("late work executed after restart")

			return nil, nil
		},
	)
	if !errors.Is(err, ErrExists) {
		t.Fatalf("got %v", err)
	}
}

func TestCancelWaitsForJoinedWork(t *testing.T) {
	m, err := New(t.TempDir(), "boot")
	if err != nil {
		t.Fatal(err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	returned := make(chan error, 1)

	go func() {
		_, err := m.Run(
			context.Background(),
			1,
			"operation",
			"fingerprint",
			func(ctx context.Context) ([]byte, error) {
				err := Join(ctx, func(child context.Context) error {
					close(entered)
					<-release

					if child.Err() == nil {
						t.Error("joined work did not observe cancellation")
					}

					return nil
				})

				return nil, err
			},
		)
		returned <- err
	}()

	<-entered

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	if err := m.Cancel(ctx, 1, "operation", func(context.Context, *Record) error {
		t.Error("cleanup before work joined")

		return nil
	}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}

	close(release)

	if err := <-returned; !errors.Is(err, ErrCanceled) {
		t.Fatalf("canceled work returned %v", err)
	}

	cleanups := 0

	if err = m.Cancel(context.Background(), 1, "operation", func(context.Context, *Record) error {
		cleanups++

		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err = m.Cancel(context.Background(), 1, "operation", func(context.Context, *Record) error {
		cleanups++

		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if cleanups != 1 {
		t.Fatalf("cleanup ran %d times", cleanups)
	}
}

func TestUncertainChildRequiresDifferentBoot(t *testing.T) {
	dir := t.TempDir()

	m, err := New(dir, "boot")
	if err != nil {
		t.Fatal(err)
	}

	childErr := errors.New("transport lost")

	_, err = m.Run(
		context.Background(),
		1,
		"operation",
		"fingerprint",
		func(ctx context.Context) ([]byte, error) {
			return nil, Join(ctx, func(context.Context) error { return childErr })
		},
	)
	if !errors.Is(err, childErr) {
		t.Fatal(err)
	}

	for _, boot := range []string{"boot", "next-boot"} {
		reopened, err := New(dir, boot)
		if err != nil {
			t.Fatal(err)
		}

		cleaned := false

		err = reopened.Cancel(
			context.Background(),
			1,
			"operation",
			func(context.Context, *Record) error {
				cleaned = true

				return nil
			},
		)
		if boot == "boot" {
			if !errors.Is(err, ErrUncertain) || cleaned {
				t.Fatalf("same-boot result: %v, cleaned=%v", err, cleaned)
			}
		} else if err != nil || !cleaned {
			t.Fatalf("new-boot result: %v, cleaned=%v", err, cleaned)
		}
	}
}

func TestAdmittedOperationsAreNeverReplayed(t *testing.T) {
	m, err := New(t.TempDir(), "boot")
	if err != nil {
		t.Fatal(err)
	}

	calls := 0

	fn := func(context.Context) ([]byte, error) {
		calls++

		return []byte("result"), nil
	}
	if _, err := m.Run(context.Background(), 1, "operation", "fingerprint", fn); err != nil {
		t.Fatal(err)
	}

	for _, fingerprint := range []string{"fingerprint", "other"} {
		if _, err := m.Run(
			context.Background(),
			1,
			"operation",
			fingerprint,
			fn,
		); !errors.Is(
			err,
			ErrExists,
		) {
			t.Fatalf("got %v", err)
		}
	}

	if calls != 1 {
		t.Fatalf("work ran %d times", calls)
	}
}

func TestFailedPreparationRecordsNoEffects(t *testing.T) {
	m, err := New(t.TempDir(), "boot")
	if err != nil {
		t.Fatal(err)
	}

	_, err = m.Run(
		context.Background(),
		2,
		"operation",
		"request",
		func(context.Context) ([]byte, error) { return nil, errors.New("validation failed") },
	)
	if err == nil {
		t.Fatal("expected validation error")
	}

	err = m.Cancel(context.Background(), 2, "operation", func(_ context.Context, r *Record) error {
		if r.MayHaveEffects || r.ResourceID != "" {
			t.Fatal("validation declared effects")
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNestedFailureCannotBeClearedByOuterJoin(t *testing.T) {
	m, err := New(t.TempDir(), "boot")
	if err != nil {
		t.Fatal(err)
	}

	_, err = m.Run(
		context.Background(),
		1,
		"operation",
		"request",
		func(ctx context.Context) ([]byte, error) {
			return nil, Join(ctx, func(ctx context.Context) error {
				if err := Join(ctx, func(context.Context) error {
					return errors.New("lost child")
				}); err == nil {
					t.Error("lost child was not reported")
				}

				return nil
			})
		},
	)
	if !errors.Is(err, ErrUncertain) {
		t.Fatalf("unresolved child returned normal success: %v", err)
	}

	_, err = m.Run(
		context.Background(),
		1,
		"operation",
		"request",
		func(context.Context) ([]byte, error) {
			t.Fatal("uncertain request was replayed")

			return nil, nil
		},
	)
	if !errors.Is(err, ErrExists) {
		t.Fatalf("unresolved child returned cached success: %v", err)
	}

	err = m.Cancel(context.Background(), 1, "operation", func(context.Context, *Record) error {
		t.Fatal("uncertain nested child was acknowledged")

		return nil
	})
	if !errors.Is(err, ErrUncertain) {
		t.Fatalf("got %v", err)
	}
}

func TestInterruptedOwnerWithJoinedChildrenCanRecover(t *testing.T) {
	m, err := New(t.TempDir(), "boot")
	if err != nil {
		t.Fatal(err)
	}

	e, err := m.load(2, "operation")
	if err != nil {
		t.Fatal(err)
	}

	e.mu.Lock()
	e.record.Fingerprint = "request"
	e.record.Active = true
	e.record.OwnerPID = os.Getpid()

	e.record.OwnerStart = "0"
	if err := m.save(e); err != nil {
		t.Fatal(err)
	}
	e.mu.Unlock()

	cleaned := false

	err = m.Cancel(context.Background(), 2, "operation", func(context.Context, *Record) error {
		cleaned = true

		return nil
	})
	if err != nil || !cleaned {
		t.Fatalf("got %v, cleaned=%v", err, cleaned)
	}
}

func TestPanicRetainsUncertainty(t *testing.T) {
	m, err := New(t.TempDir(), "boot")
	if err != nil {
		t.Fatal(err)
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected panic")
			}
		}()

		_, _ = m.Run( //nolint:errcheck // the call panics before returning
			context.Background(),
			1,
			"operation",
			"request",
			func(context.Context) ([]byte, error) { panic("interrupted owner") },
		)
	}()

	err = m.Cancel(context.Background(), 1, "operation", func(context.Context, *Record) error {
		t.Fatal("panic was acknowledged")

		return nil
	})
	if !errors.Is(err, ErrUncertain) {
		t.Fatal(err)
	}
}

func TestCanceledCallerDoesNotAdmitWork(t *testing.T) {
	m, err := New(t.TempDir(), "boot")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = m.Run(ctx, 1, "operation", "request", func(context.Context) ([]byte, error) {
		t.Fatal("cancelled request executed")

		return nil, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestCleanupPanicRetainsUncertainty(t *testing.T) {
	m, err := New(t.TempDir(), "boot")
	if err != nil {
		t.Fatal(err)
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected panic")
			}
		}()

		_ = m.Cancel( //nolint:errcheck // the call panics before returning
			context.Background(),
			1,
			"operation",
			func(context.Context, *Record) error { panic("interrupted cleanup") },
		)
	}()

	err = m.Cancel(context.Background(), 1, "operation", func(context.Context, *Record) error {
		t.Fatal("cleanup panic was acknowledged")

		return nil
	})
	if !errors.Is(err, ErrUncertain) {
		t.Fatal(err)
	}
}

func TestManagerRequiresPrivateAbsoluteStateDirectory(t *testing.T) {
	for _, dir := range []string{"", "relative"} {
		if _, err := New(dir, "boot"); err == nil {
			t.Fatalf("accepted %q", dir)
		}
	}

	dir := t.TempDir()
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}

	if _, err := New(dir, "boot"); err == nil {
		t.Fatal("accepted writable state directory")
	}
}

// Replacing the state directory with a file forces the actual persistence path
// to fail even when tests run as root.
func blockStateWrites(t *testing.T, dir string) func() {
	t.Helper()

	saved := dir + "-saved"
	if err := os.Rename(dir, saved); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(dir, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	return func() {
		if err := os.Remove(dir); err != nil {
			t.Fatal(err)
		}

		if err := os.Rename(saved, dir); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPersistenceFailuresNeverAcknowledge(t *testing.T) {
	t.Run("fence", func(t *testing.T) {
		m, err := New(filepath.Join(t.TempDir(), "state"), "boot")
		if err != nil {
			t.Fatal(err)
		}

		restore := blockStateWrites(t, m.dir)

		err = m.Cancel(context.Background(), 1, "operation", func(context.Context, *Record) error {
			t.Fatal("cleanup without durable fence")

			return nil
		})
		if err == nil {
			t.Fatal("failed fence was acknowledged")
		}

		restore()

		if err := m.Cancel(
			context.Background(),
			1,
			"operation",
			func(context.Context, *Record) error { return nil },
		); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("child intent", func(t *testing.T) {
		m, err := New(filepath.Join(t.TempDir(), "state"), "boot")
		if err != nil {
			t.Fatal(err)
		}

		_, err = m.Run(
			context.Background(),
			1,
			"operation",
			"request",
			func(ctx context.Context) ([]byte, error) {
				restore := blockStateWrites(t, m.dir)
				err := Join(ctx, func(context.Context) error {
					t.Fatal("child dispatched without durable intent")

					return nil
				})

				restore()

				return nil, err
			},
		)
		if err == nil {
			t.Fatal("failed child intent returned success")
		}
	})
	t.Run("completion", func(t *testing.T) {
		m, err := New(filepath.Join(t.TempDir(), "state"), "boot")
		if err != nil {
			t.Fatal(err)
		}

		var restore func()

		err = m.Cancel(context.Background(), 1, "operation", func(context.Context, *Record) error {
			restore = blockStateWrites(t, m.dir)

			return nil
		})
		if err == nil {
			t.Fatal("failed completion persistence was acknowledged")
		}

		restore()

		if err := m.Cancel(
			context.Background(),
			1,
			"operation",
			func(context.Context, *Record) error {
				t.Fatal("completed cleanup replayed")

				return nil
			},
		); err != nil {
			t.Fatal(err)
		}

		r, err := Read(m.dir, 1, "operation")
		if err != nil || !r.Settled || !r.Canceled {
			t.Fatalf("retry did not persist completion fence: %+v %v", r, err)
		}
	})
}

func TestAttachedRestoreCancellationRetainsBackendUncertainty(t *testing.T) {
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

	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	returned := make(chan error, 2)

	for range 2 {
		go func() {
			returned <- m.Attach(context.Background(), 2, "restore", func(ctx context.Context) error {
				entered <- struct{}{}

				<-release

				if ctx.Err() == nil {
					t.Error("admitted start did not observe cancellation")
				}

				return nil
			})
		}()
	}

	<-entered
	<-entered

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err = m.Cancel(ctx, 2, "restore", func(context.Context, *Record) error {
		t.Error("cleanup overtook admitted starts")

		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}

	err = m.Attach(context.Background(), 2, "restore", func(context.Context) error {
		t.Fatal("late start passed fence")

		return nil
	})
	if !errors.Is(err, ErrCanceled) {
		t.Fatal(err)
	}

	close(release)

	for range 2 {
		if err := <-returned; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	}

	if err := m.Cancel(context.Background(), 2, "restore", func(context.Context, *Record) error {
		t.Fatal("uncertain start was acknowledged")

		return nil
	}); !errors.Is(err, ErrUncertain) {
		t.Fatal(err)
	}

	reopened, err := New(m.dir, "boot")
	if err != nil {
		t.Fatal(err)
	}

	err = reopened.Attach(context.Background(), 2, "restore", func(context.Context) error {
		t.Fatal("late start passed durable fence")

		return nil
	})
	if !errors.Is(err, ErrCanceled) {
		t.Fatal(err)
	}
}

func TestFailedAttachedStartRetainsUncertainty(t *testing.T) {
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

	if err := m.Attach(
		context.Background(),
		2,
		"restore",
		func(context.Context) error { return errors.New("start transport lost") },
	); err == nil {
		t.Fatal("expected start failure")
	}

	err = m.Cancel(context.Background(), 2, "restore", func(context.Context, *Record) error {
		t.Fatal("failed start was acknowledged")

		return nil
	})
	if !errors.Is(err, ErrUncertain) {
		t.Fatal(err)
	}
}

func TestSynchronousJoinFailureRemainsRecoverable(t *testing.T) {
	for _, test := range []struct {
		name        string
		join        func(context.Context, func(context.Context) error) error
		recoverable bool
	}{
		{name: "detached worker", join: Join},
		{name: "synchronous call", join: JoinSynchronous, recoverable: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()

			m, err := New(dir, "boot")
			if err != nil {
				t.Fatal(err)
			}

			callErr := errors.New("pause failed")

			var recoverErr error

			_, err = m.Run(
				context.Background(),
				1,
				"checkpoint",
				"request",
				func(ctx context.Context) ([]byte, error) {
					if err := test.join(
						ctx,
						func(context.Context) error { return callErr },
					); !errors.Is(
						err,
						callErr,
					) {
						t.Errorf("join returned %v", err)
					}

					recoverErr = CanRecover(ctx, dir, "boot", 1, "checkpoint")

					return nil, callErr
				},
			)
			if !errors.Is(err, callErr) {
				t.Fatal(err)
			}

			if test.recoverable != (recoverErr == nil) {
				t.Fatalf("recovery inside the operation: %v", recoverErr)
			}

			if test.recoverable != (CanRecover(context.Background(), dir, "boot", 1, "checkpoint") == nil) {
				t.Fatal("recorded uncertainty does not match the failed call")
			}
		})
	}
}

func TestJournalCapacityRetainsExistingReplayFences(t *testing.T) {
	directory := t.TempDir()
	for i := range maxJournalRecords {
		record := Record{Version: 1, ID: fmt.Sprint(i), Kind: 1, BootID: "boot", Canceled: true, Settled: true}
		data, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		name, err := key(record.Kind, record.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	manager, err := New(directory, "boot")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.load(1, "new-operation"); err == nil || !strings.Contains(err.Error(), "capacity reached") {
		t.Fatalf("expected journal capacity error, got %v", err)
	}
	existing, err := manager.load(1, "0")
	if err != nil {
		t.Fatal(err)
	}
	if !existing.record.Canceled || !existing.record.Settled {
		t.Fatal("journal limit lost an existing replay fence")
	}
}

func TestJournalRejectsRecordsLargerThanRecoveryCanRead(t *testing.T) {
	manager, err := New(t.TempDir(), "boot")
	if err != nil {
		t.Fatal(err)
	}
	e := &entry{record: Record{Version: 1, ID: "oversized", Kind: 1, BootID: "boot", Response: bytes.Repeat([]byte("x"), maxJournalRecordSize)}}
	if err := manager.save(e); err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("expected record size error, got %v", err)
	}
	entries, err := os.ReadDir(manager.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatal("oversized response left durable or temporary records")
	}
}
