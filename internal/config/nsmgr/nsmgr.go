package nsmgr

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	nspkg "github.com/containernetworking/plugins/pkg/ns"
	"github.com/containers/storage/pkg/idtools"
	"github.com/cri-o/cri-o/utils"
	"github.com/cri-o/cri-o/utils/cmdrunner"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// NamespaceManager manages the server's namespaces.
// Specifically, it is an interface for how the server is creating namespaces,
// and can be requested to create namespaces for a pod.
type NamespaceManager struct {
	namespacesDir string
	pinnsPath     string
}

// New creates a new NamespaceManager.
func New(namespacesDir, pinnsPath string) *NamespaceManager {
	return &NamespaceManager{
		namespacesDir: namespacesDir,
		pinnsPath:     pinnsPath,
	}
}

func (mgr *NamespaceManager) Initialize() error {
	if err := os.MkdirAll(mgr.namespacesDir, 0o755); err != nil {
		return errors.Wrap(err, "invalid namespaces_dir")
	}

	for _, ns := range supportedNamespacesForPinning() {
		nsDir := mgr.dirForType(ns)
		if err := utils.IsDirectory(nsDir); err != nil {
			// The file is not a directory, but exists.
			// We should remove it.
			if errors.Is(err, syscall.ENOTDIR) {
				if err := os.Remove(nsDir); err != nil {
					return errors.Wrapf(err, "remove file to create namespaces sub-dir")
				}
				logrus.Infof("Removed file %s to create directory in that path.", nsDir)
			} else if !os.IsNotExist(err) {
				// if it's neither an error because the file exists
				// nor an error because it does not exist, it is
				// some other disk error.
				return errors.Wrapf(err, "checking whether namespaces sub-dir exists")
			}
			if err := os.MkdirAll(nsDir, 0o755); err != nil {
				return errors.Wrap(err, "invalid namespaces sub-dir")
			}
		}
	}
	return nil
}

// NewPodNamespaces creates new namespaces for a pod.
// It's responsible for running pinns and creating the Namespace objects.
// The caller is responsible for cleaning up the namespaces by calling Namespace.Remove().
func (mgr *NamespaceManager) NewPodNamespaces(cfg *PodNamespacesConfig) ([]Namespace, error) {
	if cfg == nil {
		return nil, errors.New("PodNamespacesConfig cannot be nil")
	}
	if len(cfg.Namespaces) == 0 {
		return []Namespace{}, nil
	}

	typeToArg := map[NSType]string{
		IPCNS:  "--ipc",
		UTSNS:  "--uts",
		USERNS: "--user",
		NETNS:  "--net",
	}

	pinnedNamespace := uuid.New().String()
	pinnsArgs := []string{
		"-d", mgr.namespacesDir,
		"-f", pinnedNamespace,
	}

	if len(cfg.Sysctls) != 0 {
		pinnsSysctls, err := getSysctlForPinns(cfg.Sysctls)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid sysctl")
		}
		pinnsArgs = append(pinnsArgs, "-s", pinnsSysctls)
	}

	var rootPair idtools.IDPair
	if cfg.IDMappings != nil {
		rootPair = cfg.IDMappings.RootPair()
	}

	for _, ns := range cfg.Namespaces {
		arg, ok := typeToArg[ns.Type]
		if !ok {
			return nil, errors.Errorf("Invalid namespace type: %s", ns.Type)
		}
		if ns.Host {
			arg += "=host"
		}
		pinnsArgs = append(pinnsArgs, arg)
		ns.Path = filepath.Join(mgr.namespacesDir, string(ns.Type)+"ns", pinnedNamespace)
		if cfg.IDMappings != nil {
			if err := chownDirToIDPair(ns.Path, rootPair); err != nil {
				return nil, err
			}
		}
	}

	if cfg.IDMappings != nil {
		pinnsArgs = append(pinnsArgs,
			"--uid-mapping="+getMappingsForPinns(cfg.IDMappings.UIDs()),
			"--gid-mapping="+getMappingsForPinns(cfg.IDMappings.GIDs()))
	}

	logrus.Debugf("Calling pinns with %v", pinnsArgs)
	output, err := cmdrunner.Command(mgr.pinnsPath, pinnsArgs...).CombinedOutput()
	if err != nil {
		logrus.Warnf("Pinns %v failed: %s (%v)", pinnsArgs, string(output), err)
		// cleanup the mounts
		for _, ns := range cfg.Namespaces {
			if mErr := unix.Unmount(ns.Path, unix.MNT_DETACH); mErr != nil && mErr != unix.EINVAL {
				logrus.Warnf("Failed to unmount %s: %v", ns.Path, mErr)
			}
		}

		return nil, fmt.Errorf("failed to pin namespaces %v: %s %v", cfg.Namespaces, output, err)
	}

	returnedNamespaces := make([]Namespace, 0, len(cfg.Namespaces))
	for _, ns := range cfg.Namespaces {
		ns, err := GetNamespace(ns.Path, ns.Type)
		if err != nil {
			for _, nsToClose := range returnedNamespaces {
				if err2 := nsToClose.Remove(); err2 != nil {
					logrus.Errorf("Failed to remove namespace after failed to create: %v", err2)
				}
			}
			return nil, err
		}

		returnedNamespaces = append(returnedNamespaces, ns)
	}
	return returnedNamespaces, nil
}

func chownDirToIDPair(pinPath string, rootPair idtools.IDPair) error {
	if err := os.MkdirAll(filepath.Dir(pinPath), 0o755); err != nil {
		return err
	}
	f, err := os.Create(pinPath)
	if err != nil {
		return err
	}
	f.Close()

	return os.Chown(pinPath, rootPair.UID, rootPair.GID)
}

func getMappingsForPinns(mappings []idtools.IDMap) string {
	g := new(bytes.Buffer)
	for _, m := range mappings {
		fmt.Fprintf(g, "%d-%d-%d@", m.ContainerID, m.HostID, m.Size)
	}
	return g.String()
}

func getSysctlForPinns(sysctls map[string]string) (string, error) {
	// This assumes there's no valid sysctl value with a `+` in it
	// and as such errors if one is found.
	const pinnsSysctlDelim = "+"
	g := new(bytes.Buffer)
	for key, value := range sysctls {
		if strings.Contains(key, pinnsSysctlDelim) || strings.Contains(value, pinnsSysctlDelim) {
			return "", errors.Errorf("'%s=%s' is invalid: %s found yet should not be present", key, value, pinnsSysctlDelim)
		}
		fmt.Fprintf(g, "'%s=%s'%s", key, value, pinnsSysctlDelim)
	}
	return strings.TrimSuffix(g.String(), pinnsSysctlDelim), nil
}

// NamespaceFromProcEntry creates a new namespace object from a bind mount from a processes proc entry.
// The caller is responsible for cleaning up the namespace by calling Namespace.Remove().
// This function is heavily based on containernetworking ns package found at:
// https://github.com/containernetworking/plugins/blob/5c3c17164270150467498a32c71436c7cd5501be/pkg/ns/ns.go#L140
// Credit goes to the CNI authors.
func (mgr *NamespaceManager) NamespaceFromProcEntry(pid int, nsType NSType) (_ Namespace, retErr error) {
	// now create an empty file
	f, err := os.CreateTemp(mgr.dirForType(PIDNS), string(PIDNS))
	if err != nil {
		return nil, errors.Wrapf(err, "error creating namespace path")
	}
	pinnedNamespace := f.Name()
	f.Close()

	defer func() {
		if retErr != nil {
			if err := os.Remove(pinnedNamespace); err != nil {
				logrus.Errorf("Failed to remove namespace after failure to pin namespace: %v", err)
			}
		}
	}()

	podPidnsProc := NamespacePathFromProc(nsType, pid)
	// pid must have stopped or be incorrect, report error
	if podPidnsProc == "" {
		return nil, errors.Errorf("proc entry for pid %d is gone; pid not created or stopped", pid)
	}

	// bind mount the new ns from the proc entry onto the mount point
	if err := unix.Mount(podPidnsProc, pinnedNamespace, "none", unix.MS_BIND, ""); err != nil {
		return nil, errors.Wrapf(err, "error mounting %s namespace path", string(nsType))
	}
	defer func() {
		if retErr != nil {
			if err := unix.Unmount(pinnedNamespace, unix.MNT_DETACH); err != nil && err != unix.EINVAL {
				logrus.Errorf("Failed umount after failed to pin %s namespace: %v", string(nsType), err)
			}
		}
	}()

	return GetNamespace(pinnedNamespace, nsType)
}

// dirForType returns the sub-directory for that particular NSType
// which is of the form `$namespaceDir/$nsType+"ns"`
func (mgr *NamespaceManager) dirForType(ns NSType) string {
	return filepath.Join(mgr.namespacesDir, string(ns)+"ns")
}

// NamespacePathFromProc returns the namespace path of type nsType for a given pid and type.
func NamespacePathFromProc(nsType NSType, pid int) string {
	// verify nsPath exists on the host. This will prevent us from fatally erroring
	// on network tear down if the path doesn't exist
	// Technically, this is pretty racy, but so is every check using the infra container PID.
	nsPath := fmt.Sprintf("/proc/%d/ns/%s", pid, nsType)
	if _, err := os.Stat(nsPath); err != nil {
		return ""
	}
	// verify the path we found is indeed a namespace
	if err := nspkg.IsNSorErr(nsPath); err != nil {
		return ""
	}
	return nsPath
}
