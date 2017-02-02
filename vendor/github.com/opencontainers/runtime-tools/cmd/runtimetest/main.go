package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/hashicorp/go-multierror"
	"github.com/mndrix/tap-go"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/cmd/runtimetest/mount"
	"github.com/syndtr/gocapability/capability"
	"github.com/urfave/cli"
)

// PR_GET_NO_NEW_PRIVS isn't exposed in Golang so we define it ourselves copying the value from
// the kernel
const PR_GET_NO_NEW_PRIVS = 39

const specConfig = "config.json"

var (
	defaultFS = map[string]string{
		"/proc":    "proc",
		"/sys":     "sysfs",
		"/dev/pts": "devpts",
		"/dev/shm": "tmpfs",
	}

	defaultSymlinks = map[string]string{
		"/dev/fd":     "/proc/self/fd",
		"/dev/stdin":  "/proc/self/fd/0",
		"/dev/stdout": "/proc/self/fd/1",
		"/dev/stderr": "/proc/self/fd/2",
	}

	defaultDevices = []string{
		"/dev/null",
		"/dev/zero",
		"/dev/full",
		"/dev/random",
		"/dev/urandom",
		"/dev/tty",
		"/dev/ptmx",
	}
)

type validation struct {
	test        func(*rspec.Spec) error
	description string
}

func loadSpecConfig() (spec *rspec.Spec, err error) {
	cf, err := os.Open(specConfig)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%s not found", specConfig)
		}
	}
	defer cf.Close()

	if err = json.NewDecoder(cf).Decode(&spec); err != nil {
		return
	}
	return spec, nil
}

// should be included by other platform specified process validation
func validateGeneralProcess(spec *rspec.Spec) error {
	if spec.Process.Cwd != "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		if cwd != spec.Process.Cwd {
			return fmt.Errorf("Cwd expected: %v, actual: %v", spec.Process.Cwd, cwd)
		}
	}

	for _, env := range spec.Process.Env {
		parts := strings.Split(env, "=")
		key := parts[0]
		expectedValue := parts[1]
		actualValue := os.Getenv(key)
		if actualValue != expectedValue {
			return fmt.Errorf("Env %v expected: %v, actual: %v", key, expectedValue, actualValue)
		}
	}

	return nil
}

func validateLinuxProcess(spec *rspec.Spec) error {
	logrus.Debugf("validating container process")

	validateGeneralProcess(spec)

	uid := os.Getuid()
	if uint32(uid) != spec.Process.User.UID {
		return fmt.Errorf("UID expected: %v, actual: %v", spec.Process.User.UID, uid)
	}
	gid := os.Getgid()
	if uint32(gid) != spec.Process.User.GID {
		return fmt.Errorf("GID expected: %v, actual: %v", spec.Process.User.GID, gid)
	}

	groups, err := os.Getgroups()
	if err != nil {
		return err
	}

	groupsMap := make(map[int]bool)
	for _, g := range groups {
		groupsMap[g] = true
	}

	for _, g := range spec.Process.User.AdditionalGids {
		if !groupsMap[int(g)] {
			return fmt.Errorf("Groups expected: %v, actual (should be superset): %v", spec.Process.User.AdditionalGids, groups)
		}
	}

	cmdlineBytes, err := ioutil.ReadFile("/proc/1/cmdline")
	if err != nil {
		return err
	}

	args := bytes.Split(bytes.Trim(cmdlineBytes, "\x00"), []byte("\x00"))
	if len(args) != len(spec.Process.Args) {
		return fmt.Errorf("Process arguments expected: %v, actual: %v", len(spec.Process.Args), len(args))
	}
	for i, a := range args {
		if string(a) != spec.Process.Args[i] {
			return fmt.Errorf("Process arguments expected: %v, actual: %v", string(a), spec.Process.Args[i])
		}
	}

	ret, _, errno := syscall.Syscall6(syscall.SYS_PRCTL, PR_GET_NO_NEW_PRIVS, 0, 0, 0, 0, 0)
	if errno != 0 {
		return errno
	}
	if spec.Process.NoNewPrivileges && ret != 1 {
		return fmt.Errorf("NoNewPrivileges expected: true, actual: false")
	}
	if !spec.Process.NoNewPrivileges && ret != 0 {
		return fmt.Errorf("NoNewPrivileges expected: false, actual: true")
	}

	return nil
}

func validateCapabilities(spec *rspec.Spec) error {
	logrus.Debugf("validating capabilities")

	last := capability.CAP_LAST_CAP
	// workaround for RHEL6 which has no /proc/sys/kernel/cap_last_cap
	if last == capability.Cap(63) {
		last = capability.CAP_BLOCK_SUSPEND
	}

	processCaps, err := capability.NewPid(1)
	if err != nil {
		return err
	}

	expectedCaps := make(map[string]bool)
	for _, ec := range spec.Process.Capabilities {
		expectedCaps[ec] = true
	}

	for _, cap := range capability.List() {
		if cap > last {
			continue
		}

		capKey := fmt.Sprintf("CAP_%s", strings.ToUpper(cap.String()))
		expectedSet := expectedCaps[capKey]
		actuallySet := processCaps.Get(capability.EFFECTIVE, cap)
		if expectedSet != actuallySet {
			if expectedSet {
				return fmt.Errorf("Expected Capability %v not set for process", cap.String())
			}
			return fmt.Errorf("Unexpected Capability %v set for process", cap.String())
		}
	}

	return nil
}

func validateHostname(spec *rspec.Spec) error {
	logrus.Debugf("validating hostname")
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	if spec.Hostname != "" && hostname != spec.Hostname {
		return fmt.Errorf("Hostname expected: %v, actual: %v", spec.Hostname, hostname)
	}
	return nil
}

func validateRlimits(spec *rspec.Spec) error {
	logrus.Debugf("validating rlimits")
	for _, r := range spec.Process.Rlimits {
		rl, err := strToRlimit(r.Type)
		if err != nil {
			return err
		}

		var rlimit syscall.Rlimit
		if err := syscall.Getrlimit(rl, &rlimit); err != nil {
			return err
		}

		if rlimit.Cur != r.Soft {
			return fmt.Errorf("%v rlimit soft expected: %v, actual: %v", r.Type, r.Soft, rlimit.Cur)
		}
		if rlimit.Max != r.Hard {
			return fmt.Errorf("%v rlimit hard expected: %v, actual: %v", r.Type, r.Hard, rlimit.Max)
		}
	}
	return nil
}

func validateSysctls(spec *rspec.Spec) error {
	logrus.Debugf("validating sysctls")
	for k, v := range spec.Linux.Sysctl {
		keyPath := filepath.Join("/proc/sys", strings.Replace(k, ".", "/", -1))
		vBytes, err := ioutil.ReadFile(keyPath)
		if err != nil {
			return err
		}
		value := strings.TrimSpace(string(bytes.Trim(vBytes, "\x00")))
		if value != v {
			return fmt.Errorf("Sysctl %v value expected: %v, actual: %v", k, v, value)
		}
	}
	return nil
}

func testWriteAccess(path string) error {
	tmpfile, err := ioutil.TempFile(path, "Test")
	if err != nil {
		return err
	}

	tmpfile.Close()
	os.RemoveAll(filepath.Join(path, tmpfile.Name()))

	return nil
}

func validateRootFS(spec *rspec.Spec) error {
	logrus.Debugf("validating root filesystem")
	if spec.Root.Readonly {
		err := testWriteAccess("/")
		if err == nil {
			return fmt.Errorf("Rootfs should be readonly")
		}
	}

	return nil
}

func validateDefaultFS(spec *rspec.Spec) error {
	logrus.Debugf("validating linux default filesystem")

	mountInfos, err := mount.GetMounts()
	if err != nil {
		return err
	}

	mountsMap := make(map[string]string)
	for _, mountInfo := range mountInfos {
		mountsMap[mountInfo.Mountpoint] = mountInfo.Fstype
	}

	for fs, fstype := range defaultFS {
		if !(mountsMap[fs] == fstype) {
			return fmt.Errorf("%v must exist and expected type is %v", fs, fstype)
		}
	}

	return nil
}

func validateLinuxDevices(spec *rspec.Spec) error {
	logrus.Debugf("validating linux devices")

	for _, device := range spec.Linux.Devices {
		fi, err := os.Stat(device.Path)
		if err != nil {
			return err
		}
		fStat, ok := fi.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("cannot determine state for device %s", device.Path)
		}
		var devType string
		switch fStat.Mode & syscall.S_IFMT {
		case syscall.S_IFCHR:
			devType = "c"
			break
		case syscall.S_IFBLK:
			devType = "b"
			break
		case syscall.S_IFIFO:
			devType = "p"
			break
		default:
			devType = "unmatched"
		}
		if devType != device.Type || (devType == "c" && device.Type == "u") {
			return fmt.Errorf("device %v expected type is %v, actual is %v", device.Path, device.Type, devType)
		}
		if devType != "p" {
			dev := fStat.Rdev
			major := (dev >> 8) & 0xfff
			minor := (dev & 0xff) | ((dev >> 12) & 0xfff00)
			if int64(major) != device.Major || int64(minor) != device.Minor {
				return fmt.Errorf("%v device number expected is %v:%v, actual is %v:%v", device.Path, device.Major, device.Minor, major, minor)
			}
		}
		if device.FileMode != nil {
			expectedPerm := *device.FileMode & os.ModePerm
			actualPerm := fi.Mode() & os.ModePerm
			if expectedPerm != actualPerm {
				return fmt.Errorf("%v filemode expected is %v, actual is %v", device.Path, expectedPerm, actualPerm)
			}
		}
		if device.UID != nil {
			if *device.UID != fStat.Uid {
				return fmt.Errorf("%v uid expected is %v, actual is %v", device.Path, *device.UID, fStat.Uid)
			}
		}
		if device.GID != nil {
			if *device.GID != fStat.Gid {
				return fmt.Errorf("%v uid expected is %v, actual is %v", device.Path, *device.GID, fStat.Gid)
			}
		}
	}

	return nil
}

func validateDefaultSymlinks(spec *rspec.Spec) error {
	logrus.Debugf("validating linux default symbolic links")

	for symlink, dest := range defaultSymlinks {
		fi, err := os.Lstat(symlink)
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeSymlink != os.ModeSymlink {
			return fmt.Errorf("%v is not a symbolic link as expected", symlink)
		}
		realDest, err := os.Readlink(symlink)
		if err != nil {
			return err
		}
		if realDest != dest {
			return fmt.Errorf("link destation of %v expected is %v, actual is %v", symlink, dest, realDest)
		}
	}

	return nil
}

func validateDefaultDevices(spec *rspec.Spec) error {
	logrus.Debugf("validating linux default devices")

	if spec.Process.Terminal {
		defaultDevices = append(defaultDevices, "/dev/console")
	}
	for _, device := range defaultDevices {
		fi, err := os.Stat(device)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("device node %v not found", device)
			}
			return err
		}
		if fi.Mode()&os.ModeDevice != os.ModeDevice {
			return fmt.Errorf("file %v is not a device as expected", device)
		}
	}

	return nil
}

func validateMaskedPaths(spec *rspec.Spec) error {
	logrus.Debugf("validating maskedPaths")
	for _, maskedPath := range spec.Linux.MaskedPaths {
		f, err := os.Open(maskedPath)
		if err != nil {
			return err
		}
		defer f.Close()
		b := make([]byte, 1)
		_, err = f.Read(b)
		if err != io.EOF {
			return fmt.Errorf("%v should not be readable", maskedPath)
		}
	}
	return nil
}

func validateROPaths(spec *rspec.Spec) error {
	logrus.Debugf("validating readonlyPaths")
	for _, v := range spec.Linux.ReadonlyPaths {
		err := testWriteAccess(v)
		if err == nil {
			return fmt.Errorf("%v should be readonly", v)
		}
	}

	return nil
}

func validateOOMScoreAdj(spec *rspec.Spec) error {
	logrus.Debugf("validating oomScoreAdj")
	if spec.Linux.Resources != nil && spec.Linux.Resources.OOMScoreAdj != nil {
		expected := *spec.Linux.Resources.OOMScoreAdj
		f, err := os.Open("/proc/1/oom_score_adj")
		if err != nil {
			return err
		}
		defer f.Close()

		s := bufio.NewScanner(f)
		for s.Scan() {
			if err := s.Err(); err != nil {
				return err
			}
			text := strings.TrimSpace(s.Text())
			actual, err := strconv.Atoi(text)
			if err != nil {
				return err
			}
			if actual != expected {
				return fmt.Errorf("oomScoreAdj expected: %v, actual: %v", expected, actual)
			}
		}
	}

	return nil
}

func getIDMappings(path string) ([]rspec.IDMapping, error) {
	var idMaps []rspec.IDMapping
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		if err := s.Err(); err != nil {
			return nil, err
		}

		idMap := strings.Fields(strings.TrimSpace(s.Text()))
		if len(idMap) == 3 {
			hostID, err := strconv.ParseUint(idMap[0], 0, 32)
			if err != nil {
				return nil, err
			}
			containerID, err := strconv.ParseUint(idMap[1], 0, 32)
			if err != nil {
				return nil, err
			}
			mapSize, err := strconv.ParseUint(idMap[2], 0, 32)
			if err != nil {
				return nil, err
			}
			idMaps = append(idMaps, rspec.IDMapping{HostID: uint32(hostID), ContainerID: uint32(containerID), Size: uint32(mapSize)})
		} else {
			return nil, fmt.Errorf("invalid format in %v", path)
		}
	}

	return idMaps, nil
}

func validateIDMappings(mappings []rspec.IDMapping, path string, property string) error {
	idMaps, err := getIDMappings(path)
	if err != nil {
		return fmt.Errorf("can not get items: %v", err)
	}
	if len(mappings) != 0 && len(mappings) != len(idMaps) {
		return fmt.Errorf("expected %d entries in %v, but acutal is %d", len(mappings), path, len(idMaps))
	}
	for _, v := range mappings {
		exist := false
		for _, cv := range idMaps {
			if v.HostID == cv.HostID && v.ContainerID == cv.ContainerID && v.Size == cv.Size {
				exist = true
				break
			}
		}
		if !exist {
			return fmt.Errorf("%v is not applied as expected", property)
		}
	}

	return nil
}

func validateUIDMappings(spec *rspec.Spec) error {
	logrus.Debugf("validating uidMappings")

	return validateIDMappings(spec.Linux.UIDMappings, "/proc/self/uid_map", "linux.uidMappings")
}

func validateGIDMappings(spec *rspec.Spec) error {
	logrus.Debugf("validating gidMappings")

	return validateIDMappings(spec.Linux.GIDMappings, "/proc/self/gid_map", "linux.gidMappings")
}

func mountMatch(specMount rspec.Mount, sysMount rspec.Mount) error {
	if filepath.Clean(specMount.Destination) != sysMount.Destination {
		return fmt.Errorf("mount destination expected: %v, actual: %v", specMount.Destination, sysMount.Destination)
	}

	if specMount.Type != sysMount.Type {
		return fmt.Errorf("mount %v type expected: %v, actual: %v", specMount.Destination, specMount.Type, sysMount.Type)
	}

	if filepath.Clean(specMount.Source) != sysMount.Source {
		return fmt.Errorf("mount %v source expected: %v, actual: %v", specMount.Destination, specMount.Source, sysMount.Source)
	}

	return nil
}

func validateMountsExist(spec *rspec.Spec) error {
	logrus.Debugf("validating mounts exist")

	mountInfos, err := mount.GetMounts()
	if err != nil {
		return err
	}

	mountsMap := make(map[string][]rspec.Mount)
	for _, mountInfo := range mountInfos {
		m := rspec.Mount{
			Destination: mountInfo.Mountpoint,
			Type:        mountInfo.Fstype,
			Source:      mountInfo.Source,
		}
		mountsMap[mountInfo.Mountpoint] = append(mountsMap[mountInfo.Mountpoint], m)
	}

	for _, specMount := range spec.Mounts {
		found := false
		for _, sysMount := range mountsMap[filepath.Clean(specMount.Destination)] {
			if err := mountMatch(specMount, sysMount); err == nil {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("Expected mount %v does not exist", specMount)
		}
	}

	return nil
}

func validate(context *cli.Context) error {
	logLevelString := context.String("log-level")
	logLevel, err := logrus.ParseLevel(logLevelString)
	if err != nil {
		return err
	}
	logrus.SetLevel(logLevel)

	spec, err := loadSpecConfig()
	if err != nil {
		return err
	}

	defaultValidations := []validation{
		{
			test:        validateRootFS,
			description: "root filesystem",
		},
		{
			test:        validateHostname,
			description: "hostname",
		},
		{
			test:        validateMountsExist,
			description: "mounts",
		},
	}

	linuxValidations := []validation{
		{
			test:        validateCapabilities,
			description: "capabilities",
		},
		{
			test:        validateDefaultSymlinks,
			description: "default symlinks",
		},
		{
			test:        validateDefaultFS,
			description: "default file system",
		},
		{
			test:        validateDefaultDevices,
			description: "default devices",
		},
		{
			test:        validateLinuxDevices,
			description: "linux devices",
		},
		{
			test:        validateLinuxProcess,
			description: "linux process",
		},
		{
			test:        validateMaskedPaths,
			description: "masked paths",
		},
		{
			test:        validateOOMScoreAdj,
			description: "oom score adj",
		},
		{
			test:        validateROPaths,
			description: "read only paths",
		},
		{
			test:        validateRlimits,
			description: "rlimits",
		},
		{
			test:        validateSysctls,
			description: "sysctls",
		},
		{
			test:        validateUIDMappings,
			description: "uid mappings",
		},
		{
			test:        validateGIDMappings,
			description: "gid mappings",
		},
	}

	t := tap.New()
	t.Header(0)

	var validationErrors error
	for _, v := range defaultValidations {
		err := v.test(spec)
		t.Ok(err == nil, v.description)
		if err != nil {
			validationErrors = multierror.Append(validationErrors, err)
		}
	}

	if spec.Platform.OS == "linux" {
		for _, v := range linuxValidations {
			err := v.test(spec)
			t.Ok(err == nil, v.description)
			if err != nil {
				validationErrors = multierror.Append(validationErrors, err)
			}
		}
	}
	t.AutoPlan()

	return validationErrors
}

func main() {
	app := cli.NewApp()
	app.Name = "runtimetest"
	app.Version = "0.0.1"
	app.Usage = "Compare the environment with an OCI configuration"
	app.Description = "runtimetest compares its current environment with an OCI runtime configuration read from config.json in its current working directory.  The tests are fairly generic and cover most configurations used by the runtime validation suite, but there are corner cases where a container launched by a valid runtime would not satisfy runtimetest."
	app.UsageText = "runtimetest [options]"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "log-level",
			Value: "error",
			Usage: "Log level (panic, fatal, error, warn, info, or debug)",
		},
	}

	app.Action = validate
	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}
