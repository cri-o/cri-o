Vagrant.configure("2") do |config|
  config.vm.box = "rockylinux/8"

  config.vm.provider :libvirt do |domain|
    domain.cpu_mode = "host-passthrough"
    domain.memory = 8192
    domain.cpus = 4
    ## 32GB is about the minimum for e2e tests
    # domain.memory = 32768
    ## 4 to 8 CPUs seems to be about the minimum for e2e tests
    # domain.cpus = 8
    domain.machine_virtual_size = 40

    config.vm.provision "shell", inline: <<-SHELL
      dnf install -y cloud-utils-growpart
      growpart /dev/vda 1
      xfs_growfs /dev/vda1
    SHELL

  end

  config.vm.provision "shell", inline: <<-SCRIPT
    dnf install python39 -y
    curl https://bootstrap.pypa.io/get-pip.py -o get-pip.py
    python3.9 get-pip.py
    python3.9 -m pip install ansible

    export PATH=/usr/local/bin:$PATH
    export GOPATH=/usr/go

    cd /vagrant
    echo "localhost" >> hosts
    ansible-galaxy install -r contrib/test/ci/requirements.yml
    ## only runs setup by default
    ansible-playbook contrib/test/ci/e2e-main.yml -i hosts -e "GOPATH=${GOPATH}" -e "TEST_AGENT=prow" --connection=local -vvv --tags setup
  SCRIPT

  config.vm.provider :google do |google, override|
    ## required environment variables
    # GOOGLE_PROJECT_ID: the project id to run under
    # SERVICE_ACCOUNT_FILE: the json credential filepath
    # USER: the username to use for ssh
    override.vm.box = "google/gce"

    google.google_project_id = ENV['GOOGLE_PROJECT_ID']
    google.google_json_key_location = ENV['SERVICE_ACCOUNT_FILE']

    google.image_project_id = "rocky-linux-cloud"
    google.image_family = 'rocky-linux-8'

    google.disk_size = 40 # gb
    google.disk_type = 'pd-ssd'
    google.machine_type = 'n2-standard-8'

    override.ssh.username = ENV['USER']
    override.ssh.private_key_path = "~/.ssh/id_rsa"

    # automatically shutdown after 8 hours
    config.vm.provision "auto-shutdown", type: "shell", run: "always", inline: "shutdown -P +480" # = 60 minutes * 8 hours
  end

end
