# `contrib/cni`

There are a wide variety of different [CNI][cni] network configurations. This
directory just contains some example configurations that can be used as the
basis for your own configurations (distributions should package these files in
example directories).

## Configuration Directory

By default, your CNI configurations are read from `/etc/cni/net.d`.
This can be overwritten by specifying `crio.network.network_dir` in your
override of `/etc/crio/crio.conf.d`.

CRI-O chooses a CNI configuration from this directory with lexicographic
precedence (10-config will be chosen over 99-config).
However, CRI-O will only choose a network whose name matches the value of
`crio.network.cni_default_network` (default value is `""`).
CRI-O chooses a file alphanumerically when the value of
`crio.network.cni_default_network` is `""`.

Unless you have a specific networking configuration you'd like to use,
we recommend installing either [10-crio-bridge.conflist][dual-stack],
or [11-crio-ipv4-bridge.conflist][ipv4-only].
Installing in this case means:
Copy the respective files to the configuration directory like so:

```bash
sudo cp 10-crio-bridge.conflist /etc/cni/net.d
```

By default, we install the dual stack version: [10-crio-bridge.conflist][dual-stack]

However, if you are installing on a node with ipv6 disabled
(`sysctl net.ipv6.conf.default.disable_ipv6` and
`sysctl net.ipv6.conf.all.disable_ipv6` == 0)
then we recommend you install the ipv4 only version: [11-crio-ipv4-bridge.conflist][ipv4-only]
Otherwise, you'll run into an error similar to:

```console
Interface vetha38a080a Mac doesn't match: ee:7b:4d:57:3a:d9 not found
```

Our packaging solutions assume ipv6 is available.

[dual-stack]: 10-crio-bridge.conflist
[ipv4-only]: 11-crio-ipv4-bridge.conflist

## Plugin Directory

In addition, you need to install the [CNI plugins][cni] necessary into
`/opt/cni/bin` (or the directories specified by `crio.network.plugin_dir`). The
two plugins necessary for the example CNI configurations are `loopback` and
`bridge`. Below is a tutorial on downloading and setting up the CNI plugins.

[cni]: https://github.com/containernetworking/plugins

## CNI Plugin Installation From Source

This tutorial will use the latest version of `CNI` plugins and build it from source.

Download the `CNI` plugins source tree:

```bash
git clone https://github.com/containernetworking/plugins
cd plugins
git checkout v1.1.1
```

Build the `CNI` plugins:

```bash
./build_linux.sh # or build_windows.sh
```

Output:

```text
Building plugins
  bandwidth
  firewall
  portmap
  sbr
  tuning
  vrf
  bridge
  host-device
  ipvlan
  loopback
  macvlan
  ptp
  vlan
  dhcp
  host-local
  static
```

Install the `CNI` plugins:

```bash
sudo mkdir -p /opt/cni/bin
sudo cp bin/* /opt/cni/bin/
```
