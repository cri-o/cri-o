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
var _ = Describe("high_performance_hooks", func() {
	container, err := oci.NewContainer("containerID", "", "", "",
		make(map[string]string), make(map[string]string),
		make(map[string]string), "pauseImage", "", "",
		&pb.ContainerMetadata{}, "sandboxID", false, false,
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

	Describe("setRPSLoadBalancing", func() {
		rpsFile := filepath.Join(fixturesDir, "rps_cpus")
		onlineCPUsFile := filepath.Join(fixturesDir, "online_cpus")
		var onlineCPUs string

		verifySetRPSLoadBalancing := func(enabled bool, expected string) {
			err := setRPSLoadBalancing(container, fixturesDir, onlineCPUsFile, enabled)
			Expect(err).To(BeNil())

			content, err := ioutil.ReadFile(rpsFile)
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
								Cpus: "1,2",
							},
						},
					},
				},
			)

			// create the test rps file
			err = ioutil.WriteFile(rpsFile, []byte(flags), 0o644)
			Expect(err).To(BeNil())

			// create cpus online file
			err = ioutil.WriteFile(onlineCPUsFile, []byte(onlineCPUs), 0o644)
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			if err := os.Remove(rpsFile); err != nil {
				log.Errorf(context.TODO(), "failed to remove temporary test files: %v", err)
			}
			if err := os.Remove(onlineCPUsFile); err != nil {
				log.Errorf(context.TODO(), "failed to remove temporary test files: %v", err)
			}
		})

		Context("single guaranteed pod, container start", func() {
			BeforeEach(func() {
				onlineCPUs = "0-3"
				flags = "0" // rps disabled
			})

			It("should mask the container cpus from rps", func() {
				expected := "00000000,00000009" // rps set to 0,3 (pod cpus are 1,2)
				verifySetRPSLoadBalancing(true, expected)
			})
		})

		Context("single guaranteed pod, container termination", func() {
			BeforeEach(func() {
				onlineCPUs = "0-3"
				flags = "9" // rps set by the pod to cpus 0,3
			})

			It("should clear the rps bit mask", func() {
				expected := "0" // no other pods, the rps should be disabled
				verifySetRPSLoadBalancing(false, expected)
			})
		})

		Context("existing guaranteed pod, container start", func() {
			BeforeEach(func() {
				onlineCPUs = "0-32"
				flags = "1,feffffff" // rps set by a guaranteed pod on cpu 24
			})

			It("should mask the container cpus from rps", func() {
				expected := "00000001,fefffff9" // both pods cpus 0,3,24 should not be set
				verifySetRPSLoadBalancing(true, expected)
			})
		})

		Context("existing guaranteed pod, container termination", func() {
			BeforeEach(func() {
				onlineCPUs = "0-32"
				flags = "1,fefffff9" // rps set by both pods to cpus 0,3,24
			})

			It("should clear the rps bit mask", func() {
				expected := "00000001,feffffff" // previous pod cpu 24 should not be set
				verifySetRPSLoadBalancing(false, expected)
			})
		})
	})
})
