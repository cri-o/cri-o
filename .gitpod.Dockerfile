FROM gitpod/workspace-full
# Install podman from Kubic project
RUN (echo "deb https://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable/xUbuntu_20.04/ /" | sudo tee /etc/apt/sources.list.d/devel:kubic:libcontainers:stable.list) && \
	curl -L https://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable/xUbuntu_20.04/Release.key | sudo apt-key add - && \
	sudo apt-get update && \
	sudo apt-get -y upgrade && \
	sudo apt-get -y install podman 
