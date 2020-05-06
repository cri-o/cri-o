![CRI-O logo](../logo/crio-logo.svg)

Downloading CRI-O via package manager

- [1.18 (latest stable)](#118--latest-stable-)
  * [CentOS 8](#centos-8)
  * [CentOS 8 Stream](#centos-8-stream)
  * [Debian Unstable](#debian-unstable)
  * [Debian Testing](#debian-testing)
  * [Fedora 31 or later](#fedora-31-or-later)
  * [Ubuntu 20.04](#ubuntu-2004)
- [1.17](#117)
  * [CentOS 7](#centos-7)
  * [CentOS 8](#centos-8-1)
  * [CentOS 8 Stream](#centos-8-stream-1)
  * [Debian Unstable](#debian-unstable-1)
  * [Debian Testing](#debian-testing-1)
  * [Fedora](#fedora)
  * [Ubuntu 18.04](#ubuntu-1804)
  * [Ubuntu 19.04](#ubuntu-1904)
  * [Ubuntu 19.10](#ubuntu-1910)
  * [Ubuntu 20.04](#ubuntu-2004-1)
- [1.16](#116)
  * [CentOS 7](#centos-7-1)
  * [CentOS 8](#centos-8-2)
  * [CentOS 8 Stream](#centos-8-stream-2)
  * [Debian Unstable](#debian-unstable-2)
  * [Debian Testing](#debian-testing-2)
  * [Fedora](#fedora-1)
  * [Ubuntu 18.04](#ubuntu-1804-1)
  * [Ubuntu 19.04](#ubuntu-1904-1)
  * [Ubuntu 19.10](#ubuntu-1910-1)
  * [Ubuntu 20.04](#ubuntu-2004-2)

# 1.18 (latest stable)
**Notes**:
  - CRI-O requires go 1.13 to compile. Thus, we are unable to support building on operating systems that do not have go 1.13


## 1.18.0
### CentOS 8
For CentOS 8 run the following as root:
```
dnf -y module disable container-tools
dnf -y install 'dnf-command(copr)'
dnf -y copr enable rhcontainerbot/container-selinux
cd /etc/yum.repos.d/
curl -L -o /etc/yum.repos.d/devel:kubic:libcontainers:stable.repo https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable/CentOS_8/devel:kubic:libcontainers:stable.repo
wget https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:1.18/CentOS_8/devel:kubic:libcontainers:stable:cri-o:1.18.repo
yum install cri-o
```
## CentOS 8 Stream
For CentOS 8 Stream run the following as root:
```
dnf -y module disable container-tools
dnf -y install 'dnf-command(copr)'
dnf -y copr enable rhcontainerbot/container-selinux
cd /etc/yum.repos.d/
curl -L -o /etc/yum.repos.d/devel:kubic:libcontainers:stable.repo https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable/CentOS_8_Stream/devel:kubic:libcontainers:stable.repo
wget https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:1.18/CentOS_8_Stream/devel:kubic:libcontainers:stable:cri-o:1.18.repo
yum install cri-o
```
### Debian Unstable
For Debian Unstable run the following as root:
Keep in mind that the owner of the key may distribute updates, packages and repositories that your system will trust.
```
echo 'deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable:/cri-o:/1.18/Debian_Unstable/ /' > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable:cri-o:1.18.list
wget -nv https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:1.18/Debian_Unstable/Release.key -O Release.key
apt-key add - < Release.key
apt-get update
apt-get install cri-o
```
### Debian Testing
For Debian Testing run the following as root:
Keep in mind that the owner of the key may distribute updates, packages and repositories that your system will trust.
```
echo 'deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable:/cri-o:/1.18/Debian_Testing/ /' > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable:cri-o:1.18.list
wget -nv https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:1.18/Debian_Testing/Release.key -O Release.key
apt-key add - < Release.key
apt-get update
apt-get install cri-o
```
### Fedora 31 or later
```
sudo dnf module enable cri-o:1.18
sudo dnf install cri-o
```
### Ubuntu 20.04
For Ubuntu 20.04 run the following:
Keep in mind that the owner of the key may distribute updates, packages and repositories that your system will trust.
```
sudo sh -c "echo 'deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable:/cri-o:/1.18/xUbuntu_20.04/ /' > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable:cri-o:1.18.list"
wget -nv https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:1.18/xUbuntu_20.04/Release.key -O Release.key
sudo apt-key add - < Release.key
sudo apt-get update
sudo apt-get install cri-o
```
# 1.17
## 1.17.4
### CentOS 7
For CentOS 7 run the following as root:
```
cd /etc/yum.repos.d/
curl -L -o /etc/yum.repos.d/devel:kubic:libcontainers:stable.repo https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable/CentOS_7/devel:kubic:libcontainers:stable.repo
wget https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:1.17/CentOS_7/devel:kubic:libcontainers:stable:cri-o:1.17.repo
yum install cri-o
```
### CentOS 8
For CentOS 8 run the following as root:
```
dnf -y module disable container-tools
dnf -y install 'dnf-command(copr)'
dnf -y copr enable rhcontainerbot/container-selinux
cd /etc/yum.repos.d/
curl -L -o /etc/yum.repos.d/devel:kubic:libcontainers:stable.repo https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable/CentOS_8/devel:kubic:libcontainers:stable.repo
wget https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:1.17/CentOS_8/devel:kubic:libcontainers:stable:cri-o:1.17.repo
yum install cri-o
```
### CentOS 8 Stream
For CentOS 8 Stream run the following as root:
```
dnf -y module disable container-tools
dnf -y install 'dnf-command(copr)'
dnf -y copr enable rhcontainerbot/container-selinux
cd /etc/yum.repos.d/
curl -L -o /etc/yum.repos.d/devel:kubic:libcontainers:stable.repo https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable/CentOS_8_Stream/devel:kubic:libcontainers:stable.repo
wget https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:1.17/CentOS_8_Stream/devel:kubic:libcontainers:stable:cri-o:1.17.repo
yum install cri-o
```
### Debian Unstable
For Debian Unstable run the following as root:
Keep in mind that the owner of the key may distribute updates, packages and repositories that your system will trust.
```
echo 'deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable:/cri-o:/1.17/Debian_Unstable/ /' > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable:cri-o:1.17.list
wget -nv https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:1.17/Debian_Unstable/Release.key -O Release.key
apt-key add - < Release.key
apt-get update
apt-get install cri-o
```
### Debian Testing
For Debian Testing run the following as root:
Keep in mind that the owner of the key may distribute updates, packages and repositories that your system will trust.
```
echo 'deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable:/cri-o:/1.17/Debian_Testing/ /' > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable:cri-o:1.17.list
wget -nv https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:1.17/Debian_Testing/Release.key -O Release.key
apt-key add - < Release.key
apt-get update
apt-get install cri-o
```
### Fedora
```
sudo dnf module enable cri-o:1.17
sudo dnf install cri-o
```
### Ubuntu 18.04
For Ubuntu 18.04 run the following:
Keep in mind that the owner of the key may distribute updates, packages and repositories that your system will trust.
```
sudo sh -c "echo 'deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable:/cri-o:/1.17/xUbuntu_18.04/ /' > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable:cri-o:1.17.list"
wget -nv https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:1.17/xUbuntu_18.04/Release.key -O Release.key
sudo apt-key add - < Release.key
sudo apt-get update
sudo apt-get install cri-o
```
### Ubuntu 19.04
For Ubuntu 19.10 run the following:
Keep in mind that the owner of the key may distribute updates, packages and repositories that your system will trust.
```
sudo sh -c "echo 'deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable:/cri-o:/1.17/xUbuntu_19.04/ /' > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable:cri-o:1.17.list"
wget -nv https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:1.17/xUbuntu_19.04/Release.key -O Release.key
sudo apt-key add - < Release.key
sudo apt-get update
sudo apt-get install cri-o
```
### Ubuntu 19.10
For Ubuntu 19.10 run the following:
Keep in mind that the owner of the key may distribute updates, packages and repositories that your system will trust.
```
sudo sh -c "echo 'deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable:/cri-o:/1.17/xUbuntu_19.10/ /' > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable:cri-o:1.17.list"
wget -nv https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:1.17/xUbuntu_19.10/Release.key -O Release.key
sudo apt-key add - < Release.key
sudo apt-get update
sudo apt-get install cri-o
```
### Ubuntu 20.04
For Ubuntu 20.04 run the following:
Keep in mind that the owner of the key may distribute updates, packages and repositories that your system will trust.
```
sudo sh -c "echo 'deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable:/cri-o:/1.17/xUbuntu_20.04/ /' > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable:cri-o:1.17.list"
wget -nv https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:1.17/xUbuntu_20.04/Release.key -O Release.key
sudo apt-key add - < Release.key
sudo apt-get update
sudo apt-get install cri-o
```
# 1.16
## 1.16.6
### CentOS 7
For CentOS 7 run the following as root:
```
cd /etc/yum.repos.d/
curl -L -o /etc/yum.repos.d/devel:kubic:libcontainers:stable.repo https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable/CentOS_7/devel:kubic:libcontainers:stable.repo
wget https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:1.16/CentOS_7/devel:kubic:libcontainers:stable:cri-o:1.16.repo
yum install cri-o
```
### CentOS 8
For CentOS 8 run the following as root:
```
dnf -y module disable container-tools
dnf -y install 'dnf-command(copr)'
dnf -y copr enable rhcontainerbot/container-selinux
cd /etc/yum.repos.d/
curl -L -o /etc/yum.repos.d/devel:kubic:libcontainers:stable.repo https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable/CentOS_8/devel:kubic:libcontainers:stable.repo
wget https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:1.16/CentOS_8/devel:kubic:libcontainers:stable:cri-o:1.16.repo
yum install cri-o
```
### CentOS 8 Stream
For CentOS 8 Stream run the following as root:
```
dnf -y module disable container-tools
dnf -y install 'dnf-command(copr)'
dnf -y copr enable rhcontainerbot/container-selinux
cd /etc/yum.repos.d/
curl -L -o /etc/yum.repos.d/devel:kubic:libcontainers:stable.repo https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable/CentOS_8_Stream/devel:kubic:libcontainers:stable.repo
wget https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:1.16/CentOS_8_Stream/devel:kubic:libcontainers:stable:cri-o:1.16.repo
yum install cri-o
```
### Debian Unstable
For Debian Unstable run the following as root:
Keep in mind that the owner of the key may distribute updates, packages and repositories that your system will trust.
```
echo 'deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable:/cri-o:/1.16/Debian_Unstable/ /' > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable:cri-o:1.16.list
wget -nv https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:1.16/Debian_Unstable/Release.key -O Release.key
apt-key add - < Release.key
apt-get update
apt-get install cri-o
```
### Debian Testing
For Debian Testing run the following as root:
Keep in mind that the owner of the key may distribute updates, packages and repositories that your system will trust.
```
echo 'deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable:/cri-o:/1.16/Debian_Testing/ /' > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable:cri-o:1.16.list
wget -nv https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:1.16/Debian_Testing/Release.key -O Release.key
apt-key add - < Release.key
apt-get update
apt-get install cri-o
```
### Fedora
```
sudo dnf module enable cri-o:1.16
sudo dnf install cri-o
```
### Ubuntu 18.04
For Ubuntu 18.04 run the following:
Keep in mind that the owner of the key may distribute updates, packages and repositories that your system will trust.
```
sudo sh -c "echo 'deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable:/cri-o:/1.16/xUbuntu_18.04/ /' > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable:cri-o:1.16.list"
wget -nv https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:1.16/xUbuntu_18.04/Release.key -O Release.key
sudo apt-key add - < Release.key
sudo apt-get update
sudo apt-get install cri-o
```
### Ubuntu 19.04
For Ubuntu 19.10 run the following:
Keep in mind that the owner of the key may distribute updates, packages and repositories that your system will trust.
```
sudo sh -c "echo 'deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable:/cri-o:/1.16/xUbuntu_19.04/ /' > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable:cri-o:1.16.list"
wget -nv https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:1.16/xUbuntu_19.04/Release.key -O Release.key
sudo apt-key add - < Release.key
sudo apt-get update
sudo apt-get install cri-o
```
### Ubuntu 19.10
For Ubuntu 19.10 run the following:
Keep in mind that the owner of the key may distribute updates, packages and repositories that your system will trust.
```
sudo sh -c "echo 'deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable:/cri-o:/1.16/xUbuntu_19.10/ /' > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable:cri-o:1.16.list"
wget -nv https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:1.16/xUbuntu_19.10/Release.key -O Release.key
sudo apt-key add - < Release.key
sudo apt-get update
sudo apt-get install cri-o
```
### Ubuntu 20.04
For Ubuntu 20.04 run the following:
Keep in mind that the owner of the key may distribute updates, packages and repositories that your system will trust.
```
sudo sh -c "echo 'deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable:/cri-o:/1.16/xUbuntu_20.04/ /' > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable:cri-o:1.16.list"
wget -nv https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:1.16/xUbuntu_20.04/Release.key -O Release.key
sudo apt-key add - < Release.key
sudo apt-get update
sudo apt-get install cri-o
```
