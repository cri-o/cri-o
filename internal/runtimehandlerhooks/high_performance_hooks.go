package runtimehandlerhooks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	crioannotations "github.com/cri-o/cri-o/pkg/annotations"
	"github.com/cri-o/cri-o/utils/cmdrunner"
	"github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/opencontainers/runc/libcontainer/cgroups/systemd"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

const (
	// HighPerformance contains the high-performance runtime handler name
	HighPerformance = "high-performance"
	// IrqBannedCPUConfigFile contains the original banned cpu mask configuration
	IrqBannedCPUConfigFile = "/etc/sysconfig/orig_irq_banned_cpus"
	// IrqSmpAffinityProcFile contains the default smp affinity mask configuration
	IrqSmpAffinityProcFile = "/proc/irq/default_smp_affinity"
)

const (
	annotationTrue       = "true"
	annotationDisable    = "disable"
	schedDomainDir       = "/proc/sys/kernel/sched_domain"
	cgroupMountPoint     = "/sys/fs/cgroup"
	irqBalanceBannedCpus = "IRQBALANCE_BANNED_CPUS"
	irqBalancedName      = "irqbalance"
)

// HighPerformanceHooks used to run additional hooks that will configure a system for the latency sensitive workloads
type HighPerformanceHooks struct {
	irqBalanceConfigFile string
}

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
		if err := setCPUSLoadBalancingWithRetry(ctx, c, false); err != nil {
			return fmt.Errorf("set CPU load balancing: %w", err)
		}
	}

	// disable the IRQ smp load balancing for the container CPUs
	if shouldIRQLoadBalancingBeDisabled(s.Annotations()) {
		log.Infof(ctx, "Disable irq smp balancing for container %q", c.ID())
		if err := setIRQLoadBalancing(c, false, IrqSmpAffinityProcFile, h.irqBalanceConfigFile); err != nil {
			return fmt.Errorf("set IRQ load balancing: %w", err)
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
			return fmt.Errorf("set CPU CFS quota: %w", err)
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
		if err := setCPUSLoadBalancingWithRetry(ctx, c, true); err != nil {
			return fmt.Errorf("set CPU load balancing: %w", err)
		}
	}

	// enable the IRQ smp balancing for the container CPUs
	if shouldIRQLoadBalancingBeDisabled(s.Annotations()) {
		if err := setIRQLoadBalancing(c, true, IrqSmpAffinityProcFile, h.irqBalanceConfigFile); err != nil {
			return fmt.Errorf("set IRQ load balancing: %w", err)
		}
	}

	// no need to reverse the cgroup CPU CFS quota setting as the pod cgroup will be deleted anyway

	return nil
}

func shouldCPULoadBalancingBeDisabled(annotations fields.Set) bool {
	if annotations[crioannotations.CPULoadBalancingAnnotation] == annotationTrue {
		log.Warnf(context.TODO(), annotationValueDeprecationWarning(crioannotations.CPULoadBalancingAnnotation))
	}

	return annotations[crioannotations.CPULoadBalancingAnnotation] == annotationTrue ||
		annotations[crioannotations.CPULoadBalancingAnnotation] == annotationDisable
}

func shouldCPUQuotaBeDisabled(annotations fields.Set) bool {
	if annotations[crioannotations.CPUQuotaAnnotation] == annotationTrue {
		log.Warnf(context.TODO(), annotationValueDeprecationWarning(crioannotations.CPUQuotaAnnotation))
	}

	return annotations[crioannotations.CPUQuotaAnnotation] == annotationTrue ||
		annotations[crioannotations.CPUQuotaAnnotation] == annotationDisable
}

func shouldIRQLoadBalancingBeDisabled(annotations fields.Set) bool {
	if annotations[crioannotations.IRQLoadBalancingAnnotation] == annotationTrue {
		log.Warnf(context.TODO(), annotationValueDeprecationWarning(crioannotations.IRQLoadBalancingAnnotation))
	}

	return annotations[crioannotations.IRQLoadBalancingAnnotation] == annotationTrue ||
		annotations[crioannotations.IRQLoadBalancingAnnotation] == annotationDisable
}

func annotationValueDeprecationWarning(annotation string) string {
	return fmt.Sprintf("The usage of the annotation %q with value %q will be deprecated under 1.21", annotation, "true")
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

func setCPUSLoadBalancingWithRetry(ctx context.Context, c *oci.Container, enable bool) error {
	log.Infof(ctx, "Disable cpu load balancing for container %q", c.ID())
	// it is possible to have errors during reading or writing to sched_domain files because
	// that kernel rebuilds it with updated values
	// the retry will not fix it for 100% but should reduce the possibility for failures to minimum
	// TODO: re-visit once we will have some more acceptable cgroups hierarchy to disable CPU load balancing
	// correctly via cgroups, see -https://bugzilla.redhat.com/show_bug.cgi?id=1946801
	return wait.PollImmediate(time.Second, 5*time.Second, func() (bool, error) {
		if err := setCPUSLoadBalancing(c, enable, schedDomainDir); err != nil {
			if os.IsNotExist(err) {
				log.Errorf(ctx, "Failed to set CPU load balancing: %v", err)
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
}

func setCPUSLoadBalancing(c *oci.Container, enable bool, schedDomainDir string) error {
	lspec := c.Spec().Linux
	if lspec == nil ||
		lspec.Resources == nil ||
		lspec.Resources.CPU == nil ||
		lspec.Resources.CPU.Cpus == "" {
		return fmt.Errorf("find container %s CPUs", c.ID())
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
			content, err := os.ReadFile(path)
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

			return os.WriteFile(path, []byte(newContent), 0o644)
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func setIRQLoadBalancing(c *oci.Container, enable bool, irqSmpAffinityFile, irqBalanceConfigFile string) error {
	lspec := c.Spec().Linux
	if lspec == nil ||
		lspec.Resources == nil ||
		lspec.Resources.CPU == nil ||
		lspec.Resources.CPU.Cpus == "" {
		return fmt.Errorf("find container %s CPUs", c.ID())
	}

	content, err := os.ReadFile(irqSmpAffinityFile)
	if err != nil {
		return err
	}
	currentIRQSMPSetting := strings.TrimSpace(string(content))
	newIRQSMPSetting, newIRQBalanceSetting, err := UpdateIRQSmpAffinityMask(lspec.Resources.CPU.Cpus, currentIRQSMPSetting, enable)
	if err != nil {
		return err
	}
	if err := os.WriteFile(irqSmpAffinityFile, []byte(newIRQSMPSetting), 0o644); err != nil {
		return err
	}

	isIrqConfigExists := fileExists(irqBalanceConfigFile)

	if isIrqConfigExists {
		if err := updateIrqBalanceConfigFile(irqBalanceConfigFile, newIRQBalanceSetting); err != nil {
			return err
		}
	}

	if !isServiceEnabled(irqBalancedName) || !isIrqConfigExists {
		if _, err := exec.LookPath(irqBalancedName); err != nil {
			// irqbalance is not installed, skip the rest; pod should still start, so return nil instead
			logrus.Warnf("Irqbalance binary not found: %v", err)
			return nil
		}
		// run irqbalance in daemon mode, so this won't cause delay
		cmd := cmdrunner.Command(irqBalancedName, "--oneshot")
		additionalEnv := irqBalanceBannedCpus + "=" + newIRQBalanceSetting
		cmd.Env = append(os.Environ(), additionalEnv)
		return cmd.Run()
	}

	if err := restartIrqBalanceService(); err != nil {
		logrus.Warnf("Irqbalance service restart failed: %v", err)
	}
	return nil
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
		if err := os.WriteFile(cfsQuotaPath, []byte("0"), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(parentCfsQuotaPath, []byte("0"), 0o644); err != nil {
			return err
		}
	} else {
		if err := os.WriteFile(cfsQuotaPath, []byte("-1"), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(parentCfsQuotaPath, []byte("-1"), 0o644); err != nil {
			return err
		}
	}

	return nil
}

// RestoreIrqBalanceConfig restores irqbalance service with original banned cpu mask settings
func RestoreIrqBalanceConfig(irqBalanceConfigFile, irqBannedCPUConfigFile, irqSmpAffinityProcFile string) error {
	content, err := os.ReadFile(irqSmpAffinityProcFile)
	if err != nil {
		return err
	}
	current := strings.TrimSpace(string(content))
	// remove ","; now each element is "0-9,a-f"
	s := strings.ReplaceAll(current, ",", "")
	currentMaskArray, err := mapHexCharToByte(s)
	if err != nil {
		return err
	}
	if !isAllBitSet(currentMaskArray) {
		// not system reboot scenario, just return it.
		return nil
	}

	bannedCPUMasks, err := retrieveIrqBannedCPUMasks(irqBalanceConfigFile)
	if err != nil {
		// Ignore returning err as given irqBalanceConfigFile may not exist.
		return nil
	}
	if !fileExists(irqBannedCPUConfigFile) {
		irqBannedCPUsConfig, err := os.Create(irqBannedCPUConfigFile)
		if err != nil {
			return err
		}
		defer irqBannedCPUsConfig.Close()
		_, err = irqBannedCPUsConfig.WriteString(bannedCPUMasks)
		if err != nil {
			return err
		}
		return nil
	}

	content, err = os.ReadFile(irqBannedCPUConfigFile)
	if err != nil {
		return err
	}
	origBannedCPUMasks := strings.TrimSpace(string(content))

	if bannedCPUMasks == origBannedCPUMasks {
		return nil
	}
	if err := updateIrqBalanceConfigFile(irqBalanceConfigFile, origBannedCPUMasks); err != nil {
		return err
	}
	if isServiceEnabled(irqBalancedName) {
		if err := restartIrqBalanceService(); err != nil {
			logrus.Warnf("Irqbalance service restart failed: %v", err)
		}
	}
	return nil
}
