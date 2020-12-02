go 1.15

module github.com/cri-o/cri-o

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/Microsoft/go-winio v0.4.15
	github.com/blang/semver v3.5.1+incompatible
	github.com/containerd/containerd v1.4.1
	github.com/containerd/ttrpc v1.0.1
	github.com/containernetworking/cni v0.8.0
	github.com/containernetworking/plugins v0.8.7
	github.com/containers/buildah v1.18.0
	github.com/containers/common v0.29.0
	github.com/containers/conmon v2.0.20+incompatible
	github.com/containers/image/v5 v5.8.1
	github.com/containers/ocicrypt v1.0.3
	github.com/containers/podman/v2 v2.2.0
	github.com/containers/storage v1.24.1
	github.com/coreos/go-systemd/v22 v22.1.0
	github.com/cpuguy83/go-md2man v1.0.10
	github.com/creack/pty v1.1.11
	github.com/cri-o/ocicni v0.2.1-0.20201102180012-75c612fda1a2
	github.com/cyphar/filepath-securejoin v0.2.2
	github.com/docker/distribution v2.7.1+incompatible
	github.com/docker/go-units v0.4.0
	github.com/fsnotify/fsnotify v1.4.9
	github.com/go-zoo/bone v1.3.0
	github.com/godbus/dbus/v5 v5.0.3
	github.com/gogo/protobuf v1.3.1
	github.com/golang/mock v1.4.3
	github.com/google/renameio v0.1.0
	github.com/google/uuid v1.1.2
	github.com/grpc-ecosystem/go-grpc-middleware v1.2.0
	github.com/hpcloud/tail v1.0.0
	github.com/json-iterator/go v1.1.10
	github.com/mattn/go-isatty v0.0.12 // indirect
	github.com/onsi/ginkgo v1.14.2
	github.com/onsi/gomega v1.10.3
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.2-0.20200206005212-79b036d80240
	github.com/opencontainers/runc v1.0.0-rc92
	github.com/opencontainers/runtime-spec v1.0.3-0.20200817204227-f9c09b4ea1df
	github.com/opencontainers/runtime-tools v0.9.1-0.20200714183735-07406c5828aa
	github.com/opencontainers/selinux v1.6.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.7.1
	github.com/psampaz/go-mod-outdated v0.6.0
	github.com/sirupsen/logrus v1.7.0
	github.com/soheilhy/cmux v0.1.4
	github.com/stretchr/testify v1.6.1
	github.com/syndtr/gocapability v0.0.0-20180916011248-d98352740cb2
	github.com/urfave/cli/v2 v2.3.0
	github.com/vishvananda/netlink v1.1.0
	golang.org/x/net v0.0.0-20201006153459-a7d1128ccaa0
	golang.org/x/sync v0.0.0-20201020160332-67f06af15bc9
	golang.org/x/sys v0.0.0-20201029080932-201ba4db2418
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	google.golang.org/grpc v1.30.0
	k8s.io/api v0.19.2
	k8s.io/apimachinery v0.19.4
	k8s.io/client-go v0.0.0
	k8s.io/cri-api v0.0.0
	k8s.io/klog/v2 v2.3.0
	k8s.io/kubernetes v1.19.2
	k8s.io/release v0.4.0
	k8s.io/utils v0.0.0-20200912215256-4140de9c8800
	mvdan.cc/sh/v3 v3.2.0
)

replace (
	github.com/golang/protobuf => github.com/golang/protobuf v1.3.5
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20200117163144-32f20d992d24
	google.golang.org/grpc => google.golang.org/grpc v1.27.0
	k8s.io/api => k8s.io/kubernetes/staging/src/k8s.io/api v0.0.0-20200922090449-51ffb495f752
	k8s.io/apiextensions-apiserver => k8s.io/kubernetes/staging/src/k8s.io/apiextensions-apiserver v0.0.0-20200922090449-51ffb495f752
	k8s.io/apimachinery => k8s.io/kubernetes/staging/src/k8s.io/apimachinery v0.0.0-20200922090449-51ffb495f752
	k8s.io/apiserver => k8s.io/kubernetes/staging/src/k8s.io/apiserver v0.0.0-20200922090449-51ffb495f752
	k8s.io/cli-runtime => k8s.io/kubernetes/staging/src/k8s.io/cli-runtime v0.0.0-20200922090449-51ffb495f752
	k8s.io/client-go => k8s.io/kubernetes/staging/src/k8s.io/client-go v0.0.0-20200922090449-51ffb495f752
	k8s.io/cloud-provider => k8s.io/kubernetes/staging/src/k8s.io/cloud-provider v0.0.0-20200922090449-51ffb495f752
	k8s.io/cluster-bootstrap => k8s.io/kubernetes/staging/src/k8s.io/cluster-bootstrap v0.0.0-20200922090449-51ffb495f752
	k8s.io/code-generator => k8s.io/kubernetes/staging/src/k8s.io/code-generator v0.0.0-20200922090449-51ffb495f752
	k8s.io/component-base => k8s.io/kubernetes/staging/src/k8s.io/component-base v0.0.0-20200922090449-51ffb495f752
	k8s.io/cri-api => k8s.io/kubernetes/staging/src/k8s.io/cri-api v0.0.0-20200922090449-51ffb495f752
	k8s.io/csi-translation-lib => k8s.io/kubernetes/staging/src/k8s.io/csi-translation-lib v0.0.0-20200922090449-51ffb495f752
	k8s.io/kube-aggregator => k8s.io/kubernetes/staging/src/k8s.io/kube-aggregator v0.0.0-20200922090449-51ffb495f752
	k8s.io/kube-controller-manager => k8s.io/kubernetes/staging/src/k8s.io/kube-controller-manager v0.0.0-20200922090449-51ffb495f752
	k8s.io/kube-proxy => k8s.io/kubernetes/staging/src/k8s.io/kube-proxy v0.0.0-20200922090449-51ffb495f752
	k8s.io/kube-scheduler => k8s.io/kubernetes/staging/src/k8s.io/kube-scheduler v0.0.0-20200922090449-51ffb495f752
	k8s.io/kubectl => k8s.io/kubernetes/staging/src/k8s.io/kubectl v0.0.0-20200922090449-51ffb495f752
	k8s.io/kubelet => k8s.io/kubernetes/staging/src/k8s.io/kubelet v0.0.0-20200922090449-51ffb495f752
	k8s.io/kubernetes => k8s.io/kubernetes v1.20.0-alpha.1
	k8s.io/legacy-cloud-providers => k8s.io/kubernetes/staging/src/k8s.io/legacy-cloud-providers v0.0.0-20200922090449-51ffb495f752
	k8s.io/metrics => k8s.io/kubernetes/staging/src/k8s.io/metrics v0.0.0-20200922090449-51ffb495f752
	k8s.io/mount-utils => k8s.io/kubernetes/staging/src/k8s.io/mount-utils v0.0.0-20200922090449-51ffb495f752
	k8s.io/sample-apiserver => k8s.io/kubernetes/staging/src/k8s.io/sample-apiserver v0.0.0-20200922090449-51ffb495f752
)
