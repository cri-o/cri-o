# Legacy Installation Instructions

## openSUSE

```shell
sudo zypper install cri-o
```

## Fedora 31 or later

```shell
sudo dnf module enable cri-o:$VERSION
sudo dnf install cri-o
```

For Fedora, we only support setting minor versions. i.e: `VERSION=1.18`,
and do not support pinning patch versions: `VERSION=1.18.3`

Note: as of 1.24.0, the `cri-o` package no longer depends on
`containernetworking-plugins` package.
Removing this dependency allows users to install their own CNI plugins without
having to remove files first.
If users want to use the previously provided CNI plugins, they should also run:

```shell
sudo dnf install containernetworking-plugins
```

## Other yum based operating systems

To install on the following operating systems, set the environment variable ```$OS```
to the appropriate value from the following table:

| Operating system | $OS               |
| ---------------- | ----------------- |
| Centos 9 Stream  | `CentOS_9_Stream` |
| Centos 8         | `CentOS_8`        |
| Centos 8 Stream  | `CentOS_8_Stream` |
| Centos 7         | `CentOS_7`        |

And then run the following as root:

<!-- markdownlint-disable MD013 -->
```shell
curl -L -o /etc/yum.repos.d/devel:kubic:libcontainers:stable.repo https://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable/$OS/devel:kubic:libcontainers:stable.repo
curl -L -o /etc/yum.repos.d/devel:kubic:libcontainers:stable:cri-o:$VERSION.repo https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:$VERSION/$OS/devel:kubic:libcontainers:stable:cri-o:$VERSION.repo

or if you are using a subproject release:

curl -L -o /etc/yum.repos.d/devel:kubic:libcontainers:stable:cri-o:${VERSION}.repo https://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable:/cri-o:/${SUBVERSION}:/${VERSION}/$OS/devel:kubic:libcontainers:stable:cri-o:${SUBVERSION}:${VERSION}.repo

yum install cri-o
```
<!-- markdownlint-enable MD013 -->

Note: as of 1.24.0, the `cri-o` package no longer depends on
`containernetworking-plugins` package.
Removing this dependency allows users to install their own CNI plugins without
having to remove files first.
If users want to use the previously provided CNI plugins, they should also run:

```shell
yum install containernetworking-plugins
```

## APT based operating systems

Note: this tutorial assumes you have curl and gnupg installed

To install on the following operating systems, set the environment variable ```$OS```
to the appropriate value from the following table:

| Operating system   | $OS               |
| ------------------ | ----------------- |
| Debian 12          | `Debian_12`       |
| Debian 11          | `Debian_11`       |
| Debian 10          | `Debian_10`       |
| Raspberry Pi OS 11 | `Raspbian_11`     |
| Raspberry Pi OS 10 | `Raspbian_10`     |
| Ubuntu 22.04       | `xUbuntu_22.04`   |
| Ubuntu 21.10       | `xUbuntu_21.10`   |
| Ubuntu 21.04       | `xUbuntu_21.04`   |
| Ubuntu 20.10       | `xUbuntu_20.10`   |
| Ubuntu 20.04       | `xUbuntu_20.04`   |
| Ubuntu 18.04       | `xUbuntu_18.04`   |

If installing cri-o-runc (recommended), you'll need to install libseccomp >= 2.4.1.
**NOTE: This is not available in distros based on Debian 10(buster) or below,
so buster backports will need to be enabled:**

<!-- markdownlint-disable MD013 -->
```shell
echo 'deb http://deb.debian.org/debian buster-backports main' > /etc/apt/sources.list.d/backports.list
apt update
apt install -y -t buster-backports libseccomp2 || apt update -y -t buster-backports libseccomp2
```

And then run the following as root:

```shell
echo "deb [signed-by=/usr/share/keyrings/libcontainers-archive-keyring.gpg] https://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable/$OS/ /" > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable.list
echo "deb [signed-by=/usr/share/keyrings/libcontainers-crio-archive-keyring.gpg] https://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable:/cri-o:/$VERSION/$OS/ /" > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable:cri-o:$VERSION.list

mkdir -p /usr/share/keyrings
curl -L https://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable/$OS/Release.key | gpg --dearmor -o /usr/share/keyrings/libcontainers-archive-keyring.gpg
curl -L https://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable:/cri-o:/$VERSION/$OS/Release.key | gpg --dearmor -o /usr/share/keyrings/libcontainers-crio-archive-keyring.gpg

apt-get update
apt-get install cri-o cri-o-runc
```
<!-- markdownlint-enable MD013 -->

**Note: We include cri-o-runc because Ubuntu and Debian include their own packaged
version of runc.**
While this version should work with CRI-O, keeping the packaged versions of CRI-O
and runc in sync ensures they work together.
If you'd like to use the distribution's runc, you'll have to add the file:

```toml
[crio.runtime.runtimes.runc]
runtime_path = ""
runtime_type = "oci"
runtime_root = "/run/runc"
```

to `/etc/crio/crio.conf.d/`

Note: as of 1.24.0, the `cri-o` package no longer depends on
`containernetworking-plugins` package.
Removing this dependency allows users to install their own CNI plugins without
having to remove files first.
If users want to use the previously provided CNI plugins, they should also run:

```shell
apt-get install containernetworking-plugins
```
