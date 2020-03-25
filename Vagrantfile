# -*- mode: ruby -*-
# vi: set ft=ruby :

# All Vagrant configuration is done below. The "2" in Vagrant.configure
# configures the configuration version (we support older styles for
# backwards compatibility). Please don't change it unless you know what
# you're doing.
Vagrant.configure("2") do |config|
  # The most common configuration options are documented and commented below.
  # For a complete reference, please see the online documentation at
  # https://docs.vagrantup.com.

  # Every Vagrant development environment requires a box. You can search for
  # boxes at https://atlas.hashicorp.com/search.
  config.vm.box = "fedora/30-cloud-base"

  # Disable automatic box update checking. If you disable this, then
  # boxes will only be checked for updates when the user runs
  # `vagrant box outdated`. This is not recommended.
  # config.vm.box_check_update = false

  # Create a forwarded port mapping which allows access to a specific port
  # within the machine from a port on the host machine. In the example below,
  # accessing "localhost:8080" will access port 80 on the guest machine.
  # config.vm.network "forwarded_port", guest: 80, host: 8080

  # Create a private network, which allows host-only access to the machine
  # using a specific IP.
  # config.vm.network "private_network", ip: "192.168.33.10"

  # Create a public network, which generally matched to bridged network.
  # Bridged networks make the machine appear as another physical device on
  # your network.
  # config.vm.network "public_network"

  # Share an additional folder to the guest VM. The first argument is
  # the path on the host to the actual folder. The second argument is
  # the path on the guest to mount the folder. And the optional third
  # argument is a set of non-required options.
  config.vm.synced_folder "./", "/home/vagrant/go/src/github.com/kubernetes-incubator/cri-o", type: "rsync"

  # Provider-specific configuration so you can fine-tune various
  # backing providers for Vagrant. These expose provider-specific options.
  # Example for VirtualBox:
  #
  config.vm.provider "virtualbox" do |vb|
  #   # Display the VirtualBox GUI when booting the machine
  #   vb.gui = true
  #
  #   # Customize the amount of memory on the VM:
    vb.memory = "4096"
  end
  config.vm.provider :libvirt do |libvirt|
  #   # Customize the amount of memory on the VM:
    libvirt.memory = "4096"
  end
  #
  # View the documentation for the provider you are using for more
  # information on available options.

  # Define a Vagrant Push strategy for pushing to Atlas. Other push strategies
  # such as FTP and Heroku are also available. See the documentation at
  # https://docs.vagrantup.com/v2/push/atlas.html for more information.
  # config.push.define "atlas" do |push|
  #   push.app = "YOUR_ATLAS_USERNAME/YOUR_APPLICATION_NAME"
  # end

  # Enable provisioning with a shell script. Additional provisioners such as
  # Puppet, Chef, Ansible, Salt, and Docker are also available. Please see the
  # documentation for more information about their specific syntax and use.
  # install pkgs
  config.vm.provision "shell", inline: <<-SHELL
    dnf install -y \
      btrfs-progs-devel \
      device-mapper-devel \
      docker \
      git \
      glib2-devel \
      glibc-devel \
      glibc-static \
      go \
      golang-github-cpuguy83-go-md2man \
      gpgme-devel \
      libassuan-devel \
      libgpg-error-devel \
      libseccomp-devel \
      libselinux-devel \
      ostree-devel \
      pkgconfig \
      runc \
      skopeo-containers
    chown vagrant:vagrant -R /home/vagrant
    modprobe overlay
  SHELL
  config.vm.provision "shell", privileged: true, inline: <<-SHELL
    groupadd docker
    usermod -aG docker vagrant
    systemctl start docker.service
  SHELL
  config.vm.provision "shell", privileged: false, inline: <<-SHELL
    export GOPATH=/home/vagrant/go
    SRCDIR=$GOPATH/src/github.com/kubernetes-incubator/cri-o
    echo "export GOPATH=$GOPATH" >> $HOME/.bashrc
    echo "export PATH=$PATH:$GOPATH/bin" >> $HOME/.bashrc
    # convenience: drop into the cri-o dir when login to vm
    echo "cd $SRCDIR" >> $HOME/.bashrc
    cd $SRCDIR
    make
  SHELL
end
