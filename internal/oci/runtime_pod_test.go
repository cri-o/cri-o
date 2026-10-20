package oci_test

import (
	"os"
	"time"

	conmonClient "github.com/containers/conmon-rs/pkg/client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
	libconfig "github.com/cri-o/cri-o/pkg/config"
)

func setEnv(key, value string, set bool) {
	oldValue, wasSet := os.LookupEnv(key)

	if set {
		Expect(os.Setenv(key, value)).To(Succeed())
	} else {
		Expect(os.Unsetenv(key)).To(Succeed())
	}

	DeferCleanup(func() {
		if wasSet {
			Expect(os.Setenv(key, oldValue)).To(Succeed())
		} else {
			Expect(os.Unsetenv(key)).To(Succeed())
		}
	})
}

func newPodTestContainer(terminal, stdin, stdinOnce bool) *oci.Container {
	c, err := oci.NewContainer("id", "name", "bundlePath", "logPath",
		map[string]string{}, map[string]string{}, map[string]string{},
		"image", nil, nil, "", &types.ContainerMetadata{}, "sandbox",
		terminal, stdin, stdinOnce, "", "dir", time.Now(), "")
	Expect(err).ToNot(HaveOccurred())

	return c
}

var _ = t.Describe("RuntimePod", func() {
	Context("MonitorEnv", func() {
		It("should add the CRI-O values to the monitor env", func() {
			// Given
			setEnv("XDG_RUNTIME_DIR", "/run/user/1000", true)
			setEnv("PATH", "/crio/bin", true)

			handler := &libconfig.RuntimeHandler{MonitorEnv: []string{"FOO=bar"}}

			// When
			env := oci.MonitorEnv(handler)

			// Then
			Expect(env).To(Equal([]string{
				"FOO=bar",
				"CRI_O_EXTERNAL=1",
				"XDG_RUNTIME_DIR=/run/user/1000",
				"PATH=/crio/bin",
			}))
			Expect(handler.MonitorEnv).To(Equal([]string{"FOO=bar"}))
		})

		It("should not inherit the CRI-O env without a monitor env", func() {
			// Given
			setEnv("XDG_RUNTIME_DIR", "", false)
			setEnv("PATH", "", false)

			// When
			env := oci.MonitorEnv(&libconfig.RuntimeHandler{})

			// Then
			Expect(env).To(Equal([]string{"CRI_O_EXTERNAL=1"}))
		})

		It("should not share the monitor env between calls", func() {
			// Given
			monitorEnv := make([]string, 1, 10)
			monitorEnv[0] = "FOO=bar"
			handler := &libconfig.RuntimeHandler{MonitorEnv: monitorEnv}

			// When
			first := oci.MonitorEnv(handler)
			second := oci.MonitorEnv(handler)
			second[1] = "CHANGED=1"

			// Then
			Expect(first[1]).To(Equal("CRI_O_EXTERNAL=1"))
		})
	})

	Context("NewConmonServerConfig", func() {
		var runtime *oci.Runtime

		BeforeEach(func() {
			cfg, err := libconfig.DefaultConfig()
			Expect(err).ToNot(HaveOccurred())

			cfg.ContainerAttachSocketDir = t.MustTempDir("attach-socket")
			runtime, err = oci.New(cfg)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should pass the monitor env to the server", func() {
			// Given
			setEnv("XDG_RUNTIME_DIR", "", false)
			setEnv("PATH", "", false)

			handler := &libconfig.RuntimeHandler{
				RuntimePath: "/usr/bin/crun",
				MonitorPath: "/usr/bin/conmonrs",
				MonitorEnv: []string{
					"PATH=/usr/bin",
					"FOO=bar",
					"LOG_DRIVER=systemd",
					"HEAPTRACK_OUTPUT_PATH=/tmp",
					"HEAPTRACK_BINARY_PATH=/usr/bin/heaptrack",
				},
			}

			// When
			cfg := oci.NewConmonServerConfig(
				runtime, handler, newPodTestContainer(false, false, false),
			)

			// Then
			Expect(cfg.Env).To(Equal([]string{
				"PATH=/usr/bin",
				"FOO=bar",
				"CRI_O_EXTERNAL=1",
			}))
			Expect(cfg.LogDriver).To(Equal(conmonClient.LogDriverSystemd))
			Expect(cfg.Heaptrack.Enabled).To(BeTrue())
			Expect(cfg.Heaptrack.OutputPath).To(Equal("/tmp/cri-o.conmon-rs.id"))
			Expect(cfg.Heaptrack.BinaryPath).To(Equal("/usr/bin/heaptrack"))
			Expect(handler.MonitorEnv).To(HaveLen(5))
		})

		It("should not let the server inherit the CRI-O env", func() {
			// Given
			setEnv("CRIO_TEST_SENTINEL", "1", true)

			// When
			cfg := oci.NewConmonServerConfig(
				runtime, &libconfig.RuntimeHandler{}, newPodTestContainer(false, false, false),
			)

			// Then
			Expect(cfg.Env).NotTo(BeNil())
			Expect(cfg.Env).To(ContainElement("CRI_O_EXTERNAL=1"))
			Expect(cfg.Env).NotTo(ContainElement(HavePrefix("CRIO_TEST_SENTINEL=")))
			Expect(cfg.LogDriver).To(Equal(conmonClient.LogDriverNone))
			Expect(cfg.Heaptrack.Enabled).To(BeFalse())
		})
	})

	// The value only depends on the container config, the tty of an attach
	// request does not change it, so that all attaches of a container agree.
	DescribeTable("StopAfterStdinEOF",
		func(terminal, stdin, stdinOnce, expected bool) {
			c := newPodTestContainer(terminal, stdin, stdinOnce)
			Expect(oci.StopAfterStdinEOF(c)).To(Equal(expected))
		},
		Entry("stdin once without terminal", false, true, true, true),
		Entry("stdin without stdin once", false, true, false, false),
		Entry("stdin once with terminal", true, true, true, false),
		Entry("stdin with terminal", true, true, false, false),
		Entry("no stdin", false, false, true, false),
	)
})
