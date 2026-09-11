// Command synctrace is a minimal, single-threaded reproducer of the exact
// two-statement sequence that drives the fsync-before-rename durability fix
// in the overlay storage driver's ApplyDiffFromStagingDirectory:
//
//	if err := ioutils.SyncDirectoryContents(stagingDirectory); err != nil {
//		return err
//	}
//	return os.Rename(stagingDirectory, diffPath)
//
// It exists because tracing that same sequence inside the real crio
// server, under strace -f during a live network pull, is unreliable: crio
// is a heavily multi-threaded Go binary, and ptrace has a well-known
// attach race on newly created threads where a thread's first few
// syscalls can execute before the tracer finishes attaching, silently
// dropping them from the trace. This program calls the exact same
// production function (not a reimplementation) from a single goroutine
// with no concurrent I/O, so there is no new-thread race for strace to
// lose syscalls to, and the fsync-before-rename ordering can be verified
// with full confidence.
//
// runtime.LockOSThread is required for that guarantee to hold under real
// system load, not just in the common case: the Go scheduler can migrate
// a goroutine to a different OS thread after it returns from a long
// blocking syscall (fsync contending with other disk I/O on a busy
// machine is exactly such a syscall) if its original thread's P was
// reassigned while it was blocked. Without pinning, the fsync calls and
// the following os.Rename could end up observed on two different OS
// threads, and a non-follow-fork strace run (deliberately not using -f
// here, for the same reason as above) would only be attached to the
// first one, missing the rename.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"go.podman.io/storage/pkg/ioutils"
)

func main() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <staging-directory>\n", os.Args[0])
		os.Exit(2)
	}

	stagingDir := os.Args[1]
	finalDir := stagingDir + "-final"

	if err := populate(stagingDir); err != nil {
		fmt.Fprintf(os.Stderr, "populate staging directory: %v\n", err)
		os.Exit(1)
	}

	// This mirrors, statement for statement, what
	// overlay.Driver.ApplyDiffFromStagingDirectory does before publishing
	// a newly applied layer.
	if err := ioutils.SyncDirectoryContents(stagingDir); err != nil {
		fmt.Fprintf(os.Stderr, "sync staging directory before rename: %v\n", err)
		os.Exit(1)
	}

	if err := os.Rename(stagingDir, finalDir); err != nil {
		fmt.Fprintf(os.Stderr, "rename staging directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("OK")
}

// populate creates a staging directory shaped like a real layer diff:
// nested directories, a regular file with real content to fsync, and a
// dangling symlink (like Debian's /etc/alternatives/*) to also exercise
// the symlink fix in SyncDirectoryContents.
func populate(dir string) error {
	nested := filepath.Join(dir, "etc", "nginx", "conf.d")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		return err
	}

	content := make([]byte, 4<<20) // 4MiB: large enough that fsync has real work to do.
	for i := range content {
		content[i] = byte(i)
	}

	if err := os.WriteFile(filepath.Join(nested, "default.conf"), content, 0o644); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(dir, "run-nginx.pid"), []byte("1\n"), 0o644); err != nil {
		return err
	}

	return os.Symlink("/nonexistent/target", filepath.Join(dir, "dangling-link"))
}
