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
	"github.com/sirupsen/logrus"
	"go.uber.org/mock/gomock"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/memorystore"
	"github.com/cri-o/cri-o/internal/oci"
	crioannotations "github.com/cri-o/cri-o/pkg/annotations"
	"github.com/cri-o/cri-o/pkg/config"
	containerstoragemock "github.com/cri-o/cri-o/test/mocks/containerstorage"
	libmock "github.com/cri-o/cri-o/test/mocks/lib"
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

	containerID = "containerID"
	sandboxID   = "sandboxID"
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
	logrus.SetLevel(logrus.DebugLevel)
	var flags, bannedCPUFlags string
	var err error

	container := &oci.Container{}
	containerServer := &lib.ContainerServer{}
	libcfg := &config.Config{}

	baseSandboxBuilder := func() sandbox.Builder {
		sbox := sandbox.NewBuilder()
		createdAt := time.Now()
		sbox.SetCreatedAt(createdAt)
		sbox.SetID(sandboxID)
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

	setupContainerAndSandbox := func() {
		// Create and add sandbox with high-performance runtime handler
		sbox := baseSandboxBuilder()
		sbox.SetRuntimeHandler("high-performance")
		sbox.SetContainers(memorystore.New[*oci.Container]())
		sandboxDisableAnnotations := map[string]string{crioannotations.IRQLoadBalancingAnnotation: "disable"}
		err = sbox.SetCRISandbox(
			sbox.ID(),
			make(map[string]string),
			sandboxDisableAnnotations,
			&types.PodSandboxMetadata{},
		)
		Expect(err).ToNot(HaveOccurred())
		sb, err := sbox.GetSandbox()
		Expect(err).ToNot(HaveOccurred())
		containerServer.AddSandbox(context.TODO(), sb) //nolint:errcheck

		// Create container with CPU specification
		container, err = oci.NewContainer(containerID, containerID, "", "",
			make(map[string]string), make(map[string]string),
			make(map[string]string), "pauseImage", nil, nil, "",
			&types.ContainerMetadata{}, sandboxID, false, false,
			false, "", "", time.Now(), "")
		Expect(err).ToNot(HaveOccurred())

		// Set container spec and state
		var cpuShares uint64 = 1024
		container.SetSpec(
			&specs.Spec{
				Linux: &specs.Linux{
					Resources: &specs.LinuxResources{
						CPU: &specs.LinuxCPU{
							Cpus:   "4,5",
							Shares: &cpuShares,
						},
					},
				},
			},
		)

		// Add container to server
		containerServer.AddContainer(context.TODO(), container)
	}

	BeforeEach(func() {
		mockCtrl := gomock.NewController(GinkgoT())
		libMock := libmock.NewMockIface(mockCtrl)
		storeMock := containerstoragemock.NewMockStore(mockCtrl)
		gomock.InOrder(
			libMock.EXPECT().GetStore().Return(storeMock, nil),
			libMock.EXPECT().GetData().Return(libcfg),
		)
		containerServer, err = lib.New(context.Background(), libMock)
		Expect(err).ToNot(HaveOccurred())
		Expect(containerServer).NotTo(BeNil())

		err = os.MkdirAll(fixturesDir, os.ModePerm)
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
			err := h.setIRQLoadBalancing(context.TODO(), containerServer, container, enabled)
			Expect(err).ToNot(HaveOccurred())

			content, err := os.ReadFile(irqSmpAffinityFile)
			Expect(err).ToNot(HaveOccurred())

			Expect(strings.Trim(string(content), "\n")).To(Equal(expected))
		}

		JustBeforeEach(func() {
			setupContainerAndSandbox()

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
			err := h.setIRQLoadBalancing(context.TODO(), containerServer, container, enabled)
			Expect(err).ToNot(HaveOccurred())

			content, err := os.ReadFile(irqSmpAffinityFile)
			Expect(err).ToNot(HaveOccurred())

			Expect(strings.Trim(string(content), "\n")).To(Equal(expectedSmp))

			bannedCPUs, err := retrieveIrqBannedCPUMasks(irqBalanceConfigFile)
			Expect(err).ToNot(HaveOccurred())

			Expect(bannedCPUs).To(Equal(expectedBan))
		}

		JustBeforeEach(func() {
			setupContainerAndSandbox()

			// set irqbalanace config file with no banned cpus
			err = os.WriteFile(irqBalanceConfigFile, []byte(""), 0o644)
			Expect(err).ToNot(HaveOccurred())
			err = updateIrqBalanceConfigFile(irqBalanceConfigFile, bannedCPUFlags)
			Expect(err).ToNot(HaveOccurred())
			bannedCPUs, err := retrieveIrqBannedCPUMasks(irqBalanceConfigFile)
			Expect(err).ToNot(HaveOccurred())
			Expect(bannedCPUs).To(Equal(bannedCPUFlags))

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

		DescribeTable("handleIRQBalanceRestart scenarios",
			func(description string, isServiceEnabled bool, restartServiceSucceeds bool, pathLookupError bool, irqBalanceContent string) {
				defer func() {
					// Reset global mocks
					serviceManager = &defaultServiceManager{}
					commandRunner = &defaultCommandRunner{}
				}()

				// Setup mocks
				mockSvcMgr := &mockServiceManager{
					isServiceEnabled: map[string]bool{
						"irqbalance": isServiceEnabled,
					},
					history: []string{},
				}

				if isServiceEnabled {
					if restartServiceSucceeds {
						mockSvcMgr.restartService = map[string]error{
							"irqbalance": nil,
						}
					} else {
						mockSvcMgr.restartService = map[string]error{
							"irqbalance": errors.New("restart failed"),
						}
					}
				}

				mockCmdRunner := &mockCommandRunner{
					history: []string{},
				}

				if !isServiceEnabled {
					if pathLookupError {
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
				}

				// Setup config file
				if irqBalanceContent == "none" {
					err = os.WriteFile(irqBalanceConfigFile, []byte(""), 0o644)
				} else {
					err = os.WriteFile(irqBalanceConfigFile, []byte(irqBalanceContent), 0o644)
					Expect(err).ToNot(HaveOccurred())
					err = updateIrqBalanceConfigFile(irqBalanceConfigFile, irqBalanceContent)
					Expect(err).ToNot(HaveOccurred())
				}

				// Set global mocks
				serviceManager = mockSvcMgr
				commandRunner = mockCmdRunner

				// Execute
				h.handleIRQBalanceRestart(context.TODO())

				// Verify behavior based on scenario
				if isServiceEnabled {
					Expect(mockSvcMgr.history).To(ContainElement("systemctl is-enabled irqbalance"))
					Expect(mockSvcMgr.history).To(ContainElement("systemctl restart irqbalance"))
					Expect(mockCmdRunner.history).To(BeEmpty())

					return
				}

				// Irqbalance service is disabled.
				Expect(mockSvcMgr.history).To(ContainElement("systemctl is-enabled irqbalance"))
				Expect(mockCmdRunner.history).To(ContainElement("which irqbalance"))
				if pathLookupError || irqBalanceContent == "none" {
					Expect(mockCmdRunner.history).To(HaveLen(1))

					return
				}
				// Path of irqbalance can be found and content has IRQBALANCE_BANNED_CPUS.
				Expect(mockCmdRunner.history).ToNot(BeEmpty())
				Expect(mockCmdRunner.history).To(ContainElement(fmt.Sprintf("IRQBALANCE_BANNED_CPUS=%s /usr/bin/irqbalance --oneshot", irqBalanceContent)))
			},
			Entry("irqbalance is enabled and succeeds", "irqbalance is enabled and succeeds", true, true, false, ""),
			Entry("irqbalance is enabled and fails", "irqbalance is enabled and fails", true, false, false, ""),
			Entry("irqbalance is disabled, oneshot lookup fails", "irqbalance is disabled, oneshot lookup fails", false, false, true, ""),
			Entry("irqbalance is disabled, empty banned cpus", "irqbalance is disabled, command fails", false, false, false, ""),
			Entry("irqbalance is disabled, with banned cpus", "irqbalance is disabled, command succeeds", false, false, false, "ffff,ffff"),
			Entry("irqbalance is disabled, missing banned cpus", "irqbalance is disabled, command succeeds", false, false, false, "none"),
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
			return &generate.Generator{
				Config: &specs.Spec{
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
			}
		}

		buildContainer := func(g *generate.Generator) (*oci.Container, error) {
			c, err := oci.NewContainer(containerID, containerID, "", "",
				make(map[string]string), make(map[string]string),
				make(map[string]string), "pauseImage", nil, nil, "",
				&types.ContainerMetadata{Name: "cnt1"}, sandboxID, false, false,
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
		var cfg *config.Config
		var hooksRetriever *HooksRetriever

		formatIRQBalanceBannedCPUs := func(v string) string {
			return fmt.Sprintf("%s=%q", irqBalanceBannedCpus, v)
		}

		irqSmpAffinityFile := filepath.Join(fixturesDir, "irq_smp_affinity")
		irqBalanceConfigFile := filepath.Join(fixturesDir, "irqbalance")

		ctx := context.Background()

		verifySetIRQLoadBalancing := func(expectedIrqSmp, expectedIrqBalance string) {
			content, err := os.ReadFile(irqSmpAffinityFile)
			Expect(err).ToNot(HaveOccurred())
			Expect(strings.Trim(string(content), "\n")).To(Equal(expectedIrqSmp))

			content, err = os.ReadFile(irqBalanceConfigFile)
			Expect(err).ToNot(HaveOccurred())
			Expect(strings.Trim(string(content), "\n")).To(Equal(formatIRQBalanceBannedCPUs(expectedIrqBalance)))
		}

		createContainer := func(containerServer *lib.ContainerServer, id, cpus, runtimeName string,
			sandboxAnnotations map[string]string,
		) (*oci.Container, *sandbox.Sandbox, error) {
			fullContainerID := containerID + id
			fullSandboxID := sandboxID + id

			sbox := baseSandboxBuilder()
			sbox.SetID(fullSandboxID)
			sbox.SetRuntimeHandler(runtimeName)
			sbox.SetContainers(memorystore.New[*oci.Container]())
			err = sbox.SetCRISandbox(
				sbox.ID(),
				make(map[string]string),
				sandboxAnnotations,
				&types.PodSandboxMetadata{},
			)
			Expect(err).ToNot(HaveOccurred())
			sb, err := sbox.GetSandbox()
			Expect(err).ToNot(HaveOccurred())
			containerServer.AddSandbox(context.TODO(), sb) //nolint:errcheck

			container, err := oci.NewContainer(fullContainerID, fullContainerID, "", "",
				make(map[string]string), make(map[string]string),
				make(map[string]string), "pauseImage", nil, nil, "",
				&types.ContainerMetadata{}, fullSandboxID, false, false,
				false, "", "", time.Now(), "")
			if err != nil {
				return nil, nil, err
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

			// Add container to server
			containerServer.AddContainer(context.TODO(), container)

			return container, sb, nil
		}

		addContainersWithConcurrency := func() {
			var wg sync.WaitGroup
			for cpu := 10; cpu < 20; cpu++ {
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					container, sb, err := createContainer(
						containerServer,
						strconv.Itoa(cpu),
						strconv.Itoa(cpu),
						runtimeName,
						sandboxAnnotations,
					)
					Expect(err).ToNot(HaveOccurred())

					hooks := hooksRetriever.Get(ctx, sb.RuntimeHandler(), sb.Annotations())
					Expect(hooks).NotTo(BeNil())
					if hph, ok := hooks.(*HighPerformanceHooks); ok {
						hph.irqSMPAffinityFile = irqSmpAffinityFile
						hph.irqBalanceConfigFile = irqBalanceConfigFile
					}

					err = hooks.PreStart(ctx, containerServer, container, sb)
					Expect(err).ToNot(HaveOccurred())
				}()
			}
			wg.Wait()
		}

		JustBeforeEach(func() {
			flags = "0000,001fff0f"
			bannedCPUFlags = "ffffffff,ffff00f0"

			// Simulate a restart of crio each time as we're modifying the config between runs.
			cpuLoadBalancingAllowedAnywhereOnce = sync.Once{}

			hooksRetriever = NewHooksRetriever(ctx, cfg)

			// create tests affinity file
			err = os.WriteFile(irqSmpAffinityFile, []byte(flags), 0o644)
			Expect(err).ToNot(HaveOccurred())
			err = os.WriteFile(irqBalanceConfigFile, []byte(formatIRQBalanceBannedCPUs(bannedCPUFlags)), 0o644)
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
				libcfg.RuntimeConfig = cfg.RuntimeConfig
			})

			It("should set the correct irq bit mask with concurrency", func(ctx context.Context) {
				addContainersWithConcurrency()
				verifySetIRQLoadBalancing("00000000,0010030f", "ffffffff,ffeffcf0")
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
				libcfg.RuntimeConfig = cfg.RuntimeConfig
			})

			It("should keep the current irq bit mask but return a high performance hooks", func(ctx context.Context) {
				var wg sync.WaitGroup
				for cpu := 10; cpu < 20; cpu++ {
					wg.Add(1)
					go func() {
						defer GinkgoRecover()
						defer wg.Done()
						container, sb, err := createContainer(
							containerServer,
							strconv.Itoa(cpu),
							strconv.Itoa(cpu),
							runtimeName,
							sandboxAnnotations,
						)
						Expect(err).ToNot(HaveOccurred())

						hooks := hooksRetriever.Get(ctx, sb.RuntimeHandler(), sb.Annotations())
						Expect(hooks).NotTo(BeNil())
						hph, ok := hooks.(*HighPerformanceHooks)
						Expect(ok).To(BeTrue())
						hph.irqSMPAffinityFile = irqSmpAffinityFile
						hph.irqBalanceConfigFile = irqBalanceConfigFile

						err = hooks.PreStart(ctx, containerServer, container, sb)
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
				libcfg.RuntimeConfig = cfg.RuntimeConfig
			})

			It("should set the correct irq bit mask with concurrency", func(ctx context.Context) {
				addContainersWithConcurrency()
				verifySetIRQLoadBalancing("00000000,0010030f", "ffffffff,ffeffcf0")
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
				libcfg.RuntimeConfig = cfg.RuntimeConfig
			})

			It("should return a nil hook", func(ctx context.Context) {
				_, sb, err := createContainer(
					containerServer,
					"20",
					"20",
					runtimeName,
					sandboxAnnotations,
				)
				Expect(err).NotTo(HaveOccurred())

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
				libcfg.RuntimeConfig = cfg.RuntimeConfig
			})

			It("should set the correct irq bit mask with concurrency", func(ctx context.Context) {
				addContainersWithConcurrency()
				verifySetIRQLoadBalancing("00000000,0010030f", "ffffffff,ffeffcf0")
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
				libcfg.RuntimeConfig = cfg.RuntimeConfig
			})

			It("should yield a DefaultCPULoadBalanceHooks which keeps the old mask", func(ctx context.Context) {
				var wg sync.WaitGroup
				for cpu := 10; cpu < 20; cpu++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						container, sb, err := createContainer(
							containerServer,
							strconv.Itoa(cpu),
							strconv.Itoa(cpu),
							runtimeName,
							sandboxAnnotations,
						)
						Expect(err).ToNot(HaveOccurred())

						hooks := hooksRetriever.Get(ctx, sb.RuntimeHandler(), sb.Annotations())
						Expect(hooks).NotTo(BeNil())
						_, ok := (hooks).(*DefaultCPULoadBalanceHooks)
						Expect(ok).To(BeTrue())

						err = hooks.PreStart(ctx, containerServer, container, sb)
						Expect(err).ToNot(HaveOccurred())
					}()
				}
				wg.Wait()
				verifySetIRQLoadBalancing(flags, bannedCPUFlags)
			})
		})
	})
	Describe("Make sure that out of order calls can be handled", func() {
		var cfg *config.Config
		var hooksRetriever *HooksRetriever
		var flags string

		formatIRQBalanceBannedCPUs := func(v string) string {
			return fmt.Sprintf("%s=%q", irqBalanceBannedCpus, v)
		}

		irqSmpAffinityFile := filepath.Join(fixturesDir, "irq_smp_affinity")
		irqBalanceConfigFile := filepath.Join(fixturesDir, "irqbalance")

		ctx := context.Background()

		verifySetIRQLoadBalancing := func(expectedIrqSmp, expectedIrqBalance, testDesc string) {
			content, err := os.ReadFile(irqSmpAffinityFile)
			Expect(err).ToNot(HaveOccurred(), testDesc)
			Expect(strings.Trim(string(content), "\n")).To(Equal(expectedIrqSmp), testDesc)

			content, err = os.ReadFile(irqBalanceConfigFile)
			Expect(err).ToNot(HaveOccurred(), testDesc)
			Expect(strings.Trim(string(content), "\n")).To(Equal(formatIRQBalanceBannedCPUs(expectedIrqBalance)), testDesc)
		}

		generateSandbox := func(sandboxID, runtimeName string, sandboxAnnotations map[string]string) *sandbox.Sandbox {
			sbox := baseSandboxBuilder()
			sbox.SetID(sandboxID)
			sbox.SetRuntimeHandler(runtimeName)
			sbox.SetContainers(memorystore.New[*oci.Container]())
			err = sbox.SetCRISandbox(
				sbox.ID(),
				make(map[string]string),
				sandboxAnnotations,
				&types.PodSandboxMetadata{},
			)
			Expect(err).ToNot(HaveOccurred())
			sb, err := sbox.GetSandbox()
			Expect(err).ToNot(HaveOccurred())

			return sb
		}

		generateContainer := func(containerID, sandboxID, cpus string) *oci.Container {
			container, err := oci.NewContainer(containerID, containerID, "", "",
				make(map[string]string), make(map[string]string),
				make(map[string]string), "pauseImage", nil, nil, "",
				&types.ContainerMetadata{}, sandboxID, false, false,
				false, "", "", time.Now(), "")
			Expect(err).NotTo(HaveOccurred())

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

			return container
		}

		generateContainerWithShares := func(containerID, sandboxID, cpus string, shares uint64) *oci.Container {
			container, err := oci.NewContainer(containerID, containerID, "", "",
				make(map[string]string), make(map[string]string),
				make(map[string]string), "pauseImage", nil, nil, "",
				&types.ContainerMetadata{}, sandboxID, false, false,
				false, "", "", time.Now(), "")
			Expect(err).NotTo(HaveOccurred())

			container.SetSpec(
				&specs.Spec{
					Linux: &specs.Linux{
						Resources: &specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Cpus:   cpus,
								Shares: &shares,
							},
						},
					},
				},
			)

			return container
		}

		JustBeforeEach(func() {
			flags = "00000f0f,0f00ff0f"
			bannedCPUFlags = "fffff0f0,ffff00f0"

			// Simulate a restart of crio each time as we're modifying the config between runs.
			cpuLoadBalancingAllowedAnywhereOnce = sync.Once{}

			hooksRetriever = NewHooksRetriever(ctx, cfg)

			// create tests affinity file
			err = os.WriteFile(irqSmpAffinityFile, []byte(flags), 0o644)
			Expect(err).ToNot(HaveOccurred())
			err = os.WriteFile(irqBalanceConfigFile, []byte(formatIRQBalanceBannedCPUs(bannedCPUFlags)), 0o644)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("when spawning and deleting pods", func() {
			BeforeEach(func() {
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
				libcfg.RuntimeConfig = cfg.RuntimeConfig
			})

			It("should set the correct irq bit mask", func(ctx context.Context) {
				sandboxDisableAnnotations := map[string]string{crioannotations.IRQLoadBalancingAnnotation: "disable"}
				container1 := generateContainer("container1", "sandbox1", "0-63")
				sandbox1 := generateSandbox("sandbox1", "default", map[string]string{})
				container2 := generateContainer("container2", "sandbox2", "0-63")
				sandbox2 := generateSandbox("sandbox2", "high-performance", map[string]string{})
				container3a := generateContainer("container3a", "sandbox3", "10-19")
				container3b := generateContainer("container3b", "sandbox3", "20-29")
				sandbox3 := generateSandbox("sandbox3", "high-performance", sandboxDisableAnnotations)
				container4 := generateContainer("container4", "sandbox4", "10-19")
				sandbox4 := generateSandbox("sandbox4", "high-performance", sandboxDisableAnnotations)
				container5 := generateContainer("container5", "sandbox5", "20-29")
				sandbox5 := generateSandbox("sandbox5", "high-performance", sandboxDisableAnnotations)
				container6a := generateContainer("container6a", "sandbox6", "30-39")
				container6b := generateContainerWithShares("container6b", "sandbox6", "0-63", 512) // 512 is not divisible by 1024
				sandbox6 := generateSandbox("sandbox6", "high-performance", sandboxDisableAnnotations)

				addEvents := []struct {
					f                  func() (*sandbox.Sandbox, *oci.Container)
					expectedIrqSmp     string
					expectedIrqBalance string
				}{
					// Should be a NOOP.
					{
						f: func() (*sandbox.Sandbox, *oci.Container) {
							containerServer.AddSandbox(ctx, sandbox1) //nolint:errcheck
							containerServer.AddContainer(ctx, container1)

							return sandbox1, container1
						},
						expectedIrqSmp:     flags,
						expectedIrqBalance: bannedCPUFlags,
					},
					// Should be a NOOP.
					{
						f: func() (*sandbox.Sandbox, *oci.Container) {
							containerServer.AddSandbox(ctx, sandbox2) //nolint:errcheck
							containerServer.AddContainer(ctx, container2)

							return sandbox2, container2
						},
						expectedIrqSmp:     flags,
						expectedIrqBalance: bannedCPUFlags,
					},
					// Create a sandbox with 2 containers. Only process the event for the first container.
					{
						f: func() (*sandbox.Sandbox, *oci.Container) {
							containerServer.AddSandbox(ctx, sandbox3) //nolint:errcheck
							containerServer.AddContainer(ctx, container3a)
							containerServer.AddContainer(ctx, container3b)

							return sandbox3, container3a
						},
						expectedIrqSmp:     "00000f0f,0f00030f",
						expectedIrqBalance: "fffff0f0,f0fffcf0",
					},
					// Now, process the event for the second container.
					{
						f: func() (*sandbox.Sandbox, *oci.Container) {
							return sandbox3, container3b
						},
						expectedIrqSmp:     "00000f0f,0000030f",
						expectedIrqBalance: "fffff0f0,fffffcf0",
					},
					// Now, add 2 containers on the same CPUs to simulate out of order add/remove.
					{
						f: func() (*sandbox.Sandbox, *oci.Container) {
							containerServer.AddSandbox(ctx, sandbox4) //nolint:errcheck
							containerServer.AddContainer(ctx, container4)

							return sandbox4, container4
						},
						expectedIrqSmp:     "00000f0f,0000030f",
						expectedIrqBalance: "fffff0f0,fffffcf0",
					},
					{
						f: func() (*sandbox.Sandbox, *oci.Container) {
							containerServer.AddSandbox(ctx, sandbox5) //nolint:errcheck
							containerServer.AddContainer(ctx, container5)

							return sandbox5, container5
						},
						expectedIrqSmp:     "00000f0f,0000030f",
						expectedIrqBalance: "fffff0f0,fffffcf0",
					},
					// Add container6a (CPUs 30-39) in sandbox6
					{
						f: func() (*sandbox.Sandbox, *oci.Container) {
							containerServer.AddSandbox(ctx, sandbox6) //nolint:errcheck
							containerServer.AddContainer(ctx, container6a)

							return sandbox6, container6a
						},
						expectedIrqSmp:     "00000f00,0000030f",
						expectedIrqBalance: "fffff0ff,fffffcf0",
					},
					// Add container6b (CPUs 0-63, non-whole CPU shares) - should be ignored due to non-1024-divisible shares
					{
						f: func() (*sandbox.Sandbox, *oci.Container) {
							containerServer.AddContainer(ctx, container6b)

							return sandbox6, container6b
						},
						expectedIrqSmp:     "00000f00,0000030f",
						expectedIrqBalance: "fffff0ff,fffffcf0",
					},
				}
				for i, e := range addEvents {
					testDesc := fmt.Sprintf("test add %d", i)
					sb, container := e.f()
					hooks := hooksRetriever.Get(ctx, sb.RuntimeHandler(), sb.Annotations())
					if hooks != nil {
						if hph, ok := hooks.(*HighPerformanceHooks); ok {
							hph.irqSMPAffinityFile = irqSmpAffinityFile
							hph.irqBalanceConfigFile = irqBalanceConfigFile
						}

						err = hooks.PreStart(ctx, containerServer, container, sb)
						Expect(err).ToNot(HaveOccurred(), testDesc)
					}

					verifySetIRQLoadBalancing(e.expectedIrqSmp, e.expectedIrqBalance, testDesc)
				}

				deleteEvents := []struct {
					f                  func() (*sandbox.Sandbox, *oci.Container)
					expectedIrqSmp     string
					expectedIrqBalance string
				}{
					// Delete the sandbox with 2 containers - process the event for the first container.
					{
						f: func() (*sandbox.Sandbox, *oci.Container) {
							return sandbox3, container3a
						},
						expectedIrqSmp:     "00000f00,0000030f",
						expectedIrqBalance: "fffff0ff,fffffcf0",
					},
					// Now, process the event for the second container.
					{
						f: func() (*sandbox.Sandbox, *oci.Container) {
							return sandbox3, container3b
						},
						expectedIrqSmp:     "00000f00,0000030f",
						expectedIrqBalance: "fffff0ff,fffffcf0",
					},
					// The containers are removed after PreStop. So let's clean the containers up here.
					{
						f: func() (*sandbox.Sandbox, *oci.Container) {
							containerServer.RemoveContainer(ctx, container3a)
							containerServer.RemoveContainer(ctx, container3b)
							containerServer.RemoveSandbox(ctx, sandbox3.ID()) //nolint:errcheck

							return nil, nil
						},
					},
					// Now, delete the container on 10-19.
					{
						f: func() (*sandbox.Sandbox, *oci.Container) {
							return sandbox4, container4
						},
						expectedIrqSmp:     "00000f00,000fff0f",
						expectedIrqBalance: "fffff0ff,fff000f0",
					},
					// The container is removed after PreStop. So let's clean the container up here.
					{
						f: func() (*sandbox.Sandbox, *oci.Container) {
							containerServer.RemoveContainer(ctx, container4)
							containerServer.RemoveSandbox(ctx, sandbox4.ID()) //nolint:errcheck

							return nil, nil
						},
					},
					// Now, delete the container on 20-29.
					{
						f: func() (*sandbox.Sandbox, *oci.Container) {
							return sandbox5, container5
						},
						expectedIrqSmp:     "00000f00,3fffff0f",
						expectedIrqBalance: "fffff0ff,c00000f0",
					},
					// The containers is removed after PreStop. So let's clean the container up here.
					{
						f: func() (*sandbox.Sandbox, *oci.Container) {
							containerServer.RemoveContainer(ctx, container5)
							containerServer.RemoveSandbox(ctx, sandbox5.ID()) //nolint:errcheck

							return nil, nil
						},
					},
					// Delete container6a (CPUs 30-39)
					{
						f: func() (*sandbox.Sandbox, *oci.Container) {
							return sandbox6, container6a
						},
						expectedIrqSmp:     "00000fff,ffffff0f",
						expectedIrqBalance: "fffff000,000000f0",
					},
					// Delete container6b (CPUs 0-63, non-whole CPU shares) - should be ignored
					// Take note that it's expected that we are not back at the original flags. As cri-o spawns and
					// deletes containers with IRQ SMP affinity requests, it sets the containers back to the expected
					// state (expected after looking at all containers on those CPUs) which might be different from the
					// original state.
					{
						f: func() (*sandbox.Sandbox, *oci.Container) {
							return sandbox6, container6b
						},
						expectedIrqSmp:     "00000fff,ffffff0f",
						expectedIrqBalance: "fffff000,000000f0",
					},
					// Clean up container6a, container6b and sandbox6
					{
						f: func() (*sandbox.Sandbox, *oci.Container) {
							containerServer.RemoveContainer(ctx, container6a)
							containerServer.RemoveContainer(ctx, container6b)
							containerServer.RemoveSandbox(ctx, sandbox6.ID()) //nolint:errcheck

							return nil, nil
						},
					},
				}
				for i, e := range deleteEvents {
					testDesc := fmt.Sprintf("test delete %d", i)
					sb, container := e.f()
					if sb == nil && container == nil {
						continue
					}
					hooks := hooksRetriever.Get(ctx, sb.RuntimeHandler(), sb.Annotations())
					if hooks != nil {
						if hph, ok := hooks.(*HighPerformanceHooks); ok {
							hph.irqSMPAffinityFile = irqSmpAffinityFile
							hph.irqBalanceConfigFile = irqBalanceConfigFile
						}

						err = hooks.PreStop(ctx, containerServer, container, sb)
						Expect(err).ToNot(HaveOccurred())
					}

					verifySetIRQLoadBalancing(e.expectedIrqSmp, e.expectedIrqBalance, testDesc)
				}
			})
		})
	})
})
