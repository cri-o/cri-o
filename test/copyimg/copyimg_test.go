package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	imagecopy "go.podman.io/image/v5/copy"
	"go.podman.io/image/v5/directory"
	"go.podman.io/image/v5/signature"
	"go.podman.io/image/v5/transports/alltransports"
	"go.podman.io/image/v5/types"
)

func TestCleanupDirDestination(t *testing.T) {
	t.Parallel()

	t.Run("removes a dir destination", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "leftover-blob"), nil, 0o600); err != nil {
			t.Fatalf("write test file: %v", err)
		}

		if err := cleanupDirDestination("dir:" + dir); err != nil {
			t.Fatalf("cleanup dir destination: %v", err)
		}

		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("expected %q to be removed, got error: %v", dir, err)
		}
	})

	t.Run("ignores other transports", func(t *testing.T) {
		t.Parallel()

		if err := cleanupDirDestination("docker://quay.io/crio/fedora-crio-ci:latest"); err != nil {
			t.Fatalf("cleanup non-dir destination: %v", err)
		}
	})

	t.Run("ignores an empty path", func(t *testing.T) {
		t.Parallel()

		if err := cleanupDirDestination("dir:"); err != nil {
			t.Fatalf("cleanup empty dir destination: %v", err)
		}
	})
}

func TestDestinationIsForeign(t *testing.T) {
	t.Parallel()

	if !destinationIsForeign(
		fmt.Errorf("initializing destination: %w", directory.ErrNotContainerImageDir),
	) {
		t.Error("expected a directory image error to be foreign")
	}

	if !destinationIsForeign(
		fmt.Errorf("initializing destination: %w", directory.UnsupportedVersionError{
			Version: "9.9",
			Path:    "/tmp/destination",
		}),
	) {
		t.Error("expected an unsupported version error to be foreign")
	}

	if destinationIsForeign(errors.New("connection reset")) {
		t.Error("expected an unrelated error not to be foreign")
	}
}

func TestCopyImageWithRetryCleansUpDestination(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		// staleFile makes the destination a regular file, so that the first
		// attempt fails for a different reason than the retries.
		staleFile     bool
		retryAttempts int
		cleanupCalls  int
		checkFirstErr bool
	}{
		{
			name:          "single attempt",
			retryAttempts: 1,
			cleanupCalls:  1,
		},
		{
			name:          "every retry starts from a clean destination",
			retryAttempts: 2,
			cleanupCalls:  2,
		},
		{
			// The first attempt fails while checking the destination, the
			// second one while initializing the source.
			name:          "reports the first error of the attempts",
			staleFile:     true,
			retryAttempts: 2,
			cleanupCalls:  2,
			checkFirstErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			destDir := t.TempDir()
			if tc.staleFile {
				destDir = filepath.Join(destDir, "destination")
				if err := os.WriteFile(destDir, nil, 0o600); err != nil {
					t.Fatalf("write destination file: %v", err)
				}
			}

			// A copy from an empty source directory fails while initializing
			// the source, so it never writes anything to the destination.
			srcDir := t.TempDir()

			cleanupCalls := 0

			err := copyImageWithRetry(
				context.Background(),
				newTestPolicyContext(t),
				parseDirRef(t, destDir),
				parseDirRef(t, srcDir),
				&imagecopy.Options{},
				tc.retryAttempts,
				func() error {
					cleanupCalls++

					return cleanupDirDestination("dir:" + destDir)
				},
			)
			if err == nil {
				t.Fatal("expected the copy to fail")
			}

			if cleanupCalls != tc.cleanupCalls {
				t.Fatalf(
					"expected the cleanup to be called %d times, got %d",
					tc.cleanupCalls, cleanupCalls,
				)
			}

			if _, statErr := os.Stat(destDir); !os.IsNotExist(statErr) {
				t.Fatalf("expected %q to be removed, got error: %v", destDir, statErr)
			}

			hasFirstErr := strings.Contains(err.Error(), "first error:")
			if hasFirstErr != tc.checkFirstErr {
				t.Errorf("expected first error in message: %t, got %q", tc.checkFirstErr, err)
			}
		})
	}
}

func TestCopyImageWithRetryKeepsForeignDestination(t *testing.T) {
	t.Parallel()

	destDir := t.TempDir()
	foreignFile := filepath.Join(destDir, "important-data")

	if err := os.WriteFile(foreignFile, []byte("keep me"), 0o600); err != nil {
		t.Fatalf("write foreign file: %v", err)
	}

	srcDir := t.TempDir()

	cleanupCalls := 0

	err := copyImageWithRetry(
		context.Background(),
		newTestPolicyContext(t),
		parseDirRef(t, destDir),
		parseDirRef(t, srcDir),
		&imagecopy.Options{},
		3,
		func() error {
			cleanupCalls++

			return cleanupDirDestination("dir:" + destDir)
		},
	)
	if err == nil {
		t.Fatal("expected the copy to fail")
	}

	if !errors.Is(err, directory.ErrNotContainerImageDir) {
		t.Errorf(
			"expected the copy error to wrap %v, got %v",
			directory.ErrNotContainerImageDir,
			err,
		)
	}

	// A destination which is not a containers image directory was never
	// written to by the copy, so it has to be left alone without retrying.
	if cleanupCalls != 0 {
		t.Errorf("expected no cleanup call, got %d", cleanupCalls)
	}

	if _, statErr := os.Stat(foreignFile); statErr != nil {
		t.Errorf("expected %q to be kept: %v", foreignFile, statErr)
	}
}

func TestCopyImageWithRetryStopsWhenCleanupFails(t *testing.T) {
	t.Parallel()

	destDir := t.TempDir()
	srcDir := t.TempDir()

	cleanupFailure := errors.New("cleanup failed")
	cleanupCalls := 0

	err := copyImageWithRetry(
		context.Background(),
		newTestPolicyContext(t),
		parseDirRef(t, destDir),
		parseDirRef(t, srcDir),
		&imagecopy.Options{},
		3,
		func() error {
			cleanupCalls++

			return cleanupFailure
		},
	)
	if err == nil {
		t.Fatal("expected the copy to fail")
	}

	if !errors.Is(err, cleanupFailure) {
		t.Errorf("expected the copy error to wrap %v, got %v", cleanupFailure, err)
	}

	// Running the next attempt against the untouched destination would only
	// report that it is not a containers image directory.
	if cleanupCalls != 1 {
		t.Errorf(
			"expected the retries to stop after the failed cleanup, got %d calls",
			cleanupCalls,
		)
	}
}

func newTestPolicyContext(t *testing.T) *signature.PolicyContext {
	t.Helper()

	policyContext, err := signature.NewPolicyContext(&signature.Policy{
		Default: []signature.PolicyRequirement{signature.NewPRInsecureAcceptAnything()},
	})
	if err != nil {
		t.Fatalf("create policy context: %v", err)
	}

	t.Cleanup(func() {
		if err := policyContext.Destroy(); err != nil {
			t.Errorf("destroy policy context: %v", err)
		}
	})

	return policyContext
}

func parseDirRef(t *testing.T, dir string) types.ImageReference {
	t.Helper()

	ref, err := alltransports.ParseImageName("dir:" + dir)
	if err != nil {
		t.Fatalf("parse dir reference %q: %v", dir, err)
	}

	return ref
}
