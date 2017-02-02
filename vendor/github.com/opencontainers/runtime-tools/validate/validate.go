package validate

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Sirupsen/logrus"
	"github.com/blang/semver"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
)

const specConfig = "config.json"

var (
	defaultRlimits = []string{
		"RLIMIT_CPU",
		"RLIMIT_FSIZE",
		"RLIMIT_DATA",
		"RLIMIT_STACK",
		"RLIMIT_CORE",
		"RLIMIT_RSS",
		"RLIMIT_NPROC",
		"RLIMIT_NOFILE",
		"RLIMIT_MEMLOCK",
		"RLIMIT_AS",
		"RLIMIT_LOCKS",
		"RLIMIT_SIGPENDING",
		"RLIMIT_MSGQUEUE",
		"RLIMIT_NICE",
		"RLIMIT_RTPRIO",
		"RLIMIT_RTTIME",
	}
	defaultCaps = []string{
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
	}
)

type Validator struct {
	spec         *rspec.Spec
	bundlePath   string
	HostSpecific bool
}

func NewValidator(spec *rspec.Spec, bundlePath string, hostSpecific bool) Validator {
	return Validator{spec: spec, bundlePath: bundlePath, HostSpecific: hostSpecific}
}

func NewValidatorFromPath(bundlePath string, hostSpecific bool) (Validator, error) {
	if bundlePath == "" {
		return Validator{}, fmt.Errorf("Bundle path shouldn't be empty")
	}

	if _, err := os.Stat(bundlePath); err != nil {
		return Validator{}, err
	}

	configPath := filepath.Join(bundlePath, specConfig)
	content, err := ioutil.ReadFile(configPath)
	if err != nil {
		return Validator{}, err
	}
	if !utf8.Valid(content) {
		return Validator{}, fmt.Errorf("%q is not encoded in UTF-8", configPath)
	}
	var spec rspec.Spec
	if err = json.Unmarshal(content, &spec); err != nil {
		return Validator{}, err
	}

	return NewValidator(&spec, bundlePath, hostSpecific), nil
}

func (v *Validator) CheckAll() (msgs []string) {
	msgs = append(msgs, v.CheckRootfsPath()...)
	msgs = append(msgs, v.CheckMandatoryFields()...)
	msgs = append(msgs, v.CheckSemVer()...)
	msgs = append(msgs, v.CheckMounts()...)
	msgs = append(msgs, v.CheckPlatform()...)
	msgs = append(msgs, v.CheckProcess()...)
	msgs = append(msgs, v.CheckLinux()...)
	msgs = append(msgs, v.CheckHooks()...)

	return
}

func (v *Validator) CheckRootfsPath() (msgs []string) {
	logrus.Debugf("check rootfs path")

	var rootfsPath string
	if filepath.IsAbs(v.spec.Root.Path) {
		rootfsPath = v.spec.Root.Path
	} else {
		rootfsPath = filepath.Join(v.bundlePath, v.spec.Root.Path)
	}

	if fi, err := os.Stat(rootfsPath); err != nil {
		msgs = append(msgs, fmt.Sprintf("Cannot find the root path %q", rootfsPath))
	} else if !fi.IsDir() {
		msgs = append(msgs, fmt.Sprintf("The root path %q is not a directory.", rootfsPath))
	}

	return

}
func (v *Validator) CheckSemVer() (msgs []string) {
	logrus.Debugf("check semver")

	version := v.spec.Version
	_, err := semver.Parse(version)
	if err != nil {
		msgs = append(msgs, fmt.Sprintf("%q is not valid SemVer: %s", version, err.Error()))
	}
	if version != rspec.Version {
		msgs = append(msgs, fmt.Sprintf("internal error: validate currently only handles version %s, but the supplied configuration targets %s", rspec.Version, version))
	}

	return
}

func (v *Validator) CheckPlatform() (msgs []string) {
	logrus.Debugf("check platform")

	validCombins := map[string][]string{
		"android":   {"arm"},
		"darwin":    {"386", "amd64", "arm", "arm64"},
		"dragonfly": {"amd64"},
		"freebsd":   {"386", "amd64", "arm"},
		"linux":     {"386", "amd64", "arm", "arm64", "ppc64", "ppc64le", "mips64", "mips64le", "s390x"},
		"netbsd":    {"386", "amd64", "arm"},
		"openbsd":   {"386", "amd64", "arm"},
		"plan9":     {"386", "amd64"},
		"solaris":   {"amd64"},
		"windows":   {"386", "amd64"}}
	platform := v.spec.Platform
	for os, archs := range validCombins {
		if os == platform.OS {
			for _, arch := range archs {
				if arch == platform.Arch {
					return nil
				}
			}
			msgs = append(msgs, fmt.Sprintf("Combination of %q and %q is invalid.", platform.OS, platform.Arch))
		}
	}
	msgs = append(msgs, fmt.Sprintf("Operation system %q of the bundle is not supported yet.", platform.OS))

	return
}

func (v *Validator) CheckHooks() (msgs []string) {
	logrus.Debugf("check hooks")

	msgs = append(msgs, checkEventHooks("pre-start", v.spec.Hooks.Prestart, v.HostSpecific)...)
	msgs = append(msgs, checkEventHooks("post-start", v.spec.Hooks.Poststart, v.HostSpecific)...)
	msgs = append(msgs, checkEventHooks("post-stop", v.spec.Hooks.Poststop, v.HostSpecific)...)

	return
}

func checkEventHooks(hookType string, hooks []rspec.Hook, hostSpecific bool) (msgs []string) {
	for _, hook := range hooks {
		if !filepath.IsAbs(hook.Path) {
			msgs = append(msgs, fmt.Sprintf("The %s hook %v: is not absolute path", hookType, hook.Path))
		}

		if hostSpecific {
			fi, err := os.Stat(hook.Path)
			if err != nil {
				msgs = append(msgs, fmt.Sprintf("Cannot find %s hook: %v", hookType, hook.Path))
			}
			if fi.Mode()&0111 == 0 {
				msgs = append(msgs, fmt.Sprintf("The %s hook %v: is not executable", hookType, hook.Path))
			}
		}

		for _, env := range hook.Env {
			if !envValid(env) {
				msgs = append(msgs, fmt.Sprintf("Env %q for hook %v is in the invalid form.", env, hook.Path))
			}
		}
	}

	return
}

func (v *Validator) CheckProcess() (msgs []string) {
	logrus.Debugf("check process")

	process := v.spec.Process
	if !filepath.IsAbs(process.Cwd) {
		msgs = append(msgs, fmt.Sprintf("cwd %q is not an absolute path", process.Cwd))
	}

	for _, env := range process.Env {
		if !envValid(env) {
			msgs = append(msgs, fmt.Sprintf("env %q should be in the form of 'key=value'. The left hand side must consist solely of letters, digits, and underscores '_'.", env))
		}
	}

	for index := 0; index < len(process.Capabilities); index++ {
		capability := process.Capabilities[index]
		if !capValid(capability) {
			msgs = append(msgs, fmt.Sprintf("capability %q is not valid, man capabilities(7)", process.Capabilities[index]))
		}
	}

	for index := 0; index < len(process.Rlimits); index++ {
		if !rlimitValid(process.Rlimits[index].Type) {
			msgs = append(msgs, fmt.Sprintf("rlimit type %q is invalid.", process.Rlimits[index].Type))
		}
		if process.Rlimits[index].Hard < process.Rlimits[index].Soft {
			msgs = append(msgs, fmt.Sprintf("hard limit of rlimit %s should not be less than soft limit.", process.Rlimits[index].Type))
		}
	}

	if len(process.ApparmorProfile) > 0 {
		profilePath := filepath.Join(v.bundlePath, v.spec.Root.Path, "/etc/apparmor.d", process.ApparmorProfile)
		_, err := os.Stat(profilePath)
		if err != nil {
			msgs = append(msgs, err.Error())
		}
	}

	return
}

func supportedMountTypes(OS string, hostSpecific bool) (map[string]bool, error) {
	supportedTypes := make(map[string]bool)

	if OS != "linux" && OS != "windows" {
		logrus.Warnf("%v is not supported to check mount type", OS)
		return nil, nil
	} else if OS == "windows" {
		supportedTypes["ntfs"] = true
		return supportedTypes, nil
	}

	if hostSpecific {
		f, err := os.Open("/proc/filesystems")
		if err != nil {
			return nil, err
		}
		defer f.Close()

		s := bufio.NewScanner(f)
		for s.Scan() {
			if err := s.Err(); err != nil {
				return supportedTypes, err
			}

			text := s.Text()
			parts := strings.Split(text, "\t")
			if len(parts) > 1 {
				supportedTypes[parts[1]] = true
			} else {
				supportedTypes[parts[0]] = true
			}
		}

		supportedTypes["bind"] = true

		return supportedTypes, nil
	}
	logrus.Warn("Checking linux mount types without --host-specific is not supported yet")
	return nil, nil
}

func (v *Validator) CheckMounts() (msgs []string) {
	logrus.Debugf("check mounts")

	supportedTypes, err := supportedMountTypes(v.spec.Platform.OS, v.HostSpecific)
	if err != nil {
		msgs = append(msgs, err.Error())
		return
	}

	if supportedTypes != nil {
		for _, mount := range v.spec.Mounts {
			if !supportedTypes[mount.Type] {
				msgs = append(msgs, fmt.Sprintf("Unsupported mount type %q", mount.Type))
			}

			if !filepath.IsAbs(mount.Destination) {
				msgs = append(msgs, fmt.Sprintf("destination %v is not an absolute path", mount.Destination))
			}
		}
	}

	return
}

//Linux only
func (v *Validator) CheckLinux() (msgs []string) {
	logrus.Debugf("check linux")

	utsExists := false
	ipcExists := false
	mountExists := false
	netExists := false
	userExists := false

	for index := 0; index < len(v.spec.Linux.Namespaces); index++ {
		if !namespaceValid(v.spec.Linux.Namespaces[index]) {
			msgs = append(msgs, fmt.Sprintf("namespace %v is invalid.", v.spec.Linux.Namespaces[index]))
		} else if len(v.spec.Linux.Namespaces[index].Path) == 0 {
			if v.spec.Linux.Namespaces[index].Type == rspec.UTSNamespace {
				utsExists = true
			} else if v.spec.Linux.Namespaces[index].Type == rspec.IPCNamespace {
				ipcExists = true
			} else if v.spec.Linux.Namespaces[index].Type == rspec.NetworkNamespace {
				netExists = true
			} else if v.spec.Linux.Namespaces[index].Type == rspec.MountNamespace {
				mountExists = true
			} else if v.spec.Linux.Namespaces[index].Type == rspec.UserNamespace {
				userExists = true
			}
		}
	}

	if (len(v.spec.Linux.UIDMappings) > 0 || len(v.spec.Linux.GIDMappings) > 0) && !userExists {
		msgs = append(msgs, "UID/GID mappings requires a new User namespace to be specified as well")
	} else if len(v.spec.Linux.UIDMappings) > 5 {
		msgs = append(msgs, "Only 5 UID mappings are allowed (linux kernel restriction).")
	} else if len(v.spec.Linux.GIDMappings) > 5 {
		msgs = append(msgs, "Only 5 GID mappings are allowed (linux kernel restriction).")
	}

	for k := range v.spec.Linux.Sysctl {
		if strings.HasPrefix(k, "net.") && !netExists {
			msgs = append(msgs, fmt.Sprintf("Sysctl %v requires a new Network namespace to be specified as well", k))
		}
		if strings.HasPrefix(k, "fs.mqueue.") {
			if !mountExists || !ipcExists {
				msgs = append(msgs, fmt.Sprintf("Sysctl %v requires a new IPC namespace and Mount namespace to be specified as well", k))
			}
		}
	}

	if v.spec.Platform.OS == "linux" && !utsExists && v.spec.Hostname != "" {
		msgs = append(msgs, fmt.Sprintf("On Linux, hostname requires a new UTS namespace to be specified as well"))
	}

	for index := 0; index < len(v.spec.Linux.Devices); index++ {
		if !deviceValid(v.spec.Linux.Devices[index]) {
			msgs = append(msgs, fmt.Sprintf("device %v is invalid.", v.spec.Linux.Devices[index]))
		}
	}

	if v.spec.Linux.Resources != nil {
		ms := v.CheckLinuxResources()
		msgs = append(msgs, ms...)
	}

	if v.spec.Linux.Seccomp != nil {
		ms := v.CheckSeccomp()
		msgs = append(msgs, ms...)
	}

	switch v.spec.Linux.RootfsPropagation {
	case "":
	case "private":
	case "rprivate":
	case "slave":
	case "rslave":
	case "shared":
	case "rshared":
	default:
		msgs = append(msgs, "rootfsPropagation must be empty or one of \"private|rprivate|slave|rslave|shared|rshared\"")
	}

	for _, maskedPath := range v.spec.Linux.MaskedPaths {
		if !strings.HasPrefix(maskedPath, "/") {
			msgs = append(msgs, "maskedPath %v is not an absolute path", maskedPath)
		}
	}

	for _, readonlyPath := range v.spec.Linux.ReadonlyPaths {
		if !strings.HasPrefix(readonlyPath, "/") {
			msgs = append(msgs, "readonlyPath %v is not an absolute path", readonlyPath)
		}
	}

	return
}

func (v *Validator) CheckLinuxResources() (msgs []string) {
	logrus.Debugf("check linux resources")

	r := v.spec.Linux.Resources
	if r.Memory != nil {
		if r.Memory.Limit != nil && r.Memory.Swap != nil && uint64(*r.Memory.Limit) > uint64(*r.Memory.Swap) {
			msgs = append(msgs, fmt.Sprintf("Minimum memoryswap should be larger than memory limit"))
		}
		if r.Memory.Limit != nil && r.Memory.Reservation != nil && uint64(*r.Memory.Reservation) > uint64(*r.Memory.Limit) {
			msgs = append(msgs, fmt.Sprintf("Minimum memory limit should be larger than memory reservation"))
		}
	}

	return
}

func (v *Validator) CheckSeccomp() (msgs []string) {
	logrus.Debugf("check linux seccomp")

	s := v.spec.Linux.Seccomp
	if !seccompActionValid(s.DefaultAction) {
		msgs = append(msgs, fmt.Sprintf("seccomp defaultAction %q is invalid.", s.DefaultAction))
	}
	for index := 0; index < len(s.Syscalls); index++ {
		if !syscallValid(s.Syscalls[index]) {
			msgs = append(msgs, fmt.Sprintf("syscall %v is invalid.", s.Syscalls[index]))
		}
	}
	for index := 0; index < len(s.Architectures); index++ {
		switch s.Architectures[index] {
		case rspec.ArchX86:
		case rspec.ArchX86_64:
		case rspec.ArchX32:
		case rspec.ArchARM:
		case rspec.ArchAARCH64:
		case rspec.ArchMIPS:
		case rspec.ArchMIPS64:
		case rspec.ArchMIPS64N32:
		case rspec.ArchMIPSEL:
		case rspec.ArchMIPSEL64:
		case rspec.ArchMIPSEL64N32:
		case rspec.ArchPPC:
		case rspec.ArchPPC64:
		case rspec.ArchPPC64LE:
		case rspec.ArchS390:
		case rspec.ArchS390X:
		default:
			msgs = append(msgs, fmt.Sprintf("seccomp architecture %q is invalid", s.Architectures[index]))
		}
	}

	return
}

func envValid(env string) bool {
	items := strings.Split(env, "=")
	if len(items) < 2 {
		return false
	}
	for i, ch := range strings.TrimSpace(items[0]) {
		if !unicode.IsDigit(ch) && !unicode.IsLetter(ch) && ch != '_' {
			return false
		}
		if i == 0 && unicode.IsDigit(ch) {
			logrus.Warnf("Env %v: variable name beginning with digit is not recommended.", env)
		}
	}
	return true
}

func capValid(capability string) bool {
	for _, val := range defaultCaps {
		if val == capability {
			return true
		}
	}
	return false
}

func rlimitValid(rlimit string) bool {
	for _, val := range defaultRlimits {
		if val == rlimit {
			return true
		}
	}
	return false
}

func namespaceValid(ns rspec.Namespace) bool {
	switch ns.Type {
	case rspec.PIDNamespace:
	case rspec.NetworkNamespace:
	case rspec.MountNamespace:
	case rspec.IPCNamespace:
	case rspec.UTSNamespace:
	case rspec.UserNamespace:
	case rspec.CgroupNamespace:
	default:
		return false
	}
	return true
}

func deviceValid(d rspec.Device) bool {
	switch d.Type {
	case "b":
	case "c":
	case "u":
		if d.Major <= 0 {
			return false
		}
		if d.Minor <= 0 {
			return false
		}
	case "p":
		if d.Major > 0 || d.Minor > 0 {
			return false
		}
	default:
		return false
	}
	return true
}

func seccompActionValid(secc rspec.Action) bool {
	switch secc {
	case "":
	case rspec.ActKill:
	case rspec.ActTrap:
	case rspec.ActErrno:
	case rspec.ActTrace:
	case rspec.ActAllow:
	default:
		return false
	}
	return true
}

func syscallValid(s rspec.Syscall) bool {
	if !seccompActionValid(s.Action) {
		return false
	}
	for index := 0; index < len(s.Args); index++ {
		arg := s.Args[index]
		switch arg.Op {
		case rspec.OpNotEqual:
		case rspec.OpLessThan:
		case rspec.OpLessEqual:
		case rspec.OpEqualTo:
		case rspec.OpGreaterEqual:
		case rspec.OpGreaterThan:
		case rspec.OpMaskedEqual:
		default:
			return false
		}
	}
	return true
}

func isStruct(t reflect.Type) bool {
	return t.Kind() == reflect.Struct
}

func isStructPtr(t reflect.Type) bool {
	return t.Kind() == reflect.Ptr && t.Elem().Kind() == reflect.Struct
}

func checkMandatoryUnit(field reflect.Value, tagField reflect.StructField, parent string) (msgs []string) {
	mandatory := !strings.Contains(tagField.Tag.Get("json"), "omitempty")
	switch field.Kind() {
	case reflect.Ptr:
		if mandatory && field.IsNil() {
			msgs = append(msgs, fmt.Sprintf("'%s.%s' should not be empty.", parent, tagField.Name))
		}
	case reflect.String:
		if mandatory && (field.Len() == 0) {
			msgs = append(msgs, fmt.Sprintf("'%s.%s' should not be empty.", parent, tagField.Name))
		}
	case reflect.Slice:
		if mandatory && (field.IsNil() || field.Len() == 0) {
			msgs = append(msgs, fmt.Sprintf("'%s.%s' should not be empty.", parent, tagField.Name))
			return
		}
		for index := 0; index < field.Len(); index++ {
			mValue := field.Index(index)
			if mValue.CanInterface() {
				msgs = append(msgs, checkMandatory(mValue.Interface())...)
			}
		}
	case reflect.Map:
		if mandatory && (field.IsNil() || field.Len() == 0) {
			msgs = append(msgs, fmt.Sprintf("'%s.%s' should not be empty.", parent, tagField.Name))
			return msgs
		}
		keys := field.MapKeys()
		for index := 0; index < len(keys); index++ {
			mValue := field.MapIndex(keys[index])
			if mValue.CanInterface() {
				msgs = append(msgs, checkMandatory(mValue.Interface())...)
			}
		}
	default:
	}

	return
}

func checkMandatory(obj interface{}) (msgs []string) {
	objT := reflect.TypeOf(obj)
	objV := reflect.ValueOf(obj)
	if isStructPtr(objT) {
		objT = objT.Elem()
		objV = objV.Elem()
	} else if !isStruct(objT) {
		return
	}

	for i := 0; i < objT.NumField(); i++ {
		t := objT.Field(i).Type
		if isStructPtr(t) && objV.Field(i).IsNil() {
			if !strings.Contains(objT.Field(i).Tag.Get("json"), "omitempty") {
				msgs = append(msgs, fmt.Sprintf("'%s.%s' should not be empty", objT.Name(), objT.Field(i).Name))
			}
		} else if (isStruct(t) || isStructPtr(t)) && objV.Field(i).CanInterface() {
			msgs = append(msgs, checkMandatory(objV.Field(i).Interface())...)
		} else {
			msgs = append(msgs, checkMandatoryUnit(objV.Field(i), objT.Field(i), objT.Name())...)
		}

	}
	return
}

func (v *Validator) CheckMandatoryFields() []string {
	logrus.Debugf("check mandatory fields")

	return checkMandatory(v.spec)
}
