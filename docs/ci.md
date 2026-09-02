# CI Test Jobs

<!-- toc -->

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

Prow jobs run on GCP VMs via
[ci-operator](https://docs.ci.openshift.org/docs/architecture/ci-operator/).
Configuration lives in
[openshift/release](https://github.com/openshift/release) under
`ci-operator/config/cri-o/cri-o/`:

- **`cri-o-cri-o-main__ci.yaml`** — presubmit tests.
- **`cri-o-cri-o-main__periodics.yaml`** — periodic tests.

From these configs `prowgen` generates the Prow job definitions in
`ci-operator/jobs/cri-o/cri-o/`.

ci-operator builds a `crio-crio-base-src` image containing the CRI-O source
tree. A `skip_if_only_changed` filter skips builds when the PR only touches
docs or metadata.

### Base images

GCP VM-based presubmit jobs (Kubernetes e2e, integration, critest) run on GCE
images that have all dependencies pre-installed. Two daily periodic jobs build
these images:

| Periodic                 | Image family       | OS       |
| ------------------------ | ------------------ | -------- |
| `setup-periodic`         | `crio-setup`       | RHEL 9   |
| `setup-fedora-periodic`  | `crio-setup-fedora`| Fedora   |

Each run provisions a VM, runs
[`setup-main.yml`](../contrib/test/ci/setup-main.yml) to install all
dependencies (Go, runc, crun, conmon, conmon-rs, cri-tools, CNI plugins,
Kubernetes, bats), snapshots the disk into the image family, and cleans up
images older than two weeks. Presubmit jobs reference the family via
`--image-family`, so they always boot the latest snapshot.

Fixing a base image issue is a multi-step process: the fix must be merged to
`main` first, then the next daily periodic picks it up and rebuilds the image.
The fix cannot be validated from a PR because the periodic always reads from
`main`. A broken periodic build blocks all PR testing until the next successful
run.

OpenShift e2e jobs (`e2e-aws-ovn`, `e2e-gcp-ovn`, `perfscale`) do not use
these images — they build CRI-O RPMs into RHCOS instead.

Some tests use environment variables to change behavior:
`USE_CONMONRS` (use conmon-rs), `EVENTED_PLEG` (enable evented PLEG),
`IMAGE_FAMILY` / `IMAGE_NAME` (select a different GCE image).

### Presubmits

Run on every PR to `main`. Re-trigger with `/test <name>` in a PR comment.

#### Kubernetes e2e

Upstream Kubernetes e2e suite on a single-node cluster, skipping slow/serial/
disruptive/flaky tests. All jobs run crun on cgroupv2 (the compiled default on
RHEL 9). Several jobs pass Ansible extra-vars (`build_runc`, `build_crun`,
`cgroupv2`) that are dead letters — the playbooks never consume them.

| Context                          | Monitor   | Notes                                   |
| -------------------------------- | --------- | --------------------------------------- |
| `ci-e2e`                         | conmon    |                                         |
| `ci-e2e-conmonrs`                | conmon-rs |                                         |
| `ci-crun-e2e`                    | conmon    | duplicate of `ci-e2e`                   |
| `ci-rhel-e2e`                    | conmon    | re-runs setup; otherwise same as `ci-e2e` |
| `ci-cgroupv2-e2e`                | conmon    | duplicate of `ci-e2e`                   |
| `ci-cgroupv2-e2e-crun`           | conmon    | duplicate of `ci-e2e`                   |
| `ci-cgroupv2-e2e-features`       | conmon    | runs feature-gated tests                |
| `ci-e2e-evented-pleg` (optional) | conmon    | enables evented PLEG                    |

#### CRI conformance, integration, and Kata

| Context                    | Type        | OS     | Notes                                      |
| -------------------------- | ----------- | ------ | ------------------------------------------ |
| `ci-fedora-critest`        | critest     | Fedora |                                            |
| `ci-rhel-critest`          | critest     | RHEL   |                                            |
| `ci-fedora-integration`    | integration | Fedora |                                            |
| `ci-cgroupv2-integration`  | integration | RHEL   | duplicate of `ci-fedora-integration`       |
| `ci-fedora-kata`           | integration | Fedora | Kata runtime, subset of tests              |

#### OpenShift e2e

| Context                          | Cloud | Description                      | Trigger              |
| -------------------------------- | ----- | -------------------------------- | -------------------- |
| `e2e-aws-ovn`                    | AWS   | OpenShift e2e with OVN           | auto on code changes |
| `e2e-gcp-ovn`                    | GCP   | OpenShift e2e with OVN           | auto on code changes |
| `perfscale-control-plane-6nodes` | AWS   | Performance/scale test (6 nodes) | manual only          |

### Periodics

| Job                                      | Schedule | Description                   | Slack                |
| ---------------------------------------- | -------- | ----------------------------- | -------------------- |
| `setup-periodic`                         | daily    | RHEL image setup validation   | `#forum-node-jira`   |
| `setup-fedora-periodic`                  | daily    | Fedora image setup validation | `#forum-node-jira`   |
| `crio-node-e2e-conformance-periodic`     | @yearly  | Node e2e conformance suite    |                      |
| `crio-node-e2e-nodeconformance-periodic` | @yearly  | Node conformance suite        |                      |
| `crio-node-e2e-nodefeature-periodic`     | @yearly  | Node feature tests            |                      |

### Release branch jobs

Each `release-1.y` branch has its own ci-operator config and presubmit jobs at
`ci-operator/{config,jobs}/cri-o/cri-o/cri-o-cri-o-release-1.y-*`.
