ocid - OCI-based implementation of Kubernetes Container Runtime Interface
=

The plan is to use OCI projects and best of breed libraries for different aspects:
- Runtime: runc (or any OCI runtime-spec compliant runtime)
- Images: Image management using https://github.com/containers/image
- Storage: Storage and management of image layers using https://github.com/containers/storage
- Networking: Networking support through use of [CNI](https://github.com/containernetworking/cni)
