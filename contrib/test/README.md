# Ansible Playbooks for CI

## Overview

This directory contains Ansible playbooks used in the Prow CI system for
automated testing and integration. These playbooks are referenced in the
[OpenShift release repository](https://github.com/openshift/release/tree/master/ci-operator/step-registry/cri-o).

## Integration Testing in Prow

- The base image for the test environment is built daily with the job defined
  [here](https://github.com/openshift/release/tree/master/ci-operator/step-registry/cri-o/setup)
  using `setup-main.yml`
- All necessary dependencies are automatically installed during image creation
- The integration and e2e tests use this prebuilt image from that day
- You can trigger a rebuild of the base image by creating a PR in
  openshift/release and running `/pj-rehearse` for these pipelines
  as shown in [this PR](https://github.com/openshift/release/pull/60126):
  - periodic-ci-cri-o-cri-o-main-periodics-setup-periodic
  - periodic-ci-cri-o-cri-o-main-periodics-setup-fedora-periodic
