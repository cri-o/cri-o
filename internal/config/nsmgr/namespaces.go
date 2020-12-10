// +build linux

package nsmgr

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/containers/storage/pkg/idtools"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

func (mgr *managedNamespaceManager) NewPodNamespaces(cfg *PodNamespacesConfig) ([]Namespace, error) {
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
			err := os.MkdirAll(filepath.Dir(ns.Path), 0o755)
			if err != nil {
				return nil, err
			}
			f, err := os.Create(ns.Path)
			if err != nil {
				return nil, err
			}
			f.Close()
			if err := os.Chown(ns.Path, rootPair.UID, rootPair.GID); err != nil {
				return nil, err
			}
		}
	}

	if cfg.IDMappings != nil {
		pinnsArgs = append(pinnsArgs,
			fmt.Sprintf("--uid-mapping=%s", getMappingsForPinns(cfg.IDMappings.UIDs())),
			fmt.Sprintf("--gid-mapping=%s", getMappingsForPinns(cfg.IDMappings.GIDs())))
	}

	logrus.Debugf("calling pinns with %v", pinnsArgs)
	output, err := exec.Command(mgr.pinnsPath, pinnsArgs...).CombinedOutput()
	if err != nil {
		logrus.Warnf("pinns %v failed: %s (%v)", pinnsArgs, string(output), err)
		// cleanup the mounts
		for _, ns := range cfg.Namespaces {
			if mErr := unix.Unmount(ns.Path, unix.MNT_DETACH); mErr != nil && mErr != unix.EINVAL {
				logrus.Warnf("failed to unmount %s: %v", ns.Path, mErr)
			}
		}

		// TODO FIXME
		return nil, fmt.Errorf("failed to pin namespaces %v: %s %v", pinnsArgs, output, err)
	}

	returnedNamespaces := make([]Namespace, 0, len(cfg.Namespaces))
	for _, ns := range cfg.Namespaces {
		ns, err := GetNamespace(ns.Path, ns.Type)
		if err != nil {
			return nil, err
		}

		returnedNamespaces = append(returnedNamespaces, ns)
	}
	return returnedNamespaces, nil
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
