package oci_test

import (
	"fmt"
	"sync"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/internal/oci"
	libconfig "github.com/cri-o/cri-o/pkg/config"
)

var _ = t.Describe("Runtime", func() {
	t.Describe("ConcurrentReload", func() {
		It("should serve consistent lookups while reloads publish new tables", func() {
			// Given a runtime whose configuration is continuously reloaded
			// by a writer goroutine, while readers resolve the default
			// runtime handler concurrently, like the gRPC handlers of a
			// running daemon do during a SIGHUP reload.
			cfg, err := libconfig.DefaultConfig()
			Expect(err).ToNot(HaveOccurred())
			cfg.ContainerAttachSocketDir = t.MustTempDir("crio")
			cfg.Runtimes = libconfig.Runtimes{
				"crun": &libconfig.RuntimeHandler{RuntimePath: "/bin/true"},
			}
			cfg.DefaultRuntime = "crun"

			runtime, err := oci.New(cfg)
			Expect(err).ToNot(HaveOccurred())

			newConfig, err := libconfig.DefaultConfig()
			Expect(err).ToNot(HaveOccurred())

			const iterations = 100
			const readers = 4

			var (
				wg       sync.WaitGroup
				errCount atomic.Int32
				errMu    sync.Mutex
				firstErr error
			)
			recordErr := func(err error) {
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()

				errCount.Add(1)
			}

			stop := make(chan struct{})

			// The writer publishes a new runtime table on every iteration.
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer close(stop)

				for i := range iterations {
					newConfig.Runtimes = libconfig.Runtimes{
						"crun": &libconfig.RuntimeHandler{
							RuntimePath: "/bin/true",
						},
						fmt.Sprintf("handler-%d", i): &libconfig.RuntimeHandler{
							RuntimePath: "/bin/true",
						},
					}
					newConfig.DefaultRuntime = "crun"

					if err := cfg.ReloadRuntimes(newConfig); err != nil {
						recordErr(fmt.Errorf("reload %d failed: %w", i, err))

						return
					}
				}
			}()

			// The readers must never observe a snapshot without a valid
			// default runtime handler.
			for range readers {
				wg.Add(1)
				go func() {
					defer wg.Done()

					for {
						select {
						case <-stop:
							return
						default:
						}

						if _, err := runtime.RuntimeType(""); err != nil {
							recordErr(fmt.Errorf("default handler lookup failed: %w", err))

							return
						}

						if _, err := runtime.ValidateRuntimeHandler("crun"); err != nil {
							recordErr(fmt.Errorf("crun lookup failed: %w", err))

							return
						}
					}
				}()
			}

			wg.Wait()

			// Then: no reload failed and no lookup observed a torn or empty
			// runtime configuration.
			errMu.Lock()
			defer errMu.Unlock()

			Expect(firstErr).To(BeNil())
			Expect(errCount.Load()).To(BeNumerically("==", 0))
			Expect(cfg.RuntimeSnapshot().DefaultRuntime).To(Equal("crun"))
			Expect(cfg.RuntimeSnapshot().RuntimeHandler("")).ToNot(BeNil())
		})
	})
})
