go 1.13

module github.com/cri-o/cri-o

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/DataDog/zstd v1.4.0 // indirect
	github.com/Microsoft/go-winio v0.4.15-0.20190919025122-fc70bd9a86b5
	github.com/blang/semver v3.5.1+incompatible
	github.com/containerd/containerd v1.3.1
	github.com/containerd/fifo v0.0.0-20190816180239-bda0ff6ed73c // indirect
	github.com/containerd/go-runc v0.0.0-20190923131748-a2952bc25f51 // indirect
	github.com/containerd/project v0.0.0-20190513184420-7fb81da5e663
	github.com/containerd/ttrpc v0.0.0-20190828154514-0e0f228740de
	github.com/containernetworking/cni v0.7.2-0.20200304161608-4fae32b84921
	github.com/containernetworking/plugins v0.8.5
	github.com/containers/buildah v1.14.8
	github.com/containers/conmon v2.0.15+incompatible
	github.com/containers/image/v5 v5.4.4
	github.com/containers/libpod v1.9.2
	github.com/containers/ocicrypt v1.0.2
	github.com/containers/storage v1.19.1
	github.com/coreos/go-systemd v0.0.0-20190719114852-fd7a80b32e1f // indirect
	github.com/coreos/go-systemd/v22 v22.0.0
	github.com/cpuguy83/go-md2man v1.0.10
	github.com/creack/pty v1.1.11
	github.com/cri-o/ocicni v0.2.1-0.20200422173648-513ef787b8c9
	github.com/docker/docker v1.4.2-0.20191219165747-a9416c67da9f
	github.com/docker/go-units v0.4.0
	github.com/docker/spdystream v0.0.0-20181023171402-6480d4af844c // indirect
	github.com/elazarl/goproxy v0.0.0-20190421051319-9d40249d3c2f // indirect
	github.com/elazarl/goproxy/ext v0.0.0-20190911111923-ecfe977594f1 // indirect
	github.com/fsnotify/fsnotify v1.4.9
	github.com/go-zoo/bone v1.3.0
	github.com/godbus/dbus/v5 v5.0.3
	github.com/gogo/protobuf v1.3.1
	github.com/golang/mock v1.3.1
	github.com/google/renameio v0.1.0
	github.com/google/uuid v1.1.1
	github.com/gotestyourself/gotestyourself v2.2.0+incompatible // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.1.0
	github.com/hpcloud/tail v1.0.0
	github.com/klauspost/cpuid v1.2.1 // indirect
	github.com/onsi/ginkgo v1.12.0
	github.com/onsi/gomega v1.9.0
	github.com/opencontainers/go-digest v1.0.0-rc1
	github.com/opencontainers/image-spec v1.0.2-0.20190823105129-775207bd45b6
	github.com/opencontainers/runc v1.0.0-rc9
	github.com/opencontainers/runtime-spec v1.0.1
	github.com/opencontainers/runtime-tools v0.9.1-0.20200121211434-d1bf3e66ff0a
	github.com/opencontainers/selinux v1.5.1
	github.com/openshift/api v3.9.1-0.20190810003144-27fb16909b15+incompatible // indirect
	github.com/pkg/errors v0.9.1
	github.com/pkg/profile v1.3.0 // indirect
	github.com/prometheus/client_golang v1.2.1
	github.com/seccomp/containers-golang v0.3.1
	github.com/sirupsen/logrus v1.6.0
	github.com/soheilhy/cmux v0.1.4
	github.com/syndtr/gocapability v0.0.0-20180916011248-d98352740cb2
	github.com/uber-go/atomic v1.4.0 // indirect
	github.com/urfave/cli v1.22.1 // indirect
	github.com/urfave/cli/v2 v2.1.2-0.20200306124602-d648edd48d89
	github.com/vbatts/git-validation v1.0.0
	github.com/vbauerster/mpb v3.4.0+incompatible // indirect
	github.com/vbauerster/mpb/v4 v4.12.2 // indirect
	github.com/vishvananda/netlink v1.1.0
	golang.org/x/net v0.0.0-20200324143707-d3edc9973b7e
	golang.org/x/sync v0.0.0-20200317015054-43a5402ce75a
	golang.org/x/sys v0.0.0-20200420163511-1957bb5e6d1f
	google.golang.org/appengine v1.6.1 // indirect
	google.golang.org/grpc v1.25.1
	k8s.io/api v0.17.4
	k8s.io/apimachinery v0.17.4
	k8s.io/client-go v0.0.0
	k8s.io/cri-api v0.0.0
	k8s.io/klog v1.0.0
	k8s.io/kubernetes v1.13.0
	k8s.io/utils v0.0.0-20191114200735-6ca3b61696b6
)

replace (
	github.com/godbus/dbus => github.com/godbus/dbus v0.0.0-20190623212516-8a1682060722
	github.com/opencontainers/runtime-spec => github.com/opencontainers/runtime-spec v0.1.2-0.20190408193819-a1b50f621a48
	github.com/prometheus/client_golang => github.com/prometheus/client_golang v0.9.2
	github.com/sirupsen/logrus => github.com/sirupsen/logrus v1.4.2
	k8s.io/api => k8s.io/kubernetes/staging/src/k8s.io/api v0.0.0-20191206012503-70132b0f130a
	k8s.io/apiextensions-apiserver => k8s.io/kubernetes/staging/src/k8s.io/apiextensions-apiserver v0.0.0-20191206012503-70132b0f130a
	k8s.io/apimachinery => k8s.io/kubernetes/staging/src/k8s.io/apimachinery v0.0.0-20191206012503-70132b0f130a
	k8s.io/apiserver => k8s.io/kubernetes/staging/src/k8s.io/apiserver v0.0.0-20191206012503-70132b0f130a
	k8s.io/cli-runtime => k8s.io/kubernetes/staging/src/k8s.io/cli-runtime v0.0.0-20191206012503-70132b0f130a
	k8s.io/client-go => k8s.io/kubernetes/staging/src/k8s.io/client-go v0.0.0-20191206012503-70132b0f130a
	k8s.io/cloud-provider => k8s.io/kubernetes/staging/src/k8s.io/cloud-provider v0.0.0-20191206012503-70132b0f130a
	k8s.io/cluster-bootstrap => k8s.io/kubernetes/staging/src/k8s.io/cluster-bootstrap v0.0.0-20191206012503-70132b0f130a
	k8s.io/code-generator => k8s.io/kubernetes/staging/src/k8s.io/code-generator v0.0.0-20191206012503-70132b0f130a
	k8s.io/component-base => k8s.io/kubernetes/staging/src/k8s.io/component-base v0.0.0-20191206012503-70132b0f130a
	k8s.io/cri-api => k8s.io/kubernetes/staging/src/k8s.io/cri-api v0.0.0-20191206012503-70132b0f130a
	k8s.io/csi-translation-lib => k8s.io/kubernetes/staging/src/k8s.io/csi-translation-lib v0.0.0-20191206012503-70132b0f130a
	k8s.io/kube-aggregator => k8s.io/kubernetes/staging/src/k8s.io/kube-aggregator v0.0.0-20191206012503-70132b0f130a
	k8s.io/kube-controller-manager => k8s.io/kubernetes/staging/src/k8s.io/kube-controller-manager v0.0.0-20191206012503-70132b0f130a
	k8s.io/kube-proxy => k8s.io/kubernetes/staging/src/k8s.io/kube-proxy v0.0.0-20191206012503-70132b0f130a
	k8s.io/kube-scheduler => k8s.io/kubernetes/staging/src/k8s.io/kube-scheduler v0.0.0-20191206012503-70132b0f130a
	k8s.io/kubectl => k8s.io/kubernetes/staging/src/k8s.io/kubectl v0.0.0-20191206012503-70132b0f130a
	k8s.io/kubelet => k8s.io/kubernetes/staging/src/k8s.io/kubelet v0.0.0-20191206012503-70132b0f130a
	k8s.io/kubernetes => k8s.io/kubernetes v1.17.0
	k8s.io/legacy-cloud-providers => k8s.io/kubernetes/staging/src/k8s.io/legacy-cloud-providers v0.0.0-20191206012503-70132b0f130a
	k8s.io/metrics => k8s.io/kubernetes/staging/src/k8s.io/metrics v0.0.0-20191206012503-70132b0f130a
	k8s.io/sample-apiserver => k8s.io/kubernetes/staging/src/k8s.io/sample-apiserver v0.0.0-20191206012503-70132b0f130a
)
