package runtimehandlerhooks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	crioannotations "github.com/cri-o/cri-o/pkg/annotations"
)

const (
	fixturesDir = "fixtures/"

	latencyNA = "n/a"

	// A list of CPU governors typically supported on Linux.
	governorConservative = "conservative"
	governorOndemand     = "ondemand"
	governorPerformance  = "performance"
	governorPowersave    = "powersave"
	governorSchedutil    = "schedutil"
	governorUserspace    = "userspace"
)

// The actual test suite.
var _ = Describe("high_performance_hooks", func() {
	container, err := oci.NewContainer("containerID", "", "", "",
		make(map[string]string), make(map[string]string),
		make(map[string]string), "pauseImage", nil, nil, "",
		&types.ContainerMetadata{}, "sandboxID", false, false,
		false, "", "", time.Now(), "")
	Expect(err).ToNot(HaveOccurred())

	var flags, bannedCPUFlags string

	BeforeEach(func() {
		err := os.MkdirAll(fixturesDir, os.ModePerm)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		err := os.RemoveAll(fixturesDir)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("setIRQLoadBalancingUsingDaemonCommand", func() {
		irqSmpAffinityFile := filepath.Join(fixturesDir, "irq_smp_affinity")
		irqBalanceConfigFile := filepath.Join(fixturesDir, "irqbalance")
		verifySetIRQLoadBalancing := func(enabled bool, expected string) {
			err := setIRQLoadBalancing(context.TODO(), container, enabled, irqSmpAffinityFile, irqBalanceConfigFile)
			Expect(err).ToNot(HaveOccurred())

			content, err := os.ReadFile(irqSmpAffinityFile)
			Expect(err).ToNot(HaveOccurred())

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
			Expect(err).ToNot(HaveOccurred())
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
			Expect(err).ToNot(HaveOccurred())

			content, err := os.ReadFile(irqSmpAffinityFile)
			Expect(err).ToNot(HaveOccurred())

			Expect(strings.Trim(string(content), "\n")).To(Equal(expectedSmp))

			bannedCPUs, err := retrieveIrqBannedCPUMasks(irqBalanceConfigFile)
			Expect(err).ToNot(HaveOccurred())

			Expect(bannedCPUs).To(Equal(expectedBan))
		}

		JustBeforeEach(func() {
			// set irqbalanace config file with no banned cpus
			err = os.WriteFile(irqBalanceConfigFile, []byte(""), 0o644)
			Expect(err).ToNot(HaveOccurred())
			err = updateIrqBalanceConfigFile(irqBalanceConfigFile, bannedCPUFlags)
			Expect(err).ToNot(HaveOccurred())
			bannedCPUs, err := retrieveIrqBannedCPUMasks(irqBalanceConfigFile)
			Expect(err).ToNot(HaveOccurred())
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
			Expect(err).ToNot(HaveOccurred())
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
					Expect(err).ToNot(HaveOccurred())

					Expect(strings.Trim(string(content), "\n")).To(Equal(expected))
				}
			}

			if expected_save != "" {
				for _, cpu := range []string{"cpu0", "cpu1"} {
					content, err := os.ReadFile(filepath.Join(cpuSaveDir, cpu, "power", "pm_qos_resume_latency_us"))
					Expect(err).ToNot(HaveOccurred())
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
				Expect(err).ToNot(HaveOccurred())

				if pmQosResumeLatencyUs != "" {
					err = os.WriteFile(filepath.Join(powerDir, "pm_qos_resume_latency_us"), []byte(pmQosResumeLatencyUs), 0o644)
					Expect(err).ToNot(HaveOccurred())
				}
				if pmQosResumeLatencyUsOriginal != "" {
					powerSaveDir := filepath.Join(cpuSaveDir, cpu, "power")
					err = os.MkdirAll(powerSaveDir, os.ModePerm)
					err = os.WriteFile(filepath.Join(powerSaveDir, "pm_qos_resume_latency_us"), []byte(pmQosResumeLatencyUsOriginal), 0o644)
					Expect(err).ToNot(HaveOccurred())
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
				verifySetCPUPMQOSResumeLatency(latencyNA, latencyNA, "0", false)
			})
		})

		Context("with n/a latency and latency already saved", func() {
			BeforeEach(func() {
				pmQosResumeLatencyUs = latencyNA
				pmQosResumeLatencyUsOriginal = "0"
			})

			It("should not change the saved CPU PM QOS latency", func() {
				verifySetCPUPMQOSResumeLatency(latencyNA, latencyNA, "0", false)
			})
		})

		Context("with 0 latency", func() {
			BeforeEach(func() {
				pmQosResumeLatencyUs = latencyNA
				pmQosResumeLatencyUsOriginal = ""
			})

			It("should change the CPU PM QOS latency", func() {
				verifySetCPUPMQOSResumeLatency("0", "0", latencyNA, false)
			})
		})

		Context("with 10 latency", func() {
			BeforeEach(func() {
				pmQosResumeLatencyUs = latencyNA
				pmQosResumeLatencyUsOriginal = ""
			})

			It("should change the CPU PM QOS latency", func() {
				verifySetCPUPMQOSResumeLatency("10", "10", latencyNA, false)
			})
		})

		Context("with missing latency file", func() {
			BeforeEach(func() {
				pmQosResumeLatencyUs = ""
				pmQosResumeLatencyUsOriginal = ""
			})

			It("should fail", func() {
				verifySetCPUPMQOSResumeLatency(latencyNA, "", "", true)
			})
		})

		Context("with no latency", func() {
			BeforeEach(func() {
				pmQosResumeLatencyUs = latencyNA
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
					Expect(err).ToNot(HaveOccurred())
					Expect(strings.Trim(string(content), "\n")).To(Equal(expected))
				}
			}

			if expected_save != "" {
				for _, cpu := range []string{"cpu0", "cpu1"} {
					content, err := os.ReadFile(filepath.Join(cpuSaveDir, cpu, "cpufreq", "scaling_governor"))
					Expect(err).ToNot(HaveOccurred())
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
				Expect(err).ToNot(HaveOccurred())

				if scalingGovernor != "" {
					err = os.WriteFile(filepath.Join(cpufreqDir, "scaling_governor"), []byte(scalingGovernor), 0o644)
					Expect(err).ToNot(HaveOccurred())
				}
				if scalingAvailableGovernors != "" {
					err = os.WriteFile(filepath.Join(cpufreqDir, "scaling_available_governors"), []byte(scalingAvailableGovernors), 0o644)
					Expect(err).ToNot(HaveOccurred())
				}
				if scalingGovernorOriginal != "" {
					cpufreqSaveDir := filepath.Join(cpuSaveDir, cpu, "cpufreq")
					err = os.MkdirAll(cpufreqSaveDir, os.ModePerm)
					err = os.WriteFile(filepath.Join(cpufreqSaveDir, "scaling_governor"), []byte(scalingGovernorOriginal), 0o644)
					Expect(err).ToNot(HaveOccurred())
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
				scalingGovernor = governorSchedutil
				scalingAvailableGovernors = strings.Join([]string{
					governorConservative,
					governorOndemand,
					governorPerformance,
					governorPowersave,
					governorSchedutil,
					governorUserspace,
				}, " ")
				scalingGovernorOriginal = ""
			})

			It("should change the CPU scaling governor", func() {
				verifySetCPUScalingGovernor(governorPerformance, governorPerformance, governorSchedutil, false)
			})
		})

		Context("with available governor and governor already saved", func() {
			BeforeEach(func() {
				scalingGovernor = governorPerformance
				scalingAvailableGovernors = strings.Join([]string{
					governorConservative,
					governorOndemand,
					governorPerformance,
					governorPowersave,
					governorSchedutil,
					governorUserspace,
				}, " ")
				scalingGovernorOriginal = governorSchedutil
			})

			It("should not change the saved CPU scaling governor", func() {
				verifySetCPUScalingGovernor(governorPerformance, governorPerformance, governorSchedutil, false)
			})
		})

		Context("with unknown governor", func() {
			BeforeEach(func() {
				scalingGovernor = governorSchedutil
				scalingAvailableGovernors = strings.Join([]string{
					governorConservative,
					governorOndemand,
					governorPerformance,
					governorPowersave,
					governorSchedutil,
				}, " ")
				scalingGovernorOriginal = ""
			})

			It("should fail and not change the CPU scaling governor", func() {
				verifySetCPUScalingGovernor(governorUserspace, governorSchedutil, "", true)
			})
		})

		Context("with no scaling governor support", func() {
			BeforeEach(func() {
				scalingGovernor = ""
				scalingAvailableGovernors = ""
				scalingGovernorOriginal = ""
			})

			It("should fail", func() {
				verifySetCPUScalingGovernor(governorPerformance, "", "", true)
			})
		})

		Context("with no available scaling governors", func() {
			BeforeEach(func() {
				scalingGovernor = governorConservative
				scalingAvailableGovernors = ""
				scalingGovernorOriginal = ""
			})

			It("should fail", func() {
				verifySetCPUScalingGovernor(governorPerformance, "", "", true)
			})
		})

		Context("with no configured scaling governor", func() {
			BeforeEach(func() {
				scalingGovernor = ""
				scalingAvailableGovernors = strings.Join([]string{
					governorConservative,
					governorOndemand,
					governorPerformance,
					governorPowersave,
					governorUserspace,
				}, " ")
				scalingGovernorOriginal = ""
			})

			It("should fail", func() {
				verifySetCPUScalingGovernor(governorPerformance, "", "", true)
			})
		})

		Context("with no governor", func() {
			BeforeEach(func() {
				scalingGovernor = governorUserspace
				scalingAvailableGovernors = strings.Join([]string{
					governorConservative,
					governorOndemand,
					governorPerformance,
					governorPowersave,
					governorUserspace,
				}, " ")
				scalingGovernorOriginal = governorOndemand
			})

			It("should restore the original CPU scaling governor", func() {
				verifySetCPUScalingGovernor("", governorOndemand, "", false)
			})
		})

		Context("with no governor and no original governor", func() {
			BeforeEach(func() {
				scalingGovernor = governorPowersave
				scalingAvailableGovernors = strings.Join([]string{
					governorConservative,
					governorOndemand,
					governorPerformance,
					governorPowersave,
					governorSchedutil,
					governorUserspace,
				}, " ")
				scalingGovernorOriginal = ""
			})

			It("should not change the CPU scaling governor", func() {
				verifySetCPUScalingGovernor("", governorPowersave, "", false)
			})
		})
	})

	Describe("restoreIrqBalanceConfig", func() {
		irqSmpAffinityFile := filepath.Join(fixturesDir, "irq_smp_affinity")
		irqBalanceConfigFile := filepath.Join(fixturesDir, "irqbalance")
		irqBannedCPUConfigFile := filepath.Join(fixturesDir, "orig_irq_banned_cpus")
		verifyRestoreIrqBalanceConfig := func(expectedOrigBannedCPUs, expectedBannedCPUs string) {
			err = RestoreIrqBalanceConfig(context.TODO(), irqBalanceConfigFile, irqBannedCPUConfigFile, irqSmpAffinityFile)
			ExpectWithOffset(1, err).ToNot(HaveOccurred())

			content, err := os.ReadFile(irqBannedCPUConfigFile)
			ExpectWithOffset(1, err).ToNot(HaveOccurred())
			ExpectWithOffset(1, strings.Trim(string(content), "\n")).To(Equal(expectedOrigBannedCPUs))

			bannedCPUs, err := retrieveIrqBannedCPUMasks(irqBalanceConfigFile)
			ExpectWithOffset(1, err).ToNot(HaveOccurred())
			ExpectWithOffset(1, bannedCPUs).To(Equal(expectedBannedCPUs))
		}

		JustBeforeEach(func() {
			// create tests affinity file
			err = os.WriteFile(irqSmpAffinityFile, []byte("ffffffff,ffffffff"), 0o644)
			Expect(err).ToNot(HaveOccurred())
			// set irqbalanace config file with banned cpus mask
			err = os.WriteFile(irqBalanceConfigFile, []byte(""), 0o644)
			Expect(err).ToNot(HaveOccurred())
			err = updateIrqBalanceConfigFile(irqBalanceConfigFile, "0000ffff,ffffcfcc")
			Expect(err).ToNot(HaveOccurred())
			bannedCPUs, err := retrieveIrqBannedCPUMasks(irqBalanceConfigFile)
			Expect(err).ToNot(HaveOccurred())
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
				Expect(err).ToNot(HaveOccurred())
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
				Expect(err).ToNot(HaveOccurred())
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
				verifyConvertAnnotationToLatency("disable", latencyNA, false)
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
	Describe("PreCreate Hook", func() {
		shares := uint64(2048)
		g := &generate.Generator{
			Config: &specs.Spec{
				Process: &specs.Process{
					Env: make([]string, 0),
				},
				Linux: &specs.Linux{
					Resources: &specs.LinuxResources{
						CPU: &specs.LinuxCPU{
							Cpus:   "1,2",
							Shares: &shares,
						},
					},
				},
			},
		}
		c, err := oci.NewContainer("containerID", "", "", "",
			make(map[string]string), make(map[string]string),
			make(map[string]string), "pauseImage", nil, nil, "",
			&types.ContainerMetadata{Name: "cnt1"}, "sandboxID", false, false,
			false, "", "", time.Now(), "")
		Expect(err).ToNot(HaveOccurred())

		sbox := sandbox.NewBuilder()
		createdAt := time.Now()
		sbox.SetCreatedAt(createdAt)
		sbox.SetID("sandboxID")
		sbox.SetName("sandboxName")
		sbox.SetLogDir("test")
		sbox.SetShmPath("test")
		sbox.SetNamespace("")
		sbox.SetKubeName("")
		sbox.SetMountLabel("test")
		sbox.SetProcessLabel("test")
		sbox.SetCgroupParent("")
		sbox.SetRuntimeHandler("")
		sbox.SetResolvPath("")
		sbox.SetHostname("")
		sbox.SetPortMappings([]*hostport.PortMapping{})
		sbox.SetHostNetwork(false)
		sbox.SetUsernsMode("")
		sbox.SetPodLinuxOverhead(nil)
		sbox.SetPodLinuxResources(nil)
		err = sbox.SetCRISandbox(sbox.ID(), make(map[string]string), map[string]string{
			crioannotations.CPUSharedAnnotation + "/" + c.CRIContainer().GetMetadata().GetName(): annotationEnable,
		}, &types.PodSandboxMetadata{})
		Expect(err).ToNot(HaveOccurred())
		sbox.SetPrivileged(false)
		sbox.SetHostNetwork(false)
		sbox.SetCreatedAt(createdAt)
		sb, err := sbox.GetSandbox()
		Expect(err).ToNot(HaveOccurred())

		It("should inject env variable only to pod with cpu-shared.crio.io annotation", func() {
			h := HighPerformanceHooks{sharedCPUs: "3,4"}
			err := h.PreCreate(context.TODO(), g, sb, c)
			Expect(err).ToNot(HaveOccurred())
			env := g.Config.Process.Env
			Expect(env).To(ContainElements("OPENSHIFT_ISOLATED_CPUS=1-2", "OPENSHIFT_SHARED_CPUS=3-4"))
		})
	})
})
