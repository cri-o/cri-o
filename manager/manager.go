package manager

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/registrar"
	"github.com/docker/docker/pkg/truncindex"
	"github.com/kubernetes-incubator/cri-o/manager/apparmor"
	"github.com/kubernetes-incubator/cri-o/manager/seccomp"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/opencontainers/runc/libcontainer/label"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/rajatchopra/ocicni"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// Manager implements the RuntimeService and ImageService
type Manager struct {
	config       Config
	runtime      *oci.Runtime
	stateLock    sync.Mutex
	state        *managerState
	netPlugin    ocicni.CNIPlugin
	podNameIndex *registrar.Registrar
	podIDIndex   *truncindex.TruncIndex
	ctrNameIndex *registrar.Registrar
	ctrIDIndex   *truncindex.TruncIndex

	seccompEnabled bool
	seccompProfile seccomp.Seccomp

	appArmorEnabled bool
	appArmorProfile string
}

func (m *Manager) loadContainer(id string) error {
	config, err := ioutil.ReadFile(filepath.Join(m.runtime.ContainerDir(), id, "config.json"))
	if err != nil {
		return err
	}
	var s rspec.Spec
	if err = json.Unmarshal(config, &s); err != nil {
		return err
	}
	labels := make(map[string]string)
	if err = json.Unmarshal([]byte(s.Annotations["ocid/labels"]), &labels); err != nil {
		return err
	}
	name := s.Annotations["ocid/name"]
	name, err = m.reserveContainerName(id, name)
	if err != nil {
		return err
	}
	var metadata pb.ContainerMetadata
	if err = json.Unmarshal([]byte(s.Annotations["ocid/metadata"]), &metadata); err != nil {
		return err
	}
	sb := m.getSandbox(s.Annotations["ocid/sandbox_id"])
	if sb == nil {
		logrus.Warnf("could not get sandbox with id %s, skipping", s.Annotations["ocid/sandbox_id"])
		return nil
	}

	var tty bool
	if v := s.Annotations["ocid/tty"]; v == "true" {
		tty = true
	}
	containerPath := filepath.Join(m.runtime.ContainerDir(), id)

	var img *pb.ImageSpec
	image, ok := s.Annotations["ocid/image"]
	if ok {
		img = &pb.ImageSpec{
			Image: &image,
		}
	}

	annotations := make(map[string]string)
	if err = json.Unmarshal([]byte(s.Annotations["ocid/annotations"]), &annotations); err != nil {
		return err
	}

	ctr, err := oci.NewContainer(id, name, containerPath, s.Annotations["ocid/log_path"], sb.netNs(), labels, annotations, img, &metadata, sb.id, tty)
	if err != nil {
		return err
	}
	m.addContainer(ctr)
	if err = m.runtime.UpdateStatus(ctr); err != nil {
		logrus.Warnf("error updating status for container %s: %v", ctr.ID(), err)
	}
	if err = m.ctrIDIndex.Add(id); err != nil {
		return err
	}
	return nil
}

func configNetNsPath(spec rspec.Spec) (string, error) {
	for _, ns := range spec.Linux.Namespaces {
		if ns.Type != rspec.NetworkNamespace {
			continue
		}

		if ns.Path == "" {
			return "", fmt.Errorf("empty networking namespace")
		}

		return ns.Path, nil
	}

	return "", fmt.Errorf("missing networking namespace")
}

func (m *Manager) loadSandbox(id string) error {
	config, err := ioutil.ReadFile(filepath.Join(m.config.SandboxDir, id, "config.json"))
	if err != nil {
		return err
	}
	var s rspec.Spec
	if err = json.Unmarshal(config, &s); err != nil {
		return err
	}
	labels := make(map[string]string)
	if err = json.Unmarshal([]byte(s.Annotations["ocid/labels"]), &labels); err != nil {
		return err
	}
	name := s.Annotations["ocid/name"]
	name, err = m.reservePodName(id, name)
	if err != nil {
		return err
	}
	var metadata pb.PodSandboxMetadata
	if err = json.Unmarshal([]byte(s.Annotations["ocid/metadata"]), &metadata); err != nil {
		return err
	}

	processLabel, mountLabel, err := label.InitLabels(label.DupSecOpt(s.Process.SelinuxLabel))
	if err != nil {
		return err
	}

	annotations := make(map[string]string)
	if err = json.Unmarshal([]byte(s.Annotations["ocid/annotations"]), &annotations); err != nil {
		return err
	}

	sb := &sandbox{
		id:           id,
		name:         name,
		logDir:       s.Annotations["ocid/log_path"],
		labels:       labels,
		containers:   oci.NewMemoryStore(),
		processLabel: processLabel,
		mountLabel:   mountLabel,
		annotations:  annotations,
		metadata:     &metadata,
		shmPath:      s.Annotations["ocid/shm_path"],
	}

	// We add a netNS only if we can load a permanent one.
	// Otherwise, the sandbox will live in the host namespace.
	netNsPath, err := configNetNsPath(s)
	if err == nil {
		netNS, nsErr := netNsGet(netNsPath, sb.name)
		// If we can't load the networking namespace
		// because it's closed, we just set the sb netns
		// pointer to nil. Otherwise we return an error.
		if nsErr != nil && nsErr != errSandboxClosedNetNS {
			return nsErr
		}

		sb.netns = netNS
	}

	m.addSandbox(sb)

	sandboxPath := filepath.Join(m.config.SandboxDir, id)

	if err = label.ReserveLabel(processLabel); err != nil {
		return err
	}

	cname, err := m.reserveContainerName(s.Annotations["ocid/container_id"], s.Annotations["ocid/container_name"])
	if err != nil {
		return err
	}
	scontainer, err := oci.NewContainer(s.Annotations["ocid/container_id"], cname, sandboxPath, sandboxPath, sb.netNs(), labels, annotations, nil, nil, id, false)
	if err != nil {
		return err
	}
	sb.infraContainer = scontainer
	if err = m.runtime.UpdateStatus(scontainer); err != nil {
		logrus.Warnf("error updating status for container %s: %v", scontainer.ID(), err)
	}
	if err = m.ctrIDIndex.Add(scontainer.ID()); err != nil {
		return err
	}
	if err = m.podIDIndex.Add(id); err != nil {
		return err
	}
	return nil
}

func (m *Manager) restore() {
	sandboxDir, err := ioutil.ReadDir(m.config.SandboxDir)
	if err != nil && !os.IsNotExist(err) {
		logrus.Warnf("could not read sandbox directory %s: %v", sandboxDir, err)
	}
	for _, v := range sandboxDir {
		if !v.IsDir() {
			continue
		}
		if err = m.loadSandbox(v.Name()); err != nil {
			logrus.Warnf("could not restore sandbox %s: %v", v.Name(), err)
		}
	}
	containerDir, err := ioutil.ReadDir(m.runtime.ContainerDir())
	if err != nil && !os.IsNotExist(err) {
		logrus.Warnf("could not read container directory %s: %v", m.runtime.ContainerDir(), err)
	}
	for _, v := range containerDir {
		if !v.IsDir() {
			continue
		}
		if err := m.loadContainer(v.Name()); err != nil {
			logrus.Warnf("could not restore container %s: %v", v.Name(), err)

		}
	}
}

func (m *Manager) reservePodName(id, name string) (string, error) {
	if err := m.podNameIndex.Reserve(name, id); err != nil {
		if err == registrar.ErrNameReserved {
			id, err := m.podNameIndex.Get(name)
			if err != nil {
				logrus.Warnf("conflict, pod name %q already reserved", name)
				return "", err
			}
			return "", fmt.Errorf("conflict, name %q already reserved for pod %q", name, id)
		}
		return "", fmt.Errorf("error reserving pod name %q", name)
	}
	return name, nil
}

func (m *Manager) releasePodName(name string) {
	m.podNameIndex.Release(name)
}

func (m *Manager) reserveContainerName(id, name string) (string, error) {
	if err := m.ctrNameIndex.Reserve(name, id); err != nil {
		if err == registrar.ErrNameReserved {
			id, err := m.ctrNameIndex.Get(name)
			if err != nil {
				logrus.Warnf("conflict, ctr name %q already reserved", name)
				return "", err
			}
			return "", fmt.Errorf("conflict, name %q already reserved for ctr %q", name, id)
		}
		return "", fmt.Errorf("error reserving ctr name %s", name)
	}
	return name, nil
}

func (m *Manager) releaseContainerName(name string) {
	m.ctrNameIndex.Release(name)
}

const (
	// SeccompModeFilter refers to the syscall argument SECCOMP_MODE_FILTER.
	SeccompModeFilter = uintptr(2)
)

func seccompEnabled() bool {
	var enabled bool
	// Check if Seccomp is supported, via CONFIG_SECCOMP.
	if _, _, err := syscall.RawSyscall(syscall.SYS_PRCTL, syscall.PR_GET_SECCOMP, 0, 0); err != syscall.EINVAL {
		// Make sure the kernel has CONFIG_SECCOMP_FILTER.
		if _, _, err := syscall.RawSyscall(syscall.SYS_PRCTL, syscall.PR_SET_SECCOMP, SeccompModeFilter, 0); err != syscall.EINVAL {
			enabled = true
		}
	}
	return enabled
}

// New creates a new Manager with options provided
func New(config *Config) (*Manager, error) {
	if err := os.MkdirAll(config.ImageDir, 0755); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(config.SandboxDir, 0755); err != nil {
		return nil, err
	}

	r, err := oci.New(config.Runtime, config.ContainerDir, config.Conmon, config.ConmonEnv)
	if err != nil {
		return nil, err
	}
	sandboxes := make(map[string]*sandbox)
	containers := oci.NewMemoryStore()
	netPlugin, err := ocicni.InitCNI("")
	if err != nil {
		return nil, err
	}
	m := &Manager{
		runtime:   r,
		netPlugin: netPlugin,
		config:    *config,
		state: &managerState{
			sandboxes:  sandboxes,
			containers: containers,
		},
		seccompEnabled:  seccompEnabled(),
		appArmorEnabled: apparmor.IsEnabled(),
		appArmorProfile: config.ApparmorProfile,
	}
	seccompProfile, err := ioutil.ReadFile(config.SeccompProfile)
	if err != nil {
		return nil, fmt.Errorf("opening seccomp profile (%s) failed: %v", config.SeccompProfile, err)
	}
	var seccompConfig seccomp.Seccomp
	if err := json.Unmarshal(seccompProfile, &seccompConfig); err != nil {
		return nil, fmt.Errorf("decoding seccomp profile failed: %v", err)
	}
	m.seccompProfile = seccompConfig

	if m.appArmorEnabled && m.appArmorProfile == apparmor.DefaultApparmorProfile {
		if err := apparmor.EnsureDefaultApparmorProfile(); err != nil {
			return nil, fmt.Errorf("ensuring the default apparmor profile is installed failed: %v", err)
		}
	}

	m.podIDIndex = truncindex.NewTruncIndex([]string{})
	m.podNameIndex = registrar.NewRegistrar()
	m.ctrIDIndex = truncindex.NewTruncIndex([]string{})
	m.ctrNameIndex = registrar.NewRegistrar()

	m.restore()

	logrus.Debugf("sandboxes: %v", m.state.sandboxes)
	logrus.Debugf("containers: %v", m.state.containers)
	return m, nil
}

type managerState struct {
	sandboxes  map[string]*sandbox
	containers oci.Store
}

func (m *Manager) addSandbox(sb *sandbox) {
	m.stateLock.Lock()
	m.state.sandboxes[sb.id] = sb
	m.stateLock.Unlock()
}

func (m *Manager) getSandbox(id string) *sandbox {
	m.stateLock.Lock()
	sb := m.state.sandboxes[id]
	m.stateLock.Unlock()
	return sb
}

func (m *Manager) hasSandbox(id string) bool {
	m.stateLock.Lock()
	_, ok := m.state.sandboxes[id]
	m.stateLock.Unlock()
	return ok
}

func (m *Manager) removeSandbox(id string) {
	m.stateLock.Lock()
	delete(m.state.sandboxes, id)
	m.stateLock.Unlock()
}

func (m *Manager) addContainer(c *oci.Container) {
	m.stateLock.Lock()
	sandbox := m.state.sandboxes[c.Sandbox()]
	// TODO(runcom): handle !ok above!!! otherwise it panics!
	sandbox.addContainer(c)
	m.state.containers.Add(c.ID(), c)
	m.stateLock.Unlock()
}

func (m *Manager) getContainer(id string) *oci.Container {
	m.stateLock.Lock()
	c := m.state.containers.Get(id)
	m.stateLock.Unlock()
	return c
}

func (m *Manager) removeContainer(c *oci.Container) {
	m.stateLock.Lock()
	sandbox := m.state.sandboxes[c.Sandbox()]
	sandbox.removeContainer(c)
	m.state.containers.Delete(c.ID())
	m.stateLock.Unlock()
}
