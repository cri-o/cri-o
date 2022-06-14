go 1.15

module github.com/cri-o/cri-o

require (
	github.com/BurntSushi/toml v0.4.1
	github.com/Microsoft/go-winio v0.5.1
	github.com/blang/semver v3.5.1+incompatible
	github.com/containerd/cgroups v1.0.2
	github.com/containerd/containerd v1.5.8
	github.com/containerd/cri-containerd v1.19.0
	github.com/containerd/ttrpc v1.1.0
	github.com/containerd/typeurl v1.0.2
	github.com/containernetworking/cni v1.1.1
	github.com/containernetworking/plugins v1.1.1
	github.com/containers/buildah v1.23.1
	github.com/containers/common v0.46.1-0.20211001143714-161e078e4c7f
	github.com/containers/conmon v2.0.20+incompatible
	github.com/containers/image/v5 v5.17.0
	github.com/containers/ocicrypt v1.1.2
	github.com/containers/podman/v3 v3.2.0-rc1.0.20211005134800-8bcc086b1b9d
	github.com/containers/storage v1.37.0
	github.com/coreos/go-systemd/v22 v22.3.2
	github.com/cpuguy83/go-md2man v1.0.10
	github.com/creack/pty v1.1.17
	github.com/cri-o/ocicni v0.4.0
	github.com/cyphar/filepath-securejoin v0.2.3
	github.com/docker/distribution v2.7.1+incompatible
	github.com/docker/go-units v0.4.0
	github.com/emicklei/go-restful v2.15.0+incompatible
	github.com/fsnotify/fsnotify v1.5.1
	github.com/go-logr/logr v1.2.0
	github.com/go-zoo/bone v1.3.0
	github.com/godbus/dbus/v5 v5.0.6
	github.com/gogo/protobuf v1.3.2
	github.com/golang/mock v1.6.0
	github.com/google/renameio v1.0.1
	github.com/google/uuid v1.3.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0
	github.com/intel/goresctrl v0.2.0
	github.com/json-iterator/go v1.1.12
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.17.0
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.2-0.20210819154149-5ad6f50d6283
	github.com/opencontainers/runc v1.0.2
	github.com/opencontainers/runtime-spec v1.0.3-0.20210709190330-896175883324
	github.com/opencontainers/runtime-tools v0.9.1-0.20210326182921-59cdde06764b
	github.com/opencontainers/selinux v1.9.1
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.11.0
	github.com/psampaz/go-mod-outdated v0.8.0
	github.com/sirupsen/logrus v1.8.1
	github.com/soheilhy/cmux v0.1.5
	github.com/stretchr/testify v1.7.0
	github.com/syndtr/gocapability v0.0.0-20200815063812-42c35b437635
	github.com/urfave/cli/v2 v2.3.0
	github.com/vishvananda/netlink v1.1.1-0.20210811191823-e1a867c6b452
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.26.1
	go.opentelemetry.io/otel v1.2.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.2.0
	go.opentelemetry.io/otel/sdk v1.2.0
	golang.org/x/net v0.0.0-20211005001312-d4b1ae081e3b
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20211004093028-2c5d950f24ef
	google.golang.org/grpc v1.42.0
	k8s.io/api v0.22.2
	k8s.io/apimachinery v0.22.2
	k8s.io/client-go v0.22.2
	k8s.io/cri-api v0.23.0-alpha.3
	k8s.io/klog/v2 v2.30.0
	k8s.io/kubernetes v1.23.0
	k8s.io/release v0.8.0
	k8s.io/utils v0.0.0-20210930125809-cb0fa318a74b
	mvdan.cc/sh/v3 v3.3.1
	sigs.k8s.io/release-utils v0.3.0
	sigs.k8s.io/yaml v1.3.0
	sigs.k8s.io/zeitgeist v0.3.0
)

replace (
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20200117163144-32f20d992d24
	k8s.io/api => k8s.io/kubernetes/staging/src/k8s.io/api v0.0.0-20211207180839-ab69524f795c
	k8s.io/apiextensions-apiserver => k8s.io/kubernetes/staging/src/k8s.io/apiextensions-apiserver v0.0.0-20211207180839-ab69524f795c
	k8s.io/apimachinery => k8s.io/kubernetes/staging/src/k8s.io/apimachinery v0.0.0-20211207180839-ab69524f795c
	k8s.io/apiserver => k8s.io/kubernetes/staging/src/k8s.io/apiserver v0.0.0-20211207180839-ab69524f795c
	k8s.io/cli-runtime => k8s.io/kubernetes/staging/src/k8s.io/cli-runtime v0.0.0-20211207180839-ab69524f795c
	k8s.io/client-go => k8s.io/kubernetes/staging/src/k8s.io/client-go v0.0.0-20211207180839-ab69524f795c
	k8s.io/cloud-provider => k8s.io/kubernetes/staging/src/k8s.io/cloud-provider v0.0.0-20211207180839-ab69524f795c
	k8s.io/cluster-bootstrap => k8s.io/kubernetes/staging/src/k8s.io/cluster-bootstrap v0.0.0-20211207180839-ab69524f795c
	k8s.io/code-generator => k8s.io/kubernetes/staging/src/k8s.io/code-generator v0.0.0-20211207180839-ab69524f795c
	k8s.io/component-base => k8s.io/kubernetes/staging/src/k8s.io/component-base v0.0.0-20211207180839-ab69524f795c
	k8s.io/component-helpers => k8s.io/kubernetes/staging/src/k8s.io/component-helpers v0.0.0-20211207180839-ab69524f795c
	k8s.io/controller-manager => k8s.io/kubernetes/staging/src/k8s.io/controller-manager v0.0.0-20211207180839-ab69524f795c
	k8s.io/cri-api => k8s.io/kubernetes/staging/src/k8s.io/cri-api v0.0.0-20210928072042-90051602459c
	k8s.io/csi-translation-lib => k8s.io/kubernetes/staging/src/k8s.io/csi-translation-lib v0.0.0-20211207180839-ab69524f795c
	k8s.io/kube-aggregator => k8s.io/kubernetes/staging/src/k8s.io/kube-aggregator v0.0.0-20211207180839-ab69524f795c
	k8s.io/kube-controller-manager => k8s.io/kubernetes/staging/src/k8s.io/kube-controller-manager v0.0.0-20211207180839-ab69524f795c
	k8s.io/kube-proxy => k8s.io/kubernetes/staging/src/k8s.io/kube-proxy v0.0.0-20211207180839-ab69524f795c
	k8s.io/kube-scheduler => k8s.io/kubernetes/staging/src/k8s.io/kube-scheduler v0.0.0-20211207180839-ab69524f795c
	k8s.io/kubectl => k8s.io/kubernetes/staging/src/k8s.io/kubectl v0.0.0-20211207180839-ab69524f795c
	k8s.io/kubelet => k8s.io/kubernetes/staging/src/k8s.io/kubelet v0.0.0-20211207180839-ab69524f795c
	k8s.io/kubernetes => k8s.io/kubernetes v1.23.0
	k8s.io/legacy-cloud-providers => k8s.io/kubernetes/staging/src/k8s.io/legacy-cloud-providers v0.0.0-20211207180839-ab69524f795c
	k8s.io/metrics => k8s.io/kubernetes/staging/src/k8s.io/metrics v0.0.0-20211207180839-ab69524f795c
	k8s.io/mount-utils => k8s.io/kubernetes/staging/src/k8s.io/mount-utils v0.0.0-20211207180839-ab69524f795c
	k8s.io/pod-security-admission => k8s.io/kubernetes/staging/src/k8s.io/pod-security-admission v0.0.0-20211207180839-ab69524f795c
	k8s.io/sample-apiserver => k8s.io/kubernetes/staging/src/k8s.io/sample-apiserver v0.0.0-20211207180839-ab69524f795c
)
