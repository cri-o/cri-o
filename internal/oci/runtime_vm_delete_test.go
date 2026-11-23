package oci_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/ttrpc"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/internal/oci"
	crioconfig "github.com/cri-o/cri-o/pkg/config"
)

// shQuote quotes a path for use inside a POSIX shell script, so that paths
// containing spaces or quotes are kept as one word.
func shQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

var _ = t.Describe("RuntimeVM shim delete args", func() {
	It("should return the delete arguments", func() {
		Expect(oci.ShimDeleteArgs("abc123", "/bundle", false)).To(Equal(
			[]string{"-id", "abc123", "-bundle", "/bundle", "delete"},
		))
	})

	It("should append debug when enabled", func() {
		Expect(oci.ShimDeleteArgs("abc123", "/bundle", true)).To(Equal(
			[]string{"-id", "abc123", "-bundle", "/bundle", "-debug", "delete"},
		))
	})
})

// writeFakeShim writes a runtime binary which runs the preamble and then stores
// its arguments in argsFile.
func writeFakeShim(dir, argsFile string, preamble ...string) string {
	script := "#!/bin/sh\n"
	if len(preamble) > 0 {
		script += strings.Join(preamble, "\n") + "\n"
	}

	script += "echo \"$@\" > " + shQuote(argsFile) + "\n"

	runtimePath := filepath.Join(dir, "shim")
	Expect(os.WriteFile(runtimePath, []byte(script), 0o755)).To(Succeed())

	return runtimePath
}

// waitForShimArgs waits until the shim's background delete command stored its
// arguments and returns them.
func waitForShimArgs(argsFile string) string {
	var args string

	Eventually(func() string {
		data, err := os.ReadFile(argsFile)
		if err != nil {
			return ""
		}

		args = string(data)

		return args
	}).WithTimeout(5 * time.Second).ShouldNot(BeEmpty())

	return args
}

var _ = t.Describe("RuntimeVM DeleteContainer shim cleanup", func() {
	It("should run the shim delete for a stopped container without runtime state", func() {
		bundleDir := GinkgoT().TempDir()
		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{
			Status:   oci.ContainerStateStopped,
			Finished: time.Now(),
		})

		dir := GinkgoT().TempDir()
		argsFile := filepath.Join(dir, "args")
		r := oci.NewRuntimeVMWithHandler(&crioconfig.RuntimeHandler{
			RuntimePath: writeFakeShim(dir, argsFile),
		})

		Expect(r.DeleteContainer(context.Background(), c)).To(Succeed())

		args := waitForShimArgs(argsFile)
		Expect(args).To(ContainSubstring("-namespace default"))
		Expect(args).To(ContainSubstring("-id " + c.ID()))
		Expect(args).To(ContainSubstring("-bundle " + bundleDir))
		Expect(args).To(ContainSubstring("delete"))
	})

	It("should finish the shim delete before returning", func() {
		bundleDir := GinkgoT().TempDir()
		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{
			Status:   oci.ContainerStateStopped,
			Finished: time.Now(),
		})

		dir := GinkgoT().TempDir()
		argsFile := filepath.Join(dir, "args")
		r := oci.NewRuntimeVMWithHandler(&crioconfig.RuntimeHandler{
			RuntimePath: writeFakeShim(dir, argsFile, "sleep 0.2"),
		})

		// The caller deletes the container storage, and with it the bundle the
		// shim delete command runs from, as soon as this returns.
		Expect(r.DeleteContainer(context.Background(), c)).To(Succeed())

		args, err := os.ReadFile(argsFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(args)).To(ContainSubstring("-bundle " + bundleDir))
	})

	It("should clean up when the shim is gone during shutdown", func() {
		bundleDir := GinkgoT().TempDir()
		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{Status: oci.ContainerStateRunning})

		dir := GinkgoT().TempDir()
		argsFile := filepath.Join(dir, "args")

		r := runtimeWithTaskService(
			&crioconfig.RuntimeHandler{RuntimePath: writeFakeShim(dir, argsFile)},
			c.ID(), &fakeTaskService{shutdownErr: ttrpc.ErrClosed},
		)

		Expect(r.DeleteContainer(context.Background(), c)).To(Succeed())
		Expect(waitForShimArgs(argsFile)).To(ContainSubstring("-id " + c.ID()))
	})

	It("should return the shutdown error and skip cleanup for other errors", func() {
		bundleDir := GinkgoT().TempDir()
		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{Status: oci.ContainerStateRunning})

		dir := GinkgoT().TempDir()
		argsFile := filepath.Join(dir, "args")

		r := runtimeWithTaskService(
			&crioconfig.RuntimeHandler{RuntimePath: writeFakeShim(dir, argsFile)},
			c.ID(), &fakeTaskService{shutdownErr: context.DeadlineExceeded},
		)

		Expect(r.DeleteContainer(context.Background(), c)).To(HaveOccurred())
		Expect(argsFile).NotTo(BeAnExistingFile())
	})

	It("should not mark the container as deleting when the deletion fails", func() {
		bundleDir := GinkgoT().TempDir()
		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{Status: oci.ContainerStateRunning})

		dir := GinkgoT().TempDir()
		argsFile := filepath.Join(dir, "args")

		r := runtimeWithTaskService(
			&crioconfig.RuntimeHandler{RuntimePath: writeFakeShim(dir, argsFile)},
			c.ID(), &fakeTaskService{deleteErr: context.DeadlineExceeded},
		)

		Expect(r.DeleteContainer(context.Background(), c)).To(HaveOccurred())
		// Otherwise a shim dying later is mistaken for the deletion itself and
		// its cleanup is skipped.
		Expect(r.IsDeleting(c.ID())).To(BeFalse())
	})

	It("should retry a failed shim delete on the next attempt", func() {
		c := newTestContainerWithBundle(GinkgoT().TempDir())
		c.SetState(&oci.ContainerState{
			Status:   oci.ContainerStateStopped,
			Finished: time.Now(),
		})

		dir := GinkgoT().TempDir()
		argsFile := filepath.Join(dir, "args")
		marker := filepath.Join(dir, "attempted")

		runtimePath := writeFakeShim(dir, argsFile,
			"if [ ! -f "+shQuote(marker)+" ]; then",
			"\ttouch "+shQuote(marker),
			"\texit 1",
			"fi",
		)

		r := oci.NewRuntimeVMWithHandler(&crioconfig.RuntimeHandler{RuntimePath: runtimePath})

		Expect(r.CleanupAfterDeadShim(context.Background(), c)).To(HaveOccurred())
		Expect(marker).To(BeAnExistingFile())

		Expect(r.CleanupAfterDeadShim(context.Background(), c)).To(Succeed())
		Expect(argsFile).To(BeAnExistingFile())
	})

	It("should keep the container for a retry when the shim delete fails", func() {
		bundleDir := GinkgoT().TempDir()
		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{Status: oci.ContainerStateRunning})

		dir := GinkgoT().TempDir()
		argsFile := filepath.Join(dir, "args")
		countFile := filepath.Join(dir, "count")

		// The shim delete fails twice, then succeeds.
		runtimePath := writeFakeShim(dir, argsFile,
			"count="+shQuote(countFile),
			"n=0",
			"if [ -f \"$count\" ]; then n=$(cat \"$count\"); fi",
			"n=$((n + 1))",
			"echo \"$n\" > \"$count\"",
			"if [ \"$n\" -le 2 ]; then",
			"\texit 1",
			"fi",
		)

		r := runtimeWithTaskService(
			&crioconfig.RuntimeHandler{RuntimePath: runtimePath},
			c.ID(), &fakeTaskService{shutdownErr: ttrpc.ErrClosed},
		)

		// The removal fails, so the bundle the shim delete command runs from is
		// kept and the caller can retry.
		Expect(r.DeleteContainer(context.Background(), c)).To(HaveOccurred())
		Expect(countFile).To(BeAnExistingFile())

		// The retry finds a stopped container without runtime state, and fails
		// again, so the container is kept for another try.
		c.State().Status = oci.ContainerStateStopped
		Expect(r.DeleteContainer(context.Background(), c)).To(HaveOccurred())

		Expect(r.DeleteContainer(context.Background(), c)).To(Succeed())
		Expect(argsFile).To(BeAnExistingFile())
	})

	It("should not run the shim delete when the bundle is gone", func() {
		bundleDir := GinkgoT().TempDir()
		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{
			Status:   oci.ContainerStateStopped,
			Finished: time.Now(),
		})
		Expect(os.RemoveAll(bundleDir)).To(Succeed())

		dir := GinkgoT().TempDir()
		argsFile := filepath.Join(dir, "args")
		r := oci.NewRuntimeVMWithHandler(&crioconfig.RuntimeHandler{
			RuntimePath: writeFakeShim(dir, argsFile),
		})

		// The command runs from the bundle, so retrying cannot clean up either.
		Expect(r.DeleteContainer(context.Background(), c)).To(Succeed())
		Expect(argsFile).NotTo(BeAnExistingFile())
	})

	It("should not run the shim delete when the runtime state is gone", func() {
		bundleDir := GinkgoT().TempDir()
		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{
			Status:   oci.ContainerStateStopped,
			Finished: time.Now(),
		})

		dir := GinkgoT().TempDir()
		argsFile := filepath.Join(dir, "args")
		r := oci.NewRuntimeVMWithHandler(&crioconfig.RuntimeHandler{
			RuntimePath: writeFakeShim(dir, argsFile),
			RuntimeRoot: filepath.Join(dir, "gone"),
		})

		Expect(r.DeleteContainer(context.Background(), c)).To(Succeed())
		Expect(argsFile).NotTo(BeAnExistingFile())
	})
})
