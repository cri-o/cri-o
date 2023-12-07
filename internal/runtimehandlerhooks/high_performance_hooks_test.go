package runtimehandlerhooks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runtime-spec/specs-go"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const (
	fixturesDir = "fixtures/"
)

// The actual test suite
var _ = Describe("high_performance_hooks", func() {
	container, err := oci.NewContainer("containerID", "", "", "",
		make(map[string]string), make(map[string]string),
		make(map[string]string), "pauseImage", nil, nil,
		&types.ContainerMetadata{}, "sandboxID", false, false,
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

	Describe("setIRQLoadBalancingUsingDaemonCommand", func() {
		irqSmpAffinityFile := filepath.Join(fixturesDir, "irq_smp_affinity")
		irqBalanceConfigFile := filepath.Join(fixturesDir, "irqbalance")
		verifySetIRQLoadBalancing := func(enabled bool, expected string) {
			err := setIRQLoadBalancing(context.TODO(), container, enabled, irqSmpAffinityFile, irqBalanceConfigFile)
			Expect(err).To(BeNil())

			content, err := os.ReadFile(irqSmpAffinityFile)
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
			err = os.WriteFile(irqSmpAffinityFile, []byte(flags), 0o644)
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
			err = setIRQLoadBalancing(context.TODO(), container, enabled, irqSmpAffinityFile, irqBalanceConfigFile)
			Expect(err).To(BeNil())

			content, err := os.ReadFile(irqSmpAffinityFile)
			Expect(err).To(BeNil())

			Expect(strings.Trim(string(content), "\n")).To(Equal(expectedSmp))

			bannedCPUs, err := retrieveIrqBannedCPUMasks(irqBalanceConfigFile)
			Expect(err).To(BeNil())

			Expect(bannedCPUs).To(Equal(expectedBan))
		}

		JustBeforeEach(func() {
			// set irqbalanace config file with no banned cpus
			err = os.WriteFile(irqBalanceConfigFile, []byte(""), 0o644)
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
			err = os.WriteFile(irqSmpAffinityFile, []byte(flags), 0o644)
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

	Describe("setCPUPMQOSResumeLatency", func() {
		var pmQosResumeLatencyUs, pmQosResumeLatencyUsOriginal string
		cpuDir := filepath.Join(fixturesDir, "cpu")
		cpuSaveDir := filepath.Join(fixturesDir, "cpuSave")

		//nolint:dupl
		verifySetCPUPMQOSResumeLatency := func(latency string, expected string, expected_save string, expect_error bool) {
			err := doSetCPUPMQOSResumeLatency(container, latency, cpuDir, cpuSaveDir)
			if !expect_error {
				Expect(err).ShouldNot(HaveOccurred())
			} else {
				Expect(err).Should(HaveOccurred())
			}

			if expected != "" {
				for _, cpu := range []string{"cpu0", "cpu1"} {
					content, err := os.ReadFile(filepath.Join(cpuDir, cpu, "power", "pm_qos_resume_latency_us"))
					Expect(err).To(BeNil())

					Expect(strings.Trim(string(content), "\n")).To(Equal(expected))
				}
			}

			if expected_save != "" {
				for _, cpu := range []string{"cpu0", "cpu1"} {
					content, err := os.ReadFile(filepath.Join(cpuSaveDir, cpu, "power", "pm_qos_resume_latency_us"))
					Expect(err).To(BeNil())
					Expect(strings.Trim(string(content), "\n")).To(Equal(expected_save))
				}
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

			// create tests power files
			for _, cpu := range []string{"cpu0", "cpu1"} {
				powerDir := filepath.Join(cpuDir, cpu, "power")
				err = os.MkdirAll(powerDir, os.ModePerm)
				Expect(err).To(BeNil())

				if pmQosResumeLatencyUs != "" {
					err = os.WriteFile(filepath.Join(powerDir, "pm_qos_resume_latency_us"), []byte(pmQosResumeLatencyUs), 0o644)
					Expect(err).To(BeNil())
				}
				if pmQosResumeLatencyUsOriginal != "" {
					powerSaveDir := filepath.Join(cpuSaveDir, cpu, "power")
					err = os.MkdirAll(powerSaveDir, os.ModePerm)
					err = os.WriteFile(filepath.Join(powerSaveDir, "pm_qos_resume_latency_us"), []byte(pmQosResumeLatencyUsOriginal), 0o644)
					Expect(err).To(BeNil())
				}
			}
		})

		AfterEach(func() {
			for _, cpu := range []string{"cpu0", "cpu1"} {
				if err := os.RemoveAll(filepath.Join(fixturesDir, cpu)); err != nil {
					log.Errorf(context.TODO(), "failed to remove temporary test files: %v", err)
				}
			}
		})

		Context("with n/a latency", func() {
			BeforeEach(func() {
				pmQosResumeLatencyUs = "0"
				pmQosResumeLatencyUsOriginal = ""
			})

			It("should change the CPU PM QOS latency", func() {
				verifySetCPUPMQOSResumeLatency("n/a", "n/a", "0", false)
			})
		})

		Context("with 0 latency", func() {
			BeforeEach(func() {
				pmQosResumeLatencyUs = "n/a"
				pmQosResumeLatencyUsOriginal = ""
			})

			It("should change the CPU PM QOS latency", func() {
				verifySetCPUPMQOSResumeLatency("0", "0", "n/a", false)
			})
		})

		Context("with 10 latency", func() {
			BeforeEach(func() {
				pmQosResumeLatencyUs = "n/a"
				pmQosResumeLatencyUsOriginal = ""
			})

			It("should change the CPU PM QOS latency", func() {
				verifySetCPUPMQOSResumeLatency("10", "10", "n/a", false)
			})
		})

		Context("with missing latency file", func() {
			BeforeEach(func() {
				pmQosResumeLatencyUs = ""
				pmQosResumeLatencyUsOriginal = ""
			})

			It("should fail", func() {
				verifySetCPUPMQOSResumeLatency("n/a", "", "", true)
			})
		})

		Context("with no latency", func() {
			BeforeEach(func() {
				pmQosResumeLatencyUs = "n/a"
				pmQosResumeLatencyUsOriginal = "0"
			})

			It("should restore the original CPU PM QOS latency", func() {
				verifySetCPUPMQOSResumeLatency("", "0", "", false)
			})
		})

		Context("with no latency and no original latency", func() {
			BeforeEach(func() {
				pmQosResumeLatencyUs = "0"
				pmQosResumeLatencyUsOriginal = ""
			})

			It("should not change the CPU PM QOS latency", func() {
				verifySetCPUPMQOSResumeLatency("", "0", "", false)
			})
		})
	})

	Describe("setCPUScalingGovernor", func() {
		var scalingGovernor, scalingAvailableGovernors, scalingGovernorOriginal string
		cpuDir := filepath.Join(fixturesDir, "cpu")
		cpuSaveDir := filepath.Join(fixturesDir, "cpuSave")

		//nolint:dupl
		verifySetCPUScalingGovernor := func(governor string, expected string, expected_save string, expect_error bool) {
			err := doSetCPUFreqGovernor(container, governor, cpuDir, cpuSaveDir)
			if !expect_error {
				Expect(err).ShouldNot(HaveOccurred())
			} else {
				Expect(err).Should(HaveOccurred())
			}

			if expected != "" {
				for _, cpu := range []string{"cpu0", "cpu1"} {
					content, err := os.ReadFile(filepath.Join(cpuDir, cpu, "cpufreq", "scaling_governor"))
					Expect(err).To(BeNil())
					Expect(strings.Trim(string(content), "\n")).To(Equal(expected))
				}
			}

			if expected_save != "" {
				for _, cpu := range []string{"cpu0", "cpu1"} {
					content, err := os.ReadFile(filepath.Join(cpuSaveDir, cpu, "cpufreq", "scaling_governor"))
					Expect(err).To(BeNil())
					Expect(strings.Trim(string(content), "\n")).To(Equal(expected_save))
				}
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

			// create tests cpufreq files
			for _, cpu := range []string{"cpu0", "cpu1"} {
				cpufreqDir := filepath.Join(cpuDir, cpu, "cpufreq")
				err = os.MkdirAll(cpufreqDir, os.ModePerm)
				Expect(err).To(BeNil())

				if scalingGovernor != "" {
					err = os.WriteFile(filepath.Join(cpufreqDir, "scaling_governor"), []byte(scalingGovernor), 0o644)
					Expect(err).To(BeNil())
				}
				if scalingAvailableGovernors != "" {
					err = os.WriteFile(filepath.Join(cpufreqDir, "scaling_available_governors"), []byte(scalingAvailableGovernors), 0o644)
					Expect(err).To(BeNil())
				}
				if scalingGovernorOriginal != "" {
					cpufreqSaveDir := filepath.Join(cpuSaveDir, cpu, "cpufreq")
					err = os.MkdirAll(cpufreqSaveDir, os.ModePerm)
					err = os.WriteFile(filepath.Join(cpufreqSaveDir, "scaling_governor"), []byte(scalingGovernorOriginal), 0o644)
					Expect(err).To(BeNil())
				}
			}
		})

		AfterEach(func() {
			for _, cpu := range []string{"cpu0", "cpu1"} {
				if err := os.RemoveAll(filepath.Join(cpuDir, cpu)); err != nil {
					log.Errorf(context.TODO(), "failed to remove temporary test files: %v", err)
				}
			}
		})

		Context("with available governor", func() {
			BeforeEach(func() {
				scalingGovernor = "schedutil"
				scalingAvailableGovernors = "conservative ondemand userspace powersave performance schedutil"
				scalingGovernorOriginal = ""
			})

			It("should change the CPU scaling governor", func() {
				verifySetCPUScalingGovernor("performance", "performance", "schedutil", false)
			})
		})

		Context("with unknown governor", func() {
			BeforeEach(func() {
				scalingGovernor = "schedutil"
				scalingAvailableGovernors = "conservative ondemand powersave performance schedutil"
				scalingGovernorOriginal = ""
			})

			It("should fail and not change the CPU scaling governor", func() {
				verifySetCPUScalingGovernor("userspace", "schedutil", "", true)
			})
		})

		Context("with no scaling governor support", func() {
			BeforeEach(func() {
				scalingGovernor = ""
				scalingAvailableGovernors = ""
				scalingGovernorOriginal = ""
			})

			It("should fail", func() {
				verifySetCPUScalingGovernor("performance", "", "", true)
			})
		})

		Context("with no available scaling governors", func() {
			BeforeEach(func() {
				scalingGovernor = "conservative"
				scalingAvailableGovernors = ""
				scalingGovernorOriginal = ""
			})

			It("should fail", func() {
				verifySetCPUScalingGovernor("performance", "", "", true)
			})
		})

		Context("with no governor", func() {
			BeforeEach(func() {
				scalingGovernor = "userspace"
				scalingAvailableGovernors = "conservative ondemand userspace powersave performance"
				scalingGovernorOriginal = "ondemand"
			})

			It("should restore the original CPU scaling governor", func() {
				verifySetCPUScalingGovernor("", "ondemand", "", false)
			})
		})

		Context("with no governor and no original governor", func() {
			BeforeEach(func() {
				scalingGovernor = "powersave"
				scalingAvailableGovernors = "conservative ondemand userspace powersave performance schedutil"
				scalingGovernorOriginal = ""
			})

			It("should not change the CPU scaling governor", func() {
				verifySetCPUScalingGovernor("", "powersave", "", false)
			})
		})
	})

	Describe("restoreIrqBalanceConfig", func() {
		irqSmpAffinityFile := filepath.Join(fixturesDir, "irq_smp_affinity")
		irqBalanceConfigFile := filepath.Join(fixturesDir, "irqbalance")
		irqBannedCPUConfigFile := filepath.Join(fixturesDir, "orig_irq_banned_cpus")
		verifyRestoreIrqBalanceConfig := func(expectedOrigBannedCPUs, expectedBannedCPUs string) {
			err = RestoreIrqBalanceConfig(context.TODO(), irqBalanceConfigFile, irqBannedCPUConfigFile, irqSmpAffinityFile)
			ExpectWithOffset(1, err).To(BeNil())

			content, err := os.ReadFile(irqBannedCPUConfigFile)
			ExpectWithOffset(1, err).To(BeNil())
			ExpectWithOffset(1, strings.Trim(string(content), "\n")).To(Equal(expectedOrigBannedCPUs))

			bannedCPUs, err := retrieveIrqBannedCPUMasks(irqBalanceConfigFile)
			ExpectWithOffset(1, err).To(BeNil())
			ExpectWithOffset(1, bannedCPUs).To(Equal(expectedBannedCPUs))
		}

		JustBeforeEach(func() {
			// create tests affinity file
			err = os.WriteFile(irqSmpAffinityFile, []byte("ffffffff,ffffffff"), 0o644)
			Expect(err).To(BeNil())
			// set irqbalanace config file with banned cpus mask
			err = os.WriteFile(irqBalanceConfigFile, []byte(""), 0o644)
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
				err = os.WriteFile(irqBannedCPUConfigFile, []byte("00000000,00000000"), 0o644)
				Expect(err).To(BeNil())
			})

			It("should restore irq balance config with content from banned cpu config file", func() {
				verifyRestoreIrqBalanceConfig("00000000,00000000", "00000000,00000000")
			})
		})
	})

	Describe("convertAnnotationToLatency", func() {
		verifyConvertAnnotationToLatency := func(annotation string, expected string, expect_error bool) {
			latency, err := convertAnnotationToLatency(annotation)
			if !expect_error {
				Expect(err).ShouldNot(HaveOccurred())
			} else {
				Expect(err).Should(HaveOccurred())
			}

			if expected != "" {
				Expect(err).To(BeNil())
				Expect(latency).To(Equal(expected))
			}
		}

		Context("with enable annotation", func() {
			It("should result in latency: 0", func() {
				verifyConvertAnnotationToLatency("enable", "0", false)
			})
		})

		Context("with disable annotation", func() {
			It("should result in latency: n/a", func() {
				verifyConvertAnnotationToLatency("disable", "n/a", false)
			})
		})

		Context("with max_latency:10 annotation", func() {
			It("should result in latency: 10", func() {
				verifyConvertAnnotationToLatency("max_latency:10", "10", false)
			})
		})

		Context("with max_latency:1 annotation", func() {
			It("should result in latency: 1", func() {
				verifyConvertAnnotationToLatency("max_latency:1", "1", false)
			})
		})

		Context("with max_latency:0 annotation", func() {
			It("should result in error", func() {
				verifyConvertAnnotationToLatency("max_latency:0", "", true)
			})
		})

		Context("with max_latency:-1 annotation", func() {
			It("should result in error", func() {
				verifyConvertAnnotationToLatency("max_latency:-1", "", true)
			})
		})

		Context("with max_latency:bad annotation", func() {
			It("should result in error", func() {
				verifyConvertAnnotationToLatency("max_latency:bad", "", true)
			})
		})

		Context("with bad annotation", func() {
			It("should result in error", func() {
				verifyConvertAnnotationToLatency("bad", "", true)
			})
		})
	})
	Describe("setSharedCPUs", func() {
		Context("with empty container CPUs list", func() {
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
			It("should result in error", func() {
				_, err := setSharedCPUs(container, nil, "")
				Expect(err).To(HaveOccurred())
			})
		})
		Context("with empty shared CPUs list", func() {
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
			It("should result in error", func() {
				_, err := setSharedCPUs(container, nil, "")
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
