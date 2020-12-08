package nsmgr

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	nspkg "github.com/containernetworking/plugins/pkg/ns"
	"github.com/containers/storage/pkg/idtools"
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

// NewPodNamespaces creates new namespaces for a pod.
// It's responsible for running pinns and creating the Namespace objects.
// The caller is responsible for cleaning up the namespaces by calling Namespace.Remove().
func (mgr *NamespaceManager) NewPodNamespaces(managedNamespaces []NSType, idMappings *idtools.IDMappings, sysctls map[string]string) ([]Namespace, error) {
	if len(managedNamespaces) == 0 {
		return []Namespace{}, nil
	}

	typeToArg := map[NSType]string{
		IPCNS:  "-i",
		UTSNS:  "-u",
		USERNS: "-U",
		NETNS:  "-n",
	}

	pinnedNamespace := uuid.New().String()
	pinnsArgs := []string{
		"-d", mgr.namespacesDir,
		"-f", pinnedNamespace,
	}

	if len(sysctls) != 0 {
		pinnsArgs = append(pinnsArgs, "-s", getSysctlForPinns(sysctls))
	}

	type namespaceInfo struct {
		path   string
		nsType NSType
	}

	mountedNamespaces := make([]namespaceInfo, 0, len(managedNamespaces))

	var rootPair idtools.IDPair
	if idMappings != nil {
		rootPair = idMappings.RootPair()
	}

	for _, nsType := range managedNamespaces {
		arg, ok := typeToArg[nsType]
		if !ok {
			return nil, errors.Errorf("Invalid namespace type: %s", nsType)
		}
		pinnsArgs = append(pinnsArgs, arg)
		pinPath := filepath.Join(mgr.namespacesDir, string(nsType)+"ns", pinnedNamespace)
		mountedNamespaces = append(mountedNamespaces, namespaceInfo{
			path:   pinPath,
			nsType: nsType,
		})
		if idMappings != nil {
			if err := chownDirToIDPair(pinPath, rootPair); err != nil {
				return nil, err
			}
		}
	}

	if idMappings != nil {
		pinnsArgs = append(pinnsArgs,
			"--uid-mapping="+getMappingsForPinns(idMappings.UIDs()),
			"--gid-mapping="+getMappingsForPinns(idMappings.GIDs()))
	}

	logrus.Debugf("calling pinns with %v", pinnsArgs)
	output, err := exec.Command(mgr.pinnsPath, pinnsArgs...).CombinedOutput()
	if err != nil {
		logrus.Warnf("pinns %v failed: %s (%v)", pinnsArgs, string(output), err)
		// cleanup the mounts
		for _, info := range mountedNamespaces {
			if mErr := unix.Unmount(info.path, unix.MNT_DETACH); mErr != nil && mErr != unix.EINVAL {
				logrus.Warnf("failed to unmount %s: %v", info.path, mErr)
			}
		}

		return nil, fmt.Errorf("failed to pin namespaces %v: %s %v", managedNamespaces, output, err)
	}

	returnedNamespaces := make([]Namespace, 0, len(managedNamespaces))
	for _, info := range mountedNamespaces {
		ret, err := nspkg.GetNS(info.path)
		if err != nil {
			return nil, err
		}

		returnedNamespaces = append(returnedNamespaces, &Namespace{
			ns:     ret.(NS),
			nsType: info.nsType,
			nsPath: info.path,
		})
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
