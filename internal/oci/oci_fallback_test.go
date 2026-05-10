package oci_test

// Tests for the runtime handler config fallback introduced to fix issue #9521:
// pods must not be stuck in Terminating when their runtime class is removed
// from the live config (e.g. nvidia-container-toolkit teardown race).

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
	libconfig "github.com/cri-o/cri-o/pkg/config"
)

var _ = t.Describe("RuntimeHandlerFallback", func() {
	var sut *oci.Runtime

	const removedRuntime = "removed-runtime"

	newCtr := func(id, handler string) *oci.Container {
		ctr, err := oci.NewContainer(
			id, "", "", "",
			map[string]string{}, map[string]string{}, map[string]string{},
			"", nil, nil, "", &types.ContainerMetadata{}, sandboxID,
			false, false, false, handler, "", time.Now(), "")
		Expect(err).ToNot(HaveOccurred())

		return ctr
	}

	var removedHandler *libconfig.RuntimeHandler

	BeforeEach(func() {
		var err error

		// Fresh config for each test.
		config, err = libconfig.DefaultConfig()
		Expect(err).ToNot(HaveOccurred())

		config.ContainerAttachSocketDir = t.MustTempDir("crio")

		removedHandler = &libconfig.RuntimeHandler{
			RuntimePath: "/bin/sh",
			RuntimeRoot: "/run/crun",
		}

		// Start with both the default and the to-be-removed runtime present.
		config.DefaultRuntime = "crun"
		config.Runtimes = libconfig.Runtimes{
			"crun": &libconfig.RuntimeHandler{
				RuntimePath: "/bin/sh",
				RuntimeRoot: "/run/crun",
			},
			removedRuntime: removedHandler,
		}

		sut, err = oci.New(config)
		Expect(err).ToNot(HaveOccurred())
	})

	simulateConfigReload := func() {
		config.Runtimes = libconfig.Runtimes{
			"crun": config.Runtimes["crun"],
		}
	}

	// Main fix: EnsureRuntimeHandlerConfig + RuntimeImpl fallback
	It("RuntimeImpl fails when handler removed before any caching", func() {
		ctr := newCtr("ctr-no-cache", removedRuntime)

		simulateConfigReload()

		_, err := sut.RuntimeImpl(ctr)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(removedRuntime))
	})

	It("RuntimeImpl succeeds when handler removed after EnsureRuntimeHandlerConfig", func() {
		ctr := newCtr("ctr-with-cache", removedRuntime)

		// Pre-populate while the handler is present — this is what
		// LoadSandbox / LoadContainer call on restore.
		sut.EnsureRuntimeHandlerConfig(ctr)

		// Simulate the race: nvidia-container-toolkit removes the runtime.
		simulateConfigReload()

		// The container must still be stoppable via the fallback config.
		impl, err := sut.RuntimeImpl(ctr)
		Expect(err).ToNot(HaveOccurred())
		Expect(impl).NotTo(BeNil())
	})

	It("RuntimeImpl auto-caches handler config on the first successful call", func() {
		ctr := newCtr("ctr-auto-cache", removedRuntime)

		impl, err := sut.RuntimeImpl(ctr)
		Expect(err).ToNot(HaveOccurred())
		Expect(impl).NotTo(BeNil())

		simulateConfigReload()

		ctr2 := newCtr("ctr-auto-cache-2", removedRuntime)
		_, err = sut.RuntimeImpl(ctr2)
		Expect(err).To(HaveOccurred(), "different container ID has no cached config")
	})

	It("EnsureRuntimeHandlerConfig is a silent no-op for an unknown handler", func() {
		ctr := newCtr("ctr-unknown", "nonexistent-handler")

		// Must not panic or return an error.
		Expect(func() { sut.EnsureRuntimeHandlerConfig(ctr) }).ToNot(Panic())

		// No fallback entry was created, so RuntimeImpl still fails.
		_, err := sut.RuntimeImpl(ctr)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("nonexistent-handler"))
	})

	It("EnsureRuntimeHandlerConfig is idempotent", func() {
		ctr := newCtr("ctr-idempotent", removedRuntime)

		sut.EnsureRuntimeHandlerConfig(ctr)
		sut.EnsureRuntimeHandlerConfig(ctr) // second call must be a no-op

		simulateConfigReload()

		impl, err := sut.RuntimeImpl(ctr)
		Expect(err).ToNot(HaveOccurred())
		Expect(impl).NotTo(BeNil())
	})

	It("EnsureRuntimeHandlerConfig does nothing when handler already cached by RuntimeImpl", func() {
		ctr := newCtr("ctr-already-impl-cached", removedRuntime)

		// Populate runtimeImplMap (and runtimeHandlerConfigMap) via RuntimeImpl.
		_, err := sut.RuntimeImpl(ctr)
		Expect(err).ToNot(HaveOccurred())

		// The subsequent EnsureRuntimeHandlerConfig should be a no-op.
		Expect(func() { sut.EnsureRuntimeHandlerConfig(ctr) }).ToNot(Panic())

		simulateConfigReload()

		// Still succeeds because runtimeImplMap cache hit takes priority.
		impl, err := sut.RuntimeImpl(ctr)
		Expect(err).ToNot(HaveOccurred())
		Expect(impl).NotTo(BeNil())
	})

	// Handler-config cache isolation: each container ID is independent

	It("cached handler config for one container does not bleed into another", func() {
		ctr1 := newCtr("ctr-bleed-1", removedRuntime)
		ctr2 := newCtr("ctr-bleed-2", removedRuntime)

		// Only cache for ctr1.
		sut.EnsureRuntimeHandlerConfig(ctr1)

		simulateConfigReload()

		// ctr1 must succeed.
		impl, err := sut.RuntimeImpl(ctr1)
		Expect(err).ToNot(HaveOccurred())
		Expect(impl).NotTo(BeNil())

		// ctr2 must fail — it was never explicitly cached.
		_, err = sut.RuntimeImpl(ctr2)
		Expect(err).To(HaveOccurred())
	})

	// Default runtime is always usable (regression guard)

	It("RuntimeImpl still works for the default runtime after removedRuntime is gone", func() {
		ctr := newCtr("ctr-default", "crun")

		simulateConfigReload()

		impl, err := sut.RuntimeImpl(ctr)
		Expect(err).ToNot(HaveOccurred())
		Expect(impl).NotTo(BeNil())
	})
})
