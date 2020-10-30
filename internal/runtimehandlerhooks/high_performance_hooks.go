package runtimehandlerhooks

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/opencontainers/runc/libcontainer/cgroups/systemd"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

const (
	// HighPerformance contains the high-performance runtime handler name
	HighPerformance = "high-performance"
)

const (
	annotationCPULoadBalancing = "cpu-load-balancing.crio.io"
	annotationCPUQuota         = "cpu-quota.crio.io"
	annotationIRQLoadBalancing = "irq-load-balancing.crio.io"
	annotationTrue             = "true"
	schedDomainDir             = "/proc/sys/kernel/sched_domain"
	irqSmpAffinityProcFile     = "/proc/irq/default_smp_affinity"
	cgroupMountPoint           = "/sys/fs/cgroup"
)

// HighPerformanceHooks used to run additional hooks that will configure a system for the latency sensitive workloads
type HighPerformanceHooks struct{}

func (h *HighPerformanceHooks) PreStart(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error {
	log.Infof(ctx, "Run %q runtime handler pre-start hook for the container %q", HighPerformance, c.ID())

	if isCgroupParentBurstable(s) {
		log.Infof(ctx, "Container %q is a burstable pod. Skip PreStart.", c.ID())
		return nil
	}
	if isCgroupParentBestEffort(s) {
		log.Infof(ctx, "Container %q is a besteffort pod. Skip PreStart.", c.ID())
		return nil
	}
	if !isContainerRequestWholeCPU(c) {
		log.Infof(ctx, "Container %q requests partial cpu(s). Skip PreStart", c.ID())
		return nil
	}

	// disable the CPU load balancing for the container CPUs
	if shouldCPULoadBalancingBeDisabled(s.Annotations()) {
		log.Infof(ctx, "Disable cpu load balancing for container %q", c.ID())
		if err := setCPUSLoadBalancing(c, false, schedDomainDir); err != nil {
			return errors.Wrap(err, "set CPU load balancing")
		}
	}
	// disable the IRQ smp load balancing for the container CPUs
	if shouldIRQLoadBalancingBeDisabled(s.Annotations()) {
		log.Infof(ctx, "Disable irq smp balancing for container %q", c.ID())
		if err := setIRQLoadBalancing(c, false, irqSmpAffinityProcFile); err != nil {
			return errors.Wrap(err, "set IRQ load balancing")
		}
	}
	// disable the CFS quota for the container CPUs
	if shouldCPUQuotaBeDisabled(s.Annotations()) {
		log.Infof(ctx, "Disable cpu cfs quota for container %q", c.ID())
		cpuMountPoint, err := cgroups.FindCgroupMountpoint(cgroupMountPoint, "cpu")
		if err != nil {
			return err
		}
		if err := setCPUQuota(cpuMountPoint, s.CgroupParent(), c, false); err != nil {
			return errors.Wrap(err, "set CPU CFS quota")
		}
	}

	return nil
}

func (h *HighPerformanceHooks) PreStop(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error {
	log.Infof(ctx, "Run %q runtime handler pre-stop hook for the container %q", HighPerformance, c.ID())

	if isCgroupParentBurstable(s) {
		log.Infof(ctx, "Container %q is a burstable pod. Skip PreStop.", c.ID())
		return nil
	}
	if isCgroupParentBestEffort(s) {
		log.Infof(ctx, "Container %q is a besteffort pod. Skip PreStop.", c.ID())
		return nil
	}
	if !isContainerRequestWholeCPU(c) {
		log.Infof(ctx, "Container %q requests partial cpu(s). Skip PreStop", c.ID())
		return nil
	}

	// enable the CPU load balancing for the container CPUs
	if shouldCPULoadBalancingBeDisabled(s.Annotations()) {
		if err := setCPUSLoadBalancing(c, true, schedDomainDir); err != nil {
			return errors.Wrap(err, "set CPU load balancing")
		}
	}
	// enable the IRQ smp balancing for the container CPUs
	if shouldIRQLoadBalancingBeDisabled(s.Annotations()) {
		if err := setIRQLoadBalancing(c, true, irqSmpAffinityProcFile); err != nil {
			return errors.Wrap(err, "set IRQ load balancing")
		}
	}
	// no need to reverse the cgroup CPU CFS quota setting as the pod cgroup will be deleted anyway

	return nil
}

func shouldCPULoadBalancingBeDisabled(annotations fields.Set) bool {
	return annotations[annotationCPULoadBalancing] == annotationTrue
}

func shouldCPUQuotaBeDisabled(annotations fields.Set) bool {
	return annotations[annotationCPUQuota] == annotationTrue
}

func shouldIRQLoadBalancingBeDisabled(annotations fields.Set) bool {
	return annotations[annotationIRQLoadBalancing] == annotationTrue
}

func isCgroupParentBurstable(s *sandbox.Sandbox) bool {
	return strings.Contains(s.CgroupParent(), "burstable")
}

func isCgroupParentBestEffort(s *sandbox.Sandbox) bool {
	return strings.Contains(s.CgroupParent(), "besteffort")
}

func isContainerRequestWholeCPU(c *oci.Container) bool {
	return *(c.Spec().Linux.Resources.CPU.Shares)%1024 == 0
}

func setCPUSLoadBalancing(c *oci.Container, enable bool, schedDomainDir string) error {
	lspec := c.Spec().Linux
	if lspec == nil ||
		lspec.Resources == nil ||
		lspec.Resources.CPU == nil ||
		lspec.Resources.CPU.Cpus == "" {
		return errors.Errorf("find container %s CPUs", c.ID())
	}

	cpus, err := cpuset.Parse(lspec.Resources.CPU.Cpus)
	if err != nil {
		return err
	}

	for _, cpu := range cpus.ToSlice() {
		cpuSchedDomainDir := fmt.Sprintf("%s/cpu%d", schedDomainDir, cpu)
		err := filepath.Walk(cpuSchedDomainDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() || info.Name() != "flags" {
				return nil
			}
			content, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}

			flags, err := strconv.Atoi(strings.Trim(string(content), "\n"))
			if err != nil {
				return err
			}

			var newContent string
			if enable {
				newContent = strconv.Itoa(flags | 1)
			} else {
				// we should set the LSB to 0 to disable the load balancing for the specified CPU
				// in case of sched domain all flags can be represented by the binary number 111111111111111 that equals
				// to 32767 in the decimal form
				// see https://github.com/torvalds/linux/blob/0fe5f9ca223573167c4c4156903d751d2c8e160e/include/linux/sched/topology.h#L14
				// for more information regarding the sched domain flags
				newContent = strconv.Itoa(flags & 32766)
			}

			return ioutil.WriteFile(path, []byte(newContent), 0o644)
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func setIRQLoadBalancing(c *oci.Container, enable bool, irqSmpAffinityFile string) error {
	lspec := c.Spec().Linux
	if lspec == nil ||
		lspec.Resources == nil ||
		lspec.Resources.CPU == nil ||
		lspec.Resources.CPU.Cpus == "" {
		return errors.Errorf("find container %s CPUs", c.ID())
	}

	content, err := ioutil.ReadFile(irqSmpAffinityFile)
	if err != nil {
		return err
	}
	currentIRQSMPSetting := strings.TrimSpace(string(content))
	newIRQSMPSetting, newIRQBalanceSetting, err := UpdateIRQSmpAffinityMask(lspec.Resources.CPU.Cpus, currentIRQSMPSetting, enable)
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(irqSmpAffinityFile, []byte(newIRQSMPSetting), 0o644); err != nil {
		return err
	}
	if _, err := exec.LookPath("irqbalance"); err != nil {
		// irqbalance is not installed, skip the rest; pod should still start, so return nil instead
		logrus.Warnf("irqbalance binary not found: %v", err)
		return nil
	}
	// run irqbalance in daemon mode, so this won't cause delay
	cmd := exec.Command("irqbalance", "--oneshot")
	additionalEnv := "IRQBALANCE_BANNED_CPUS=" + newIRQBalanceSetting
	cmd.Env = append(os.Environ(), additionalEnv)
	return cmd.Run()
}

func setCPUQuota(cpuMountPoint, parentDir string, c *oci.Container, enable bool) error {
	var rpath string
	var err error
	var cfsQuotaPath string
	var parentCfsQuotaPath string
	var cgroupManager cgmgr.CgroupManager

	if strings.HasSuffix(parentDir, ".slice") {
		// systemd fs
		if cgroupManager, err = cgmgr.SetCgroupManager("systemd"); err != nil {
			return nil
		}
		parentPath, err := systemd.ExpandSlice(parentDir)
		if err != nil {
			return err
		}
		parentCfsQuotaPath = filepath.Join(cpuMountPoint, parentPath, "cpu.cfs_quota_us")
		if rpath, err = cgroupManager.ContainerCgroupAbsolutePath(parentDir, c.ID()); err != nil {
			return err
		}
		cfsQuotaPath = filepath.Join(cpuMountPoint, rpath, "cpu.cfs_quota_us")
	} else {
		// cgroupfs
		if cgroupManager, err = cgmgr.SetCgroupManager("cgroupfs"); err != nil {
			return nil
		}
		parentCfsQuotaPath = filepath.Join(cpuMountPoint, parentDir, "cpu.cfs_quota_us")
		if rpath, err = cgroupManager.ContainerCgroupAbsolutePath(parentDir, c.ID()); err != nil {
			return err
		}
		cfsQuotaPath = filepath.Join(cpuMountPoint, rpath, "cpu.cfs_quota_us")
	}

	if _, err := os.Stat(cfsQuotaPath); err != nil {
		return err
	}
	if _, err := os.Stat(parentCfsQuotaPath); err != nil {
		return err
	}

	if enable {
		// there should have no use case to get here, as the pod cgroup will be deleted when the pod end
		if err := ioutil.WriteFile(cfsQuotaPath, []byte("0"), 0o644); err != nil {
			return err
		}
		if err := ioutil.WriteFile(parentCfsQuotaPath, []byte("0"), 0o644); err != nil {
			return err
		}
	} else {
		if err := ioutil.WriteFile(cfsQuotaPath, []byte("-1"), 0o644); err != nil {
			return err
		}
		if err := ioutil.WriteFile(parentCfsQuotaPath, []byte("-1"), 0o644); err != nil {
			return err
		}
	}

	return nil
}
