package oci_test

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"time"

	task "github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/containerd/ttrpc"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/emptypb"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
)

type fakeTaskService struct {
	task.TaskService

	killErr error
}

func (f *fakeTaskService) Kill(_ context.Context, _ *task.KillRequest) (*emptypb.Empty, error) {
	return nil, f.killErr
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
