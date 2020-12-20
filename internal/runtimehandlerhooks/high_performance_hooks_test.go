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
)

const (
	fixturesDir = "fixtures/"
)

// The actual test suite
var _ = Describe("high_performance_hooks", func() {
	container, err := oci.NewContainer("containerID", "", "", "",
		make(map[string]string), make(map[string]string),
		make(map[string]string), "pauseImage", "", "",
		&oci.Metadata{}, "sandboxID", false, false,
		false, "", "", time.Now(), "")
	Expect(err).To(BeNil())

	var flags string

	BeforeEach(func() {
		err := os.MkdirAll(fixturesDir, os.ModePerm)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		err := os.RemoveAll(fixturesDir)
		Expect(err).To(BeNil())
	})

	Describe("setCPUSLoadBalancing", func() {
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

				err = ioutil.WriteFile(filepath.Join(flagsDir, "flags"), []byte(flags), 0o644)
				Expect(err).To(BeNil())
			}
		})

		AfterEach(func() {
			for _, cpu := range []string{"cpu0", "cpu1"} {
				if err := os.RemoveAll(filepath.Join(fixturesDir, cpu)); err != nil {
					log.Errorf(context.TODO(), "failed to remove temporary test files: %v", err)
				}
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

	Describe("setIRQLoadBalancing", func() {
		irqSmpAffinityFile := filepath.Join(fixturesDir, "irq_smp_affinity")
		verifySetIRQLoadBalancing := func(enabled bool, expected string) {
			err := setIRQLoadBalancing(container, enabled, irqSmpAffinityFile)
			Expect(err).To(BeNil())

			content, err := ioutil.ReadFile(irqSmpAffinityFile)
			Expect(err).To(BeNil())

			Expect(strings.Trim(string(content), "\n")).To(Equal(expected))
		}

		JustBeforeEach(func() {
			// set container CPUs
			container.SetSpec(
				&specs.Spec{
					Linux: &specs.Linux{
						Resources: &specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Cpus: "4,5",
							},
						},
					},
				},
			)

			// create tests affinity file
			err = ioutil.WriteFile(irqSmpAffinityFile, []byte(flags), 0o644)
			Expect(err).To(BeNil())
		})

		Context("with enabled equals to true", func() {
			BeforeEach(func() {
				flags = "0000,00003003"
			})

			It("should set the irq bit mask", func() {
				verifySetIRQLoadBalancing(true, "00000000,00003033")
			})
		})

		Context("with enabled equals to false", func() {
			BeforeEach(func() {
				flags = "00000000,00003033"
			})

			It("should clear the irq bit mask", func() {
				verifySetIRQLoadBalancing(false, "00000000,00003003")
			})
		})
	})

	Describe("setCPUQuota", func() {
		containerID := container.ID()
		parent := "parent.slice"
		child := "crio" + "-" + containerID + ".scope"
		childCgroup := parent + ":" + "crio" + ":" + containerID
		cpuMountPoint := filepath.Join(fixturesDir, "cgroup", "cpu")
		parentFolder := filepath.Join(cpuMountPoint, parent)
		childFolder := filepath.Join(cpuMountPoint, parent, child)

		verifySetCPUQuota := func(enabled bool, expected string) {
			err := setCPUQuota(cpuMountPoint, parent, container, enabled)
			Expect(err).To(BeNil())

			content, err := ioutil.ReadFile(filepath.Join(childFolder, "cpu.cfs_quota_us"))
			Expect(err).To(BeNil())
			Expect(strings.Trim(string(content), "\n")).To(Equal(expected))

			content, err = ioutil.ReadFile(filepath.Join(parentFolder, "cpu.cfs_quota_us"))
			Expect(err).To(BeNil())
			Expect(strings.Trim(string(content), "\n")).To(Equal(expected))
		}

		BeforeEach(func() {
			if err := os.MkdirAll(childFolder, os.ModePerm); err != nil {
				log.Errorf(context.TODO(), "failed to create temporary cgroup folder: %v", err)
			}
			if err := ioutil.WriteFile(filepath.Join(parentFolder, "cpu.cfs_quota_us"), []byte("900\n"), 0o644); err != nil {
				log.Errorf(context.TODO(), "failed to create cpu.cfs_quota_us cgroup file: %v", err)
			}
			if err := ioutil.WriteFile(filepath.Join(childFolder, "cpu.cfs_quota_us"), []byte("900\n"), 0o644); err != nil {
				log.Errorf(context.TODO(), "failed to create cpu.cfs_quota_us cgroup file: %v", err)
			}
			container.SetSpec(
				&specs.Spec{
					Linux: &specs.Linux{
						CgroupsPath: childCgroup,
					},
				},
			)
		})

		AfterEach(func() {
			if err := os.RemoveAll(parentFolder); err != nil {
				log.Errorf(context.TODO(), "failed to remove temporary cgroup folder: %v", err)
			}
		})

		Context("with enabled equals to true", func() {
			It("should set cpu.cfs_quota_us to 0", func() {
				verifySetCPUQuota(true, "0")
			})
		})

		Context("with enabled equals to false", func() {
			It("should set cpu.cfs_quota_us to -1", func() {
				verifySetCPUQuota(false, "-1")
			})
		})
	})
})
