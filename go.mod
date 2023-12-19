go 1.21

module github.com/cri-o/cri-o

require (
	github.com/BurntSushi/toml v1.3.2
	github.com/Microsoft/go-winio v0.6.1
	github.com/blang/semver v3.5.1+incompatible
	github.com/blang/semver/v4 v4.0.0
	github.com/checkpoint-restore/checkpointctl v1.1.0
	github.com/checkpoint-restore/go-criu/v7 v7.0.0
	github.com/containerd/cgroups v1.1.0
	github.com/containerd/containerd v1.7.11
	github.com/containerd/cri-containerd v1.19.0
	github.com/containerd/fifo v1.1.0
	github.com/containerd/nri v0.5.0
	github.com/containerd/ttrpc v1.2.2
	github.com/containerd/typeurl v1.0.3-0.20220422153119-7f6e6d160d67
	github.com/containernetworking/cni v1.1.2
	github.com/containernetworking/plugins v1.4.0
	github.com/containers/common v0.57.1
	github.com/containers/conmon v2.0.20+incompatible
	github.com/containers/conmon-rs v0.6.2-0.20230920142715-f5a362044a57
	github.com/containers/image/v5 v5.29.0
	github.com/containers/kubensmnt v1.2.0
	github.com/containers/ocicrypt v1.1.9
	github.com/containers/podman/v4 v4.8.2
	github.com/containers/storage v1.51.0
	github.com/coreos/go-systemd/v22 v22.5.1-0.20231103132048-7d375ecc2b09
	github.com/cpuguy83/go-md2man v1.0.10
	github.com/creack/pty v1.1.21
	github.com/cri-o/ocicni v0.4.1
	github.com/cyphar/filepath-securejoin v0.2.4
	github.com/docker/distribution v2.8.3+incompatible
	github.com/docker/go-units v0.5.0
	github.com/fsnotify/fsnotify v1.7.0
	github.com/go-chi/chi/v5 v5.0.10
	github.com/go-logr/logr v1.3.0
	github.com/godbus/dbus/v5 v5.1.1-0.20230522191255-76236955d466
	github.com/gogo/protobuf v1.3.2
	github.com/golang/mock v1.6.0
	github.com/google/go-github/v50 v50.2.0
	github.com/google/renameio v1.0.1
	github.com/google/uuid v1.5.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0
	github.com/intel/goresctrl v0.5.0
	github.com/json-iterator/go v1.1.12
	github.com/kata-containers/kata-containers/src/runtime v0.0.0-20231214145909-c7c763220340
	github.com/onsi/ginkgo/v2 v2.13.2
	github.com/onsi/gomega v1.30.0
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.1.0-rc5
	github.com/opencontainers/runc v1.1.10
	github.com/opencontainers/runtime-spec v1.1.1-0.20230922153023-c0e90434df2a
	github.com/opencontainers/runtime-tools v0.9.1-0.20230914150019-408c51e934dc
	github.com/opencontainers/selinux v1.11.0
	github.com/prometheus/client_golang v1.17.0
	github.com/seccomp/libseccomp-golang v0.10.0
	github.com/sirupsen/logrus v1.9.3
	github.com/soheilhy/cmux v0.1.5
	github.com/stretchr/testify v1.8.4
	github.com/syndtr/gocapability v0.0.0-20200815063812-42c35b437635
	github.com/uptrace/opentelemetry-go-extra/otellogrus v0.2.3
	github.com/urfave/cli/v2 v2.26.0
	github.com/vishvananda/netlink v1.2.1-beta.2
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.46.1
	go.opentelemetry.io/otel v1.21.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.21.0
	go.opentelemetry.io/otel/sdk v1.21.0
	go.opentelemetry.io/otel/trace v1.21.0
	golang.org/x/net v0.19.0
	golang.org/x/sync v0.5.0
	golang.org/x/sys v0.15.0
	google.golang.org/grpc v1.60.0
	google.golang.org/protobuf v1.31.0
	k8s.io/api v0.29.0
	k8s.io/apimachinery v0.29.0
	k8s.io/client-go v0.29.0
	k8s.io/cri-api v0.29.0
	k8s.io/klog/v2 v2.110.1
	k8s.io/kubelet v0.29.0
	k8s.io/utils v0.0.0-20231127182322-b307cd553661
	sigs.k8s.io/release-sdk v0.10.4
	sigs.k8s.io/release-utils v0.7.7
	sigs.k8s.io/yaml v1.4.0
	tags.cncf.io/container-device-interface v0.6.2
)

require (
	capnproto.org/go/capnp/v3 v3.0.0-alpha.25 // indirect
	dario.cat/mergo v1.0.0 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20230124172434-306776ec8161 // indirect
	github.com/Microsoft/hcsshim v0.12.0-rc.1 // indirect
	github.com/ProtonMail/go-crypto v0.0.0-20230923063757-afb1ddc0824c // indirect
	github.com/VividCortex/ewma v1.2.0 // indirect
	github.com/acarl005/stripansi v0.0.0-20180116102854-5a71ef0e047d // indirect
	github.com/acomagu/bufpipe v1.0.4 // indirect
	github.com/aead/serpent v0.0.0-20160714141033-fba169763ea6 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff/v4 v4.2.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/chzyer/readline v1.5.1 // indirect
	github.com/cilium/ebpf v0.9.1 // indirect
	github.com/cloudflare/circl v1.3.5 // indirect
	github.com/containerd/cgroups/v3 v3.0.2 // indirect
	github.com/containerd/console v1.0.3 // indirect
	github.com/containerd/go-runc v1.1.0 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/containerd/stargz-snapshotter/estargz v0.15.1 // indirect
	github.com/containerd/typeurl/v2 v2.1.1 // indirect
	github.com/containers/buildah v1.33.2 // indirect
	github.com/containers/libtrust v0.0.0-20230121012942-c1716e8a8d01 // indirect
	github.com/containers/luksy v0.0.0-20231030195837-b5a7f79da98b // indirect
	github.com/containers/psgo v1.8.0 // indirect
	github.com/coreos/go-systemd v0.0.0-20190719114852-fd7a80b32e1f // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.3 // indirect
	github.com/cyberphone/json-canonicalization v0.0.0-20231011164504-785e29786b46 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/disiqueira/gotree/v3 v3.0.2 // indirect
	github.com/distribution/reference v0.5.0 // indirect
	github.com/docker/docker v24.0.7+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.8.0 // indirect
	github.com/docker/go-connections v0.4.1-0.20231031175723-0b8c1f4e07a0 // indirect
	github.com/docker/go-events v0.0.0-20190806004212-e31b211e4f1c // indirect
	github.com/docker/go-metrics v0.0.1 // indirect
	github.com/docker/go-plugins-helpers v0.0.0-20211224144127-6eecb7beb651 // indirect
	github.com/emicklei/go-restful/v3 v3.11.0 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/fsouza/go-dockerclient v1.10.0 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.5.0 // indirect
	github.com/go-git/go-git/v5 v5.10.0 // indirect
	github.com/go-jose/go-jose/v3 v3.0.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/analysis v0.21.4 // indirect
	github.com/go-openapi/errors v0.20.4 // indirect
	github.com/go-openapi/jsonpointer v0.20.0 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/loads v0.21.2 // indirect
	github.com/go-openapi/runtime v0.26.0 // indirect
	github.com/go-openapi/spec v0.20.9 // indirect
	github.com/go-openapi/strfmt v0.21.7 // indirect
	github.com/go-openapi/swag v0.22.4 // indirect
	github.com/go-openapi/validate v0.22.1 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/gnostic-models v0.6.9-0.20230804172637-c7be7c783f49 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/go-containerregistry v0.16.1 // indirect
	github.com/google/go-github/v56 v56.0.0 // indirect
	github.com/google/go-intervals v0.0.2 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/pprof v0.0.0-20231127191134-f3a68a39ae15 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/gorilla/mux v1.8.1 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.18.1 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/jinzhu/copier v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/kevinburke/ssh_config v1.2.0 // indirect
	github.com/klauspost/compress v1.17.3 // indirect
	github.com/klauspost/pgzip v1.2.6 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/letsencrypt/boulder v0.0.0-20231026200631-000cd05d5491 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/manifoldco/promptui v0.9.0 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/mattn/go-shellwords v1.0.12 // indirect
	github.com/mattn/go-sqlite3 v1.14.18 // indirect
	github.com/matttproud/golang_protobuf_extensions/v2 v2.0.0 // indirect
	github.com/miekg/pkcs11 v1.1.1 // indirect
	github.com/mistifyio/go-zfs/v3 v3.0.1 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/moby/buildkit v0.12.3 // indirect
	github.com/moby/patternmatcher v0.6.0 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/moby/sys/mountinfo v0.7.1 // indirect
	github.com/moby/sys/sequential v0.5.0 // indirect
	github.com/moby/term v0.5.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/nxadm/tail v1.4.11 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/openshift/imagebuilder v1.2.6-0.20231110114814-35a50d57f722 // indirect
	github.com/ostreedev/ostree-go v0.0.0-20210805093236-719684c64e4f // indirect
	github.com/pjbgf/sha1cd v0.3.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pkg/sftp v1.13.6 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/proglottis/gpgme v0.1.3 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.45.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/rivo/uniseg v0.4.4 // indirect
	github.com/russross/blackfriday v1.6.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/secure-systems-lab/go-securesystemslib v0.7.0 // indirect
	github.com/sergi/go-diff v1.3.1 // indirect
	github.com/sigstore/fulcio v1.4.3 // indirect
	github.com/sigstore/rekor v1.3.3 // indirect
	github.com/sigstore/sigstore v1.7.5 // indirect
	github.com/skeema/knownhosts v1.2.0 // indirect
	github.com/spf13/cobra v1.8.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stefanberger/go-pkcs11uri v0.0.0-20201008174630-78d3cae3a980 // indirect
	github.com/sylabs/sif/v2 v2.15.0 // indirect
	github.com/tchap/go-patricia/v2 v2.3.1 // indirect
	github.com/titanous/rocacheck v0.0.0-20171023193734-afe73141d399 // indirect
	github.com/ulikunitz/xz v0.5.11 // indirect
	github.com/uptrace/opentelemetry-go-extra/otelutil v0.2.3 // indirect
	github.com/vbatts/tar-split v0.11.5 // indirect
	github.com/vbauerster/mpb/v8 v8.6.2 // indirect
	github.com/vishvananda/netns v0.0.4 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	github.com/xrash/smetrics v0.0.0-20201216005158-039620a65673 // indirect
	go.etcd.io/bbolt v1.3.8 // indirect
	go.mongodb.org/mongo-driver v1.12.1 // indirect
	go.mozilla.org/pkcs7 v0.0.0-20210826202110-33d05740a352 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.21.0 // indirect
	go.opentelemetry.io/otel/metric v1.21.0 // indirect
	go.opentelemetry.io/proto/otlp v1.0.0 // indirect
	golang.org/x/crypto v0.16.0 // indirect
	golang.org/x/exp v0.0.0-20231006140011-7918f672742d // indirect
	golang.org/x/mod v0.14.0 // indirect
	golang.org/x/oauth2 v0.14.0 // indirect
	golang.org/x/term v0.15.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	golang.org/x/tools v0.16.0 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/genproto v0.0.0-20231127180814-3a041ad873d4 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20231127180814-3a041ad873d4 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20231127180814-3a041ad873d4 // indirect
	gopkg.in/go-jose/go-jose.v2 v2.6.1 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/apiserver v0.29.0 // indirect
	k8s.io/kube-openapi v0.0.0-20231010175941-2dd684a91f00 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
	tags.cncf.io/container-device-interface/specs-go v0.6.0 // indirect
	zenhack.net/go/util v0.0.0-20230218002511-744d2d6d1739 // indirect
)
