# cri-o

This is the cri-o daemon as a system container.

## Building the image from source:

```
# git clone https://github.com/kubernetes-incubator/cri-o
# cd contrib/test/system-container
# docker build -t cri-o .
```

## Running the system container, with the atomic CLI:

Pull from registry into ostree:

```
# atomic pull --storage ostree $REGISTRY/cri-o
```

Or alternatively, pull from local docker:

```
# atomic pull --storage ostree docker:cri-o:latest
```

Install the container:

Currently we recommend using --system-package=no to avoid having rpmbuild create an rpm file
during installation. This flag will tell the atomic CLI to fall back to copying files to the
host instead.

```
# atomic install --system --system-package=no --name=crio ($REGISTRY)/cri-o
```

Start as a systemd service:

```
# systemctl start crio
```

Stopping the service

```
# systemctl stop crio
```

Removing the container

```
# atomic uninstall crio
```

## Binary version

You can find the image automatically built as: docker.io/crio/cri-o
