ocid - OCI-based implementation of Kubernetes Container Runtime Interface
=

### Status: pre-alpha

This is an implementation of the Kubernetes Container Runtime Interface (CRI) that will allow Kubernetes to directly launch and manage Open Container Initiative (OCI) containers.

The plan is to use OCI projects and best of breed libraries for different aspects:
- Runtime: runc (or any OCI runtime-spec implementation)
- Images: Image management using https://github.com/containers/image
- Storage: Storage and management of image layers using https://github.com/containers/storage
- Networking: Networking support through use of [CNI](https://github.com/containernetworking/cni)

It is currently in active development in the Kubernetes community through the [design proposal](https://github.com/kubernetes/kubernetes/pull/26788).  Questions and issues should be raised in the Kubernetes [sig-node Slack channel](https://kubernetes.slack.com/archives/sig-node).
