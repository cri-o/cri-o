# Install with Package Managers

CRI-O builds for native package managers using [openSUSE's OBS](build.opensuse.org)

## Supported Versions
Below is a compatiblity matrix between versions of CRI-O (y-axis) and distributions (x-axis)

|      | Fedora 31+ | openSUSE | CentOS_8 | CentOS_8_Stream | CentOS_7 | Debian_Unstable | Debian_Testing | Debian 10 | Rasbian_10 | xUbuntu_20.04 | xUbuntu_19.10 | xUbuntu_19.04 | xUbuntu_18.04 |
| ---- | ---------- | -------- | -------- | --------------- | -------- | --------------- | -------------- | --------- | ---------- | ------------- | ------------- | ------------- | ------------- |
| 1.18 | ✓          | ✓        | ✓        | ✓               |          | ✓               | ✓              |           |            | ✓             |               |               |               |
| 1.17 | ✓          | ✓        | ✓        | ✓               | ✓        | ✓               | ✓              | ✓         | ✓          | ✓             | ✓             | ✓             | ✓             |
| 1.16 | ✓          | ✓        | ✓        | ✓               | ✓        | ✓               | ✓              | ✓         | ✓          | ✓             | ✓             | ✓             | ✓             |

To install, choose a supported version for your operating system, and export it as a variable, like so:
`export VERSION=1.18`

We also save releases as subprojects. If you'd, for instance, like to use `1.18.3` you can set
`export VERSION=1.18:1.18.3`

## Installation Instructions

### openSUSE:

Run the following as root:
```shell
zypper install -y cri-o
```

### Fedora 31 or later

Run the following as root:
```shell
dnf module enable cri-o:$VERSION
dnf install -y cri-o
```
For Fedora, we only support setting minor versions. i.e: `VERSION=1.18`, and do not support pinning patch versions: `VERSION=1.18.3`

### Other yum based operating systems
To install on the following operating systems, set the environment variable $OS as the appropriate field in the following table:

| Operating system | $OS               |
| ---------------- | ----------------- |
| Centos 8         | `CentOS_8`        |
| Centos 8 Stream  | `CentOS_8_Stream` |
| Centos 7         | `CentOS_7`        |


And then run the following as root:
```shell
curl -L -o /etc/yum.repos.d/devel:kubic:libcontainers:stable.repo https://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable/$OS/devel:kubic:libcontainers:stable.repo
curl -L -o /etc/yum.repos.d/devel:kubic:libcontainers:stable:cri-o:$VERSION.repo https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:$VERSION/$OS/devel:kubic:libcontainers:stable:cri-o:$VERSION.repo
yum install -y cri-o
```

### Apt based operating systems

Note: these are prerequisites for installing

To install on the following operating systems, set the environment variable $OS as the appropriate field in the following table:
| Operating system | $OS               |
| ---------------- | ----------------- |
| Debian Unstable  | `Debian_Unstable` |
| Debian Testing   | `Debian_Testing`  |
| Ubuntu 20.04     | `xUbuntu_20.04`   |
| Ubuntu 19.10     | `xUbuntu_19.10`   |
| Ubuntu 19.04     | `xUbuntu_19.04`   |
| Ubuntu 18.04     | `xUbuntu_18.04`   |

And then run the following as root:
```shell
apt update
apt install -y ca-certificates curl gnupg

echo "deb [signed-by=/usr/share/keyrings/libcontainers-archive-keyring.gpg] https://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable/$OS/ /" > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable.list
echo "deb [signed-by=/usr/share/keyrings/libcontainers-crio-archive-keyring.gpg] https://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable:/cri-o:/$VERSION/$OS/ /" > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable:cri-o:$VERSION.list

mkdir -p /usr/share/keyrings
curl -L https://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable/$OS/Release.key | gpg --dearmor -o /usr/share/keyrings/libcontainers-archive-keyring.gpg
curl -L https://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable:/cri-o:/$VERSION/$OS/Release.key | gpg --dearmor -o /usr/share/keyrings/libcontainers-crio-archive-keyring.gpg

apt install -y cri-o
```
