package nsmgr

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/containers/storage/pkg/idtools"
	"github.com/cri-o/cri-o/utils"
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
		pinnsArgs = append(pinnsArgs, "-s", getSysctlForPinns(cfg.Sysctls))
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
	output, err := exec.Command(mgr.pinnsPath, pinnsArgs...).CombinedOutput()
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

func getSysctlForPinns(sysctls map[string]string) string {
	// this assumes there's no sysctl with a `+` in it
	const pinnsSysctlDelim = "+"
	g := new(bytes.Buffer)
	for key, value := range sysctls {
		fmt.Fprintf(g, "'%s=%s'%s", key, value, pinnsSysctlDelim)
	}
	return strings.TrimSuffix(g.String(), pinnsSysctlDelim)
}

// dirForType returns the sub-directory for that particular NSType
// which is of the form `$namespaceDir/$nsType+"ns"`
func (mgr *NamespaceManager) dirForType(ns NSType) string {
	return filepath.Join(mgr.namespacesDir, string(ns)+"ns")
}
