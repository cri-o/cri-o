# CI Test Jobs

<!-- toc -->

- [GitHub Actions](#github-actions)
- [OpenShift CI (Prow)](#openshift-ci-prow)
  - [Base images](#base-images)
  - [Presubmits](#presubmits)
    - [Kubernetes e2e](#kubernetes-e2e)
    - [CRI conformance, integration, and Kata](#cri-conformance-integration-and-kata)
    - [Performance and scale](#performance-and-scale)
  - [Postsubmits](#postsubmits)
  - [Periodics](#periodics)
  - [Release branch jobs](#release-branch-jobs)

<!-- /toc -->

CRI-O runs test jobs on two platforms:

- **GitHub Actions** — unit and integration tests in
  [`.github/workflows/`](../.github/workflows/).
- **OpenShift CI (Prow)** — e2e, integration, and conformance tests in
  [openshift/release](https://github.com/openshift/release).

For non-test workflows see the manifests in
[`.github/workflows/`](../.github/workflows/). For release automation see
[`scripts/ci.md`](../scripts/ci.md).

## GitHub Actions

[`test.yml`](../.github/workflows/test.yml) runs
[Ginkgo](https://onsi.github.io/ginkgo/) unit tests on amd64 (root + rootless)
and arm64 (root), with coverage uploaded to Codecov.

[`integration.yml`](../.github/workflows/integration.yml) runs
[BATS](https://github.com/bats-core/bats-core) integration tests and
[critest](https://github.com/kubernetes-sigs/cri-tools) CRI conformance across
conmon/conmon-rs, amd64/arm64, and user namespace mode. All jobs use crun.

The integration workflow installs dependencies via
[`scripts/github-actions-setup`](../scripts/github-actions-setup) with versions
pinned in [`scripts/versions`](../scripts/versions).

## OpenShift CI (Prow)

Prow jobs are defined in [openshift/release](https://github.com/openshift/release/tree/main/ci-operator/config/cri-o/cri-o):

- [`cri-o-cri-o-main__ci.yaml`](https://github.com/openshift/release/blob/main/ci-operator/config/cri-o/cri-o/cri-o-cri-o-main__ci.yaml)
  defines VM-based presubmit tests.
- [`cri-o-cri-o-main__periodics.yaml`](https://github.com/openshift/release/blob/main/ci-operator/config/cri-o/cri-o/cri-o-cri-o-main__periodics.yaml)
  defines VM-based periodic tests and image setup jobs.
- [`cri-o-cri-o-main.yaml`](https://github.com/openshift/release/blob/main/ci-operator/config/cri-o/cri-o/cri-o-cri-o-main.yaml)
  defines OpenShift e2e postsubmits and the perfscale presubmit. It builds
  CRI-O RPMs with `hack/build-rpms.sh` and layers them into RHCOS.

For current schedules and triggers, consult these configs and the generated
[Prow job definitions](https://github.com/openshift/release/tree/main/ci-operator/jobs/cri-o/cri-o).
The [Prow job history](https://prow.ci.openshift.org/?repo=cri-o%2Fcri-o)
shows recent runs.

The VM-based jobs use [ci-operator](https://docs.ci.openshift.org/docs/architecture/ci-operator/)
to build a `crio-crio-base-src` image containing the CRI-O source tree. The
generated `images`, `ci-images`, and `periodics-images` presubmits have a
`skip_if_only_changed` filter for documentation, `.github/`, and other
non-code paths. The `ci-*` test jobs still run on docs-only PRs.

### Base images

GCP VM-based presubmit jobs (Kubernetes e2e, integration, critest) run on GCE
images with dependencies pre-installed. The `setup-periodic` and
`setup-fedora-periodic` jobs build the `crio-setup` (RHEL 9) and
`crio-setup-fedora` (Fedora) image families, respectively. See their
[configuration](https://github.com/openshift/release/blob/main/ci-operator/config/cri-o/cri-o/cri-o-cri-o-main__periodics.yaml)
and [RHEL](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/periodic-ci-cri-o-cri-o-main-periodics-setup-periodic)
and [Fedora](https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/periodic-ci-cri-o-cri-o-main-periodics-setup-fedora-periodic)
job histories for their current schedules and runs.

Each run provisions a VM, runs
[`setup-main.yml`](../contrib/test/ci/setup-main.yml) to install all
dependencies (Go, runc, crun, conmon, conmon-rs, cri-tools, CNI plugins,
Kubernetes, bats), snapshots the disk into the image family, and cleans up
images older than two weeks. Presubmit jobs reference the family via
`--image-family`, so they always boot the latest snapshot.

To update a base image dependency, merge the fix to `main` and trigger the setup
job to rebuild the image. The setup jobs read from `main`, so a PR cannot test
the new image through those jobs. If setup fails before creating an image, the
family retains its last good image; a successful build that snapshots a broken
dependency can affect subsequent PR tests.

Periodic jobs cannot be triggered with `/test`. To rebuild a base image manually,
use the [Gangway REST API](https://docs.ci.openshift.org/docs/how-tos/triggering-prowjobs-via-rest/)
with an authentication token. With `oc` logged in to the app.ci cluster, trigger
the RHEL setup job with:

```sh
curl --fail-with-body -X POST \
  -H "Authorization: Bearer $(oc whoami -t)" \
  -H 'Content-Type: application/json' \
  -d '{"job_name":"periodic-ci-cri-o-cri-o-main-periodics-setup-periodic","job_execution_type":"1"}' \
  https://gangway-ci.apps.ci.l2s4.p1.openshiftapps.com/v1/executions
```

OpenShift e2e and perfscale jobs build CRI-O RPMs into RHCOS and do not use
these images.

The setup workflow can set `IMAGE_NAME` to select a source image; it takes
precedence over `IMAGE_FAMILY` when provisioning. Tests use `IMAGE_FAMILY` to
boot from the image family, which the setup workflow also snapshots into.

### Presubmits

Presubmit jobs target PRs to `main`. Most run automatically; optional jobs run
when requested. Re-trigger with `/test <name>` in a PR comment.
The `ci-rhel-e2e-conmonrs` job sets `USE_CONMONRS` to use conmon-rs, and the
optional `ci-rhel-e2e-evented-pleg` job sets `EVENTED_PLEG`.

#### Kubernetes e2e

Upstream Kubernetes e2e suite on a single-node cluster. The standard e2e jobs
skip slow, serial, disruptive, flaky, and `[Feature:*]` tests. The features job
focuses on `[NodeFeature:*]` and selected `[Feature:*]` tests; it skips slow,
serial, and flaky tests. All jobs run on RHEL 9 with cgroup v2 and use CRI-O's
default OCI runtime, crun.

| Context                               | Monitor   | Notes                |
| ------------------------------------- | --------- | -------------------- |
| `ci-rhel-e2e`                         | conmon    |                      |
| `ci-rhel-e2e-conmonrs`                | conmon-rs |                      |
| `ci-rhel-e2e-features`                | conmon    | feature-gated tests  |
| `ci-rhel-e2e-evented-pleg` (optional) | conmon    | evented PLEG enabled |

#### CRI conformance, integration, and Kata

| Context                      | Type        | OS     | Notes                         |
| ---------------------------- | ----------- | ------ | ----------------------------- |
| `ci-fedora-critest`          | critest     | Fedora |                               |
| `ci-rhel-critest`            | critest     | RHEL   |                               |
| `ci-fedora-integration`      | integration | Fedora |                               |
| `ci-fedora-integration-kata` | integration | Fedora | Kata runtime, subset of tests |

#### Performance and scale

`perfscale-control-plane-6nodes` runs a six-node AWS performance and scale
test when requested.

### Postsubmits

After changes merge to `main`, `e2e-aws-ovn` and `e2e-gcp-ovn` run OpenShift
e2e tests with OVN on AWS and GCP, respectively. They do not run as PR checks.

### Periodics

The `setup-periodic` and `setup-fedora-periodic` jobs build the base images
described above. The `crio-node-e2e-conformance-periodic`,
`crio-node-e2e-nodeconformance-periodic`, and
`crio-node-e2e-nodefeature-periodic` jobs run node e2e suites. Their schedules
are in the [periodic configuration](https://github.com/openshift/release/blob/main/ci-operator/config/cri-o/cri-o/cri-o-cri-o-main__periodics.yaml).

### Release branch jobs

Release branch configurations, where present, are named
`ci-operator/config/cri-o/cri-o/cri-o-cri-o-release-1.y.yaml`. Their generated
presubmits are under `ci-operator/jobs/cri-o/cri-o/`. The current release
configurations run only the `images`, `e2e-aws-ovn`, and `e2e-gcp-ovn`
presubmits, not the `ci-*` VM jobs.
