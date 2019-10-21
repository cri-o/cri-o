go 1.12

module github.com/cri-o/cri-o

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/Microsoft/go-winio v0.4.14
	github.com/blang/semver v3.5.1+incompatible
	github.com/containerd/containerd v1.2.9
	github.com/containerd/fifo v0.0.0-20190226154929-a9fb20d87448 // indirect
	github.com/containerd/go-runc v0.0.0-20190603165425-9007c2405372 // indirect
	github.com/containerd/project v0.0.0-20190513184420-7fb81da5e663
	github.com/containerd/ttrpc v0.0.0-20190613183316-1fb3814edf44
	github.com/containernetworking/cni v0.7.1
	github.com/containernetworking/plugins v0.8.2
	github.com/containers/buildah v1.11.2
	github.com/containers/conmon v2.0.0+incompatible
	github.com/containers/image v3.0.2+incompatible
	github.com/containers/libpod v1.5.1
	github.com/containers/storage v1.13.4
	github.com/coreos/go-systemd v0.0.0-20190719114852-fd7a80b32e1f
	github.com/cpuguy83/go-md2man v1.0.10
	github.com/cri-o/ocicni v0.1.1-0.20190702175919-7762645d18ca
	github.com/docker/docker v0.7.3-0.20190410184157-6d18c6a06295
	github.com/docker/go-units v0.4.0
	github.com/fsnotify/fsnotify v1.4.7
	github.com/go-zoo/bone v1.3.0
	github.com/godbus/dbus v4.1.0+incompatible
	github.com/gogo/protobuf v1.2.2-0.20190723190241-65acae22fc9d
	github.com/golang/mock v1.3.1
	github.com/golangci/golangci-lint v1.18.0
	github.com/google/renameio v0.1.0
	github.com/google/uuid v1.1.1
	github.com/hpcloud/tail v1.0.0
	github.com/kr/pty v1.1.8
	github.com/onsi/ginkgo v1.10.1
	github.com/onsi/gomega v1.7.0
	github.com/opencontainers/go-digest v1.0.0-rc1
	github.com/opencontainers/image-spec v1.0.1
	github.com/opencontainers/runc v1.0.0-rc8.0.20190827142921-dd075602f158
	github.com/opencontainers/runtime-spec v1.0.1
	github.com/opencontainers/runtime-tools v0.9.0
	github.com/opencontainers/selinux v1.3.0
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v1.1.0
	github.com/seccomp/containers-golang v0.3.1
	github.com/sirupsen/logrus v1.4.2
	github.com/soheilhy/cmux v0.1.4
	github.com/syndtr/gocapability v0.0.0-20180916011248-d98352740cb2
	github.com/urfave/cli v1.22.1
	github.com/vbatts/git-validation v1.0.0
	golang.org/x/net v0.0.0-20190918130420-a8b05e9114ab
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	golang.org/x/sys v0.0.0-20190919044723-0c1ff786ef13
	google.golang.org/grpc v1.23.1
	k8s.io/api v0.0.0
	k8s.io/apimachinery v0.0.0
	k8s.io/client-go v0.0.0
	k8s.io/cri-api v0.0.0
	k8s.io/kubernetes v0.0.0
	k8s.io/utils v0.0.0-20190920012459-5008bf6f8cd6
)

replace (
	github.com/godbus/dbus => github.com/godbus/dbus v0.0.0-20190623212516-8a1682060722
	github.com/opencontainers/runtime-spec => github.com/opencontainers/runtime-spec v0.1.2-0.20190408193819-a1b50f621a48
	github.com/prometheus/client_golang => github.com/prometheus/client_golang v0.9.2
	k8s.io/api => k8s.io/kubernetes/staging/src/k8s.io/api v0.0.0-20190913145653-2bd9643cee5b
	k8s.io/apiextensions-apiserver => k8s.io/kubernetes/staging/src/k8s.io/apiextensions-apiserver v0.0.0-20190913145653-2bd9643cee5b
	k8s.io/apimachinery => k8s.io/kubernetes/staging/src/k8s.io/apimachinery v0.0.0-20190913145653-2bd9643cee5b
	k8s.io/apiserver => k8s.io/kubernetes/staging/src/k8s.io/apiserver v0.0.0-20190913145653-2bd9643cee5b
	k8s.io/cli-runtime => k8s.io/kubernetes/staging/src/k8s.io/cli-runtime v0.0.0-20190913145653-2bd9643cee5b
	k8s.io/client-go => k8s.io/kubernetes/staging/src/k8s.io/client-go v0.0.0-20190913145653-2bd9643cee5b
	k8s.io/cloud-provider => k8s.io/kubernetes/staging/src/k8s.io/cloud-provider v0.0.0-20190913145653-2bd9643cee5b
	k8s.io/cluster-bootstrap => k8s.io/kubernetes/staging/src/k8s.io/cluster-bootstrap v0.0.0-20190913145653-2bd9643cee5b
	k8s.io/code-generator => k8s.io/kubernetes/staging/src/k8s.io/code-generator v0.0.0-20190913145653-2bd9643cee5b
	k8s.io/component-base => k8s.io/kubernetes/staging/src/k8s.io/component-base v0.0.0-20190913145653-2bd9643cee5b
	k8s.io/cri-api => k8s.io/kubernetes/staging/src/k8s.io/cri-api v0.0.0-20190913145653-2bd9643cee5b
	k8s.io/csi-translation-lib => k8s.io/kubernetes/staging/src/k8s.io/csi-translation-lib v0.0.0-20190913145653-2bd9643cee5b
	k8s.io/kube-aggregator => k8s.io/kubernetes/staging/src/k8s.io/kube-aggregator v0.0.0-20190913145653-2bd9643cee5b
	k8s.io/kube-controller-manager => k8s.io/kubernetes/staging/src/k8s.io/kube-controller-manager v0.0.0-20190913145653-2bd9643cee5b
	k8s.io/kube-proxy => k8s.io/kubernetes/staging/src/k8s.io/kube-proxy v0.0.0-20190913145653-2bd9643cee5b
	k8s.io/kube-scheduler => k8s.io/kubernetes/staging/src/k8s.io/kube-scheduler v0.0.0-20190913145653-2bd9643cee5b
	k8s.io/kubectl => k8s.io/kubernetes/staging/src/k8s.io/kubectl v0.0.0-20190913145653-2bd9643cee5b
	k8s.io/kubelet => k8s.io/kubernetes/staging/src/k8s.io/kubelet v0.0.0-20190913145653-2bd9643cee5b
	k8s.io/kubernetes => k8s.io/kubernetes v1.16.0
	k8s.io/legacy-cloud-providers => k8s.io/kubernetes/staging/src/k8s.io/legacy-cloud-providers v0.0.0-20190913145653-2bd9643cee5b
	k8s.io/metrics => k8s.io/kubernetes/staging/src/k8s.io/metrics v0.0.0-20190913145653-2bd9643cee5b
	k8s.io/sample-apiserver => k8s.io/kubernetes/staging/src/k8s.io/sample-apiserver v0.0.0-20190913145653-2bd9643cee5b
)
