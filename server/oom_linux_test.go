package server

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"

	"github.com/containerd/containerd/sys"
	"github.com/opencontainers/runtime-tools/generate"

	"github.com/cri-o/cri-o/internal/annotations"
)

func TestRestrictOOMScoreAdj(t *testing.T) {
	t.Parallel()

	intPtr := func(i int) *int { return &i }

	for _, tc := range []struct {
		name      string
		adj       *int
		lowest    int
		err       error
		expected  *int
		requested string
	}{
		{
			name:      "raises a lower value",
			adj:       intPtr(-998),
			lowest:    -500,
			expected:  intPtr(-500),
			requested: "-998",
		},
		{
			name:      "raises a lower value to zero",
			adj:       intPtr(-997),
			lowest:    0,
			expected:  intPtr(0),
			requested: "-997",
		},
		{name: "keeps an equal value", adj: intPtr(-500), lowest: -500, expected: intPtr(-500)},
		{name: "keeps a higher value", adj: intPtr(1000), lowest: 0, expected: intPtr(1000)},
		{name: "keeps an unset value", lowest: 0},
		{
			name:     "keeps the value if the lowest one is unknown",
			adj:      intPtr(-998),
			err:      errors.New("read error"),
			expected: intPtr(-998),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, err := generate.New("linux")
			if err != nil {
				t.Fatalf("generate.New: %v", err)
			}

			g.Config.Process.OOMScoreAdj = tc.adj

			restrictOOMScoreAdj(context.Background(), &g, func() (int, error) {
				return tc.lowest, tc.err
			})

			got := g.Config.Process.OOMScoreAdj
			switch {
			case tc.expected == nil && got != nil:
				t.Fatalf("expected no OOM score adjustment, got %d", *got)
			case tc.expected != nil && got == nil:
				t.Fatalf("expected OOM score adjustment %d, got none", *tc.expected)
			case tc.expected != nil && *got != *tc.expected:
				t.Fatalf("expected OOM score adjustment %d, got %d", *tc.expected, *got)
			}

			requested, ok := g.Config.Annotations[annotations.RequestedOOMScoreAdj]
			if tc.requested == "" && ok {
				t.Fatalf("expected no requested OOM score adjustment, got %q", requested)
			}

			if tc.requested != requested {
				t.Fatalf(
					"expected requested OOM score adjustment %q, got %q",
					tc.requested, requested,
				)
			}
		})
	}
}

func TestProbeMinOOMScoreAdj(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		current  int
		min      int
		expected int
		restored bool
	}{
		{name: "minimum below the current value", current: 200, min: 100, expected: 100, restored: true},
		{name: "minimum equal to the current value", current: 0, min: 0, expected: 0, restored: true},
		{name: "lowest possible minimum", current: 0, min: -1000, expected: -1000, restored: true},
		{name: "negative minimum", current: -500, min: -998, expected: -998, restored: true},
		{name: "lowest possible current value", current: -1000, min: -1000, expected: -1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			value := tc.current
			writes := 0

			lowest, err := probeMinOOMScoreAdj(
				func() (int, error) { return value, nil },
				func(adj int) error {
					writes++

					if adj < tc.min {
						return &os.PathError{
							Op: "write", Path: ownOOMScoreAdjPath, Err: syscall.EACCES,
						}
					}

					value = adj

					return nil
				},
			)
			if err != nil {
				t.Fatalf("probeMinOOMScoreAdj: %v", err)
			}

			if lowest != tc.expected {
				t.Fatalf("expected minimum %d, got %d", tc.expected, lowest)
			}

			if value != tc.current {
				t.Fatalf("expected the current value %d to be restored, got %d", tc.current, value)
			}

			if tc.restored != (writes > 0) {
				t.Fatalf("expected writes: %t, got %d", tc.restored, writes)
			}
		})
	}
}

func TestProbeMinOOMScoreAdjReadError(t *testing.T) {
	t.Parallel()

	_, err := probeMinOOMScoreAdj(
		func() (int, error) { return 0, errors.New("read error") },
		func(int) error {
			t.Fatal("unexpected write")

			return nil
		},
	)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestProbeMinOOMScoreAdjRestoreError(t *testing.T) {
	t.Parallel()

	current := 0

	_, err := probeMinOOMScoreAdj(
		func() (int, error) { return current, nil },
		func(adj int) error {
			if adj == current {
				return errors.New("write error")
			}

			return nil
		},
	)
	if err == nil {
		t.Fatal("expected an error")
	}
}

// TestProbeMinOOMScoreAdjOwnProcess probes the minimum of the test process,
// which gets its current value restored.
func TestProbeMinOOMScoreAdjOwnProcess(t *testing.T) {
	read := func() (int, error) { return sys.GetOOMScoreAdj(os.Getpid()) }

	before, err := read()
	if err != nil {
		t.Fatalf("read OOM score adjustment: %v", err)
	}

	lowest, err := probeMinOOMScoreAdj(read, writeOwnOOMScoreAdj)
	if err != nil {
		t.Fatalf("probeMinOOMScoreAdj: %v", err)
	}

	if lowest < sys.OOMScoreAdjMin || lowest > before {
		t.Fatalf("minimum %d not between %d and %d", lowest, sys.OOMScoreAdjMin, before)
	}

	after, err := read()
	if err != nil {
		t.Fatalf("read OOM score adjustment: %v", err)
	}

	if after != before {
		t.Fatalf("expected OOM score adjustment %d to be restored, got %d", before, after)
	}

	t.Logf("OOM score adjustment %d, minimum %d", before, lowest)

	// The value below the minimum gets rejected.
	if lowest > sys.OOMScoreAdjMin {
		if err := writeOwnOOMScoreAdj(lowest - 1); err == nil {
			if err := writeOwnOOMScoreAdj(before); err != nil {
				t.Logf("restore OOM score adjustment: %v", err)
			}

			t.Fatalf("expected %s to reject %d", ownOOMScoreAdjPath, lowest-1)
		}
	}
}
