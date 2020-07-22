package runtimehandlerhooks

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runtime-spec/specs-go"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

const (
	fixturesDir = "fixtures/"
)

// The actual test suite
var _ = Describe("setCPUSLoadBalancing", func() {
	var container *oci.Container
	var flags string

	verifySetCPULoadBalancing := func(enabled bool, expected string) {
		err := setCPUSLoadBalancing(container, enabled, fixturesDir)
		Expect(err).To(BeNil())

		for _, cpu := range []string{"cpu0", "cpu1"} {
			content, err := ioutil.ReadFile(filepath.Join(fixturesDir, cpu, "domain0", "flags"))
			Expect(err).To(BeNil())

			Expect(strings.Trim(string(content), "\n")).To(Equal(expected))
		}
	}

	JustBeforeEach(func() {
		var err error
		container, err = oci.NewContainer("containerID", "", "", "",
			make(map[string]string), make(map[string]string),
			make(map[string]string), "pauseImage", "", "",
			&pb.ContainerMetadata{}, "sandboxID", false, false,
			false, "", "", time.Now(), "")
		Expect(err).To(BeNil())

		// set container CPUs
		container.SetSpec(
			&specs.Spec{
				Linux: &specs.Linux{
					Resources: &specs.LinuxResources{
						CPU: &specs.LinuxCPU{
							Cpus: "0,1",
						},
					},
				},
			},
		)

		// create tests flags files
		for _, cpu := range []string{"cpu0", "cpu1"} {
			flagsDir := filepath.Join(fixturesDir, cpu, "domain0")
			err = os.MkdirAll(flagsDir, os.ModePerm)
			Expect(err).To(BeNil())

			err = ioutil.WriteFile(filepath.Join(flagsDir, "flags"), []byte(flags), 0644)
			Expect(err).To(BeNil())
		}
	})

	AfterEach(func() {
		for _, cpu := range []string{"cpu0", "cpu1"} {
			err := os.RemoveAll(filepath.Join(fixturesDir, cpu))
			log.Errorf(context.TODO(), "failed to remove temporary test files: %v", err)
		}
	})

	Context("with enabled equals to true", func() {
		BeforeEach(func() {
			flags = "4142"
		})

		It("should enable the CPU load balancing", func() {
			verifySetCPULoadBalancing(true, "4143")
		})
	})

	Context("with enabled equals to false", func() {
		BeforeEach(func() {
			flags = "4143"
		})

		It("should disable the CPU load balancing", func() {
			verifySetCPULoadBalancing(false, "4142")
		})
	})
})
