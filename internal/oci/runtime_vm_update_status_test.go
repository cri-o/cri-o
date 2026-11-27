package oci_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	task "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/ttrpc"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
)

type fakeTaskService struct {
	task.TaskService

	killErr   error
	killCtx   context.Context
	stateResp *task.StateResponse
	stateErr  error
	stateCtx  context.Context
}

func (f *fakeTaskService) Kill(ctx context.Context, _ *task.KillRequest) (*emptypb.Empty, error) {
	f.killCtx = ctx

	return nil, f.killErr
}

func (f *fakeTaskService) State(
	ctx context.Context, _ *task.StateRequest,
) (*task.StateResponse, error) {
	f.stateCtx = ctx

	if f.stateErr != nil {
		return nil, f.stateErr
	}

	return f.stateResp, nil
}

func newTestContainerWithBundle(bundlePath string) *oci.Container {
	c, err := oci.NewContainer("test-ctr-id", "test-ctr", bundlePath, "",
		map[string]string{}, map[string]string{},
		map[string]string{}, "", nil, nil, "",
		&types.ContainerMetadata{}, "test-sandbox", false,
		false, false, "", "", time.Now(), "")
	Expect(err).ToNot(HaveOccurred())

	return c
}

var _ = t.Describe("RuntimeVM updateContainerStatus", func() {
	It("should mark container stopped when shim address is missing", func() {
		bundleDir := GinkgoT().TempDir()
		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{})
		c.State().Status = oci.ContainerStateCreated

		r := oci.NewRuntimeVM()
		err := r.UpdateContainerStatus(context.Background(), c)
		Expect(err).ToNot(HaveOccurred())
		Expect(string(c.State().Status)).To(Equal(oci.ContainerStateStopped))
		Expect(c.State().Finished.IsZero()).To(BeFalse())
		Expect(c.State().ExitCode).ToNot(BeNil())
		Expect(*c.State().ExitCode).To(Equal(int32(255)))
	})

	It("should keep stopped status when shim address is missing and already stopped", func() {
		bundleDir := GinkgoT().TempDir()
		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{})
		c.State().Status = oci.ContainerStateStopped

		r := oci.NewRuntimeVM()
		err := r.UpdateContainerStatus(context.Background(), c)
		Expect(err).ToNot(HaveOccurred())
		Expect(string(c.State().Status)).To(Equal(oci.ContainerStateStopped))
	})

	It("should mark container stopped when shim connection fails", func() {
		bundleDir := GinkgoT().TempDir()
		addressPath := filepath.Join(bundleDir, "address")
		Expect(os.WriteFile(
			addressPath, []byte("unix:///nonexistent/shim.sock\n"), 0o644,
		)).To(Succeed())

		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{})
		c.State().Status = oci.ContainerStateRunning

		r := oci.NewRuntimeVM()
		err := r.UpdateContainerStatus(context.Background(), c)
		Expect(err).ToNot(HaveOccurred())
		Expect(string(c.State().Status)).To(Equal(oci.ContainerStateStopped))
		Expect(c.State().Finished.IsZero()).To(BeFalse())
		Expect(c.State().ExitCode).ToNot(BeNil())
		Expect(*c.State().ExitCode).To(Equal(int32(255)))
	})

	It("should preserve finished time and exit code when already stopped and shim connection fails",
		func() {
			bundleDir := GinkgoT().TempDir()
			addressPath := filepath.Join(bundleDir, "address")
			Expect(os.WriteFile(
				addressPath, []byte("unix:///nonexistent/shim.sock\n"), 0o644,
			)).To(Succeed())

			originalFinished := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			originalExitCode := int32(137)

			c := newTestContainerWithBundle(bundleDir)
			c.SetState(&oci.ContainerState{})
			c.State().Status = oci.ContainerStateStopped
			c.State().Finished = originalFinished
			c.State().ExitCode = &originalExitCode

			r := oci.NewRuntimeVM()
			err := r.UpdateContainerStatus(context.Background(), c)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(c.State().Status)).To(Equal(oci.ContainerStateStopped))
			Expect(c.State().Finished).To(Equal(originalFinished))
			Expect(c.State().ExitCode).ToNot(BeNil())
			Expect(*c.State().ExitCode).To(Equal(originalExitCode))
		})

	It("should return error when shim address is unreadable", func() {
		bundleDir := GinkgoT().TempDir()
		addressPath := filepath.Join(bundleDir, "address")
		Expect(os.Mkdir(addressPath, 0o755)).To(Succeed())

		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{})
		c.State().Status = oci.ContainerStateRunning

		r := oci.NewRuntimeVM()
		err := r.UpdateContainerStatus(context.Background(), c)
		Expect(err).To(HaveOccurred())
		Expect(string(c.State().Status)).To(Equal(oci.ContainerStateRunning))
	})
})

var _ = t.Describe("RuntimeVM StopContainer after restore", func() {
	It("should short-circuit when container was marked stopped during restore", func() {
		bundleDir := GinkgoT().TempDir()
		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{})
		c.State().Status = oci.ContainerStateRunning

		r := oci.NewRuntimeVM()
		Expect(r.UpdateContainerStatus(context.Background(), c)).To(Succeed())
		Expect(string(c.State().Status)).To(Equal(oci.ContainerStateStopped))
		Expect(r.HasTask()).To(BeFalse())

		Expect(r.StopContainer(context.Background(), c, 30)).To(Succeed())
	})

	It("should not panic when task is nil and container is stopped", func() {
		bundleDir := GinkgoT().TempDir()
		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{})
		c.State().Status = oci.ContainerStateStopped
		c.State().Finished = time.Now()

		r := oci.NewRuntimeVM()
		Expect(r.HasTask()).To(BeFalse())
		Expect(r.StopContainer(context.Background(), c, 30)).To(Succeed())
	})
})

var _ = t.Describe("RuntimeVM DeleteContainer after restore", func() {
	It("should succeed when container is stopped and has no shim info", func() {
		bundleDir := GinkgoT().TempDir()
		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{})
		c.State().Status = oci.ContainerStateRunning

		r := oci.NewRuntimeVM()
		Expect(r.UpdateContainerStatus(context.Background(), c)).To(Succeed())
		Expect(string(c.State().Status)).To(Equal(oci.ContainerStateStopped))

		Expect(r.DeleteContainer(context.Background(), c)).To(Succeed())
	})
})

var _ = t.Describe("RuntimeVM kill", func() {
	It("should return nil when shim has already exited (ttrpc.ErrClosed) for stop signals", func() {
		r := oci.NewRuntimeVMWithTask(&fakeTaskService{killErr: ttrpc.ErrClosed})
		Expect(r.Kill(context.Background(), "ctr-id", "", syscall.SIGTERM)).NotTo(HaveOccurred())
	})

	It("should report not alive instead of panicking when the shim is not connected", func() {
		bundleDir := GinkgoT().TempDir()
		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{})

		r := oci.NewRuntimeVM()
		Expect(r.HasTask()).To(BeFalse())

		alive, err := r.IsContainerAlive(context.Background(), c)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("is not connected"))
		Expect(alive).To(BeFalse())
	})

	It("should propagate errors that are not ttrpc.ErrClosed", func() {
		r := oci.NewRuntimeVMWithTask(&fakeTaskService{killErr: context.DeadlineExceeded})
		Expect(r.Kill(context.Background(), "ctr-id", "", syscall.SIGTERM)).To(HaveOccurred())
	})

	It("should return nil when kill succeeds", func() {
		r := oci.NewRuntimeVMWithTask(&fakeTaskService{killErr: nil})
		Expect(r.Kill(context.Background(), "ctr-id", "", syscall.SIGTERM)).NotTo(HaveOccurred())
	})
})

var _ = t.Describe("RuntimeVM IsContainerAlive", func() {
	It("should report a stopped container as not alive without asking the shim", func() {
		r := oci.NewRuntimeVM()
		c := newTestContainerWithBundle(GinkgoT().TempDir())
		c.SetState(&oci.ContainerState{})
		c.State().Status = oci.ContainerStateStopped

		Expect(r.HasTask()).To(BeFalse())

		alive, err := r.IsContainerAlive(context.Background(), c)
		Expect(err).ToNot(HaveOccurred())
		Expect(alive).To(BeFalse())
	})

	It("should ask the shim for the state with a bounded context", func() {
		taskService := &fakeTaskService{
			stateResp: &task.StateResponse{Status: tasktypes.Status_RUNNING},
		}
		r := oci.NewRuntimeVMWithTask(taskService)
		c := newTestContainerWithBundle(GinkgoT().TempDir())

		_, err := r.IsContainerAlive(context.Background(), c)
		Expect(err).ToNot(HaveOccurred())

		Expect(taskService.stateCtx).ToNot(BeNil())
		_, hasDeadline := taskService.stateCtx.Deadline()
		Expect(hasDeadline).To(BeTrue())

		// A Kill with an empty exec ID makes kata freeze and thaw the whole
		// container, which resumes a paused workload.
		Expect(taskService.killCtx).To(BeNil())
	})

	DescribeTable("should report the state from the shim",
		func(shimState tasktypes.Status, wantAlive bool) {
			r := oci.NewRuntimeVMWithTask(
				&fakeTaskService{stateResp: &task.StateResponse{Status: shimState}},
			)
			c := newTestContainerWithBundle(GinkgoT().TempDir())

			alive, err := r.IsContainerAlive(context.Background(), c)
			Expect(err).ToNot(HaveOccurred())
			Expect(alive).To(Equal(wantAlive))
		},
		Entry("running", tasktypes.Status_RUNNING, true),
		Entry("paused", tasktypes.Status_PAUSED, true),
		Entry("created", tasktypes.Status_CREATED, true),
		Entry("stopped", tasktypes.Status_STOPPED, false),
		Entry("unknown", tasktypes.Status_UNKNOWN, false),
	)

	DescribeTable("should report not alive when the shim no longer has the container",
		func(shimErr error) {
			r := oci.NewRuntimeVMWithTask(&fakeTaskService{stateErr: shimErr})
			c := newTestContainerWithBundle(GinkgoT().TempDir())

			alive, err := r.IsContainerAlive(context.Background(), c)
			Expect(err).ToNot(HaveOccurred())
			Expect(alive).To(BeFalse())
		},
		Entry("shim closed", ttrpc.ErrClosed),
		Entry("task not found", status.Error(codes.NotFound, "task not found")),
	)

	DescribeTable("should report an unknown state when the shim does not answer",
		func(shimErr error) {
			r := oci.NewRuntimeVMWithTask(&fakeTaskService{stateErr: shimErr})
			c := newTestContainerWithBundle(GinkgoT().TempDir())

			alive, err := r.IsContainerAlive(context.Background(), c)
			Expect(alive).To(BeFalse())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("check whether container"))
		},
		Entry("deadline", context.DeadlineExceeded),
		Entry("cancellation", context.Canceled),
		Entry("shim error", status.Error(codes.Unavailable, "shim busy")),
	)
})

var _ = t.Describe("RuntimeVM exec eligibility", func() {
	execContainer := func(r oci.RuntimeVM, c *oci.Container) error {
		return r.ExecContainer(
			context.Background(), c, []string{"true"}, nil, nil, nil, false, nil,
		)
	}

	It("should reject a container the shim reports as not running", func() {
		taskService := &fakeTaskService{
			stateResp: &task.StateResponse{Status: tasktypes.Status_STOPPED},
		}
		r := oci.NewRuntimeVMWithTask(taskService)
		c := newTestContainerWithBundle(GinkgoT().TempDir())

		err := execContainer(r, c)
		Expect(errors.Is(err, oci.ErrContainerNotRunning)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring("container is not running"))

		// The exec asked the shim, so the deadline comes from the VM runtime.
		Expect(taskService.stateCtx).ToNot(BeNil())
	})

	It("should reject a container the shim no longer has on exec sync", func() {
		r := oci.NewRuntimeVMWithTask(&fakeTaskService{stateErr: ttrpc.ErrClosed})
		c := newTestContainerWithBundle(GinkgoT().TempDir())

		_, err := r.ExecSyncContainer(context.Background(), c, []string{"true"}, 0)
		Expect(errors.Is(err, oci.ErrContainerNotRunning)).To(BeTrue())
	})

	It("should reject a stopped container without a shim as not running", func() {
		r := oci.NewRuntimeVM()
		c := newTestContainerWithBundle(GinkgoT().TempDir())
		c.SetState(&oci.ContainerState{})
		c.State().Status = oci.ContainerStateStopped

		Expect(r.HasTask()).To(BeFalse())

		_, err := r.ExecSyncContainer(context.Background(), c, []string{"true"}, 0)
		Expect(errors.Is(err, oci.ErrContainerNotRunning)).To(BeTrue())
	})

	It("should report an unknown state when the shim does not answer", func() {
		r := oci.NewRuntimeVMWithTask(&fakeTaskService{stateErr: context.DeadlineExceeded})
		c := newTestContainerWithBundle(GinkgoT().TempDir())

		err := execContainer(r, c)

		var unknown *oci.ErrContainerStateUnknown
		Expect(errors.As(err, &unknown)).To(BeTrue())
		Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue())
	})

	It("should report an unknown state when the shim is not connected", func() {
		r := oci.NewRuntimeVM()
		c := newTestContainerWithBundle(GinkgoT().TempDir())

		err := execContainer(r, c)

		var unknown *oci.ErrContainerStateUnknown
		Expect(errors.As(err, &unknown)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring("is not connected"))
	})
})
