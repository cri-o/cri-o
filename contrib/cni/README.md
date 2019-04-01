## `contrib/cni` ##

There are a wide variety of different [CNI][cni] network configurations. This
directory just contains some example configurations that can be used as the
basis for your own configurations (distributions should package these files in
example directories).

To use these configurations, place them in `/etc/cni/net.d` (or the directory
specified by `crio.network.network_dir` in your `crio.conf`).

CRI-O will only choose networks that have the name specified in
`crio.network.default_network` in crio.conf. The default value for this is `crio`.

In addition, you need to install the [CNI plugins][cni] necessary into
`/opt/cni/bin` (or the directories specified by `crio.network.plugin_dir`). The
two plugins necessary for the example CNI configurations are `loopback` and
`bridge`. Below is a tutorial on downloading and setting up the CNI plugins.

[cni]: https://github.com/containernetworking/plugins

### Plugins tutorial

This tutorial will use the latest version of `CNI` plugins from the master branch and build it from source.

Download the `CNI` plugins source tree:

```bash
git clone https://github.com/containernetworking/plugins
cd plugins
git checkout v0.7.4
```

Build the `CNI` plugins:

```
./build_linux.sh # or build_windows.sh
```

Output:

```
Building API
Building reference CLI
Building plugins
   flannel
   tuning
   bridge
   ipvlan
   loopback
   macvlan
   ptp
   dhcp
   host-local
   noop
```

Install the `CNI` plugins:

```
sudo mkdir -p /opt/cni/bin
sudo cp bin/* /opt/cni/bin/
```
