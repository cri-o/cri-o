package oci_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/containerd/ttrpc"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/internal/oci"
	crioconfig "github.com/cri-o/cri-o/pkg/config"
)

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

// writeFakeShim writes a runtime binary which stores its arguments in argsFile.
func writeFakeShim(dir, argsFile string) string {
	script := "#!/bin/sh\necho \"$@\" > " + argsFile + "\n"
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

	It("should clean up when the shim is gone during shutdown", func() {
		bundleDir := GinkgoT().TempDir()
		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{Status: oci.ContainerStateRunning})

		dir := GinkgoT().TempDir()
		argsFile := filepath.Join(dir, "args")
		r := oci.NewRuntimeVMWithHandler(&crioconfig.RuntimeHandler{
			RuntimePath: writeFakeShim(dir, argsFile),
		})

		server, client := net.Pipe()

		DeferCleanup(func() {
			_ = server.Close()
			_ = client.Close()
		})

		r.AddContainer(c.ID())
		r.SetTask(&fakeTaskService{shutdownErr: ttrpc.ErrClosed})
		r.SetClient(ttrpc.NewClient(client))

		Expect(r.DeleteContainer(context.Background(), c)).To(Succeed())
		Expect(waitForShimArgs(argsFile)).To(ContainSubstring("-id " + c.ID()))
	})

	It("should return the shutdown error and skip cleanup for other errors", func() {
		bundleDir := GinkgoT().TempDir()
		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{Status: oci.ContainerStateRunning})

		dir := GinkgoT().TempDir()
		argsFile := filepath.Join(dir, "args")
		r := oci.NewRuntimeVMWithHandler(&crioconfig.RuntimeHandler{
			RuntimePath: writeFakeShim(dir, argsFile),
		})

		server, client := net.Pipe()

		DeferCleanup(func() {
			_ = server.Close()
			_ = client.Close()
		})

		r.AddContainer(c.ID())
		r.SetTask(&fakeTaskService{shutdownErr: context.DeadlineExceeded})
		r.SetClient(ttrpc.NewClient(client))

		Expect(r.DeleteContainer(context.Background(), c)).To(HaveOccurred())
		Consistently(argsFile).WithTimeout(100 * time.Millisecond).ShouldNot(BeAnExistingFile())
	})

	It("should not mark the container as deleting when the deletion fails", func() {
		bundleDir := GinkgoT().TempDir()
		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{Status: oci.ContainerStateRunning})

		dir := GinkgoT().TempDir()
		argsFile := filepath.Join(dir, "args")
		r := oci.NewRuntimeVMWithHandler(&crioconfig.RuntimeHandler{
			RuntimePath: writeFakeShim(dir, argsFile),
		})

		server, client := net.Pipe()

		DeferCleanup(func() {
			_ = server.Close()
			_ = client.Close()
		})

		r.AddContainer(c.ID())
		r.SetTask(&fakeTaskService{deleteErr: context.DeadlineExceeded})
		r.SetClient(ttrpc.NewClient(client))

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
		runtimePath := filepath.Join(dir, "shim")

		script := "#!/bin/sh\n" +
			"if [ ! -f " + marker + " ]; then\n" +
			"\ttouch " + marker + "\n" +
			"\texit 1\n" +
			"fi\n" +
			"echo \"$@\" > " + argsFile + "\n"
		Expect(os.WriteFile(runtimePath, []byte(script), 0o755)).To(Succeed())

		r := oci.NewRuntimeVMWithHandler(&crioconfig.RuntimeHandler{RuntimePath: runtimePath})

		Expect(r.CleanupAfterDeadShim(context.Background(), c)).To(HaveOccurred())
		Expect(marker).To(BeAnExistingFile())

		Expect(r.CleanupAfterDeadShim(context.Background(), c)).To(Succeed())
		Expect(argsFile).To(BeAnExistingFile())
	})
})
