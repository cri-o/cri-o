go 1.14

module github.com/cri-o/cri-o

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/Microsoft/go-winio v0.4.15-0.20200113171025-3fe6c5262873
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver v3.5.1+incompatible
	github.com/containerd/cgroups v0.0.0-20200327175542-b44481373989 // indirect
	github.com/containerd/containerd v1.3.3
	github.com/containerd/continuity v0.0.0-20200228182428-0f16d7a0959c // indirect
	github.com/containerd/release-tool v0.0.0-20200218161915-2230b10b5f0b
	github.com/containerd/ttrpc v0.0.0-20200121165050-0be804eadb15
	github.com/containernetworking/cni v0.7.2-0.20200304161608-4fae32b84921
	github.com/containernetworking/plugins v0.8.5
	github.com/containers/buildah v1.14.6-0.20200331161119-912ca5acd9c7
	github.com/containers/conmon v2.0.12+incompatible
	github.com/containers/image/v5 v5.3.1
	github.com/containers/libpod v1.8.2
	github.com/containers/libtrust v0.0.0-20200211134951-685ad84e365b // indirect
	github.com/containers/ocicrypt v1.0.1
	github.com/containers/storage v1.18.1
	github.com/coreos/go-systemd v0.0.0-20190719114852-fd7a80b32e1f // indirect
	github.com/coreos/go-systemd/v22 v22.0.0
	github.com/cpuguy83/go-md2man v1.0.10
	github.com/cri-o/ocicni v0.1.1-0.20200218164758-d2881573038f
	github.com/cyphar/filepath-securejoin v0.2.2
	github.com/docker/go-units v0.4.0
	github.com/docker/spdystream v0.0.0-20181023171402-6480d4af844c // indirect
	github.com/docopt/docopt-go v0.0.0-20180111231733-ee0de3bc6815 // indirect
	github.com/elazarl/goproxy v0.0.0-20190421051319-9d40249d3c2f // indirect
	github.com/elazarl/goproxy/ext v0.0.0-20190911111923-ecfe977594f1 // indirect
	github.com/fsnotify/fsnotify v1.4.9
	github.com/go-ini/ini v1.51.1 // indirect
	github.com/go-zoo/bone v1.3.0
	github.com/godbus/dbus/v5 v5.0.3
	github.com/gogo/protobuf v1.3.1
	github.com/golang/groupcache v0.0.0-20200121045136-8c9f03a8e57e // indirect
	github.com/golang/mock v1.4.1
	github.com/golang/protobuf v1.3.5 // indirect
	github.com/google/renameio v0.1.0
	github.com/google/uuid v1.1.1
	github.com/gorilla/handlers v1.4.2 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.2.0
	github.com/hpcloud/tail v1.0.0
	github.com/imdario/mergo v0.3.9 // indirect
	github.com/kr/pty v1.1.8
	github.com/mrtazz/checkmake v0.0.0-20191009095831-03dd76b964dd // indirect
	github.com/olekukonko/tablewriter v0.0.4 // indirect
	github.com/onsi/ginkgo v1.12.0
	github.com/onsi/gomega v1.9.0
	github.com/opencontainers/go-digest v1.0.0-rc1
	github.com/opencontainers/image-spec v1.0.2-0.20200206005212-79b036d80240
	github.com/opencontainers/runc v1.0.0-rc9
	github.com/opencontainers/runtime-spec v1.0.2
	github.com/opencontainers/runtime-tools v0.9.1-0.20200121211434-d1bf3e66ff0a
	github.com/opencontainers/selinux v1.4.0
	github.com/pkg/errors v0.9.1
	github.com/pkg/profile v1.4.0 // indirect
	github.com/pquerna/ffjson v0.0.0-20190930134022-aa0246cd15f7 // indirect
	github.com/prometheus/client_golang v1.5.1
	github.com/prometheus/common v0.9.1 // indirect
	github.com/prometheus/procfs v0.0.11 // indirect
	github.com/seccomp/containers-golang v0.3.2
	github.com/sirupsen/logrus v1.5.0
	github.com/soheilhy/cmux v0.1.4
	github.com/syndtr/gocapability v0.0.0-20180916011248-d98352740cb2
	github.com/urfave/cli/v2 v2.1.1
	github.com/vbatts/git-validation v1.1.0
	github.com/vishvananda/netlink v1.1.0
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	go.opencensus.io v0.22.3 // indirect
	golang.org/x/crypto v0.0.0-20200323165209-0ec3e9974c59 // indirect
	golang.org/x/net v0.0.0-20200324143707-d3edc9973b7e
	golang.org/x/sync v0.0.0-20200317015054-43a5402ce75a
	golang.org/x/sys v0.0.0-20200331124033-c3d80250170d
	golang.org/x/time v0.0.0-20190921001708-c4c64cad1fd0 // indirect
	google.golang.org/genproto v0.0.0-20200331122359-1ee6d9798940 // indirect
	google.golang.org/grpc v1.28.0
	gopkg.in/square/go-jose.v2 v2.4.1 // indirect
	k8s.io/api v0.18.0
	k8s.io/apimachinery v0.18.0
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/cri-api v0.0.0
	k8s.io/klog v1.0.0
	k8s.io/kubernetes v1.13.0
	k8s.io/release v0.2.5
	k8s.io/utils v0.0.0-20200229041039-0a110f9eb7ab
	mvdan.cc/sh/v3 v3.0.2
	sigs.k8s.io/structured-merge-diff/v3 v3.0.0 // indirect
)

replace (
	github.com/godbus/dbus => github.com/godbus/dbus v0.0.0-20190623212516-8a1682060722
	github.com/opencontainers/runtime-spec => github.com/opencontainers/runtime-spec v0.1.2-0.20190408193819-a1b50f621a48
	github.com/prometheus/client_golang => github.com/prometheus/client_golang v0.9.2
	k8s.io/api => k8s.io/kubernetes/staging/src/k8s.io/api v0.0.0-20200218170745-b7dcc4ac2981
	k8s.io/apiextensions-apiserver => k8s.io/kubernetes/staging/src/k8s.io/apiextensions-apiserver v0.0.0-20200218170745-b7dcc4ac2981
	k8s.io/apimachinery => k8s.io/kubernetes/staging/src/k8s.io/apimachinery v0.0.0-20200218170745-b7dcc4ac2981
	k8s.io/apiserver => k8s.io/kubernetes/staging/src/k8s.io/apiserver v0.0.0-20200218170745-b7dcc4ac2981
	k8s.io/cli-runtime => k8s.io/kubernetes/staging/src/k8s.io/cli-runtime v0.0.0-20200218170745-b7dcc4ac2981
	k8s.io/client-go => k8s.io/kubernetes/staging/src/k8s.io/client-go v0.0.0-20200218170745-b7dcc4ac2981
	k8s.io/cloud-provider => k8s.io/kubernetes/staging/src/k8s.io/cloud-provider v0.0.0-20200218170745-b7dcc4ac2981
	k8s.io/cluster-bootstrap => k8s.io/kubernetes/staging/src/k8s.io/cluster-bootstrap v0.0.0-20200218170745-b7dcc4ac2981
	k8s.io/code-generator => k8s.io/kubernetes/staging/src/k8s.io/code-generator v0.0.0-20200218170745-b7dcc4ac2981
	k8s.io/component-base => k8s.io/kubernetes/staging/src/k8s.io/component-base v0.0.0-20200218170745-b7dcc4ac2981
	k8s.io/cri-api => k8s.io/kubernetes/staging/src/k8s.io/cri-api v0.0.0-20200218170745-b7dcc4ac2981
	k8s.io/csi-translation-lib => k8s.io/kubernetes/staging/src/k8s.io/csi-translation-lib v0.0.0-20200218170745-b7dcc4ac2981
	k8s.io/kube-aggregator => k8s.io/kubernetes/staging/src/k8s.io/kube-aggregator v0.0.0-20200218170745-b7dcc4ac2981
	k8s.io/kube-controller-manager => k8s.io/kubernetes/staging/src/k8s.io/kube-controller-manager v0.0.0-20200218170745-b7dcc4ac2981
	k8s.io/kube-proxy => k8s.io/kubernetes/staging/src/k8s.io/kube-proxy v0.0.0-20200218170745-b7dcc4ac2981
	k8s.io/kube-scheduler => k8s.io/kubernetes/staging/src/k8s.io/kube-scheduler v0.0.0-20200218170745-b7dcc4ac2981
	k8s.io/kubectl => k8s.io/kubernetes/staging/src/k8s.io/kubectl v0.0.0-20200218170745-b7dcc4ac2981
	k8s.io/kubelet => k8s.io/kubernetes/staging/src/k8s.io/kubelet v0.0.0-20200218170745-b7dcc4ac2981
	k8s.io/kubernetes => k8s.io/kubernetes v1.18.0-beta.0
	k8s.io/legacy-cloud-providers => k8s.io/kubernetes/staging/src/k8s.io/legacy-cloud-providers v0.0.0-20200218170745-b7dcc4ac2981
	k8s.io/metrics => k8s.io/kubernetes/staging/src/k8s.io/metrics v0.0.0-20200218170745-b7dcc4ac2981
	k8s.io/sample-apiserver => k8s.io/kubernetes/staging/src/k8s.io/sample-apiserver v0.0.0-20200218170745-b7dcc4ac2981
)
