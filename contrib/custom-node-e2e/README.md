# Node e2e test with custom CRI-O builds

The behavior of [Kubernetes node e2e tests](https://github.com/kubernetes/community/blob/41289f0f7e1fdc559c24e09c8e66c1f805fdfd52/contributors/devel/sig-node/e2e-node-tests.md)
that use [CRI-O as the CRI](https://github.com/kubernetes/test-infra/tree/859a94267c109ec709326f60cadf5add07e87cf4/jobs/e2e_node/crio),
which run on k8s CI could be replicated locally through GCP VM(s).

This could be helpful to debug node e2e test failures that can come up
while developing CRI-O itself. The `create-ignition-config.sh` script builds
CRI-O on the local system, creates an installation bundle tarball and uploads
it to a GCS bucket of user's choice.
While running the tests, a [GCP instance is created](https://github.com/kubernetes/community/blob/41289f0f7e1fdc559c24e09c8e66c1f805fdfd52/contributors/devel/sig-node/e2e-node-tests.md#remotely)
and the [Fedora CoreOS machine](https://github.com/kubernetes/test-infra/tree/859a94267c109ec709326f60cadf5add07e87cf4/jobs/e2e_node/crio/latest)
consumes the output ignition config which installs the custom CRI-O build
and e2e tests are run against it.

## Usage

<!-- markdownlint-disable MD013 -->
```shell
./create-ignition-config.sh -d CRIO_DIR -i IGNITION_OUT_DIR -b GCS_BUCKET_NAME [ -s GCS_SA_PATH ] [ -e EXTRA_CONFIG_PATH ] [ -h ]
```
<!-- markdownlint-enable MD013 -->

1. Required options:

    a. `-d CRIO_DIR`: path to cri-o source

    b. `-i IGNITION_OUT_DIR`: output directory for generated ignition config

    c. `-b GCS_BUCKET_NAME`: valid GCS bucket name with upload permissions and
    [public read access set up](https://cloud.google.com/storage/docs/access-control/making-data-public)
2. Optional flags:

    a. `-s GCS_SA_PATH`: (optional) path to GCP service account file,
    if unused `gcloud` command should have auth correctly setup.

    b. `-e EXTRA_CONFIG_PATH`: (optional) path to directory containing
    additional set of cri-o config files.
