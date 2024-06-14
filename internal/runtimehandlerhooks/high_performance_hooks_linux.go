package runtimehandlerhooks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	crioannotations "github.com/cri-o/cri-o/pkg/annotations"
	"github.com/cri-o/cri-o/utils/cmdrunner"
	"github.com/opencontainers/runc/libcontainer/cgroups"
	libCtrMgr "github.com/opencontainers/runc/libcontainer/cgroups/manager"
	"github.com/opencontainers/runc/libcontainer/configs"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/utils/cpuset"
)

const (
	// HighPerformance contains the high-performance runtime handler name
	HighPerformance = "high-performance"
	// IrqSmpAffinityProcFile contains the default smp affinity mask configuration
	IrqSmpAffinityProcFile = "/proc/irq/default_smp_affinity"
)

const (
	annotationTrue       = "true"
	annotationDisable    = "disable"
	annotationEnable     = "enable"
	schedDomainDir       = "/proc/sys/kernel/sched_domain"
	cgroupMountPoint     = "/sys/fs/cgroup"
	irqBalanceBannedCpus = "IRQBALANCE_BANNED_CPUS"
	irqBalancedName      = "irqbalance"
	sysCPUDir            = "/sys/devices/system/cpu"
	sysCPUSaveDir        = "/var/run/crio/cpu"
	milliCPUToCPU        = 1000
)

const (
	cgroupSubTreeControl = "cgroup.subtree_control"
	cgroupV1QuotaFile    = "cpu.cfs_quota_us"
	cgroupV2QuotaFile    = "cpu.max"
	cpusetCpus           = "cpuset.cpus"
	cpusetCpusExclusive  = "cpuset.cpus.exclusive"
	IsolatedCPUsEnvVar   = "OPENSHIFT_ISOLATED_CPUS"
	SharedCPUsEnvVar     = "OPENSHIFT_SHARED_CPUS"
)

// HighPerformanceHooks used to run additional hooks that will configure a system for the latency sensitive workloads
type HighPerformanceHooks struct {
	irqBalanceConfigFile string
	cpusetLock           sync.Mutex
	sharedCPUs           string
}

func (h *HighPerformanceHooks) PreCreate(ctx context.Context, specgen *generate.Generator, s *sandbox.Sandbox, c *oci.Container) error {
	log.Infof(ctx, "Run %q runtime handler pre-create hook for the container %q", HighPerformance, c.ID())
	if !shouldRunHooks(ctx, c.ID(), specgen.Config, s) {
		return nil
	}

	if requestedSharedCPUs(s.Annotations(), c.CRIContainer().GetMetadata().GetName()) {
		if isContainerCPUsSpecEmpty(specgen.Config) {
			return fmt.Errorf("no cpus found for container %q", c.Name())
		}
		cpusString := specgen.Config.Linux.Resources.CPU.Cpus
		exclusiveCPUs, err := cpuset.Parse(cpusString)
		if err != nil {
			return fmt.Errorf("failed to parse container %q cpus: %w", c.Name(), err)
		}
		if h.sharedCPUs == "" {
			return fmt.Errorf("shared CPUs were requested for container %q but none are defined", c.Name())
		}
		sharedCPUSet, err := cpuset.Parse(h.sharedCPUs)
		if err != nil {
			return fmt.Errorf("failed to parse shared cpus: %w", err)
		}
		// We must inject the environment variables in the PreCreate stage,
		// because in the PreStart stage the process is already constructed.
		// by the low-level runtime and the environment variables are already finalized.
		injectCpusetEnv(specgen, &exclusiveCPUs, &sharedCPUSet)
	}
	return nil
}

func (h *HighPerformanceHooks) PreStart(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error {
	log.Infof(ctx, "Run %q runtime handler pre-start hook for the container %q", HighPerformance, c.ID())

	cSpec := c.Spec()
	if !shouldRunHooks(ctx, c.ID(), &cSpec, s) {
		return nil
	}

	// creating libctr managers is expensive on v1. Reuse between CPU load balancing and CPU quota
	podManager, containerManagers, err := libctrManagersForPodAndContainerCgroup(c, s.CgroupParent())
	if err != nil {
		return err
	}

	var sharedCPUsRequested bool
	if requestedSharedCPUs(s.Annotations(), c.CRIContainer().GetMetadata().GetName()) {
		sharedCPUsRequested = true
		if containerManagers, err = setSharedCPUs(c, containerManagers, h.sharedCPUs); err != nil {
			return fmt.Errorf("setSharedCPUs: failed to set shared CPUs for container %q; %w", c.Name(), err)
		}
		if err := injectQuotaGivenSharedCPUs(c, podManager, containerManagers, h.sharedCPUs); err != nil {
			return err
		}
	}

	// disable the CPU load balancing for the container CPUs
	if shouldCPULoadBalancingBeDisabled(s.Annotations()) {
		if err := h.setCPULoadBalancing(c, podManager, containerManagers, false, sharedCPUsRequested); err != nil {
			return fmt.Errorf("set CPU load balancing: %w", err)
		}
	}

	// disable the IRQ smp load balancing for the container CPUs
	if shouldIRQLoadBalancingBeDisabled(s.Annotations()) {
		log.Infof(ctx, "Disable irq smp balancing for container %q", c.ID())
		if err := setIRQLoadBalancing(ctx, c, false, IrqSmpAffinityProcFile, h.irqBalanceConfigFile); err != nil {
			return fmt.Errorf("set IRQ load balancing: %w", err)
		}
	}

	// disable the CFS quota for the container CPUs
	if shouldCPUQuotaBeDisabled(s.Annotations()) {
		log.Infof(ctx, "Disable cpu cfs quota for container %q", c.ID())
		if err := setCPUQuota(podManager, containerManagers); err != nil {
			return fmt.Errorf("set CPU CFS quota: %w", err)
		}
	}

	// Configure c-states for the container CPUs.
	if configure, value := shouldCStatesBeConfigured(s.Annotations()); configure {
		maxLatency, err := convertAnnotationToLatency(value)
		if err != nil {
			return err
		}

		if maxLatency != "" {
			log.Infof(ctx, "Configure c-states for container %q to %q (pm_qos_resume_latency_us: %q)", c.ID(), value, maxLatency)
			if err := setCPUPMQOSResumeLatency(c, maxLatency); err != nil {
				return fmt.Errorf("set CPU PM QOS resume latency: %w", err)
			}
		}
	}

	// Configure cpu freq governor for the container CPUs.
	if configure, value := shouldFreqGovernorBeConfigured(s.Annotations()); configure {
		log.Infof(ctx, "Configure cpu freq governor for container %q to %q", c.ID(), value)
		// Set the cpu freq governor to specified value.
		if err := setCPUFreqGovernor(c, value); err != nil {
			return fmt.Errorf("set CPU scaling governor: %w", err)
		}
	}

	return nil
}

func (h *HighPerformanceHooks) PreStop(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	log.Infof(ctx, "Run %q runtime handler pre-stop hook for the container %q", HighPerformance, c.ID())

	cSpec := c.Spec()
	if !shouldRunHooks(ctx, c.ID(), &cSpec, s) {
		return nil
	}

	// enable the IRQ smp balancing for the container CPUs
	if shouldIRQLoadBalancingBeDisabled(s.Annotations()) {
		if err := setIRQLoadBalancing(ctx, c, true, IrqSmpAffinityProcFile, h.irqBalanceConfigFile); err != nil {
			return fmt.Errorf("set IRQ load balancing: %w", err)
		}
	}

	// disable the CPU load balancing for the container CPUs
	if shouldCPULoadBalancingBeDisabled(s.Annotations()) {
		podManager, containerManagers, err := libctrManagersForPodAndContainerCgroup(c, s.CgroupParent())
		if err != nil {
			return err
		}
		if err := h.setCPULoadBalancing(c, podManager, containerManagers, true, requestedSharedCPUs(s.Annotations(), c.CRIContainer().GetMetadata().GetName())); err != nil {
			return fmt.Errorf("set CPU load balancing: %w", err)
		}
	}

	// no need to reverse the cgroup CPU CFS quota setting as the pod cgroup will be deleted anyway

	// Restore the c-state configuration for the container CPUs (only do this when the annotation is
	// present - without the annotation we do not modify the c-state).
	if configure, _ := shouldCStatesBeConfigured(s.Annotations()); configure {
		// Restore the original resume latency value.
		if err := setCPUPMQOSResumeLatency(c, ""); err != nil {
			return fmt.Errorf("set CPU PM QOS resume latency: %w", err)
		}
	}

	// Restore the cpu freq governor for the container CPUs (only do this when the annotation is
	// present - without the annotation we do not modify the governor).
	if configure, _ := shouldFreqGovernorBeConfigured(s.Annotations()); configure {
		// Restore the original scaling governor.
		if err := setCPUFreqGovernor(c, ""); err != nil {
			return fmt.Errorf("set CPU scaling governor: %w", err)
		}
	}

	return nil
}

// If CPU load balancing is enabled, then *all* containers must run this PostStop hook.
func (*HighPerformanceHooks) PostStop(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error {
	// We could check if `!cpuLoadBalancingAllowed()` here, but it requires access to the config, which would be
	// odd to plumb. Instead, always assume if they're using a HighPerformanceHook, they have CPULoadBalanceDisabled
	// annotation allowed.
	h := &DefaultCPULoadBalanceHooks{}
	return h.PostStop(ctx, c, s)
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

func shouldCStatesBeConfigured(annotations fields.Set) (present bool, value string) {
	value, present = annotations[crioannotations.CPUCStatesAnnotation]
	return
}

func shouldFreqGovernorBeConfigured(annotations fields.Set) (present bool, value string) {
	value, present = annotations[crioannotations.CPUFreqGovernorAnnotation]
	return
}

func annotationValueDeprecationWarning(annotation string) string {
	return fmt.Sprintf("The usage of the annotation %q with value %q will be deprecated under 1.21", annotation, "true")
}

func requestedSharedCPUs(annotations fields.Set, cName string) bool {
	key := crioannotations.CPUSharedAnnotation + "/" + cName
	v, ok := annotations[key]
	return ok && v == annotationEnable
}

// setCPULoadBalancing relies on the cpuset cgroup to disable load balancing for containers.
// The requisite condition to allow this is `cpuset.sched_load_balance` field must be set to 0 for all cgroups
// that intersect with `cpuset.cpus` of the container that desires load balancing.
// Since CRI-O is the owner of the container cgroup, it must set this value for
// the container. Some other entity (kubelet, external service) must ensure this is the case for all
// other cgroups that intersect (at minimum: all parent cgroups of this cgroup).
func (h *HighPerformanceHooks) setCPULoadBalancing(c *oci.Container, podManager cgroups.Manager, containerManagers []cgroups.Manager, enable, sharedCPUsRequested bool) error {
	if node.CgroupIsV2() {
		return h.setCPULoadBalancingV2(c, podManager, containerManagers, enable, sharedCPUsRequested)
	}
	if !enable {
		if err := disableCPULoadBalancingV1(containerManagers); err != nil {
			return err
		}
	}
	// There is nothing to do in cgroupv1 to re-enable load balancing
	return nil
}

// desiredManagerCPUSetState is a wrapper struct for a libcontainer cgroup manager
// Inside, it tracks the desired state of the cgroup. `exclusiveCPUs` signifies which
// CPUs are being set/unset as load balanced for this cgroup.
// `shared` is for cgroups that have multiple children--some of which will have load balancing
// disabled, some that won't.
type desiredManagerCPUSetState struct {
	manager       cgroups.Manager
	exclusiveCPUs cpuset.CPUSet
	cpus          cpuset.CPUSet
}

// On cgroupv2 systems, a new kernel API has been added to support load balancing
// in a "remote" partition, layers away from the root cgroup.
// This is done with a special file `cpuset.cpus.exclusive` which can be written to
// request that cpuset be on standby for use by a cgroup that desires to be a partition.
// To do this, each parent of the final cgroup must also have this value in the cpuset.cpus.exclusive,
// and the final cgroup must have cpuset.cpus.partition = isolated.
// This will cause the kernel to put that cpuset in a separate scheduling domain.
// While this requires CRI-O to write to cgroups it does not own, it would be cumbersome to teach
// other components in the system (kubelet/cpumanager) which cpu is newly set to exclusive each time a pod request load balancing disabled.
// Thus, this implementation assumes a certain amount of ownership CRI-O takes over this field. This ownership may not apply in the future.
// Another note on cgroup ownership: currently, CRI-O overwrites cpuset.cpus, which is a field managed by systemd.
// To avoid systemd clobbering this value, a libcontainer cgroup manager object is created, and through it CRI-O will use dbus to make changes to the cgroup.
func (h *HighPerformanceHooks) setCPULoadBalancingV2(c *oci.Container, podManager cgroups.Manager, containerManagers []cgroups.Manager, enable, sharedCPUsRequested bool) (retErr error) {
	cpusString := c.Spec().Linux.Resources.CPU.Cpus
	exclusiveCPUs, err := cpuset.Parse(cpusString)
	if err != nil {
		return err
	}
	// We need to construct a slice of managers from top to bottom. We already have the container's manager,
	// potentially the parent manager, and pod manager.
	// So we can begin by constructing all the parents of the pod manager
	podManagerPath := podManager.Path("")
	systemd := false
	if strings.HasSuffix(podManagerPath, ".slice") {
		systemd = true
	}

	// The desired structure here, assuming systemd cgroup manager, is something like the following:
	// /sys/fs/cgroup/kubepods.slice/kubepods-podID.slice/crio-ID.scope
	// where everything above crio-ID.scope is "shared" and crio-ID.scope will be private.
	// "shared" in this context means there will be other active cgroups as children, so we can't have cpuset.cpus
	// only have the exclusive set. Instead, those shared cgroups must have the full set, and cpuset.cpus.exclusive
	// should still have the exclusive set.
	directories := strings.Split(strings.TrimPrefix(podManagerPath, cgroupMountPoint), "/")
	managers := make([]*desiredManagerCPUSetState, 0)
	parent := ""

	allCPUs, err := fullCPUSet()
	if err != nil {
		return err
	}

	for _, dir := range directories {
		if dir == "" {
			continue
		}
		mgr, err := libctrManager(dir, parent, systemd)
		if err != nil {
			return err
		}
		managers = append(managers, &desiredManagerCPUSetState{
			manager:       mgr,
			exclusiveCPUs: exclusiveCPUs,
			cpus:          allCPUs,
		})
		parent = filepath.Join(parent, dir)
	}

	var childState *desiredManagerCPUSetState
	ctrCgroupCPUs := exclusiveCPUs
	if sharedCPUsRequested {
		// the child cgroup already created earlier by setSharedCPUs()
		childCgroup, err := getManagerByIndex(len(containerManagers)-1, containerManagers)
		if err != nil {
			return err
		}
		sharedCPUSet, err := cpuset.Parse(h.sharedCPUs)
		if err != nil {
			return fmt.Errorf("failed to parse shared cpus: %w", err)
		}
		childState = &desiredManagerCPUSetState{
			manager:       childCgroup,
			exclusiveCPUs: exclusiveCPUs,
			cpus:          exclusiveCPUs,
		}
		// The container cgroup's cpuset.cpu should be the exclusive and the shared
		// A manager process will move the exclusive process into the sub cgroup, which will
		// only have the exclusiveCPUs, and be isolated
		ctrCgroupCPUs = exclusiveCPUs.Union(sharedCPUSet)
	}
	for _, mgr := range containerManagers {
		managers = append(managers, &desiredManagerCPUSetState{
			manager:       mgr,
			exclusiveCPUs: exclusiveCPUs,
			cpus:          ctrCgroupCPUs,
		})
	}
	if childState != nil {
		managers[len(managers)-1] = childState
	}

	if len(managers) == 0 {
		return errors.New("cgroup hierarchy setup unexpectedly, no cgroups of container found")
	}

	// Revert changes made to avoid weird error states.
	// Changes are applied in reverse, so hopefully all the changes are reverted correctly.
	defer func() {
		if retErr != nil {
			if err = h.addOrRemoveCpusetFromManagers(managers, enable); err != nil {
				log.Errorf(context.Background(), "Failed to revert cpuset values: %v", err)
			}
		}
	}()
	if err := h.addOrRemoveCpusetFromManagers(managers, !enable); err != nil {
		return err
	}

	// If re-enabling load balancing, then no need to write "isolated" to the cgroup.
	// It should be cleaned up soon anyway.
	if enable {
		return nil
	}
	// The last entry is the actual container cgroup, so write to it directly to finish the work.
	return cgroups.WriteFile(managers[len(managers)-1].manager.Path(""), "cpuset.cpus.partition", "isolated")
}

func (h *HighPerformanceHooks) addOrRemoveCpusetFromManagers(states []*desiredManagerCPUSetState, add bool) error {
	// Adding, we go top to bottom, when removing, bottom to top.
	if !add {
		slices.Reverse(states)
	}
	for _, state := range states {
		// If we're updating a cpuset to be exclusive, we need to set cpuset.cpus first
		if add {
			// If the cgroup is shared among exclusive and not exclusive cgroups, then
			// we should write the full cpuset. This allows the kernel to toggle the cpuset
			// based on whether the cgroup uses partition mode "isolated", and ensures children
			// cgroups that don't have load balance disabled can use the remaining cpus.
			if err := h.addOrRemoveCpusetFromManager(state.manager, state.cpus, add, cpusetCpus); err != nil {
				return err
			}
		}

		// Unconditionally update cpuset.cpus.exclusive, as this file must contain any cpus that we intend to isolate.
		if err := h.addOrRemoveCpusetFromManager(state.manager, state.exclusiveCPUs, add, cpusetCpusExclusive); err != nil {
			return err
		}

		// If we're removing, we need to remove from cpuset.cpus after it's been removed from cpuset.cpus.exclusive
		// Don't modify cpuset.cpus if it contains non-exclusive cpus.
		// Not only is there no need, it doesn't even work because that cpuset has been written to the other children by the kernel.
		if !add && state.exclusiveCPUs.Equals(state.cpus) {
			if err := h.addOrRemoveCpusetFromManager(state.manager, state.cpus, add, cpusetCpus); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *HighPerformanceHooks) addOrRemoveCpusetFromManager(mgr cgroups.Manager, cpus cpuset.CPUSet, add bool, file string) error {
	h.cpusetLock.Lock()
	defer h.cpusetLock.Unlock()

	currentCpusStr, err := cgroups.ReadFile(mgr.Path(""), file)
	if err != nil {
		return err
	}
	currentCpus, err := cpuset.Parse(strings.TrimSpace(currentCpusStr))
	if err != nil {
		return err
	}

	var targetCpus cpuset.CPUSet
	if add {
		targetCpus = currentCpus.Union(cpus)
	} else {
		targetCpus = currentCpus.Difference(cpus)
	}

	if targetCpus.Equals(currentCpus) {
		return nil
	}

	// if we're writing to cpuset.cpus.exclusive, libcontainer manager doesn't have a field to manage it,
	// so write to a file instead.
	if file == cpusetCpusExclusive {
		// For some reason, just writing the empty string doesn't work
		toWrite := targetCpus.String()
		if toWrite == "" {
			toWrite = "\n"
		}
		return cgroups.WriteFile(mgr.Path(""), file, toWrite)
	}
	// otherwise, we should use the mgr directly, as it will go through systemd if necessary
	return mgr.Set(&configs.Resources{
		SkipDevices: true,
		CpusetCpus:  targetCpus.String(),
	})
}

var (
	fullCPUSetOnce sync.Once
	fullCPUSetErr  error
	fullCPUSetVar  cpuset.CPUSet
)

// fullCPUSet returns the node's full CPUSet, which is used to populate the shared cgroups.
// Do this just once. It's not particularly inefficient, but there's no need to recalculate.
func fullCPUSet() (cpuset.CPUSet, error) {
	fullCPUSetOnce.Do(func() {
		content, err := os.ReadFile(filepath.Join(sysCPUDir, "online"))
		if err != nil {
			fullCPUSetErr = err
			return
		}
		fullCPUSetVar, fullCPUSetErr = cpuset.Parse(strings.TrimSpace(string(content)))
	})
	return fullCPUSetVar, fullCPUSetErr
}

// The requisite condition to allow this is `cpuset.sched_load_balance` field must be set to 0 for all cgroups
// that intersect with `cpuset.cpus` of the container that desires load balancing.
// Since CRI-O is the owner of the container cgroup, it must set this value for
// the container. Some other entity (kubelet, external service) must ensure this is the case for all
// other cgroups that intersect (at minimum: all parent cgroups of this cgroup).
func disableCPULoadBalancingV1(containerManagers []cgroups.Manager) error {
	for i := len(containerManagers) - 1; i >= 0; i-- {
		cpusetPath := containerManagers[i].Path("cpuset")
		if err := cgroups.WriteFile(cpusetPath, "cpuset.sched_load_balance", "0"); err != nil {
			return err
		}
	}
	return nil
}

func setIRQLoadBalancing(ctx context.Context, c *oci.Container, enable bool, irqSmpAffinityFile, irqBalanceConfigFile string) error {
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
			log.Warnf(ctx, "Irqbalance binary not found: %v", err)
			return nil
		}
		// run irqbalance in daemon mode, so this won't cause delay
		cmd := cmdrunner.Command(irqBalancedName, "--oneshot")
		additionalEnv := irqBalanceBannedCpus + "=" + newIRQBalanceSetting
		cmd.Env = append(os.Environ(), additionalEnv)
		return cmd.Run()
	}

	if err := restartIrqBalanceService(); err != nil {
		log.Warnf(ctx, "Irqbalance service restart failed: %v", err)
	}
	return nil
}

func setCPUQuota(podManager cgroups.Manager, containerManagers []cgroups.Manager) error {
	if err := disableCPUQuotaForCgroup(podManager); err != nil {
		return err
	}
	for _, containerManager := range containerManagers {
		if err := disableCPUQuotaForCgroup(containerManager); err != nil {
			return err
		}
	}
	return nil
}

func libctrManagersForPodAndContainerCgroup(c *oci.Container, parentDir string) (podManager cgroups.Manager, containerManagers []cgroups.Manager, _ error) {
	var (
		cgroupManager cgmgr.CgroupManager
		err           error
	)

	if strings.HasSuffix(parentDir, ".slice") {
		if cgroupManager, err = cgmgr.SetCgroupManager("systemd"); err != nil {
			// Programming error, this is only possible if the manager string is invalid.
			panic(err)
		}
	} else if cgroupManager, err = cgmgr.SetCgroupManager("cgroupfs"); err != nil {
		// Programming error, this is only possible if the manager string is invalid.
		panic(err)
	}

	containerCgroupFullPath, err := cgroupManager.ContainerCgroupAbsolutePath(parentDir, c.ID())
	if err != nil {
		return nil, nil, err
	}

	podCgroupFullPath := filepath.Dir(containerCgroupFullPath)
	podManager, err = libctrManager(filepath.Base(podCgroupFullPath), filepath.Dir(podCgroupFullPath), cgroupManager.IsSystemd())
	if err != nil {
		return nil, nil, err
	}

	containerCgroup := filepath.Base(containerCgroupFullPath)
	// A quirk of libcontainer's cgroup driver.
	if cgroupManager.IsSystemd() {
		containerCgroup = c.ID()
	}

	containerManager, err := libctrManager(containerCgroup, filepath.Dir(containerCgroupFullPath), cgroupManager.IsSystemd())
	if err != nil {
		return nil, nil, err
	}
	containerManagers = []cgroups.Manager{containerManager}

	// crun actually does the cgroup configuration in a child of the cgroup CRI-O expects to be the container's
	extraManager, err := trueContainerCgroupManager(containerCgroupFullPath)
	if err != nil {
		return nil, nil, err
	}
	if extraManager != nil {
		containerManagers = append(containerManagers, extraManager)
	}
	return podManager, containerManagers, nil
}

func trueContainerCgroupManager(expectedContainerCgroup string) (cgroups.Manager, error) {
	// HACK: There isn't really a better way to check if the actual container cgroup is in a child cgroup of the expected.
	// We could check /proc/$pid/cgroup, but we need to be able to query this after the container exits and the process is gone.
	// We know the source of this: crun creates a sub cgroup of the container to do the actual management, to enforce systemd's single
	// owner rule. Thus, we need to hardcode this check.
	actualContainerCgroup := filepath.Join(expectedContainerCgroup, "container")
	// Choose cpuset as the cgroup to check, with little reason.
	cgroupRoot := cgroupMountPoint
	if !node.CgroupIsV2() {
		cgroupRoot += "/cpuset"
	}
	if _, err := os.Stat(filepath.Join(cgroupRoot, actualContainerCgroup)); err != nil {
		return nil, nil
	}
	// must be crun, make another libctrManager. Regardless of cgroup driver, it will be treated as cgroupfs
	return libctrManager(filepath.Base(actualContainerCgroup), filepath.Dir(actualContainerCgroup), false)
}

func disableCPUQuotaForCgroup(mgr cgroups.Manager) error {
	return mgr.Set(&configs.Resources{
		SkipDevices: true,
		CpuQuota:    -1,
	})
}

func libctrManager(cgroup, parent string, systemd bool) (cgroups.Manager, error) {
	if systemd {
		parent = filepath.Base(parent)
		if parent == "." {
			// libcontainer shorthand for root
			// see https://github.com/opencontainers/runc/blob/9fffadae8/libcontainer/cgroups/systemd/common.go#L71
			parent = "-.slice"
		}
	}
	cg := &configs.Cgroup{
		Name:   cgroup,
		Parent: parent,
		Resources: &configs.Resources{
			SkipDevices: true,
		},
		Systemd: systemd,
		// If the cgroup manager is systemd, then libcontainer
		// will construct the cgroup path (for scopes) as:
		// ScopePrefix-Name.scope. For slices, and for cgroupfs manager,
		// this will be ignored.
		// See: https://github.com/opencontainers/runc/tree/main/libcontainer/cgroups/systemd/common.go:getUnitName
		ScopePrefix: cgmgr.CrioPrefix,
	}
	return libCtrMgr.New(cg)
}

// safe fetch of cgroup manager from managers slice
func getManagerByIndex(idx int, containerManagers []cgroups.Manager) (cgroups.Manager, error) {
	length := len(containerManagers)
	if length == 0 {
		return nil, errors.New("getManagerByIndex: no cgroup manager were found")
	}
	if length-1 < idx || idx < 0 {
		return nil, errors.New("getManagerByIndex: invalid index")
	}
	return containerManagers[idx], nil
}

// setCPUPMQOSResumeLatency sets the pm_qos_resume_latency_us for a cpu and stores the original
// value so it can be restored later. If the latency is an empty string, the original latency
// value is restored.
func setCPUPMQOSResumeLatency(c *oci.Container, latency string) error {
	return doSetCPUPMQOSResumeLatency(c, latency, sysCPUDir, sysCPUSaveDir)
}

// doSetCPUPMQOSResumeLatency facilitates unit testing by allowing the directories to be specified as parameters.
func doSetCPUPMQOSResumeLatency(c *oci.Container, latency, cpuDir, cpuSaveDir string) error {
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

	for _, cpu := range cpus.List() {
		latencyFile := fmt.Sprintf("%s/cpu%d/power/pm_qos_resume_latency_us", cpuDir, cpu)
		cpuPowerSaveDir := fmt.Sprintf("%s/cpu%d/power", cpuSaveDir, cpu)
		latencyFileOrig := path.Join(cpuPowerSaveDir, "pm_qos_resume_latency_us")

		if latency != "" {
			// Don't overwrite the original latency if it has already been saved. This can happen if
			// a container is restarted, as this will cause the PreStart hooks to be called again.
			if !fileExists(latencyFileOrig) {
				// Retrieve the current latency.
				latencyOrig, err := os.ReadFile(latencyFile)
				if err != nil {
					return err
				}

				// Save the current latency so we can restore it later.
				err = os.MkdirAll(cpuPowerSaveDir, 0o750)
				if err != nil {
					return err
				}
				err = os.WriteFile(latencyFileOrig, latencyOrig, 0o644)
				if err != nil {
					return err
				}
			}

			// Update the pm_qos_resume_latency_us.
			err = os.WriteFile(latencyFile, []byte(latency), 0o644)
			if err != nil {
				return err
			}

			continue
		}

		// Retrieve the original latency.
		latencyOrig, err := os.ReadFile(latencyFileOrig)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// The latency may have already been restored by a previous invocation of the hook.
				return nil
			}
			return err
		}

		// Restore the original latency.
		err = os.WriteFile(latencyFile, latencyOrig, 0o644)
		if err != nil {
			return err
		}

		// Remove the saved latency.
		err = os.Remove(latencyFileOrig)
		if err != nil {
			return err
		}
	}

	return nil
}

// isCPUGovernorSupported checks whether the cpu governor is supported for the specified cpu.
func isCPUGovernorSupported(governor, cpuDir string, cpu int) error {
	// Get available cpu scaling governors.
	availGovernorFile := fmt.Sprintf("%s/cpu%d/cpufreq/scaling_available_governors", cpuDir, cpu)
	availGovernors, err := os.ReadFile(availGovernorFile)
	if err != nil {
		return err
	}

	// Is the scaling governor supported?
	for _, availableGovernor := range strings.Fields(string(availGovernors)) {
		if availableGovernor == governor {
			return nil
		}
	}

	return fmt.Errorf("governor %s not available for cpu %d", governor, cpu)
}

// setCPUFreqGovernor sets the scaling_governor for a cpu and stores the original
// value so it can be restored later. If the governor is an empty string, the original
// scaling_governor value is restored.
func setCPUFreqGovernor(c *oci.Container, governor string) error {
	return doSetCPUFreqGovernor(c, governor, sysCPUDir, sysCPUSaveDir)
}

// doSetCPUFreqGovernor facilitates unit testing by allowing the directories to be specified as parameters.
func doSetCPUFreqGovernor(c *oci.Container, governor, cpuDir, cpuSaveDir string) error {
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

	for _, cpu := range cpus.List() {
		governorFile := fmt.Sprintf("%s/cpu%d/cpufreq/scaling_governor", cpuDir, cpu)
		cpuFreqSaveDir := fmt.Sprintf("%s/cpu%d/cpufreq", cpuSaveDir, cpu)
		governorFileOrig := path.Join(cpuFreqSaveDir, "scaling_governor")

		if governor != "" {
			// Is the new scaling governor supported?
			if err := isCPUGovernorSupported(governor, cpuDir, cpu); err != nil {
				return err
			}

			// Don't overwrite the original governor if it has already been saved. This can happen if
			// a container is restarted, as this will cause the PreStart hooks to be called again.
			if !fileExists(governorFileOrig) {
				// Retrieve the current scaling governor.
				governorOrig, err := os.ReadFile(governorFile)
				if err != nil {
					return err
				}

				// Save the current governor so we can restore it later.
				err = os.MkdirAll(cpuFreqSaveDir, 0o750)
				if err != nil {
					return err
				}
				err = os.WriteFile(governorFileOrig, governorOrig, 0o644)
				if err != nil {
					return err
				}
			}

			// Update the governor.
			err = os.WriteFile(governorFile, []byte(governor), 0o644)
			if err != nil {
				return err
			}

			continue
		}

		// Retrieve the original scaling governor.
		governorOrig, err := os.ReadFile(governorFileOrig)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// The governor may have already been restored by a previous invocation of the hook.
				return nil
			}
			return err
		}

		// Restore the original governor.
		err = os.WriteFile(governorFile, governorOrig, 0o644)
		if err != nil {
			return err
		}

		// Remove the saved governor.
		err = os.Remove(governorFileOrig)
		if err != nil {
			return err
		}
	}

	return nil
}

// RestoreIrqBalanceConfig restores irqbalance service with original banned cpu mask settings
func RestoreIrqBalanceConfig(ctx context.Context, irqBalanceConfigFile, irqBannedCPUConfigFile, irqSmpAffinityProcFile string) error {
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
		log.Infof(ctx, "Restore irqbalance config: not system reboot, ignoring")
		return nil
	}

	bannedCPUMasks, err := retrieveIrqBannedCPUMasks(irqBalanceConfigFile)
	if err != nil {
		// Ignore returning err as given irqBalanceConfigFile may not exist.
		log.Infof(ctx, "Restore irqbalance config: failed to get current CPU ban list, ignoring")
		return nil
	}

	if !fileExists(irqBannedCPUConfigFile) {
		log.Infof(ctx, "Creating banned CPU list file %q", irqBannedCPUConfigFile)
		irqBannedCPUsConfig, err := os.Create(irqBannedCPUConfigFile)
		if err != nil {
			return err
		}
		defer irqBannedCPUsConfig.Close()
		_, err = irqBannedCPUsConfig.WriteString(bannedCPUMasks)
		if err != nil {
			return err
		}
		log.Infof(ctx, "Restore irqbalance config: created backup file")
		return nil
	}

	content, err = os.ReadFile(irqBannedCPUConfigFile)
	if err != nil {
		return err
	}
	origBannedCPUMasks := strings.TrimSpace(string(content))

	if bannedCPUMasks == origBannedCPUMasks {
		log.Infof(ctx, "Restore irqbalance config: nothing to do")
		return nil
	}

	log.Infof(ctx, "Restore irqbalance banned CPU list in %q to %q", irqBalanceConfigFile, origBannedCPUMasks)
	if err := updateIrqBalanceConfigFile(irqBalanceConfigFile, origBannedCPUMasks); err != nil {
		return err
	}
	if isServiceEnabled(irqBalancedName) {
		if err := restartIrqBalanceService(); err != nil {
			log.Warnf(ctx, "Irqbalance service restart failed: %v", err)
		}
	}
	return nil
}

func ShouldCPUQuotaBeDisabled(ctx context.Context, cid string, cSpec *specs.Spec, s *sandbox.Sandbox, annotations fields.Set) bool {
	if !shouldRunHooks(ctx, cid, cSpec, s) {
		return false
	}
	if annotations[crioannotations.CPUQuotaAnnotation] == annotationTrue {
		log.Warnf(context.TODO(), annotationValueDeprecationWarning(crioannotations.CPUQuotaAnnotation))
	}

	return annotations[crioannotations.CPUQuotaAnnotation] == annotationTrue ||
		annotations[crioannotations.CPUQuotaAnnotation] == annotationDisable
}

func shouldRunHooks(ctx context.Context, id string, cSpec *specs.Spec, s *sandbox.Sandbox) bool {
	if isCgroupParentBurstable(s) {
		log.Infof(ctx, "Container %q is a burstable pod. Skip PreStart.", id)
		return false
	}
	if isCgroupParentBestEffort(s) {
		log.Infof(ctx, "Container %q is a besteffort pod. Skip PreStart.", id)
		return false
	}
	if !isContainerRequestWholeCPU(cSpec) {
		log.Infof(ctx, "Container %q requests partial cpu(s). Skip PreStart", id)
		return false
	}
	return true
}

func isCgroupParentBurstable(s *sandbox.Sandbox) bool {
	return strings.Contains(s.CgroupParent(), "burstable")
}

func isCgroupParentBestEffort(s *sandbox.Sandbox) bool {
	return strings.Contains(s.CgroupParent(), "besteffort")
}

func isContainerRequestWholeCPU(cSpec *specs.Spec) bool {
	return *(cSpec.Linux.Resources.CPU.Shares)%1024 == 0
}

// convertAnnotationToLatency converts the cpu-c-states.crio.io annotation to a maximum
// latency value in microseconds.
//
// The cpu-c-states.crio.io annotation can be used to control c-states in several ways:
//
//	enable: enable all c-states (cpu-c-states.crio.io: "enable")
//	disable: disable all c-states (cpu-c-states.crio.io: "disable")
//	max_latency: enable c-states with a maximum latency in microseconds
//	             (for example,  cpu-c-states.crio.io: "max_latency:10")
//
// Examples:
//
// cpu-c-states.crio.io: "disable" (disable all c-states)
// cpu-c-states.crio.io: "enable" (enable all c-states)
// cpu-c-states.crio.io: "max_latency:10" (use a max latency of 10us)
func convertAnnotationToLatency(annotation string) (maxLatency string, err error) {
	//nolint:gocritic // this would not be better as a switch statement
	if annotation == annotationEnable {
		// Enable all c-states.
		return "0", nil
	} else if annotation == annotationDisable {
		// Disable all c-states.
		return "n/a", nil //nolint:goconst // there are not 4 occurrences of this string
	} else if strings.HasPrefix(annotation, "max_latency:") {
		// Use the latency provided
		latency, err := strconv.Atoi(strings.TrimPrefix(annotation, "max_latency:"))
		if err != nil {
			return "", err
		}

		// Latency must be greater than 0
		if latency > 0 {
			return strconv.Itoa(latency), nil
		}
	}

	return "", fmt.Errorf("invalid annotation value %s", annotation)
}

func setSharedCPUs(c *oci.Container, containerManagers []cgroups.Manager, sharedCPUs string) ([]cgroups.Manager, error) {
	cSpec := c.Spec()
	if isContainerCPUsSpecEmpty(&cSpec) {
		return nil, fmt.Errorf("no cpus found for container %q", c.Name())
	}
	cpusString := cSpec.Linux.Resources.CPU.Cpus
	exclusiveCPUs, err := cpuset.Parse(cpusString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse container %q cpus: %w", c.Name(), err)
	}
	if sharedCPUs == "" {
		return nil, fmt.Errorf("shared CPUs were requested for container %q but none are defined", c.Name())
	}
	sharedCPUSet, err := cpuset.Parse(sharedCPUs)
	if err != nil {
		return nil, fmt.Errorf("failed to parse shared cpus: %w", err)
	}
	ctrManager, err := getManagerByIndex(len(containerManagers)-1, containerManagers)
	if err != nil {
		return nil, err
	}
	if err := ctrManager.Set(&configs.Resources{
		SkipDevices: true,
		CpusetCpus:  exclusiveCPUs.Union(sharedCPUSet).String(),
	}); err != nil {
		return nil, err
	}
	if node.CgroupIsV2() {
		// we need to move the isolated cpus into a separate child cgroup
		// on V2 all controllers are under the same path
		ctrCgroup := ctrManager.Path("")
		if err := cgroups.WriteFile(ctrCgroup, cgroupSubTreeControl, "+cpu +cpuset"); err != nil {
			return nil, err
		}
		// create a new cgroupfs manager
		childCgroup, err := libctrManager("cgroup-child", strings.TrimPrefix(ctrCgroup, cgroupMountPoint), false)
		if err != nil {
			return nil, err
		}
		if err := childCgroup.Apply(-1); err != nil {
			return nil, err
		}
		// add the exclusive cpus under the child cgroup in case
		// this makes the handling of load-balancing disablement simpler in case it required
		if err := childCgroup.Set(&configs.Resources{
			SkipDevices: true,
			CpusetCpus:  exclusiveCPUs.String(),
		}); err != nil {
			return nil, err
		}
		containerManagers = append(containerManagers, childCgroup)
	}
	// here we return the containerManagers with the child cgroup inside
	// this is required in case load-balancing disablement is requested for the pod
	return containerManagers, nil
}

func isContainerCPUsSpecEmpty(spec *specs.Spec) bool {
	return spec.Linux == nil ||
		spec.Linux.Resources == nil ||
		spec.Linux.Resources.CPU == nil ||
		spec.Linux.Resources.CPU.Cpus == ""
}

func injectQuotaGivenSharedCPUs(c *oci.Container, podManager cgroups.Manager, containerManagers []cgroups.Manager, sharedCPUs string) error {
	cpuSpec := c.Spec().Linux.Resources.CPU
	isolatedCPUSet, err := cpuset.Parse(cpuSpec.Cpus)
	if err != nil {
		return fmt.Errorf("failed to parse container %q cpus: %w", c.Name(), err)
	}
	sharedCPUSet, err := cpuset.Parse(sharedCPUs)
	if err != nil {
		return fmt.Errorf("failed to parse shared cpus: %w", err)
	}
	if sharedCPUSet.IsEmpty() {
		return errors.New("shared CPU set is empty")
	}

	// pod level operations
	newPodQuota, err := calculatePodQuota(&sharedCPUSet, podManager, *cpuSpec.Period)
	if err != nil {
		return fmt.Errorf("failed to calculate pod quota: %w", err)
	}
	// the Set function knows to handle -1 value for both v1 and v2
	err = podManager.Set(&configs.Resources{
		SkipDevices: true,
		CpuQuota:    newPodQuota,
	})
	if err != nil {
		return err
	}
	// container level operations
	ctrCPUSet := isolatedCPUSet.Union(sharedCPUSet)
	ctrQuota, err := calculateMaximalQuota(&ctrCPUSet, *cpuSpec.Period)
	if err != nil {
		return fmt.Errorf("failed to calculate container %s quota: %w", c.ID(), err)
	}
	manager, err := getManagerByIndex(len(containerManagers)-1, containerManagers)
	if err != nil {
		return err
	}
	return manager.Set(&configs.Resources{
		SkipDevices: true,
		CpuQuota:    ctrQuota,
	})
}

func calculateMaximalQuota(cpus *cpuset.CPUSet, period uint64) (quota int64, err error) {
	quan, err := resource.ParseQuantity(strconv.Itoa(cpus.Size()))
	if err != nil {
		return
	}
	// after we divide in milliCPUToCPU, it's safe to convert into int64
	quota = int64((uint64(quan.MilliValue()) * period) / milliCPUToCPU)
	return
}

func calculatePodQuota(sharedCpus *cpuset.CPUSet, podManager cgroups.Manager, period uint64) (int64, error) {
	var quotaGetFunc func(cgroups.Manager) (string, error)
	if node.CgroupIsV2() {
		quotaGetFunc = getPodQuotaV2
	} else {
		quotaGetFunc = getPodQuotaV1
	}
	existingQuota, err := quotaGetFunc(podManager)
	if err != nil {
		return 0, err
	}
	// the pod is already at its maximal quota
	// we return -1 for both cgroup v1 and v2
	if existingQuota == "-1" || existingQuota == "max" {
		return -1, nil
	}
	additionalQuota, err := calculateMaximalQuota(sharedCpus, period)
	if err != nil {
		return 0, err
	}
	q, err := strconv.ParseInt(existingQuota, 10, 0)
	if err != nil {
		return 0, err
	}
	return q + additionalQuota, err
}

func getPodQuotaV1(mng cgroups.Manager) (string, error) {
	controllerPath := mng.Path("cpu")
	q, err := cgroups.ReadFile(controllerPath, cgroupV1QuotaFile)
	if err != nil {
		return "", err
	}
	cpuQuota := strings.TrimSuffix(q, "\n")
	return cpuQuota, nil
}

func getPodQuotaV2(mng cgroups.Manager) (string, error) {
	controllerPath := mng.Path("")
	cpuQuotaAndPeriod, err := cgroups.ReadFile(controllerPath, cgroupV2QuotaFile)
	if err != nil {
		return "", err
	}
	// in v2, the quota file contains both quota and period
	// example: max 100000
	cpuQuota := strings.Split(strings.TrimSuffix(cpuQuotaAndPeriod, "\n"), " ")[0]
	return cpuQuota, nil
}

func injectCpusetEnv(specgen *generate.Generator, isolated, shared *cpuset.CPUSet) {
	spec := specgen.Config
	spec.Process.Env = append(spec.Process.Env,
		fmt.Sprintf("%s=%s", IsolatedCPUsEnvVar, isolated.String()),
		fmt.Sprintf("%s=%s", SharedCPUsEnvVar, shared.String()))
}
