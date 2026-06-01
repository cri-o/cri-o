package oci_test

import (
	"context"
	"syscall"

	task "github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/containerd/ttrpc"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/cri-o/cri-o/internal/oci"
)

// fakeTaskService is a minimal task.TaskService stub for testing kill() behavior.
// Only Kill() is implemented; all other methods panic if called.
type fakeTaskService struct {
	task.TaskService

	killErr error
}

func (f *fakeTaskService) Kill(_ context.Context, _ *task.KillRequest) (*emptypb.Empty, error) {
	return nil, f.killErr
}

var _ = t.Describe("RuntimeVM", func() {
	Describe("kill", func() {
		It("should return nil when shim has already exited (ttrpc.ErrClosed)", func() {
			// Given — shim ttrpc connection is closed (shim exited before kill was called)
			r := oci.NewRuntimeVMWithTask(&fakeTaskService{killErr: ttrpc.ErrClosed})

			// When
			err := r.Kill("ctr-id", "", syscall.SIGTERM)

			// Then — a kill on a dead shim means the container is already stopped
			Expect(err).NotTo(HaveOccurred())
		})

		It("should propagate errors that are not ttrpc.ErrClosed", func() {
			// Given — shim returns a real error (e.g. container not found)
			r := oci.NewRuntimeVMWithTask(&fakeTaskService{killErr: ttrpc.ErrClosed})
			rErr := oci.NewRuntimeVMWithTask(&fakeTaskService{killErr: context.DeadlineExceeded})

			// Sanity: ErrClosed is swallowed
			Expect(r.Kill("ctr-id", "", syscall.SIGTERM)).NotTo(HaveOccurred())

			// Other errors are propagated
			Expect(rErr.Kill("ctr-id", "", syscall.SIGTERM)).To(HaveOccurred())
		})

		It("should return nil when kill succeeds", func() {
			// Given — shim is alive and kill succeeds
			r := oci.NewRuntimeVMWithTask(&fakeTaskService{killErr: nil})

			// When
			err := r.Kill("ctr-id", "", syscall.SIGTERM)

			// Then
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
