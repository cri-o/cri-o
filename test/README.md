# CRIO Integration Tests

Integration tests provide end-to-end testing of CRIO.

Note that integration tests do **not** replace unit tests.

As a rule of thumb, code should be tested thoroughly with unit tests.
Integration tests on the other hand are meant to test a specific feature end
to end.

Integration tests are written in _bash_ using the
[bats](https://github.com/bats-core/bats-core) framework.

## Running integration tests

### Containerized tests

The easiest way to run integration tests is with Podman:

```shell
make integration
```

To run a single test bucket:

```shell
make integration TESTFLAGS="runtimeversion.bats"
```

### On your host

To run the integration tests on your host, you will first need to setup a
development environment plus
[bats](https://github.com/bats-core/bats-core#installing-bats-from-source)
For example:

```shell
cd ~/go/src/github.com
git clone https://github.com/bats-core/bats-core.git
cd bats
./install.sh /usr/local
```

You will also need to install the [CNI](https://github.com/containernetworking/cni)
plugins as the default pod test template runs without host networking.

Check instruction [here](/contrib/cni/README.md#cni-plugin-installation-from-source)

Then you can run the tests on your host:

```shell
sudo make localintegration
```

To run a single test bucket:

```shell
make localintegration TESTFLAGS="runtimeversion.bats"
```

Or you can just run them directly using bats

```shell
sudo bats test
```

#### Runtime selection

Tests on the host will run with `runc` as the default runtime.
However you can select other OCI compatible runtimes by setting
the `RUNTIME` environment variable.

For example, to use [crun](https://github.com/containers/crun) instead of `runc`:

```shell
make CONTAINER_DEFAULT_RUNTIME=crun localintegration
```

If you'd like to run the tests with a runtime of a different type, you need to
also specify `$RUNTIME_TYPE`

For example, to use
[kata](https://github.com/kata-containers/kata-containers) with shim v2:

```shell
make CONTAINER_DEFAULT_RUNTIME=containerd-shim-kata-v2 RUNTIME_TYPE=vm localintegration
```

## Writing integration tests

[Helper functions](https://github.com/cri-o/cri-o/blob/main/test/helpers.bash)
are provided in order to facilitate writing tests.

```sh
#!/usr/bin/env bats

# This will load the helpers.
load helpers

# setup is called at the beginning of every test.
function setup() {
}

# teardown is called at the end of every test.
function teardown() {
  cleanup_test
}

@test "crictl runtimeversion" {
  start_crio
  crictl runtimeversion
}
```
