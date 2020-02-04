go 1.13

module github.com/cri-o/cri-o

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/Microsoft/go-winio v0.4.15-0.20200113171025-3fe6c5262873
	github.com/blang/semver v3.5.1+incompatible
	github.com/containerd/containerd v1.3.2
	github.com/containerd/release-tool v0.0.0-20200109140359-1e44fc26031c
	github.com/containerd/ttrpc v0.0.0-20200121165050-0be804eadb15
	github.com/containernetworking/cni v0.7.2-0.20190904153231-83439463f784
	github.com/containernetworking/plugins v0.8.5
	github.com/containers/buildah v1.13.2
	github.com/containers/conmon v2.0.10+incompatible
	github.com/containers/image/v5 v5.2.0
	github.com/containers/libpod v1.7.0
	github.com/containers/ocicrypt v0.0.0-20191112201931-142388cb70de
	github.com/containers/psgo v1.4.0
	github.com/containers/storage v1.15.8
	github.com/coreos/go-systemd/v22 v22.0.0
	github.com/cpuguy83/go-md2man v1.0.10
	github.com/cri-o/ocicni v0.1.1-0.20191204151446-e25b504ca708
	github.com/docker/docker v1.4.2-0.20200130191408-40190add56ec
	github.com/docker/go-units v0.4.0
	github.com/fsnotify/fsnotify v1.4.7
	github.com/go-zoo/bone v1.3.0
	github.com/godbus/dbus/v5 v5.0.3
	github.com/gogo/protobuf v1.3.1
	github.com/golang/mock v1.4.0
	github.com/google/renameio v0.1.0
	github.com/google/uuid v1.1.1
	github.com/grpc-ecosystem/go-grpc-middleware v1.1.0
	github.com/hpcloud/tail v1.0.0
	github.com/kr/pty v1.1.8
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.8.1
	github.com/opencontainers/go-digest v1.0.0-rc1
	github.com/opencontainers/image-spec v1.0.2-0.20191218002246-9ea04d1f37d7
	github.com/opencontainers/runc v1.0.0-rc9
	github.com/opencontainers/runtime-spec v1.0.1
	github.com/opencontainers/runtime-tools v0.9.1-0.20200121211434-d1bf3e66ff0a
	github.com/opencontainers/selinux v1.3.1
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.4.0
	github.com/seccomp/containers-golang v0.3.2
	github.com/sirupsen/logrus v1.4.2
	github.com/soheilhy/cmux v0.1.4
	github.com/syndtr/gocapability v0.0.0-20180916011248-d98352740cb2
	github.com/urfave/cli/v2 v2.1.1
	github.com/vbatts/git-validation v1.0.0
	github.com/vishvananda/netlink v1.1.0
	golang.org/x/net v0.0.0-20200114155413-6afb5195e5aa
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	golang.org/x/sys v0.0.0-20200124204421-9fbb57f87de9
	google.golang.org/grpc v1.27.0
	k8s.io/api v0.17.0
	k8s.io/apimachinery v0.17.0
	k8s.io/client-go v0.0.0
	k8s.io/cri-api v0.0.0
	k8s.io/kubernetes v1.13.0
	k8s.io/utils v0.0.0-20200124190032-861946025e34
)

replace (
	github.com/godbus/dbus => github.com/godbus/dbus v0.0.0-20190623212516-8a1682060722
	github.com/opencontainers/runtime-spec => github.com/opencontainers/runtime-spec v0.1.2-0.20190408193819-a1b50f621a48
	github.com/prometheus/client_golang => github.com/prometheus/client_golang v0.9.2
	k8s.io/api => k8s.io/kubernetes/staging/src/k8s.io/api v0.0.0-20200121093009-a7b4459cd1cf
	k8s.io/apiextensions-apiserver => k8s.io/kubernetes/staging/src/k8s.io/apiextensions-apiserver v0.0.0-20200121093009-a7b4459cd1cf
	k8s.io/apimachinery => k8s.io/kubernetes/staging/src/k8s.io/apimachinery v0.0.0-20200121093009-a7b4459cd1cf
	k8s.io/apiserver => k8s.io/kubernetes/staging/src/k8s.io/apiserver v0.0.0-20200121093009-a7b4459cd1cf
	k8s.io/cli-runtime => k8s.io/kubernetes/staging/src/k8s.io/cli-runtime v0.0.0-20200121093009-a7b4459cd1cf
	k8s.io/client-go => k8s.io/kubernetes/staging/src/k8s.io/client-go v0.0.0-20200121093009-a7b4459cd1cf
	k8s.io/cloud-provider => k8s.io/kubernetes/staging/src/k8s.io/cloud-provider v0.0.0-20200121093009-a7b4459cd1cf
	k8s.io/cluster-bootstrap => k8s.io/kubernetes/staging/src/k8s.io/cluster-bootstrap v0.0.0-20200121093009-a7b4459cd1cf
	k8s.io/code-generator => k8s.io/kubernetes/staging/src/k8s.io/code-generator v0.0.0-20200121093009-a7b4459cd1cf
	k8s.io/component-base => k8s.io/kubernetes/staging/src/k8s.io/component-base v0.0.0-20200121093009-a7b4459cd1cf
	k8s.io/cri-api => k8s.io/kubernetes/staging/src/k8s.io/cri-api v0.0.0-20200121093009-a7b4459cd1cf
	k8s.io/csi-translation-lib => k8s.io/kubernetes/staging/src/k8s.io/csi-translation-lib v0.0.0-20200121093009-a7b4459cd1cf
	k8s.io/kube-aggregator => k8s.io/kubernetes/staging/src/k8s.io/kube-aggregator v0.0.0-20200121093009-a7b4459cd1cf
	k8s.io/kube-controller-manager => k8s.io/kubernetes/staging/src/k8s.io/kube-controller-manager v0.0.0-20200121093009-a7b4459cd1cf
	k8s.io/kube-proxy => k8s.io/kubernetes/staging/src/k8s.io/kube-proxy v0.0.0-20200121093009-a7b4459cd1cf
	k8s.io/kube-scheduler => k8s.io/kubernetes/staging/src/k8s.io/kube-scheduler v0.0.0-20200121093009-a7b4459cd1cf
	k8s.io/kubectl => k8s.io/kubernetes/staging/src/k8s.io/kubectl v0.0.0-20200121093009-a7b4459cd1cf
	k8s.io/kubelet => k8s.io/kubernetes/staging/src/k8s.io/kubelet v0.0.0-20200121093009-a7b4459cd1cf
	k8s.io/kubernetes => k8s.io/kubernetes v1.18.0-alpha.2
	k8s.io/legacy-cloud-providers => k8s.io/kubernetes/staging/src/k8s.io/legacy-cloud-providers v0.0.0-20200121093009-a7b4459cd1cf
	k8s.io/metrics => k8s.io/kubernetes/staging/src/k8s.io/metrics v0.0.0-20200121093009-a7b4459cd1cf
	k8s.io/sample-apiserver => k8s.io/kubernetes/staging/src/k8s.io/sample-apiserver v0.0.0-20200121093009-a7b4459cd1cf
)
