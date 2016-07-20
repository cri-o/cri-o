// Package generate implements functions generating container config files.
package generate

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/syndtr/gocapability/capability"
)

var (
	// Namespaces include the names of supported namespaces.
	Namespaces = []string{"network", "pid", "mount", "ipc", "uts", "user", "cgroup"}
)

// Generator represents a generator for a container spec.
type Generator struct {
	spec *rspec.Spec
}

// New creates a spec Generator with the default spec.
func New() Generator {
	spec := rspec.Spec{
		Version: rspec.Version,
		Platform: rspec.Platform{
			OS:   runtime.GOOS,
			Arch: runtime.GOARCH,
		},
		Root: rspec.Root{
			Path:     "",
			Readonly: false,
		},
		Process: rspec.Process{
			Terminal: false,
			User:     rspec.User{},
			Args: []string{
				"sh",
			},
			Env: []string{
				"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
				"TERM=xterm",
			},
			Cwd: "/",
			Capabilities: []string{
				"CAP_CHOWN",
				"CAP_DAC_OVERRIDE",
				"CAP_FSETID",
				"CAP_FOWNER",
				"CAP_MKNOD",
				"CAP_NET_RAW",
				"CAP_SETGID",
				"CAP_SETUID",
				"CAP_SETFCAP",
				"CAP_SETPCAP",
				"CAP_NET_BIND_SERVICE",
				"CAP_SYS_CHROOT",
				"CAP_KILL",
				"CAP_AUDIT_WRITE",
			},
			Rlimits: []rspec.Rlimit{
				{
					Type: "RLIMIT_NOFILE",
					Hard: uint64(1024),
					Soft: uint64(1024),
				},
			},
		},
		Hostname: "mrsdalloway",
		Mounts: []rspec.Mount{
			{
				Destination: "/proc",
				Type:        "proc",
				Source:      "proc",
				Options:     nil,
			},
			{
				Destination: "/dev",
				Type:        "tmpfs",
				Source:      "tmpfs",
				Options:     []string{"nosuid", "strictatime", "mode=755", "size=65536k"},
			},
			{
				Destination: "/dev/pts",
				Type:        "devpts",
				Source:      "devpts",
				Options:     []string{"nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620", "gid=5"},
			},
			{
				Destination: "/dev/shm",
				Type:        "tmpfs",
				Source:      "shm",
				Options:     []string{"nosuid", "noexec", "nodev", "mode=1777", "size=65536k"},
			},
			{
				Destination: "/dev/mqueue",
				Type:        "mqueue",
				Source:      "mqueue",
				Options:     []string{"nosuid", "noexec", "nodev"},
			},
			{
				Destination: "/sys",
				Type:        "sysfs",
				Source:      "sysfs",
				Options:     []string{"nosuid", "noexec", "nodev", "ro"},
			},
		},
		Linux: &rspec.Linux{
			Resources: &rspec.Resources{
				Devices: []rspec.DeviceCgroup{
					{
						Allow:  false,
						Access: strPtr("rwm"),
					},
				},
			},
			Namespaces: []rspec.Namespace{
				{
					Type: "pid",
				},
				{
					Type: "network",
				},
				{
					Type: "ipc",
				},
				{
					Type: "uts",
				},
				{
					Type: "mount",
				},
			},
			Devices: []rspec.Device{},
		},
	}
	return Generator{&spec}
}

// NewFromSpec creates a spec Generator from a given spec.
func NewFromSpec(spec *rspec.Spec) Generator {
	return Generator{spec}
}

// NewFromFile loads the template specifed in a file into a spec Generator.
func NewFromFile(path string) (Generator, error) {
	cf, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Generator{}, fmt.Errorf("template configuration at %s not found", path)
		}
	}
	defer cf.Close()

	return NewFromTemplate(cf)
}

// NewFromTemplate loads the template from io.Reader into a spec Generator.
func NewFromTemplate(r io.Reader) (Generator, error) {
	var spec rspec.Spec
	if err := json.NewDecoder(r).Decode(&spec); err != nil {
		return Generator{}, err
	}
	return Generator{&spec}, nil
}

// SetSpec sets the spec in the Generator g.
func (g Generator) SetSpec(spec *rspec.Spec) {
	g.spec = spec
}

// GetSpec gets the spec in the Generator g.
func (g Generator) GetSpec() *rspec.Spec {
	return g.spec
}

// Save writes the spec into w.
func (g Generator) Save(w io.Writer) error {
	data, err := json.MarshalIndent(g.spec, "", "\t")
	if err != nil {
		return err
	}

	_, err = w.Write(data)
	if err != nil {
		return err
	}

	return nil
}

// SaveToFile writes the spec into a file.
func (g Generator) SaveToFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return g.Save(f)
}

// SetVersion sets g.spec.Version.
func (g Generator) SetVersion(version string) {
	g.spec.Version = version
}

// SetRootPath sets g.spec.Root.Path.
func (g Generator) SetRootPath(path string) {
	g.spec.Root.Path = path
}

// SetRootReadonly sets g.spec.Root.Readonly.
func (g Generator) SetRootReadonly(b bool) {
	g.spec.Root.Readonly = b
}

// SetHostname sets g.spec.Hostname.
func (g Generator) SetHostname(s string) {
	g.spec.Hostname = s
}

// ClearAnnotations clears g.spec.Annotations.
func (g Generator) ClearAnnotations() {
	g.spec.Annotations = make(map[string]string)
}

// AddAnnotation adds an annotation into g.spec.Annotations.
func (g Generator) AddAnnotation(s string) error {
	if g.spec.Annotations == nil {
		g.spec.Annotations = make(map[string]string)
	}

	pair := strings.Split(s, "=")
	if len(pair) != 2 {
		return fmt.Errorf("incorrectly specified annotation: %s", s)
	}
	g.spec.Annotations[pair[0]] = pair[1]
	return nil
}

// RemoveAnnotation remove an annotation from g.spec.Annotations.
func (g Generator) RemoveAnnotation(key string) {
	if g.spec.Annotations == nil {
		return
	}
	delete(g.spec.Annotations, key)
}

// SetPlatformOS sets g.spec.Process.OS.
func (g Generator) SetPlatformOS(os string) {
	g.spec.Platform.OS = os
}

// SetPlatformArch sets g.spec.Platform.Arch.
func (g Generator) SetPlatformArch(arch string) {
	g.spec.Platform.Arch = arch
}

// SetProcessUID sets g.spec.Process.User.UID.
func (g Generator) SetProcessUID(uid uint32) {
	g.spec.Process.User.UID = uid
}

// SetProcessGID sets g.spec.Process.User.GID.
func (g Generator) SetProcessGID(gid uint32) {
	g.spec.Process.User.GID = gid
}

// SetProcessCwd sets g.spec.Process.Cwd.
func (g Generator) SetProcessCwd(cwd string) {
	g.spec.Process.Cwd = cwd
}

// SetProcessNoNewPrivileges sets g.spec.Process.NoNewPrivileges.
func (g Generator) SetProcessNoNewPrivileges(b bool) {
	g.spec.Process.NoNewPrivileges = b
}

// SetProcessTerminal sets g.spec.Process.Terminal.
func (g Generator) SetProcessTerminal(b bool) {
	g.spec.Process.Terminal = b
}

// SetProcessApparmorProfile sets g.spec.Process.ApparmorProfile.
func (g Generator) SetProcessApparmorProfile(prof string) {
	g.spec.Process.ApparmorProfile = prof
}

// SetProcessArgs sets g.spec.Process.Args.
func (g Generator) SetProcessArgs(args []string) {
	g.spec.Process.Args = args
}

// ClearProcessEnv clears g.spec.Process.Env.
func (g Generator) ClearProcessEnv() {
	g.spec.Process.Env = []string{}
}

// AddProcessEnv adds env into g.spec.Process.Env.
func (g Generator) AddProcessEnv(env string) {
	g.spec.Process.Env = append(g.spec.Process.Env, env)
}

// ClearProcessAdditionalGids clear g.spec.Process.AdditionalGids.
func (g Generator) ClearProcessAdditionalGids() {
	g.spec.Process.User.AdditionalGids = []uint32{}
}

// AddProcessAdditionalGid adds an additional gid into g.spec.Process.AdditionalGids.
func (g Generator) AddProcessAdditionalGid(gid string) error {
	groupID, err := strconv.Atoi(gid)
	if err != nil {
		return err
	}

	for _, group := range g.spec.Process.User.AdditionalGids {
		if group == uint32(groupID) {
			return nil
		}
	}
	g.spec.Process.User.AdditionalGids = append(g.spec.Process.User.AdditionalGids, uint32(groupID))
	return nil
}

// SetProcessSelinuxLabel sets g.spec.Process.SelinuxLabel.
func (g Generator) SetProcessSelinuxLabel(label string) {
	g.spec.Process.SelinuxLabel = label
}

// SetLinuxCgroupsPath sets g.spec.Linux.CgroupsPath.
func (g Generator) SetLinuxCgroupsPath(path string) {
	g.spec.Linux.CgroupsPath = strPtr(path)
}

// SetLinuxMountLabel sets g.spec.Linux.MountLabel.
func (g Generator) SetLinuxMountLabel(label string) {
	g.spec.Linux.MountLabel = label
}

// SetLinuxResourcesCPUShares sets g.spec.Linux.Resources.CPU.Shares.
func (g Generator) SetLinuxResourcesCPUShares(shares uint64) {
	g.spec.Linux.Resources.CPU.Shares = &shares
}

// SetLinuxResourcesCPUQuota sets g.spec.Linux.Resources.CPU.Quota.
func (g Generator) SetLinuxResourcesCPUQuota(quota uint64) {
	g.spec.Linux.Resources.CPU.Quota = &quota
}

// SetLinuxResourcesCPUPeriod sets g.spec.Linux.Resources.CPU.Period.
func (g Generator) SetLinuxResourcesCPUPeriod(period uint64) {
	g.spec.Linux.Resources.CPU.Period = &period
}

// SetLinuxResourcesCPURealtimeRuntime sets g.spec.Linux.Resources.CPU.RealtimeRuntime.
func (g Generator) SetLinuxResourcesCPURealtimeRuntime(time uint64) {
	g.spec.Linux.Resources.CPU.RealtimeRuntime = &time
}

// SetLinuxResourcesCPURealtimePeriod sets g.spec.Linux.Resources.CPU.RealtimePeriod.
func (g Generator) SetLinuxResourcesCPURealtimePeriod(period uint64) {
	g.spec.Linux.Resources.CPU.RealtimePeriod = &period
}

// SetLinuxResourcesCPUCpus sets g.spec.Linux.Resources.CPU.Cpus.
func (g Generator) SetLinuxResourcesCPUCpus(cpus string) {
	g.spec.Linux.Resources.CPU.Cpus = &cpus
}

// SetLinuxResourcesCPUMems sets g.spec.Linux.Resources.CPU.Mems.
func (g Generator) SetLinuxResourcesCPUMems(mems string) {
	g.spec.Linux.Resources.CPU.Mems = &mems
}

// SetLinuxResourcesMemoryLimit sets g.spec.Linux.Resources.Memory.Limit.
func (g Generator) SetLinuxResourcesMemoryLimit(limit uint64) {
	if g.spec.Linux == nil {
		g.spec.Linux = &rspec.Linux{}
	}

	if g.spec.Linux.Resources == nil {
		g.spec.Linux.Resources = &rspec.Resources{}
	}

	if g.spec.Linux.Resources.Memory == nil {
		g.spec.Linux.Resources.Memory = &rspec.Memory{}
	}

	g.spec.Linux.Resources.Memory.Limit = &limit
}

// SetLinuxResourcesMemoryReservation sets g.spec.Linux.Resources.Memory.Reservation.
func (g Generator) SetLinuxResourcesMemoryReservation(reservation uint64) {
	g.spec.Linux.Resources.Memory.Reservation = &reservation
}

// SetLinuxResourcesMemorySwap sets g.spec.Linux.Resources.Memory.Swap.
func (g Generator) SetLinuxResourcesMemorySwap(swap uint64) {
	g.spec.Linux.Resources.Memory.Swap = &swap
}

// SetLinuxResourcesMemoryKernel sets g.spec.Linux.Resources.Memory.Kernel.
func (g Generator) SetLinuxResourcesMemoryKernel(kernel uint64) {
	g.spec.Linux.Resources.Memory.Kernel = &kernel
}

// SetLinuxResourcesMemoryKernelTCP sets g.spec.Linux.Resources.Memory.KernelTCP.
func (g Generator) SetLinuxResourcesMemoryKernelTCP(kernelTCP uint64) {
	g.spec.Linux.Resources.Memory.KernelTCP = &kernelTCP
}

// SetLinuxResourcesMemorySwappiness sets g.spec.Linux.Resources.Memory.Swappiness.
func (g Generator) SetLinuxResourcesMemorySwappiness(swappiness uint64) {
	g.spec.Linux.Resources.Memory.Swappiness = &swappiness
}

// ClearLinuxSysctl clears g.spec.Linux.Sysctl.
func (g Generator) ClearLinuxSysctl() {
	g.spec.Linux.Sysctl = make(map[string]string)
}

// AddLinuxSysctl adds a new sysctl config into g.spec.Linux.Sysctl.
func (g Generator) AddLinuxSysctl(s string) error {
	if g.spec.Linux.Sysctl == nil {
		g.spec.Linux.Sysctl = make(map[string]string)
	}

	pair := strings.Split(s, "=")
	if len(pair) != 2 {
		return fmt.Errorf("incorrectly specified sysctl: %s", s)
	}
	g.spec.Linux.Sysctl[pair[0]] = pair[1]
	return nil
}

// RemoveLinuxSysctl removes a sysctl config from g.spec.Linux.Sysctl.
func (g Generator) RemoveLinuxSysctl(key string) {
	if g.spec.Linux.Sysctl == nil {
		return
	}
	delete(g.spec.Linux.Sysctl, key)
}

// SetLinuxSeccompDefault sets g.spec.Linux.Seccomp.DefaultAction.
func (g Generator) SetLinuxSeccompDefault(sdefault string) error {
	switch sdefault {
	case "":
	case "SCMP_ACT_KILL":
	case "SCMP_ACT_TRAP":
	case "SCMP_ACT_ERRNO":
	case "SCMP_ACT_TRACE":
	case "SCMP_ACT_ALLOW":
	default:
		return fmt.Errorf("seccomp-default must be empty or one of " +
			"SCMP_ACT_KILL|SCMP_ACT_TRAP|SCMP_ACT_ERRNO|SCMP_ACT_TRACE|" +
			"SCMP_ACT_ALLOW")
	}

	if g.spec.Linux.Seccomp == nil {
		g.spec.Linux.Seccomp = &rspec.Seccomp{}
	}

	g.spec.Linux.Seccomp.DefaultAction = rspec.Action(sdefault)
	return nil
}

func checkSeccompArch(arch string) error {
	switch arch {
	case "":
	case "SCMP_ARCH_X86":
	case "SCMP_ARCH_X86_64":
	case "SCMP_ARCH_X32":
	case "SCMP_ARCH_ARM":
	case "SCMP_ARCH_AARCH64":
	case "SCMP_ARCH_MIPS":
	case "SCMP_ARCH_MIPS64":
	case "SCMP_ARCH_MIPS64N32":
	case "SCMP_ARCH_MIPSEL":
	case "SCMP_ARCH_MIPSEL64":
	case "SCMP_ARCH_MIPSEL64N32":
	default:
		return fmt.Errorf("seccomp-arch must be empty or one of " +
			"SCMP_ARCH_X86|SCMP_ARCH_X86_64|SCMP_ARCH_X32|SCMP_ARCH_ARM|" +
			"SCMP_ARCH_AARCH64SCMP_ARCH_MIPS|SCMP_ARCH_MIPS64|" +
			"SCMP_ARCH_MIPS64N32|SCMP_ARCH_MIPSEL|SCMP_ARCH_MIPSEL64|" +
			"SCMP_ARCH_MIPSEL64N32")
	}
	return nil
}

// ClearLinuxSeccompArch clears g.spec.Linux.Seccomp.Architectures.
func (g Generator) ClearLinuxSeccompArch() {
	if g.spec.Linux.Seccomp == nil {
		return
	}

	g.spec.Linux.Seccomp.Architectures = []rspec.Arch{}
}

// AddLinuxSeccompArch adds sArch into g.spec.Linux.Seccomp.Architectures.
func (g Generator) AddLinuxSeccompArch(sArch string) error {
	if err := checkSeccompArch(sArch); err != nil {
		return err
	}

	if g.spec.Linux.Seccomp == nil {
		g.spec.Linux.Seccomp = &rspec.Seccomp{}
	}

	g.spec.Linux.Seccomp.Architectures = append(g.spec.Linux.Seccomp.Architectures, rspec.Arch(sArch))

	return nil
}

// RemoveSeccompArch removes sArch from g.spec.Linux.Seccomp.Architectures.
func (g Generator) RemoveSeccompArch(sArch string) error {
	if err := checkSeccompArch(sArch); err != nil {
		return err
	}

	if g.spec.Linux.Seccomp == nil {
		return nil
	}

	for i, arch := range g.spec.Linux.Seccomp.Architectures {
		if string(arch) == sArch {
			g.spec.Linux.Seccomp.Architectures = append(g.spec.Linux.Seccomp.Architectures[:i], g.spec.Linux.Seccomp.Architectures[i+1:]...)
			return nil
		}
	}

	return nil
}

func checkSeccompSyscallAction(syscall string) error {
	switch syscall {
	case "":
	case "SCMP_ACT_KILL":
	case "SCMP_ACT_TRAP":
	case "SCMP_ACT_ERRNO":
	case "SCMP_ACT_TRACE":
	case "SCMP_ACT_ALLOW":
	default:
		return fmt.Errorf("seccomp-syscall action must be empty or " +
			"one of SCMP_ACT_KILL|SCMP_ACT_TRAP|SCMP_ACT_ERRNO|" +
			"SCMP_ACT_TRACE|SCMP_ACT_ALLOW")
	}
	return nil
}

func checkSeccompSyscallArg(arg string) error {
	switch arg {
	case "":
	case "SCMP_CMP_NE":
	case "SCMP_CMP_LT":
	case "SCMP_CMP_LE":
	case "SCMP_CMP_EQ":
	case "SCMP_CMP_GE":
	case "SCMP_CMP_GT":
	case "SCMP_CMP_MASKED_EQ":
	default:
		return fmt.Errorf("seccomp-syscall args must be " +
			"empty or one of SCMP_CMP_NE|SCMP_CMP_LT|" +
			"SCMP_CMP_LE|SCMP_CMP_EQ|SCMP_CMP_GE|" +
			"SCMP_CMP_GT|SCMP_CMP_MASKED_EQ")
	}
	return nil
}

func parseSeccompSyscall(s string) (rspec.Syscall, error) {
	syscall := strings.Split(s, ":")
	if len(syscall) != 3 {
		return rspec.Syscall{}, fmt.Errorf("seccomp sysctl must consist of 3 parameters")
	}
	name := syscall[0]
	if err := checkSeccompSyscallAction(syscall[1]); err != nil {
		return rspec.Syscall{}, err
	}
	action := rspec.Action(syscall[1])

	var Args []rspec.Arg
	if strings.EqualFold(syscall[2], "") {
		Args = nil
	} else {
		argsslice := strings.Split(syscall[2], ",")
		for _, argsstru := range argsslice {
			args := strings.Split(argsstru, "/")
			if len(args) == 4 {
				index, err := strconv.Atoi(args[0])
				value, err := strconv.Atoi(args[1])
				value2, err := strconv.Atoi(args[2])
				if err != nil {
					return rspec.Syscall{}, err
				}
				if err := checkSeccompSyscallArg(args[3]); err != nil {
					return rspec.Syscall{}, err
				}
				op := rspec.Operator(args[3])
				Arg := rspec.Arg{
					Index:    uint(index),
					Value:    uint64(value),
					ValueTwo: uint64(value2),
					Op:       op,
				}
				Args = append(Args, Arg)
			} else {
				return rspec.Syscall{}, fmt.Errorf("seccomp-sysctl args error: %s", argsstru)
			}
		}
	}

	return rspec.Syscall{
		Name:   name,
		Action: action,
		Args:   Args,
	}, nil
}

// ClearLinuxSeccompSyscall clears g.spec.Linux.Seccomp.Syscalls.
func (g Generator) ClearLinuxSeccompSyscall() {
	if g.spec.Linux.Seccomp == nil {
		return
	}

	g.spec.Linux.Seccomp.Syscalls = []rspec.Syscall{}
}

// AddLinuxSeccompSyscall adds sSyscall into g.spec.Linux.Seccomp.Syscalls.
func (g Generator) AddLinuxSeccompSyscall(sSyscall string) error {
	f, err := parseSeccompSyscall(sSyscall)
	if err != nil {
		return err
	}

	if g.spec.Linux.Seccomp == nil {
		g.spec.Linux.Seccomp = &rspec.Seccomp{}
	}

	g.spec.Linux.Seccomp.Syscalls = append(g.spec.Linux.Seccomp.Syscalls, f)
	return nil
}

// AddLinuxSeccompSyscallAllow adds seccompAllow into g.spec.Linux.Seccomp.Syscalls.
func (g Generator) AddLinuxSeccompSyscallAllow(seccompAllow string) {
	if g.spec.Linux.Seccomp == nil {
		g.spec.Linux.Seccomp = &rspec.Seccomp{}
	}

	syscall := rspec.Syscall{
		Name:   seccompAllow,
		Action: "SCMP_ACT_ALLOW",
	}
	g.spec.Linux.Seccomp.Syscalls = append(g.spec.Linux.Seccomp.Syscalls, syscall)
}

// AddLinuxSeccompSyscallErrno adds seccompErrno into g.spec.Linux.Seccomp.Syscalls.
func (g Generator) AddLinuxSeccompSyscallErrno(seccompErrno string) {
	if g.spec.Linux.Seccomp == nil {
		g.spec.Linux.Seccomp = &rspec.Seccomp{}
	}

	syscall := rspec.Syscall{
		Name:   seccompErrno,
		Action: "SCMP_ACT_ERRNO",
	}
	g.spec.Linux.Seccomp.Syscalls = append(g.spec.Linux.Seccomp.Syscalls, syscall)
}

// RemoveSeccompSyscallByName removes all the seccomp syscalls with the given
// name from g.spec.Linux.Seccomp.Syscalls.
func (g Generator) RemoveSeccompSyscallByName(name string) error {
	if g.spec.Linux.Seccomp == nil {
		return nil
	}

	var r []rspec.Syscall
	for _, syscall := range g.spec.Linux.Seccomp.Syscalls {
		if strings.Compare(name, syscall.Name) != 0 {
			r = append(r, syscall)
		}
	}
	g.spec.Linux.Seccomp.Syscalls = r
	return nil
}

// RemoveSeccompSyscallByAction removes all the seccomp syscalls with the given
// action from g.spec.Linux.Seccomp.Syscalls.
func (g Generator) RemoveSeccompSyscallByAction(action string) error {
	if g.spec.Linux.Seccomp == nil {
		return nil
	}

	if err := checkSeccompSyscallAction(action); err != nil {
		return err
	}

	var r []rspec.Syscall
	for _, syscall := range g.spec.Linux.Seccomp.Syscalls {
		if strings.Compare(action, string(syscall.Action)) != 0 {
			r = append(r, syscall)
		}
	}
	g.spec.Linux.Seccomp.Syscalls = r
	return nil
}

// RemoveSeccompSyscall removes all the seccomp syscalls with the given
// name and action from g.spec.Linux.Seccomp.Syscalls.
func (g Generator) RemoveSeccompSyscall(name string, action string) error {
	if g.spec.Linux.Seccomp == nil {
		return nil
	}

	if err := checkSeccompSyscallAction(action); err != nil {
		return err
	}

	var r []rspec.Syscall
	for _, syscall := range g.spec.Linux.Seccomp.Syscalls {
		if !(strings.Compare(name, syscall.Name) == 0 &&
			strings.Compare(action, string(syscall.Action)) == 0) {
			r = append(r, syscall)
		}
	}
	g.spec.Linux.Seccomp.Syscalls = r
	return nil
}

func parseIDMapping(idms string) (rspec.IDMapping, error) {
	idm := strings.Split(idms, ":")
	if len(idm) != 3 {
		return rspec.IDMapping{}, fmt.Errorf("idmappings error: %s", idms)
	}

	hid, err := strconv.Atoi(idm[0])
	if err != nil {
		return rspec.IDMapping{}, err
	}

	cid, err := strconv.Atoi(idm[1])
	if err != nil {
		return rspec.IDMapping{}, err
	}

	size, err := strconv.Atoi(idm[2])
	if err != nil {
		return rspec.IDMapping{}, err
	}

	idMapping := rspec.IDMapping{
		HostID:      uint32(hid),
		ContainerID: uint32(cid),
		Size:        uint32(size),
	}
	return idMapping, nil
}

// ClearLinuxUIDMappings clear g.spec.Linux.UIDMappings.
func (g Generator) ClearLinuxUIDMappings() {
	g.spec.Linux.UIDMappings = []rspec.IDMapping{}
}

// AddLinuxUIDMapping adds uidMap into g.spec.Linux.UIDMappings.
func (g Generator) AddLinuxUIDMapping(uidMap string) error {
	r, err := parseIDMapping(uidMap)
	if err != nil {
		return err
	}

	g.spec.Linux.UIDMappings = append(g.spec.Linux.UIDMappings, r)
	return nil
}

// ClearLinuxGIDMappings clear g.spec.Linux.GIDMappings.
func (g Generator) ClearLinuxGIDMappings() {
	g.spec.Linux.GIDMappings = []rspec.IDMapping{}
}

// AddLinuxGIDMapping adds gidMap into g.spec.Linux.GIDMappings.
func (g Generator) AddLinuxGIDMapping(gidMap string) error {
	r, err := parseIDMapping(gidMap)
	if err != nil {
		return err
	}

	g.spec.Linux.GIDMappings = append(g.spec.Linux.GIDMappings, r)
	return nil
}

// SetLinuxRootPropagation sets g.spec.Linux.RootfsPropagation.
func (g Generator) SetLinuxRootPropagation(rp string) error {
	switch rp {
	case "":
	case "private":
	case "rprivate":
	case "slave":
	case "rslave":
	case "shared":
	case "rshared":
	default:
		return fmt.Errorf("rootfs-propagation must be empty or one of private|rprivate|slave|rslave|shared|rshared")
	}
	g.spec.Linux.RootfsPropagation = rp
	return nil
}

func parseHook(s string) rspec.Hook {
	parts := strings.Split(s, ":")
	args := []string{}
	path := parts[0]
	if len(parts) > 1 {
		args = parts[1:]
	}
	return rspec.Hook{Path: path, Args: args}
}

// ClearPreStartHooks clear g.spec.Hooks.Prestart.
func (g Generator) ClearPreStartHooks() {
	g.spec.Hooks.Prestart = []rspec.Hook{}
}

// AddPreStartHook add a prestart hook into g.spec.Hooks.Prestart.
func (g Generator) AddPreStartHook(s string) error {
	hook := parseHook(s)
	g.spec.Hooks.Prestart = append(g.spec.Hooks.Prestart, hook)
	return nil
}

// ClearPostStopHooks clear g.spec.Hooks.Poststop.
func (g Generator) ClearPostStopHooks() {
	g.spec.Hooks.Poststop = []rspec.Hook{}
}

// AddPostStopHook adds a poststop hook into g.spec.Hooks.Poststop.
func (g Generator) AddPostStopHook(s string) error {
	hook := parseHook(s)
	g.spec.Hooks.Poststop = append(g.spec.Hooks.Poststop, hook)
	return nil
}

// ClearPostStartHooks clear g.spec.Hooks.Poststart.
func (g Generator) ClearPostStartHooks() {
	g.spec.Hooks.Poststart = []rspec.Hook{}
}

// AddPostStartHook adds a poststart hook into g.spec.Hooks.Poststart.
func (g Generator) AddPostStartHook(s string) error {
	hook := parseHook(s)
	g.spec.Hooks.Poststart = append(g.spec.Hooks.Poststart, hook)
	return nil
}

// AddTmpfsMount adds a tmpfs mount into g.spec.Mounts.
func (g Generator) AddTmpfsMount(dest string) error {
	mnt := rspec.Mount{
		Destination: dest,
		Type:        "tmpfs",
		Source:      "tmpfs",
		Options:     []string{"nosuid", "nodev", "mode=755"},
	}

	g.spec.Mounts = append(g.spec.Mounts, mnt)
	return nil
}

// AddCgroupsMount adds a cgroup mount into g.spec.Mounts.
func (g Generator) AddCgroupsMount(mountCgroupOption string) error {
	switch mountCgroupOption {
	case "ro":
	case "rw":
	case "no":
		return nil
	default:
		return fmt.Errorf("--mount-cgroups should be one of (ro,rw,no)")
	}

	mnt := rspec.Mount{
		Destination: "/sys/fs/cgroup",
		Type:        "cgroup",
		Source:      "cgroup",
		Options:     []string{"nosuid", "noexec", "nodev", "relatime", mountCgroupOption},
	}
	g.spec.Mounts = append(g.spec.Mounts, mnt)

	return nil
}

// AddBindMount adds a bind mount into g.spec.Mounts.
func (g Generator) AddBindMount(bind string) error {
	var source, dest string
	options := "ro"
	bparts := strings.SplitN(bind, ":", 3)
	switch len(bparts) {
	case 2:
		source, dest = bparts[0], bparts[1]
	case 3:
		source, dest, options = bparts[0], bparts[1], bparts[2]
	default:
		return fmt.Errorf("--bind should have format src:dest:[options]")
	}

	defaultOptions := []string{"bind"}
	mnt := rspec.Mount{
		Destination: dest,
		Type:        "bind",
		Source:      source,
		Options:     append(defaultOptions, options),
	}
	g.spec.Mounts = append(g.spec.Mounts, mnt)
	return nil
}

// SetupPrivileged sets up the priviledge-related fields inside g.spec.
func (g Generator) SetupPrivileged(privileged bool) {
	if privileged {
		// Add all capabilities in privileged mode.
		var finalCapList []string
		for _, cap := range capability.List() {
			finalCapList = append(finalCapList, fmt.Sprintf("CAP_%s", strings.ToUpper(cap.String())))
		}
		g.spec.Process.Capabilities = finalCapList
		g.spec.Process.SelinuxLabel = ""
		g.spec.Process.ApparmorProfile = ""
		g.spec.Linux.Seccomp = nil
	}
}

func checkCap(c string) error {
	isValid := false
	cp := strings.ToUpper(c)

	for _, cap := range capability.List() {
		if cp == strings.ToUpper(cap.String()) {
			isValid = true
			break
		}
	}

	if !isValid {
		return fmt.Errorf("Invalid value passed for adding capability")
	}
	return nil
}

// ClearProcessCapabilities clear g.spec.Process.Capabilities.
func (g Generator) ClearProcessCapabilities() {
	g.spec.Process.Capabilities = []string{}
}

// AddProcessCapability adds a process capability into g.spec.Process.Capabilities.
func (g Generator) AddProcessCapability(c string) error {
	if err := checkCap(c); err != nil {
		return err
	}

	cp := fmt.Sprintf("CAP_%s", strings.ToUpper(c))

	for _, cap := range g.spec.Process.Capabilities {
		if strings.ToUpper(cap) == cp {
			return nil
		}
	}

	g.spec.Process.Capabilities = append(g.spec.Process.Capabilities, cp)
	return nil
}

// DropProcessCapability drops a process capability from g.spec.Process.Capabilities.
func (g Generator) DropProcessCapability(c string) error {
	if err := checkCap(c); err != nil {
		return err
	}

	cp := fmt.Sprintf("CAP_%s", strings.ToUpper(c))

	for i, cap := range g.spec.Process.Capabilities {
		if strings.ToUpper(cap) == cp {
			g.spec.Process.Capabilities = append(g.spec.Process.Capabilities[:i], g.spec.Process.Capabilities[i+1:]...)
			return nil
		}
	}

	return nil
}

func mapStrToNamespace(ns string, path string) (rspec.Namespace, error) {
	switch ns {
	case "network":
		return rspec.Namespace{Type: rspec.NetworkNamespace, Path: path}, nil
	case "pid":
		return rspec.Namespace{Type: rspec.PIDNamespace, Path: path}, nil
	case "mount":
		return rspec.Namespace{Type: rspec.MountNamespace, Path: path}, nil
	case "ipc":
		return rspec.Namespace{Type: rspec.IPCNamespace, Path: path}, nil
	case "uts":
		return rspec.Namespace{Type: rspec.UTSNamespace, Path: path}, nil
	case "user":
		return rspec.Namespace{Type: rspec.UserNamespace, Path: path}, nil
	case "cgroup":
		return rspec.Namespace{Type: rspec.CgroupNamespace, Path: path}, nil
	default:
		return rspec.Namespace{}, fmt.Errorf("Should not reach here!")
	}
}

// ClearLinuxNamespaces clear g.spec.Linux.Namespaces.
func (g Generator) ClearLinuxNamespaces() {
	g.spec.Linux.Namespaces = []rspec.Namespace{}
}

// AddOrReplaceLinuxNamespace adds or replaces a namespace inside
// g.spec.Linux.Namespaces.
func (g Generator) AddOrReplaceLinuxNamespace(ns string, path string) error {
	namespace, err := mapStrToNamespace(ns, path)
	if err != nil {
		return err
	}

	for i, ns := range g.spec.Linux.Namespaces {
		if ns.Type == namespace.Type {
			g.spec.Linux.Namespaces[i] = namespace
			return nil
		}
	}
	g.spec.Linux.Namespaces = append(g.spec.Linux.Namespaces, namespace)
	return nil
}

// RemoveLinuxNamespace removes a namespace from g.spec.Linux.Namespaces.
func (g Generator) RemoveLinuxNamespace(ns string) error {
	namespace, err := mapStrToNamespace(ns, "")
	if err != nil {
		return err
	}

	for i, ns := range g.spec.Linux.Namespaces {
		if ns.Type == namespace.Type {
			g.spec.Linux.Namespaces = append(g.spec.Linux.Namespaces[:i], g.spec.Linux.Namespaces[i+1:]...)
			return nil
		}
	}
	return nil
}

// strPtr returns the pointer pointing to the string s.
func strPtr(s string) *string { return &s }

// FIXME: this function is not used.
func parseArgs(args2parse string) ([]*rspec.Arg, error) {
	var Args []*rspec.Arg
	argstrslice := strings.Split(args2parse, ",")
	for _, argstr := range argstrslice {
		args := strings.Split(argstr, "/")
		if len(args) == 4 {
			index, err := strconv.Atoi(args[0])
			value, err := strconv.Atoi(args[1])
			value2, err := strconv.Atoi(args[2])
			if err != nil {
				return nil, err
			}
			switch args[3] {
			case "":
			case "SCMP_CMP_NE":
			case "SCMP_CMP_LT":
			case "SCMP_CMP_LE":
			case "SCMP_CMP_EQ":
			case "SCMP_CMP_GE":
			case "SCMP_CMP_GT":
			case "SCMP_CMP_MASKED_EQ":
			default:
				return nil, fmt.Errorf("seccomp-sysctl args must be empty or one of SCMP_CMP_NE|SCMP_CMP_LT|SCMP_CMP_LE|SCMP_CMP_EQ|SCMP_CMP_GE|SCMP_CMP_GT|SCMP_CMP_MASKED_EQ")
			}
			op := rspec.Operator(args[3])
			Arg := rspec.Arg{
				Index:    uint(index),
				Value:    uint64(value),
				ValueTwo: uint64(value2),
				Op:       op,
			}
			Args = append(Args, &Arg)
		} else {
			return nil, fmt.Errorf("seccomp-sysctl args error: %s", argstr)
		}
	}
	return Args, nil
}
