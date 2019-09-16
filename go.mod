go 1.12

module github.com/cri-o/cri-o

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/Microsoft/go-winio v0.4.12
	github.com/blang/semver v3.5.1+incompatible
	github.com/containerd/containerd v1.2.7
	github.com/containerd/fifo v0.0.0-20190226154929-a9fb20d87448 // indirect
	github.com/containerd/go-runc v0.0.0-20190226155025-7d11b49dc076 // indirect
	github.com/containerd/project v0.0.0-20190306185219-831961d1e0c8
	github.com/containerd/ttrpc v0.0.0-20180920185216-2a805f718635
	github.com/containernetworking/cni v0.7.1
	github.com/containernetworking/plugins v0.8.1
	github.com/containers/buildah v1.10.1
	github.com/containers/image v3.0.2+incompatible
	github.com/containers/libpod v1.5.0
	github.com/containers/storage v1.13.3
	github.com/coreos/go-systemd v0.0.0-20190620071333-e64a0ec8b42a
	github.com/cpuguy83/go-md2man v1.0.10
	github.com/cri-o/ocicni v0.1.1-0.20190702175919-7762645d18ca
	github.com/docker/docker v0.7.3-0.20190410184157-6d18c6a06295
	github.com/docker/go-units v0.4.0
	github.com/fsnotify/fsnotify v1.4.7
	github.com/go-zoo/bone v1.3.0
	github.com/godbus/dbus v0.0.0-20181101234600-2ff6f7ffd60f
	github.com/gogo/protobuf v1.2.1
	github.com/golang/mock v1.3.1
	github.com/golangci/golangci-lint v1.17.1
	github.com/google/renameio v0.1.0
	github.com/hpcloud/tail v1.0.0
	github.com/kr/pty v1.1.5
	github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega v1.5.0
	github.com/opencontainers/go-digest v1.0.0-rc1
	github.com/opencontainers/image-spec v1.0.1
	github.com/opencontainers/runc v1.0.0-rc8
	github.com/opencontainers/runtime-spec v1.0.1
	github.com/opencontainers/runtime-tools v0.9.0
	github.com/opencontainers/selinux v1.2.2
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v1.0.0
	github.com/seccomp/containers-golang v0.3.1
	github.com/sirupsen/logrus v1.4.2
	github.com/soheilhy/cmux v0.1.4
	github.com/syndtr/gocapability v0.0.0-20180916011248-d98352740cb2
	github.com/urfave/cli v1.20.0
	github.com/vbatts/git-validation v1.0.0
	golang.org/x/net v0.0.0-20190628185345-da137c7871d7
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
	golang.org/x/sys v0.0.0-20190626221950-04f50cda93cb
	google.golang.org/grpc v1.21.1
	k8s.io/api v0.0.0
	k8s.io/apimachinery v0.0.0
	k8s.io/client-go v0.0.0
	k8s.io/cri-api v0.0.0
	k8s.io/kubernetes v0.0.0
	k8s.io/utils v0.0.0-20190607212802-c55fbcfc754a
)

replace (
	github.com/opencontainers/runtime-spec => github.com/opencontainers/runtime-spec v0.1.2-0.20190408193819-a1b50f621a48
	k8s.io/api => k8s.io/kubernetes/staging/src/k8s.io/api v0.0.0-20190615005809-e8462b5b5dc2
	k8s.io/apiextensions-apiserver => k8s.io/kubernetes/staging/src/k8s.io/apiextensions-apiserver v0.0.0-20190615005809-e8462b5b5dc2
	k8s.io/apimachinery => k8s.io/kubernetes/staging/src/k8s.io/apimachinery v0.0.0-20190615005809-e8462b5b5dc2
	k8s.io/apiserver => k8s.io/kubernetes/staging/src/k8s.io/apiserver v0.0.0-20190615005809-e8462b5b5dc2
	k8s.io/cli-runtime => k8s.io/kubernetes/staging/src/k8s.io/cli-runtime v0.0.0-20190615005809-e8462b5b5dc2
	k8s.io/client-go => k8s.io/kubernetes/staging/src/k8s.io/client-go v0.0.0-20190615005809-e8462b5b5dc2
	k8s.io/cloud-provider => k8s.io/kubernetes/staging/src/k8s.io/cloud-provider v0.0.0-20190615005809-e8462b5b5dc2
	k8s.io/cluster-bootstrap => k8s.io/kubernetes/staging/src/k8s.io/cluster-bootstrap v0.0.0-20190615005809-e8462b5b5dc2
	k8s.io/code-generator => k8s.io/kubernetes/staging/src/k8s.io/code-generator v0.0.0-20190615005809-e8462b5b5dc2
	k8s.io/component-base => k8s.io/kubernetes/staging/src/k8s.io/component-base v0.0.0-20190615005809-e8462b5b5dc2
	k8s.io/cri-api => k8s.io/kubernetes/staging/src/k8s.io/cri-api v0.0.0-20190615005809-e8462b5b5dc2
	k8s.io/csi-translation-lib => k8s.io/kubernetes/staging/src/k8s.io/csi-translation-lib v0.0.0-20190615005809-e8462b5b5dc2
	k8s.io/kube-aggregator => k8s.io/kubernetes/staging/src/k8s.io/kube-aggregator v0.0.0-20190615005809-e8462b5b5dc2
	k8s.io/kube-controller-manager => k8s.io/kubernetes/staging/src/k8s.io/kube-controller-manager v0.0.0-20190615005809-e8462b5b5dc2
	k8s.io/kube-proxy => k8s.io/kubernetes/staging/src/k8s.io/kube-proxy v0.0.0-20190615005809-e8462b5b5dc2
	k8s.io/kube-scheduler => k8s.io/kubernetes/staging/src/k8s.io/kube-scheduler v0.0.0-20190615005809-e8462b5b5dc2
	k8s.io/kubelet => k8s.io/kubernetes/staging/src/k8s.io/kubelet v0.0.0-20190615005809-e8462b5b5dc2
	k8s.io/kubernetes => k8s.io/kubernetes v1.15.0
	k8s.io/legacy-cloud-providers => k8s.io/kubernetes/staging/src/k8s.io/legacy-cloud-providers v0.0.0-20190615005809-e8462b5b5dc2
	k8s.io/metrics => k8s.io/kubernetes/staging/src/k8s.io/metrics v0.0.0-20190615005809-e8462b5b5dc2
	k8s.io/sample-apiserver => k8s.io/kubernetes/staging/src/k8s.io/sample-apiserver v0.0.0-20190615005809-e8462b5b5dc2
)
