package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/opencontainers/runtime-tools/generate/seccomp"
	"github.com/urfave/cli"
)

var generateFlags = []cli.Flag{
	cli.StringFlag{Name: "apparmor", Usage: "specifies the the apparmor profile for the container"},
	cli.StringFlag{Name: "arch", Value: runtime.GOARCH, Usage: "architecture the container is created for"},
	cli.StringSliceFlag{Name: "args", Usage: "command to run in the container"},
	cli.StringSliceFlag{Name: "bind", Usage: "bind mount directories src:dest[:options...]"},
	cli.StringSliceFlag{Name: "cap-add", Usage: "add Linux capabilities"},
	cli.StringSliceFlag{Name: "cap-drop", Usage: "drop Linux capabilities"},
	cli.StringFlag{Name: "cgroups-path", Usage: "specify the path to the cgroups"},
	cli.StringFlag{Name: "cwd", Value: "/", Usage: "current working directory for the process"},
	cli.BoolFlag{Name: "disable-oom-kill", Usage: "disable OOM Killer"},
	cli.StringSliceFlag{Name: "env", Usage: "add environment variable e.g. key=value"},
	cli.StringSliceFlag{Name: "env-file", Usage: "read in a file of environment variables"},
	cli.IntFlag{Name: "gid", Usage: "gid for the process"},
	cli.StringSliceFlag{Name: "gidmappings", Usage: "add GIDMappings e.g HostID:ContainerID:Size"},
	cli.StringSliceFlag{Name: "groups", Usage: "supplementary groups for the process"},
	cli.StringFlag{Name: "hostname", Usage: "hostname value for the container"},
	cli.StringSliceFlag{Name: "label", Usage: "add annotations to the configuration e.g. key=value"},
	cli.Uint64Flag{Name: "linux-cpu-shares", Usage: "the relative share of CPU time available to the tasks in a cgroup"},
	cli.Uint64Flag{Name: "linux-cpu-period", Usage: "the CPU period to be used for hardcapping (in usecs)"},
	cli.Uint64Flag{Name: "linux-cpu-quota", Usage: "the allowed CPU time in a given period (in usecs)"},
	cli.StringFlag{Name: "linux-cpus", Usage: "CPUs to use within the cpuset (default is to use any CPU available)"},
	cli.Uint64Flag{Name: "linux-mem-kernel-limit", Usage: "kernel memory limit (in bytes)"},
	cli.Uint64Flag{Name: "linux-mem-kernel-tcp", Usage: "kernel memory limit for tcp (in bytes)"},
	cli.Uint64Flag{Name: "linux-mem-limit", Usage: "memory limit (in bytes)"},
	cli.Uint64Flag{Name: "linux-mem-reservation", Usage: "memory reservation or soft limit (in bytes)"},
	cli.Uint64Flag{Name: "linux-mem-swap", Usage: "total memory limit (memory + swap) (in bytes)"},
	cli.Uint64Flag{Name: "linux-mem-swappiness", Usage: "how aggressive the kernel will swap memory pages (Range from 0 to 100)"},
	cli.StringFlag{Name: "linux-mems", Usage: "list of memory nodes in the cpuset (default is to use any available memory node)"},
	cli.StringSliceFlag{Name: "linux-namespace-add", Usage: "adds a namespace to the set of namespaces to create or join of the form 'ns[:path]'"},
	cli.StringSliceFlag{Name: "linux-namespace-remove", Usage: "removes a namespace from the set of namespaces to create or join of the form 'ns'"},
	cli.BoolFlag{Name: "linux-namespace-remove-all", Usage: "removes all namespaces from the set of namespaces created or joined"},
	cli.IntFlag{Name: "linux-network-classid", Usage: "specifies class identifier tagged by container's network packets"},
	cli.StringSliceFlag{Name: "linux-network-priorities", Usage: "specifies priorities of network traffic"},
	cli.Int64Flag{Name: "linux-pids-limit", Usage: "maximum number of PIDs"},
	cli.Uint64Flag{Name: "linux-realtime-period", Usage: "CPU period to be used for realtime scheduling (in usecs)"},
	cli.Uint64Flag{Name: "linux-realtime-runtime", Usage: "the time realtime scheduling may use (in usecs)"},
	cli.StringSliceFlag{Name: "masked-paths", Usage: "specifies paths can not be read inside container"},
	cli.StringFlag{Name: "mount-cgroups", Value: "no", Usage: "mount cgroups (rw,ro,no)"},
	cli.StringFlag{Name: "mount-label", Usage: "selinux mount context label"},
	cli.BoolFlag{Name: "no-new-privileges", Usage: "set no new privileges bit for the container process"},
	cli.IntFlag{Name: "oom-score-adj", Usage: "oom_score_adj for the container"},
	cli.StringFlag{Name: "os", Value: runtime.GOOS, Usage: "operating system the container is created for"},
	cli.StringFlag{Name: "output", Usage: "output file (defaults to stdout)"},
	cli.StringSliceFlag{Name: "poststart", Usage: "set command to run in poststart hooks"},
	cli.StringSliceFlag{Name: "poststop", Usage: "set command to run in poststop hooks"},
	cli.StringSliceFlag{Name: "prestart", Usage: "set command to run in prestart hooks"},
	cli.BoolFlag{Name: "privileged", Usage: "enable privileged container settings"},
	cli.StringSliceFlag{Name: "readonly-paths", Usage: "specifies paths readonly inside container"},
	cli.StringFlag{Name: "rootfs-path", Value: "rootfs", Usage: "path to the root filesystem"},
	cli.StringFlag{Name: "rootfs-propagation", Usage: "mount propagation for rootfs"},
	cli.BoolFlag{Name: "rootfs-readonly", Usage: "make the container's rootfs readonly"},
	cli.StringSliceFlag{Name: "rlimits-add", Usage: "specifies resource limits for processes inside the container. "},
	cli.StringSliceFlag{Name: "rlimits-remove", Usage: "remove specified resource limits for processes inside the container. "},
	cli.BoolFlag{Name: "rlimits-remove-all", Usage: "remove all resource limits for processes inside the container. "},
	cli.StringFlag{Name: "seccomp-allow", Usage: "specifies syscalls to respond with allow"},
	cli.StringFlag{Name: "seccomp-arch", Usage: "specifies additional architectures permitted to be used for system calls"},
	cli.StringFlag{Name: "seccomp-default", Usage: "specifies default action to be used for system calls and removes existing rules with specified action"},
	cli.StringFlag{Name: "seccomp-default-force", Usage: "same as seccomp-default but does not remove existing rules with specified action"},
	cli.StringFlag{Name: "seccomp-errno", Usage: "specifies syscalls to respond with errno"},
	cli.StringFlag{Name: "seccomp-kill", Usage: "specifies syscalls to respond with kill"},
	cli.BoolFlag{Name: "seccomp-only", Usage: "specifies to export just a seccomp configuration file"},
	cli.StringFlag{Name: "seccomp-remove", Usage: "specifies syscalls to remove seccomp rules for"},
	cli.BoolFlag{Name: "seccomp-remove-all", Usage: "removes all syscall rules from seccomp configuration"},
	cli.StringFlag{Name: "seccomp-trace", Usage: "specifies syscalls to respond with trace"},
	cli.StringFlag{Name: "seccomp-trap", Usage: "specifies syscalls to respond with trap"},
	cli.StringFlag{Name: "selinux-label", Usage: "process selinux label"},
	cli.StringSliceFlag{Name: "sysctl", Usage: "add sysctl settings e.g net.ipv4.forward=1"},
	cli.StringFlag{Name: "template", Usage: "base template to use for creating the configuration"},
	cli.StringSliceFlag{Name: "tmpfs", Usage: "mount tmpfs e.g. ContainerDIR[:OPTIONS...]"},
	cli.BoolFlag{Name: "tty", Usage: "allocate a new tty for the container process"},
	cli.IntFlag{Name: "uid", Usage: "uid for the process"},
	cli.StringSliceFlag{Name: "uidmappings", Usage: "add UIDMappings e.g HostID:ContainerID:Size"},
}

var generateCommand = cli.Command{
	Name:   "generate",
	Usage:  "generate an OCI spec file",
	Flags:  generateFlags,
	Before: before,
	Action: func(context *cli.Context) error {
		// Start from the default template.
		specgen := generate.New()

		var template string
		if context.IsSet("template") {
			template = context.String("template")
		}
		if template != "" {
			var err error
			specgen, err = generate.NewFromFile(template)
			if err != nil {
				return err
			}
		}

		err := setupSpec(&specgen, context)
		if err != nil {
			return err
		}

		var exportOpts generate.ExportOptions
		exportOpts.Seccomp = context.Bool("seccomp-only")

		if context.IsSet("output") {
			err = specgen.SaveToFile(context.String("output"), exportOpts)
		} else {
			err = specgen.Save(os.Stdout, exportOpts)
		}
		if err != nil {
			return err
		}
		return nil
	},
}

func setupSpec(g *generate.Generator, context *cli.Context) error {
	if context.GlobalBool("host-specific") {
		g.HostSpecific = true
	}

	spec := g.Spec()

	if len(spec.Version) == 0 {
		g.SetVersion(rspec.Version)
	}

	if context.IsSet("hostname") {
		g.SetHostname(context.String("hostname"))
	}

	g.SetPlatformOS(context.String("os"))
	g.SetPlatformArch(context.String("arch"))

	if context.IsSet("label") {
		annotations := context.StringSlice("label")
		for _, s := range annotations {
			pair := strings.SplitN(s, "=", 2)
			if len(pair) != 2 || pair[0] == "" {
				return fmt.Errorf("incorrectly specified annotation: %s", s)
			}
			g.AddAnnotation(pair[0], pair[1])
		}
	}

	g.SetRootPath(context.String("rootfs-path"))

	if context.IsSet("rootfs-readonly") {
		g.SetRootReadonly(context.Bool("rootfs-readonly"))
	}

	if context.IsSet("uid") {
		g.SetProcessUID(uint32(context.Int("uid")))
	}

	if context.IsSet("gid") {
		g.SetProcessGID(uint32(context.Int("gid")))
	}

	if context.IsSet("selinux-label") {
		g.SetProcessSelinuxLabel(context.String("selinux-label"))
	}

	g.SetProcessCwd(context.String("cwd"))

	if context.IsSet("apparmor") {
		g.SetProcessApparmorProfile(context.String("apparmor"))
	}

	if context.IsSet("no-new-privileges") {
		g.SetProcessNoNewPrivileges(context.Bool("no-new-privileges"))
	}

	if context.IsSet("tty") {
		g.SetProcessTerminal(context.Bool("tty"))
	}

	if context.IsSet("args") {
		g.SetProcessArgs(context.StringSlice("args"))
	}

	{
		envs, err := readKVStrings(context.StringSlice("env-file"), context.StringSlice("env"))
		if err != nil {
			return err
		}

		for _, env := range envs {
			name, value, err := parseEnv(env)
			if err != nil {
				return err
			}
			g.AddProcessEnv(name, value)
		}
	}

	if context.IsSet("groups") {
		groups := context.StringSlice("groups")
		for _, group := range groups {
			groupID, err := strconv.Atoi(group)
			if err != nil {
				return err
			}
			g.AddProcessAdditionalGid(uint32(groupID))
		}
	}

	if context.IsSet("cgroups-path") {
		g.SetLinuxCgroupsPath(context.String("cgroups-path"))
	}

	if context.IsSet("masked-paths") {
		paths := context.StringSlice("masked-paths")
		for _, path := range paths {
			g.AddLinuxMaskedPaths(path)
		}
	}

	if context.IsSet("readonly-paths") {
		paths := context.StringSlice("readonly-paths")
		for _, path := range paths {
			g.AddLinuxReadonlyPaths(path)
		}
	}

	if context.IsSet("mount-label") {
		g.SetLinuxMountLabel(context.String("mount-label"))
	}

	if context.IsSet("sysctl") {
		sysctls := context.StringSlice("sysctl")
		for _, s := range sysctls {
			pair := strings.Split(s, "=")
			if len(pair) != 2 {
				return fmt.Errorf("incorrectly specified sysctl: %s", s)
			}
			g.AddLinuxSysctl(pair[0], pair[1])
		}
	}

	g.SetupPrivileged(context.Bool("privileged"))

	if context.IsSet("cap-add") {
		addCaps := context.StringSlice("cap-add")
		for _, cap := range addCaps {
			if err := g.AddProcessCapability(cap); err != nil {
				return err
			}
		}
	}

	if context.IsSet("cap-drop") {
		dropCaps := context.StringSlice("cap-drop")
		for _, cap := range dropCaps {
			if err := g.DropProcessCapability(cap); err != nil {
				return err
			}
		}
	}

	var uidMaps, gidMaps []string

	if context.IsSet("uidmappings") {
		uidMaps = context.StringSlice("uidmappings")
	}

	if context.IsSet("gidmappings") {
		gidMaps = context.StringSlice("gidmappings")
	}

	// Add default user namespace.
	if len(uidMaps) > 0 || len(gidMaps) > 0 {
		g.AddOrReplaceLinuxNamespace("user", "")
	}

	if context.IsSet("tmpfs") {
		tmpfsSlice := context.StringSlice("tmpfs")
		for _, s := range tmpfsSlice {
			dest, options, err := parseTmpfsMount(s)
			if err != nil {
				return err
			}
			g.AddTmpfsMount(dest, options)
		}
	}

	mountCgroupOption := context.String("mount-cgroups")
	if err := g.AddCgroupsMount(mountCgroupOption); err != nil {
		return err
	}

	if context.IsSet("bind") {
		binds := context.StringSlice("bind")
		for _, bind := range binds {
			source, dest, options, err := parseBindMount(bind)
			if err != nil {
				return err
			}
			g.AddBindMount(source, dest, options)
		}
	}

	if context.IsSet("prestart") {
		preStartHooks := context.StringSlice("prestart")
		for _, hook := range preStartHooks {
			path, args := parseHook(hook)
			g.AddPreStartHook(path, args)
		}
	}

	if context.IsSet("poststop") {
		postStopHooks := context.StringSlice("poststop")
		for _, hook := range postStopHooks {
			path, args := parseHook(hook)
			g.AddPostStopHook(path, args)
		}
	}

	if context.IsSet("poststart") {
		postStartHooks := context.StringSlice("poststart")
		for _, hook := range postStartHooks {
			path, args := parseHook(hook)
			g.AddPostStartHook(path, args)
		}
	}

	if context.IsSet("rootfs-propagation") {
		rp := context.String("rootfs-propagation")
		if err := g.SetLinuxRootPropagation(rp); err != nil {
			return err
		}
	}

	for _, uidMap := range uidMaps {
		hid, cid, size, err := parseIDMapping(uidMap)
		if err != nil {
			return err
		}

		g.AddLinuxUIDMapping(hid, cid, size)
	}

	for _, gidMap := range gidMaps {
		hid, cid, size, err := parseIDMapping(gidMap)
		if err != nil {
			return err
		}

		g.AddLinuxGIDMapping(hid, cid, size)
	}

	if context.IsSet("disable-oom-kill") {
		g.SetLinuxResourcesDisableOOMKiller(context.Bool("disable-oom-kill"))
	}

	if context.IsSet("oom-score-adj") {
		g.SetLinuxResourcesOOMScoreAdj(context.Int("oom-score-adj"))
	}

	if context.IsSet("linux-cpu-shares") {
		g.SetLinuxResourcesCPUShares(context.Uint64("linux-cpu-shares"))
	}

	if context.IsSet("linux-cpu-period") {
		g.SetLinuxResourcesCPUPeriod(context.Uint64("linux-cpu-period"))
	}

	if context.IsSet("linux-cpu-quota") {
		g.SetLinuxResourcesCPUQuota(context.Uint64("linux-cpu-quota"))
	}

	if context.IsSet("linux-realtime-runtime") {
		g.SetLinuxResourcesCPURealtimeRuntime(context.Uint64("linux-realtime-runtime"))
	}

	if context.IsSet("linux-pids-limit") {
		g.SetLinuxResourcesPidsLimit(context.Int64("linux-pids-limit"))
	}

	if context.IsSet("linux-realtime-period") {
		g.SetLinuxResourcesCPURealtimePeriod(context.Uint64("linux-realtime-period"))
	}

	if context.IsSet("linux-cpus") {
		g.SetLinuxResourcesCPUCpus(context.String("linux-cpus"))
	}

	if context.IsSet("linux-mems") {
		g.SetLinuxResourcesCPUMems(context.String("linux-mems"))
	}

	if context.IsSet("linux-mem-limit") {
		g.SetLinuxResourcesMemoryLimit(context.Uint64("linux-mem-limit"))
	}

	if context.IsSet("linux-mem-reservation") {
		g.SetLinuxResourcesMemoryReservation(context.Uint64("linux-mem-reservation"))
	}

	if context.IsSet("linux-mem-swap") {
		g.SetLinuxResourcesMemorySwap(context.Uint64("linux-mem-swap"))
	}

	if context.IsSet("linux-mem-kernel-limit") {
		g.SetLinuxResourcesMemoryKernel(context.Uint64("linux-mem-kernel-limit"))
	}

	if context.IsSet("linux-mem-kernel-tcp") {
		g.SetLinuxResourcesMemoryKernelTCP(context.Uint64("linux-mem-kernel-tcp"))
	}

	if context.IsSet("linux-mem-swappiness") {
		g.SetLinuxResourcesMemorySwappiness(context.Uint64("linux-mem-swappiness"))
	}

	if context.IsSet("linux-network-classid") {
		g.SetLinuxResourcesNetworkClassID(uint32(context.Int("linux-network-classid")))
	}

	if context.IsSet("linux-network-priorities") {
		priorities := context.StringSlice("linux-network-priorities")
		for _, p := range priorities {
			name, priority, err := parseNetworkPriority(p)
			if err != nil {
				return err
			}
			if priority == -1 {
				g.DropLinuxResourcesNetworkPriorities(name)
			} else {
				g.AddLinuxResourcesNetworkPriorities(name, uint32(priority))
			}
		}
	}

	if context.IsSet("linux-namespace-add") {
		namespaces := context.StringSlice("linux-namespace-add")
		for _, ns := range namespaces {
			name, path, err := parseNamespace(ns)
			if err != nil {
				return err
			}
			if err := g.AddOrReplaceLinuxNamespace(name, path); err != nil {
				return err
			}
		}
	}

	if context.IsSet("linux-namespace-remove") {
		namespaces := context.StringSlice("linux-namespace-remove")
		for _, name := range namespaces {
			if err := g.RemoveLinuxNamespace(name); err != nil {
				return err
			}
		}
	}

	if context.Bool("linux-namespace-remove-all") {
		g.ClearLinuxNamespaces()
	}

	if context.IsSet("rlimits-add") {
		rlimits := context.StringSlice("rlimits-add")
		for _, rlimit := range rlimits {
			rType, rHard, rSoft, err := parseRlimit(rlimit)
			if err != nil {
				return err
			}
			g.AddProcessRlimits(rType, rHard, rSoft)
		}
	}

	if context.IsSet("rlimits-remove") {
		rlimits := context.StringSlice("rlimits-remove")
		for _, rlimit := range rlimits {
			err := g.RemoveProcessRlimits(rlimit)
			if err != nil {
				return err
			}
		}
	}

	if context.Bool("rlimits-remove-all") {
		g.ClearProcessRlimits()
	}

	err := addSeccomp(context, g)
	return err
}

func parseIDMapping(idms string) (uint32, uint32, uint32, error) {
	idm := strings.Split(idms, ":")
	if len(idm) != 3 {
		return 0, 0, 0, fmt.Errorf("idmappings error: %s", idms)
	}

	hid, err := strconv.Atoi(idm[0])
	if err != nil {
		return 0, 0, 0, err
	}

	cid, err := strconv.Atoi(idm[1])
	if err != nil {
		return 0, 0, 0, err
	}

	size, err := strconv.Atoi(idm[2])
	if err != nil {
		return 0, 0, 0, err
	}

	return uint32(hid), uint32(cid), uint32(size), nil
}

func parseHook(s string) (string, []string) {
	parts := strings.Split(s, ":")
	args := []string{}
	path := parts[0]
	if len(parts) > 1 {
		args = parts[1:]
	}
	return path, args
}

func parseNetworkPriority(np string) (string, int32, error) {
	var err error

	parts := strings.Split(np, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid value %v for --linux-network-priorities", np)
	}
	priority, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, err
	}

	return parts[0], int32(priority), nil
}

func parseTmpfsMount(s string) (string, []string, error) {
	var dest string
	var options []string
	var err error

	parts := strings.Split(s, ":")
	if len(parts) == 2 {
		dest = parts[0]
		options = strings.Split(parts[1], ",")
	} else if len(parts) == 1 {
		dest = parts[0]
		options = []string{"rw", "noexec", "nosuid", "nodev", "size=65536k"}
	} else {
		err = fmt.Errorf("invalid value for --tmpfs")
	}

	return dest, options, err
}

func parseBindMount(s string) (string, string, []string, error) {
	var source, dest string
	options := []string{}

	bparts := strings.SplitN(s, ":", 3)
	switch len(bparts) {
	case 2:
		source, dest = bparts[0], bparts[1]
	case 3:
		source, dest, options = bparts[0], bparts[1], strings.Split(bparts[2], ":")
	default:
		return source, dest, options, fmt.Errorf("--bind should have format src:dest[:options...]")
	}

	return source, dest, options, nil
}

func parseRlimit(rlimit string) (string, uint64, uint64, error) {
	parts := strings.Split(rlimit, ":")
	if len(parts) != 3 {
		return "", 0, 0, fmt.Errorf("invalid rlimits value: %s", rlimit)
	}

	hard, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, 0, err
	}

	soft, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", 0, 0, err
	}

	return parts[0], uint64(hard), uint64(soft), nil
}

func parseNamespace(ns string) (string, string, error) {
	parts := strings.SplitN(ns, ":", 2)
	if len(parts) == 0 || parts[0] == "" {
		return "", "", fmt.Errorf("invalid namespace value: %s", ns)
	}

	nsType := parts[0]
	nsPath := ""

	if len(parts) == 2 {
		nsPath = parts[1]
	}

	return nsType, nsPath, nil
}

func addSeccomp(context *cli.Context, g *generate.Generator) error {

	// Set the DefaultAction of seccomp
	if context.IsSet("seccomp-default") {
		seccompDefault := context.String("seccomp-default")
		err := g.SetDefaultSeccompAction(seccompDefault)
		if err != nil {
			return err
		}
	} else if context.IsSet("seccomp-default-force") {
		seccompDefaultForced := context.String("seccomp-default-force")
		err := g.SetDefaultSeccompActionForce(seccompDefaultForced)
		if err != nil {
			return err
		}
	}

	// Add the additional architectures permitted to be used for system calls
	if context.IsSet("seccomp-arch") {
		seccompArch := context.String("seccomp-arch")
		architectureArgs := strings.Split(seccompArch, ",")
		for _, arg := range architectureArgs {
			err := g.SetSeccompArchitecture(arg)
			if err != nil {
				return err
			}
		}
	}

	if context.IsSet("seccomp-errno") {
		err := seccompSet(context, "errno", g)
		if err != nil {
			return err
		}
	}

	if context.IsSet("seccomp-kill") {
		err := seccompSet(context, "kill", g)
		if err != nil {
			return err
		}
	}

	if context.IsSet("seccomp-trace") {
		err := seccompSet(context, "trace", g)
		if err != nil {
			return err
		}
	}

	if context.IsSet("seccomp-trap") {
		err := seccompSet(context, "trap", g)
		if err != nil {
			return err
		}
	}

	if context.IsSet("seccomp-allow") {
		err := seccompSet(context, "allow", g)
		if err != nil {
			return err
		}
	}

	if context.IsSet("seccomp-remove") {
		seccompRemove := context.String("seccomp-remove")
		err := g.RemoveSeccompRule(seccompRemove)
		if err != nil {
			return err
		}
	}

	if context.Bool("seccomp-remove-all") {
		err := g.RemoveAllSeccompRules()
		if err != nil {
			return err
		}
	}
	return nil
}

func seccompSet(context *cli.Context, seccompFlag string, g *generate.Generator) error {
	flagInput := context.String("seccomp-" + seccompFlag)
	flagArgs := strings.Split(flagInput, ",")
	setSyscallArgsSlice := []seccomp.SyscallOpts{}
	for _, flagArg := range flagArgs {
		comparisonArgs := strings.Split(flagArg, ":")
		if len(comparisonArgs) == 5 {
			setSyscallArgs := seccomp.SyscallOpts{
				Action:   seccompFlag,
				Syscall:  comparisonArgs[0],
				Index:    comparisonArgs[1],
				Value:    comparisonArgs[2],
				ValueTwo: comparisonArgs[3],
				Operator: comparisonArgs[4],
			}
			setSyscallArgsSlice = append(setSyscallArgsSlice, setSyscallArgs)
		} else if len(comparisonArgs) == 1 {
			setSyscallArgs := seccomp.SyscallOpts{
				Action:  seccompFlag,
				Syscall: comparisonArgs[0],
			}
			setSyscallArgsSlice = append(setSyscallArgsSlice, setSyscallArgs)
		} else {
			return fmt.Errorf("invalid syscall argument formatting %v", comparisonArgs)
		}

		for _, r := range setSyscallArgsSlice {
			err := g.SetSyscallAction(r)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// readKVStrings reads a file of line terminated key=value pairs, and overrides any keys
// present in the file with additional pairs specified in the override parameter
//
// This function is copied from github.com/docker/docker/runconfig/opts/parse.go
func readKVStrings(files []string, override []string) ([]string, error) {
	envVariables := []string{}
	for _, ef := range files {
		parsedVars, err := parseEnvFile(ef)
		if err != nil {
			return nil, err
		}
		envVariables = append(envVariables, parsedVars...)
	}
	// parse the '-e' and '--env' after, to allow override
	envVariables = append(envVariables, override...)

	return envVariables, nil
}

// parseEnv splits a given environment variable (of the form name=value) into
// (name, value). An error is returned if there is no "=" in the line or if the
// name is empty.
func parseEnv(env string) (string, string, error) {
	parts := strings.SplitN(env, "=", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("environment variable must contain '=': %s", env)
	}

	name, value := parts[0], parts[1]
	if name == "" {
		return "", "", fmt.Errorf("environment variable must have non-empty name: %s", env)
	}
	return name, value, nil
}

// parseEnvFile reads a file with environment variables enumerated by lines
//
// ``Environment variable names used by the utilities in the Shell and
// Utilities volume of IEEE Std 1003.1-2001 consist solely of uppercase
// letters, digits, and the '_' (underscore) from the characters defined in
// Portable Character Set and do not begin with a digit. *But*, other
// characters may be permitted by an implementation; applications shall
// tolerate the presence of such names.''
// -- http://pubs.opengroup.org/onlinepubs/009695399/basedefs/xbd_chap08.html
//
// As of #16585, it's up to application inside docker to validate or not
// environment variables, that's why we just strip leading whitespace and
// nothing more.
//
// This function is copied from github.com/docker/docker/runconfig/opts/envfile.go
func parseEnvFile(filename string) ([]string, error) {
	fh, err := os.Open(filename)
	if err != nil {
		return []string{}, err
	}
	defer fh.Close()

	lines := []string{}
	scanner := bufio.NewScanner(fh)
	currentLine := 0
	utf8bom := []byte{0xEF, 0xBB, 0xBF}
	for scanner.Scan() {
		scannedBytes := scanner.Bytes()
		if !utf8.Valid(scannedBytes) {
			return []string{}, fmt.Errorf("env file %s contains invalid utf8 bytes at line %d: %v", filename, currentLine+1, scannedBytes)
		}
		// We trim UTF8 BOM
		if currentLine == 0 {
			scannedBytes = bytes.TrimPrefix(scannedBytes, utf8bom)
		}
		// trim the line from all leading whitespace first
		line := strings.TrimLeftFunc(string(scannedBytes), unicode.IsSpace)
		currentLine++
		// line is not empty, and not starting with '#'
		if len(line) > 0 && !strings.HasPrefix(line, "#") {
			data := strings.SplitN(line, "=", 2)

			// trim the front of a variable, but nothing else
			variable := strings.TrimLeft(data[0], whiteSpaces)
			if strings.ContainsAny(variable, whiteSpaces) {
				return []string{}, ErrBadEnvVariable{fmt.Sprintf("variable '%s' has white spaces", variable)}
			}

			if len(data) > 1 {

				// pass the value through, no trimming
				lines = append(lines, fmt.Sprintf("%s=%s", variable, data[1]))
			} else {
				// if only a pass-through variable is given, clean it up.
				lines = append(lines, fmt.Sprintf("%s=%s", strings.TrimSpace(line), os.Getenv(line)))
			}
		}
	}
	return lines, scanner.Err()
}

var whiteSpaces = " \t"

// ErrBadEnvVariable typed error for bad environment variable
type ErrBadEnvVariable struct {
	msg string
}

func (e ErrBadEnvVariable) Error() string {
	return fmt.Sprintf("poorly formatted environment: %s", e.msg)
}
