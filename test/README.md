# CRIO Integration Tests

Integration tests provide end-to-end testing of CRIO.

Note that integration tests do **not** replace unit tests.

As a rule of thumb, code should be tested thoroughly with unit tests.
Integration tests on the other hand are meant to test a specific feature end
to end.

Integration tests are written in *bash* using the
[bats](https://github.com/bats-core/bats-core) framework.

## Running integration tests

### Containerized tests

The easiest way to run integration tests is with Podman:
```
$ make integration
```

To run a single test bucket:
```
$ make integration TESTFLAGS="runtimeversion.bats"
```

### On your host

To run the integration tests on your host, you will first need to setup a development environment plus
[bats](https://github.com/bats-core/bats-core#installing-bats-from-source)
For example:
```
$ cd ~/go/src/github.com
$ git clone https://github.com/bats-core/bats-core.git
$ cd bats
$ ./install.sh /usr/local
```

You will also need to install the [CNI](https://github.com/containernetworking/cni) plugins as
the default pod test template runs without host networking:

```
$ cd "$GOPATH/src/github.com/containernetworking"
$ git clone https://github.com/containernetworking/plugins.git
$ cd plugins
$ git checkout -q dcf7368eeab15e2affc6256f0bb1e84dd46a34de
$ ./build.sh
$ mkdir -p /opt/cni/bin
$ cp bin/* /opt/cni/bin/
```

Then you can run the tests on your host:
```
$ sudo make localintegration
```

To run a single test bucket:
```
$ make localintegration TESTFLAGS="runtimeversion.bats"
```

Or you can just run them directly using bats
```
$ sudo bats test
```

#### Runtime selection
Tests on the host will run with `runc` as the default runtime.
However you can select other OCI compatible runtimes by setting
the `RUNTIME` environment variable.

For example, to use [crun](https://github.com/containers/crun) instead of `runc`:
```
make CONTAINER_DEFAULT_RUNTIME=crun localintegration
```

If you'd like to run the tests with a runtime of a different type, you need to also specify `$RUNTIME_TYPE`

For example, to use [kata](https://github.com/kata-containers/runtime) with shim v2:
```
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
	[ "$status" -eq 0 ]
}

```
