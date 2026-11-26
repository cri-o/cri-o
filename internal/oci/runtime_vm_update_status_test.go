package oci_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"

	task "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/ttrpc"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/emptypb"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
	crioconfig "github.com/cri-o/cri-o/pkg/config"
)

type fakeTaskService struct {
	task.TaskService

	killErr     error
	deleteErr   error
	shutdownErr error
	stateResp   *task.StateResponse
	stateErr    error
}

func (f *fakeTaskService) State(
	_ context.Context,
	_ *task.StateRequest,
) (*task.StateResponse, error) {
	if f.stateErr != nil {
		return nil, f.stateErr
	}

	return f.stateResp, nil
}

func (f *fakeTaskService) Kill(_ context.Context, _ *task.KillRequest) (*emptypb.Empty, error) {
	return nil, f.killErr
}

func (f *fakeTaskService) Delete(
	_ context.Context,
	_ *task.DeleteRequest,
) (*task.DeleteResponse, error) {
	return nil, f.deleteErr
}

func (f *fakeTaskService) Shutdown(
	_ context.Context,
	_ *task.ShutdownRequest,
) (*emptypb.Empty, error) {
	return nil, f.shutdownErr
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

// runtimeWithTaskService returns a runtime backed by the provided handler and
// task service.
func runtimeWithTaskService(
	handler *crioconfig.RuntimeHandler,
	ctrID string,
	svc *fakeTaskService,
) oci.RuntimeVM {
	server, conn := net.Pipe()

	DeferCleanup(func() {
		_ = server.Close()
		_ = conn.Close()
	})

	r := oci.NewRuntimeVMWithHandler(handler)
	r.AddContainer(ctrID)
	r.SetTask(svc)
	r.SetClient(ttrpc.NewClient(conn))

	return r
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

	It("should record 255 when the container was already stopped without an exit code", func() {
		bundleDir := GinkgoT().TempDir()
		addressPath := filepath.Join(bundleDir, "address")
		Expect(os.WriteFile(
			addressPath, []byte("unix:///nonexistent/shim.sock\n"), 0o644,
		)).To(Succeed())

		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{})
		c.State().Status = oci.ContainerStateStopped
		c.State().Finished = time.Now()

		r := oci.NewRuntimeVM()
		Expect(r.UpdateContainerStatus(context.Background(), c)).To(Succeed())
		Expect(c.State().ExitCode).ToNot(BeNil())
		Expect(*c.State().ExitCode).To(Equal(int32(255)))
	})

	It("should not cache the exit state of a running task", func() {
		bundleDir := GinkgoT().TempDir()
		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{})
		c.State().Status = oci.ContainerStateCreated

		svc := &fakeTaskService{stateResp: &task.StateResponse{
			Status:     tasktypes.Status_RUNNING,
			ExitStatus: 0,
		}}
		r := runtimeWithTaskService(&crioconfig.RuntimeHandler{}, c.ID(), svc)

		Expect(r.UpdateContainerStatus(context.Background(), c)).To(Succeed())
		Expect(string(c.State().Status)).To(Equal(oci.ContainerStateRunning))
		Expect(c.State().ExitCode).To(BeNil())
		Expect(c.State().Finished.IsZero()).To(BeTrue())

		// The shim disconnects: the container stops at the time of detection.
		svc.stateResp = nil
		svc.stateErr = ttrpc.ErrClosed

		Expect(r.UpdateContainerStatus(context.Background(), c)).To(Succeed())
		Expect(string(c.State().Status)).To(Equal(oci.ContainerStateStopped))
		Expect(c.State().ExitCode).ToNot(BeNil())
		Expect(*c.State().ExitCode).To(Equal(int32(255)))
		Expect(c.State().Finished).To(BeTemporally("~", time.Now(), time.Minute))
	})

	It("should replace a stale finish time when the shim dies", func() {
		// An unset exit time reported by a running task is cached as the Unix
		// epoch by older versions, which IsZero does not recognize. A container
		// dying with that timestamp must not report finished_at = 0.
		bundleDir := GinkgoT().TempDir()
		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{})
		c.State().Status = oci.ContainerStateRunning
		c.State().Finished = time.Unix(0, 0)

		r := runtimeWithTaskService(
			&crioconfig.RuntimeHandler{}, c.ID(),
			&fakeTaskService{stateErr: ttrpc.ErrClosed},
		)

		Expect(r.UpdateContainerStatus(context.Background(), c)).To(Succeed())
		Expect(string(c.State().Status)).To(Equal(oci.ContainerStateStopped))
		Expect(c.State().ExitCode).ToNot(BeNil())
		Expect(*c.State().ExitCode).To(Equal(int32(255)))
		Expect(c.State().Finished).To(BeTemporally("~", time.Now(), time.Minute))
	})

	It("should keep the exit code reported by the wait when the shim is unreachable", func() {
		bundleDir := GinkgoT().TempDir()
		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{})
		c.State().Status = oci.ContainerStateRunning

		waitExitCode := int32(42)
		c.State().ExitCode = &waitExitCode

		r := runtimeWithTaskService(
			&crioconfig.RuntimeHandler{}, c.ID(),
			&fakeTaskService{stateErr: ttrpc.ErrClosed},
		)

		Expect(r.UpdateContainerStatus(context.Background(), c)).To(Succeed())
		Expect(string(c.State().Status)).To(Equal(oci.ContainerStateStopped))
		Expect(c.State().ExitCode).ToNot(BeNil())
		Expect(*c.State().ExitCode).To(Equal(waitExitCode))
	})

	It("should keep the exit code of a stopped task after the shim disconnects", func() {
		bundleDir := GinkgoT().TempDir()
		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{})
		c.State().Status = oci.ContainerStateRunning

		svc := &fakeTaskService{stateResp: &task.StateResponse{
			Status:     tasktypes.Status_STOPPED,
			ExitStatus: 0,
		}}
		r := runtimeWithTaskService(&crioconfig.RuntimeHandler{}, c.ID(), svc)

		Expect(r.UpdateContainerStatus(context.Background(), c)).To(Succeed())
		Expect(string(c.State().Status)).To(Equal(oci.ContainerStateStopped))
		Expect(c.State().ExitCode).ToNot(BeNil())
		Expect(*c.State().ExitCode).To(Equal(int32(0)))

		// The shim disconnects after it reported the exit.
		svc.stateResp = nil
		svc.stateErr = ttrpc.ErrClosed

		Expect(r.UpdateContainerStatus(context.Background(), c)).To(Succeed())
		Expect(c.State().ExitCode).ToNot(BeNil())
		Expect(*c.State().ExitCode).To(Equal(int32(0)))
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
		Expect(r.Kill("ctr-id", "", syscall.SIGTERM)).NotTo(HaveOccurred())
	})

	It("should propagate ttrpc.ErrClosed for signal 0 so IsContainerAlive returns false", func() {
		bundleDir := GinkgoT().TempDir()
		c := newTestContainerWithBundle(bundleDir)
		c.SetState(&oci.ContainerState{})

		r := oci.NewRuntimeVMWithTask(&fakeTaskService{killErr: ttrpc.ErrClosed})
		Expect(r.IsContainerAlive(c)).To(BeFalse())
	})

	It("should propagate errors that are not ttrpc.ErrClosed", func() {
		r := oci.NewRuntimeVMWithTask(&fakeTaskService{killErr: ttrpc.ErrClosed})
		rErr := oci.NewRuntimeVMWithTask(&fakeTaskService{killErr: context.DeadlineExceeded})

		Expect(r.Kill("ctr-id", "", syscall.SIGTERM)).NotTo(HaveOccurred())
		Expect(rErr.Kill("ctr-id", "", syscall.SIGTERM)).To(HaveOccurred())
	})

	It("should return nil when kill succeeds", func() {
		r := oci.NewRuntimeVMWithTask(&fakeTaskService{killErr: nil})
		Expect(r.Kill("ctr-id", "", syscall.SIGTERM)).NotTo(HaveOccurred())
	})
})
