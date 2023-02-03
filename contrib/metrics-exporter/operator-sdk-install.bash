#!/bin/bash

set -u

set -e

KUBERNETES_VERSION=v1.22.10
GO_VERSION=1.19.1


sudo apt-get update
sudo apt-get install snap

# uninstall existing golang 

sudo rm -rvf /usr/local/go/

# install go  -> GO_VERSION

VERSION=${GO_VERSION} # go version
ARCH="amd64" # go architecture
curl -O -L "https://golang.org/dl/go${VERSION}.linux-${ARCH}.tar.gz"
ls -l

#Extract the tarball using the tar command:

sudo tar -xf "go${VERSION}.linux-${ARCH}.tar.gz"
ls -l
cd go/
ls -l
cd ..


#Set up the permissions using the chown command/chmod command:
sudo chown -R root:root ./go

sudo rm -f -R /usr/local/go

sudo mv -v go /usr/local

rm -f go*.tar.gz

cd ~


# Kustomize 

curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"  | bash

chmod a+x kustomize
sudo mv kustomize /usr/local/bin/kustomize



# Kubebuilder 

cd ~

curl -L -o kubebuilder https://go.kubebuilder.io/dl/latest/$(go env GOOS)/$(go env GOARCH)
sudo chmod +x kubebuilder && sudo mv kubebuilder /usr/local/bin/kubebuilder

# Operator SDK -> OPERATOR_SDK_VERSION

git clone https://github.com/operator-framework/operator-sdk
cd operator-sdk
git checkout ${OPERATOR_SDK_VERSION}
make install

cd ~

rm -fr operator-sdk

# Kubernetes staff 

alias k='kubectl'
alias kubectx='kubectl config use-context '
alias kubens='kubectl config set-context --current --namespace '
alias kubegc='kubectl config  get-contexts'
alias kge='kubectl get events --sort-by=.metadata.creationTimestamp'

echo alias k=\'kubectl\' >> ~/.bashrc
echo alias kubectx=\'kubectl config use-context \' >> ~/.bashrc
echo alias kubens=\'kubectl config set-context --current --namespace \' >> ~/.bashrc
echo alias kubegc=\'kubectl config  get-contexts\' >> ~/.bashrc
echo alias kge=\'kubectl get events --sort-by=.metadata.creationTimestamp\' >> ~/.bashrc


source ~/.bashrc

# krew plugins 

(
  set -x; cd "$(mktemp -d)" &&
  OS="$(uname | tr '[:upper:]' '[:lower:]')" &&
  ARCH="$(uname -m | sed -e 's/x86_64/amd64/' -e 's/\(arm\)\(64\)\?.*/\1\2/' -e 's/aarch64$/arm64/')" &&
  KREW="krew-${OS}_${ARCH}" &&
  curl -fsSLO "https://github.com/kubernetes-sigs/krew/releases/latest/download/${KREW}.tar.gz" &&
  tar zxvf "${KREW}.tar.gz" &&
  ./"${KREW}" install krew
)

#Add the $HOME/.krew/bin directory to your PATH environment variable. To do this, update your .bashrc or .zshrc file and append the following line:

export PATH="${KREW_ROOT:-$HOME/.krew}/bin:$PATH"

echo export PATH=\"${KREW_ROOT:-$HOME/.krew}/bin:$PATH\" >> ~/.bashrc


kubectl krew install split-yaml
kubectl krew install neat
kubectl krew install prune-unused
kubectl krew install get-all

#Kubectl plugin list   

kubectl krew list 

echo "Component Versions"
kustomize version
kubebuilder version
operator-sdk version
helm version 

# Install kubernetes -> KUBERNETES_VERSION

minikube start -p local --kubernetes-version=${KUBERNETES_VERSION}

kubectl get nodes -o wide

# Install VSC extentions 

code --install-extension redhat.vscode-yaml --force
code --install-extension ms-kubernetes-tools.vscode-kubernetes-tools --force

code --install-extension golang.Go --force
code --install-extension 766b.go-outliner --force

code --install-extension mhutchie.git-graph --force

code --install-extension ms-azuretools.vscode-docker  --force

code --install-extension ms-vscode.makefile-tools --force

echo "done"