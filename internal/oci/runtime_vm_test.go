package oci_test

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
	libconfig "github.com/cri-o/cri-o/pkg/config"
	"github.com/cri-o/cri-o/utils/errdefs"
)

func newTestRuntimeVM() oci.RuntimeVM {
	return oci.NewRuntimeVM(&libconfig.RuntimeHandler{
		RuntimeRoot: t.MustTempDir("runtime-vm-root"),
	})
}

func newContainerWithBundle(bundleDir string) *oci.Container {
	ctr, err := oci.NewContainer(
		"vm-ctr-id", "vm-ctr-name", bundleDir, "",
		map[string]string{}, map[string]string{}, map[string]string{},
		"", nil, nil, "", &types.ContainerMetadata{}, "sandbox-id",
		false, false, false, "", "", time.Now(), "",
	)
	Expect(err).ToNot(HaveOccurred())

	return ctr
}

func setContainerStatus(ctr *oci.Container, status rspec.ContainerState) {
	state := &oci.ContainerState{}
	state.Status = status
	ctr.SetState(state)
}

var _ = t.Describe("RuntimeVM", func() {
	Context("kill() nil-task guard", func() {
		It("returns ErrNotFound instead of panicking when task is nil", func() {
			// Given: runtimeVM reconstructed after restart — task connection not yet established
			sut := newTestRuntimeVM()
			Expect(sut.HasTask()).To(BeFalse())

			// When: kill is called with signal 0 (liveness probe)
			err := sut.Kill("ctr-id", "", syscall.Signal(0))

			// Then: must not panic and must report ErrNotFound
			Expect(err).To(MatchError(errdefs.ErrNotFound))
		})

		It("returns ErrNotFound for SIGTERM when task is nil", func() {
			sut := newTestRuntimeVM()

			err := sut.Kill("ctr-id", "", syscall.SIGTERM)

			Expect(err).To(MatchError(errdefs.ErrNotFound))
		})

		It("returns ErrNotFound for exec-scoped SIGKILL when task is nil", func() {
			sut := newTestRuntimeVM()

			err := sut.Kill("ctr-id", "exec-id", syscall.SIGKILL)

			Expect(err).To(MatchError(errdefs.ErrNotFound))
		})
	})

	Context("connectTask()", func() {
		var ctx = context.Background()

		It("returns nil without connecting for a stopped container with no address file", func() {
			// Given: task is nil, container is Stopped, and the shim address file is absent
			// This is the expected state when CRI-O restarts and the container was already done.
			sut := newTestRuntimeVM()
			bundleDir := t.MustTempDir("bundle-stopped")
			ctr := newContainerWithBundle(bundleDir)
			setContainerStatus(ctr, rspec.ContainerState(oci.ContainerStateStopped))

			// When
			err := sut.ConnectTask(ctx, ctr)

			// Then: graceful no-op — a stopped container needs no shim connection
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.HasTask()).To(BeFalse())
		})

		It("returns an error for a running container when the address file is missing", func() {
			// Given: task is nil, container is Running, shim address file absent
			sut := newTestRuntimeVM()
			bundleDir := t.MustTempDir("bundle-running-no-addr")
			ctr := newContainerWithBundle(bundleDir)
			setContainerStatus(ctr, rspec.ContainerState(oci.ContainerStateRunning))

			// When
			err := sut.ConnectTask(ctx, ctr)

			// Then: error surfaced — the address file must exist for a running container
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("read shim address"))
		})

		It("returns an error when the address file points to a non-existent socket", func() {
			// Given: task is nil, container is Running, address file present but
			// references a socket path with no listening shim
			sut := newTestRuntimeVM()
			bundleDir := t.MustTempDir("bundle-bad-addr")
			ctr := newContainerWithBundle(bundleDir)
			setContainerStatus(ctr, rspec.ContainerState(oci.ContainerStateRunning))

			bogusAddr := filepath.Join(bundleDir, "nonexistent.sock")
			Expect(os.WriteFile(
				filepath.Join(bundleDir, "address"),
				[]byte(bogusAddr+"\n"),
				0o644,
			)).To(Succeed())

			// When
			err := sut.ConnectTask(ctx, ctr)

			// Then: connection must fail — no shim is listening at that address
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("connect to shim"))
		})
	})
})
