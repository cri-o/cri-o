package runtimehandlerhooks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/utils/cpuset"

	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	crioannotations "github.com/cri-o/cri-o/pkg/annotations"
	"github.com/cri-o/cri-o/pkg/config"
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

type mockServiceManager struct {
	isServiceEnabled map[string]bool
	restartService   map[string]error
	history          []string
}

func (m *mockServiceManager) IsServiceEnabled(serviceName string) bool {
	m.history = append(m.history, "systemctl is-enabled "+serviceName)
	if m.isServiceEnabled == nil {
		return false
	}

	return m.isServiceEnabled[serviceName]
}

func (m *mockServiceManager) RestartService(serviceName string) error {
	m.history = append(m.history, "systemctl restart "+serviceName)
	if m.restartService == nil {
		return errors.New("service not found")
	}

	if _, ok := m.restartService[serviceName]; !ok {
		return errors.New("service not found")
	}

	return m.restartService[serviceName]
}

type mockCommandRunner struct {
	lookPath map[string]struct {
		path string
		err  error
	}
	history []string
}

func (m *mockCommandRunner) LookPath(file string) (string, error) {
	m.history = append(m.history, "which "+file)
	if m.lookPath == nil {
		return "", errors.New("path not found")
	}

	if _, ok := m.lookPath[file]; !ok {
		return "", errors.New("path not found")
	}

	return m.lookPath[file].path, m.lookPath[file].err
}

func (m *mockCommandRunner) RunCommand(name string, env []string, arg ...string) error {
	m.history = append(m.history, fmt.Sprintf(
		"%s %s %s",
		strings.Join(env, " "),
		name,
		strings.Join(arg, " "),
	))

	return nil
}

// The actual test suite.
var _ = Describe("high_performance_hooks", func() {
	container, err := oci.NewContainer("containerID", "", "", "",
		make(map[string]string), make(map[string]string),
		make(map[string]string), "pauseImage", nil, nil, "",
		&types.ContainerMetadata{}, "sandboxID", false, false,
		false, "", "", time.Now(), "")
	Expect(err).ToNot(HaveOccurred())

	var flags, bannedCPUFlags string

	baseSandboxBuilder := func() sandbox.Builder {
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
		sbox.SetPrivileged(false)
		sbox.SetHostNetwork(false)
		sbox.SetCreatedAt(createdAt)

		return sbox
	}

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
			h := &HighPerformanceHooks{
				irqBalanceConfigFile: irqBalanceConfigFile,
				irqSMPAffinityFile:   irqSmpAffinityFile,
			}
			err := h.setIRQLoadBalancing(context.TODO(), container, cpuset.CPUSet{}, enabled)
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
			h := &HighPerformanceHooks{
				irqBalanceConfigFile: irqBalanceConfigFile,
				irqSMPAffinityFile:   irqSmpAffinityFile,
			}
			err = h.setIRQLoadBalancing(context.TODO(), container, cpuset.CPUSet{}, enabled)
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

	Describe("setIRQLoadBalancing with housekeeping CPUs", func() {
		irqSmpAffinityFile := filepath.Join(fixturesDir, "irq_smp_affinity")
		irqBalanceConfigFile := filepath.Join(fixturesDir, "irqbalance")
		sysCPUDir := filepath.Join(fixturesDir, "cpus")

		createSysCPUThreadSiblingsDir := func(testCPUDir string, numCPUs, topology int) {
			err := os.MkdirAll(testCPUDir, os.ModePerm)
			Expect(err).ToNot(HaveOccurred())

			Expect(numCPUs%topology).To(Equal(0), "num cpus and topology mismatch")

			// Create CPU directories and topology files for CPUs.
			for cpu := 0; cpu < numCPUs; cpu += topology {
				// Create thread siblings based on hyperthreading simulation.
				// E.g., with topology == 2, CPUs 0,1 are siblings; 2,3 are siblings; 4,5 are siblings; 6,7 are siblings.
				siblings := []int{}
				for sibling := cpu; sibling < cpu+topology; sibling++ {
					siblings = append(siblings, sibling)
				}
				siblingsSet := cpuset.New(siblings...)

				for sibling := cpu; sibling < cpu+topology; sibling++ {
					cpuTopologyDir := filepath.Join(testCPUDir, fmt.Sprintf("cpu%d", sibling), "topology")
					err := os.MkdirAll(cpuTopologyDir, os.ModePerm)
					Expect(err).ToNot(HaveOccurred())
					siblingsFile := filepath.Join(cpuTopologyDir, "thread_siblings_list")
					err = os.WriteFile(siblingsFile, []byte(siblingsSet.String()), 0o644)
					Expect(err).ToNot(HaveOccurred())
				}
			}
		}

		createInvalidSysCPUThreadSiblingsDir := func(testCPUDir string, numCPUs int) {
			err := os.MkdirAll(testCPUDir, os.ModePerm)
			Expect(err).ToNot(HaveOccurred())

			// Create CPU directories and topology files for CPUs.
			for cpu := range numCPUs {
				cpuTopologyDir := filepath.Join(testCPUDir, fmt.Sprintf("cpu%d", cpu), "topology")
				err := os.MkdirAll(cpuTopologyDir, os.ModePerm)
				Expect(err).ToNot(HaveOccurred())
				siblingsFile := filepath.Join(cpuTopologyDir, "thread_siblings_list")
				err = os.WriteFile(siblingsFile, []byte("invalid"), 0o644)
				Expect(err).ToNot(HaveOccurred())
			}
		}

		createContainerSandbox := func(cpus string, annotations map[string]string) (*oci.Container, *sandbox.Sandbox) {
			c, err := oci.NewContainer("containerID", "", "", "",
				make(map[string]string), make(map[string]string),
				make(map[string]string), "pauseImage", nil, nil, "",
				&types.ContainerMetadata{}, "sandboxID", false, false,
				false, "", "", time.Now(), "")
			Expect(err).ToNot(HaveOccurred())
			c.SetSpec(
				&specs.Spec{
					Linux: &specs.Linux{
						Resources: &specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Cpus: cpus,
							},
						},
					},
				},
			)
			sbox := baseSandboxBuilder()
			err = sbox.SetCRISandbox(sbox.ID(), make(map[string]string), annotations, &types.PodSandboxMetadata{})
			Expect(err).ToNot(HaveOccurred())
			sb, err := sbox.GetSandbox()
			Expect(err).ToNot(HaveOccurred())

			return c, sb
		}

		verifySetIRQLoadBalancing := func(c *oci.Container, sb *sandbox.Sandbox, enabled bool,
			expectedSmp, expectedBan, expectedHousekeepingCPUs string, expectFailure bool,
		) {
			h := &HighPerformanceHooks{
				irqBalanceConfigFile: irqBalanceConfigFile,
				irqSMPAffinityFile:   irqSmpAffinityFile,
				sysCPUDir:            sysCPUDir,
			}

			// For container start (enabled == false), we must calculate the housekeeping CPUs,
			// otherwise housekeepingSiblings is the empty set.
			housekeepingSiblings := cpuset.CPUSet{}
			if !enabled {
				spec := c.Spec()
				housekeepingSiblings, err = h.getHousekeepingCPUs(&spec, sb.Annotations())
				if expectFailure {
					Expect(err).To(HaveOccurred())

					return
				} else {
					Expect(err).ToNot(HaveOccurred())
				}
			}

			err = h.setIRQLoadBalancing(context.TODO(), c, housekeepingSiblings, enabled)
			Expect(err).ToNot(HaveOccurred())

			content, err := os.ReadFile(irqSmpAffinityFile)
			Expect(err).ToNot(HaveOccurred())

			Expect(strings.Trim(string(content), "\n")).To(Equal(expectedSmp))

			bannedCPUs, err := retrieveIrqBannedCPUMasks(irqBalanceConfigFile)
			Expect(err).ToNot(HaveOccurred())
			Expect(bannedCPUs).To(Equal(expectedBan))

			// Also test that injection via injectHousekeepingEnv works correctly.
			specgen := generate.NewFromSpec(&specs.Spec{Process: &specs.Process{}})
			if !housekeepingSiblings.IsEmpty() {
				err := injectHousekeepingEnv(&specgen, housekeepingSiblings)
				Expect(err).ToNot(HaveOccurred())
			}
			var expectedHousekeepingAnnotation []string
			if expectedHousekeepingCPUs != "" {
				expectedHousekeepingAnnotation = append(
					expectedHousekeepingAnnotation,
					fmt.Sprintf("%s=%s", HousekeepingCPUsEnvVar, expectedHousekeepingCPUs),
				)
			}
			Expect(specgen.Config.Process.Env).To(Equal(expectedHousekeepingAnnotation))
		}

		JustBeforeEach(func() {
			err = os.WriteFile(irqBalanceConfigFile, []byte(""), 0o644)
			Expect(err).ToNot(HaveOccurred())
			err = updateIrqBalanceConfigFile(irqBalanceConfigFile, bannedCPUFlags)
			Expect(err).ToNot(HaveOccurred())
			bannedCPUs, err := retrieveIrqBannedCPUMasks(irqBalanceConfigFile)
			Expect(err).ToNot(HaveOccurred())
			Expect(bannedCPUs).To(Equal(bannedCPUFlags))
			err = os.WriteFile(irqSmpAffinityFile, []byte(flags), 0o644)
			Expect(err).ToNot(HaveOccurred())
		})

		// This should be covered in other tests already, but test this here for completeness and safe measure.
		Context("with enabled equals to true", func() {
			BeforeEach(func() {
				flags = "00000000,ffffff0f"
				bannedCPUFlags = "ffffffff,000000f0"
			})

			It("should set the irq bit mask with housekeeping CPUs annotation present", func() {
				c, sb := createContainerSandbox("4,5,6,7", map[string]string{
					crioannotations.IRQLoadBalancingAnnotation: annotationHousekeeping,
				})
				verifySetIRQLoadBalancing(c, sb, true, "00000000,ffffffff", "ffffffff,00000000", "", false)
			})
		})

		Context("with enabled equals to false", func() {
			BeforeEach(func() {
				flags = "00000000,ffffffff"
				bannedCPUFlags = "ffffffff,00000000"
			})

			It("should set the irq bit mask without housekeeping CPUs", func() {
				c, sb := createContainerSandbox("4,5,6,7", map[string]string{})
				verifySetIRQLoadBalancing(c, sb, false, "00000000,ffffff0f", "ffffffff,000000f0", "", false)
			})

			It("should set the irq bit mask with housekeeping CPUs when no thread siblings files are present", func() {
				c, sb := createContainerSandbox("4,5,6,7", map[string]string{
					crioannotations.IRQLoadBalancingAnnotation: annotationHousekeeping,
				})
				verifySetIRQLoadBalancing(c, sb, false, "00000000,ffffff1f", "ffffffff,000000e0", "4", false)
			})

			It("should set the irq bit mask with housekeeping CPUs and no siblings", func() {
				createSysCPUThreadSiblingsDir(sysCPUDir, 64, 1)
				c, sb := createContainerSandbox("4,5,6,7", map[string]string{
					crioannotations.IRQLoadBalancingAnnotation: annotationHousekeeping,
				})
				verifySetIRQLoadBalancing(c, sb, false, "00000000,ffffff1f", "ffffffff,000000e0", "4", false)
			})

			It("should set the irq bit mask with housekeeping CPUs and siblings (topology 2)", func() {
				createSysCPUThreadSiblingsDir(sysCPUDir, 64, 2)
				c, sb := createContainerSandbox("4,5,6,7", map[string]string{
					crioannotations.IRQLoadBalancingAnnotation: annotationHousekeeping,
				})
				verifySetIRQLoadBalancing(c, sb, false, "00000000,ffffff3f", "ffffffff,000000c0", "4-5", false)
			})

			It("should set the irq bit mask with housekeeping CPUs and siblings (topology 4)", func() {
				createSysCPUThreadSiblingsDir(sysCPUDir, 64, 4)
				c, sb := createContainerSandbox("4-11", map[string]string{
					crioannotations.IRQLoadBalancingAnnotation: annotationHousekeeping,
				})
				verifySetIRQLoadBalancing(c, sb, false, "00000000,fffff0ff", "ffffffff,00000f00", "4-7", false)
			})

			It("should fail with invalid siblings files", func() {
				createInvalidSysCPUThreadSiblingsDir(sysCPUDir, 64)
				c, sb := createContainerSandbox("4-11", map[string]string{
					crioannotations.IRQLoadBalancingAnnotation: annotationHousekeeping,
				})
				verifySetIRQLoadBalancing(c, sb, false, "00000000,fffff0ff", "ffffffff,00000f00", "4-7", true)
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
		var mockSvcMgr *mockServiceManager

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

			mockSvcMgr = &mockServiceManager{
				isServiceEnabled: map[string]bool{
					"irqbalance": true,
				},
				restartService: map[string]error{
					"irqbalance": nil,
				},
				history: []string{},
			}
			serviceManager = mockSvcMgr
		})

		JustAfterEach(func() {
			serviceManager = &defaultServiceManager{}
		})

		Context("when banned cpu config file doesn't exist", func() {
			BeforeEach(func() {
				// ensure banned cpu config file doesn't exist
				os.Remove(irqBannedCPUConfigFile)
			})

			It("should set banned cpu config file from irq balance config", func() {
				verifyRestoreIrqBalanceConfig("0000ffff,ffffcfcc", "0000ffff,ffffcfcc")
				Expect(mockSvcMgr.history).NotTo(ContainElement("systemctl is-enabled irqbalance"))
				Expect(mockSvcMgr.history).NotTo(ContainElement("systemctl restart irqbalance"))
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
				Expect(mockSvcMgr.history).To(ContainElement("systemctl is-enabled irqbalance"))
				Expect(mockSvcMgr.history).To(ContainElement("systemctl restart irqbalance"))
			})
		})
	})

	Describe("handleIRQBalanceRestart", func() {
		irqBalanceConfigFile := filepath.Join(fixturesDir, "irqbalance")

		h := &HighPerformanceHooks{
			irqBalanceConfigFile: irqBalanceConfigFile,
		}

		type parameters struct {
			isServiceEnabled         bool
			irqBalanceFileExists     bool
			restartServiceSucceeds   bool
			pathLookupError          bool
			calculatedIRQBalanceMask string
		}

		DescribeTable("handleIRQBalanceRestart scenarios",
			func(p parameters, serviceMgrHistory, cmdRunnerHistory []string) {
				defer func() {
					// Reset global mocks.
					serviceManager = &defaultServiceManager{}
					commandRunner = &defaultCommandRunner{}
				}()

				// Setup mocks according to parameters and irqbalance config file.
				mockSvcMgr := &mockServiceManager{
					isServiceEnabled: map[string]bool{
						"irqbalance": p.isServiceEnabled,
					},
					history: []string{},
				}
				mockCmdRunner := &mockCommandRunner{
					history: []string{},
				}

				if p.restartServiceSucceeds {
					mockSvcMgr.restartService = map[string]error{
						"irqbalance": nil,
					}
				} else {
					mockSvcMgr.restartService = map[string]error{
						"irqbalance": errors.New("restart failed"),
					}
				}

				if p.pathLookupError {
					mockCmdRunner.lookPath = map[string]struct {
						path string
						err  error
					}{
						"irqbalance": {path: "", err: errors.New("not found")},
					}
				} else {
					mockCmdRunner.lookPath = map[string]struct {
						path string
						err  error
					}{
						"irqbalance": {path: "/usr/bin/irqbalance", err: nil},
					}
				}
				if p.irqBalanceFileExists {
					err = os.WriteFile(irqBalanceConfigFile, []byte(""), 0o644)
					Expect(err).ToNot(HaveOccurred())
					err = updateIrqBalanceConfigFile(irqBalanceConfigFile, p.calculatedIRQBalanceMask)
					Expect(err).ToNot(HaveOccurred())
				}
				serviceManager = mockSvcMgr
				commandRunner = mockCmdRunner

				// Execute application logic.
				if !h.handleIRQBalanceRestart(context.TODO(), "container-name") {
					h.handleIRQBalanceOneShot(context.TODO(), "container-name", p.calculatedIRQBalanceMask)
				}

				// Verify behavior based on scenario.
				Expect(mockSvcMgr.history).To(Equal(serviceMgrHistory))
				Expect(mockCmdRunner.history).To(Equal(cmdRunnerHistory))
			},
			Entry("irqbalance is enabled and succeeds",
				parameters{isServiceEnabled: true, irqBalanceFileExists: true, restartServiceSucceeds: true, pathLookupError: false, calculatedIRQBalanceMask: "ffff,ffff"},
				[]string{
					"systemctl is-enabled irqbalance",
					"systemctl restart irqbalance",
				},
				[]string{}),
			Entry("irqbalance is enabled but irqbalance file does not exist",
				parameters{isServiceEnabled: true, irqBalanceFileExists: false, restartServiceSucceeds: false, pathLookupError: false, calculatedIRQBalanceMask: "ffff,ffff"},
				[]string{
					"systemctl is-enabled irqbalance",
				},
				[]string{
					"which irqbalance",
					"IRQBALANCE_BANNED_CPUS=ffff,ffff /usr/bin/irqbalance --oneshot",
				}),
			Entry("irqbalance is enabled and fails but oneshot works",
				parameters{isServiceEnabled: true, irqBalanceFileExists: true, restartServiceSucceeds: false, pathLookupError: false, calculatedIRQBalanceMask: "ffff,ffff"},
				[]string{
					"systemctl is-enabled irqbalance",
					"systemctl restart irqbalance",
				},
				[]string{
					"which irqbalance",
					"IRQBALANCE_BANNED_CPUS=ffff,ffff /usr/bin/irqbalance --oneshot",
				}),
			Entry("irqbalance is disabled but irqBalance file exists",
				parameters{isServiceEnabled: false, irqBalanceFileExists: true, restartServiceSucceeds: false, pathLookupError: false, calculatedIRQBalanceMask: "ffff,ffff"},
				[]string{
					"systemctl is-enabled irqbalance",
				},
				[]string{
					"which irqbalance",
					"IRQBALANCE_BANNED_CPUS=ffff,ffff /usr/bin/irqbalance --oneshot",
				}),
			Entry("irqbalance is disabled, oneshot lookup fails",
				parameters{isServiceEnabled: false, irqBalanceFileExists: true, restartServiceSucceeds: false, pathLookupError: true, calculatedIRQBalanceMask: "ffff,ffff"},
				[]string{
					"systemctl is-enabled irqbalance",
				},
				[]string{
					"which irqbalance",
				}),
		)
	})

	Describe("updateNewIRQSMPAffinityMask rollback", func() {
		irqBalanceConfigFile := filepath.Join(fixturesDir, "irqbalance")
		irqSMPAffinityFile := filepath.Join(fixturesDir, "irqsmpaffinity")

		h := &HighPerformanceHooks{
			irqSMPAffinityFile:   irqSMPAffinityFile,
			irqBalanceConfigFile: irqBalanceConfigFile,
		}

		type parameters struct {
			irqBalanceFileRO           bool
			originalIRQSMPAffinityMask string
			expectedIRQSMPAffinityMask string
		}

		DescribeTable("test rollback",
			func(p parameters) {
				err := os.WriteFile(irqSMPAffinityFile, []byte(p.originalIRQSMPAffinityMask), 0o644)
				Expect(err).ToNot(HaveOccurred())

				if p.irqBalanceFileRO {
					err = os.Symlink("/proc/version", irqBalanceConfigFile)
					Expect(err).ToNot(HaveOccurred())
				} else {
					err = os.WriteFile(irqBalanceConfigFile, []byte(""), 0o644)
					Expect(err).ToNot(HaveOccurred())
				}

				_, err = h.updateNewIRQSMPAffinityMask(context.TODO(), "cID", "CName", cpuSetOrDie("2-3"), false)
				if p.irqBalanceFileRO {
					Expect(err).To(HaveOccurred())
				} else {
					Expect(err).NotTo(HaveOccurred())
				}

				writtenMask, err := os.ReadFile(irqSMPAffinityFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(writtenMask)).To(Equal(p.expectedIRQSMPAffinityMask))
			},
			Entry("writing IRQ balance file fails",
				parameters{
					irqBalanceFileRO:           true,
					originalIRQSMPAffinityMask: "ffffffff",
					expectedIRQSMPAffinityMask: "ffffffff",
				}),
			Entry("writing IRQ balance file succeeds",
				parameters{
					irqBalanceFileRO:           false,
					originalIRQSMPAffinityMask: "ffffffff",
					expectedIRQSMPAffinityMask: "fffffff3",
				}),
		)
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
		baseGenerator := func() *generate.Generator {
			g := generate.NewFromSpec(
				&specs.Spec{
					Process: &specs.Process{
						Env: make([]string, 0),
					},
					Linux: &specs.Linux{
						Resources: &specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: &shares,
							},
						},
					},
				},
			)

			return &g
		}

		buildContainer := func(g *generate.Generator) (*oci.Container, error) {
			c, err := oci.NewContainer("containerID", "", "", "",
				make(map[string]string), make(map[string]string),
				make(map[string]string), "pauseImage", nil, nil, "",
				&types.ContainerMetadata{Name: "cnt1"}, "sandboxID", false, false,
				false, "", "", time.Now(), "")
			if err != nil {
				return nil, err
			}
			c.SetSpec(g.Config)

			return c, nil
		}

		var (
			sbSharedAnnotation   *sandbox.Sandbox
			sbNoSharedAnnotation *sandbox.Sandbox
			genExclusiveCPUs     *generate.Generator
			genNoExclusiveCPUs   *generate.Generator
		)

		BeforeEach(func() {
			// initialize generator
			genNoExclusiveCPUs = baseGenerator()

			genExclusiveCPUs = baseGenerator()
			genExclusiveCPUs.Config.Linux.Resources.CPU.Cpus = "1-2"

			// initialize sandbox
			sbox := baseSandboxBuilder()
			err = sbox.SetCRISandbox(sbox.ID(), make(map[string]string), map[string]string{}, &types.PodSandboxMetadata{})
			sbNoSharedAnnotation, err = sbox.GetSandbox()
			Expect(err).ToNot(HaveOccurred())

			sbox = baseSandboxBuilder()
			err = sbox.SetCRISandbox(sbox.ID(), make(map[string]string), map[string]string{
				crioannotations.CPUSharedAnnotation + "/cnt1": annotationEnable,
			}, &types.PodSandboxMetadata{})
			Expect(err).ToNot(HaveOccurred())
			sbSharedAnnotation, err = sbox.GetSandbox()
			Expect(err).ToNot(HaveOccurred())
		})

		var (
			g  *generate.Generator
			c  *oci.Container
			sb *sandbox.Sandbox
		)
		Context("sharedCPUs && FirstExecCPUAffinity", func() {
			h := HighPerformanceHooks{execCPUAffinity: config.ExecCPUAffinityTypeFirst, sharedCPUs: "3,4"}
			Context("with exclusive & shared CPUs", func() {
				BeforeEach(func() {
					g = genExclusiveCPUs
					sb = sbSharedAnnotation
					c, err = buildContainer(g)
					Expect(err).ToNot(HaveOccurred())
				})

				It("should inject env variable only to pod with cpu-shared.crio.io annotation", func() {
					err = h.PreCreate(context.TODO(), g, sb, c)
					Expect(err).ToNot(HaveOccurred())
					env := g.Config.Process.Env
					Expect(env).To(ContainElements("OPENSHIFT_ISOLATED_CPUS=1-2", "OPENSHIFT_SHARED_CPUS=3-4"))
				})

				It("should choose the first CPU in shared CPUs", func() {
					err := h.PreCreate(context.TODO(), g, sb, c)
					Expect(err).ToNot(HaveOccurred())
					Expect(g.Config.Process.ExecCPUAffinity.Initial).To(Equal("3"))
				})
			})

			Context("with exclusive & !shared CPUs", func() {
				BeforeEach(func() {
					g = genExclusiveCPUs
					sb = sbNoSharedAnnotation
					c, err = buildContainer(g)
					Expect(err).ToNot(HaveOccurred())
				})

				It("should choose the first CPU in exclusive CPUs", func() {
					err := h.PreCreate(context.TODO(), g, sb, c)
					Expect(err).ToNot(HaveOccurred())
					Expect(g.Config.Process.ExecCPUAffinity.Initial).To(Equal("1"))
				})
			})

			Context("with !exclusive & shared CPUs", func() {
				BeforeEach(func() {
					g = genNoExclusiveCPUs
					sb = sbSharedAnnotation
					c, err = buildContainer(g)
					Expect(err).ToNot(HaveOccurred())
				})

				It("should get an error", func() {
					err := h.PreCreate(context.TODO(), g, sb, c)
					Expect(err).To(HaveOccurred())
				})
			})

			Context("with !exclusive & !shared CPUs", func() {
				BeforeEach(func() {
					g = genNoExclusiveCPUs
					sb = sbNoSharedAnnotation
					c, err = buildContainer(g)
					Expect(err).ToNot(HaveOccurred())
				})

				It("should not use ExecCPUAffinity", func() {
					err := h.PreCreate(context.TODO(), g, sb, c)
					Expect(err).ToNot(HaveOccurred())
					Expect(g.Config.Process.ExecCPUAffinity).To(BeNil())
				})
			})
		})

		Context("No shared CPUs and FirstExecCPUAffinity", func() {
			h := HighPerformanceHooks{execCPUAffinity: config.ExecCPUAffinityTypeFirst}
			Context("with shared CPUs", func() {
				BeforeEach(func() {
					g = genExclusiveCPUs
					sb = sbSharedAnnotation
					c, err = buildContainer(g)
					Expect(err).ToNot(HaveOccurred())
				})

				It("should get an error", func() {
					err := h.PreCreate(context.TODO(), g, sb, c)
					Expect(err).To(HaveOccurred())
				})
			})

			Context("with exclusive CPUs", func() {
				BeforeEach(func() {
					g = genExclusiveCPUs
					sb = sbNoSharedAnnotation
					c, err = buildContainer(g)
					Expect(err).ToNot(HaveOccurred())
				})

				It("should choose the first CPU in exclusive CPUs", func() {
					err := h.PreCreate(context.TODO(), g, sb, c)
					Expect(err).ToNot(HaveOccurred())
					Expect(g.Config.Process.ExecCPUAffinity.Initial).To(Equal("1"))
				})
			})
		})

		Context("DefaultExecCPUAffinity", func() {
			h := HighPerformanceHooks{execCPUAffinity: config.ExecCPUAffinityTypeDefault, sharedCPUs: "3,4"}
			BeforeEach(func() {
				g = genExclusiveCPUs
				sb = sbSharedAnnotation
				c, err = buildContainer(g)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should not use ExecCPUAffinity", func() {
				err := h.PreCreate(context.TODO(), g, sb, c)
				Expect(err).ToNot(HaveOccurred())
				Expect(g.Config.Process.ExecCPUAffinity).To(BeNil())
			})
		})
	})
	Describe("Make sure that correct runtime handler hooks are set", func() {
		var runtimeName string
		var sandboxAnnotations map[string]string
		var sb *sandbox.Sandbox
		var cfg *config.Config
		var hooksRetriever *HooksRetriever

		formatIRQBalanceBannedCPUs := func(v string) string {
			return fmt.Sprintf("%s=%q", irqBalanceBannedCpus, v)
		}

		irqSmpAffinityFile := filepath.Join(fixturesDir, "irq_smp_affinity")
		irqBalanceConfigFile := filepath.Join(fixturesDir, "irqbalance")
		flags := "0000,0000ffff"
		bannedCPUFlags = "ffffffff,ffff0000"

		ctx := context.Background()

		verifySetIRQLoadBalancing := func(expectedIrqSmp, expectedIrqBalance string) {
			content, err := os.ReadFile(irqSmpAffinityFile)
			Expect(err).ToNot(HaveOccurred())
			Expect(strings.Trim(string(content), "\n")).To(Equal(expectedIrqSmp))

			content, err = os.ReadFile(irqBalanceConfigFile)
			Expect(err).ToNot(HaveOccurred())
			Expect(strings.Trim(string(content), "\n")).To(Equal(formatIRQBalanceBannedCPUs(expectedIrqBalance)))
		}

		createContainer := func(cpus string) (*oci.Container, error) {
			container, err := oci.NewContainer("containerID", "", "", "",
				make(map[string]string), make(map[string]string),
				make(map[string]string), "pauseImage", nil, nil, "",
				&types.ContainerMetadata{}, "sandboxID", false, false,
				false, "", "", time.Now(), "")
			if err != nil {
				return nil, err
			}
			var cpuShares uint64 = 1024
			container.SetSpec(
				&specs.Spec{
					Linux: &specs.Linux{
						Resources: &specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Cpus:   cpus,
								Shares: &cpuShares,
							},
						},
					},
				},
			)

			return container, nil
		}

		JustBeforeEach(func() {
			// Simulate a restart of crio each time as we're modifying the config between runs.
			cpuLoadBalancingAllowedAnywhereOnce = sync.Once{}

			hooksRetriever = NewHooksRetriever(ctx, cfg)

			// create tests affinity file
			err = os.WriteFile(irqSmpAffinityFile, []byte(flags), 0o644)
			Expect(err).ToNot(HaveOccurred())
			err = os.WriteFile(irqBalanceConfigFile, []byte(formatIRQBalanceBannedCPUs(bannedCPUFlags)), 0o644)
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
			sbox.SetRuntimeHandler(runtimeName)
			sbox.SetResolvPath("")
			sbox.SetHostname("")
			sbox.SetPortMappings([]*hostport.PortMapping{})
			sbox.SetHostNetwork(false)
			sbox.SetUsernsMode("")
			sbox.SetPodLinuxOverhead(nil)
			sbox.SetPodLinuxResources(nil)
			err = sbox.SetCRISandbox(
				sbox.ID(),
				map[string]string{},
				sandboxAnnotations,
				&types.PodSandboxMetadata{},
			)
			Expect(err).ToNot(HaveOccurred())
			sbox.SetPrivileged(false)
			sbox.SetHostNetwork(false)
			sbox.SetCreatedAt(createdAt)
			sb, err = sbox.GetSandbox()
			Expect(err).ToNot(HaveOccurred())
		})

		Context("with runtime name high-performance and sandbox disable annotation", func() {
			BeforeEach(func() {
				runtimeName = "high-performance"
				sandboxAnnotations = map[string]string{crioannotations.IRQLoadBalancingAnnotation: "disable"}
				cfg = &config.Config{
					RuntimeConfig: config.RuntimeConfig{
						IrqBalanceConfigFile: irqBalanceConfigFile,
						Runtimes: config.Runtimes{
							"high-performance": {
								AllowedAnnotations: []string{},
							},
							"default": {},
						},
					},
				}
			})

			It("should set the correct irq bit mask with concurrency", func(ctx context.Context) {
				hooks := hooksRetriever.Get(ctx, sb.RuntimeHandler(), sb.Annotations())
				Expect(hooks).NotTo(BeNil())
				if hph, ok := hooks.(*HighPerformanceHooks); ok {
					hph.irqSMPAffinityFile = irqSmpAffinityFile
					hph.irqBalanceConfigFile = irqBalanceConfigFile
				}
				var wg sync.WaitGroup
				for cpu := range 16 {
					wg.Add(1)
					go func() {
						defer wg.Done()
						container, err := createContainer(strconv.Itoa(cpu))
						Expect(err).ToNot(HaveOccurred())
						err = hooks.PreStart(ctx, container, sb)
						Expect(err).ToNot(HaveOccurred())
					}()
				}
				wg.Wait()
				verifySetIRQLoadBalancing("00000000,00000000", "ffffffff,ffffffff")
			})
		})

		Context("with runtime name high-performance and sandbox without any annotation", func() {
			BeforeEach(func() {
				runtimeName = "high-performance"
				sandboxAnnotations = map[string]string{}
				cfg = &config.Config{
					RuntimeConfig: config.RuntimeConfig{
						IrqBalanceConfigFile: irqBalanceConfigFile,
						Runtimes: config.Runtimes{
							"high-performance": {
								AllowedAnnotations: []string{},
							},
							"default": {},
						},
					},
				}
			})

			It("should keep the current irq bit mask but return a high performance hooks", func(ctx context.Context) {
				hooks := hooksRetriever.Get(ctx, sb.RuntimeHandler(), sb.Annotations())
				Expect(hooks).NotTo(BeNil())
				hph, ok := hooks.(*HighPerformanceHooks)
				Expect(ok).To(BeTrue())
				hph.irqSMPAffinityFile = irqSmpAffinityFile
				hph.irqBalanceConfigFile = irqBalanceConfigFile

				var wg sync.WaitGroup
				for cpu := range 16 {
					wg.Add(1)
					go func() {
						defer wg.Done()
						container, err := createContainer(strconv.Itoa(cpu))
						Expect(err).ToNot(HaveOccurred())
						err = hooks.PreStart(ctx, container, sb)
						Expect(err).ToNot(HaveOccurred())
					}()
				}
				wg.Wait()
				verifySetIRQLoadBalancing(flags, bannedCPUFlags)
			})
		})

		Context("with runtime name hp and sandbox disable annotation", func() {
			BeforeEach(func() {
				runtimeName = "hp"
				sandboxAnnotations = map[string]string{crioannotations.IRQLoadBalancingAnnotation: "disable"}
				cfg = &config.Config{
					RuntimeConfig: config.RuntimeConfig{
						IrqBalanceConfigFile: irqBalanceConfigFile,
						Runtimes: config.Runtimes{
							"hp": {
								AllowedAnnotations: []string{
									crioannotations.IRQLoadBalancingAnnotation,
								},
							},
							"default": {},
						},
					},
				}
			})

			It("should set the correct irq bit mask with concurrency", func(ctx context.Context) {
				hooks := hooksRetriever.Get(ctx, sb.RuntimeHandler(), sb.Annotations())
				Expect(hooks).NotTo(BeNil())
				if hph, ok := hooks.(*HighPerformanceHooks); ok {
					hph.irqSMPAffinityFile = irqSmpAffinityFile
					hph.irqBalanceConfigFile = irqBalanceConfigFile
				}
				var wg sync.WaitGroup
				for cpu := range 16 {
					wg.Add(1)
					go func() {
						defer wg.Done()
						container, err := createContainer(strconv.Itoa(cpu))
						Expect(err).ToNot(HaveOccurred())
						err = hooks.PreStart(ctx, container, sb)
						Expect(err).ToNot(HaveOccurred())
					}()
				}
				wg.Wait()
				verifySetIRQLoadBalancing("00000000,00000000", "ffffffff,ffffffff")
			})
		})

		Context("with runtime name hp and sandbox without any annotation", func() {
			BeforeEach(func() {
				runtimeName = "hp"
				sandboxAnnotations = map[string]string{}
				cfg = &config.Config{
					RuntimeConfig: config.RuntimeConfig{
						IrqBalanceConfigFile: irqBalanceConfigFile,
						Runtimes: config.Runtimes{
							"hp": {
								AllowedAnnotations: []string{
									crioannotations.IRQLoadBalancingAnnotation,
								},
							},
							"default": {},
						},
					},
				}
			})

			It("should return a nil hook", func(ctx context.Context) {
				hooks := hooksRetriever.Get(ctx, sb.RuntimeHandler(), sb.Annotations())
				Expect(hooks).To(BeNil())
			})
		})

		// The following test case should never happen in the real world. However, it makes sure that the checks
		// actually look at the runtime name and at the sandbox annotation and if _either_ signals that high performance
		// hooks should be enabled then enable them.
		Context("with runtime name default and sandbox disable annotation", func() {
			BeforeEach(func() {
				runtimeName = "default"
				sandboxAnnotations = map[string]string{crioannotations.IRQLoadBalancingAnnotation: "disable"}
				cfg = &config.Config{
					RuntimeConfig: config.RuntimeConfig{
						IrqBalanceConfigFile: irqBalanceConfigFile,
						Runtimes: config.Runtimes{
							"default": {},
						},
					},
				}
			})

			It("should set the correct irq bit mask with concurrency", func(ctx context.Context) {
				hooks := hooksRetriever.Get(ctx, sb.RuntimeHandler(), sb.Annotations())
				Expect(hooks).NotTo(BeNil())
				if hph, ok := hooks.(*HighPerformanceHooks); ok {
					hph.irqSMPAffinityFile = irqSmpAffinityFile
					hph.irqBalanceConfigFile = irqBalanceConfigFile
				}
				var wg sync.WaitGroup
				for cpu := range 16 {
					wg.Add(1)
					go func() {
						defer wg.Done()
						container, err := createContainer(strconv.Itoa(cpu))
						Expect(err).ToNot(HaveOccurred())
						err = hooks.PreStart(ctx, container, sb)
						Expect(err).ToNot(HaveOccurred())
					}()
				}
				wg.Wait()
				verifySetIRQLoadBalancing("00000000,00000000", "ffffffff,ffffffff")
			})
		})

		Context("with runtime name default, CPU balancing annotation present and sandbox without any annotation", func() {
			BeforeEach(func() {
				runtimeName = "default"
				sandboxAnnotations = map[string]string{}
				cfg = &config.Config{
					RuntimeConfig: config.RuntimeConfig{
						IrqBalanceConfigFile: irqBalanceConfigFile,
						Runtimes: config.Runtimes{
							"high-performance": {
								AllowedAnnotations: []string{},
							},
							"hp": {
								AllowedAnnotations: []string{
									crioannotations.IRQLoadBalancingAnnotation,
								},
							},
							"cpu-balancing-anywhere": {
								AllowedAnnotations: []string{
									crioannotations.CPULoadBalancingAnnotation,
								},
							},
							"default": {},
						},
					},
				}
			})

			It("should yield a DefaultCPULoadBalanceHooks which keeps the old mask", func(ctx context.Context) {
				hooks := hooksRetriever.Get(ctx, sb.RuntimeHandler(), sb.Annotations())
				Expect(hooks).NotTo(BeNil())
				_, ok := (hooks).(*DefaultCPULoadBalanceHooks)
				Expect(ok).To(BeTrue())
				var wg sync.WaitGroup
				for cpu := range 16 {
					wg.Add(1)
					go func() {
						defer wg.Done()
						container, err := createContainer(strconv.Itoa(cpu))
						Expect(err).ToNot(HaveOccurred())
						err = hooks.PreStart(ctx, container, sb)
						Expect(err).ToNot(HaveOccurred())
					}()
				}
				wg.Wait()
				verifySetIRQLoadBalancing(flags, bannedCPUFlags)
			})
		})
	})
})
