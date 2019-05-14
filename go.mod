go 1.12

module github.com/cri-o/cri-o

require (
	github.com/14rcole/gopopulate v0.0.0-20180821133914-b175b219e774 // indirect
	github.com/BurntSushi/toml v0.3.1
	github.com/Microsoft/go-winio v0.4.12
	github.com/Microsoft/hcsshim v0.6.2 // indirect
	github.com/VividCortex/ewma v1.1.1 // indirect
	github.com/checkpoint-restore/go-criu v0.0.0-20181120144056-17b0214f6c48 // indirect
	github.com/containerd/cgroups v0.0.0-20180515175038-5e610833b720
	github.com/containerd/containerd v1.2.6
	github.com/containerd/fifo v0.0.0-20180307165137-3d5202aec260 // indirect
	github.com/containerd/go-runc v0.0.0-20180907222934-5a6d9f37cfa3 // indirect
	github.com/containerd/project v0.0.0-20190306185219-831961d1e0c8
	github.com/containerd/ttrpc v0.0.0-20180920185216-2a805f718635
	github.com/containernetworking/cni v0.7.0
	github.com/containernetworking/plugins v0.7.5
	github.com/containers/buildah v1.7.2
	github.com/containers/image v1.5.1
	github.com/containers/libpod v1.3.0
	github.com/containers/psgo v0.0.0-20180822121435-5dde6da0bc88 // indirect
	github.com/containers/storage v1.12.6
	github.com/coreos/go-iptables v0.0.0-20180704133345-25d087f3cffd // indirect
	github.com/coreos/go-systemd v0.0.0-20180511133405-39ca1b05acc7
	github.com/cpuguy83/go-md2man v1.0.10
	github.com/cri-o/ocicni v0.0.0-20190328132530-0c180f981b27
	github.com/cyphar/filepath-securejoin v0.2.1 // indirect
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/docker v0.7.3-0.20190410184157-6d18c6a06295
	github.com/docker/docker-credential-helpers v0.6.1 // indirect
	github.com/docker/go-metrics v0.0.0-20181218153428-b84716841b82 // indirect
	github.com/docker/go-units v0.3.3
	github.com/docker/libtrust v0.0.0-20160708172513-aabc10ec26b7 // indirect
	github.com/docker/spdystream v0.0.0-20181023171402-6480d4af844c // indirect
	github.com/elazarl/goproxy v0.0.0-20190410145444-c548f45dcf1d // indirect
	github.com/elazarl/goproxy/ext v0.0.0-20190410145444-c548f45dcf1d // indirect
	github.com/fsnotify/fsnotify v1.4.7
	github.com/fsouza/go-dockerclient v1.3.0 // indirect
	github.com/go-zoo/bone v1.3.0
	github.com/godbus/dbus v4.1.0+incompatible
	github.com/gogo/protobuf v1.2.1
	github.com/golang/groupcache v0.0.0-20170421005642-b710c8433bd1 // indirect
	github.com/golang/mock v1.3.0
	github.com/golangci/golangci-lint v1.16.1-0.20190402065613-de1d1ad903cd
	github.com/google/uuid v1.1.1 // indirect
	github.com/hashicorp/errwrap v0.0.0-20141028054710-7554cd9344ce // indirect
	github.com/hashicorp/go-multierror v0.0.0-20170622060955-83588e72410a // indirect
	github.com/hashicorp/go-version v1.1.0 // indirect
	github.com/hpcloud/tail v1.0.0
	github.com/klauspost/compress v1.4.1 // indirect
	github.com/klauspost/cpuid v1.2.0 // indirect
	github.com/klauspost/pgzip v1.2.1 // indirect
	github.com/kr/pty v1.1.4
	github.com/mattn/go-isatty v0.0.4 // indirect
	github.com/mattn/go-shellwords v1.0.5 // indirect
	github.com/mistifyio/go-zfs v2.1.1+incompatible // indirect
	github.com/mrunalp/fileutils v0.0.0-20171103030105-7d4729fb3618 // indirect
	github.com/mtrmac/gpgme v0.0.0-20170102180018-b2432428689c // indirect
	github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega v1.5.0
	github.com/opencontainers/go-digest v1.0.0-rc1
	github.com/opencontainers/image-spec v1.0.1
	github.com/opencontainers/runc v1.0.0-rc8
	github.com/opencontainers/runtime-spec v1.0.1
	github.com/opencontainers/runtime-tools v0.3.1-0.20190418135848-095789df6c2b
	github.com/opencontainers/selinux v1.2.2
	github.com/openshift/imagebuilder v0.0.0-20190308124740-705fe9255c57 // indirect
	github.com/opentracing/opentracing-go v0.0.0-20190218023034-25a84ff92183 // indirect
	github.com/ostreedev/ostree-go v0.0.0-20181213164143-d0388bd827cf // indirect
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v0.9.2
	github.com/prometheus/common v0.3.0 // indirect
	github.com/seccomp/containers-golang v0.0.0-20180629143253-cdfdaa7543f4 // indirect
	github.com/seccomp/libseccomp-golang v0.9.0
	github.com/sirupsen/logrus v1.4.1
	github.com/soheilhy/cmux v0.1.4
	github.com/spf13/afero v1.2.2 // indirect
	github.com/syndtr/gocapability v0.0.0-20160928074757-e7cb7fa329f4
	github.com/tchap/go-patricia v2.2.6+incompatible // indirect
	github.com/ulikunitz/xz v0.5.4 // indirect
	github.com/urfave/cli v1.20.0
	github.com/vbatts/git-validation v0.0.0-20181108135710-7b29366a0dd3
	github.com/vbatts/tar-split v0.11.1 // indirect
	github.com/vbauerster/mpb v3.4.0+incompatible // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20180127040702-4e3ac2762d5f // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.1.0 // indirect
	golang.org/x/net v0.0.0-20190424112056-4829fb13d2c6
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
	golang.org/x/sys v0.0.0-20190425145619-16072639606e
	golang.org/x/text v0.3.2 // indirect
	google.golang.org/grpc v1.20.1
	k8s.io/api v0.0.0
	k8s.io/apimachinery v0.0.0
	k8s.io/client-go v0.0.0
	k8s.io/cri-api v0.0.0
	k8s.io/kubernetes v0.0.0
	k8s.io/utils v0.0.0-20190221042446-c2654d5206da
)

replace (
	cloud.google.com/go => cloud.google.com/go v0.34.0
	github.com/opencontainers/runtime-spec => github.com/opencontainers/runtime-spec v0.1.2-0.20190408193819-a1b50f621a48
	k8s.io/api => k8s.io/kubernetes/staging/src/k8s.io/api v0.0.0-20190429174420-306740f81c3b
	k8s.io/apiextensions-apiserver => k8s.io/kubernetes/staging/src/k8s.io/apiextensions-apiserver v0.0.0-20190429174420-306740f81c3b
	k8s.io/apimachinery => k8s.io/kubernetes/staging/src/k8s.io/apimachinery v0.0.0-20190429174420-306740f81c3b
	k8s.io/apiserver => k8s.io/kubernetes/staging/src/k8s.io/apiserver v0.0.0-20190429174420-306740f81c3b
	k8s.io/cli-runtime => k8s.io/kubernetes/staging/src/k8s.io/cli-runtime v0.0.0-20190429174420-306740f81c3b
	k8s.io/client-go => k8s.io/kubernetes/staging/src/k8s.io/client-go v0.0.0-20190429174420-306740f81c3b
	k8s.io/cloud-provider => k8s.io/kubernetes/staging/src/k8s.io/cloud-provider v0.0.0-20190429174420-306740f81c3b
	k8s.io/cluster-bootstrap => k8s.io/kubernetes/staging/src/k8s.io/cluster-bootstrap v0.0.0-20190429174420-306740f81c3b
	k8s.io/code-generator => k8s.io/kubernetes/staging/src/k8s.io/code-generator v0.0.0-20190429174420-306740f81c3b
	k8s.io/component-base => k8s.io/kubernetes/staging/src/k8s.io/component-base v0.0.0-20190429174420-306740f81c3b
	k8s.io/cri-api => k8s.io/kubernetes/staging/src/k8s.io/cri-api v0.0.0-20190429174420-306740f81c3b
	k8s.io/csi-translation-lib => k8s.io/kubernetes/staging/src/k8s.io/csi-translation-lib v0.0.0-20190429174420-306740f81c3b
	k8s.io/kube-aggregator => k8s.io/kubernetes/staging/src/k8s.io/kube-aggregator v0.0.0-20190429174420-306740f81c3b
	k8s.io/kube-controller-manager => k8s.io/kubernetes/staging/src/k8s.io/kube-controller-manager v0.0.0-20190429174420-306740f81c3b
	k8s.io/kube-proxy => k8s.io/kubernetes/staging/src/k8s.io/kube-proxy v0.0.0-20190429174420-306740f81c3b
	k8s.io/kube-scheduler => k8s.io/kubernetes/staging/src/k8s.io/kube-scheduler v0.0.0-20190429174420-306740f81c3b
	k8s.io/kubelet => k8s.io/kubernetes/staging/src/k8s.io/kubelet v0.0.0-20190429174420-306740f81c3b
	k8s.io/kubernetes => k8s.io/kubernetes v1.15.0-alpha.2
	k8s.io/legacy-cloud-providers => k8s.io/kubernetes/staging/src/k8s.io/legacy-cloud-providers v0.0.0-20190429174420-306740f81c3b
	k8s.io/metrics => k8s.io/kubernetes/staging/src/k8s.io/metrics v0.0.0-20190429174420-306740f81c3b
	k8s.io/sample-apiserver => k8s.io/kubernetes/staging/src/k8s.io/sample-apiserver v0.0.0-20190429174420-306740f81c3b
)
