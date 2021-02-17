# CRI-O Adopters

Below is a non-exhaustive list of CRI-O adopters in production environments:

* [Red Hat's Openshift Container Platform](https://www.openshift.com/) uses CRI-O as the only supported CRI implementation starting with the release of OpenShift 4. CRI-O was chosen because it provides a lean, stable, simple and complete container runtime that moves in lock-step with Kubernetes, while also simplifying the support and configuration of clusters.
* [Oracle Linux Cloud Native Environment](https://www.oracle.com/it-infrastructure/software.html) has used CRI-O since release due to its tight focus on the Kubernetes CRI and its ability to manage both the [runc](https://opencontainers.org/) and [Kata Containers](https://katacontainers.io/) runtime engines.
* [SUSE CaaS Platform](https://www.suse.com/products/caas-platform) uses CRI-O
  since version 3. It has been initially supported as technology preview in
  version 3 and moved to the default Kubernetes container runtime since version
  4 as a replacement for Docker.
* [openSUSE Kubic](https://kubic.opensuse.org) is a Certified Kubernetes
  distribution based on openSUSE MicroOS. It uses CRI-O as a supported container
  runtime for Kubernetes.
