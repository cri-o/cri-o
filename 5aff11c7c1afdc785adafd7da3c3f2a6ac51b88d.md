- [CRI-O 5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d](#cri-o-5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d)
  - [Downloads](#downloads)
  - [Changelog since v1.29.0](#changelog-since-v1290)
    - [Changes by Kind](#changes-by-kind)
      - [API Change](#api-change)
      - [Feature](#feature)
      - [Uncategorized](#uncategorized)
  - [Dependencies](#dependencies)
    - [Added](#added)
    - [Changed](#changed)
    - [Removed](#removed)

# CRI-O 5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d

The release notes have been generated for the commit range
[v1.29.0...5aff11c](https://github.com/cri-o/cri-o/compare/v1.29.0...5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d) on Thu, 18 Jan 2024 22:35:44 UTC.

## Downloads

Download one of our static release bundles via our Google Cloud Bucket:

- [cri-o.amd64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz](https://storage.googleapis.com/cri-o/artifacts/cri-o.amd64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz)
  - [cri-o.amd64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.sha256sum](https://storage.googleapis.com/cri-o/artifacts/cri-o.amd64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.sha256sum)
  - [cri-o.amd64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.sig](https://storage.googleapis.com/cri-o/artifacts/cri-o.amd64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.sig)
  - [cri-o.amd64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.cert](https://storage.googleapis.com/cri-o/artifacts/cri-o.amd64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.cert)
  - [cri-o.amd64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.spdx](https://storage.googleapis.com/cri-o/artifacts/cri-o.amd64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.spdx)
  - [cri-o.amd64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.spdx.sig](https://storage.googleapis.com/cri-o/artifacts/cri-o.amd64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.spdx.sig)
  - [cri-o.amd64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.spdx.cert](https://storage.googleapis.com/cri-o/artifacts/cri-o.amd64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.spdx.cert)
- [cri-o.arm64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz](https://storage.googleapis.com/cri-o/artifacts/cri-o.arm64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz)
  - [cri-o.arm64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.sha256sum](https://storage.googleapis.com/cri-o/artifacts/cri-o.arm64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.sha256sum)
  - [cri-o.arm64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.sig](https://storage.googleapis.com/cri-o/artifacts/cri-o.arm64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.sig)
  - [cri-o.arm64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.cert](https://storage.googleapis.com/cri-o/artifacts/cri-o.arm64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.cert)
  - [cri-o.arm64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.spdx](https://storage.googleapis.com/cri-o/artifacts/cri-o.arm64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.spdx)
  - [cri-o.arm64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.spdx.sig](https://storage.googleapis.com/cri-o/artifacts/cri-o.arm64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.spdx.sig)
  - [cri-o.arm64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.spdx.cert](https://storage.googleapis.com/cri-o/artifacts/cri-o.arm64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.spdx.cert)
- [cri-o.ppc64le.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz](https://storage.googleapis.com/cri-o/artifacts/cri-o.ppc64le.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz)
  - [cri-o.ppc64le.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.sha256sum](https://storage.googleapis.com/cri-o/artifacts/cri-o.ppc64le.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.sha256sum)
  - [cri-o.ppc64le.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.sig](https://storage.googleapis.com/cri-o/artifacts/cri-o.ppc64le.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.sig)
  - [cri-o.ppc64le.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.cert](https://storage.googleapis.com/cri-o/artifacts/cri-o.ppc64le.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.cert)
  - [cri-o.ppc64le.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.spdx](https://storage.googleapis.com/cri-o/artifacts/cri-o.ppc64le.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.spdx)
  - [cri-o.ppc64le.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.spdx.sig](https://storage.googleapis.com/cri-o/artifacts/cri-o.ppc64le.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.spdx.sig)
  - [cri-o.ppc64le.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.spdx.cert](https://storage.googleapis.com/cri-o/artifacts/cri-o.ppc64le.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.spdx.cert)

To verify the artifact signatures via [cosign](https://github.com/sigstore/cosign), run:

```console
> export COSIGN_EXPERIMENTAL=1
> cosign verify-blob cri-o.amd64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz \
    --certificate-identity https://github.com/cri-o/cri-o/.github/workflows/test.yml@refs/tags/5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com \
    --certificate-github-workflow-repository cri-o/cri-o \
    --certificate-github-workflow-ref refs/tags/5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d \
    --signature cri-o.amd64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.sig \
    --certificate cri-o.amd64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.cert
```

To verify the bill of materials (SBOM) in [SPDX](https://spdx.org) format using the [bom](https://sigs.k8s.io/bom) tool, run:

```console
> tar xfz cri-o.amd64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz
> bom validate -e cri-o.amd64.5aff11c7c1afdc785adafd7da3c3f2a6ac51b88d.tar.gz.spdx -d cri-o
```

## Changelog since v1.29.0

### Changes by Kind

#### API Change
 - Added more file system information in `ImageFsInfo` as part of the garbage collection KEP. (#7269, @kannon92)

#### Feature
 - Add support for --metrics-host. (#7677, @rphillips)

#### Uncategorized
 - Update linked logs to drop an intermediate directory and append `.log` to the container symlink (#7653, @haircommander)

## Dependencies

### Added
_Nothing has changed._

### Changed
- github.com/containers/podman/v4: [v4.8.2 → v4.8.3](https://github.com/containers/podman/v4/compare/v4.8.2...v4.8.3)
- github.com/intel/goresctrl: [v0.5.0 → v0.6.0](https://github.com/intel/goresctrl/compare/v0.5.0...v0.6.0)
- github.com/urfave/cli/v2: [v2.26.0 → v2.27.0](https://github.com/urfave/cli/v2/compare/v2.26.0...v2.27.0)
- golang.org/x/exp: 7918f67 → aacd6d4
- golang.org/x/sync: v0.5.0 → v0.6.0

### Removed
- github.com/google/go-github/v50: [v50.2.0](https://github.com/google/go-github/v50/tree/v50.2.0)
