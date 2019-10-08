go 1.12

module github.com/cri-o/cri-o

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/Microsoft/go-winio v0.4.11
	github.com/Microsoft/hcsshim v0.6.2 // indirect
	github.com/VividCortex/ewma v1.1.1 // indirect
	github.com/beorn7/perks v0.0.0-20160229213445-3ac7bf7a47d1 // indirect
	github.com/blang/semver v3.5.0+incompatible
	github.com/checkpoint-restore/go-criu v0.0.0-20181120144056-17b0214f6c48 // indirect
	github.com/containerd/cgroups v0.0.0-20180515175038-5e610833b720
	github.com/containerd/console v0.0.0-20170925154832-84eeaae905fa // indirect
	github.com/containerd/containerd v1.2.2
	github.com/containerd/fifo v0.0.0-20180307165137-3d5202aec260 // indirect
	github.com/containerd/go-runc v0.0.0-20180907222934-5a6d9f37cfa3 // indirect
	github.com/containerd/ttrpc v0.0.0-20190828172938-92c8520ef9f8
	github.com/containerd/typeurl v0.0.0-20180627222232-a93fcdb778cd // indirect
	github.com/containernetworking/cni v0.7.2-0.20190904153231-83439463f784
	github.com/containernetworking/plugins v0.7.5
	github.com/containers/buildah v1.11.4
	github.com/containers/image/v5 v5.0.0
	github.com/containers/libpod v1.6.3-0.20191111140219-de32b89eff09
	github.com/containers/psgo v1.3.0 // indirect
	github.com/containers/storage v1.12.11-0.20190912205451-e30d5056f279
	github.com/coreos/go-iptables v0.3.1-0.20180704133345-25d087f3cffd // indirect
	github.com/coreos/go-systemd v0.0.0-20161114122254-48702e0da86b
	github.com/coreos/pkg v0.0.0-20160727233714-3ac0863d7acf // indirect
	github.com/cri-o/ocicni v0.1.1-0.20190920040751-deac903fd99b
	github.com/cyphar/filepath-securejoin v0.2.1 // indirect
	github.com/docker/distribution v2.6.0-rc.1.0.20170817175659-5f6282db7d65+incompatible // indirect
	github.com/docker/docker v1.4.2-0.20190307005417-54dddadc7d5d
	github.com/docker/docker-credential-helpers v0.6.1 // indirect
	github.com/docker/go-units v0.3.3
	github.com/docker/libnetwork v0.8.0-dev.2.0.20190301200423-5f7a3f68c3d9 // indirect
	github.com/docker/libtrust v0.0.0-20160708172513-aabc10ec26b7 // indirect
	github.com/docker/spdystream v0.0.0-20181023171402-6480d4af844c // indirect
	github.com/emicklei/go-restful v1.1.4-0.20170410110728-ff4f55a20633 // indirect
	github.com/etcd-io/bbolt v1.3.3 // indirect
	github.com/fsnotify/fsnotify v1.4.7
	github.com/fsouza/go-dockerclient v1.3.0 // indirect
	github.com/ghodss/yaml v1.0.1-0.20180820084758-c7ce16629ff4 // indirect
	github.com/go-zoo/bone v0.0.0-20170711140942-031b4005dfe2
	github.com/godbus/dbus v0.0.0-20181101234600-2ff6f7ffd60f
	github.com/gogo/protobuf v1.1.1
	github.com/golang/groupcache v0.0.0-20170421005642-b710c8433bd1 // indirect
	github.com/golang/mock v1.2.0
	github.com/google/btree v0.0.0-20160524151835-7d79101e329e // indirect
	github.com/google/gofuzz v0.0.0-20161122191042-44d81051d367 // indirect
	github.com/google/renameio v0.1.0
	github.com/googleapis/gnostic v0.0.0-20170729233727-0c5108395e2d // indirect
	github.com/gregjones/httpcache v0.0.0-20170728041850-787624de3eb7 // indirect
	github.com/hashicorp/errwrap v0.0.0-20141028054710-7554cd9344ce // indirect
	github.com/hashicorp/go-multierror v0.0.0-20170622060955-83588e72410a // indirect
	github.com/hashicorp/golang-lru v0.0.0-20160207214719-a0d98a5f2880 // indirect
	github.com/hpcloud/tail v1.0.0
	github.com/imdario/mergo v0.0.0-20160216103600-3e95a51e0639 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/ishidawataru/sctp v0.0.0-20180213033435-07191f837fed // indirect
	github.com/json-iterator/go v0.0.0-20180612202835-f2b4162afba3 // indirect
	github.com/klauspost/compress v1.4.1 // indirect
	github.com/klauspost/cpuid v1.2.0 // indirect
	github.com/klauspost/pgzip v1.2.1 // indirect
	github.com/kr/pty v1.0.0
	github.com/lithammer/dedent v1.1.0 // indirect
	github.com/mattn/go-isatty v0.0.4 // indirect
	github.com/mattn/go-shellwords v1.0.6 // indirect
	github.com/matttproud/golang_protobuf_extensions v0.0.0-20150406173934-fc2b8d3a73c4 // indirect
	github.com/mistifyio/go-zfs v2.1.1+incompatible // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v0.0.0-20180320133207-05fbef0ca5da // indirect
	github.com/mrunalp/fileutils v0.0.0-20171103030105-7d4729fb3618 // indirect
	github.com/mtrmac/gpgme v0.0.0-20170102180018-b2432428689c // indirect
	github.com/onsi/ginkgo v1.10.3
	github.com/onsi/gomega v1.7.1
	github.com/opencontainers/go-digest v1.0.0-rc1.0.20180430190053-c9281466c8b2
	github.com/opencontainers/image-spec v1.0.1
	github.com/opencontainers/runc v1.0.0-rc6.0.20190322180631-11fc498ffa5c
	github.com/opencontainers/runtime-spec v0.1.2-0.20190618234442-a950415649c7
	github.com/opencontainers/runtime-tools v0.6.0
	github.com/opencontainers/selinux v1.0.1-0.20190313214808-9e2c5215628a
	github.com/openshift/imagebuilder v0.0.0-20190308124740-705fe9255c57 // indirect
	github.com/opentracing/opentracing-go v1.0.3-0.20190218023034-25a84ff92183 // indirect
	github.com/ostreedev/ostree-go v0.0.0-20181213164143-d0388bd827cf // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pkg/errors v0.8.0
	github.com/pquerna/ffjson v0.0.0-20171002144729-d49c2bc1aa13 // indirect
	github.com/prometheus/client_golang v0.8.1-0.20170531130054-e7e903064f5e
	github.com/prometheus/client_model v0.0.0-20150212101744-fa8ad6fec335 // indirect
	github.com/prometheus/common v0.0.0-20170427095455-13ba4ddd0caa // indirect
	github.com/prometheus/procfs v0.0.0-20170519190837-65c1f6f8f0fc // indirect
	github.com/renstrom/dedent v1.0.0 // indirect
	github.com/seccomp/containers-golang v0.0.0-20180629143253-cdfdaa7543f4 // indirect
	github.com/seccomp/libseccomp-golang v0.9.0
	github.com/sirupsen/logrus v1.4.2
	github.com/soheilhy/cmux v0.1.4
	github.com/spf13/cobra v0.0.3 // indirect
	github.com/spf13/pflag v1.0.1 // indirect
	github.com/syndtr/gocapability v0.0.0-20160928074757-e7cb7fa329f4
	github.com/tchap/go-patricia v2.2.6+incompatible // indirect
	github.com/ulikunitz/xz v0.5.4 // indirect
	github.com/urfave/cli v1.20.0
	github.com/vbatts/tar-split v0.10.2 // indirect
	github.com/vbauerster/mpb v3.4.0+incompatible // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20180127040702-4e3ac2762d5f // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.1.0 // indirect
	golang.org/x/net v0.0.0-20190311183353-d8887717615a
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
	golang.org/x/sys v0.0.0-20190425145619-16072639606e
	golang.org/x/text v0.3.1-0.20181211190257-17bcc049122f // indirect
	golang.org/x/time v0.0.0-20161028155119-f51c12702a4d // indirect
	google.golang.org/grpc v1.23.0
	gopkg.in/inf.v0 v0.9.0 // indirect
	gopkg.in/mgo.v2 v2.0.0-20180705113604-9856a29383ce // indirect
	k8s.io/api release-1.14
	k8s.io/apiextensions-apiserver release-1.14
	k8s.io/apimachinery release-1.14
	k8s.io/apiserver release-1.14
	k8s.io/client-go kubernetes-1.14.8
	k8s.io/cloud-provider release-1.14
	k8s.io/csi-api release-1.14 // indirect
	k8s.io/klog v0.0.0-20181108234604-8139d8cb77af // indirect
	k8s.io/kube-openapi v0.0.0-20190306001800-15615b16d372 // indirect
	k8s.io/kubernetes release-1.14
	k8s.io/utils v0.0.0-20190221042446-c2654d5206da
	sigs.k8s.io/yaml v1.1.0 // indirect
)

replace (
	k8s.io/api => k8s.io/api kubernetes-1.14.0
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver kubernetes-1.14.0
	k8s.io/apimachinery => k8s.io/apimachinery kubernetes-1.14.0
	k8s.io/apiserver => k8s.io/apiserver kubernetes-1.14.0
	k8s.io/client-go => k8s.io/client-go kubernetes-1.14.0
	k8s.io/cloud-provider => k8s.io/cloud-provider kubernetes-1.14.0
	k8s.io/csi-api => k8s.io/csi-api kubernetes-1.14.0
	k8s.io/klog => k8s.io/klog v0.0.0-20181108234604-8139d8cb77af
	k8s.io/kubernetes => k8s.io/kubernetes v1.14.0
	google.golang.org/grpc => github.com/grpc/grpc-go v1.23.0
	sigs.k8s.io/yaml => github.com/kubernetes-sigs/yaml v1.1.0
)
