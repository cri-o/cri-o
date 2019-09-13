package server

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/cri-o/cri-o/lib/sandbox"
	"github.com/cri-o/cri-o/server/metrics"
	"github.com/cri-o/ocicni/pkg/ocicni"
	units "github.com/docker/go-units"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/runtime-tools/validate"
	"github.com/pkg/errors"
	"github.com/syndtr/gocapability/capability"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
	"k8s.io/kubernetes/pkg/kubelet/types"
)

const (
	// According to http://man7.org/linux/man-pages/man5/resolv.conf.5.html:
	// "The search list is currently limited to six domains with a total of 256 characters."
	maxDNSSearches = 6

	maxLabelSize = 4096
)

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func removeFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}

func parseDNSOptions(servers, searches, options []string, path string) error {
	nServers := len(servers)
	nSearches := len(searches)
	nOptions := len(options)
	if nServers == 0 && nSearches == 0 && nOptions == 0 {
		return copyFile("/etc/resolv.conf", path)
	}

	if nSearches > maxDNSSearches {
		return fmt.Errorf("DNSOption.Searches has more than 6 domains")
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if nSearches > 0 {
		data := fmt.Sprintf("search %s\n", strings.Join(searches, " "))
		_, err = f.Write([]byte(data))
		if err != nil {
			return err
		}
	}

	if nServers > 0 {
		data := fmt.Sprintf("nameserver %s\n", strings.Join(servers, "\nnameserver "))
		_, err = f.Write([]byte(data))
		if err != nil {
			return err
		}
	}

	if nOptions > 0 {
		data := fmt.Sprintf("options %s\n", strings.Join(options, " "))
		_, err = f.Write([]byte(data))
		if err != nil {
			return err
		}
	}

	return nil
}

func newPodNetwork(sb *sandbox.Sandbox) ocicni.PodNetwork {
	return ocicni.PodNetwork{
		Name:      sb.KubeName(),
		Namespace: sb.Namespace(),
		Networks:  make([]ocicni.NetAttachment, 0),
		ID:        sb.ID(),
		NetNS:     sb.NetNsPath(),
	}
}

// inStringSlice checks whether a string is inside a string slice.
// Comparison is case insensitive.
func inStringSlice(ss []string, str string) bool {
	for _, s := range ss {
		if strings.EqualFold(s, str) {
			return true
		}
	}
	return false
}

// getOCICapabilitiesList returns a list of all available capabilities.
func getOCICapabilitiesList() []string {
	caps := make([]string, 0, len(capability.List()))
	for _, cap := range capability.List() {
		if cap > validate.LastCap() {
			continue
		}
		caps = append(caps, "CAP_"+strings.ToUpper(cap.String()))
	}
	return caps
}

func recordOperation(operation string, start time.Time) {
	metrics.CRIOOperations.WithLabelValues(operation).Inc()
	metrics.CRIOOperationsLatency.WithLabelValues(operation).Observe(metrics.SinceInMicroseconds(start))
}

// recordError records error for metric if an error occurred.
func recordError(operation string, err error) {
	if err != nil {
		// TODO(runcom): handle timeout from ctx as well
		metrics.CRIOOperationsErrors.WithLabelValues(operation).Inc()
	}
}

func validateLabels(labels map[string]string) error {
	for k, v := range labels {
		if (len(k) + len(v)) > maxLabelSize {
			if len(k) > 10 {
				k = k[:10]
			}
			return fmt.Errorf("label key and value greater than maximum size (%d bytes), key: %s", maxLabelSize, k)
		}
	}
	return nil
}

func mergeEnvs(imageConfig *v1.Image, kubeEnvs []*pb.KeyValue) []string {
	envs := []string{}
	if kubeEnvs == nil && imageConfig != nil {
		envs = imageConfig.Config.Env
	} else {
		for _, item := range kubeEnvs {
			if item.GetKey() == "" {
				continue
			}
			envs = append(envs, item.GetKey()+"="+item.GetValue())
		}
		if imageConfig != nil {
			for _, imageEnv := range imageConfig.Config.Env {
				var found bool
				parts := strings.SplitN(imageEnv, "=", 2)
				if len(parts) != 2 {
					continue
				}
				imageEnvKey := parts[0]
				if imageEnvKey == "" {
					continue
				}
				for _, kubeEnv := range envs {
					kubeEnvKey := strings.SplitN(kubeEnv, "=", 2)[0]
					if kubeEnvKey == "" {
						continue
					}
					if imageEnvKey == kubeEnvKey {
						found = true
						break
					}
				}
				if !found {
					envs = append(envs, imageEnv)
				}
			}
		}
	}
	return envs
}

// Namespace represents a kernel namespace name.
type Namespace string

const (
	// IpcNamespace is the Linux IPC namespace
	IpcNamespace = Namespace("ipc")

	// NetNamespace is the network namespace
	NetNamespace = Namespace("net")

	// UnknownNamespace is the zero value if no namespace is known
	UnknownNamespace = Namespace("")
)

var namespaces = map[string]Namespace{
	"kernel.sem": IpcNamespace,
}

var prefixNamespaces = map[string]Namespace{
	"kernel.shm": IpcNamespace,
	"kernel.msg": IpcNamespace,
	"fs.mqueue.": IpcNamespace,
	"net.":       NetNamespace,
}

// validateSysctl checks that a sysctl is whitelisted because it is known
// to be namespaced by the Linux kernel.
// The parameters hostNet and hostIPC are used to forbid sysctls for pod sharing the
// respective namespaces with the host. This check is only used on sysctls defined by
// the user in the crio.conf file.
func validateSysctl(sysctl string, hostNet, hostIPC bool) error {
	nsErrorFmt := "%q not allowed with host %s enabled"
	if ns, found := namespaces[sysctl]; found {
		if ns == IpcNamespace && hostIPC {
			return errors.Errorf(nsErrorFmt, sysctl, ns)
		}
		if ns == NetNamespace && hostNet {
			return errors.Errorf(nsErrorFmt, sysctl, ns)
		}
		return nil
	}
	for p, ns := range prefixNamespaces {
		if strings.HasPrefix(sysctl, p) {
			if ns == IpcNamespace && hostIPC {
				return errors.Errorf(nsErrorFmt, sysctl, ns)
			}
			if ns == NetNamespace && hostNet {
				return errors.Errorf(nsErrorFmt, sysctl, ns)
			}
			return nil
		}
	}
	return errors.Errorf("%q not whitelisted", sysctl)
}

type ulimit struct {
	name string
	hard uint64
	soft uint64
}

func getUlimitsFromConfig(config Config) ([]ulimit, error) {
	ulimits := make([]ulimit, 0, len(config.RuntimeConfig.DefaultUlimits))
	for _, u := range config.RuntimeConfig.DefaultUlimits {
		ul, err := units.ParseUlimit(u)
		if err != nil {
			return nil, err
		}
		rl, err := ul.GetRlimit()
		if err != nil {
			return nil, err
		}
		// This sucks, but it's the runtime-tools interface
		ulimits = append(ulimits, ulimit{name: "RLIMIT_" + strings.ToUpper(ul.Name), hard: rl.Hard, soft: rl.Soft})
	}
	return ulimits, nil
}

// Translate container labels to a description of the container
func translateLabelsToDescription(labels map[string]string) string {
	return fmt.Sprintf("%s/%s/%s", labels[types.KubernetesPodNamespaceLabel], labels[types.KubernetesPodNameLabel], labels[types.KubernetesContainerNameLabel])
}
