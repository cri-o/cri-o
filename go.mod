go 1.12

module github.com/cri-o/cri-o

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/Microsoft/go-winio v0.4.14
	github.com/blang/semver v3.5.1+incompatible
	github.com/containerd/cgroups v0.0.0-20180515175038-5e610833b720
	github.com/containerd/console v0.0.0-20170925154832-84eeaae905fa // indirect
	github.com/containerd/containerd v1.3.0
	github.com/containerd/fifo v0.0.0-20180307165137-3d5202aec260 // indirect
	github.com/containerd/go-runc v0.0.0-20180907222934-5a6d9f37cfa3 // indirect
	github.com/containerd/ttrpc v0.0.0-20190828172938-92c8520ef9f8
	github.com/containerd/typeurl v0.0.0-20180627222232-a93fcdb778cd // indirect
	github.com/containernetworking/cni v0.7.2-0.20190904153231-83439463f784
	github.com/containernetworking/plugins v0.8.2
	github.com/containers/buildah v1.11.5-0.20191031204705-20e92ffe0982
	github.com/containers/image/v5 v5.0.1-0.20200205124631-82291c45f2b0
	github.com/containers/libpod v1.6.3-0.20191111140219-de32b89eff09
	github.com/containers/storage v1.13.5
	github.com/coreos/go-systemd v0.0.0-20190719114852-fd7a80b32e1f
	github.com/cpuguy83/go-md2man v1.0.10
	github.com/cri-o/ocicni v0.1.1-0.20190920040751-deac903fd99b
	github.com/docker/docker v1.4.2-0.20190927142053-ada3c14355ce
	github.com/docker/go-units v0.4.0
	github.com/emicklei/go-restful v1.1.4-0.20170410110728-ff4f55a20633 // indirect
	github.com/fsnotify/fsnotify v1.4.7
	github.com/ghodss/yaml v1.0.1-0.20180820084758-c7ce16629ff4 // indirect
	github.com/go-zoo/bone v0.0.0-20170711140942-031b4005dfe2
	github.com/godbus/dbus v0.0.0-20181101234600-2ff6f7ffd60f
	github.com/gogo/protobuf v1.2.1
	github.com/golang/mock v1.2.0
	github.com/golangci/golangci-lint v1.21.0
	github.com/google/renameio v0.1.0
	github.com/googleapis/gnostic v0.0.0-20170729233727-0c5108395e2d // indirect
	github.com/gregjones/httpcache v0.0.0-20170728041850-787624de3eb7 // indirect
	github.com/hashicorp/go-multierror v1.0.0
	github.com/hashicorp/golang-lru v0.0.0-20160207214719-a0d98a5f2880 // indirect
	github.com/hpcloud/tail v1.0.0
	github.com/kr/pty v1.1.8
	github.com/lithammer/dedent v1.1.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.1 // indirect
	github.com/onsi/ginkgo v1.10.3
	github.com/onsi/gomega v1.7.1
	github.com/opencontainers/go-digest v1.0.0-rc1.0.20180430190053-c9281466c8b2
	github.com/opencontainers/image-spec v1.0.2-0.20190823105129-775207bd45b6
	github.com/opencontainers/runc v1.0.0-rc8.0.20190827142921-dd075602f158
	github.com/opencontainers/runtime-spec v0.1.2-0.20190618234442-a950415649c7
	github.com/opencontainers/runtime-tools v0.9.1-0.20200121211434-d1bf3e66ff0a
	github.com/opencontainers/selinux v1.3.0
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v1.1.0
	github.com/renstrom/dedent v1.0.0 // indirect
	github.com/seccomp/libseccomp-golang v0.9.1
	github.com/sirupsen/logrus v1.4.2
	github.com/soheilhy/cmux v0.1.4
	github.com/syndtr/gocapability v0.0.0-20180916011248-d98352740cb2
	github.com/urfave/cli v1.20.0
	github.com/vbatts/git-validation v1.0.0
	golang.org/x/net v0.0.0-20190923162816-aa69164e4478
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
	golang.org/x/sys v0.0.0-20190922100055-0a153f010e69
	google.golang.org/grpc v1.24.0
	k8s.io/api v0.0.0-20191004102349-159aefb8556b
	k8s.io/apiextensions-apiserver v0.0.0-20191004105649-b14e3c49469a // indirect
	k8s.io/apimachinery v0.0.0-20191004074956-c5d2f014d689
	k8s.io/apiserver v0.0.0-20191109015554-8577c320c87f // indirect
	k8s.io/client-go v11.0.1-0.20191004102930-01520b8320fc+incompatible
	k8s.io/cloud-provider v0.0.0-20191004111010-9775d7be8494 // indirect
	k8s.io/csi-api v0.0.0-20191004110013-47566b0fae2b // indirect
	k8s.io/klog v1.0.0
	k8s.io/kube-openapi v0.0.0-20190306001800-15615b16d372 // indirect
	k8s.io/kubernetes v1.14.10-beta.0.0.20191205115033-6b6e640a7aaf
	k8s.io/utils v0.0.0-20190607212802-c55fbcfc754a
	sigs.k8s.io/yaml v1.1.0 // indirect
)

replace (
	google.golang.org/grpc => github.com/grpc/grpc-go v1.23.0
	k8s.io/api => k8s.io/api v0.0.0-20190313235455-40a48860b5ab
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20190315093550-53c4693659ed
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190313205120-d7deff9243b1
	k8s.io/apiserver => k8s.io/apiserver v0.0.0-20190313205120-8b27c41bdbb1
	k8s.io/client-go => k8s.io/client-go v11.0.0+incompatible
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.0.0-20190314002645-c892ea32361a
	k8s.io/csi-api => k8s.io/csi-api v0.0.0-20190314001839-693d387aa133
	k8s.io/klog => k8s.io/klog v0.0.0-20181108234604-8139d8cb77af
	k8s.io/kubernetes => k8s.io/kubernetes v1.14.0
	sigs.k8s.io/yaml => github.com/kubernetes-sigs/yaml v1.1.0
)
