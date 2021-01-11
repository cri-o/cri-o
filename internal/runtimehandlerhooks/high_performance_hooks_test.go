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

	var flags, bannedCPUFlags string

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

	Describe("setIRQLoadBalancingUsingDaemonCommand", func() {
		irqSmpAffinityFile := filepath.Join(fixturesDir, "irq_smp_affinity")
		irqBalanceConfigFile := filepath.Join(fixturesDir, "irqbalance")
		verifySetIRQLoadBalancing := func(enabled bool, expected string) {
			err := setIRQLoadBalancing(container, enabled, irqSmpAffinityFile, irqBalanceConfigFile)
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

	Describe("setIRQLoadBalancingUsingServiceRestart", func() {
		irqSmpAffinityFile := filepath.Join(fixturesDir, "irq_smp_affinity")
		irqBalanceConfigFile := filepath.Join(fixturesDir, "irqbalance")
		verifySetIRQLoadBalancing := func(enabled bool, expectedSmp, expectedBan string) {
			err = setIRQLoadBalancing(container, enabled, irqSmpAffinityFile, irqBalanceConfigFile)
			Expect(err).To(BeNil())

			content, err := ioutil.ReadFile(irqSmpAffinityFile)
			Expect(err).To(BeNil())

			Expect(strings.Trim(string(content), "\n")).To(Equal(expectedSmp))

			bannedCPUs, err := retrieveIrqBannedCPUMasks(irqBalanceConfigFile)
			Expect(err).To(BeNil())

			Expect(bannedCPUs).To(Equal(expectedBan))
		}

		JustBeforeEach(func() {
			// set irqbalanace config file with no banned cpus
			err = ioutil.WriteFile(irqBalanceConfigFile, []byte(""), 0o644)
			Expect(err).To(BeNil())
			err = updateIrqBalanceConfigFile(irqBalanceConfigFile, bannedCPUFlags)
			Expect(err).To(BeNil())
			bannedCPUs, err := retrieveIrqBannedCPUMasks(irqBalanceConfigFile)
			Expect(err).To(BeNil())
			Expect(bannedCPUs).To(Equal(bannedCPUFlags))
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
				flags = "00000000,00003003"
				bannedCPUFlags = "ffffffff,ffffcffc"
			})

			It("should set the irq bit mask", func() {
				verifySetIRQLoadBalancing(true, "00000000,00003033", "ffffffff,ffffcfcc")
			})
		})

		Context("with enabled equals to false", func() {
			BeforeEach(func() {
				flags = "00000000,00003033"
				bannedCPUFlags = "ffffffff,ffffcfcc"
			})

			It("should clear the irq bit mask", func() {
				verifySetIRQLoadBalancing(false, "00000000,00003003", "ffffffff,ffffcffc")
			})
		})
	})

	Describe("restoreIrqBalanceConfig", func() {
		irqSmpAffinityFile := filepath.Join(fixturesDir, "irq_smp_affinity")
		irqBalanceConfigFile := filepath.Join(fixturesDir, "irqbalance")
		irqBannedCPUConfigFile := filepath.Join(fixturesDir, "orig_irq_banned_cpus")
		verifyRestoreIrqBalanceConfig := func(expectedOrigBannedCPUs, expectedBannedCPUs string) {
			err = RestoreIrqBalanceConfig(irqBalanceConfigFile, irqBannedCPUConfigFile, irqSmpAffinityFile)
			Expect(err).To(BeNil())

			content, err := ioutil.ReadFile(irqBannedCPUConfigFile)
			Expect(err).To(BeNil())
			Expect(strings.Trim(string(content), "\n")).To(Equal(expectedOrigBannedCPUs))

			bannedCPUs, err := retrieveIrqBannedCPUMasks(irqBalanceConfigFile)
			Expect(err).To(BeNil())
			Expect(bannedCPUs).To(Equal(expectedBannedCPUs))
		}

		JustBeforeEach(func() {
			// create tests affinity file
			err = ioutil.WriteFile(irqSmpAffinityFile, []byte("ffffffff,ffffffff"), 0o644)
			Expect(err).To(BeNil())
			// set irqbalanace config file with banned cpus mask
			err = ioutil.WriteFile(irqBalanceConfigFile, []byte(""), 0o644)
			Expect(err).To(BeNil())
			err = updateIrqBalanceConfigFile(irqBalanceConfigFile, "0000ffff,ffffcfcc")
			Expect(err).To(BeNil())
			bannedCPUs, err := retrieveIrqBannedCPUMasks(irqBalanceConfigFile)
			Expect(err).To(BeNil())
			Expect(bannedCPUs).To(Equal("0000ffff,ffffcfcc"))
		})

		Context("when banned cpu config file doesn't exist", func() {
			BeforeEach(func() {
				// ensure banned cpu config file doesn't exist
				os.Remove(irqBannedCPUConfigFile)
			})

			It("should set banned cpu config file from irq balance config", func() {
				verifyRestoreIrqBalanceConfig("0000ffff,ffffcfcc", "0000ffff,ffffcfcc")
			})
		})

		Context("when banned cpu config file exists", func() {
			BeforeEach(func() {
				// create banned cpu config file
				os.Remove(irqBannedCPUConfigFile)
				err = ioutil.WriteFile(irqBannedCPUConfigFile, []byte("00000000,00000000"), 0o644)
				Expect(err).To(BeNil())
			})

			It("should restore irq balance config with content from banned cpu config file", func() {
				verifyRestoreIrqBalanceConfig("00000000,00000000", "00000000,00000000")
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
