# CRI-O Dependency Report

_Generated on Mon, 26 Jul 2021 12:12:52 UTC for commit [a212a95][0]._

[0]: https://github.com/cri-o/cri-o/commit/a212a95459fcf2234b1371f5307c51683ba37ff2

## Outdated Dependencies

|              MODULE              |                  VERSION                  |            NEW VERSION             | DIRECT | VALID TIMESTAMPS |
|----------------------------------|-------------------------------------------|------------------------------------|--------|------------------|
| github.com/containerd/containerd | v1.5.3                                    | v1.5.4                             | true   | true             |
| github.com/containers/buildah    | v1.21.3                                   | v1.21.4                            | true   | true             |
| github.com/containers/common     | v0.38.12                                  | v1.0.0                             | true   | false            |
| github.com/containers/image/v5   | v5.13.2                                   | v5.14.0                            | true   | true             |
| github.com/containers/podman/v3  | v3.2.2                                    | v3.2.3                             | true   | true             |
| github.com/containers/storage    | v1.32.3                                   | v1.32.6                            | true   | true             |
| github.com/go-logr/logr          | v0.4.0                                    | v1.0.0                             | true   | true             |
| github.com/opencontainers/runc   | v1.0.0-rc95.0.20210521141834-a95237f81684 | v1.0.1                             | true   | true             |
| golang.org/x/net                 | v0.0.0-20210428140749-89ef3d95e781        | v0.0.0-20210716203947-853a461950ff | true   | true             |
| golang.org/x/sys                 | v0.0.0-20210603125802-9665404d3644        | v0.0.0-20210630005230-0f9fa26af87c | true   | true             |
| k8s.io/api                       | v0.0.0-20210408162405-cb303e613a12        | v0.0.0-20210726093233-ee5df7cbcfff | true   | true             |
| k8s.io/client-go                 | v0.0.0-20210408162405-cb303e613a12        | v0.0.0-20210726093233-ee5df7cbcfff | true   | true             |
| k8s.io/cri-api                   | v0.0.0-20210408162405-cb303e613a12        | v0.0.0-20210726093233-ee5df7cbcfff | true   | true             |
| k8s.io/kubernetes                | v1.21.0                                   | v1.21.3                            | true   | true             |
| k8s.io/release                   | v0.8.0                                    | v0.9.0                             | true   | true             |
| k8s.io/utils                     | v0.0.0-20210305010621-2afb4311ab10        | v0.0.0-20210722164352-7f3ee0f31471 | true   | true             |

## All Dependencies

|                                MODULE                                |                      VERSION                      |            NEW VERSION             | DIRECT | VALID TIMESTAMPS |
|----------------------------------------------------------------------|---------------------------------------------------|------------------------------------|--------|------------------|
| bazil.org/fuse                                                       | v0.0.0-20160811212531-371fbbdaa898                | v0.0.0-20200524192727-fb710f7dfd05 | false  | true             |
| bitbucket.org/bertimus9/systemstat                                   | v0.0.0-20180207000608-0eeff89b0690                |                                    | false  | true             |
| cloud.google.com/go                                                  | v0.75.0                                           | v0.88.0                            | false  | true             |
| cloud.google.com/go/bigquery                                         | v1.8.0                                            | v1.19.0                            | false  | true             |
| cloud.google.com/go/datastore                                        | v1.1.0                                            | v1.5.0                             | false  | true             |
| cloud.google.com/go/firestore                                        | v1.1.0                                            | v1.5.0                             | false  | true             |
| cloud.google.com/go/logging                                          | v1.1.2                                            | v1.4.2                             | false  | true             |
| cloud.google.com/go/pubsub                                           | v1.3.1                                            | v1.13.0                            | false  | true             |
| cloud.google.com/go/storage                                          | v1.12.0                                           | v1.16.0                            | false  | true             |
| dmitri.shuralyov.com/gpu/mtl                                         | v0.0.0-20201218220906-28db891af037                |                                    | false  | true             |
| github.com/14rcole/gopopulate                                        | v0.0.0-20180821133914-b175b219e774                |                                    | false  | true             |
| github.com/Azure/azure-sdk-for-go                                    | v43.0.0+incompatible                              | v55.8.0+incompatible               | false  | true             |
| github.com/Azure/go-ansiterm                                         | v0.0.0-20170929234023-d6e3b3328b78                | v0.0.0-20210617225240-d185dfc1b5a1 | false  | true             |
| github.com/Azure/go-autorest                                         | v14.2.0+incompatible                              |                                    | false  | true             |
| github.com/Azure/go-autorest/autorest                                | v0.11.12                                          | v0.11.19                           | false  | true             |
| github.com/Azure/go-autorest/autorest/adal                           | v0.9.5                                            | v0.9.14                            | false  | true             |
| github.com/Azure/go-autorest/autorest/date                           | v0.3.0                                            |                                    | false  | true             |
| github.com/Azure/go-autorest/autorest/mocks                          | v0.4.1                                            |                                    | false  | true             |
| github.com/Azure/go-autorest/autorest/to                             | v0.3.0                                            | v0.4.0                             | false  | true             |
| github.com/Azure/go-autorest/autorest/validation                     | v0.2.0                                            | v0.3.1                             | false  | true             |
| github.com/Azure/go-autorest/logger                                  | v0.2.0                                            | v0.2.1                             | false  | true             |
| github.com/Azure/go-autorest/tracing                                 | v0.6.0                                            |                                    | false  | true             |
| github.com/BurntSushi/toml                                           | v0.3.1                                            |                                    | true   | true             |
| github.com/BurntSushi/xgb                                            | v0.0.0-20160522181843-27f122750802                | v0.0.0-20210121224620-deaf085860bc | false  | true             |
| github.com/GoogleCloudPlatform/k8s-cloud-provider                    | v0.0.0-20200415212048-7901bc822317                | v1.17.0                            | false  | true             |
| github.com/GoogleCloudPlatform/testgrid                              | v0.0.38                                           | v0.0.82                            | false  | true             |
| github.com/JeffAshton/win_pdh                                        | v0.0.0-20161109143554-76bb4ee9f0ab                |                                    | false  | true             |
| github.com/MakeNowJust/heredoc                                       | v0.0.0-20170808103936-bb23615498cd                | v1.0.0                             | false  | true             |
| github.com/Microsoft/go-winio                                        | v0.5.0                                            |                                    | true   | true             |
| github.com/Microsoft/hcsshim                                         | v0.8.18                                           | v0.8.20                            | false  | true             |
| github.com/Microsoft/hcsshim/test                                    | v0.0.0-20210227013316-43a75bb4edd3                | v0.0.0-20210723194632-8ef3c7401da3 | false  | true             |
| github.com/NYTimes/gziphandler                                       | v1.1.1                                            |                                    | false  | true             |
| github.com/OneOfOne/xxhash                                           | v1.2.2                                            | v1.2.8                             | false  | true             |
| github.com/OpenPeeDeeP/depguard                                      | v1.0.1                                            |                                    | false  | true             |
| github.com/PuerkitoBio/purell                                        | v1.1.1                                            |                                    | false  | true             |
| github.com/PuerkitoBio/urlesc                                        | v0.0.0-20170810143723-de5bf2ad4578                |                                    | false  | true             |
| github.com/Shopify/logrus-bugsnag                                    | v0.0.0-20171204204709-577dee27f20d                |                                    | false  | true             |
| github.com/StackExchange/wmi                                         | v0.0.0-20190523213315-cbe66965904d                | v1.2.0                             | false  | true             |
| github.com/VividCortex/ewma                                          | v1.2.0                                            |                                    | false  | true             |
| github.com/acarl005/stripansi                                        | v0.0.0-20180116102854-5a71ef0e047d                |                                    | false  | true             |
| github.com/agnivade/levenshtein                                      | v1.0.1                                            | v1.1.1                             | false  | true             |
| github.com/ajstarks/svgo                                             | v0.0.0-20180226025133-644b8db467af                | v0.0.0-20210406150507-75cfd577ce75 | false  | true             |
| github.com/alcortesm/tgz                                             | v0.0.0-20161220082320-9c5fe88206d7                |                                    | false  | true             |
| github.com/alecthomas/template                                       | v0.0.0-20190718012654-fb15b899a751                |                                    | false  | true             |
| github.com/alecthomas/units                                          | v0.0.0-20190924025748-f65c72e2690d                | v0.0.0-20210208195552-ff826a37aa15 | false  | true             |
| github.com/alexflint/go-filemutex                                    | v0.0.0-20171022225611-72bdc8eae2ae                | v1.1.0                             | false  | true             |
| github.com/andreyvit/diff                                            | v0.0.0-20170406064948-c7f18ee00883                |                                    | false  | true             |
| github.com/andybalholm/brotli                                        | v1.0.0                                            | v1.0.3                             | false  | true             |
| github.com/anmitsu/go-shlex                                          | v0.0.0-20161002113705-648efa622239                | v0.0.0-20200514113438-38f4b401e2be | false  | true             |
| github.com/antihax/optional                                          | v1.0.0                                            |                                    | false  | true             |
| github.com/armon/circbuf                                             | v0.0.0-20150827004946-bbbad097214e                | v0.0.0-20190214190532-5111143e8da2 | false  | true             |
| github.com/armon/consul-api                                          | v0.0.0-20180202201655-eb2c6b5be1b6                |                                    | false  | true             |
| github.com/armon/go-metrics                                          | v0.0.0-20180917152333-f0300d1749da                | v0.3.9                             | false  | true             |
| github.com/armon/go-radix                                            | v0.0.0-20180808171621-7fddfc383310                | v1.0.0                             | false  | true             |
| github.com/armon/go-socks5                                           | v0.0.0-20160902184237-e75332964ef5                |                                    | false  | true             |
| github.com/asaskevich/govalidator                                    | v0.0.0-20190424111038-f61b66f89f4a                | v0.0.0-20210307081110-f21760c49a8d | false  | true             |
| github.com/auth0/go-jwt-middleware                                   | v0.0.0-20170425171159-5493cabe49f7                | v1.0.1                             | false  | true             |
| github.com/aws/aws-sdk-go                                            | v1.37.6                                           | v1.40.7                            | false  | true             |
| github.com/beorn7/perks                                              | v1.0.1                                            |                                    | false  | true             |
| github.com/bgentry/speakeasy                                         | v0.1.0                                            |                                    | false  | true             |
| github.com/bifurcation/mint                                          | v0.0.0-20180715133206-93c51c6ce115                | v0.0.0-20210616192047-fd18df995463 | false  | true             |
| github.com/bitly/go-simplejson                                       | v0.5.0                                            |                                    | false  | true             |
| github.com/bits-and-blooms/bitset                                    | v1.2.0                                            |                                    | false  | true             |
| github.com/bketelsen/crypt                                           | v0.0.3-0.20200106085610-5cbc8cc4026c              | v0.0.4                             | false  | true             |
| github.com/blang/semver                                              | v3.5.1+incompatible                               |                                    | true   | true             |
| github.com/bmizerany/assert                                          | v0.0.0-20160611221934-b7ed37b82869                |                                    | false  | true             |
| github.com/boltdb/bolt                                               | v1.3.1                                            |                                    | false  | true             |
| github.com/bombsimon/wsl/v3                                          | v3.0.0                                            | v3.3.0                             | false  | true             |
| github.com/bshuster-repo/logrus-logstash-hook                        | v0.4.1                                            | v1.0.2                             | false  | true             |
| github.com/buger/goterm                                              | v0.0.0-20181115115552-c206103e1f37                | v1.0.1                             | false  | true             |
| github.com/buger/jsonparser                                          | v0.0.0-20180808090653-f4dd9f5a6b44                | v1.1.1                             | false  | true             |
| github.com/bugsnag/bugsnag-go                                        | v0.0.0-20141110184014-b1d153021fcd                | v2.1.1+incompatible                | false  | true             |
| github.com/bugsnag/osext                                             | v0.0.0-20130617224835-0dd3f918b21b                |                                    | false  | true             |
| github.com/bugsnag/panicwrap                                         | v0.0.0-20151223152923-e2c28503fcd0                | v1.3.3                             | false  | true             |
| github.com/caddyserver/caddy                                         | v1.0.3                                            | v1.0.5                             | false  | true             |
| github.com/carolynvs/magex                                           | v0.6.0                                            |                                    | false  | true             |
| github.com/cenkalti/backoff                                          | v2.1.1+incompatible                               | v2.2.1+incompatible                | false  | true             |
| github.com/cenkalti/backoff/v4                                       | v4.1.0                                            | v4.1.1                             | false  | true             |
| github.com/census-instrumentation/opencensus-proto                   | v0.2.1                                            | v0.3.0                             | false  | true             |
| github.com/cespare/xxhash                                            | v1.1.0                                            |                                    | false  | true             |
| github.com/cespare/xxhash/v2                                         | v2.1.1                                            |                                    | false  | true             |
| github.com/chai2010/gettext-go                                       | v0.0.0-20160711120539-c6fed771bfd5                | v1.0.2                             | false  | true             |
| github.com/checkpoint-restore/checkpointctl                          | v0.0.0-20210301084134-a2024f5584e7                | v0.0.0-20210421054446-1ca930a3ec85 | false  | true             |
| github.com/checkpoint-restore/go-criu                                | v0.0.0-20190109184317-bdb7599cd87b                | v4.0.0+incompatible                | false  | true             |
| github.com/checkpoint-restore/go-criu/v4                             | v4.1.0                                            |                                    | false  | true             |
| github.com/checkpoint-restore/go-criu/v5                             | v5.0.0                                            | v5.1.0                             | false  | true             |
| github.com/cheekybits/genny                                          | v0.0.0-20170328200008-9127e812e1e9                | v1.0.0                             | false  | true             |
| github.com/cheggaaa/pb/v3                                            | v3.0.5                                            | v3.0.8                             | false  | true             |
| github.com/chzyer/logex                                              | v1.1.10                                           |                                    | false  | true             |
| github.com/chzyer/readline                                           | v0.0.0-20180603132655-2972be24d48e                |                                    | false  | true             |
| github.com/chzyer/test                                               | v0.0.0-20180213035817-a1ea475d72b1                | v0.0.0-20210722231415-061457976a23 | false  | true             |
| github.com/cilium/ebpf                                               | v0.5.0                                            | v0.6.2                             | false  | true             |
| github.com/client9/misspell                                          | v0.3.4                                            |                                    | false  | true             |
| github.com/clusterhq/flocker-go                                      | v0.0.0-20160920122132-2b8b7259d313                |                                    | false  | true             |
| github.com/cncf/udpa/go                                              | v0.0.0-20201120205902-5459f2c99403                | v0.0.0-20210322005330-6414d713912e | false  | true             |
| github.com/cncf/xds/go                                               | v0.0.0-20210312221358-fbca930ec8ed                | v0.0.0-20210722021550-dd25fe81a445 | false  | true             |
| github.com/cockroachdb/datadriven                                    | v0.0.0-20190809214429-80d97fb3cbaa                | v1.0.0                             | false  | true             |
| github.com/container-orchestrated-devices/container-device-interface | v0.0.0-20210325223243-f99e8b6c10b9                | v0.0.0-20210630201133-52edcb4f837a | false  | true             |
| github.com/container-storage-interface/spec                          | v1.3.0                                            | v1.5.0                             | false  | true             |
| github.com/containerd/aufs                                           | v1.0.0                                            |                                    | false  | true             |
| github.com/containerd/btrfs                                          | v1.0.0                                            |                                    | false  | true             |
| github.com/containerd/cgroups                                        | v1.0.1                                            |                                    | true   | true             |
| github.com/containerd/console                                        | v1.0.2                                            |                                    | false  | true             |
| github.com/containerd/containerd                                     | v1.5.3                                            | v1.5.4                             | true   | true             |
| github.com/containerd/continuity                                     | v0.1.0                                            |                                    | false  | true             |
| github.com/containerd/cri-containerd                                 | v1.19.0                                           |                                    | true   | true             |
| github.com/containerd/fifo                                           | v1.0.0                                            |                                    | false  | true             |
| github.com/containerd/go-cni                                         | v1.0.2                                            |                                    | false  | true             |
| github.com/containerd/go-runc                                        | v1.0.0                                            |                                    | false  | true             |
| github.com/containerd/imgcrypt                                       | v1.1.1                                            |                                    | false  | true             |
| github.com/containerd/nri                                            | v0.1.0                                            |                                    | false  | true             |
| github.com/containerd/stargz-snapshotter/estargz                     | v0.0.0-20201217071531-2b97b583765b                | v0.7.0                             | false  | true             |
| github.com/containerd/ttrpc                                          | v1.0.2                                            |                                    | true   | true             |
| github.com/containerd/typeurl                                        | v1.0.2                                            |                                    | true   | true             |
| github.com/containerd/zfs                                            | v1.0.0                                            |                                    | false  | true             |
| github.com/containernetworking/cni                                   | v0.8.1                                            |                                    | true   | true             |
| github.com/containernetworking/plugins                               | v0.9.1                                            |                                    | true   | true             |
| github.com/containers/buildah                                        | v1.21.3                                           | v1.21.4                            | true   | true             |
| github.com/containers/common                                         | v0.38.12                                          | v1.0.0                             | true   | false            |
| github.com/containers/conmon                                         | v2.0.20+incompatible                              |                                    | true   | true             |
| github.com/containers/image/v5                                       | v5.13.2                                           | v5.14.0                            | true   | true             |
| github.com/containers/libtrust                                       | v0.0.0-20190913040956-14b96171aa3b                | v0.0.0-20200511145503-9c3a6c22cd9a | false  | true             |
| github.com/containers/ocicrypt                                       | v1.1.2                                            |                                    | true   | true             |
| github.com/containers/podman/v3                                      | v3.2.2                                            | v3.2.3                             | true   | true             |
| github.com/containers/psgo                                           | v1.5.2                                            |                                    | false  | true             |
| github.com/containers/storage                                        | v1.32.3                                           | v1.32.6                            | true   | true             |
| github.com/coredns/corefile-migration                                | v1.0.11                                           | v1.0.12                            | false  | true             |
| github.com/coreos/bbolt                                              | v1.3.2                                            | v1.3.6                             | false  | true             |
| github.com/coreos/etcd                                               | v3.3.13+incompatible                              | v3.3.25+incompatible               | false  | true             |
| github.com/coreos/go-etcd                                            | v2.0.0+incompatible                               |                                    | false  | true             |
| github.com/coreos/go-iptables                                        | v0.5.0                                            | v0.6.0                             | false  | true             |
| github.com/coreos/go-oidc                                            | v2.1.0+incompatible                               | v2.2.1+incompatible                | false  | true             |
| github.com/coreos/go-semver                                          | v0.3.0                                            |                                    | false  | true             |
| github.com/coreos/go-systemd                                         | v0.0.0-20190321100706-95778dfbb74e                | v0.0.0-20191104093116-d3cd4ed1dbcf | false  | true             |
| github.com/coreos/go-systemd/v22                                     | v22.3.2                                           |                                    | true   | true             |
| github.com/coreos/pkg                                                | v0.0.0-20180928190104-399ea9e2e55f                |                                    | false  | true             |
| github.com/coreos/stream-metadata-go                                 | v0.0.0-20210225230131-70edb9eb47b3                | v0.1.1                             | false  | true             |
| github.com/cpuguy83/go-md2man                                        | v1.0.10                                           |                                    | true   | true             |
| github.com/cpuguy83/go-md2man/v2                                     | v2.0.0                                            | v2.0.1                             | false  | true             |
| github.com/creack/pty                                                | v1.1.13                                           |                                    | true   | true             |
| github.com/cri-o/ocicni                                              | v0.2.1-0.20210623033107-4ea5fb8752cf              |                                    | true   | true             |
| github.com/cyphar/filepath-securejoin                                | v0.2.3                                            |                                    | true   | true             |
| github.com/d2g/dhcp4                                                 | v0.0.0-20170904100407-a1d1b6c41b1c                |                                    | false  | true             |
| github.com/d2g/dhcp4client                                           | v1.0.0                                            |                                    | false  | true             |
| github.com/d2g/dhcp4server                                           | v0.0.0-20181031114812-7d4a0a7f59a5                |                                    | false  | true             |
| github.com/d2g/hardwareaddr                                          | v0.0.0-20190221164911-e7d9fbe030e4                |                                    | false  | true             |
| github.com/danieljoos/wincred                                        | v1.1.0                                            | v1.1.1                             | false  | true             |
| github.com/davecgh/go-spew                                           | v1.1.1                                            |                                    | false  | true             |
| github.com/daviddengcn/go-colortext                                  | v0.0.0-20160507010035-511bcaf42ccd                | v1.0.0                             | false  | true             |
| github.com/denverdino/aliyungo                                       | v0.0.0-20190125010748-a747050bb1ba                | v0.0.0-20210518071019-eb3bbb144d8a | false  | true             |
| github.com/dgrijalva/jwt-go                                          | v3.2.0+incompatible                               |                                    | false  | true             |
| github.com/dgryski/go-sip13                                          | v0.0.0-20181026042036-e10d5fee7954                | v0.0.0-20200911182023-62edffca9245 | false  | true             |
| github.com/digitalocean/go-libvirt                                   | v0.0.0-20201209184759-e2a69bcd5bd1                | v0.0.0-20210723161134-761cfeeb5968 | false  | true             |
| github.com/digitalocean/go-qemu                                      | v0.0.0-20210209191958-152a1535e49f                | v0.0.0-20210326154740-ac9e0b687001 | false  | true             |
| github.com/disiqueira/gotree/v3                                      | v3.0.2                                            |                                    | false  | true             |
| github.com/dnaeon/go-vcr                                             | v1.0.1                                            | v1.2.0                             | false  | true             |
| github.com/docker/cli                                                | v0.0.0-20191017083524-a8ff7f821017                | v20.10.7+incompatible              | false  | true             |
| github.com/docker/distribution                                       | v2.7.1+incompatible                               |                                    | true   | true             |
| github.com/docker/docker                                             | v20.10.6+incompatible                             | v20.10.7+incompatible              | false  | true             |
| github.com/docker/docker-credential-helpers                          | v0.6.4                                            |                                    | false  | true             |
| github.com/docker/go-connections                                     | v0.4.0                                            |                                    | false  | true             |
| github.com/docker/go-events                                          | v0.0.0-20190806004212-e31b211e4f1c                |                                    | false  | true             |
| github.com/docker/go-metrics                                         | v0.0.1                                            |                                    | false  | true             |
| github.com/docker/go-plugins-helpers                                 | v0.0.0-20200102110956-c9a8a2d92ccc                | v0.0.0-20210623094020-7ef169fb8b8e | false  | true             |
| github.com/docker/go-units                                           | v0.4.0                                            |                                    | true   | true             |
| github.com/docker/libnetwork                                         | v0.8.0-dev.2.0.20190625141545-5a177b73e316        |                                    | false  | true             |
| github.com/docker/libtrust                                           | v0.0.0-20160708172513-aabc10ec26b7                |                                    | false  | true             |
| github.com/docopt/docopt-go                                          | v0.0.0-20180111231733-ee0de3bc6815                |                                    | false  | true             |
| github.com/dsnet/compress                                            | v0.0.1                                            |                                    | false  | true             |
| github.com/dsnet/golib                                               | v0.0.0-20171103203638-1ea166775780                | v1.0.2                             | false  | true             |
| github.com/dustin/go-humanize                                        | v1.0.0                                            |                                    | false  | true             |
| github.com/elazarl/goproxy                                           | v0.0.0-20180725130230-947c36da3153                | v0.0.0-20210110162100-a92cc753f88e | false  | true             |
| github.com/emicklei/go-restful                                       | v2.15.0+incompatible                              |                                    | true   | true             |
| github.com/emirpasic/gods                                            | v1.12.0                                           |                                    | false  | true             |
| github.com/envoyproxy/go-control-plane                               | v0.9.9-0.20210512163311-63b5d3c536b0              | v0.9.9                             | false  | true             |
| github.com/envoyproxy/protoc-gen-validate                            | v0.1.0                                            | v0.6.1                             | false  | true             |
| github.com/euank/go-kmsg-parser                                      | v2.0.0+incompatible                               |                                    | false  | true             |
| github.com/evanphx/json-patch                                        | v4.9.0+incompatible                               | v4.11.0+incompatible               | false  | true             |
| github.com/exponent-io/jsonpath                                      | v0.0.0-20151013193312-d6023ce2651d                | v0.0.0-20210407135951-1de76d718b3f | false  | true             |
| github.com/fanliao/go-promise                                        | v0.0.0-20141029170127-1890db352a72                |                                    | false  | true             |
| github.com/fatih/camelcase                                           | v1.0.0                                            |                                    | false  | true             |
| github.com/fatih/color                                               | v1.9.0                                            | v1.12.0                            | false  | true             |
| github.com/flynn/go-shlex                                            | v0.0.0-20150515145356-3f9db97f8568                |                                    | false  | true             |
| github.com/fogleman/gg                                               | v1.2.1-0.20190220221249-0403632d5b90              | v1.3.0                             | false  | true             |
| github.com/form3tech-oss/jwt-go                                      | v3.2.2+incompatible                               | v3.2.3+incompatible                | false  | true             |
| github.com/frankban/quicktest                                        | v1.11.3                                           | v1.13.0                            | false  | true             |
| github.com/fsnotify/fsnotify                                         | v1.4.9                                            |                                    | true   | true             |
| github.com/fsouza/go-dockerclient                                    | v1.7.2                                            | v1.7.3                             | false  | true             |
| github.com/fullsailor/pkcs7                                          | v0.0.0-20190404230743-d7302db945fa                |                                    | false  | true             |
| github.com/fvbommel/sortorder                                        | v1.0.1                                            | v1.0.2                             | false  | true             |
| github.com/garyburd/redigo                                           | v0.0.0-20150301180006-535138d7bcd7                | v1.6.2                             | false  | true             |
| github.com/ghodss/yaml                                               | v1.0.0                                            |                                    | false  | true             |
| github.com/gliderlabs/ssh                                            | v0.2.2                                            | v0.3.3                             | false  | true             |
| github.com/globalsign/mgo                                            | v0.0.0-20181015135952-eeefdecb41b8                |                                    | false  | true             |
| github.com/go-acme/lego                                              | v2.5.0+incompatible                               | v2.7.2+incompatible                | false  | true             |
| github.com/go-bindata/go-bindata                                     | v3.1.1+incompatible                               | v3.1.2+incompatible                | false  | true             |
| github.com/go-critic/go-critic                                       | v0.4.1                                            | v0.5.7                             | false  | true             |
| github.com/go-errors/errors                                          | v1.0.1                                            | v1.4.0                             | false  | true             |
| github.com/go-git/gcfg                                               | v1.5.0                                            |                                    | false  | true             |
| github.com/go-git/go-billy/v5                                        | v5.0.0                                            | v5.3.1                             | false  | true             |
| github.com/go-git/go-git-fixtures/v4                                 | v4.0.2-0.20200613231340-f56387b50c12              | v4.2.2                             | false  | true             |
| github.com/go-git/go-git/v5                                          | v5.2.0                                            | v5.4.2                             | false  | true             |
| github.com/go-gl/glfw                                                | v0.0.0-20190409004039-e6da0acd62b1                | v0.0.0-20210715014612-ab6297867137 | false  | true             |
| github.com/go-gl/glfw/v3.3/glfw                                      | v0.0.0-20200222043503-6f7a984d4dc4                | v0.0.0-20210715014612-ab6297867137 | false  | true             |
| github.com/go-ini/ini                                                | v1.25.4                                           | v1.62.0                            | false  | true             |
| github.com/go-kit/kit                                                | v0.9.0                                            | v0.11.0                            | false  | true             |
| github.com/go-kit/log                                                | v0.1.0                                            |                                    | false  | true             |
| github.com/go-lintpack/lintpack                                      | v0.5.2                                            |                                    | false  | true             |
| github.com/go-logfmt/logfmt                                          | v0.5.0                                            |                                    | false  | true             |
| github.com/go-logr/logr                                              | v0.4.0                                            | v1.0.0                             | true   | true             |
| github.com/go-ole/go-ole                                             | v1.2.4                                            | v1.2.5                             | false  | true             |
| github.com/go-openapi/analysis                                       | v0.19.5                                           | v0.20.1                            | false  | true             |
| github.com/go-openapi/errors                                         | v0.19.2                                           | v0.20.0                            | false  | true             |
| github.com/go-openapi/jsonpointer                                    | v0.19.3                                           | v0.19.5                            | false  | true             |
| github.com/go-openapi/jsonreference                                  | v0.19.3                                           | v0.19.6                            | false  | true             |
| github.com/go-openapi/loads                                          | v0.19.4                                           | v0.20.2                            | false  | true             |
| github.com/go-openapi/runtime                                        | v0.19.4                                           | v0.19.29                           | false  | true             |
| github.com/go-openapi/spec                                           | v0.19.5                                           | v0.20.3                            | false  | true             |
| github.com/go-openapi/strfmt                                         | v0.19.5                                           | v0.20.1                            | false  | true             |
| github.com/go-openapi/swag                                           | v0.19.5                                           | v0.19.15                           | false  | true             |
| github.com/go-openapi/validate                                       | v0.19.8                                           | v0.20.2                            | false  | true             |
| github.com/go-ozzo/ozzo-validation                                   | v3.5.0+incompatible                               | v3.6.0+incompatible                | false  | true             |
| github.com/go-sql-driver/mysql                                       | v1.5.0                                            | v1.6.0                             | false  | true             |
| github.com/go-stack/stack                                            | v1.8.0                                            |                                    | false  | true             |
| github.com/go-task/slim-sprig                                        | v0.0.0-20210107165309-348f09dbbbc0                |                                    | false  | true             |
| github.com/go-toolsmith/astcast                                      | v1.0.0                                            |                                    | false  | true             |
| github.com/go-toolsmith/astcopy                                      | v1.0.0                                            |                                    | false  | true             |
| github.com/go-toolsmith/astequal                                     | v1.0.0                                            |                                    | false  | true             |
| github.com/go-toolsmith/astfmt                                       | v1.0.0                                            |                                    | false  | true             |
| github.com/go-toolsmith/astinfo                                      | v0.0.0-20180906194353-9809ff7efb21                | v1.0.0                             | false  | true             |
| github.com/go-toolsmith/astp                                         | v1.0.0                                            |                                    | false  | true             |
| github.com/go-toolsmith/pkgload                                      | v1.0.0                                            | v1.0.1                             | false  | true             |
| github.com/go-toolsmith/strparse                                     | v1.0.0                                            |                                    | false  | true             |
| github.com/go-toolsmith/typep                                        | v1.0.0                                            | v1.0.2                             | false  | true             |
| github.com/go-xmlfmt/xmlfmt                                          | v0.0.0-20191208150333-d5b6f63a941b                |                                    | false  | true             |
| github.com/go-zoo/bone                                               | v1.3.0                                            |                                    | true   | true             |
| github.com/gobuffalo/here                                            | v0.6.0                                            | v0.6.2                             | false  | true             |
| github.com/gobwas/glob                                               | v0.2.3                                            |                                    | false  | true             |
| github.com/godbus/dbus                                               | v0.0.0-20190422162347-ade71ed3457e                |                                    | false  | true             |
| github.com/godbus/dbus/v5                                            | v5.0.4                                            |                                    | true   | true             |
| github.com/gofrs/flock                                               | v0.8.0                                            | v0.8.1                             | false  | true             |
| github.com/gogo/googleapis                                           | v1.4.0                                            | v1.4.1                             | false  | true             |
| github.com/gogo/protobuf                                             | v1.3.2                                            |                                    | true   | true             |
| github.com/golang/freetype                                           | v0.0.0-20170609003504-e2365dfdc4a0                |                                    | false  | true             |
| github.com/golang/glog                                               | v0.0.0-20160126235308-23def4e6c14b                | v0.0.0-20210429001901-424d2337a529 | false  | true             |
| github.com/golang/groupcache                                         | v0.0.0-20200121045136-8c9f03a8e57e                | v0.0.0-20210331224755-41bb18bfe9da | false  | true             |
| github.com/golang/mock                                               | v1.6.0                                            |                                    | true   | true             |
| github.com/golang/protobuf                                           | v1.5.2                                            |                                    | false  | true             |
| github.com/golang/snappy                                             | v0.0.3                                            | v0.0.4                             | false  | true             |
| github.com/golangci/check                                            | v0.0.0-20180506172741-cfe4005ccda2                |                                    | false  | true             |
| github.com/golangci/dupl                                             | v0.0.0-20180902072040-3e9179ac440a                |                                    | false  | true             |
| github.com/golangci/errcheck                                         | v0.0.0-20181223084120-ef45e06d44b6                |                                    | false  | true             |
| github.com/golangci/go-misc                                          | v0.0.0-20180628070357-927a3d87b613                |                                    | false  | true             |
| github.com/golangci/goconst                                          | v0.0.0-20180610141641-041c5f2b40f3                |                                    | false  | true             |
| github.com/golangci/gocyclo                                          | v0.0.0-20180528134321-2becd97e67ee                | v0.0.0-20180528144436-0a533e8fa43d | false  | true             |
| github.com/golangci/gofmt                                            | v0.0.0-20190930125516-244bba706f1a                |                                    | false  | true             |
| github.com/golangci/golangci-lint                                    | v1.25.0                                           | v1.41.1                            | false  | true             |
| github.com/golangci/ineffassign                                      | v0.0.0-20190609212857-42439a7714cc                |                                    | false  | true             |
| github.com/golangci/lint-1                                           | v0.0.0-20191013205115-297bf364a8e0                |                                    | false  | true             |
| github.com/golangci/maligned                                         | v0.0.0-20180506175553-b1d89398deca                |                                    | false  | true             |
| github.com/golangci/misspell                                         | v0.0.0-20180809174111-950f5d19e770                | v0.3.5                             | false  | true             |
| github.com/golangci/prealloc                                         | v0.0.0-20180630174525-215b22d4de21                |                                    | false  | true             |
| github.com/golangci/revgrep                                          | v0.0.0-20180526074752-d9c87f5ffaf0                | v0.0.0-20210208091834-cd28932614b5 | false  | true             |
| github.com/golangci/unconvert                                        | v0.0.0-20180507085042-28b1c447d1f4                |                                    | false  | true             |
| github.com/golangplus/testing                                        | v0.0.0-20180327235837-af21d9c3145e                | v1.0.0                             | false  | true             |
| github.com/gomarkdown/markdown                                       | v0.0.0-20200824053859-8c8b3816f167                | v0.0.0-20210514010506-3b9f47219fe7 | false  | true             |
| github.com/google/btree                                              | v1.0.0                                            | v1.0.1                             | false  | true             |
| github.com/google/cadvisor                                           | v0.39.0                                           | v0.40.0                            | false  | true             |
| github.com/google/go-cmp                                             | v0.5.5                                            | v0.5.6                             | false  | true             |
| github.com/google/go-containerregistry                               | v0.3.0                                            | v0.5.1                             | false  | true             |
| github.com/google/go-github/v33                                      | v33.0.0                                           |                                    | false  | true             |
| github.com/google/go-intervals                                       | v0.0.2                                            |                                    | false  | true             |
| github.com/google/go-querystring                                     | v1.0.0                                            | v1.1.0                             | false  | true             |
| github.com/google/gofuzz                                             | v1.1.0                                            | v1.2.0                             | false  | true             |
| github.com/google/licenseclassifier/v2                               | v2.0.0-alpha.1                                    |                                    | false  | true             |
| github.com/google/martian                                            | v2.1.0+incompatible                               |                                    | false  | true             |
| github.com/google/martian/v3                                         | v3.1.0                                            | v3.2.1                             | false  | true             |
| github.com/google/pprof                                              | v0.0.0-20201218002935-b9804c9f04c2                | v0.0.0-20210720184732-4bb14d4b1be1 | false  | true             |
| github.com/google/renameio                                           | v1.0.1                                            |                                    | true   | true             |
| github.com/google/shlex                                              | v0.0.0-20191202100458-e7afc7fbc510                |                                    | false  | true             |
| github.com/google/uuid                                               | v1.3.0                                            |                                    | true   | true             |
| github.com/googleapis/gax-go/v2                                      | v2.0.5                                            |                                    | false  | true             |
| github.com/googleapis/gnostic                                        | v0.4.1                                            | v0.5.5                             | false  | true             |
| github.com/gophercloud/gophercloud                                   | v0.1.0                                            | v0.19.0                            | false  | true             |
| github.com/gopherjs/gopherjs                                         | v0.0.0-20181017120253-0766667cb4d1                | v0.0.0-20210722203344-69c5ea87048d | false  | true             |
| github.com/gorilla/handlers                                          | v0.0.0-20150720190736-60c7bfde3e33                | v1.5.1                             | false  | true             |
| github.com/gorilla/mux                                               | v1.8.0                                            |                                    | false  | true             |
| github.com/gorilla/schema                                            | v1.2.0                                            |                                    | false  | true             |
| github.com/gorilla/websocket                                         | v1.4.2                                            |                                    | false  | true             |
| github.com/gostaticanalysis/analysisutil                             | v0.0.0-20190318220348-4088753ea4d3                | v0.7.1                             | false  | true             |
| github.com/gregjones/httpcache                                       | v0.0.0-20180305231024-9cad4c3443a7                | v0.0.0-20190611155906-901d90724c79 | false  | true             |
| github.com/grpc-ecosystem/go-grpc-middleware                         | v1.3.0                                            |                                    | true   | true             |
| github.com/grpc-ecosystem/go-grpc-prometheus                         | v1.2.0                                            |                                    | false  | true             |
| github.com/grpc-ecosystem/grpc-gateway                               | v1.16.0                                           |                                    | false  | true             |
| github.com/hashicorp/consul/api                                      | v1.1.0                                            | v1.9.1                             | false  | true             |
| github.com/hashicorp/consul/sdk                                      | v0.1.1                                            | v0.8.0                             | false  | true             |
| github.com/hashicorp/errwrap                                         | v1.0.0                                            | v1.1.0                             | false  | true             |
| github.com/hashicorp/go-cleanhttp                                    | v0.5.1                                            | v0.5.2                             | false  | true             |
| github.com/hashicorp/go-hclog                                        | v0.9.2                                            | v0.16.2                            | false  | true             |
| github.com/hashicorp/go-immutable-radix                              | v1.0.0                                            | v1.3.1                             | false  | true             |
| github.com/hashicorp/go-msgpack                                      | v0.5.3                                            | v1.1.5                             | false  | true             |
| github.com/hashicorp/go-multierror                                   | v1.1.1                                            |                                    | false  | true             |
| github.com/hashicorp/go-retryablehttp                                | v0.6.4                                            | v0.7.0                             | false  | true             |
| github.com/hashicorp/go-rootcerts                                    | v1.0.0                                            | v1.0.2                             | false  | true             |
| github.com/hashicorp/go-sockaddr                                     | v1.0.0                                            | v1.0.2                             | false  | true             |
| github.com/hashicorp/go-syslog                                       | v1.0.0                                            |                                    | false  | true             |
| github.com/hashicorp/go-uuid                                         | v1.0.1                                            | v1.0.2                             | false  | true             |
| github.com/hashicorp/go.net                                          | v0.0.1                                            |                                    | false  | true             |
| github.com/hashicorp/golang-lru                                      | v0.5.3                                            | v0.5.4                             | false  | true             |
| github.com/hashicorp/hcl                                             | v1.0.0                                            |                                    | false  | true             |
| github.com/hashicorp/logutils                                        | v1.0.0                                            |                                    | false  | true             |
| github.com/hashicorp/mdns                                            | v1.0.0                                            | v1.0.4                             | false  | true             |
| github.com/hashicorp/memberlist                                      | v0.1.3                                            | v0.2.4                             | false  | true             |
| github.com/hashicorp/serf                                            | v0.8.2                                            | v0.9.5                             | false  | true             |
| github.com/heketi/heketi                                             | v10.2.0+incompatible                              | v10.3.0+incompatible               | false  | true             |
| github.com/heketi/tests                                              | v0.0.0-20151005000721-f3775cbcefd6                |                                    | false  | true             |
| github.com/hpcloud/tail                                              | v1.0.0                                            |                                    | false  | true             |
| github.com/hugelgupf/socketpair                                      | v0.0.0-20190730060125-05d35a94e714                |                                    | false  | true             |
| github.com/ianlancetaylor/demangle                                   | v0.0.0-20200824232613-28f6c0f3b639                | v0.0.0-20210724235854-665d3a6fe486 | false  | true             |
| github.com/imdario/mergo                                             | v0.3.12                                           |                                    | false  | true             |
| github.com/inconshreveable/mousetrap                                 | v1.0.0                                            |                                    | false  | true             |
| github.com/insomniacslk/dhcp                                         | v0.0.0-20210120172423-cc9239ac6294                | v0.0.0-20210621130208-1cac67f12b1e | false  | true             |
| github.com/ishidawataru/sctp                                         | v0.0.0-20210226210310-f2269e66cdee                | v0.0.0-20210707070123-9a39160e9062 | false  | true             |
| github.com/j-keck/arping                                             | v0.0.0-20160618110441-2cf9dc699c56                | v1.0.2                             | false  | true             |
| github.com/jamescun/tuntap                                           | v0.0.0-20190712092105-cb1fb277045c                |                                    | false  | true             |
| github.com/jbenet/go-context                                         | v0.0.0-20150711004518-d14ea06fba99                |                                    | false  | true             |
| github.com/jessevdk/go-flags                                         | v1.4.0                                            | v1.5.0                             | false  | true             |
| github.com/jimstudt/http-authentication                              | v0.0.0-20140401203705-3eca13d6893a                |                                    | false  | true             |
| github.com/jingyugao/rowserrcheck                                    | v0.0.0-20191204022205-72ab7603b68a                | v1.1.0                             | false  | true             |
| github.com/jinzhu/copier                                             | v0.3.0                                            | v0.3.2                             | false  | true             |
| github.com/jirfag/go-printf-func-name                                | v0.0.0-20191110105641-45db9963cdd3                | v0.0.0-20200119135958-7558a9eaa5af | false  | true             |
| github.com/jmespath/go-jmespath                                      | v0.4.0                                            |                                    | false  | true             |
| github.com/jmespath/go-jmespath/internal/testify                     | v1.5.1                                            |                                    | false  | true             |
| github.com/jmoiron/sqlx                                              | v1.2.1-0.20190826204134-d7d95172beb5              | v1.3.4                             | false  | true             |
| github.com/joefitzgerald/rainbow-reporter                            | v0.1.0                                            |                                    | false  | true             |
| github.com/jonboulle/clockwork                                       | v0.1.0                                            | v0.2.2                             | false  | true             |
| github.com/jpillora/backoff                                          | v1.0.0                                            |                                    | false  | true             |
| github.com/jsimonetti/rtnetlink                                      | v0.0.0-20201110080708-d2c240429e6c                | v0.0.0-20210721205614-4cc3c1489576 | false  | true             |
| github.com/json-iterator/go                                          | v1.1.11                                           |                                    | true   | true             |
| github.com/jstemmer/go-junit-report                                  | v0.9.1                                            |                                    | false  | true             |
| github.com/jtolds/gls                                                | v4.20.0+incompatible                              |                                    | false  | true             |
| github.com/juju/ansiterm                                             | v0.0.0-20180109212912-720a0952cc2a                | v0.0.0-20210706145210-9283cdf370b5 | false  | true             |
| github.com/julienschmidt/httprouter                                  | v1.3.0                                            |                                    | false  | true             |
| github.com/jung-kurt/gofpdf                                          | v1.0.3-0.20190309125859-24315acbbda5              | v1.16.2                            | false  | true             |
| github.com/karrick/godirwalk                                         | v1.16.1                                           |                                    | false  | true             |
| github.com/kevinburke/ssh_config                                     | v0.0.0-20190725054713-01f96b0aa0cd                | v1.1.0                             | false  | true             |
| github.com/kisielk/errcheck                                          | v1.5.0                                            | v1.6.0                             | false  | true             |
| github.com/kisielk/gotool                                            | v1.0.0                                            |                                    | false  | true             |
| github.com/klauspost/compress                                        | v1.13.1                                           |                                    | false  | true             |
| github.com/klauspost/cpuid                                           | v1.2.0                                            | v1.3.1                             | false  | true             |
| github.com/klauspost/pgzip                                           | v1.2.5                                            |                                    | false  | true             |
| github.com/konsorten/go-windows-terminal-sequences                   | v1.0.3                                            |                                    | false  | true             |
| github.com/kr/logfmt                                                 | v0.0.0-20140226030751-b84e30acd515                | v0.0.0-20210122060352-19f9bcb100e6 | false  | true             |
| github.com/kr/pretty                                                 | v0.2.1                                            |                                    | false  | true             |
| github.com/kr/pty                                                    | v1.1.8                                            |                                    | false  | true             |
| github.com/kr/text                                                   | v0.2.0                                            |                                    | false  | true             |
| github.com/kylelemons/godebug                                        | v0.0.0-20170820004349-d65d576e9348                | v1.1.0                             | false  | true             |
| github.com/lib/pq                                                    | v1.2.0                                            | v1.10.2                            | false  | true             |
| github.com/libopenstorage/openstorage                                | v1.0.0                                            | v8.0.0+incompatible                | false  | true             |
| github.com/liggitt/tabwriter                                         | v0.0.0-20181228230101-89fcab3d43de                |                                    | false  | true             |
| github.com/lithammer/dedent                                          | v1.1.0                                            |                                    | false  | true             |
| github.com/logrusorgru/aurora                                        | v0.0.0-20181002194514-a7b3b318ed4e                | v2.0.3+incompatible                | false  | true             |
| github.com/lpabon/godbc                                              | v0.1.1                                            |                                    | false  | true             |
| github.com/lucas-clemente/aes12                                      | v0.0.0-20171027163421-cd47fb39b79f                |                                    | false  | true             |
| github.com/lucas-clemente/quic-clients                               | v0.1.0                                            |                                    | false  | true             |
| github.com/lucas-clemente/quic-go                                    | v0.10.2                                           | v0.22.0                            | false  | true             |
| github.com/lucas-clemente/quic-go-certificates                       | v0.0.0-20160823095156-d2f86524cced                |                                    | false  | true             |
| github.com/lunixbochs/vtclean                                        | v0.0.0-20180621232353-2d01aacdc34a                | v1.0.0                             | false  | true             |
| github.com/magefile/mage                                             | v1.11.0                                           |                                    | false  | true             |
| github.com/magiconair/properties                                     | v1.8.1                                            | v1.8.5                             | false  | true             |
| github.com/mailru/easyjson                                           | v0.7.0                                            | v0.7.7                             | false  | true             |
| github.com/manifoldco/promptui                                       | v0.8.0                                            |                                    | false  | true             |
| github.com/maratori/testpackage                                      | v1.0.1                                            |                                    | false  | true             |
| github.com/markbates/pkger                                           | v0.17.1                                           |                                    | false  | true             |
| github.com/marstr/guid                                               | v1.1.0                                            |                                    | false  | true             |
| github.com/marten-seemann/qtls                                       | v0.2.3                                            | v0.10.0                            | false  | true             |
| github.com/matoous/godox                                             | v0.0.0-20190911065817-5d6d842e92eb                | v0.0.0-20210227103229-6504466cf951 | false  | true             |
| github.com/mattn/go-colorable                                        | v0.1.8                                            |                                    | false  | true             |
| github.com/mattn/go-isatty                                           | v0.0.12                                           | v0.0.13                            | false  | true             |
| github.com/mattn/go-runewidth                                        | v0.0.13                                           |                                    | false  | true             |
| github.com/mattn/go-shellwords                                       | v1.0.12                                           |                                    | false  | true             |
| github.com/mattn/go-sqlite3                                          | v1.9.0                                            | v1.14.8                            | false  | true             |
| github.com/mattn/goveralls                                           | v0.0.2                                            | v0.0.9                             | false  | true             |
| github.com/matttproud/golang_protobuf_extensions                     | v1.0.2-0.20181231171920-c182affec369              |                                    | false  | true             |
| github.com/maxbrunsfeld/counterfeiter/v6                             | v6.4.1                                            |                                    | false  | true             |
| github.com/mdlayher/ethernet                                         | v0.0.0-20190606142754-0394541c37b7                |                                    | false  | true             |
| github.com/mdlayher/netlink                                          | v1.1.1                                            | v1.4.1                             | false  | true             |
| github.com/mdlayher/raw                                              | v0.0.0-20191009151244-50f2db8cc065                | v0.0.0-20210412142147-51b895745faf | false  | true             |
| github.com/mholt/archiver/v3                                         | v3.5.0                                            |                                    | false  | true             |
| github.com/mholt/certmagic                                           | v0.6.2-0.20190624175158-6a42ef9fe8c2              | v0.14.1                            | false  | true             |
| github.com/miekg/dns                                                 | v1.1.35                                           | v1.1.43                            | false  | true             |
| github.com/miekg/pkcs11                                              | v1.0.3                                            |                                    | false  | true             |
| github.com/mindprince/gonvml                                         | v0.0.0-20190828220739-9ebdce4bb989                |                                    | false  | true             |
| github.com/mistifyio/go-zfs                                          | v2.1.2-0.20190413222219-f784269be439+incompatible |                                    | false  | true             |
| github.com/mitchellh/cli                                             | v1.0.0                                            | v1.1.2                             | false  | true             |
| github.com/mitchellh/go-homedir                                      | v1.1.0                                            |                                    | false  | true             |
| github.com/mitchellh/go-ps                                           | v0.0.0-20190716172923-621e5597135b                | v1.0.0                             | false  | true             |
| github.com/mitchellh/go-testing-interface                            | v1.0.0                                            | v1.14.1                            | false  | true             |
| github.com/mitchellh/go-wordwrap                                     | v1.0.0                                            | v1.0.1                             | false  | true             |
| github.com/mitchellh/gox                                             | v0.4.0                                            | v1.0.1                             | false  | true             |
| github.com/mitchellh/iochan                                          | v1.0.0                                            |                                    | false  | true             |
| github.com/mitchellh/mapstructure                                    | v1.4.1                                            |                                    | false  | true             |
| github.com/mitchellh/osext                                           | v0.0.0-20151018003038-5e2d6d41470f                |                                    | false  | true             |
| github.com/mmarkdown/mmark                                           | v2.0.40+incompatible                              |                                    | false  | true             |
| github.com/moby/ipvs                                                 | v1.0.1                                            |                                    | false  | true             |
| github.com/moby/locker                                               | v1.0.1                                            |                                    | false  | true             |
| github.com/moby/spdystream                                           | v0.2.0                                            |                                    | false  | true             |
| github.com/moby/sys/mount                                            | v0.2.0                                            |                                    | false  | true             |
| github.com/moby/sys/mountinfo                                        | v0.4.1                                            |                                    | false  | true             |
| github.com/moby/sys/symlink                                          | v0.1.0                                            |                                    | false  | true             |
| github.com/moby/term                                                 | v0.0.0-20201216013528-df9cb8a40635                | v0.0.0-20210619224110-3f7ff695adc6 | false  | true             |
| github.com/moby/vpnkit                                               | v0.5.0                                            |                                    | false  | true             |
| github.com/modern-go/concurrent                                      | v0.0.0-20180306012644-bacd9c7ef1dd                |                                    | false  | true             |
| github.com/modern-go/reflect2                                        | v1.0.1                                            |                                    | false  | true             |
| github.com/mohae/deepcopy                                            | v0.0.0-20170603005431-491d3605edfb                | v0.0.0-20170929034955-c48cc78d4826 | false  | true             |
| github.com/monochromegane/go-gitignore                               | v0.0.0-20200626010858-205db1a8cc00                |                                    | false  | true             |
| github.com/morikuni/aec                                              | v1.0.0                                            |                                    | false  | true             |
| github.com/mozilla/tls-observatory                                   | v0.0.0-20190404164649-a3c1b6cfecfd                | v0.0.0-20210609171429-7bc42856d2e5 | false  | true             |
| github.com/mrunalp/fileutils                                         | v0.5.0                                            |                                    | false  | true             |
| github.com/mtrmac/gpgme                                              | v0.1.2                                            |                                    | false  | true             |
| github.com/munnerz/goautoneg                                         | v0.0.0-20191010083416-a7dc8b61c822                |                                    | false  | true             |
| github.com/mvdan/xurls                                               | v1.1.0                                            |                                    | false  | true             |
| github.com/mwitkow/go-conntrack                                      | v0.0.0-20190716064945-2f068394615f                |                                    | false  | true             |
| github.com/mxk/go-flowrate                                           | v0.0.0-20140419014527-cca7078d478f                |                                    | false  | true             |
| github.com/nakabonne/nestif                                          | v0.3.0                                            |                                    | false  | true             |
| github.com/naoina/go-stringutil                                      | v0.1.0                                            |                                    | false  | true             |
| github.com/naoina/toml                                               | v0.1.1                                            |                                    | false  | true             |
| github.com/nbutton23/zxcvbn-go                                       | v0.0.0-20180912185939-ae427f1e4c1d                | v0.0.0-20210217022336-fa2cb2858354 | false  | true             |
| github.com/ncw/swift                                                 | v1.0.47                                           | v1.0.53                            | false  | true             |
| github.com/niemeyer/pretty                                           | v0.0.0-20200227124842-a10e7caefd8e                |                                    | false  | true             |
| github.com/nozzle/throttler                                          | v0.0.0-20180817012639-2ea982251481                |                                    | false  | true             |
| github.com/nwaples/rardecode                                         | v1.1.0                                            | v1.1.1                             | false  | true             |
| github.com/nxadm/tail                                                | v1.4.8                                            |                                    | false  | true             |
| github.com/oklog/ulid                                                | v1.3.1                                            |                                    | false  | true             |
| github.com/olekukonko/tablewriter                                    | v0.0.5                                            |                                    | false  | true             |
| github.com/onsi/ginkgo                                               | v1.16.4                                           |                                    | true   | true             |
| github.com/onsi/gomega                                               | v1.14.0                                           |                                    | true   | true             |
| github.com/opencontainers/go-digest                                  | v1.0.0                                            |                                    | true   | true             |
| github.com/opencontainers/image-spec                                 | v1.0.2-0.20210708142037-083f635f2b04              |                                    | true   | true             |
| github.com/opencontainers/runc                                       | v1.0.0-rc95.0.20210521141834-a95237f81684         | v1.0.1                             | true   | true             |
| github.com/opencontainers/runtime-spec                               | v1.0.3-0.20210709190330-896175883324              |                                    | true   | true             |
| github.com/opencontainers/runtime-tools                              | v0.9.1-0.20210326182921-59cdde06764b              |                                    | true   | true             |
| github.com/opencontainers/selinux                                    | v1.8.2                                            |                                    | true   | true             |
| github.com/openshift/imagebuilder                                    | v1.2.2-0.20210415181909-87f3e48c2656              |                                    | false  | true             |
| github.com/opentracing/opentracing-go                                | v1.1.0                                            | v1.2.0                             | false  | true             |
| github.com/ostreedev/ostree-go                                       | v0.0.0-20190702140239-759a8c1ac913                | v0.0.0-20210511152353-2ca91aaf921c | false  | true             |
| github.com/pascaldekloe/goe                                          | v0.0.0-20180627143212-57f6aae5913c                | v0.1.0                             | false  | true             |
| github.com/pborman/uuid                                              | v1.2.0                                            | v1.2.1                             | false  | true             |
| github.com/pelletier/go-buffruneio                                   | v0.2.0                                            | v0.3.0                             | false  | true             |
| github.com/pelletier/go-toml                                         | v1.8.1                                            | v1.9.3                             | false  | true             |
| github.com/peterbourgon/diskv                                        | v2.0.1+incompatible                               |                                    | false  | true             |
| github.com/phayes/checkstyle                                         | v0.0.0-20170904204023-bfd46e6a821d                |                                    | false  | true             |
| github.com/pierrec/lz4/v4                                            | v4.0.3                                            | v4.1.8                             | false  | true             |
| github.com/pkg/diff                                                  | v0.0.0-20210226163009-20ebb0f2a09e                |                                    | false  | true             |
| github.com/pkg/errors                                                | v0.9.1                                            |                                    | true   | true             |
| github.com/pmezard/go-difflib                                        | v1.0.0                                            |                                    | false  | true             |
| github.com/posener/complete                                          | v1.1.1                                            | v1.2.3                             | false  | true             |
| github.com/pquerna/cachecontrol                                      | v0.0.0-20171018203845-0dec1b30a021                | v0.1.0                             | false  | true             |
| github.com/pquerna/ffjson                                            | v0.0.0-20190813045741-dac163c6c0a9                | v0.0.0-20190930134022-aa0246cd15f7 | false  | true             |
| github.com/prometheus/client_golang                                  | v1.11.0                                           |                                    | true   | true             |
| github.com/prometheus/client_model                                   | v0.2.0                                            |                                    | false  | true             |
| github.com/prometheus/common                                         | v0.26.0                                           | v0.29.0                            | false  | true             |
| github.com/prometheus/procfs                                         | v0.6.0                                            | v0.7.1                             | false  | true             |
| github.com/prometheus/tsdb                                           | v0.7.1                                            | v0.10.0                            | false  | true             |
| github.com/psampaz/go-mod-outdated                                   | v0.8.0                                            |                                    | true   | true             |
| github.com/quasilyte/go-consistent                                   | v0.0.0-20190521200055-c6f3937de18c                | v0.0.0-20200404105227-766526bf1e96 | false  | true             |
| github.com/quobyte/api                                               | v0.1.8                                            | v1.0.0                             | false  | true             |
| github.com/remyoudompheng/bigfft                                     | v0.0.0-20170806203942-52369c62f446                | v0.0.0-20200410134404-eec4a21b6bb0 | false  | true             |
| github.com/rivo/uniseg                                               | v0.2.0                                            |                                    | false  | true             |
| github.com/rjeczalik/notify                                          | v0.9.2                                            |                                    | true   | true             |
| github.com/robfig/cron                                               | v1.1.0                                            | v1.2.0                             | false  | true             |
| github.com/rogpeppe/fastuuid                                         | v1.2.0                                            |                                    | false  | true             |
| github.com/rogpeppe/go-internal                                      | v1.7.0                                            | v1.8.0                             | false  | true             |
| github.com/rootless-containers/rootlesskit                           | v0.14.2                                           |                                    | false  | true             |
| github.com/rubiojr/go-vhd                                            | v0.0.0-20200706105327-02e210299021                | v0.0.0-20200706122120-ccecf6c0760f | false  | true             |
| github.com/russross/blackfriday                                      | v1.5.2                                            | v1.6.0                             | false  | true             |
| github.com/russross/blackfriday/v2                                   | v2.1.0                                            |                                    | false  | true             |
| github.com/ryancurrah/gomodguard                                     | v1.0.2                                            | v1.2.2                             | false  | true             |
| github.com/ryanuber/columnize                                        | v0.0.0-20160712163229-9b3edd62028f                | v2.1.2+incompatible                | false  | true             |
| github.com/safchain/ethtool                                          | v0.0.0-20190326074333-42ed695e3de8                | v0.0.0-20201023143004-874930cb3ce0 | false  | true             |
| github.com/saschagrunert/ccli                                        | v1.0.2-0.20200423111659-b68f755cc0f5              |                                    | false  | true             |
| github.com/saschagrunert/go-modiff                                   | v1.3.0                                            |                                    | false  | true             |
| github.com/satori/go.uuid                                            | v1.2.0                                            |                                    | false  | true             |
| github.com/sclevine/spec                                             | v1.4.0                                            |                                    | false  | true             |
| github.com/sean-/seed                                                | v0.0.0-20170313163322-e2103e2c3529                |                                    | false  | true             |
| github.com/seccomp/libseccomp-golang                                 | v0.9.2-0.20200616122406-847368b35ebf              |                                    | false  | true             |
| github.com/securego/gosec                                            | v0.0.0-20200103095621-79fbf3af8d83                | v0.0.0-20200401082031-e946c8c39989 | false  | true             |
| github.com/sendgrid/rest                                             | v2.6.2+incompatible                               | v2.6.4+incompatible                | false  | true             |
| github.com/sendgrid/sendgrid-go                                      | v3.7.2+incompatible                               | v3.10.0+incompatible               | false  | true             |
| github.com/sergi/go-diff                                             | v1.1.0                                            | v1.2.0                             | false  | true             |
| github.com/shirou/gopsutil                                           | v0.0.0-20190901111213-e4ec7b275ada                | v3.21.6+incompatible               | false  | true             |
| github.com/shirou/gopsutil/v3                                        | v3.20.12                                          | v3.21.6                            | false  | true             |
| github.com/shirou/w32                                                | v0.0.0-20160930032740-bb4de0191aa4                |                                    | false  | true             |
| github.com/shurcooL/go                                               | v0.0.0-20180423040247-9e1955d9fb6e                | v0.0.0-20200502201357-93f07166e636 | false  | true             |
| github.com/shurcooL/go-goon                                          | v0.0.0-20170922171312-37c2f522c041                | v0.0.0-20210110234559-7585751d9a17 | false  | true             |
| github.com/shurcooL/sanitized_anchor_name                            | v1.0.0                                            |                                    | false  | true             |
| github.com/sirupsen/logrus                                           | v1.8.1                                            |                                    | true   | true             |
| github.com/smartystreets/assertions                                  | v0.0.0-20180927180507-b2de0cb4f26d                | v1.2.0                             | false  | true             |
| github.com/smartystreets/goconvey                                    | v1.6.4                                            |                                    | false  | true             |
| github.com/soheilhy/cmux                                             | v0.1.5                                            |                                    | true   | true             |
| github.com/sourcegraph/go-diff                                       | v0.5.1                                            | v0.6.1                             | false  | true             |
| github.com/spaolacci/murmur3                                         | v0.0.0-20180118202830-f09979ecbc72                | v1.1.0                             | false  | true             |
| github.com/spf13/afero                                               | v1.2.2                                            | v1.6.0                             | false  | true             |
| github.com/spf13/cast                                                | v1.3.0                                            | v1.3.1                             | false  | true             |
| github.com/spf13/cobra                                               | v1.1.3                                            | v1.2.1                             | false  | true             |
| github.com/spf13/jwalterweatherman                                   | v1.1.0                                            |                                    | false  | true             |
| github.com/spf13/pflag                                               | v1.0.5                                            |                                    | false  | true             |
| github.com/spf13/viper                                               | v1.7.0                                            | v1.8.1                             | false  | true             |
| github.com/spiegel-im-spiegel/errs                                   | v1.0.2                                            | v1.0.4                             | false  | true             |
| github.com/spiegel-im-spiegel/go-cvss                                | v0.4.0                                            |                                    | false  | true             |
| github.com/src-d/gcfg                                                | v1.4.0                                            |                                    | false  | true             |
| github.com/stefanberger/go-pkcs11uri                                 | v0.0.0-20201008174630-78d3cae3a980                |                                    | false  | true             |
| github.com/storageos/go-api                                          | v2.2.0+incompatible                               | v2.4.0+incompatible                | false  | true             |
| github.com/stretchr/objx                                             | v0.2.0                                            | v0.3.0                             | false  | true             |
| github.com/stretchr/testify                                          | v1.7.0                                            |                                    | true   | true             |
| github.com/subosito/gotenv                                           | v1.2.0                                            |                                    | false  | true             |
| github.com/syndtr/gocapability                                       | v0.0.0-20180916011248-d98352740cb2                | v0.0.0-20200815063812-42c35b437635 | true   | true             |
| github.com/tchap/go-patricia                                         | v2.3.0+incompatible                               |                                    | false  | true             |
| github.com/tetafro/godot                                             | v0.2.5                                            | v1.4.8                             | false  | true             |
| github.com/thecodeteam/goscaleio                                     | v0.1.0                                            |                                    | false  | true             |
| github.com/tidwall/pretty                                            | v1.0.0                                            | v1.2.0                             | false  | true             |
| github.com/timakin/bodyclose                                         | v0.0.0-20190930140734-f7f2e9bca95e                | v0.0.0-20210704033933-f49887972144 | false  | true             |
| github.com/tmc/grpc-websocket-proxy                                  | v0.0.0-20190109142713-0ad062ec5ee5                | v0.0.0-20201229170055-e5319fda7802 | false  | true             |
| github.com/tommy-muehle/go-mnd                                       | v1.3.1-0.20200224220436-e6f9a994e8fa              |                                    | false  | true             |
| github.com/u-root/u-root                                             | v7.0.0+incompatible                               |                                    | false  | true             |
| github.com/uber/jaeger-client-go                                     | v2.28.0+incompatible                              | v2.29.1+incompatible               | false  | true             |
| github.com/ugorji/go                                                 | v1.1.4                                            | v1.2.6                             | false  | true             |
| github.com/ugorji/go/codec                                           | v0.0.0-20181204163529-d75b2dcb6bc8                | v1.2.6                             | false  | true             |
| github.com/ulikunitz/xz                                              | v0.5.10                                           |                                    | false  | true             |
| github.com/ultraware/funlen                                          | v0.0.2                                            | v0.0.3                             | false  | true             |
| github.com/ultraware/whitespace                                      | v0.0.4                                            |                                    | false  | true             |
| github.com/urfave/cli                                                | v1.22.2                                           | v1.22.5                            | false  | true             |
| github.com/urfave/cli/v2                                             | v2.3.0                                            |                                    | true   | true             |
| github.com/urfave/negroni                                            | v1.0.0                                            |                                    | false  | true             |
| github.com/uudashr/gocognit                                          | v1.0.1                                            | v1.0.5                             | false  | true             |
| github.com/valyala/bytebufferpool                                    | v1.0.0                                            |                                    | false  | true             |
| github.com/valyala/fasthttp                                          | v1.2.0                                            | v1.28.0                            | false  | true             |
| github.com/valyala/quicktemplate                                     | v1.2.0                                            | v1.6.3                             | false  | true             |
| github.com/valyala/tcplisten                                         | v0.0.0-20161114210144-ceec8f93295a                | v1.0.0                             | false  | true             |
| github.com/vbatts/tar-split                                          | v0.11.1                                           |                                    | false  | true             |
| github.com/vbauerster/mpb/v5                                         | v5.4.0                                            |                                    | false  | true             |
| github.com/vbauerster/mpb/v6                                         | v6.0.3                                            | v6.0.4                             | false  | true             |
| github.com/vbauerster/mpb/v7                                         | v7.0.2                                            | v7.0.3                             | false  | true             |
| github.com/vdemeester/k8s-pkg-credentialprovider                     | v1.18.1-0.20201019120933-f1d16962a4db             | v1.21.0                            | false  | true             |
| github.com/vektah/gqlparser                                          | v1.1.2                                            | v1.3.1                             | false  | true             |
| github.com/vishvananda/netlink                                       | v1.1.1-0.20201029203352-d40f9887b852              |                                    | true   | true             |
| github.com/vishvananda/netns                                         | v0.0.0-20200728191858-db3c7e526aae                | v0.0.0-20210104183010-2eb08e3e575f | false  | true             |
| github.com/vmware/govmomi                                            | v0.20.3                                           | v0.26.0                            | false  | true             |
| github.com/willf/bitset                                              | v1.1.11                                           | v1.2.0                             | false  | true             |
| github.com/xanzy/go-gitlab                                           | v0.43.0                                           | v0.50.1                            | false  | true             |
| github.com/xanzy/ssh-agent                                           | v0.2.1                                            | v0.3.0                             | false  | true             |
| github.com/xeipuuv/gojsonpointer                                     | v0.0.0-20190809123943-df4f5c81cb3b                | v0.0.0-20190905194746-02993c407bfb | false  | true             |
| github.com/xeipuuv/gojsonreference                                   | v0.0.0-20180127040603-bd5ef7bd5415                |                                    | false  | true             |
| github.com/xeipuuv/gojsonschema                                      | v1.2.0                                            |                                    | false  | true             |
| github.com/xi2/xz                                                    | v0.0.0-20171230120015-48954b6210f8                |                                    | false  | true             |
| github.com/xiang90/probing                                           | v0.0.0-20190116061207-43a291ad63a2                |                                    | false  | true             |
| github.com/xlab/treeprint                                            | v0.0.0-20181112141820-a009c3971eca                | v1.1.0                             | false  | true             |
| github.com/xordataexchange/crypt                                     | v0.0.3-0.20170626215501-b2862e3d0a77              |                                    | false  | true             |
| github.com/yuin/goldmark                                             | v1.3.5                                            | v1.4.0                             | false  | true             |
| github.com/yvasiyarov/go-metrics                                     | v0.0.0-20140926110328-57bccd1ccd43                | v0.0.0-20150112132944-c25f46c4b940 | false  | true             |
| github.com/yvasiyarov/gorelic                                        | v0.0.0-20141212073537-a9bba5b9ab50                | v0.0.7                             | false  | true             |
| github.com/yvasiyarov/newrelic_platform_go                           | v0.0.0-20140908184405-b21fdbd4370f                | v0.0.0-20160601141957-9c099fbc30e9 | false  | true             |
| go.etcd.io/bbolt                                                     | v1.3.6                                            |                                    | false  | true             |
| go.etcd.io/etcd                                                      | v0.5.0-alpha.5.0.20200910180754-dd1b699fc489      |                                    | false  | true             |
| go.mongodb.org/mongo-driver                                          | v1.1.2                                            | v1.7.0                             | false  | true             |
| go.mozilla.org/pkcs7                                                 | v0.0.0-20200128120323-432b2356ecb1                | v0.0.0-20210722211354-3a1db52591c7 | false  | true             |
| go.opencensus.io                                                     | v0.22.5                                           | v0.23.0                            | false  | true             |
| go.opentelemetry.io/proto/otlp                                       | v0.7.0                                            | v0.9.0                             | false  | true             |
| go.starlark.net                                                      | v0.0.0-20200306205701-8dd3e2ee1dd5                | v0.0.0-20210602144842-1cdb82c9e17a | false  | true             |
| go.uber.org/atomic                                                   | v1.4.0                                            | v1.9.0                             | false  | true             |
| go.uber.org/multierr                                                 | v1.1.0                                            | v1.7.0                             | false  | true             |
| go.uber.org/zap                                                      | v1.10.0                                           | v1.18.1                            | false  | true             |
| golang.org/dl                                                        | v0.0.0-20190829154251-82a15e2f2ead                | v0.0.0-20210713194856-38ddc79c2163 | false  | true             |
| golang.org/x/crypto                                                  | v0.0.0-20210322153248-0c34fe9e7dc2                | v0.0.0-20210711020723-a769d52b0f97 | false  | true             |
| golang.org/x/exp                                                     | v0.0.0-20210220032938-85be41e4509f                | v0.0.0-20210722180016-6781d3edade3 | false  | true             |
| golang.org/x/image                                                   | v0.0.0-20190802002840-cff245a6509b                | v0.0.0-20210628002857-a66eb6448b8d | false  | true             |
| golang.org/x/lint                                                    | v0.0.0-20201208152925-83fdc39ff7b5                | v0.0.0-20210508222113-6edffad5e616 | false  | true             |
| golang.org/x/mobile                                                  | v0.0.0-20201217150744-e6ae53a27f4f                | v0.0.0-20210716004757-34ab1303b554 | false  | true             |
| golang.org/x/mod                                                     | v0.4.2                                            |                                    | false  | true             |
| golang.org/x/net                                                     | v0.0.0-20210428140749-89ef3d95e781                | v0.0.0-20210716203947-853a461950ff | true   | true             |
| golang.org/x/oauth2                                                  | v0.0.0-20210112200429-01de73cf58bd                | v0.0.0-20210628180205-a41e5a781914 | false  | true             |
| golang.org/x/sync                                                    | v0.0.0-20210220032951-036812b2e83c                |                                    | true   | true             |
| golang.org/x/sys                                                     | v0.0.0-20210603125802-9665404d3644                | v0.0.0-20210630005230-0f9fa26af87c | true   | true             |
| golang.org/x/term                                                    | v0.0.0-20210503060354-a79de5458b56                | v0.0.0-20210615171337-6886f2dfbf5b | false  | true             |
| golang.org/x/text                                                    | v0.3.6                                            |                                    | false  | true             |
| golang.org/x/time                                                    | v0.0.0-20210220033141-f8bda1e9f3ba                | v0.0.0-20210723032227-1f47c861a9ac | false  | true             |
| golang.org/x/tools                                                   | v0.1.1                                            | v0.1.5                             | false  | true             |
| golang.org/x/xerrors                                                 | v0.0.0-20200804184101-5ec99f83aff1                |                                    | false  | true             |
| gonum.org/v1/gonum                                                   | v0.6.2                                            | v0.9.3                             | false  | true             |
| gonum.org/v1/netlib                                                  | v0.0.0-20190331212654-76723241ea4e                | v0.0.0-20210302091547-ede94419cf37 | false  | true             |
| gonum.org/v1/plot                                                    | v0.0.0-20190515093506-e2840ee46a6b                | v0.9.0                             | false  | true             |
| google.golang.org/api                                                | v0.36.0                                           | v0.51.0                            | false  | true             |
| google.golang.org/appengine                                          | v1.6.7                                            |                                    | false  | true             |
| google.golang.org/cloud                                              | v0.0.0-20151119220103-975617b05ea8                | v0.88.0                            | false  | true             |
| google.golang.org/genproto                                           | v0.0.0-20200117163144-32f20d992d24                | v0.0.0-20210722135532-667f2b7c528f | false  | true             |
| google.golang.org/grpc                                               | v1.39.0                                           |                                    | true   | true             |
| google.golang.org/protobuf                                           | v1.26.0                                           | v1.27.1                            | false  | true             |
| gopkg.in/airbrake/gobrake.v2                                         | v2.0.9                                            |                                    | false  | true             |
| gopkg.in/alecthomas/kingpin.v2                                       | v2.2.6                                            |                                    | false  | true             |
| gopkg.in/check.v1                                                    | v1.0.0-20200902074654-038fdea0a05b                | v1.0.0-20201130134442-10cb98267c6c | false  | true             |
| gopkg.in/cheggaaa/pb.v1                                              | v1.0.25                                           | v1.0.28                            | false  | true             |
| gopkg.in/errgo.v2                                                    | v2.1.0                                            |                                    | false  | true             |
| gopkg.in/fsnotify.v1                                                 | v1.4.7                                            |                                    | false  | true             |
| gopkg.in/gcfg.v1                                                     | v1.2.0                                            | v1.2.3                             | false  | true             |
| gopkg.in/gemnasium/logrus-airbrake-hook.v2                           | v2.1.2                                            |                                    | false  | true             |
| gopkg.in/inf.v0                                                      | v0.9.1                                            |                                    | false  | true             |
| gopkg.in/ini.v1                                                      | v1.51.0                                           | v1.62.0                            | false  | true             |
| gopkg.in/mcuadros/go-syslog.v2                                       | v2.2.1                                            | v2.3.0                             | false  | true             |
| gopkg.in/natefinch/lumberjack.v2                                     | v2.0.0                                            |                                    | false  | true             |
| gopkg.in/resty.v1                                                    | v1.12.0                                           |                                    | false  | true             |
| gopkg.in/square/go-jose.v2                                           | v2.5.1                                            | v2.6.0                             | false  | true             |
| gopkg.in/src-d/go-billy.v4                                           | v4.3.2                                            |                                    | false  | true             |
| gopkg.in/src-d/go-git-fixtures.v3                                    | v3.5.0                                            |                                    | false  | true             |
| gopkg.in/src-d/go-git.v4                                             | v4.13.1                                           |                                    | false  | true             |
| gopkg.in/tomb.v1                                                     | v1.0.0-20141024135613-dd632973f1e7                |                                    | false  | true             |
| gopkg.in/warnings.v0                                                 | v0.1.2                                            |                                    | false  | true             |
| gopkg.in/yaml.v2                                                     | v2.4.0                                            |                                    | false  | true             |
| gopkg.in/yaml.v3                                                     | v3.0.0-20210107192922-496545a6307b                |                                    | false  | true             |
| gotest.tools                                                         | v2.2.0+incompatible                               |                                    | false  | true             |
| gotest.tools/v3                                                      | v3.0.3                                            |                                    | false  | true             |
| honnef.co/go/tools                                                   | v0.0.1-2020.1.4                                   | v0.2.0                             | false  | true             |
| k8s.io/api                                                           | v0.0.0-20210408162405-cb303e613a12                | v0.0.0-20210726093233-ee5df7cbcfff | true   | true             |
| k8s.io/apiextensions-apiserver                                       | v0.0.0-20210408162405-cb303e613a12                | v0.0.0-20210726093233-ee5df7cbcfff | false  | true             |
| k8s.io/apimachinery                                                  | v0.0.0-20210408162405-cb303e613a12                | v0.0.0-20210726093233-ee5df7cbcfff | true   | true             |
| k8s.io/apiserver                                                     | v0.0.0-20210408162405-cb303e613a12                | v0.0.0-20210726093233-ee5df7cbcfff | false  | true             |
| k8s.io/cli-runtime                                                   | v0.0.0-20210408162405-cb303e613a12                | v0.0.0-20210726093233-ee5df7cbcfff | false  | true             |
| k8s.io/client-go                                                     | v0.0.0-20210408162405-cb303e613a12                | v0.0.0-20210726093233-ee5df7cbcfff | true   | true             |
| k8s.io/cloud-provider                                                | v0.0.0-20210408162405-cb303e613a12                | v0.0.0-20210726093233-ee5df7cbcfff | false  | true             |
| k8s.io/cluster-bootstrap                                             | v0.0.0-20210408162405-cb303e613a12                | v0.0.0-20210726093233-ee5df7cbcfff | false  | true             |
| k8s.io/code-generator                                                | v0.0.0-20210408162405-cb303e613a12                | v0.0.0-20210726093233-ee5df7cbcfff | false  | true             |
| k8s.io/component-base                                                | v0.0.0-20210408162405-cb303e613a12                | v0.0.0-20210726093233-ee5df7cbcfff | false  | true             |
| k8s.io/component-helpers                                             | v0.0.0-20210408162405-cb303e613a12                | v0.0.0-20210726093233-ee5df7cbcfff | false  | true             |
| k8s.io/controller-manager                                            | v0.0.0-20210408162405-cb303e613a12                | v0.0.0-20210726093233-ee5df7cbcfff | false  | true             |
| k8s.io/cri-api                                                       | v0.0.0-20210408162405-cb303e613a12                | v0.0.0-20210726093233-ee5df7cbcfff | true   | true             |
| k8s.io/csi-translation-lib                                           | v0.0.0-20210408162405-cb303e613a12                | v0.0.0-20210726093233-ee5df7cbcfff | false  | true             |
| k8s.io/gengo                                                         | v0.0.0-20201214224949-b6c5ce23f027                | v0.0.0-20210707163844-888f71863374 | false  | true             |
| k8s.io/heapster                                                      | v1.2.0-beta.1                                     | v1.5.4                             | false  | true             |
| k8s.io/klog                                                          | v1.0.0                                            |                                    | false  | true             |
| k8s.io/klog/v2                                                       | v2.10.0                                           |                                    | true   | true             |
| k8s.io/kube-aggregator                                               | v0.0.0-20210408162405-cb303e613a12                | v0.0.0-20210726093233-ee5df7cbcfff | false  | true             |
| k8s.io/kube-controller-manager                                       | v0.0.0-20210408162405-cb303e613a12                | v0.0.0-20210726093233-ee5df7cbcfff | false  | true             |
| k8s.io/kube-openapi                                                  | v0.0.0-20210305001622-591a79e4bda7                | v0.0.0-20210527164424-3c818078ee3d | false  | true             |
| k8s.io/kube-proxy                                                    | v0.0.0-20210408162405-cb303e613a12                | v0.0.0-20210726093233-ee5df7cbcfff | false  | true             |
| k8s.io/kube-scheduler                                                | v0.0.0-20210408162405-cb303e613a12                | v0.0.0-20210726093233-ee5df7cbcfff | false  | true             |
| k8s.io/kubectl                                                       | v0.0.0-20210408162405-cb303e613a12                | v0.0.0-20210726093233-ee5df7cbcfff | false  | true             |
| k8s.io/kubelet                                                       | v0.0.0-20210408162405-cb303e613a12                | v0.0.0-20210726093233-ee5df7cbcfff | false  | true             |
| k8s.io/kubernetes                                                    | v1.21.0                                           | v1.21.3                            | true   | true             |
| k8s.io/legacy-cloud-providers                                        | v0.0.0-20210408162405-cb303e613a12                | v0.0.0-20210726093233-ee5df7cbcfff | false  | true             |
| k8s.io/metrics                                                       | v0.0.0-20210408162405-cb303e613a12                | v0.0.0-20210726093233-ee5df7cbcfff | false  | true             |
| k8s.io/mount-utils                                                   | v0.0.0-20210408162405-cb303e613a12                | v0.0.0-20210726093233-ee5df7cbcfff | false  | true             |
| k8s.io/release                                                       | v0.8.0                                            | v0.9.0                             | true   | true             |
| k8s.io/sample-apiserver                                              | v0.0.0-20210408162405-cb303e613a12                | v0.0.0-20210726093233-ee5df7cbcfff | false  | true             |
| k8s.io/system-validators                                             | v1.4.0                                            | v1.5.0                             | false  | true             |
| k8s.io/utils                                                         | v0.0.0-20210305010621-2afb4311ab10                | v0.0.0-20210722164352-7f3ee0f31471 | true   | true             |
| modernc.org/cc                                                       | v1.0.0                                            | v1.0.1                             | false  | true             |
| modernc.org/golex                                                    | v1.0.0                                            | v1.0.1                             | false  | true             |
| modernc.org/mathutil                                                 | v1.0.0                                            | v1.4.1                             | false  | true             |
| modernc.org/strutil                                                  | v1.0.0                                            | v1.1.1                             | false  | true             |
| modernc.org/xc                                                       | v1.0.0                                            |                                    | false  | true             |
| mvdan.cc/editorconfig                                                | v0.2.0                                            |                                    | false  | true             |
| mvdan.cc/interfacer                                                  | v0.0.0-20180901003855-c20040233aed                |                                    | false  | true             |
| mvdan.cc/lint                                                        | v0.0.0-20170908181259-adc824a0674b                |                                    | false  | true             |
| mvdan.cc/sh/v3                                                       | v3.3.0                                            |                                    | true   | true             |
| mvdan.cc/unparam                                                     | v0.0.0-20190720180237-d51796306d8f                | v0.0.0-20210701114405-894c3c7ee6a6 | false  | true             |
| rsc.io/binaryregexp                                                  | v0.2.0                                            |                                    | false  | true             |
| rsc.io/pdf                                                           | v0.1.1                                            |                                    | false  | true             |
| rsc.io/quote/v3                                                      | v3.1.0                                            |                                    | false  | true             |
| rsc.io/sampler                                                       | v1.3.0                                            | v1.99.99                           | false  | true             |
| sigs.k8s.io/apiserver-network-proxy/konnectivity-client              | v0.0.15                                           | v0.0.22                            | false  | true             |
| sigs.k8s.io/kustomize/api                                            | v0.8.5                                            | v0.8.11                            | false  | true             |
| sigs.k8s.io/kustomize/cmd/config                                     | v0.9.7                                            | v0.9.13                            | false  | true             |
| sigs.k8s.io/kustomize/kustomize/v4                                   | v4.0.5                                            | v4.2.0                             | false  | true             |
| sigs.k8s.io/kustomize/kyaml                                          | v0.10.15                                          | v0.11.0                            | false  | true             |
| sigs.k8s.io/mdtoc                                                    | v1.0.1                                            |                                    | false  | true             |
| sigs.k8s.io/release-utils                                            | v0.3.0                                            |                                    | true   | true             |
| sigs.k8s.io/structured-merge-diff/v4                                 | v4.1.0                                            | v4.1.2                             | false  | true             |
| sigs.k8s.io/yaml                                                     | v1.2.0                                            |                                    | false  | true             |
| sigs.k8s.io/zeitgeist                                                | v0.3.0                                            |                                    | true   | true             |
| sourcegraph.com/sqs/pbtypes                                          | v0.0.0-20180604144634-d3ebe8f20ae4                | v1.0.0                             | false  | true             |
