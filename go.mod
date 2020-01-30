go 1.13

module github.com/cri-o/cri-o

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/Microsoft/go-winio v0.4.15-0.20190919025122-fc70bd9a86b5
	github.com/blang/semver v3.5.1+incompatible
	github.com/containerd/containerd v1.3.1
	github.com/containerd/release-tool v0.0.0-20200109140359-1e44fc26031c
	github.com/containerd/ttrpc v0.0.0-20190828154514-0e0f228740de
	github.com/containernetworking/cni v0.7.1
	github.com/containernetworking/plugins v0.8.4
	github.com/containers/buildah v1.11.5-0.20191031204705-20e92ffe0982
	github.com/containers/conmon v2.0.9+incompatible
	github.com/containers/image/v5 v5.1.0
	github.com/containers/libpod v1.6.3-0.20191101152258-04e8bf3dba50
	github.com/containers/ocicrypt v0.0.0-20190930154801-b87a4a69c741
	github.com/containers/psgo v1.4.0
	github.com/containers/storage v1.15.4-0.20191218193401-e18327c57107
	github.com/coreos/go-systemd/v22 v22.0.0
	github.com/cpuguy83/go-md2man v1.0.10
	github.com/cri-o/ocicni v0.1.1-0.20190702175919-7762645d18ca
	github.com/docker/docker v1.4.2-0.20190927142053-ada3c14355ce
	github.com/docker/go-units v0.4.0
	github.com/fsnotify/fsnotify v1.4.7
	github.com/go-zoo/bone v1.3.0
	github.com/godbus/dbus/v5 v5.0.3
	github.com/gogo/protobuf v1.2.2-0.20190723190241-65acae22fc9d
	github.com/golang/mock v1.3.1
	github.com/google/renameio v0.1.0
	github.com/google/uuid v1.1.1
	github.com/grpc-ecosystem/go-grpc-middleware v1.1.0
	github.com/hpcloud/tail v1.0.0
	github.com/kr/pty v1.1.8
	github.com/onsi/ginkgo v1.10.3
	github.com/onsi/gomega v1.7.1
	github.com/opencontainers/go-digest v1.0.0-rc1
	github.com/opencontainers/image-spec v1.0.2-0.20190823105129-775207bd45b6
	github.com/opencontainers/runc v1.0.0-rc9
	github.com/opencontainers/runtime-spec v1.0.1
	github.com/opencontainers/runtime-tools v0.9.1-0.20200121211434-d1bf3e66ff0a
	github.com/opencontainers/selinux v1.3.1-0.20190929122143-5215b1806f52
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v1.2.1
	github.com/seccomp/containers-golang v0.3.1
	github.com/sirupsen/logrus v1.4.2
	github.com/soheilhy/cmux v0.1.4
	github.com/syndtr/gocapability v0.0.0-20180916011248-d98352740cb2
	github.com/urfave/cli/v2 v2.1.1
	github.com/vbatts/git-validation v1.0.0
	github.com/vishvananda/netlink v1.0.0
	golang.org/x/net v0.0.0-20191004110552-13f9640d40b9
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	golang.org/x/sys v0.0.0-20191127021746-63cb32ae39b2
	google.golang.org/grpc v1.25.1
	k8s.io/api v0.0.0
	k8s.io/apimachinery v0.0.0
	k8s.io/client-go v0.0.0
	k8s.io/cri-api v0.0.0
	k8s.io/kubernetes v1.13.0
	k8s.io/utils v0.0.0-20191114200735-6ca3b61696b6
)

replace (
	github.com/godbus/dbus => github.com/godbus/dbus v0.0.0-20190623212516-8a1682060722
	github.com/opencontainers/runtime-spec => github.com/opencontainers/runtime-spec v0.1.2-0.20190408193819-a1b50f621a48
	github.com/prometheus/client_golang => github.com/prometheus/client_golang v0.9.2
	k8s.io/api => k8s.io/kubernetes/staging/src/k8s.io/api v0.0.0-20191218085820-ac4079a5cc22
	k8s.io/apiextensions-apiserver => k8s.io/kubernetes/staging/src/k8s.io/apiextensions-apiserver v0.0.0-20191218085820-ac4079a5cc22
	k8s.io/apimachinery => k8s.io/kubernetes/staging/src/k8s.io/apimachinery v0.0.0-20191218085820-ac4079a5cc22
	k8s.io/apiserver => k8s.io/kubernetes/staging/src/k8s.io/apiserver v0.0.0-20191218085820-ac4079a5cc22
	k8s.io/cli-runtime => k8s.io/kubernetes/staging/src/k8s.io/cli-runtime v0.0.0-20191218085820-ac4079a5cc22
	k8s.io/client-go => k8s.io/kubernetes/staging/src/k8s.io/client-go v0.0.0-20191218085820-ac4079a5cc22
	k8s.io/cloud-provider => k8s.io/kubernetes/staging/src/k8s.io/cloud-provider v0.0.0-20191218085820-ac4079a5cc22
	k8s.io/cluster-bootstrap => k8s.io/kubernetes/staging/src/k8s.io/cluster-bootstrap v0.0.0-20191218085820-ac4079a5cc22
	k8s.io/code-generator => k8s.io/kubernetes/staging/src/k8s.io/code-generator v0.0.0-20191218085820-ac4079a5cc22
	k8s.io/component-base => k8s.io/kubernetes/staging/src/k8s.io/component-base v0.0.0-20191218085820-ac4079a5cc22
	k8s.io/cri-api => k8s.io/kubernetes/staging/src/k8s.io/cri-api v0.0.0-20191218085820-ac4079a5cc22
	k8s.io/csi-translation-lib => k8s.io/kubernetes/staging/src/k8s.io/csi-translation-lib v0.0.0-20191218085820-ac4079a5cc22
	k8s.io/kube-aggregator => k8s.io/kubernetes/staging/src/k8s.io/kube-aggregator v0.0.0-20191218085820-ac4079a5cc22
	k8s.io/kube-controller-manager => k8s.io/kubernetes/staging/src/k8s.io/kube-controller-manager v0.0.0-20191218085820-ac4079a5cc22
	k8s.io/kube-proxy => k8s.io/kubernetes/staging/src/k8s.io/kube-proxy v0.0.0-20191218085820-ac4079a5cc22
	k8s.io/kube-scheduler => k8s.io/kubernetes/staging/src/k8s.io/kube-scheduler v0.0.0-20191218085820-ac4079a5cc22
	k8s.io/kubectl => k8s.io/kubernetes/staging/src/k8s.io/kubectl v0.0.0-20191218085820-ac4079a5cc22
	k8s.io/kubelet => k8s.io/kubernetes/staging/src/k8s.io/kubelet v0.0.0-20191218085820-ac4079a5cc22
	k8s.io/kubernetes => k8s.io/kubernetes v1.18.0-alpha.1
	k8s.io/legacy-cloud-providers => k8s.io/kubernetes/staging/src/k8s.io/legacy-cloud-providers v0.0.0-20191218085820-ac4079a5cc22
	k8s.io/metrics => k8s.io/kubernetes/staging/src/k8s.io/metrics v0.0.0-20191218085820-ac4079a5cc22
	k8s.io/sample-apiserver => k8s.io/kubernetes/staging/src/k8s.io/sample-apiserver v0.0.0-20191218085820-ac4079a5cc22
)
